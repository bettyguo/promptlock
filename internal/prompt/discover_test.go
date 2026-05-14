package prompt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/promptlock/promptlock/internal/prompt"
)

// setupRepo writes a small fake repo with a `prompts/` tree to a temp dir.
func setupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"prompts/foo.prompt.md":            stubPrompt("foo"),
		"prompts/bar/baz.prompt.md":        stubPrompt("bar/baz"),
		"prompts/.hidden/x.prompt.md":      stubPrompt("hidden"),  // should be skipped
		"prompts/notaprompt.md":            "ignore me",
		"prompts/sub/sub2/qux.prompt.md":   stubPrompt("sub/sub2/qux"),
		"some-other-dir/ignored.prompt.md": stubPrompt("ignored"),
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func stubPrompt(id string) string {
	return "---\nid: \"" + id + "\"\nversion: \"0.1.0\"\nmodel: \"m\"\n---\n# User\nhi\n"
}

func TestDiscover_FindsPromptsRecursivelyAndSkipsNonPrompts(t *testing.T) {
	root := setupRepo(t)
	got, err := prompt.Discover(prompt.DiscoverOptions{Root: root})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantIDs := []string{"bar/baz", "foo", "sub/sub2/qux"} // sorted
	if len(got) != len(wantIDs) {
		t.Fatalf("found %d prompts, want %d: %+v", len(got), len(wantIDs), got)
	}
	for i, w := range wantIDs {
		if got[i].IDFromPath != w {
			t.Errorf("got[%d].IDFromPath = %q, want %q", i, got[i].IDFromPath, w)
		}
	}
}

func TestDiscover_MissingPromptsDir(t *testing.T) {
	root := t.TempDir()
	got, err := prompt.Discover(prompt.DiscoverOptions{Root: root})
	if err != nil {
		t.Fatalf("Discover should be tolerant of missing dir: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestDiscover_IDFilter(t *testing.T) {
	root := setupRepo(t)
	got, err := prompt.Discover(prompt.DiscoverOptions{
		Root:     root,
		IDFilter: func(id string) bool { return id == "foo" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].IDFromPath != "foo" {
		t.Errorf("unexpected: %+v", got)
	}
}
