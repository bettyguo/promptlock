package metrics

import (
	"context"
	"regexp"
)

// Regex checks output matches a regex from metric_config.pattern.
type Regex struct {
	cache map[string]*regexp.Regexp
}

// Name returns "regex".
func (*Regex) Name() string { return "regex" }

// Score implements Metric.
func (r *Regex) Score(_ context.Context, output string, _ any, _, cfg map[string]any) Result {
	pat := cfgString(cfg, "pattern", "")
	if pat == "" {
		return Result{Error: "metric_config.pattern is required for `regex`"}
	}
	if r.cache == nil {
		r.cache = map[string]*regexp.Regexp{}
	}
	re, ok := r.cache[pat]
	if !ok {
		var err error
		re, err = regexp.Compile(pat)
		if err != nil {
			return Result{Error: "invalid regex: " + err.Error()}
		}
		r.cache[pat] = re
	}
	if re.MatchString(output) {
		return Result{Score: 1.0}
	}
	return Result{Score: 0.0, Detail: map[string]any{"pattern": pat}}
}
