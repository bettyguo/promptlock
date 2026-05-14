# Prompt file format

Additive frontmatter fields are forward-compatible: the parser warns on unknown
keys but accepts them.

## File extension and discovery

- Extension: `.prompt.md` (e.g. `support/triage.prompt.md`).
- Discovery: by default `promptlock` walks `prompts/**/*.prompt.md` from the
  repo root.

## Top-level structure

```
---
<YAML frontmatter>
---

# System
<system message, optional>

# User
<user message, required>

# Assistant
<assistant priming, optional, for few-shot>
```

Headings are `H1` (`#`). Any `H2`+ headings inside a section are content for that section. The first occurrence of `# System`, `# User`, or `# Assistant` (in document order) ends the prior section.

## Frontmatter: required fields

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Path-like identifier, e.g. `support/customer-triage`. Must match the file's path under `prompts/` (without extension); enforced by `validate`. |
| `version` | string | Semver (`MAJOR.MINOR.PATCH`). Drives diff semantics, lockfile, rollback. |
| `model` | string | Default model for evals. Provider inferred from the model ID prefix unless overridden in `evals[].provider`. |

## Frontmatter: optional fields

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `description` | string | "" | One-liner shown by `promptlock list`. |
| `tags` | string[] | [] | Filter target for `--tag`. |
| `temperature` | float | provider default | |
| `max_tokens` | int | provider default | |
| `top_p` | float | provider default | |
| `stop` | string[] | [] | |
| `inputs` | array | [] | See "Inputs" below. |
| `outputs.schema` | JSON Schema object | none | Drives `json_schema` metric and `validate`. |
| `evals` | array | [] | See "Evals" below. |
| `metadata` | object | {} | Free-form user metadata; opaque to promptlock. |

## Inputs

Each entry:

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | Required. Must be a valid Jinja identifier. Referenced as `{{name}}` in the body. |
| `description` | string | Optional. |
| `type` | string | One of `string` (default), `integer`, `float`, `boolean`, `array`, `object`. Validated against eval dataset rows. |
| `required` | bool | Default `true`. |
| `default` | any | Required if `required: false` and used in body. |

## Evals

Each entry declares one eval suite for this prompt:

| Field | Type | Notes |
|-------|------|-------|
| `dataset` | string | Path to JSONL or CSV file, relative to repo root. Each row provides values for the prompt's `inputs` plus an `expected` field. |
| `metric` | string | One of `exact_match`, `regex`, `contains`, `json_schema`, `llm_judge`, `custom`. |
| `threshold` | float | 0.0–1.0. Mean score below this fails the eval. |
| `provider` | string | Optional override. Default: derived from top-level `model`. |
| `model` | string | Optional override. Default: top-level `model`. |
| `temperature` | float | Optional override for the eval run. |
| `metric_config` | object | Metric-specific config (e.g. regex pattern, llm_judge rubric). |

## Body templating

- Jinja-compatible `{{var}}` substitution for declared `inputs`.
- Supported subset: simple variable substitution and basic filters
  (`upper`, `lower`, `default`, `length`, `tojson`).
- No control-flow tags, no custom filters, no file includes.
- Unrendered `{{var}}` references that don't match a declared input fail `validate`.

## Canonical example

```markdown
---
id: "support/customer-triage"
version: "1.4.0"
description: "Categorizes incoming support tickets and assigns urgency."
model: "claude-opus-4-7"
temperature: 0.3
max_tokens: 1024
tags: ["support", "classification"]

inputs:
  - name: ticket_text
    description: "The raw customer ticket body."
    type: string
    required: true
  - name: customer_tier
    description: "Subscription tier of the customer."
    type: string
    required: false
    default: "free"

outputs:
  schema:
    type: object
    required: ["category", "urgency"]
    properties:
      category:
        enum: ["billing", "technical", "feature_request", "other"]
      urgency:
        type: integer
        minimum: 1
        maximum: 5

evals:
  - dataset: tests/datasets/support_triage_v1.jsonl
    metric: json_schema
    threshold: 0.95
  - dataset: tests/datasets/support_triage_v1.jsonl
    metric: exact_match
    threshold: 0.85
    metric_config:
      field: category
---

# System

You are a customer support classifier. You read incoming support tickets
and emit a strict JSON object with two fields: `category` and `urgency`.

The customer is on the `{{customer_tier}}` tier.

Output JSON only. No prose.

# User

Ticket:
{{ticket_text}}
```

## Hashing (lockfile-relevant)

The `content_hash` recorded in `promptlock.lock` is `SHA256` of the *normalized* file:

1. Line endings normalized to `\n`.
2. Trailing whitespace stripped from each line.
3. Trailing blank lines collapsed to a single `\n`.
4. Frontmatter YAML re-serialized with sorted keys (so `model: x\ntemperature: 0.3` and `temperature: 0.3\nmodel: x` hash equally).
5. Body left as-is after normalization.

Whitespace-only edits don't change the hash, so they don't trigger `drift`.

## Validation rules

`promptlock validate` enforces:

- All required frontmatter fields present and well-typed.
- `id` matches file path under `prompts/`.
- `version` is valid semver.
- All `{{var}}` references in body resolve to a declared input.
- All declared `inputs` are referenced in body (warning, not error: optional inputs may be intentionally unused in the template).
- `evals[].dataset` files exist and contain rows compatible with the declared `inputs`.
- `outputs.schema` is valid JSON Schema (draft 2020-12).
- No duplicate `id` across the repo.
