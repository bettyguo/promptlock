package render_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/render"
)

func TestRender_SimpleSubstitution(t *testing.T) {
	out, err := render.Render("Hello {{name}}!", map[string]any{"name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hello World!" {
		t.Errorf("got %q", out)
	}
}

func TestRender_WhitespaceTolerance(t *testing.T) {
	out, _ := render.Render("{{ name }} and {{name }}", map[string]any{"name": "Alice"})
	if out != "Alice and Alice" {
		t.Errorf("got %q", out)
	}
}

func TestRender_MissingVarErrs(t *testing.T) {
	_, err := render.Render("Hello {{who}}", map[string]any{})
	var miss *render.ErrMissingVar
	if !errors.As(err, &miss) || miss.Name != "who" {
		t.Errorf("want ErrMissingVar(who), got %v", err)
	}
}

func TestRender_DefaultFilter(t *testing.T) {
	out, err := render.Render(`Tier: {{tier | default("free")}}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Tier: free" {
		t.Errorf("got %q", out)
	}
}

func TestRender_DefaultFilter_NotApplied(t *testing.T) {
	out, _ := render.Render(`Tier: {{tier | default("free")}}`, map[string]any{"tier": "pro"})
	if out != "Tier: pro" {
		t.Errorf("got %q", out)
	}
}

func TestRender_UpperLower(t *testing.T) {
	out, _ := render.Render("{{a|upper}} {{b|lower}}", map[string]any{"a": "go", "b": "PROMPT"})
	if out != "GO prompt" {
		t.Errorf("got %q", out)
	}
}

func TestRender_TojsonFilter(t *testing.T) {
	out, err := render.Render(`{{obj | tojson}}`, map[string]any{
		"obj": map[string]any{"k": 1, "v": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// JSON map order is unspecified — check both keys present
	if !strings.Contains(out, `"k":1`) || !strings.Contains(out, `"v":"x"`) {
		t.Errorf("got %q", out)
	}
}

func TestRender_NestedDottedAccess(t *testing.T) {
	out, err := render.Render("{{user.name}}", map[string]any{
		"user": map[string]any{"name": "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Bob" {
		t.Errorf("got %q", out)
	}
}

func TestRender_UnknownFilterErrs(t *testing.T) {
	_, err := render.Render(`{{x | nope}}`, map[string]any{"x": "y"})
	if err == nil || !strings.Contains(err.Error(), "unknown filter") {
		t.Errorf("want unknown-filter error, got %v", err)
	}
}

func TestRender_LengthOnString(t *testing.T) {
	out, _ := render.Render("{{s|length}}", map[string]any{"s": "héllo"})
	if out != "5" {
		t.Errorf("got %q", out)
	}
}

func TestRender_PreservesNonExpressionContent(t *testing.T) {
	in := "Plain text\n\n{ not template } { also not }\nLine."
	out, err := render.Render(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("got %q", out)
	}
}
