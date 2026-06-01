# Foundation: review-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:review-auto`. It produces a read-only, expert-augmented
review of a Gerrit change and a machine-readable verdict. It **never edits
code, never posts comments, never votes, never pushes, and never chains** into
comments-and-votes or any fix flow — those belong to a composite.

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| change number | **instruction box** — type the numeric Gerrit change # when launching, OR have HEAD already checked out at the change tip | yes (one of the two) | e.g. `407569`. A `Change-Id` is not enough; the numeric change resolves the rest. |
| `--focus "<text>"` | **instruction box** (optional) | no | Free-text emphasis directing review attention to specific areas. |
| `ticket_context` (key + acceptance criteria) | **instruction box** — pass `--ticket KEY` and/or the acceptance criteria; a composite supplies these from `resolve-change-from-ticket` | no | When present, this becomes a **ticket-rooted review**: the flow verifies the change satisfies each acceptance criterion (drives `ac_status`/`ac_gaps`). Absent → review on code merits only (`ac_status: n/a`). |

The ticket key jet auto-appends below may be the review's ticket context. If the
unit of work is a bare Gerrit change with no ticket, use the change number you
were given and ignore the appended ticket fields.

## What to do

1. Resolve the change: use the numeric change number from the instruction box,
   else the Gerrit commit already at HEAD.
2. Invoke **`/canvas-lms-common:review-auto`** — append `--focus "<text>"` if a
   focus was given, and pass the ticket context (`--ticket KEY` + acceptance
   criteria) if supplied — and let it run the full review + expert synthesis.
   With ticket context present, it also walks each acceptance criterion and
   classifies `ac_status`.
3. Read-only: do not edit code, push, post comments, or vote.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/review-auto` defines it,
so a composite can route on `verdict`/`ac_status` and feed `critical[]`+`ac_gaps[]`
onward:

```
## Review Summary (machine-readable)
verdict: <pass | changes-requested>   # changes-requested if any Critical Issues OR ac_status != met
tickets: [LX-123, ...]
ac_status: <met | partial | unmet | n/a>   # n/a when no ticket context was provided
ac_gaps: [ "<unsatisfied criterion>", ... ]  # empty unless partial/unmet
critical_count: <n>
suggestion_count: <n>
critical:
  - file:line — <one-line problem> — <one-line fix>
  ...
```

- `verdict: pass` only when there are zero Critical Issues **and** `ac_status` is
  `met` or `n/a`. Any unsatisfied acceptance criterion is a Critical Issue and
  forces `verdict: changes-requested`.

## Hard stops — surface, never swallow

The flow halts (no output block) when: no change to review (neither arg nor a
Gerrit commit at HEAD), `gerry fetch` fails, or HEAD is not a Gerrit change and
no number was given.

- **Report the stop reason verbatim.** Do not invent a verdict.
- **Do not auto-retry.** Do not chain into comments-and-votes on a stop.
