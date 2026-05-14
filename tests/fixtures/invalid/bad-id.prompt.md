---
id: "Invalid/Bad ID"
version: "not-a-semver"
model: "claude-opus-4-7"
inputs:
  - name: 1bad
    type: weirdtype
evals:
  - dataset: tests/datasets/x.jsonl
    metric: bogus_metric
    threshold: 5
---

# User

References {{undeclared_var}} which is not declared.
