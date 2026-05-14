package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/promptlock/promptlock/internal/eval/metrics"
	"github.com/promptlock/promptlock/internal/eval/providers"
	"github.com/promptlock/promptlock/internal/prompt"
	"github.com/promptlock/promptlock/internal/render"
)

// Run is one invocation of `promptlock eval` over one prompt + dataset + metric.
type Run struct {
	PromptID string
	Version  string
	Dataset  string
	Metric   string
	Provider string
	Model    string

	Rows      []Row
	RowResults []RowResult

	Aggregate float64 // mean of row scores
	Threshold float64
	Passed    bool

	InputTokens  int
	OutputTokens int

	StartedAt  time.Time
	FinishedAt time.Time
}

// RowResult captures per-row data.
type RowResult struct {
	Index        int            `json:"index"`
	Output       string         `json:"output"`
	Score        float64        `json:"score"`
	Error        string         `json:"error,omitempty"`
	Detail       map[string]any `json:"detail,omitempty"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	CacheHit     bool           `json:"cache_hit"`
}

// Options control a single Run.
type Options struct {
	Provider           providers.Provider
	Cache              *Cache
	Parallel           int
	AllowCustomMetrics bool
	JudgeProvider      providers.Provider // for llm_judge; usually = Provider
	OnRowDone          func(rr RowResult, total int) // optional progress callback
	OnRunStart         func(promptID, datasetPath, metric string, total int)
}

// RunPrompt runs every eval declared on `p`. If `evalFilter` is non-nil it must
// return true for each Eval index to include.
func RunPrompt(ctx context.Context, p *prompt.Prompt, datasets map[string][]Row, opts Options) ([]Run, error) {
	if opts.Provider == nil {
		return nil, fmt.Errorf("eval: no provider")
	}
	if opts.Parallel < 1 {
		opts.Parallel = 8
	}

	var runs []Run
	for _, ev := range p.Frontmatter.Evals {
		rows, ok := datasets[ev.Dataset]
		if !ok && ev.Metric != "json_schema" {
			return nil, fmt.Errorf("eval: dataset %q not loaded", ev.Dataset)
		}
		// json_schema can run with no dataset against the prompt's outputs.schema
		// — we synthesize a single row from the prompt's declared inputs (if any).
		if !ok {
			rows = []Row{{Inputs: defaultInputs(p), Index: 1}}
		}
		run, err := runOneEval(ctx, p, ev, rows, opts)
		if err != nil {
			return runs, err
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

func defaultInputs(p *prompt.Prompt) map[string]any {
	in := map[string]any{}
	for _, x := range p.Frontmatter.Inputs {
		if x.Default != nil {
			in[x.Name] = x.Default
		} else {
			in[x.Name] = ""
		}
	}
	return in
}

func runOneEval(ctx context.Context, p *prompt.Prompt, ev prompt.Eval, rows []Row, opts Options) (*Run, error) {
	provider := opts.Provider
	model := p.Frontmatter.Model
	if ev.Model != "" {
		model = ev.Model
	}
	if ev.Provider != "" {
		var err error
		provider, err = providers.Get(ev.Provider)
		if err != nil {
			return nil, fmt.Errorf("eval: %w", err)
		}
	}
	temperature := p.Frontmatter.Temperature
	if ev.Temperature != nil {
		temperature = ev.Temperature
	}

	// Build the metric. llm_judge gets a JudgeFunc bound to the configured judge.
	judge := opts.JudgeProvider
	if judge == nil {
		judge = provider
	}
	judgeFn := metrics.JudgeFunc(func(ctx context.Context, req providers.Request) (providers.Response, error) {
		return judge.Call(ctx, req)
	})
	metric, err := metrics.Get(ev.Metric, judgeFn)
	if err != nil {
		return nil, err
	}
	if c, ok := metric.(*metrics.Custom); ok {
		c.Allow = opts.AllowCustomMetrics
	}

	// Compose the metric's effective config (frontmatter cfg + outputs.schema for json_schema).
	cfg := map[string]any{}
	for k, v := range ev.MetricConfig {
		cfg[k] = v
	}
	if ev.Metric == "json_schema" && p.Frontmatter.Outputs != nil && p.Frontmatter.Outputs.Schema != nil {
		cfg["_outputs_schema"] = p.Frontmatter.Outputs.Schema
	}

	run := &Run{
		PromptID:  p.Frontmatter.ID,
		Version:   p.Frontmatter.Version,
		Dataset:   ev.Dataset,
		Metric:    ev.Metric,
		Provider:  provider.Name(),
		Model:     model,
		Rows:      rows,
		Threshold: ev.Threshold,
		StartedAt: time.Now(),
	}
	if opts.OnRunStart != nil {
		opts.OnRunStart(run.PromptID, ev.Dataset, ev.Metric, len(rows))
	}

	// Bounded parallelism via a buffered channel.
	sem := make(chan struct{}, opts.Parallel)
	results := make([]RowResult, len(rows))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, row := range rows {
		i, row := i, row
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			rr := evalOneRow(ctx, p, row, provider, model, temperature, metric, cfg, opts.Cache)
			results[i] = rr
			mu.Lock()
			run.InputTokens += rr.InputTokens
			run.OutputTokens += rr.OutputTokens
			if rr.Error != "" && firstErr == nil {
				firstErr = fmt.Errorf("row %d: %s", row.Index, rr.Error)
			}
			mu.Unlock()
			if opts.OnRowDone != nil {
				opts.OnRowDone(rr, len(rows))
			}
		}()
	}
	wg.Wait()
	run.RowResults = results
	run.FinishedAt = time.Now()

	if firstErr != nil {
		return run, firstErr
	}
	// Aggregate: mean of row scores.
	if len(results) > 0 {
		var sum float64
		for _, r := range results {
			sum += r.Score
		}
		run.Aggregate = sum / float64(len(results))
	}
	run.Passed = run.Aggregate >= run.Threshold
	return run, nil
}

func evalOneRow(
	ctx context.Context,
	p *prompt.Prompt,
	row Row,
	provider providers.Provider,
	model string,
	temperature *float64,
	metric metrics.Metric,
	cfg map[string]any,
	cache *Cache,
) RowResult {
	rr := RowResult{Index: row.Index}

	system, err := render.Render(p.Body.System, row.Inputs)
	if err != nil {
		rr.Error = "render system: " + err.Error()
		return rr
	}
	user, err := render.Render(p.Body.User, row.Inputs)
	if err != nil {
		rr.Error = "render user: " + err.Error()
		return rr
	}
	msgs := []providers.Message{{Role: "user", Content: user}}
	if p.Body.Assistant != "" {
		assist, err := render.Render(p.Body.Assistant, row.Inputs)
		if err != nil {
			rr.Error = "render assistant: " + err.Error()
			return rr
		}
		msgs = append(msgs, providers.Message{Role: "assistant", Content: assist})
	}
	req := providers.Request{
		Model:       model,
		System:      system,
		Messages:    msgs,
		Temperature: temperature,
		MaxTokens:   p.Frontmatter.MaxTokens,
		TopP:        p.Frontmatter.TopP,
		Stop:        p.Frontmatter.Stop,
	}
	var resp *providers.Response
	if cache != nil {
		key := cache.Key(provider.Name(), req)
		hit, err := cache.Get(key)
		if err == nil && hit != nil {
			resp = hit
			rr.CacheHit = true
		} else if err == nil {
			r, callErr := provider.Call(ctx, req)
			if callErr != nil {
				rr.Error = callErr.Error()
				return rr
			}
			resp = &r
			_ = cache.Put(key, r)
		} else {
			rr.Error = "cache: " + err.Error()
			return rr
		}
	} else {
		r, callErr := provider.Call(ctx, req)
		if callErr != nil {
			rr.Error = callErr.Error()
			return rr
		}
		resp = &r
	}
	rr.Output = resp.Output
	rr.InputTokens = resp.InputTokens
	rr.OutputTokens = resp.OutputTokens

	mr := metric.Score(ctx, resp.Output, row.Expected, row.Inputs, cfg)
	rr.Score = mr.Score
	rr.Detail = mr.Detail
	if mr.Error != "" {
		rr.Error = "metric: " + mr.Error
	}
	return rr
}

// MarshalJSON gives Run a clean serialization for `--format json` output.
func (r Run) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PromptID     string      `json:"prompt_id"`
		Version      string      `json:"version"`
		Dataset      string      `json:"dataset"`
		Metric       string      `json:"metric"`
		Provider     string      `json:"provider"`
		Model        string      `json:"model"`
		Rows         int         `json:"rows"`
		Aggregate    float64     `json:"score"`
		Threshold    float64     `json:"threshold"`
		Passed       bool        `json:"passed"`
		InputTokens  int         `json:"input_tokens"`
		OutputTokens int         `json:"output_tokens"`
		StartedAt    time.Time   `json:"started_at"`
		FinishedAt   time.Time   `json:"finished_at"`
		RowResults   []RowResult `json:"rows_detail,omitempty"`
	}{
		PromptID:     r.PromptID,
		Version:      r.Version,
		Dataset:      r.Dataset,
		Metric:       r.Metric,
		Provider:     r.Provider,
		Model:        r.Model,
		Rows:         len(r.Rows),
		Aggregate:    r.Aggregate,
		Threshold:    r.Threshold,
		Passed:       r.Passed,
		InputTokens:  r.InputTokens,
		OutputTokens: r.OutputTokens,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		RowResults:   r.RowResults,
	})
}
