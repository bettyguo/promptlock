---
id: "echo/echo"
version: "0.1.0"
model: "mock/echo"
description: "Mock prompt: returns the echoed input. Used to dogfood `promptlock eval`."
inputs:
  - name: payload
    type: string
evals:
  - dataset: tests/datasets/echo.jsonl
    metric: exact_match
    threshold: 0.5
---

# User

echo: {{payload}}
