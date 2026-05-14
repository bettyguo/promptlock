package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/promptlock/promptlock/internal/eval/providers"
)

// LLMJudge invokes a judge LLM with a rubric and parses a numeric score.
//
// metric_config:
//   judge_provider: name (default: same as eval provider)
//   judge_model:    id   (default: same as eval model)
//   rubric:         string  — required
//   scale:          [min, max] floats (default [0,1])
//   use_reference:  bool — include row.expected in the judge prompt
//
// Output parsing: extract the LAST integer or float in the judge response.
type LLMJudge struct {
	Judge JudgeFunc

	calibrationOnce sync.Once
}

// Name returns "llm_judge".
func (*LLMJudge) Name() string { return "llm_judge" }

// Score implements Metric.
func (l *LLMJudge) Score(ctx context.Context, output string, expected any, _, cfg map[string]any) Result {
	if l.Judge == nil {
		return Result{Error: "llm_judge: no judge function injected (provider not available)"}
	}
	rubric := cfgString(cfg, "rubric", "")
	if rubric == "" {
		return Result{Error: "llm_judge: metric_config.rubric is required"}
	}
	scaleMin, scaleMax := 0.0, 1.0
	if scaleAny, ok := cfg["scale"].([]any); ok && len(scaleAny) == 2 {
		scaleMin = anyFloat(scaleAny[0])
		scaleMax = anyFloat(scaleAny[1])
	}
	if scaleMax <= scaleMin {
		return Result{Error: "llm_judge: scale must be [min, max] with max > min"}
	}

	system := rubric + "\n\nReturn ONLY a single numeric score on the scale [" +
		fmtFloat(scaleMin) + ", " + fmtFloat(scaleMax) + "]. No prose."
	user := "Candidate output:\n" + output
	if cfgBool(cfg, "use_reference", false) && expected != nil {
		user = "Reference output:\n" + coerceCmp(expected) + "\n\n" + user
	}
	model := cfgString(cfg, "judge_model", "")
	if model == "" {
		return Result{Error: "llm_judge: metric_config.judge_model is required"}
	}
	temp := 0.0
	maxTok := 16
	resp, err := l.Judge(ctx, providers.Request{
		Model:       model,
		System:      system,
		Messages:    []providers.Message{{Role: "user", Content: user}},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})
	if err != nil {
		return Result{Error: "llm_judge: " + err.Error()}
	}
	raw := strings.TrimSpace(resp.Output)
	num, ok := lastNumber(raw)
	if !ok {
		return Result{Score: 0, Detail: map[string]any{"judge_output": raw, "reason": "no number found"}}
	}
	// Normalize to [0,1].
	norm := (num - scaleMin) / (scaleMax - scaleMin)
	if norm < 0 {
		norm = 0
	}
	if norm > 1 {
		norm = 1
	}
	return Result{
		Score:  norm,
		Detail: map[string]any{"raw_score": num, "judge_output": raw},
	}
}

// CalibrationWarning returns the once-per-run text we surface to the user.
// Surfaced exactly once per process via sync.Once; the runner pulls this and
// prints it before any llm_judge metric runs.
func (l *LLMJudge) CalibrationWarning() string {
	var msg string
	l.calibrationOnce.Do(func() {
		msg = "warning: llm_judge is uncalibrated. Treat scores as relative, not absolute.\n" +
			"  Consider running with two different seeds to estimate variance,\n" +
			"  and gate CI on regression deltas rather than absolute thresholds."
	})
	return msg
}

func anyFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// lastNumber returns the last integer or float in s (e.g. "Score: 4" → 4).
func lastNumber(s string) (float64, bool) {
	var (
		buf     strings.Builder
		lastNum string
	)
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := buf.String()
		if _, err := strconv.ParseFloat(t, 64); err == nil {
			lastNum = t
		}
		buf.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
			buf.WriteByte(c)
		} else {
			flush()
		}
	}
	flush()
	if lastNum == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(lastNum, 64)
	if err != nil {
		return 0, false
	}
	_ = fmt.Sprint
	return f, true
}
