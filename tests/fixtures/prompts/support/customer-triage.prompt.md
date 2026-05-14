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
  - dataset: datasets/support_triage_v1.jsonl
    metric: json_schema
    threshold: 0.95
  - dataset: datasets/support_triage_v1.jsonl
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
