package lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/promptlock/promptlock/internal/prompt"
)

// CheckStatus is the per-entry verdict from Check.
type CheckStatus string

const (
	StatusOK           CheckStatus = "OK"
	StatusDrift        CheckStatus = "DRIFT"          // hash differs vs disk
	StatusDeleted      CheckStatus = "DELETED"        // locked but not on disk
	StatusUnlocked     CheckStatus = "UNLOCKED"       // on disk but not locked
	StatusNeedsEval    CheckStatus = "NEEDS_EVAL"     // declares evals but lockfile has no last_eval
	StatusDatasetDrift CheckStatus = "DATASET_DRIFT"  // dataset bytes changed since lock
)

// CheckResult is one row of the check report.
type CheckResult struct {
	ID     string      `json:"id"`
	File   string      `json:"file,omitempty"`
	Status CheckStatus `json:"status"`
	Reason string      `json:"reason,omitempty"`
}

// Check validates the lockfile against current disk state under repoRoot.
// Returns one result per (locked OR on-disk) prompt id, sorted by id.
func Check(repoRoot string) ([]CheckResult, error) {
	lf, err := Load(filepath.Join(repoRoot, Filename))
	if err != nil {
		return nil, err
	}
	disco, err := prompt.Discover(prompt.DiscoverOptions{Root: repoRoot})
	if err != nil {
		return nil, err
	}

	onDisk := map[string]prompt.Discovered{}
	for _, d := range disco {
		onDisk[d.IDFromPath] = d
	}
	locked := map[string]Entry{}
	for _, e := range lf.Prompts {
		locked[e.ID] = e
	}

	ids := map[string]struct{}{}
	for k := range onDisk {
		ids[k] = struct{}{}
	}
	for k := range locked {
		ids[k] = struct{}{}
	}

	var out []CheckResult
	for id := range ids {
		entry, hasEntry := locked[id]
		disk, hasDisk := onDisk[id]
		switch {
		case !hasEntry && hasDisk:
			out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusUnlocked,
				Reason: "prompt exists but is not in lockfile (run `promptlock lock`)"})
		case hasEntry && !hasDisk:
			out = append(out, CheckResult{ID: id, File: entry.File, Status: StatusDeleted,
				Reason: "lockfile references a file not present on disk"})
		default:
			data, err := os.ReadFile(disk.Path)
			if err != nil {
				out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusDrift,
					Reason: "could not read file: " + err.Error()})
				continue
			}
			h, err := HashFile(data)
			if err != nil {
				out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusDrift,
					Reason: "hash failed: " + err.Error()})
				continue
			}
			if h != entry.ContentHash {
				out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusDrift,
					Reason: fmt.Sprintf("file edited (%s → %s)", short(entry.ContentHash), short(h))})
				continue
			}
			p, err := prompt.Parse(disk.RelToRoot, data)
			if err == nil && len(p.Frontmatter.Evals) > 0 && entry.LastEval == nil {
				out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusNeedsEval,
					Reason: "prompt declares evals but lockfile has no last_eval"})
				continue
			}
			if entry.LastEval != nil {
				if d := datasetDrift(repoRoot, entry); d != "" {
					out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusDatasetDrift,
						Reason: d})
					continue
				}
			}
			out = append(out, CheckResult{ID: id, File: disk.RelToRoot, Status: StatusOK})
		}
	}
	sortByID(out)
	return out, nil
}

func datasetDrift(repoRoot string, e Entry) string {
	for _, d := range e.LastEval.Datasets {
		path := d.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Sprintf("dataset %s referenced by lockfile is missing", d.Path)
			}
			return fmt.Sprintf("dataset %s read failed: %v", d.Path, err)
		}
		got := HashDataset(data)
		if got != d.Hash {
			return fmt.Sprintf("dataset %s edited (%s → %s)", d.Path, short(d.Hash), short(got))
		}
	}
	return ""
}

func short(h string) string {
	if len(h) > 14 {
		return h[:14] + "..."
	}
	return h
}

func sortByID(rs []CheckResult) {
	for i := 1; i < len(rs); i++ {
		j := i
		for j > 0 && rs[j-1].ID > rs[j].ID {
			rs[j-1], rs[j] = rs[j], rs[j-1]
			j--
		}
	}
}

// HasFailures returns true if any non-OK status is present.
func HasFailures(results []CheckResult) bool {
	for _, r := range results {
		if r.Status != StatusOK {
			return true
		}
	}
	return false
}
