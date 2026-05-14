---
id: "onboarding/welcome"
version: "0.2.0"
model: "claude-haiku-4-5-20251001"
description: "First-touch welcome message for newly signed-up users."
tags: ["onboarding"]

inputs:
  - name: user_name
    type: string
---

# User

Write a warm, two-sentence welcome message for {{user_name}}, who just signed up.
