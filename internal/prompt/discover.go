package prompt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PromptExtension is the extension (including leading dot pair) for prompt files.
const PromptExtension = ".prompt.md"

// DiscoverOptions controls how Discover walks the filesystem.
type DiscoverOptions struct {
	// Root is the directory to walk. Required.
	Root string
	// PromptsDir is the subdirectory under Root that holds prompts. Default "prompts".
	PromptsDir string
	// IDFilter, if non-nil, returns true to keep a discovered prompt's ID.
	IDFilter func(id string) bool
}

// Discovered is one discovered prompt file (not yet fully parsed).
type Discovered struct {
	Path        string // absolute or root-relative; same form as input Root
	RelToRoot   string // path relative to Root, with forward slashes
	IDFromPath  string // derived: relative path under PromptsDir minus extension
	PromptsRoot string // resolved promptsRoot used for id derivation
}

// Discover walks Root for *.prompt.md files under PromptsDir.
func Discover(opts DiscoverOptions) ([]Discovered, error) {
	if opts.Root == "" {
		return nil, errors.New("discover: Root is required")
	}
	dir := opts.PromptsDir
	if dir == "" {
		dir = "prompts"
	}
	root := filepath.Join(opts.Root, dir)
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // empty result; not an error
		}
		return nil, fmt.Errorf("discover: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("discover: %s is not a directory", root)
	}

	var out []Discovered
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// skip hidden subdirs (e.g. .git inside prompts/, just in case)
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return fs.SkipDir
			}
			return nil
		}
		// We match the compound extension `.prompt.md`. filepath.Ext only returns `.md`.
		if !strings.HasSuffix(d.Name(), PromptExtension) {
			return nil
		}
		rel, err := filepath.Rel(opts.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// Derive ID from the path under PromptsDir.
		relUnderPrompts, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(filepath.ToSlash(relUnderPrompts), PromptExtension)
		if opts.IDFilter != nil && !opts.IDFilter(id) {
			return nil
		}
		out = append(out, Discovered{
			Path:        path,
			RelToRoot:   rel,
			IDFromPath:  id,
			PromptsRoot: dir,
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("discover: walk: %w", walkErr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IDFromPath < out[j].IDFromPath })
	return out, nil
}

// LoadDiscovered reads and parses a discovered prompt file.
func LoadDiscovered(d Discovered) (*Prompt, error) {
	data, err := os.ReadFile(d.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", d.Path, err)
	}
	return Parse(d.RelToRoot, data)
}
