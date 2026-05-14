package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/cli"
)

// setupRepo creates a temp working dir with the test fixtures copied into it.
func setupRepo(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "tests", "fixtures")
	root := t.TempDir()
	walk := func(srcSub, dstSub string) {
		err := filepath.Walk(filepath.Join(src, srcSub), func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(filepath.Join(src, srcSub), p)
			dst := filepath.Join(root, dstSub, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	walk("prompts", "prompts")
	return root
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = cli.Run("test", args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestCLI_ListHuman(t *testing.T) {
	root := setupRepo(t)
	stdout, _, code := runCLI(t, "list", "--cwd", root)
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "support/customer-triage") {
		t.Errorf("missing prompt in list output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "onboarding/welcome") {
		t.Errorf("missing prompt in list output:\n%s", stdout)
	}
}

func TestCLI_ListJSON(t *testing.T) {
	root := setupRepo(t)
	stdout, _, code := runCLI(t, "list", "--cwd", root, "--format", "json")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(rows) < 2 {
		t.Errorf("got %d rows, want ≥2", len(rows))
	}
}

func TestCLI_ListTagFilter(t *testing.T) {
	root := setupRepo(t)
	stdout, _, code := runCLI(t, "list", "--cwd", root, "--tag", "onboarding")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "onboarding/welcome") {
		t.Errorf("missing onboarding prompt: %s", stdout)
	}
	if strings.Contains(stdout, "support/customer-triage") {
		t.Errorf("filter should have excluded support: %s", stdout)
	}
}

func TestCLI_ShowHuman(t *testing.T) {
	root := setupRepo(t)
	stdout, _, code := runCLI(t, "show", "--cwd", root, "support/customer-triage")
	if code != cli.ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(strings.ToLower(stdout), "support") || !strings.Contains(strings.ToLower(stdout), "classif") {
		t.Errorf("show output missing body: %s", stdout)
	}
}

func TestCLI_ShowMissing(t *testing.T) {
	root := setupRepo(t)
	_, stderr, code := runCLI(t, "show", "--cwd", root, "no/such/prompt")
	if code == cli.ExitOK {
		t.Errorf("expected non-zero exit")
	}
	if !strings.Contains(stderr, "no prompt with id") {
		t.Errorf("expected helpful error, got: %s", stderr)
	}
}

func TestCLI_ValidateAllGood(t *testing.T) {
	root := setupRepo(t)
	stdout, _, code := runCLI(t, "validate", "--cwd", root)
	if code != cli.ExitOK {
		t.Fatalf("exit %d, stdout:\n%s", code, stdout)
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	_, stderr, code := runCLI(t, "frobnicate")
	if code != cli.ExitUsage {
		t.Errorf("exit %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr missing message: %s", stderr)
	}
}

func TestCLI_HelpFlag(t *testing.T) {
	stdout, _, code := runCLI(t, "--help")
	if code != cli.ExitOK {
		t.Errorf("exit %d", code)
	}
	if !strings.Contains(stdout, "promptlock") {
		t.Errorf("help missing program name: %s", stdout)
	}
}
