// Package render implements a small Jinja-compatible subset for prompt
// templating: {{var}} substitution, dotted lookups, and the filters default,
// tojson, upper, lower, length. Undeclared variables (without a default
// filter) are an error rather than silently substituting empty strings.
//
// Control-flow tags (if / for), custom filters, whitespace control, and
// includes are not supported.
package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ErrMissingVar is returned when a template references an undefined variable
// without a `default(...)` filter.
type ErrMissingVar struct{ Name string }

func (e *ErrMissingVar) Error() string {
	return fmt.Sprintf("template references {{%s}} but no value was provided", e.Name)
}

// Render substitutes {{...}} expressions in tmpl using values from vars.
// Missing variables cause ErrMissingVar unless a `default(...)` filter is used.
func Render(tmpl string, vars map[string]any) (string, error) {
	var firstErr error
	out := tagPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		expr := strings.TrimSpace(match[2 : len(match)-2])
		val, err := evalExpr(expr, vars)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// tagPattern matches `{{ ... }}` lazily. Multiline-safe.
var tagPattern = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// evalExpr parses a single expression body (already trimmed of `{{` `}}`).
// Grammar: <var> ( '|' <filter>('(' arg ')')? )*
func evalExpr(expr string, vars map[string]any) (string, error) {
	parts := splitPipe(expr)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty template expression")
	}
	varName := strings.TrimSpace(parts[0])
	val, hasVal := lookup(varName, vars)
	hasDefault := false

	// Walk filters left-to-right; some filters synthesize a value (default()).
	for _, raw := range parts[1:] {
		fname, arg, hasArg := parseFilter(strings.TrimSpace(raw))
		switch fname {
		case "default":
			if !hasArg {
				return "", fmt.Errorf("filter `default` requires an argument")
			}
			if !hasVal {
				val = arg
				hasVal = true
				hasDefault = true
			}
		case "tojson":
			if !hasVal {
				return "", &ErrMissingVar{Name: varName}
			}
			b, err := json.Marshal(val)
			if err != nil {
				return "", fmt.Errorf("tojson: %w", err)
			}
			val = string(b)
		case "upper":
			if !hasVal {
				return "", &ErrMissingVar{Name: varName}
			}
			val = strings.ToUpper(coerceString(val))
		case "lower":
			if !hasVal {
				return "", &ErrMissingVar{Name: varName}
			}
			val = strings.ToLower(coerceString(val))
		case "length":
			if !hasVal {
				return "", &ErrMissingVar{Name: varName}
			}
			val = lengthOf(val)
		default:
			return "", fmt.Errorf("unknown filter %q", fname)
		}
	}
	if !hasVal {
		return "", &ErrMissingVar{Name: varName}
	}
	if hasDefault {
		_ = hasDefault // (no special path; default values go through coerceString)
	}
	return coerceString(val), nil
}

// lookup supports dotted access: "user.name" walks into nested maps.
func lookup(name string, vars map[string]any) (any, bool) {
	if vars == nil {
		return nil, false
	}
	if !strings.Contains(name, ".") {
		v, ok := vars[name]
		return v, ok
	}
	cur := any(vars)
	for _, seg := range strings.Split(name, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[seg]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// parseFilter splits "name" or "name(arg)" into pieces. The argument is
// stripped of one layer of surrounding quotes if quoted.
func parseFilter(s string) (name, arg string, hasArg bool) {
	open := strings.Index(s, "(")
	if open < 0 {
		return s, "", false
	}
	if !strings.HasSuffix(s, ")") {
		return s, "", false
	}
	name = strings.TrimSpace(s[:open])
	body := strings.TrimSpace(s[open+1 : len(s)-1])
	if len(body) >= 2 && (body[0] == '"' && body[len(body)-1] == '"' ||
		body[0] == '\'' && body[len(body)-1] == '\'') {
		body = body[1 : len(body)-1]
	}
	return name, body, true
}

// splitPipe splits an expression on `|`, but only when the pipe is *not*
// inside a parenthesized argument. Conservative: we don't support nested
// parens or strings containing `|` characters in v1 (template-author scope).
func splitPipe(expr string) []string {
	var out []string
	depth := 0
	last := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 {
				out = append(out, expr[last:i])
				last = i + 1
			}
		}
	}
	out = append(out, expr[last:])
	return out
}

func coerceString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func lengthOf(v any) any {
	switch x := v.(type) {
	case string:
		return len([]rune(x))
	case []any:
		return len(x)
	case map[string]any:
		return len(x)
	}
	return 0
}
