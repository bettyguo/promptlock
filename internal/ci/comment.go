// Package ci renders Markdown PR comments from eval results and posts them to
// GitHub or GitLab via their REST APIs. Auth is read from env (GITHUB_TOKEN /
// CI_JOB_TOKEN); repo and PR number come from the standard CI env vars.
package ci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// EvalResults is the parsed `promptlock eval --ci` JSON we render from.
type EvalResults struct {
	SchemaVersion     int             `json:"schema_version"`
	PromptlockVersion string          `json:"promptlock_version"`
	Results           []PromptResults `json:"results"`
}

// PromptResults is one prompt's bundle of eval runs.
type PromptResults struct {
	PromptID string    `json:"prompt_id"`
	Runs     []RunInfo `json:"runs"`
	Err      string    `json:"error,omitempty"`
}

// RunInfo is one (prompt, dataset, metric) eval row.
type RunInfo struct {
	PromptID     string  `json:"prompt_id"`
	Version      string  `json:"version"`
	Dataset      string  `json:"dataset"`
	Metric       string  `json:"metric"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Rows         int     `json:"rows"`
	Score        float64 `json:"score"`
	Threshold    float64 `json:"threshold"`
	Passed       bool    `json:"passed"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// CommentMarker is the HTML marker we use to identify our PR comment for
// idempotent updates. Includes the repo slug so multiple promptlock-using
// repos don't collide if the same bot posts on cross-repo issues.
const CommentMarker = "<!-- promptlock-comment v1 -->"

// LoadResults parses an `eval --ci` JSON document.
func LoadResults(r io.Reader) (*EvalResults, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("ci: read results: %w", err)
	}
	var er EvalResults
	if err := json.Unmarshal(data, &er); err != nil {
		return nil, fmt.Errorf("ci: parse results: %w", err)
	}
	return &er, nil
}

// RenderMarkdown produces the PR comment body from results.
func RenderMarkdown(er *EvalResults) string {
	var b strings.Builder
	b.WriteString(CommentMarker)
	b.WriteString("\n## promptlock eval\n\n")

	if len(er.Results) == 0 {
		b.WriteString("_No prompts evaluated._\n")
		return b.String()
	}

	totalPrompts, totalFailing, totalRows := 0, 0, 0
	totalIn, totalOut := 0, 0
	results := append([]PromptResults(nil), er.Results...)
	sort.Slice(results, func(i, j int) bool { return results[i].PromptID < results[j].PromptID })

	for _, pr := range results {
		totalPrompts++
		failing := false
		for _, run := range pr.Runs {
			if !run.Passed {
				failing = true
			}
			totalRows += run.Rows
			totalIn += run.InputTokens
			totalOut += run.OutputTokens
		}
		if failing || pr.Err != "" {
			totalFailing++
		}

		fmt.Fprintf(&b, "### `%s`", pr.PromptID)
		if len(pr.Runs) > 0 && pr.Runs[0].Version != "" {
			fmt.Fprintf(&b, "  · v%s", pr.Runs[0].Version)
		}
		b.WriteString("\n\n")

		if pr.Err != "" {
			fmt.Fprintf(&b, "❌ **error:** %s\n\n", pr.Err)
			continue
		}

		b.WriteString("| metric | dataset | score | threshold | rows | status |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, run := range pr.Runs {
			status := "✅ pass"
			if !run.Passed {
				status = "❌ fail"
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %.3f | %.3f | %d | %s |\n",
				run.Metric, dispDataset(run.Dataset), run.Score, run.Threshold, run.Rows, status)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "**%d / %d prompts pass.** ", totalPrompts-totalFailing, totalPrompts)
	if totalFailing > 0 {
		fmt.Fprintf(&b, "CI failed.\nAfter review, re-run `promptlock lock` locally to accept new baselines.\n")
	} else {
		b.WriteString("CI passed.\n")
	}
	fmt.Fprintf(&b, "\n_Eval cost: %d input + %d output tokens across %d rows. promptlock %s._\n",
		totalIn, totalOut, totalRows, er.PromptlockVersion)

	return b.String()
}

func dispDataset(p string) string {
	if p == "" {
		return "(none)"
	}
	// Trim long prefixes for readability in tables.
	if len(p) > 40 {
		return "…" + p[len(p)-39:]
	}
	return p
}

// ResolvePRNumber pulls the PR number from $GITHUB_REF (refs/pull/<n>/merge)
// or returns an explicit override.
func ResolvePRNumber(githubRef, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if githubRef == "" {
		return "", fmt.Errorf("ci: GITHUB_REF unset and no --pr override")
	}
	parts := strings.Split(githubRef, "/")
	for i, p := range parts {
		if p == "pull" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("ci: GITHUB_REF %q is not a pull-request ref", githubRef)
}

// commentJSON is the GitHub-API request body shape for create/update.
type commentJSON struct {
	Body string `json:"body"`
}

// MarshalCommentJSON builds the request body for POST/PATCH.
func MarshalCommentJSON(body string) []byte {
	b, _ := json.Marshal(commentJSON{Body: body})
	return b
}

// FoldExistingMarker returns true if `body` already contains the promptlock
// marker. Used to identify our comment among a list of issue comments.
func FoldExistingMarker(body string) bool {
	return bytes.Contains([]byte(body), []byte(CommentMarker))
}
