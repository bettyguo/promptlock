// Package metrics implements eval metrics: exact_match, contains, regex,
// json_schema, llm_judge, custom.
package metrics

import (
	"context"
	"fmt"

	"github.com/promptlock/promptlock/internal/eval/providers"
)

// Result is one metric's output for one row.
type Result struct {
	Score   float64        // [0.0, 1.0]
	Detail  map[string]any // optional metric-specific detail (parse errors, etc.)
	Error   string         // non-empty when the metric itself errored (not a score-of-zero)
}

// Metric is one scoring strategy. Implementations are stateless and reused
// across rows; per-row state lives in Result.Detail.
type Metric interface {
	Name() string
	// Score scores `output` against `expected`. cfg is the prompt's
	// metric_config block (may be nil). row is the source row, in case the
	// metric needs to reach into Inputs (e.g. exact_match field selection).
	Score(ctx context.Context, output string, expected any, row map[string]any, cfg map[string]any) Result
}

// JudgeFunc lets llm_judge invoke an LLM. Injected to avoid a hard dep
// from metrics → eval → metrics cycle.
type JudgeFunc func(ctx context.Context, req providers.Request) (providers.Response, error)

// Get returns a Metric by name. judge is required for "llm_judge"; passing nil
// substitutes a stub that errors on use, which is the desired behavior in
// places (like `validate`) where evals shouldn't run for real.
func Get(name string, judge JudgeFunc) (Metric, error) {
	switch name {
	case "exact_match":
		return &ExactMatch{}, nil
	case "contains":
		return &Contains{}, nil
	case "regex":
		return &Regex{}, nil
	case "json_schema":
		return &JSONSchema{}, nil
	case "llm_judge":
		return &LLMJudge{Judge: judge}, nil
	case "custom":
		return &Custom{}, nil
	}
	return nil, fmt.Errorf("unknown metric %q", name)
}
