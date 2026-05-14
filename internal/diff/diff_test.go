package diff_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/diff"
	"github.com/promptlock/promptlock/internal/prompt"
)

func parse(t *testing.T, doc string) *prompt.Prompt {
	t.Helper()
	p, err := prompt.Parse("test", []byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return p
}

func TestDiff_NoChanges(t *testing.T) {
	doc := "---\nid: \"x\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n# User\nhello"
	p := parse(t, doc)
	r := diff.Diff(p, p)
	if r.HasChanges {
		t.Errorf("identical prompts should produce no changes, got: %+v", r)
	}
}

func TestDiff_VersionAndModelChange(t *testing.T) {
	a := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"claude-opus-4-6\"\n---\n# User\nhello")
	b := parse(t, "---\nid: \"x\"\nversion: \"1.1.0\"\nmodel: \"claude-opus-4-7\"\n---\n# User\nhello")
	r := diff.Diff(a, b)
	if !r.HasChanges {
		t.Fatal("expected changes")
	}
	if r.VersionChange == nil || r.VersionChange.Before != "1.0.0" || r.VersionChange.After != "1.1.0" {
		t.Errorf("version change wrong: %+v", r.VersionChange)
	}
	found := false
	for _, c := range r.FrontmatterChanges {
		if c.Path == "model" && c.Before == "claude-opus-4-6" && c.After == "claude-opus-4-7" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing model change in: %+v", r.FrontmatterChanges)
	}
}

func TestDiff_BodyWordLevel(t *testing.T) {
	a := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"m\"\ninputs:\n  - name: x\n---\n# User\nRead the ticket and classify {{x}}.")
	b := parse(t, "---\nid: \"x\"\nversion: \"1.0.1\"\nmodel: \"m\"\ninputs:\n  - name: x\n---\n# User\nRead the ticket carefully and classify {{x}}.")
	r := diff.Diff(a, b)
	if len(r.BodyChanges) == 0 {
		t.Fatal("expected body changes")
	}
	user := findSection(r.BodyChanges, "user")
	if user == nil || !user.Changed {
		t.Fatal("user section should be changed")
	}
	// Should have an insert op containing "carefully"
	found := false
	for _, op := range user.Operations {
		if op.Op == "insert" && strings.Contains(op.Text, "carefully") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing insert with 'carefully': %+v", user.Operations)
	}
}

func TestDiff_TemplateVarAtomic(t *testing.T) {
	a := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"m\"\ninputs:\n  - name: foo\n---\n# User\nUse {{foo}} now.")
	b := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"m\"\ninputs:\n  - name: bar\n---\n# User\nUse {{bar}} now.")
	r := diff.Diff(a, b)
	user := findSection(r.BodyChanges, "user")
	if user == nil {
		t.Fatal("missing user section")
	}
	// Should be a delete of "{{foo}}" and an insert of "{{bar}}", not character-level.
	deleteFound, insertFound := false, false
	for _, op := range user.Operations {
		if op.Op == "delete" && op.Text == "{{foo}}" {
			deleteFound = true
		}
		if op.Op == "insert" && op.Text == "{{bar}}" {
			insertFound = true
		}
	}
	if !deleteFound || !insertFound {
		t.Errorf("template vars not atomic: %+v", user.Operations)
	}
}

func TestDiff_InputsAddedAndRemoved(t *testing.T) {
	a := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"m\"\ninputs:\n  - name: kept\n  - name: removed_one\n---\n# User\n{{kept}} {{removed_one}}")
	b := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"m\"\ninputs:\n  - name: kept\n  - name: added_one\n---\n# User\n{{kept}} {{added_one}}")
	r := diff.Diff(a, b)
	gotKinds := map[string]string{}
	for _, ic := range r.InputsChanges {
		gotKinds[ic.Name] = ic.Kind
	}
	if gotKinds["removed_one"] != "removed" {
		t.Errorf("expected removed_one removed: %v", gotKinds)
	}
	if gotKinds["added_one"] != "added" {
		t.Errorf("expected added_one added: %v", gotKinds)
	}
	if _, kept := gotKinds["kept"]; kept {
		t.Errorf("kept input shouldn't appear in changes: %v", gotKinds)
	}
}

func TestDiff_BeforeMissing(t *testing.T) {
	b := parse(t, "---\nid: \"new\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n# User\nhi")
	r := diff.Diff(nil, b)
	if !r.BeforeMissing || !r.HasChanges {
		t.Errorf("expected new-file diff")
	}
}

func TestRender_HumanOutput(t *testing.T) {
	a := parse(t, "---\nid: \"x\"\nversion: \"1.0.0\"\nmodel: \"old-m\"\n---\n# User\nfoo bar baz")
	b := parse(t, "---\nid: \"x\"\nversion: \"1.0.1\"\nmodel: \"new-m\"\n---\n# User\nfoo qux baz")
	r := diff.Diff(a, b)
	var buf bytes.Buffer
	diff.Render(&buf, r, false)
	out := buf.String()
	for _, want := range []string{"x", "1.0.0 → 1.0.1", "old-m", "new-m", "[-bar-]", "[+qux+]"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q in:\n%s", want, out)
		}
	}
}

func findSection(secs []diff.SectionChange, name string) *diff.SectionChange {
	for i := range secs {
		if secs[i].Section == name {
			return &secs[i]
		}
	}
	return nil
}
