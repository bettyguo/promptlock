---
id: "demo/echo-classifier"
version: "0.1.0"
description: "Mock-provider demo: returns whatever follows `echo:` in the user message."
model: "mock/echo"
tags: ["demo"]

inputs:
  - name: phrase
    type: string

evals:
  - dataset: datasets/echo.jsonl
    metric: exact_match
    threshold: 0.99
---

# System

Reply with exactly the text after `echo:` and nothing else.

# User

echo: {{phrase}}
