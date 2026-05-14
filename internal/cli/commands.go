package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/promptlock/promptlock/internal/prompt"
)

// commonFlags are flags every subcommand accepts.
type commonFlags struct {
	cwd    string
	format string // "human" | "json"
}

// addCommon attaches --cwd and --format to the given flag set.
func addCommon(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.cwd, "cwd", "", "operate from this directory (default: current directory)")
	fs.StringVar(&c.format, "format", "human", "output format: human | json")
	return c
}

// resolveCwd returns cwd or the process working dir.
func (c *commonFlags) resolveCwd() (string, error) {
	if c.cwd != "" {
		return c.cwd, nil
	}
	return os.Getwd()
}

func cmdList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	tag := fs.String("tag", "", "filter by tag")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	discovered, err := prompt.Discover(prompt.DiscoverOptions{Root: cwd})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	type row struct {
		ID          string   `json:"id"`
		Version     string   `json:"version"`
		Description string   `json:"description,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		Path        string   `json:"path"`
	}
	var rows []row
	for _, d := range discovered {
		p, err := prompt.LoadDiscovered(d)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
			continue
		}
		if *tag != "" {
			if !contains(p.Frontmatter.Tags, *tag) {
				continue
			}
		}
		rows = append(rows, row{
			ID:          firstNonEmpty(p.Frontmatter.ID, d.IDFromPath),
			Version:     p.Frontmatter.Version,
			Description: p.Frontmatter.Description,
			Tags:        p.Frontmatter.Tags,
			Path:        d.RelToRoot,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })

	if common.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
		return ExitOK
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no prompts found (looked under prompts/**/*.prompt.md)")
		return ExitOK
	}
	maxID, maxVer := 2, 7
	for _, r := range rows {
		if l := len(r.ID); l > maxID {
			maxID = l
		}
		if l := len(r.Version); l > maxVer {
			maxVer = l
		}
	}
	fmt.Fprintf(stdout, "%-*s  %-*s  %s\n", maxID, "id", maxVer, "version", "description")
	fmt.Fprintf(stdout, "%s  %s  %s\n", strings.Repeat("-", maxID), strings.Repeat("-", maxVer), strings.Repeat("-", 11))
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-*s  %-*s  %s\n", maxID, r.ID, maxVer, r.Version, r.Description)
	}
	return ExitOK
}

func cmdShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: promptlock show <prompt-id>")
		return ExitUsage
	}
	id := fs.Arg(0)
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	discovered, err := prompt.Discover(prompt.DiscoverOptions{
		Root:     cwd,
		IDFilter: func(d string) bool { return d == id },
	})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(discovered) == 0 {
		fmt.Fprintf(stderr, "promptlock: no prompt with id %q found\n", id)
		return ExitConfig
	}
	p, err := prompt.LoadDiscovered(discovered[0])
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	if common.format == "json" {
		out := map[string]any{
			"path":        p.Path,
			"frontmatter": p.Frontmatter,
			"body": map[string]string{
				"system":    p.Body.System,
				"user":      p.Body.User,
				"assistant": p.Body.Assistant,
			},
			"refs": p.VarRefs(),
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return ExitOK
	}
	// Human: just dump the file.
	stdout.Write(p.Raw)
	if len(p.Raw) > 0 && p.Raw[len(p.Raw)-1] != '\n' {
		fmt.Fprintln(stdout)
	}
	return ExitOK
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
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

	// If positional args given, validate just those IDs; else validate all.
	var idFilter func(string) bool
	if fs.NArg() > 0 {
		want := map[string]bool{}
		for _, id := range fs.Args() {
			want[id] = true
		}
		idFilter = func(id string) bool { return want[id] }
	}
	discovered, err := prompt.Discover(prompt.DiscoverOptions{Root: cwd, IDFilter: idFilter})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(discovered) == 0 {
		fmt.Fprintln(stderr, "promptlock: no prompts to validate")
		return ExitConfig
	}

	type result struct {
		Path     string          `json:"path"`
		ID       string          `json:"id,omitempty"`
		Issues   []prompt.Issue  `json:"issues,omitempty"`
		HasError bool            `json:"has_error"`
		ParseErr string          `json:"parse_error,omitempty"`
	}
	var results []result
	totalErrors := 0
	for _, d := range discovered {
		p, err := prompt.LoadDiscovered(d)
		if err != nil {
			results = append(results, result{Path: d.RelToRoot, ParseErr: err.Error(), HasError: true})
			totalErrors++
			continue
		}
		v := prompt.Validate(p, "prompts")
		r := result{
			Path:     d.RelToRoot,
			ID:       p.Frontmatter.ID,
			Issues:   v.Issues,
			HasError: v.HasError,
		}
		if v.HasError {
			totalErrors++
		}
		results = append(results, r)
	}

	if common.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"total_prompts": len(results),
			"with_errors":   totalErrors,
			"results":       results,
		})
		if totalErrors > 0 {
			return ExitAssertion
		}
		return ExitOK
	}

	for _, r := range results {
		if r.ParseErr != "" {
			fmt.Fprintf(stdout, "✗ %s\n  parse error: %s\n", r.Path, r.ParseErr)
			continue
		}
		marker := "✓"
		if r.HasError {
			marker = "✗"
		}
		fmt.Fprintf(stdout, "%s %s (%s)\n", marker, r.Path, r.ID)
		for _, iss := range r.Issues {
			fmt.Fprintf(stdout, "    [%s] %s: %s\n", iss.Severity, iss.Field, iss.Message)
		}
	}
	fmt.Fprintf(stdout, "\n%d prompt(s) validated, %d with errors\n", len(results), totalErrors)
	if totalErrors > 0 {
		return ExitAssertion
	}
	return ExitOK
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
