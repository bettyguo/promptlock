package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/promptlock/promptlock/internal/prompt"
	"github.com/promptlock/promptlock/internal/version"
)

// cmdVersion implements:
//
//	promptlock version              -> print promptlock version
//	promptlock version bump <id>    -> bump a prompt's version (--major|--minor|--patch)
func cmdVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "promptlock", currentVersion)
		return ExitOK
	}
	if args[0] == "bump" {
		return cmdVersionBump(args[1:], stdout, stderr)
	}
	// Allow `promptlock version --short` for parity with future tools.
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	short := fs.Bool("short", false, "print just the version number")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	if *short {
		fmt.Fprintln(stdout, currentVersion)
	} else {
		fmt.Fprintln(stdout, "promptlock", currentVersion)
	}
	return ExitOK
}

func cmdVersionBump(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("version bump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	major := fs.Bool("major", false, "bump major (X.0.0)")
	minor := fs.Bool("minor", false, "bump minor (x.X.0)")
	patch := fs.Bool("patch", false, "bump patch (x.x.X)")
	dryRun := fs.Bool("dry-run", false, "print the new version without writing the file")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: promptlock version bump <prompt-id> [--major|--minor|--patch]")
		return ExitUsage
	}
	count := 0
	for _, b := range []bool{*major, *minor, *patch} {
		if b {
			count++
		}
	}
	if count == 0 {
		*patch = true
	} else if count > 1 {
		fmt.Fprintln(stderr, "version bump: pick exactly one of --major, --minor, --patch")
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
	d := disco[0]
	p, err := prompt.LoadDiscovered(d)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	v, err := version.Parse(p.Frontmatter.Version)
	if err != nil {
		fmt.Fprintf(stderr, "promptlock: current version %q is not valid semver: %v\n", p.Frontmatter.Version, err)
		return ExitConfig
	}
	var next version.Version
	switch {
	case *major:
		next = v.BumpMajor()
	case *minor:
		next = v.BumpMinor()
	default:
		next = v.BumpPatch()
	}
	newV := next.String()

	if *dryRun {
		fmt.Fprintf(stdout, "%s: %s → %s (dry-run; file not modified)\n", id, v, newV)
		return ExitOK
	}

	// Rewrite the file: replace the version line in the frontmatter only.
	newBytes, err := rewriteVersion(p.Raw, newV)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if err := os.WriteFile(d.Path, newBytes, 0o644); err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	fmt.Fprintf(stdout, "%s: %s → %s\n", id, v, newV)
	return ExitOK
}

// rewriteVersion does an in-place replace of the version line in the
// frontmatter. Re-emitting via yaml.Marshal would reorder keys and drop
// comments, so we touch only the matched line.
func rewriteVersion(raw []byte, newVersion string) ([]byte, error) {
	// Find frontmatter close.
	text := string(raw)
	const delim = "---"
	idx1 := strings.Index(text, delim)
	if idx1 < 0 {
		return nil, fmt.Errorf("no frontmatter delimiter found")
	}
	// Skip past first delim line.
	nl1 := strings.IndexByte(text[idx1:], '\n')
	if nl1 < 0 {
		return nil, fmt.Errorf("malformed frontmatter")
	}
	yamlStart := idx1 + nl1 + 1
	// Find close.
	idx2 := strings.Index(text[yamlStart:], "\n"+delim)
	if idx2 < 0 {
		return nil, fmt.Errorf("frontmatter never closes")
	}
	yamlEnd := yamlStart + idx2 + 1 // include the leading newline so close-delim stays put
	yamlBlock := text[yamlStart:yamlEnd]

	// Find version line.
	lines := strings.Split(yamlBlock, "\n")
	versionLine := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "version:") || strings.HasPrefix(trim, "version :") {
			versionLine = i
			break
		}
	}
	if versionLine < 0 {
		return nil, fmt.Errorf("no `version:` key in frontmatter")
	}
	// Preserve indent and quote style.
	original := lines[versionLine]
	indentLen := len(original) - len(strings.TrimLeft(original, " \t"))
	indent := original[:indentLen]
	useDoubleQuotes := strings.Contains(original, "\"")
	useSingleQuotes := strings.Contains(original, "'") && !useDoubleQuotes
	var quoted string
	switch {
	case useDoubleQuotes:
		quoted = "\"" + newVersion + "\""
	case useSingleQuotes:
		quoted = "'" + newVersion + "'"
	default:
		quoted = "\"" + newVersion + "\""
	}
	lines[versionLine] = indent + "version: " + quoted

	newYAML := strings.Join(lines, "\n")
	out := text[:yamlStart] + newYAML + text[yamlEnd:]
	return []byte(out), nil
}
