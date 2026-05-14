package metrics

import (
	"context"
	"strings"
)

// Contains checks that `expected` (a string) appears as a substring in output.
type Contains struct{}

// Name returns "contains".
func (*Contains) Name() string { return "contains" }

// Score implements Metric.
func (*Contains) Score(_ context.Context, output string, expected any, _, cfg map[string]any) Result {
	wantStr := coerceCmp(expected)
	gotStr := output
	if cfgBool(cfg, "case_sensitive", false) {
		// keep as-is
	} else {
		wantStr = strings.ToLower(wantStr)
		gotStr = strings.ToLower(gotStr)
	}
	if wantStr == "" {
		return Result{Score: 0, Detail: map[string]any{"reason": "no `expected` value to look for"}}
	}
	if strings.Contains(gotStr, wantStr) {
		return Result{Score: 1.0}
	}
	return Result{Score: 0.0, Detail: map[string]any{"want_substring": wantStr}}
}
