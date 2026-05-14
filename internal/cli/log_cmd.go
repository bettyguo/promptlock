package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/promptlock/promptlock/internal/git"
	"github.com/promptlock/promptlock/internal/prompt"
)

func cmdLog(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	max := fs.Int("max", 20, "max commits to show (0 = no limit)")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: promptlock log <prompt-id>")
		return ExitUsage
	}
	id := fs.Arg(0)
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	disco, err := prompt.Discover(prompt.DiscoverOptions{
		Root:     cwd,
		IDFilter: func(d string) bool { return d == id },
	})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(disco) == 0 {
		fmt.Fprintf(stderr, "promptlock: no prompt with id %q found\n", id)
		return ExitConfig
	}
	g := git.New(cwd)
	if !g.IsRepo() {
		fmt.Fprintln(stderr, "promptlock: not in a git work tree")
		return ExitConfig
	}
	// `git log -- path` resolves the path relative to the invocation cwd
	// (unlike `git show ref:path`, which is repo-root-relative). RelToRoot is
	// already relative to cwd.
	commits, err := g.LogForFile(disco[0].RelToRoot, *max)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if common.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(commits)
		return ExitOK
	}
	if len(commits) == 0 {
		fmt.Fprintln(stdout, "(no commits affecting this file)")
		return ExitOK
	}
	for _, c := range commits {
		fmt.Fprintf(stdout, "%s  %s  %s  %s\n", c.SHA[:8], c.AuthorDate, c.Author, c.Subject)
	}
	return ExitOK
}
