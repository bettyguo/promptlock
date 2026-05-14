package diff

import (
	"fmt"
	"io"
	"strings"
)

// Render writes a human-friendly representation of the diff to w. When useColor
// is true, ANSI color codes are emitted; otherwise [-...-] / [+...+] bracket
// markers are used (suitable for non-TTY logs and CI).
func Render(w io.Writer, r *Result, useColor bool) {
	if r == nil {
		return
	}
	header := r.PromptID
	if r.VersionChange != nil {
		header = fmt.Sprintf("%s  %s → %s", r.PromptID, dispVersion(r.VersionChange.Before), dispVersion(r.VersionChange.After))
	}
	fmt.Fprintln(w, header)

	if r.BeforeMissing {
		fmt.Fprintln(w, "  (new file)")
	}
	if r.AfterMissing {
		fmt.Fprintln(w, "  (file deleted)")
	}

	if !r.HasChanges {
		fmt.Fprintln(w, "  (no semantic changes)")
		return
	}

	if len(r.FrontmatterChanges) > 0 {
		fmt.Fprintln(w, "  frontmatter:")
		for _, c := range r.FrontmatterChanges {
			renderFmChange(w, c)
		}
	}

	if len(r.InputsChanges) > 0 {
		fmt.Fprintln(w, "  inputs:")
		for _, ic := range r.InputsChanges {
			renderInputChange(w, ic)
		}
	}

	for _, sec := range r.BodyChanges {
		if !sec.Changed {
			continue
		}
		fmt.Fprintf(w, "  body — # %s:\n", strings.Title(sec.Section))
		fmt.Fprint(w, "    ")
		renderOps(w, sec.Operations, useColor)
		fmt.Fprintln(w)
	}
}

func dispVersion(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}

func renderFmChange(w io.Writer, c FrontmatterChange) {
	switch c.Kind {
	case "added":
		fmt.Fprintf(w, "    + %s: %v\n", c.Path, c.After)
	case "removed":
		fmt.Fprintf(w, "    - %s: %v\n", c.Path, c.Before)
	default:
		fmt.Fprintf(w, "    %s: %v → %v\n", c.Path, c.Before, c.After)
	}
}

func renderInputChange(w io.Writer, ic InputChange) {
	switch ic.Kind {
	case "added":
		fmt.Fprintf(w, "    + %s (%v)\n", ic.Name, summaryInput(ic.After))
	case "removed":
		fmt.Fprintf(w, "    - %s\n", ic.Name)
	default:
		fmt.Fprintf(w, "    ~ %s: %v → %v\n", ic.Name, summaryInput(ic.Before), summaryInput(ic.After))
	}
}

func summaryInput(m map[string]any) string {
	if m == nil {
		return "(none)"
	}
	t, _ := m["type"].(string)
	if t == "" {
		t = "string"
	}
	req := ""
	if r, ok := m["required"].(bool); ok && !r {
		req = ", optional"
	}
	def := ""
	if d, ok := m["default"]; ok {
		def = fmt.Sprintf(", default=%v", d)
	}
	return t + req + def
}

func renderOps(w io.Writer, ops []EditOp, useColor bool) {
	for _, op := range ops {
		switch op.Op {
		case "equal":
			fmt.Fprint(w, op.Text)
		case "insert":
			if useColor {
				fmt.Fprintf(w, "\x1b[32m%s\x1b[0m", op.Text)
			} else {
				fmt.Fprintf(w, "[+%s+]", op.Text)
			}
		case "delete":
			if useColor {
				fmt.Fprintf(w, "\x1b[31m%s\x1b[0m", op.Text)
			} else {
				fmt.Fprintf(w, "[-%s-]", op.Text)
			}
		}
	}
}
