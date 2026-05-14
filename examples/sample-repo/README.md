# promptlock-example

A small, realistic shape of a `promptlock`-managed prompts directory. Five
prompts, one dataset per prompt, a committed lockfile, and the GitHub Actions
wiring.

`promptlock validate` and `promptlock eval --provider mock` work end-to-end
with no API keys.

## Structure

```
sample-repo/
├── .github/workflows/promptlock.yml
├── prompts/
│   ├── support/triage.prompt.md
│   ├── support/escalation.prompt.md
│   ├── onboarding/welcome.prompt.md
│   ├── marketing/subject-line.prompt.md
│   └── content/summarize.prompt.md
├── tests/datasets/
└── promptlock.lock
```

Coverage of the metric surface: `exact_match`, `contains`, `regex`,
`json_schema`, `llm_judge`.

## Try it

```bash
promptlock validate
promptlock eval --prompt onboarding/welcome --provider mock
promptlock check
```

Then edit a prompt and:

```bash
promptlock diff onboarding/welcome
promptlock eval --prompt onboarding/welcome --provider mock
promptlock lock
promptlock check
```
