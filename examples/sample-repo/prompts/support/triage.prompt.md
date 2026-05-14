---
id: "support/triage"
version: "1.0.0"
description: "Classifies an incoming support ticket into a category + urgency."
model: "claude-opus-4-7"
temperature: 0.0
max_tokens: 256
tags: ["support", "classification"]

inputs:
  - name: ticket_text
    description: "The raw customer ticket body."
    type: string

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
  - dataset: tests/datasets/triage.jsonl
    metric: json_schema
    threshold: 1.00
  - dataset: tests/datasets/triage.jsonl
    metric: exact_match
    threshold: 0.85
    metric_config:
      field: category
---

# System

You are a support-ticket classifier. Read the ticket and emit JSON:

```
{ "category": "<billing|technical|feature_request|other>", "urgency": <1-5> }
```

`urgency` rubric: 1 = informational; 3 = standard request; 5 = production outage.
Return JSON only — no prose, no code fences.

# User

Ticket:
{{ticket_text}}
