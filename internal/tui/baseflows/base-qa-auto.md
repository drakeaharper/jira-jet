# Foundation: qa-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:qa-auto`. It drives the **live Canvas env in a real browser**
(via the `agent-browser` skill) through a ticket's manual test steps, asserts
the expected behavior, and emits a pass/fail verdict. It is read-only on the
codebase — it reports findings, it does **not** fix, push, or chain.

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| Test Plan block | **instruction box** — paste the `## Test Plan (machine-readable)` block from `setup-test-auto` | preferred | Provides `env_url`, `course_url`, `feature_flag`, `logins.{teacher,student}.{unique_id,password}`, `steps[]`, `expected[]`. |
| ticket key | **auto-appended by jet** / current branch | fallback | If no Test Plan is supplied, re-derive by running `/setup-test-auto` for the ticket first. |

## What to do

1. Obtain the Test Plan (from the instruction box; else run `/setup-test-auto`
   for the ticket to produce one).
2. Invoke **`/canvas-lms-common:qa-auto`** with that Test Plan. It uses the
   `agent-browser` skill to navigate, log in, perform each step, screenshot, and
   assert each `expected[]` behavior against observed state. Do not simulate.
3. Report findings only. Do not edit code, push, or launch a fix flow.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/qa-auto` defines it, so a
composite can route on `verdict` and classify each finding by `likely_owner`:

```
## QA Result (machine-readable)
ticket: <TICKET>
env_url: <url>
verdict: <pass | fail>          # fail if any expectation not-met or feature error
steps_run: <n>
screenshots: [<path>, ...]
findings:                        # empty on pass
  - step: <which step / expectation>
    expected: <what should have happened>
    observed: <what actually happened>
    evidence: <screenshot path / console error>
    likely_owner: <code-bug | data-setup | flag-off | unknown>
```

- `verdict: pass` only when every `expected[]` is met and no feature-related
  errors occurred.
- `likely_owner` is the routing key a composite uses: `code-bug` → fix flow,
  `data-setup` → re-run setup-test, `flag-off`/`unknown` → stop / human triage.

## Hard stops — surface, never swallow

The flow halts (no QA Result) when: no Test Plan is available, the
`agent-browser` skill is unavailable, the env URL doesn't load, or login fails.

- **Report the stop reason verbatim** with any screenshots already captured.
- **Do not auto-retry.** Do not chain into a fix flow on a stop.
