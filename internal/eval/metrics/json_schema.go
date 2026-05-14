package metrics

import (
	"context"
	"encoding/json"
	"fmt"
)

// JSONSchema validates that output is valid JSON conforming to a schema.
//
// We implement a minimal JSON Schema subset (draft 2020-12 lite) — enough for
// the v1 prompt format spec:
//
//   - type: "object" | "string" | "integer" | "number" | "boolean" | "array"
//   - required: [field, ...] (object only)
//   - properties: { name: <subschema> } (object only)
//   - items: <subschema> (array only)
//   - enum: [value, ...]
//   - minimum / maximum (number / integer only)
//
// The schema is read from metric_config.schema if set, else from the prompt's
// outputs.schema (passed via cfg["_outputs_schema"] by the runner).
type JSONSchema struct{}

// Name returns "json_schema".
func (*JSONSchema) Name() string { return "json_schema" }

// Score implements Metric.
func (*JSONSchema) Score(_ context.Context, output string, _ any, _, cfg map[string]any) Result {
	schema := schemaFromCfg(cfg)
	if schema == nil {
		return Result{Error: "no schema configured (set metric_config.schema or outputs.schema)"}
	}

	var parsed any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return Result{
			Score:  0,
			Detail: map[string]any{"parse_error": err.Error()},
		}
	}
	if errs := validateSchema(parsed, schema, ""); len(errs) > 0 {
		return Result{
			Score:  0,
			Detail: map[string]any{"schema_errors": errs},
		}
	}
	return Result{Score: 1.0}
}

func schemaFromCfg(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	if s, ok := cfg["schema"].(map[string]any); ok {
		return s
	}
	if s, ok := cfg["_outputs_schema"].(map[string]any); ok {
		return s
	}
	return nil
}

// validateSchema returns a list of human-readable errors. Empty = valid.
func validateSchema(value any, schema map[string]any, path string) []string {
	if schema == nil {
		return nil
	}
	var errs []string
	prefix := path
	if prefix == "" {
		prefix = "$"
	}

	if t, ok := schema["type"].(string); ok {
		if !typeMatches(value, t) {
			return []string{fmt.Sprintf("%s: expected type %q, got %s", prefix, t, jsonTypeOf(value))}
		}
	}

	if enum, ok := schema["enum"].([]any); ok {
		ok := false
		for _, opt := range enum {
			if jsonEqual(opt, value) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: value %v not in enum %v", prefix, value, enum))
		}
	}

	switch v := value.(type) {
	case map[string]any:
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				rs, _ := r.(string)
				if _, present := v[rs]; !present {
					errs = append(errs, fmt.Sprintf("%s: missing required field %q", prefix, rs))
				}
			}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for k, sub := range props {
				if subschema, ok := sub.(map[string]any); ok {
					if subVal, present := v[k]; present {
						errs = append(errs, validateSchema(subVal, subschema, prefix+"."+k)...)
					}
				}
			}
		}
	case []any:
		if items, ok := schema["items"].(map[string]any); ok {
			for i, elem := range v {
				errs = append(errs, validateSchema(elem, items, fmt.Sprintf("%s[%d]", prefix, i))...)
			}
		}
	case float64:
		if mn, ok := schema["minimum"].(float64); ok && v < mn {
			errs = append(errs, fmt.Sprintf("%s: %v < minimum %v", prefix, v, mn))
		}
		if mx, ok := schema["maximum"].(float64); ok && v > mx {
			errs = append(errs, fmt.Sprintf("%s: %v > maximum %v", prefix, v, mx))
		}
	}
	return errs
}

func typeMatches(v any, t string) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	case "number":
		_, ok := v.(float64)
		return ok
	case "null":
		return v == nil
	}
	return false
}

func jsonTypeOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
