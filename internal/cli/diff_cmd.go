package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/promptlock/promptlock/internal/diff"
	"github.com/promptlock/promptlock/internal/git"
	"github.com/promptlock/promptlock/internal/prompt"
)

func cmdDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	against := fs.String("against", "HEAD", "git ref to diff against (HEAD, branch, sha, or A..B)")
	all := fs.Bool("changed", false, "diff all changed prompts (vs --against)")
	color := fs.Bool("color", false, "force ANSI color in human output (auto-detected for TTY)")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	useColor := *color || isTTY(stdout)

	var ids []string
	if *all {
		ids = nil // means "all"
	} else if fs.NArg() == 1 {
		ids = []string{fs.Arg(0)}
	} else {
		fmt.Fprintln(stderr, "usage: promptlock diff <prompt-id> [--against <ref>]\n   or: promptlock diff --changed [--against <ref>]")
		return ExitUsage
	}

	g := git.New(cwd)
	if !g.IsRepo() {
		fmt.Fprintln(stderr, "promptlock: not in a git work tree (diff requires git)")
		return ExitConfig
	}
	prefix, err := g.Prefix()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	disco, err := prompt.Discover(prompt.DiscoverOptions{
		Root: cwd,
		IDFilter: func(id string) bool {
			if len(ids) == 0 {
				return true
			}
			for _, want := range ids {
				if id == want {
					return true
				}
			}
			return false
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(disco) == 0 {
		if len(ids) > 0 {
			fmt.Fprintf(stderr, "promptlock: no prompt with id %q found\n", ids[0])
		} else {
			fmt.Fprintln(stdout, "no prompts found")
		}
		return ExitConfig
	}

	type fileResult struct {
		Result *diff.Result `json:"result"`
		Path   string       `json:"path"`
	}
	var results []fileResult
	for _, d := range disco {
		afterPrompt, err := prompt.LoadDiscovered(d)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
			continue
		}
		// Load `before` from git ref. ShowFile expects a path relative to the
		// git repo root, so prefix the discovery-relative path with the cwd's
		// position inside the repo (`git rev-parse --show-prefix`).
		gitRelPath := prefix + d.RelToRoot
		beforeBytes, err := g.ShowFile(*against, gitRelPath)
		var beforePrompt *prompt.Prompt
		switch {
		case err == nil:
			bp, perr := prompt.Parse(d.RelToRoot, beforeBytes)
			if perr != nil {
				fmt.Fprintf(stderr, "warning: parse %s@%s: %v\n", d.RelToRoot, *against, perr)
				continue
			}
			beforePrompt = bp
		case git.IsPathNotInRef(err):
			beforePrompt = nil
		default:
			var pe *git.PathNotInRefError
			if errors.As(err, &pe) {
				beforePrompt = nil
			} else {
				fmt.Fprintf(stderr, "warning: git show %s@%s: %v\n", d.RelToRoot, *against, err)
				continue
			}
		}

		r := diff.Diff(beforePrompt, afterPrompt)
		r.AfterRef = "WORKING_TREE"
		r.BeforeRef = *against
		if !*all && !r.HasChanges {
			// Single-file mode: show "no changes" message.
			fmt.Fprintf(stdout, "%s: no semantic changes vs %s\n", d.IDFromPath, *against)
			return ExitOK
		}
		if r.HasChanges {
			results = append(results, fileResult{Result: r, Path: d.RelToRoot})
		}
	}

	if common.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return ExitOK
	}
	if len(results) == 0 {
		fmt.Fprintf(stdout, "no semantic changes vs %s\n", *against)
		return ExitOK
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "  %s\n", r.Path)
		diff.Render(stdout, r.Result, useColor)
		fmt.Fprintln(stdout)
	}
	return ExitOK
}

// isTTY reports whether w is a terminal. Best-effort: only true when w is
// os.Stdout and stdout is a character device.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

