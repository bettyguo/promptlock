package lock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/promptlock/promptlock/internal/lock"
)

func TestLockfile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "promptlock.lock")

	lf := lock.New("0.1.0")
	temp := 0.3
	lf.Upsert(lock.Entry{
		ID:          "support/triage",
		Version:     "1.0.0",
		File:        "prompts/support/triage.prompt.md",
		ContentHash: "sha256:abc123",
		LastEval: &lock.EvalInfo{
			Provider:          "anthropic",
			Model:             "claude-opus-4-7",
			Temperature:       &temp,
			Timestamp:         time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
			PromptlockVersion: "0.1.0",
			Datasets: []lock.DatasetInfo{
				{Path: "tests/datasets/triage.jsonl", Hash: "sha256:def", Rows: 50},
			},
			Scores: []lock.ScoreInfo{
				{Dataset: "tests/datasets/triage.jsonl", Metric: "exact_match", Score: 0.88, Threshold: 0.85, Aggregate: "mean"},
			},
			Tokens: &lock.TokensInfo{Input: 4400, Output: 612},
		},
	})
	if err := lf.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := lock.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := len(loaded.Prompts); got != 1 {
		t.Fatalf("got %d prompts, want 1", got)
	}
	e := loaded.Prompts[0]
	if e.ID != "support/triage" || e.ContentHash != "sha256:abc123" {
		t.Errorf("entry mismatch: %+v", e)
	}
	if e.LastEval == nil || len(e.LastEval.Scores) != 1 || e.LastEval.Scores[0].Score != 0.88 {
		t.Errorf("last_eval lost: %+v", e.LastEval)
	}
}

func TestLockfile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	lf, err := lock.Load(filepath.Join(dir, "nope.lock"))
	if err != nil {
		t.Fatalf("missing lockfile should not error, got: %v", err)
	}
	if len(lf.Prompts) != 0 {
		t.Errorf("expected empty, got %+v", lf.Prompts)
	}
}

func TestLockfile_NewerSchemaRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "promptlock.lock")
	if err := os.WriteFile(path, []byte("schema_version: 999\nprompts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Load(path); err == nil {
		t.Error("expected error loading future schema_version")
	}
}

func TestLockfile_UpsertAndRemove(t *testing.T) {
	lf := lock.New("0.1.0")
	lf.Upsert(lock.Entry{ID: "a", ContentHash: "sha256:1"})
	lf.Upsert(lock.Entry{ID: "b", ContentHash: "sha256:2"})
	lf.Upsert(lock.Entry{ID: "a", ContentHash: "sha256:1-updated"})

	if got := len(lf.Prompts); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	a, ok := lf.Find("a")
	if !ok || a.ContentHash != "sha256:1-updated" {
		t.Errorf("upsert didn't update: %+v", a)
	}
	if !lf.Remove("a") {
		t.Error("remove should return true")
	}
	if _, ok := lf.Find("a"); ok {
		t.Error("a should be gone")
	}
	if lf.Remove("nope") {
		t.Error("remove of absent should return false")
	}
}
