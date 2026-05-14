package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/promptlock/promptlock/internal/git"
	"github.com/promptlock/promptlock/internal/prompt"
	"github.com/promptlock/promptlock/internal/version"
)

// cmdRollback restores a prompt to the content it had at a previous version.
// Walks `git log` for the file, finds the commit where the frontmatter version
// matched, copies that content into the working tree, and bumps patch forward
// so the rollback is a normal forward commit rather than a history rewrite.
// Refuses to clobber a dirty working tree without --force.
func cmdRollback(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	force := fs.Bool("force", false, "rollback even if working tree is dirty for this file")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: promptlock rollback <prompt-id> <version>")
		return ExitUsage
	}
	id, target := fs.Arg(0), fs.Arg(1)
	if _, err := version.Parse(target); err != nil {
		fmt.Fprintf(stderr, "promptlock: target version %q is not valid semver: %v\n", target, err)
		return ExitUsage
	}

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
		fmt.Fprintf(stderr, "promptlock: no prompt with id %q\n", id)
		return ExitConfig
	}
	d := disco[0]
	g := git.New(cwd)
	if !g.IsRepo() {
		fmt.Fprintln(stderr, "promptlock: not in a git work tree")
		return ExitConfig
	}
	prefix, err := g.Prefix()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	gitPath := prefix + d.RelToRoot

	if !*force {
		// Refuse if file is dirty in working tree.
		out, err := g.Run("status", "--porcelain", "--", gitPath)
		if err != nil {
			fmt.Fprintln(stderr, "promptlock:", err)
			return ExitConfig
		}
		if len(strings.TrimSpace(string(out))) > 0 {
			fmt.Fprintf(stderr, "promptlock: %s has uncommitted changes; commit/stash first or pass --force\n", d.RelToRoot)
			return ExitConfig
		}
	}

	// Walk commits affecting this file; for each, fetch the file and parse to
	// find the one whose frontmatter has version == target. Most-recent first.
	commits, err := g.LogForFile(d.RelToRoot, 0)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	var found string
	for _, c := range commits {
		data, err := g.ShowFile(c.SHA, gitPath)
		if err != nil {
			continue
		}
		p, err := prompt.Parse(d.RelToRoot, data)
		if err != nil {
			continue
		}
		if p.Frontmatter.Version == target {
			found = c.SHA
			break
		}
	}
	if found == "" {
		fmt.Fprintf(stderr, "promptlock: no commit found where %s had version %q\n", id, target)
		return ExitConfig
	}

	data, err := g.ShowFile(found, gitPath)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	// Bump patch forward so the rollback isn't a step back in version space.
	currentData, err := os.ReadFile(d.Path)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	curParsed, err := prompt.Parse(d.RelToRoot, currentData)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	curV, err := version.Parse(curParsed.Frontmatter.Version)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	bumped := curV.BumpPatch().String()
	rewritten, err := rewriteVersion(data, bumped)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	// Need full-disk path; d.Path already absolute via discovery.
	target_path := d.Path
	if !filepath.IsAbs(target_path) {
		target_path = filepath.Join(cwd, d.RelToRoot)
	}
	if err := os.WriteFile(target_path, rewritten, 0o644); err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	fmt.Fprintf(stdout, "%s rolled back to content from commit %s (now version %s)\n", id, found[:8], bumped)
	fmt.Fprintln(stdout, "Review the change with `git diff`, then `git commit`.")
	return ExitOK
}
