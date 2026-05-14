package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Custom invokes a user script per row. Contract:
//
//	./my_metric.sh <row.json> <output.txt>
//	# stdout: {"score": 0.87, "details": {...}}
//	# exit code 0 = scored OK; non-zero = metric error
//
// Disabled by default in CI; runner respects --allow-custom-metrics flag and
// returns Result.Error if not allowed. metric_config.command is required.
type Custom struct {
	Allow bool // gate set by runner before Score()
}

// Name returns "custom".
func (*Custom) Name() string { return "custom" }

// Score implements Metric.
func (c *Custom) Score(ctx context.Context, output string, _ any, row, cfg map[string]any) Result {
	if !c.Allow {
		return Result{Error: "custom metrics disabled (pass --allow-custom-metrics to enable)"}
	}
	cmd := cfgString(cfg, "command", "")
	if cmd == "" {
		return Result{Error: "custom: metric_config.command is required"}
	}

	tmp, err := os.CreateTemp("", "promptlock-output-*.txt")
	if err != nil {
		return Result{Error: "custom: tempfile: " + err.Error()}
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(output); err != nil {
		return Result{Error: "custom: write tempfile: " + err.Error()}
	}
	tmp.Close()

	rowJSON, _ := json.Marshal(row)
	process := exec.CommandContext(ctx, cmd, string(rowJSON), tmp.Name())
	out, err := process.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Result{Error: fmt.Sprintf("custom: script exit %d: %s", ee.ExitCode(), ee.Stderr)}
		}
		return Result{Error: "custom: " + err.Error()}
	}
	var parsed struct {
		Score   float64        `json:"score"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return Result{Error: "custom: script output not parseable JSON: " + err.Error()}
	}
	r := Result{Score: parsed.Score}
	if parsed.Details != nil {
		r.Detail = parsed.Details
	}
	return r
}
