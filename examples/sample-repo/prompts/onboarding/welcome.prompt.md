---
id: "onboarding/welcome"
version: "0.2.0"
description: "First-touch welcome message for newly signed-up users."
model: "mock/echo"
tags: ["onboarding"]

inputs:
  - name: user_name
    description: "The user's display name."
    type: string

evals:
  - dataset: tests/datasets/welcome.jsonl
    metric: contains
    threshold: 1.00
    metric_config:
      case_sensitive: false
---

# System

Reply with exactly the text after `echo:` and nothing else.

# User

echo: Welcome to the platform, {{user_name}}! Glad to have you on board.
