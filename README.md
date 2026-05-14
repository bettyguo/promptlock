# promptlock

Workflow tooling for production prompts. File-based, language-agnostic, OSS.

Prompts live as plain markdown files in your repo. `promptlock` adds versioning,
semantic diff, eval-on-PR, lockfiles, drift detection, and rollback, on top of
your existing Git workflow.

## Why

Prompts are production assets. They have semantics, versions, regressions, and
reviewers. Today they tend to live in one of two unhappy places:

1. A vendor SaaS (PromptLayer, Humanloop, LangSmith). PR review is impossible,
   the audit log lives in someone else's database, and your incident response
   degrades when their service is down.
2. A `prompts/` folder in your repo, ungoverned. Git's line-diff doesn't know
   that adding the word "carefully" to a system message can shift behavior, and
   there's no CI gate that says "this prompt now scores 0.43 on the eval suite
   where it used to score 0.91; do not merge."

`promptlock` is the missing layer between (2) and the engineering practices we
already use for code.

## Comparison

|                                              | promptlock | promptfoo | PromptLayer | Langfuse | Humanloop |
|----------------------------------------------|:----------:|:---------:|:-----------:|:--------:|:---------:|
| File-based, lives in your repo               | yes        | partial   | no (SaaS)   | no       | no (SaaS) |
| Lockfile (eval scores + content hash)        | yes        | no        | no          | no       | no        |
| Semantic diff (word-level, frontmatter)      | yes        | no        | UI-only     | UI-only  | UI-only   |
| Multi-provider eval                          | yes        | yes       | partial     | yes      | yes       |
| OSS / MIT                                    | yes        | yes       | no          | core     | no        |
| CI integration with regression gating        | yes        | partial   | partial     | no       | partial   |

## Quickstart

```bash
git clone https://github.com/promptlock/promptlock && cd promptlock
go build -o promptlock ./cmd/promptlock

# Then, in a repo with prompts/*.prompt.md files somewhere:
./promptlock list
./promptlock validate
./promptlock diff support/triage
./promptlock eval
./promptlock lock
./promptlock check          # this is the CI gate

# Bump and roll back
./promptlock version bump support/triage --minor
./promptlock rollback support/triage 1.3.0
```

In CI (a working sample sits at [`examples/ci-integration/`](examples/ci-integration/)):

```yaml
on: [pull_request]
jobs:
  eval:
    runs-on: ubuntu-latest
    permissions: { contents: read, pull-requests: write }
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - run: |
          curl -fsSL https://github.com/promptlock/promptlock/releases/latest/download/install.sh | sh
          echo "$HOME/.local/bin" >> $GITHUB_PATH
      - run: promptlock validate && promptlock check
      - run: promptlock eval --ci > eval.json
        env: { ANTHROPIC_API_KEY: '${{ secrets.ANTHROPIC_API_KEY }}' }
      - run: promptlock comment --github --from eval.json
        env: { GITHUB_TOKEN: '${{ secrets.GITHUB_TOKEN }}' }
```

## Format

Prompts are markdown files with YAML frontmatter. See [docs/format.md](docs/format.md)
for the full spec.

```markdown
---
id: "support/customer-triage"
version: "1.4.0"
model: "claude-opus-4-7"
temperature: 0.3
inputs:
  - name: ticket_text
    type: string
evals:
  - dataset: datasets/support_triage_v1.jsonl
    metric: exact_match
    threshold: 0.85
---

# System
You are a customer support classifier...

# User
Ticket: {{ticket_text}}
```

The format is a deliberate superset of Microsoft's `.prompty`. A `.prompty`
importer is on the roadmap.

## Lockfile

`promptlock.lock` is YAML, committed alongside your prompts:

```yaml
schema_version: 1
prompts:
  - id: support/customer-triage
    version: 1.4.0
    file: prompts/support/customer-triage.prompt.md
    content_hash: sha256:7c8e9a2b...
    last_eval:
      provider: anthropic
      model: claude-opus-4-7
      timestamp: 2026-05-14T11:04:54Z
      scores:
        - dataset: datasets/support_triage_v1.jsonl
          metric: exact_match
          score: 0.88
          threshold: 0.85
```

`promptlock check` fails when the working tree's content hash doesn't match the
lockfile. `promptlock lock` updates it after you've reviewed.

## Commands

| | |
|---|---|
| `list` | discovered prompts |
| `show <id>` | one prompt, fully parsed |
| `validate` | format spec + var-ref + cross-references |
| `diff <id> [--against ref]` | semantic diff (frontmatter + body word-level) |
| `eval [--changed]` | run declared evals; `--ci` for JSON |
| `lock` | refresh `promptlock.lock` |
| `check` | CI gate: working tree matches lockfile? |
| `drift` | what's out of sync (informational, never fails) |
| `rollback <id> <version>` | restore a prompt from the commit where it had that version |
| `comment --github` | upsert a PR comment from `eval` JSON |
| `version bump <id>` | bump major/minor/patch |
| `log <id>` | git log for one prompt |

Run `promptlock <command> -h` for flags.

## Out of scope

We don't do hosted SaaS, prompt observability/traces, prompt-authoring GUIs, or
LLM-driven prompt generation. Those are different products; compose with them
when you need them.

## License

MIT.
