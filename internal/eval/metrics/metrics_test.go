package metrics

import (
	"context"
	"strings"
	"testing"
)

func TestExactMatch_Match(t *testing.T) {
	m := &ExactMatch{}
	r := m.Score(context.Background(), "hello", "HELLO", nil, nil)
	if r.Score != 1.0 {
		t.Errorf("got %v, want 1.0 (case-insensitive default)", r.Score)
	}
}

func TestExactMatch_FieldExtract(t *testing.T) {
	m := &ExactMatch{}
	r := m.Score(context.Background(),
		`{"category":"billing","urgency":3}`,
		map[string]any{"category": "billing", "urgency": 3},
		nil,
		map[string]any{"field": "category"},
	)
	if r.Score != 1.0 {
		t.Errorf("got %v detail=%v", r.Score, r.Detail)
	}
}

func TestContains_Match(t *testing.T) {
	m := &Contains{}
	r := m.Score(context.Background(), "the quick brown fox", "brown", nil, nil)
	if r.Score != 1.0 {
		t.Errorf("got %v", r.Score)
	}
}

func TestContains_NoMatch(t *testing.T) {
	m := &Contains{}
	r := m.Score(context.Background(), "abc", "xyz", nil, nil)
	if r.Score != 0 {
		t.Errorf("got %v", r.Score)
	}
}

func TestRegex_Match(t *testing.T) {
	m := &Regex{}
	r := m.Score(context.Background(), "user_id=123", nil, nil, map[string]any{"pattern": `^user_id=\d+$`})
	if r.Score != 1.0 {
		t.Errorf("got %v", r.Score)
	}
}

func TestRegex_BadPattern(t *testing.T) {
	m := &Regex{}
	r := m.Score(context.Background(), "x", nil, nil, map[string]any{"pattern": "[unclosed"})
	if r.Error == "" {
		t.Error("expected error for bad regex")
	}
}

func TestJSONSchema_Valid(t *testing.T) {
	m := &JSONSchema{}
	r := m.Score(context.Background(),
		`{"category":"billing","urgency":3}`,
		nil,
		nil,
		map[string]any{
			"schema": map[string]any{
				"type":     "object",
				"required": []any{"category", "urgency"},
				"properties": map[string]any{
					"category": map[string]any{"enum": []any{"billing", "technical"}},
					"urgency":  map[string]any{"type": "integer", "minimum": 1.0, "maximum": 5.0},
				},
			},
		},
	)
	if r.Score != 1.0 {
		t.Errorf("got %v detail=%v", r.Score, r.Detail)
	}
}

func TestJSONSchema_Invalid(t *testing.T) {
	m := &JSONSchema{}
	r := m.Score(context.Background(),
		`{"category":"weather","urgency":99}`,
		nil,
		nil,
		map[string]any{
			"schema": map[string]any{
				"type":     "object",
				"required": []any{"category", "urgency"},
				"properties": map[string]any{
					"category": map[string]any{"enum": []any{"billing", "technical"}},
					"urgency":  map[string]any{"type": "integer", "minimum": 1.0, "maximum": 5.0},
				},
			},
		},
	)
	if r.Score != 0 {
		t.Errorf("got %v, want 0", r.Score)
	}
	if errs, ok := r.Detail["schema_errors"].([]string); !ok || len(errs) == 0 {
		t.Errorf("expected schema_errors, got %v", r.Detail)
	}
}

func TestJSONSchema_BadJSON(t *testing.T) {
	m := &JSONSchema{}
	r := m.Score(context.Background(), "not json", nil, nil, map[string]any{
		"schema": map[string]any{"type": "object"},
	})
	if r.Score != 0 || r.Detail["parse_error"] == nil {
		t.Errorf("got %v %v", r.Score, r.Detail)
	}
}

func TestCustom_Disabled(t *testing.T) {
	m := &Custom{Allow: false}
	r := m.Score(context.Background(), "x", nil, nil, map[string]any{"command": "/bin/true"})
	if r.Error == "" || !strings.Contains(r.Error, "disabled") {
		t.Errorf("expected disabled error, got %+v", r)
	}
}

func TestLastNumberHelper(t *testing.T) {
	cases := map[string]float64{
		"4":         4,
		"Score: 3":  3,
		"3.14":      3.14,
		"foo 1 bar 2": 2,
		"-0.5":      -0.5,
	}
	for in, want := range cases {
		got, ok := lastNumber(in)
		if !ok {
			t.Errorf("lastNumber(%q) returned !ok", in)
			continue
		}
		if got != want {
			t.Errorf("lastNumber(%q) = %v, want %v", in, got, want)
		}
	}
}
