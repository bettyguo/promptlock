// Package eval orchestrates eval runs: load dataset rows, render the prompt
// per row, call the provider, score against the configured metric, aggregate.
package eval

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Row is one dataset row: input values for the prompt + an optional `expected`
// value used by some metrics. Extra fields beyond the prompt's declared inputs
// are preserved (Extra) — useful for `metric_config.field` and similar.
type Row struct {
	Inputs   map[string]any
	Expected any              // value of the "expected" key, if present
	Extra    map[string]any   // anything beyond Inputs and Expected
	Index    int              // 1-based row number for error messages
}

// LoadDataset reads a JSONL or CSV dataset from path. Format is inferred from
// file extension (.jsonl, .json → JSONL; .csv → CSV).
func LoadDataset(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("dataset %s: %w", path, err)
	}
	defer f.Close()

	switch {
	case strings.HasSuffix(strings.ToLower(path), ".csv"):
		return loadCSV(f)
	default:
		return loadJSONL(f)
	}
}

// loadJSONL reads newline-delimited JSON objects.
func loadJSONL(r io.Reader) ([]Row, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1 MB max line
	var out []Row
	idx := 0
	for scanner.Scan() {
		idx++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("dataset row %d: invalid JSON: %w", idx, err)
		}
		row := Row{Inputs: map[string]any{}, Extra: map[string]any{}, Index: idx}
		for k, v := range raw {
			switch k {
			case "expected":
				row.Expected = v
			default:
				// Heuristic: any key the prompt's inputs[] declares is an input;
				// anything else is Extra. Without knowing the prompt here, we
				// stash everything in Inputs and let the runner re-bucket.
				row.Inputs[k] = v
			}
		}
		out = append(out, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("dataset read: %w", err)
	}
	return out, nil
}

// loadCSV reads a CSV file with a header row. Cells that look like JSON
// (start with `{`, `[`, or `"`) are JSON-parsed; otherwise kept as strings.
func loadCSV(r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("dataset header: %w", err)
	}
	for i, h := range header {
		header[i] = strings.TrimSpace(h)
	}

	var out []Row
	idx := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dataset row %d: %w", idx+1, err)
		}
		idx++
		row := Row{Inputs: map[string]any{}, Extra: map[string]any{}, Index: idx}
		for i, cell := range rec {
			if i >= len(header) {
				continue
			}
			val := decodeCell(cell)
			if header[i] == "expected" {
				row.Expected = val
			} else {
				row.Inputs[header[i]] = val
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func decodeCell(s string) any {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	switch t[0] {
	case '{', '[', '"':
		var v any
		if err := json.Unmarshal([]byte(t), &v); err == nil {
			return v
		}
	}
	// Try a few primitives.
	if t == "true" {
		return true
	}
	if t == "false" {
		return false
	}
	return s
}
