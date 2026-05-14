package prompt_test

import (
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/prompt"
)

func TestValidate_GoodPrompt(t *testing.T) {
	data := mustReadFixture(t, "prompts/support/customer-triage.prompt.md")
	p, err := prompt.Parse("prompts/support/customer-triage.prompt.md", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "prompts")
	if r.HasError {
		t.Errorf("expected no errors; got: %v", r.Issues)
	}
}

func TestValidate_BadIDAndMissingFields(t *testing.T) {
	data := mustReadFixture(t, "invalid/bad-id.prompt.md")
	p, err := prompt.Parse("invalid/bad-id.prompt.md", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "prompts")
	if !r.HasError {
		t.Errorf("expected errors; got none")
	}
	wantSubstrings := []string{
		"frontmatter.id",      // bad id format
		"frontmatter.version", // not semver
		"frontmatter.inputs[0].name",
		"unknown type",
		"unknown metric",
		"out of range",
		"undeclared_var",
	}
	concat := concatIssues(r.Issues)
	for _, w := range wantSubstrings {
		if !strings.Contains(concat, w) {
			t.Errorf("missing expected error substring %q in:\n%s", w, concat)
		}
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	data := mustReadFixture(t, "invalid/missing-version.prompt.md")
	p, err := prompt.Parse("invalid/missing-version.prompt.md", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "prompts")
	if !r.HasError {
		t.Errorf("expected errors; got none")
	}
	if !strings.Contains(concatIssues(r.Issues), "missing required field") {
		t.Errorf("missing 'missing required field' error: %v", r.Issues)
	}
}

func TestValidate_IDPathMismatch(t *testing.T) {
	doc := []byte("---\nid: \"foo/bar\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n# User\nhi")
	p, err := prompt.Parse("prompts/support/customer-triage.prompt.md", doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "prompts")
	if !r.HasError {
		t.Error("expected id-path-mismatch error")
	}
	if !strings.Contains(concatIssues(r.Issues), "does not match file path") {
		t.Errorf("missing path-mismatch error in: %v", r.Issues)
	}
}

func TestValidate_UnusedInputWarning(t *testing.T) {
	doc := []byte(`---
id: "test/x"
version: "0.1.0"
model: "m"
inputs:
  - name: used
  - name: unused
---

# User
{{used}}
`)
	p, err := prompt.Parse("test/x.prompt.md", doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "")
	if r.HasError {
		t.Errorf("should not have errors, only warnings: %v", r.Issues)
	}
	found := false
	for _, iss := range r.Issues {
		if iss.Severity == "warning" && strings.Contains(iss.Message, "unused") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing unused-input warning: %v", r.Issues)
	}
}

func TestValidate_UnknownFrontmatterKeyWarning(t *testing.T) {
	doc := []byte("---\nid: \"x\"\nversion: \"0.1.0\"\nmodel: \"m\"\nfuture: 1\n---\n# User\nhi")
	p, err := prompt.Parse("test", doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := prompt.Validate(p, "")
	if r.HasError {
		t.Errorf("should not error on unknown keys: %v", r.Issues)
	}
	if !strings.Contains(concatIssues(r.Issues), "future") {
		t.Errorf("expected warning about 'future' key: %v", r.Issues)
	}
}

func concatIssues(iss []prompt.Issue) string {
	var b strings.Builder
	for _, i := range iss {
		b.WriteString(i.Error())
		b.WriteString("\n")
	}
	return b.String()
}
