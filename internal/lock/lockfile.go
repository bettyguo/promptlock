package lock

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the on-disk lockfile schema we write. Bump on any
// backward-incompatible change.
const SchemaVersion = 1

// Filename is the canonical lockfile name at the repo root.
const Filename = "promptlock.lock"

// Lockfile is the parsed in-memory representation.
type Lockfile struct {
	SchemaVersion int       `yaml:"schema_version"`
	GeneratedBy   string    `yaml:"generated_by,omitempty"`
	GeneratedAt   time.Time `yaml:"generated_at,omitempty"`
	Prompts       []Entry   `yaml:"prompts"`
}

// Entry is one prompt's lockfile record.
type Entry struct {
	ID          string    `yaml:"id"`
	Version     string    `yaml:"version"`
	File        string    `yaml:"file"`
	ContentHash string    `yaml:"content_hash"`
	LastEval    *EvalInfo `yaml:"last_eval,omitempty"`
}

// EvalInfo records the most recent eval execution for a prompt.
type EvalInfo struct {
	Provider          string         `yaml:"provider"`
	Model             string         `yaml:"model"`
	Temperature       *float64       `yaml:"temperature,omitempty"`
	Seed              *int64         `yaml:"seed,omitempty"`
	Timestamp         time.Time      `yaml:"timestamp"`
	PromptlockVersion string         `yaml:"promptlock_version,omitempty"`
	Datasets          []DatasetInfo  `yaml:"datasets"`
	Scores            []ScoreInfo    `yaml:"scores"`
	Tokens            *TokensInfo    `yaml:"tokens,omitempty"`
}

// DatasetInfo records the dataset(s) used for the eval.
type DatasetInfo struct {
	Path string `yaml:"path"`
	Hash string `yaml:"hash"`
	Rows int    `yaml:"rows"`
}

// ScoreInfo records one (dataset, metric) → score.
type ScoreInfo struct {
	Dataset          string  `yaml:"dataset"`
	Metric           string  `yaml:"metric"`
	MetricConfigHash string  `yaml:"metric_config_hash,omitempty"`
	Score            float64 `yaml:"score"`
	Threshold        float64 `yaml:"threshold,omitempty"`
	Aggregate        string  `yaml:"aggregate,omitempty"`
}

// TokensInfo records aggregate token counts.
type TokensInfo struct {
	Input  int `yaml:"input"`
	Output int `yaml:"output"`
}

// New returns an empty Lockfile with the current schema version stamped.
func New(promptlockVersion string) *Lockfile {
	return &Lockfile{
		SchemaVersion: SchemaVersion,
		GeneratedBy:   "promptlock " + promptlockVersion,
		GeneratedAt:   time.Now().UTC().Truncate(time.Second),
	}
}

// Load reads the lockfile at path. If the file does not exist, returns an
// empty Lockfile and a nil error (no lockfile yet is a valid state).
func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Lockfile{SchemaVersion: SchemaVersion}, nil
		}
		return nil, fmt.Errorf("lockfile: read %s: %w", path, err)
	}
	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("lockfile: parse %s: %w", path, err)
	}
	if lf.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf(
			"lockfile schema_version %d is newer than this promptlock (supports up to %d); upgrade promptlock",
			lf.SchemaVersion, SchemaVersion,
		)
	}
	if lf.SchemaVersion == 0 {
		lf.SchemaVersion = SchemaVersion
	}
	return &lf, nil
}

// Save writes the lockfile atomically (write to .tmp, fsync, rename).
// Entries are sorted by ID for stable diffs.
func (lf *Lockfile) Save(path string) error {
	sort.SliceStable(lf.Prompts, func(i, j int) bool { return lf.Prompts[i].ID < lf.Prompts[j].ID })

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(lf); err != nil {
		return fmt.Errorf("lockfile: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("lockfile: encoder close: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("lockfile: mkdir: %w", err)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("lockfile: open tmp: %w", err)
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("lockfile: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("lockfile: fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("lockfile: close: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("lockfile: rename: %w", err)
	}
	return nil
}

// Find returns the entry for id (and true) or zero (and false).
func (lf *Lockfile) Find(id string) (Entry, bool) {
	for _, e := range lf.Prompts {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Upsert replaces or inserts the entry for e.ID.
func (lf *Lockfile) Upsert(e Entry) {
	for i, existing := range lf.Prompts {
		if existing.ID == e.ID {
			lf.Prompts[i] = e
			return
		}
	}
	lf.Prompts = append(lf.Prompts, e)
}

// Remove deletes the entry for id (no-op if absent). Returns true when removed.
func (lf *Lockfile) Remove(id string) bool {
	for i, e := range lf.Prompts {
		if e.ID == id {
			lf.Prompts = append(lf.Prompts[:i], lf.Prompts[i+1:]...)
			return true
		}
	}
	return false
}
