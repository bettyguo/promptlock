package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/promptlock/promptlock/internal/lock"
	"github.com/promptlock/promptlock/internal/prompt"
)

// CheckResult is the per-prompt verdict.
type CheckResult struct {
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status"` // OK | DRIFT | DELETED | UNLOCKED | NEEDS EVAL
	Reason string `json:"reason,omitempty"`
}

// runCheck does the bulk of `check` and `drift` (which differ only in exit
// code semantics: drift always exits 0).
func runCheck(cwd string) ([]CheckResult, error) {
	disco, err := prompt.Discover(prompt.DiscoverOptions{Root: cwd})
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(cwd, lock.Filename)
	lf, err := lock.Load(lockPath)
	if err != nil {
		return nil, err
	}

	have := map[string]prompt.Discovered{}
	for _, d := range disco {
		have[d.IDFromPath] = d
	}
	locked := map[string]lock.Entry{}
	for _, e := range lf.Prompts {
		locked[e.ID] = e
	}

	var out []CheckResult
	seen := map[string]bool{}
	for _, d := range disco {
		seen[d.IDFromPath] = true
		entry, ok := locked[d.IDFromPath]
		if !ok {
			out = append(out, CheckResult{ID: d.IDFromPath, Path: d.RelToRoot, Status: "UNLOCKED", Reason: "new prompt; run `promptlock lock`"})
			continue
		}
		data, err := os.ReadFile(d.Path)
		if err != nil {
			out = append(out, CheckResult{ID: d.IDFromPath, Path: d.RelToRoot, Status: "DRIFT", Reason: "read failed: " + err.Error()})
			continue
		}
		curHash, err := lock.HashFile(data)
		if err != nil {
			out = append(out, CheckResult{ID: d.IDFromPath, Path: d.RelToRoot, Status: "DRIFT", Reason: "hash failed: " + err.Error()})
			continue
		}
		if curHash != entry.ContentHash {
			out = append(out, CheckResult{
				ID:     d.IDFromPath,
				Path:   d.RelToRoot,
				Status: "DRIFT",
				Reason: fmt.Sprintf("content_hash %s → %s", short(entry.ContentHash), short(curHash)),
			})
			continue
		}
		// Hash matches. Check if eval is required and present.
		p, err := prompt.Parse(d.RelToRoot, data)
		if err == nil && len(p.Frontmatter.Evals) > 0 && entry.LastEval == nil {
			out = append(out, CheckResult{
				ID:     d.IDFromPath,
				Path:   d.RelToRoot,
				Status: "NEEDS EVAL",
				Reason: "declared evals; never run (run `promptlock lock`)",
			})
			continue
		}
		out = append(out, CheckResult{ID: d.IDFromPath, Path: d.RelToRoot, Status: "OK"})
	}
	// Locked entries with no on-disk file are DELETED.
	for id, entry := range locked {
		if seen[id] {
			continue
		}
		out = append(out, CheckResult{ID: id, Path: entry.File, Status: "DELETED", Reason: "in lockfile but file is gone (run `promptlock lock` to remove)"})
	}
	return out, nil
}

func cmdCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	results, err := runCheck(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	exit := ExitOK
	for _, r := range results {
		if r.Status != "OK" {
			exit = ExitAssertion
			break
		}
	}
	emitCheckResults(stdout, results, common.format)
	return exit
}

func cmdDrift(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	results, err := runCheck(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	emitCheckResults(stdout, results, common.format)
	// Always exit 0 — drift is informational.
	return ExitOK
}

func emitCheckResults(w io.Writer, results []CheckResult, format string) {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	if len(results) == 0 {
		fmt.Fprintln(w, "no prompts found")
		return
	}
	fmt.Fprintf(w, "%-12s %-32s %s\n", "status", "id", "reason")
	for _, r := range results {
		fmt.Fprintf(w, "%-12s %-32s %s\n", r.Status, r.ID, r.Reason)
	}
}

func short(h string) string {
	if len(h) <= 14 {
		return h
	}
	return h[:14] + "…"
}
