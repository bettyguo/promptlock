package diff

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/promptlock/promptlock/internal/prompt"
)

// diffFrontmatter compares two parsed frontmatters and returns scalar field
// changes. Inputs[] are diffed separately by diffInputs.
func diffFrontmatter(before, after prompt.Frontmatter) []FrontmatterChange {
	var changes []FrontmatterChange

	// Scalar fields we explicitly compare. Order matters for deterministic output.
	type scalar struct {
		path string
		a    any
		b    any
	}
	scalars := []scalar{
		{"id", before.ID, after.ID},
		{"version", before.Version, after.Version},
		{"description", before.Description, after.Description},
		{"model", before.Model, after.Model},
		{"temperature", derefF64(before.Temperature), derefF64(after.Temperature)},
		{"max_tokens", derefInt(before.MaxTokens), derefInt(after.MaxTokens)},
		{"top_p", derefF64(before.TopP), derefF64(after.TopP)},
	}
	for _, s := range scalars {
		if changeOf(s.a, s.b); !equalAny(s.a, s.b) {
			changes = append(changes, FrontmatterChange{
				Path: s.path, Before: s.a, After: s.b, Kind: changeOf(s.a, s.b),
			})
		}
	}

	// Slices we compare structurally (ordered).
	if !equalStrSlice(before.Stop, after.Stop) {
		changes = append(changes, FrontmatterChange{
			Path: "stop", Before: before.Stop, After: after.Stop, Kind: changeOf(before.Stop, after.Stop),
		})
	}
	if !equalStrSlice(before.Tags, after.Tags) {
		changes = append(changes, FrontmatterChange{
			Path: "tags", Before: before.Tags, After: after.Tags, Kind: changeOf(before.Tags, after.Tags),
		})
	}

	// outputs.schema: shallow structural diff only.
	beforeSchema := outputsSchema(before.Outputs)
	afterSchema := outputsSchema(after.Outputs)
	if !reflect.DeepEqual(beforeSchema, afterSchema) {
		changes = append(changes, FrontmatterChange{
			Path: "outputs.schema", Before: beforeSchema, After: afterSchema,
			Kind: changeOf(beforeSchema, afterSchema),
		})
	}

	// Evals: match by (dataset, metric); changes shown path-indexed.
	changes = append(changes, diffEvals(before.Evals, after.Evals)...)

	// Extra (unknown) keys: union of both sides.
	keys := map[string]struct{}{}
	for k := range before.Extra {
		keys[k] = struct{}{}
	}
	for k := range after.Extra {
		keys[k] = struct{}{}
	}
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	for _, k := range sortedKeys {
		bv, bok := before.Extra[k]
		av, aok := after.Extra[k]
		if !bok {
			changes = append(changes, FrontmatterChange{Path: k, After: av, Kind: "added"})
			continue
		}
		if !aok {
			changes = append(changes, FrontmatterChange{Path: k, Before: bv, Kind: "removed"})
			continue
		}
		if !reflect.DeepEqual(bv, av) {
			changes = append(changes, FrontmatterChange{Path: k, Before: bv, After: av, Kind: "modified"})
		}
	}
	return changes
}

func diffEvals(before, after []prompt.Eval) []FrontmatterChange {
	type key struct{ dataset, metric string }
	bMap := map[key]prompt.Eval{}
	aMap := map[key]prompt.Eval{}
	for _, e := range before {
		bMap[key{e.Dataset, e.Metric}] = e
	}
	for _, e := range after {
		aMap[key{e.Dataset, e.Metric}] = e
	}
	keys := map[key]struct{}{}
	for k := range bMap {
		keys[k] = struct{}{}
	}
	for k := range aMap {
		keys[k] = struct{}{}
	}
	keyList := make([]key, 0, len(keys))
	for k := range keys {
		keyList = append(keyList, k)
	}
	sort.Slice(keyList, func(i, j int) bool {
		if keyList[i].dataset != keyList[j].dataset {
			return keyList[i].dataset < keyList[j].dataset
		}
		return keyList[i].metric < keyList[j].metric
	})
	var out []FrontmatterChange
	for _, k := range keyList {
		path := fmt.Sprintf("evals[%s/%s]", k.dataset, k.metric)
		bv, bok := bMap[k]
		av, aok := aMap[k]
		switch {
		case !bok:
			out = append(out, FrontmatterChange{Path: path, After: evalToMap(av), Kind: "added"})
		case !aok:
			out = append(out, FrontmatterChange{Path: path, Before: evalToMap(bv), Kind: "removed"})
		default:
			if !reflect.DeepEqual(evalToMap(bv), evalToMap(av)) {
				out = append(out, FrontmatterChange{Path: path, Before: evalToMap(bv), After: evalToMap(av), Kind: "modified"})
			}
		}
	}
	return out
}

func evalToMap(e prompt.Eval) map[string]any {
	m := map[string]any{
		"dataset":   e.Dataset,
		"metric":    e.Metric,
		"threshold": e.Threshold,
	}
	if e.Provider != "" {
		m["provider"] = e.Provider
	}
	if e.Model != "" {
		m["model"] = e.Model
	}
	if e.Temperature != nil {
		m["temperature"] = *e.Temperature
	}
	if e.MetricConfig != nil {
		m["metric_config"] = e.MetricConfig
	}
	return m
}

// diffInputs returns input-level changes (matched by name).
func diffInputs(before, after []prompt.Input) []InputChange {
	bMap := map[string]prompt.Input{}
	aMap := map[string]prompt.Input{}
	for _, in := range before {
		bMap[in.Name] = in
	}
	for _, in := range after {
		aMap[in.Name] = in
	}
	names := map[string]struct{}{}
	for n := range bMap {
		names[n] = struct{}{}
	}
	for n := range aMap {
		names[n] = struct{}{}
	}
	sortedNames := make([]string, 0, len(names))
	for n := range names {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)
	var out []InputChange
	for _, n := range sortedNames {
		bv, bok := bMap[n]
		av, aok := aMap[n]
		switch {
		case !bok:
			out = append(out, InputChange{Name: n, Kind: "added", After: inputToMap(av)})
		case !aok:
			out = append(out, InputChange{Name: n, Kind: "removed", Before: inputToMap(bv)})
		default:
			bm := inputToMap(bv)
			am := inputToMap(av)
			if !reflect.DeepEqual(bm, am) {
				out = append(out, InputChange{Name: n, Kind: "modified", Before: bm, After: am})
			}
		}
	}
	return out
}

func inputToMap(in prompt.Input) map[string]any {
	m := map[string]any{
		"name": in.Name,
		"type": in.EffectiveType(),
	}
	if in.Description != "" {
		m["description"] = in.Description
	}
	if in.Required != nil {
		m["required"] = *in.Required
	}
	if in.Default != nil {
		m["default"] = in.Default
	}
	return m
}

func outputsSchema(o *prompt.Outputs) map[string]any {
	if o == nil {
		return nil
	}
	return o.Schema
}

func changeOf(a, b any) string {
	az := isZero(a)
	bz := isZero(b)
	switch {
	case az && !bz:
		return "added"
	case !az && bz:
		return "removed"
	default:
		return "modified"
	}
}

func isZero(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case int, int64:
		return x == 0
	case float64:
		return x == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return reflect.ValueOf(v).IsZero()
}

func equalAny(a, b any) bool { return reflect.DeepEqual(a, b) }

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func derefF64(p *float64) any {
	if p == nil {
		return float64(0)
	}
	return *p
}

func derefInt(p *int) any {
	if p == nil {
		return 0
	}
	return *p
}

