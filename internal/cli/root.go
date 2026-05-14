// Package cli implements the promptlock command-line dispatcher.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// parseFlexible reorders args so flag-like tokens come before positional ones
// before calling fs.Parse. The stdlib flag package stops at the first
// non-flag, which gives a worse UX than every Cobra/Click-style CLI users are
// used to.
func parseFlexible(fs *flag.FlagSet, args []string) error {
	var flagArgs, positional []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			i++
			continue
		}
		// It's flag-shaped. --name=value is a single token.
		name := strings.TrimLeft(a, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			flagArgs = append(flagArgs, a)
			i++
			continue
		}
		f := fs.Lookup(name)
		if f != nil {
			if isBoolFlag(f) {
				flagArgs = append(flagArgs, a)
				i++
			} else if i+1 < len(args) {
				flagArgs = append(flagArgs, a, args[i+1])
				i += 2
			} else {
				flagArgs = append(flagArgs, a)
				i++
			}
		} else {
			// Unknown flag — let Parse complain.
			flagArgs = append(flagArgs, a)
			i++
		}
	}
	return fs.Parse(append(flagArgs, positional...))
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface{ IsBoolFlag() bool }
	if bf, ok := f.Value.(boolFlag); ok {
		return bf.IsBoolFlag()
	}
	return false
}

const (
	ExitOK        = 0
	ExitAssertion = 1 // eval below threshold, drift, lockfile mismatch
	ExitUsage     = 2
	ExitConfig    = 3 // missing env, bad config
	ExitProvider  = 4 // network, rate limit, auth
	ExitInternal  = 5
)

// Cmd is one CLI subcommand. Run returns the exit code.
type Cmd struct {
	Name string
	Help string
	Run  func(args []string, stdout, stderr io.Writer) int
}

// commands is the central registry. Add new subcommands here.
func commands() []Cmd {
	return []Cmd{
		{Name: "list", Help: "List discovered prompts", Run: cmdList},
		{Name: "show", Help: "Show a prompt's parsed structure", Run: cmdShow},
		{Name: "validate", Help: "Validate prompts against the format spec", Run: cmdValidate},
		{Name: "diff", Help: "Show semantic diff vs. a git ref", Run: cmdDiff},
		{Name: "eval", Help: "Run evals declared in prompt frontmatter", Run: cmdEval},
		{Name: "lock", Help: "Refresh promptlock.lock content_hash + scores", Run: cmdLock},
		{Name: "check", Help: "Verify the working tree matches promptlock.lock (CI gate)", Run: cmdCheck},
		{Name: "drift", Help: "Show drift vs. promptlock.lock; never fails (informational)", Run: cmdDrift},
		{Name: "rollback", Help: "Restore a prompt's content from the commit where it had version X", Run: cmdRollback},
		{Name: "comment", Help: "Render or post a PR comment from `eval` JSON output", Run: cmdComment},
		{Name: "version", Help: "Print version, or `version bump <id>` to bump a prompt", Run: cmdVersion},
		{Name: "log", Help: "Show git log for a prompt", Run: cmdLog},
	}
}

// Run dispatches to the requested subcommand.
func Run(promptlockVersion string, args []string, stdout, stderr io.Writer) int {
	currentVersion = promptlockVersion
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(stdout)
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	if args[0] == "-v" || args[0] == "--version" {
		fmt.Fprintln(stdout, "promptlock", promptlockVersion)
		return ExitOK
	}
	cmd := args[0]
	for _, c := range commands() {
		if c.Name == cmd {
			return c.Run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "promptlock: unknown command %q\n\n", cmd)
	printUsage(stderr)
	return ExitUsage
}

var currentVersion = "dev"

func printUsage(w io.Writer) {
	var b strings.Builder
	b.WriteString("promptlock: workflow tooling for production prompts.\n\n")
	b.WriteString("Usage:\n  promptlock <command> [flags]\n\nCommands:\n")
	for _, c := range commands() {
		fmt.Fprintf(&b, "  %-12s %s\n", c.Name, c.Help)
	}
	b.WriteString("\nRun 'promptlock <command> -h' for command-specific flags.\n")
	io.WriteString(w, b.String())
}
