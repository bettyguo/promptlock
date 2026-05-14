package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExactMatch checks output == expected after normalization.
//
// metric_config:
//   trim_whitespace: bool (default true)
//   case_sensitive:  bool (default false)
//   field:           string — extract this top-level field from JSON output
//                    (or expected) before comparison
type ExactMatch struct{}

// Name returns "exact_match".
func (*ExactMatch) Name() string { return "exact_match" }

// Score implements Metric.
func (*ExactMatch) Score(_ context.Context, output string, expected any, _, cfg map[string]any) Result {
	trim := cfgBool(cfg, "trim_whitespace", true)
	caseSensitive := cfgBool(cfg, "case_sensitive", false)
	field := cfgString(cfg, "field", "")

	gotStr := output
	wantStr := coerceCmp(expected)

	if field != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			return Result{Score: 0, Detail: map[string]any{"reason": "output is not valid JSON"}}
		}
		val, ok := parsed[field]
		if !ok {
			return Result{Score: 0, Detail: map[string]any{"reason": fmt.Sprintf("field %q absent from output", field)}}
		}
		gotStr = coerceCmp(val)

		// expected may be a scalar or a map carrying our field.
		if expMap, ok := expected.(map[string]any); ok {
			if v, ok := expMap[field]; ok {
				wantStr = coerceCmp(v)
			}
		}
	}

	if trim {
		gotStr = strings.TrimSpace(gotStr)
		wantStr = strings.TrimSpace(wantStr)
	}
	if !caseSensitive {
		gotStr = strings.ToLower(gotStr)
		wantStr = strings.ToLower(wantStr)
	}
	if gotStr == wantStr {
		return Result{Score: 1.0}
	}
	return Result{
		Score: 0.0,
		Detail: map[string]any{
			"got":  gotStr,
			"want": wantStr,
		},
	}
}

// coerceCmp turns any → its string comparison form.
func coerceCmp(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case json.Number:
		return x.String()
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func cfgBool(cfg map[string]any, key string, def bool) bool {
	if cfg == nil {
		return def
	}
	if v, ok := cfg[key].(bool); ok {
		return v
	}
	return def
}

func cfgString(cfg map[string]any, key, def string) string {
	if cfg == nil {
		return def
	}
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return def
}
