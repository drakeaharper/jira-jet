# Foundation: start-ticket --auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/dragon-canvas:start-ticket --auto`. It produces a change for a Jira ticket,
**commits once, and pushes it to Gerrit itself** (`status: pushed`). It does
**not** run tests-setup, QA, review, or chain — those are separate steps a
composite sequences.

> 1.5.0 model: the inner flow owns its own push. This base no longer treats push
> as a separate gate.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| ticket key | **auto-appended by jet** (the selected ticket's context is added below this prompt) | yes | e.g. `LX-1234`. Identify it from the appended ticket context first. |

`/start-ticket --auto` derives scope from the ticket and asks nothing.

## What to do

1. Read the appended ticket context and identify the ticket key.
2. Invoke **`/dragon-canvas:start-ticket --auto <TICKET_KEY>`** and let it drive
   the whole job: deep analysis, issue-type branch, failing test for bugs,
   implementation, pre-commit verification, single commit, **and the
   `git push origin HEAD:refs/for/master` that creates/updates the Gerrit
   change** (pre-merge, abandonable — never merge/submit).
3. Do not run `/setup-test`, `/qa`, or `/review` here.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/start-ticket --auto`
defines it, so a composite can gate on `status` / `commit_sha` / `gerrit_change`:

```
## Ticket Result (machine-readable)
status: pushed | stopped
ticket: <KEY>
issue_type: bug | story | support
branch: <branch>
commit_sha: <sha | null>
gerrit_change: <url or change # | null>
flag: <feature flag | none>
files: [ <path>, ... ]
tests:
  result: pass | fail | not-run
  command: <what was run>
assumptions:
  - "<decision made in place of a gate> — <evidence file:line>"
stop_reason: <present only when status: stopped>
```

- `status: pushed` requires a single commit (`commit_sha` set), `tests.result:
  pass`, and a successful push (`gerrit_change` set).

## Hard stops — surface, never swallow

If the flow hits a hard stop (ticket key undeterminable, Jira fetch fails,
genuinely ambiguous requirements, code not locatable, missing external facts,
tests cannot run, **or the push fails**), it emits `status: stopped`, with
`commit_sha`/`gerrit_change` as far as it got, and a one-line `stop_reason`.

- **Return the `stop_reason` + the partial `assumptions` log.** Do not paper over it.
- **Do not auto-retry** the same inputs.
- A `status: stopped` means nothing was pushed (or push failed) — a composite
  should **leave the env claimed** for recovery, not release.
