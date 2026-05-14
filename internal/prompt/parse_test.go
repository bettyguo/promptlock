package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/prompt"
	"github.com/promptlock/promptlock/internal/version"
)

func mustReadFixture(t *testing.T, rel string) []byte {
	t.Helper()
	// Tests run from the package dir; fixtures live at the repo root.
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return data
}

func TestParse_FullFixture(t *testing.T) {
	data := mustReadFixture(t, "prompts/support/customer-triage.prompt.md")
	p, err := prompt.Parse("prompts/support/customer-triage.prompt.md", data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if p.Frontmatter.ID != "support/customer-triage" {
		t.Errorf("ID = %q, want %q", p.Frontmatter.ID, "support/customer-triage")
	}
	if p.Frontmatter.Version == "" {
		t.Errorf("Version is empty")
	}
	if _, err := version.Parse(p.Frontmatter.Version); err != nil {
		t.Errorf("Version %q is not valid semver: %v", p.Frontmatter.Version, err)
	}
	if p.Frontmatter.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q", p.Frontmatter.Model)
	}
	if p.Frontmatter.Temperature == nil || *p.Frontmatter.Temperature != 0.3 {
		t.Errorf("Temperature = %v, want 0.3", p.Frontmatter.Temperature)
	}
	if got := len(p.Frontmatter.Inputs); got != 2 {
		t.Errorf("inputs len = %d, want 2", got)
	}
	if got := len(p.Frontmatter.Evals); got != 2 {
		t.Errorf("evals len = %d, want 2", got)
	}
	if !strings.Contains(strings.ToLower(p.Body.System), "support") || !strings.Contains(strings.ToLower(p.Body.System), "classif") {
		t.Errorf("System body missing expected text:\n%s", p.Body.System)
	}
	if !strings.Contains(p.Body.User, "{{ticket_text}}") {
		t.Errorf("User body missing template var:\n%s", p.Body.User)
	}
	refs := p.VarRefs()
	wantRefs := []string{"customer_tier", "ticket_text"}
	if len(refs) != len(wantRefs) {
		t.Fatalf("refs = %v, want %v", refs, wantRefs)
	}
	got := map[string]bool{}
	for _, r := range refs {
		got[r] = true
	}
	for _, w := range wantRefs {
		if !got[w] {
			t.Errorf("missing ref %q (got %v)", w, refs)
		}
	}
}

func TestParse_MinimalFixture(t *testing.T) {
	data := mustReadFixture(t, "prompts/onboarding/welcome.prompt.md")
	p, err := prompt.Parse("prompts/onboarding/welcome.prompt.md", data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if p.Body.System != "" {
		t.Errorf("System should be empty for minimal fixture, got %q", p.Body.System)
	}
	if !strings.Contains(p.Body.User, "{{user_name}}") {
		t.Errorf("user body missing var, got %q", p.Body.User)
	}
}

func TestParse_BodyVariations(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		system string
		user   string
		assist string
	}{
		{
			name:   "system + user",
			body:   "# System\nbe nice\n\n# User\nhello",
			system: "be nice",
			user:   "hello",
		},
		{
			name:   "user only",
			body:   "# User\njust a user message",
			user:   "just a user message",
		},
		{
			name:   "case insensitive headings",
			body:   "# system\nfoo\n# USER\nbar\n# Assistant\nbaz",
			system: "foo",
			user:   "bar",
			assist: "baz",
		},
		{
			name: "no headings = whole body is user",
			body: "no headings here, just text",
			user: "no headings here, just text",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := "---\nid: \"test/x\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n\n" + c.body
			p, err := prompt.Parse("test", []byte(doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if p.Body.System != c.system {
				t.Errorf("System = %q, want %q", p.Body.System, c.system)
			}
			if p.Body.User != c.user {
				t.Errorf("User = %q, want %q", p.Body.User, c.user)
			}
			if p.Body.Assistant != c.assist {
				t.Errorf("Assistant = %q, want %q", p.Body.Assistant, c.assist)
			}
		})
	}
}

func TestParse_MalformedFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string // substring of error message
	}{
		{"no opening delim", "id: x\nversion: 0.1.0", "must begin with"},
		{"no closing delim", "---\nid: x\nversion: 0.1.0\n", "never closes"},
		{"invalid yaml", "---\nid: [unclosed\n---\n# User\nhi", "invalid YAML"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := prompt.Parse("test", []byte(c.doc))
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want substring %q", err, c.want)
			}
		})
	}
}

func TestParse_BOMHandled(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	doc := append(bom, []byte("---\nid: \"x\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n# User\nhi")...)
	p, err := prompt.Parse("test", doc)
	if err != nil {
		t.Fatalf("Parse with BOM: %v", err)
	}
	if p.Frontmatter.ID != "x" {
		t.Errorf("ID = %q", p.Frontmatter.ID)
	}
}

func TestParse_PreservesUnknownKeys(t *testing.T) {
	doc := "---\nid: \"x\"\nversion: \"0.1.0\"\nmodel: \"m\"\nfuture_field: true\n---\n# User\nhi"
	p, err := prompt.Parse("test", []byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Frontmatter.Extra == nil || p.Frontmatter.Extra["future_field"] != true {
		t.Errorf("Extra = %v, want future_field=true", p.Frontmatter.Extra)
	}
}
