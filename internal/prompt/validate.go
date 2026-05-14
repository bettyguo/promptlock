package prompt

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/promptlock/promptlock/internal/version"
)

// Issue is a single validation finding.
type Issue struct {
	Severity string // "error" | "warning"
	Field    string // dotted path, e.g. "frontmatter.id"
	Message  string
}

func (i Issue) Error() string {
	return fmt.Sprintf("[%s] %s: %s", i.Severity, i.Field, i.Message)
}

// ValidationResult aggregates issues for one prompt.
type ValidationResult struct {
	Path     string
	Issues   []Issue
	HasError bool
}

// Add appends an issue and updates HasError.
func (r *ValidationResult) Add(sev, field, msg string) {
	r.Issues = append(r.Issues, Issue{Severity: sev, Field: field, Message: msg})
	if sev == "error" {
		r.HasError = true
	}
}

// validMetrics are the metric IDs accepted in the format spec.
var validMetrics = map[string]bool{
	"exact_match": true,
	"contains":    true,
	"regex":       true,
	"json_schema": true,
	"llm_judge":   true,
	"custom":      true,
}

// validInputTypes are accepted by the format spec.
var validInputTypes = map[string]bool{
	"string": true, "integer": true, "float": true, "boolean": true, "array": true, "object": true,
}

// Validate runs the v1 validation rules on a parsed prompt.
//
// promptsRoot, if non-empty, is the root directory under which the prompt's
// path will be checked against its declared id. Pass "" to skip the id-vs-path
// cross-check (e.g. when validating in-memory data not anchored to a repo).
func Validate(p *Prompt, promptsRoot string) *ValidationResult {
	r := &ValidationResult{Path: p.Path}
	fm := &p.Frontmatter

	// Required fields.
	if fm.ID == "" {
		r.Add("error", "frontmatter.id", "missing required field")
	} else if !isValidID(fm.ID) {
		r.Add("error", "frontmatter.id", "must be path-like, segments of [a-z0-9_-] separated by '/'")
	}
	if fm.Version == "" {
		r.Add("error", "frontmatter.version", "missing required field")
	} else {
		v, err := version.Parse(fm.Version)
		if err != nil {
			r.Add("error", "frontmatter.version", err.Error())
		} else {
			fm.ParsedVersion = v
		}
	}
	if fm.Model == "" {
		r.Add("error", "frontmatter.model", "missing required field")
	}

	// id-vs-path cross-check.
	if promptsRoot != "" && fm.ID != "" && p.Path != "" {
		want := filepath.ToSlash(filepath.Join(promptsRoot, fm.ID+".prompt.md"))
		got := filepath.ToSlash(p.Path)
		if !strings.EqualFold(got, want) {
			r.Add("error", "frontmatter.id",
				fmt.Sprintf("id %q does not match file path %q (expected %q)", fm.ID, got, want))
		}
	}

	// Inputs.
	seenInputs := map[string]bool{}
	for i, in := range fm.Inputs {
		field := fmt.Sprintf("frontmatter.inputs[%d]", i)
		if in.Name == "" {
			r.Add("error", field+".name", "missing")
			continue
		}
		if !isValidIdentifier(in.Name) {
			r.Add("error", field+".name",
				fmt.Sprintf("invalid identifier %q (need [A-Za-z_][A-Za-z0-9_]*)", in.Name))
		}
		if seenInputs[in.Name] {
			r.Add("error", field+".name", fmt.Sprintf("duplicate input %q", in.Name))
		}
		seenInputs[in.Name] = true
		if in.Type != "" && !validInputTypes[in.Type] {
			r.Add("error", field+".type",
				fmt.Sprintf("unknown type %q (allowed: string, integer, float, boolean, array, object)", in.Type))
		}
		if in.Required != nil && !*in.Required && in.Default == nil {
			r.Add("warning", field+".default",
				"optional input has no `default`; templates referencing it without `default(...)` may render empty")
		}
	}

	// Body required: must have a User section (or any body content fallback).
	if strings.TrimSpace(p.Body.User) == "" {
		r.Add("error", "body", "missing `# User` section (or non-empty body)")
	}

	// {{var}} cross-reference.
	refs := p.VarRefs()
	for _, ref := range refs {
		if !seenInputs[ref] {
			r.Add("error", "body",
				fmt.Sprintf("body references {{%s}} but no input named %q is declared", ref, ref))
		}
	}
	used := map[string]bool{}
	for _, ref := range refs {
		used[ref] = true
	}
	for name := range seenInputs {
		if !used[name] {
			r.Add("warning", "frontmatter.inputs",
				fmt.Sprintf("declared input %q is not referenced in body", name))
		}
	}

	// Evals.
	for i, ev := range fm.Evals {
		field := fmt.Sprintf("frontmatter.evals[%d]", i)
		if ev.Metric == "" {
			r.Add("error", field+".metric", "missing")
		} else if !validMetrics[ev.Metric] {
			r.Add("error", field+".metric",
				fmt.Sprintf("unknown metric %q (allowed: exact_match, contains, regex, json_schema, llm_judge, custom)", ev.Metric))
		}
		if ev.Dataset == "" && ev.Metric != "json_schema" {
			r.Add("error", field+".dataset",
				fmt.Sprintf("metric %q requires a dataset", ev.Metric))
		}
		if ev.Threshold < 0 || ev.Threshold > 1 {
			r.Add("error", field+".threshold",
				fmt.Sprintf("threshold %v out of range [0,1]", ev.Threshold))
		}
	}

	// Outputs.schema: shallow check only — full JSON Schema validation runs at eval time.
	if fm.Outputs != nil && fm.Outputs.Schema != nil {
		if t, ok := fm.Outputs.Schema["type"]; ok {
			if _, isStr := t.(string); !isStr {
				r.Add("error", "frontmatter.outputs.schema.type", "expected string")
			}
		}
	}

	// Forward-compat warnings for unknown frontmatter keys.
	for k := range fm.Extra {
		r.Add("warning", "frontmatter."+k,
			"unknown frontmatter key (parser preserved it but format spec doesn't define it)")
	}

	return r
}

// isValidID checks the id segment grammar.
func isValidID(id string) bool {
	if id == "" {
		return false
	}
	if id[0] == '/' || id[len(id)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(id, "/") {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z',
				r >= '0' && r <= '9',
				r == '-', r == '_':
			default:
				return false
			}
		}
	}
	return true
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
