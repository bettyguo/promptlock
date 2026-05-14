package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/promptlock/promptlock/internal/eval"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDataset_JSONL(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "data.jsonl", `{"x": 1, "expected": "a"}
{"x": 2, "expected": "b"}

{"x": 3, "expected": "c"}
`)
	rows, err := eval.LoadDataset(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Inputs["x"] != float64(1) {
		t.Errorf("row 0 x = %v", rows[0].Inputs["x"])
	}
	if rows[0].Expected != "a" {
		t.Errorf("row 0 expected = %v", rows[0].Expected)
	}
}

func TestLoadDataset_JSONL_BadJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "bad.jsonl", `{"x":1}
{not json}
`)
	_, err := eval.LoadDataset(p)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadDataset_CSV(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "data.csv", "x,expected\n1,a\n2,b\n")
	rows, err := eval.LoadDataset(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Inputs["x"] != "1" {
		t.Errorf("row 0 x = %v (want \"1\")", rows[0].Inputs["x"])
	}
	if rows[0].Expected != "a" {
		t.Errorf("row 0 expected = %v", rows[0].Expected)
	}
}

func TestLoadDataset_NotFound(t *testing.T) {
	_, err := eval.LoadDataset("/no/such/file.jsonl")
	if err == nil {
		t.Fatal("expected error")
	}
}
