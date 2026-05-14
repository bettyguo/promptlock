package ci

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	er := &EvalResults{
		SchemaVersion:     1,
		PromptlockVersion: "0.1.0-test",
		Results: []PromptResults{{
			PromptID: "support/triage",
			Runs: []RunInfo{
				{Metric: "exact_match", Dataset: "tests/d.jsonl", Score: 0.88, Threshold: 0.85, Passed: true, Rows: 50, Version: "1.4.0"},
				{Metric: "json_schema", Dataset: "tests/d.jsonl", Score: 0.99, Threshold: 0.95, Passed: true, Rows: 50, Version: "1.4.0"},
			},
		}},
	}
	out := RenderMarkdown(er)
	for _, want := range []string{
		CommentMarker,
		"## promptlock eval",
		"`support/triage`",
		"`exact_match`",
		"0.880",
		"✅ pass",
		"1 / 1 prompts pass",
		"promptlock 0.1.0-test",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderMarkdown_Failure(t *testing.T) {
	er := &EvalResults{
		PromptlockVersion: "0.1",
		Results: []PromptResults{{
			PromptID: "x",
			Runs: []RunInfo{
				{Metric: "exact_match", Dataset: "d.jsonl", Score: 0.40, Threshold: 0.85, Passed: false, Rows: 10},
			},
		}},
	}
	out := RenderMarkdown(er)
	if !strings.Contains(out, "❌ fail") {
		t.Errorf("missing fail marker: %s", out)
	}
	if !strings.Contains(out, "CI failed") {
		t.Errorf("missing CI-failed message: %s", out)
	}
}

func TestRenderMarkdown_Error(t *testing.T) {
	er := &EvalResults{Results: []PromptResults{{PromptID: "x", Err: "missing API key"}}}
	out := RenderMarkdown(er)
	if !strings.Contains(out, "missing API key") {
		t.Errorf("missing error in output: %s", out)
	}
}

func TestResolvePRNumber(t *testing.T) {
	cases := []struct {
		ref      string
		override string
		want     string
		wantErr  bool
	}{
		{"refs/pull/123/merge", "", "123", false},
		{"refs/pull/4567/head", "", "4567", false},
		{"", "99", "99", false},
		{"refs/pull/123/merge", "5", "5", false}, // override wins
		{"refs/heads/main", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		got, err := ResolvePRNumber(c.ref, c.override)
		if c.wantErr {
			if err == nil {
				t.Errorf("ref=%q override=%q expected error", c.ref, c.override)
			}
			continue
		}
		if err != nil {
			t.Errorf("ref=%q override=%q unexpected error: %v", c.ref, c.override, err)
		}
		if got != c.want {
			t.Errorf("ref=%q override=%q got %q want %q", c.ref, c.override, got, c.want)
		}
	}
}

func TestNextLinkParse(t *testing.T) {
	header := `<https://api.github.com/page2>; rel="next", <https://api.github.com/last>; rel="last"`
	if got := nextLink(header); got != "https://api.github.com/page2" {
		t.Errorf("got %q", got)
	}
	if got := nextLink(""); got != "" {
		t.Errorf("empty should give empty, got %q", got)
	}
	if got := nextLink(`<https://api.github.com/last>; rel="last"`); got != "" {
		t.Errorf("no next link should give empty, got %q", got)
	}
}
