package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/promptlock/promptlock/internal/eval"
	"github.com/promptlock/promptlock/internal/eval/providers"
	"github.com/promptlock/promptlock/internal/lock"
	"github.com/promptlock/promptlock/internal/prompt"
)

// cmdLock implements `promptlock lock` — refresh promptlock.lock entries for
// every (or selected) prompt: recompute content_hash and (unless --no-eval)
// re-run declared evals, then write out the new lockfile.
func cmdLock(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	noEval := fs.Bool("no-eval", false, "update content_hash only; don't re-run evals")
	provider := fs.String("provider", "", "override declared provider (mock | anthropic | openai | gemini | ollama)")
	parallel := fs.Int("parallel", 8, "max concurrent provider calls")
	if err := parseFlexible(fs, args); err != nil {
		return ExitUsage
	}
	cwd, err := common.resolveCwd()
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}

	idsWanted := map[string]bool{}
	for _, a := range fs.Args() {
		idsWanted[a] = true
	}

	disco, err := prompt.Discover(prompt.DiscoverOptions{
		Root: cwd,
		IDFilter: func(id string) bool {
			if len(idsWanted) == 0 {
				return true
			}
			return idsWanted[id]
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	if len(disco) == 0 {
		fmt.Fprintln(stderr, "promptlock: no prompts to lock")
		return ExitConfig
	}

	lockPath := filepath.Join(cwd, lock.Filename)
	lf, err := lock.Load(lockPath)
	if err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	lf.SchemaVersion = lock.SchemaVersion
	lf.GeneratedBy = "promptlock " + currentVersion
	lf.GeneratedAt = time.Now().UTC().Truncate(time.Second)

	cache, _ := eval.NewCache(filepath.Join(cwd, ".promptlock", "cache"))

	for _, d := range disco {
		data, err := os.ReadFile(d.Path)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
			continue
		}
		hash, err := lock.HashFile(data)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
			continue
		}
		p, err := prompt.Parse(d.RelToRoot, data)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", d.RelToRoot, err)
			continue
		}
		entry := lock.Entry{
			ID:          firstNonEmpty(p.Frontmatter.ID, d.IDFromPath),
			Version:     p.Frontmatter.Version,
			File:        d.RelToRoot,
			ContentHash: hash,
		}
		// Preserve previous LastEval unless we're going to refresh it.
		if prev, ok := lf.Find(entry.ID); ok && *noEval {
			entry.LastEval = prev.LastEval
		}

		if !*noEval && len(p.Frontmatter.Evals) > 0 {
			info, err := runEvalsForLock(p, cwd, *provider, *parallel, cache)
			if err != nil {
				fmt.Fprintf(stderr, "✗ %s: eval failed: %v\n", entry.ID, err)
			} else {
				entry.LastEval = info
			}
		}
		lf.Upsert(entry)
		fmt.Fprintf(stdout, "  %s  %s\n", entry.ID, entry.ContentHash[:14]+"…")
	}

	// Remove entries for prompts that no longer exist on disk.
	have := map[string]bool{}
	for _, d := range disco {
		have[d.IDFromPath] = true
	}
	if len(idsWanted) == 0 {
		// Only prune in full-repo mode.
		var toRemove []string
		for _, e := range lf.Prompts {
			if !have[e.ID] {
				toRemove = append(toRemove, e.ID)
			}
		}
		for _, id := range toRemove {
			lf.Remove(id)
			fmt.Fprintf(stdout, "  removed: %s (no longer on disk)\n", id)
		}
	}

	if err := lf.Save(lockPath); err != nil {
		fmt.Fprintln(stderr, "promptlock:", err)
		return ExitConfig
	}
	fmt.Fprintf(stdout, "wrote %s (%d prompt(s))\n", lock.Filename, len(lf.Prompts))
	return ExitOK
}

// runEvalsForLock runs all declared evals for one prompt and returns a fresh
// EvalInfo suitable for the lockfile.
func runEvalsForLock(p *prompt.Prompt, cwd, providerOverride string, parallel int, cache *eval.Cache) (*lock.EvalInfo, error) {
	provName := providerOverride
	if provName == "" {
		provName, _ = providers.InferProvider(p.Frontmatter.Model)
		if provName == "" {
			return nil, fmt.Errorf("cannot infer provider for model %q", p.Frontmatter.Model)
		}
	}
	prov, err := providers.Get(provName)
	if err != nil {
		return nil, err
	}

	// Load datasets.
	datasets := map[string][]eval.Row{}
	dsHashes := map[string]string{}
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
			return nil, err
		}
		datasets[ev.Dataset] = rows
		raw, err := os.ReadFile(path)
		if err == nil {
			dsHashes[ev.Dataset] = lock.HashDataset(raw)
		}
	}

	runs, err := eval.RunPrompt(context.Background(), p, datasets, eval.Options{
		Provider: prov,
		Cache:    cache,
		Parallel: parallel,
	})
	if err != nil {
		return nil, err
	}

	info := &lock.EvalInfo{
		Provider:          prov.Name(),
		Model:             p.Frontmatter.Model,
		Temperature:       p.Frontmatter.Temperature,
		Timestamp:         time.Now().UTC().Truncate(time.Second),
		PromptlockVersion: currentVersion,
	}

	usedDatasets := map[string]bool{}
	for _, run := range runs {
		info.Scores = append(info.Scores, lock.ScoreInfo{
			Dataset:   run.Dataset,
			Metric:    run.Metric,
			Score:     run.Aggregate,
			Threshold: run.Threshold,
			Aggregate: "mean",
		})
		if !usedDatasets[run.Dataset] {
			usedDatasets[run.Dataset] = true
			info.Datasets = append(info.Datasets, lock.DatasetInfo{
				Path: run.Dataset,
				Hash: dsHashes[run.Dataset],
				Rows: len(run.Rows),
			})
		}
	}
	// Sum tokens.
	var inT, outT int
	for _, run := range runs {
		inT += run.InputTokens
		outT += run.OutputTokens
	}
	if inT > 0 || outT > 0 {
		info.Tokens = &lock.TokensInfo{Input: inT, Output: outT}
	}
	return info, nil
}

