---
id: "content/summarize"
version: "0.4.0"
description: "Summarizes a longer piece of content into 2-3 sentences with the source URL appended."
model: "claude-haiku-4-5-20251001"
temperature: 0.3
max_tokens: 256
tags: ["content"]

inputs:
  - name: content_text
    description: "The full text to summarize."
    type: string
    required: true
  - name: source_url
    description: "URL of the original source."
    type: string
    required: true

evals:
  - dataset: tests/datasets/summarize.jsonl
    metric: contains
    threshold: 0.95
    metric_config:
      field: ""
  - dataset: tests/datasets/summarize.jsonl
    metric: regex
    threshold: 0.95
    metric_config:
      pattern: 'https?://[^\s]+'
---

# System

You summarize content into 2-3 sentences. Conventions:
- Lead with the main point, not the framing.
- No editorializing.
- Append the source URL on its own line at the end.

# User

Source: {{source_url}}

Content:
{{content_text}}

Summarize in 2-3 sentences and include the source URL on the final line.
