---
id: "marketing/subject-line"
version: "0.2.0"
description: "Drafts a marketing email subject line for a campaign brief; judged by an LLM rubric."
model: "claude-opus-4-7"
temperature: 0.7
max_tokens: 64
tags: ["marketing"]

inputs:
  - name: brief
    description: "One-sentence campaign brief."
    type: string
    required: true
  - name: audience
    description: "Target audience descriptor (e.g. 'free-tier developers', 'enterprise buyers')."
    type: string
    required: true

evals:
  - dataset: tests/datasets/subject-line.jsonl
    metric: llm_judge
    threshold: 0.6
    metric_config:
      rubric: |
        Rate the subject line from 0 to 5 on these criteria, then output the integer score:
          - 5: highly compelling, specific, and well-targeted
          - 4: clearly relevant and tempting
          - 3: serviceable but generic
          - 2: weak, off-topic, or wrong audience
          - 0-1: spammy, broken, or completely unrelated
        Output ONLY the integer (0-5). No explanation.
      scale: [0, 5]
      judge_model: "claude-haiku-4-5-20251001"
---

# System

You write marketing email subject lines. Style:
- 50 characters or fewer
- Specific to the audience, not generic
- No emoji unless the brand is playful
- Avoid all-caps, exclamation marks, and "Don't miss" / "Last chance" clichés

# User

Brief: {{brief}}
Audience: {{audience}}

Write the subject line. Output ONLY the subject line — no quotes, no explanation.
