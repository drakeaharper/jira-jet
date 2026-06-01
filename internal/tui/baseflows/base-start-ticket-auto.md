# Foundation: start-ticket-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:start-ticket-auto`. It produces and verifies a change for a
Jira ticket and **commits once** — it does **not** push, review, set up tests,
or chain into any other flow. Push, QA, and routing belong to a composite
workflow built on top of this one.

> This is a starting template. Refine it for your repo, then save under a new
> name. The base templates themselves are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| ticket key | **auto-appended by jet** (the selected ticket's context is added below this prompt) | yes | e.g. `LX-1234`. Identify it from the appended ticket context before doing anything. |

No other inputs. `/start-ticket-auto` derives scope from the ticket
(description, comments, acceptance criteria, attachments, labels, issue type)
and asks nothing.

## What to do

1. Read the appended ticket context and identify the ticket key.
2. Invoke **`/canvas-lms-common:start-ticket-auto <TICKET_KEY>`** and let it drive
   the entire workflow (deep analysis, issue-type branch, failing test for bugs,
   implementation, pre-commit verification, single commit).
3. Do **not** run `/review`, `/setup-test`, or `git push`. Those are separate
   steps a composite sequences. This workflow ends when the flow returns.

## Output (the workflow's result — emit verbatim)

End your final message with the flow's machine-readable block exactly as
`/start-ticket-auto` defines it, so a composite workflow can gate on `status`
and `commit_sha`:

```
## Ticket Result (machine-readable)
status: committed | stopped
ticket: <KEY>
issue_type: bug | story | support
branch: <branch>
commit_sha: <sha | null>
flag: <feature flag | none>
files: [ <path>, ... ]
tests:
  result: pass | fail | not-run
  command: <what was run>
assumptions:
  - "<decision made in place of a gate> — <evidence file:line>"
stop_reason: <present only when status: stopped>
```

- `status: committed` requires a single commit (`commit_sha` set) and
  `tests.result: pass`.

## Hard stops — surface, never swallow

If the flow hits a hard stop (ticket key undeterminable, Jira fetch fails,
genuinely ambiguous requirements that change the approach, code not locatable,
external facts missing, tests cannot run), it emits `status: stopped`,
`commit_sha: null`, and a one-line `stop_reason`.

- **Return the `stop_reason` plus the partial `assumptions` log.** Do not paper
  over it.
- **Do not auto-retry** the same inputs.
- Do not push or chain on a stop.
