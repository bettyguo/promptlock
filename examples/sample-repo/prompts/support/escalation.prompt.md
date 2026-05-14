---
id: "support/escalation"
version: "0.3.0"
description: "Decides whether a triaged ticket needs to be paged to on-call, with a brief reason."
model: "claude-haiku-4-5-20251001"
temperature: 0.1
max_tokens: 256
tags: ["support", "escalation"]

inputs:
  - name: ticket_summary
    description: "One-paragraph summary of the ticket."
    type: string
    required: true
  - name: customer_tier
    description: "Subscription tier."
    type: string
    required: true
  - name: urgency
    description: "Urgency 1-5 from triage."
    type: integer
    required: true

outputs:
  schema:
    type: object
    required: ["page", "reason"]
    properties:
      page: { type: boolean }
      reason: { type: string }

evals:
  - dataset: tests/datasets/escalation.jsonl
    metric: json_schema
    threshold: 0.95
  - dataset: tests/datasets/escalation.jsonl
    metric: exact_match
    threshold: 0.80
    metric_config:
      field: page
---

# System

You decide whether a triaged support ticket needs to wake up on-call.

Page (`page: true`) only when ALL of these are true:
- urgency is 4 or 5, AND
- customer_tier is "enterprise" or "pro" with documented uptime SLA, AND
- the issue is not a known/scheduled incident

Otherwise, do not page. The cost of unnecessary pages is high. Err on the side of NOT paging when in doubt.

Output strict JSON: `{"page": true|false, "reason": "<one short sentence>"}`. No prose around the JSON.

# User

Tier: {{customer_tier}}
Urgency: {{urgency}}

Ticket:
{{ticket_summary}}
