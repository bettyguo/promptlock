package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/promptlock/promptlock/internal/eval"
	"github.com/promptlock/promptlock/internal/eval/providers"
	"github.com/promptlock/promptlock/internal/prompt"
)

type runResult struct {
	PromptID string     `json:"prompt_id"`
	Runs     []eval.Run `json:"runs,omitempty"`
	Err      string     `json:"error,omitempty"`
}

func cmdEval(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	promptFlag := fs.String("prompt", "", "evaluate this prompt id (default: every prompt with declared evals)")
	provider := fs.String("provider", "", "override provider for all evals (anthropic|openai|gemini|ollama|mock); inferred from model if absent")
	parallel := fs.Int("parallel", 8, "max concurrent provider calls")
	noCache := fs.Bool("no-cache", false, "disable response cache")
	cacheDir := fs.String("cache", ".promptlock/cache", "cache directory")
	allowCustom := fs.Bool("allow-custom-metrics", false, "permit `custom` metric scripts (security: arbitrary exec)")
	ci := fs.Bool("ci", false, "machine-readable JSON output; exit 1 on any failure")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	disco, err := prompt.Discover(prompt.DiscoverOptions{
		Root: cwd,
		IDFilter: func(id string) bool {
			if *promptFlag == "" {
				return true
			}
			return id == *promptFlag
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(disco) == 0 {
		fmt.Fprintln(stderr, "promptlock: no prompts to evaluate")
		return ExitConfig
	}

	var cache *eval.Cache
	if !*noCache {
		path := *cacheDir
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		c, err := eval.NewCache(path)
		if err != nil {
			fmt.Fprintln(stderr, "promptlock: cache init:", err)
			return ExitConfig
		}
		cache = c
	}

	var allResults []runResult
	exitCode := ExitOK
	for _, d := range disco {
		res, code := evalOnePrompt(d, cwd, *provider, *parallel, *allowCustom, *ci, cache, stdout, stderr)
		if res != nil {
			allResults = append(allResults, *res)
		}
		if code > exitCode {
			exitCode = code
		}
	}

	if *ci || common.format == "json" {
		out := map[string]any{
			"schema_version":     1,
			"promptlock_version": currentVersion,
			"results":            allResults,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return exitCode
	}
	for _, res := range allResults {
		if res.Err != "" {
			fmt.Fprintf(stdout, "✗ %s: %s\n", res.PromptID, res.Err)
			continue
		}
		for _, run := range res.Runs {
			marker := "✓"
			if !run.Passed {
				marker = "✗"
			}
			fmt.Fprintf(stdout, "%s %s · %s · %s · score=%.3f (threshold %.3f) · %d rows · %d/%d tokens\n",
				marker, run.PromptID, run.Metric, run.Provider,
				run.Aggregate, run.Threshold,
				len(run.Rows), run.InputTokens, run.OutputTokens)
		}
	}
	return exitCode
}

// evalOnePrompt loads, picks provider, runs all evals declared on one prompt.
// Returns nil + ExitOK when the prompt has no declared evals (skip).
func evalOnePrompt(
	d prompt.Discovered,
	cwd string,
	providerOverride string,
	parallel int,
	allowCustom bool,
	ci bool,
	cache *eval.Cache,
	stdout, stderr io.Writer,
) (*runResult, int) {
	p, err := prompt.LoadDiscovered(d)
	if err != nil {
		fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
		return nil, ExitOK
	}
	if len(p.Frontmatter.Evals) == 0 {
		return nil, ExitOK
	}
	datasets := map[string][]eval.Row{}
	for _, ev := range p.Frontmatter.Evals {
		if ev.Dataset == "" {
			continue
		}
		if _, loaded := datasets[ev.Dataset]; loaded {
			continue
		}
		path := ev.Dataset
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		rows, err := eval.LoadDataset(path)
		if err != nil {
			return &runResult{PromptID: p.Frontmatter.ID, Err: err.Error()}, ExitConfig
		}
		datasets[ev.Dataset] = rows
	}

	provName := providerOverride
	if provName == "" {
		provName, _ = providers.InferProvider(p.Frontmatter.Model)
		if provName == "" {
			return &runResult{
				PromptID: p.Frontmatter.ID,
				Err:      fmt.Sprintf("cannot infer provider for model %q (pass --provider)", p.Frontmatter.Model),
			}, ExitConfig
		}
	}
	prov, err := providers.Get(provName)
	if err != nil {
		return &runResult{PromptID: p.Frontmatter.ID, Err: err.Error()}, ExitConfig
	}

	opts := eval.Options{
		Provider:           prov,
		Cache:              cache,
		Parallel:           parallel,
		AllowCustomMetrics: allowCustom,
		OnRunStart: func(promptID, datasetPath, metric string, total int) {
			// Progress goes to stderr so JSON / --ci output stays clean for piping.
			fmt.Fprintf(stderr, "→ %s · %s · %s · %s · %d rows\n",
				promptID, prov.Name(), metric, datasetPath, total)
		},
	}

	runs, err := eval.RunPrompt(context.Background(), p, datasets, opts)
	res := &runResult{PromptID: p.Frontmatter.ID, Runs: runs}
	code := ExitOK
	if err != nil {
		res.Err = err.Error()
		if providers.IsAuthErr(err) {
			code = ExitConfig
		} else {
			code = ExitProvider
		}
	}
	for _, run := range runs {
		if !run.Passed && code < ExitAssertion {
			code = ExitAssertion
		}
	}
	return res, code
}
