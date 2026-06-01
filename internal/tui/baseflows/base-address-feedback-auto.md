# Foundation: address-feedback-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:address-feedback-auto`. It pulls Gerrit review comments,
applies the valid ones, and **amends the commit** — it does **not** push or
re-review. Push + re-run review belong to a composite (per the orchestration
contract, `amended_sha` present ⇒ push then re-review).

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| change number | **instruction box** — type the numeric Gerrit change #, else it is inferred from HEAD's commit subject | yes (arg preferred) | e.g. `407569`. If the subject match yields zero or more than one open change, the flow hard-stops. |

The ticket context jet auto-appends is **incidental** — the unit of work is a
Gerrit change. Use the change number you were given.

## What to do

1. Resolve the change number (instruction-box arg first; else HEAD subject match).
2. Invoke **`/canvas-lms-common:address-feedback-auto <CHANGE_NUMBER>`**. It
   classifies each reviewer comment `valid` / `invalid` / `needs-direction`,
   applies the valid ones, amends the single commit, and re-verifies tests.
3. Do **not** `git push`. Push is the explicit gate a composite owns.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/address-feedback-auto`
defines it, so a composite can gate on `status` and `amended_sha`:

```
## Feedback Result (machine-readable)
status: amended | stopped
change: <number>
amended_sha: <sha | null>
comments:
  applied:    [ "<file:line> — <what changed>", ... ]
  skipped:    [ "<file:line> — <summary> — <why invalid>", ... ]
  needs_direction: [ "<file:line> — <both sides>", ... ]
files: [ <path>, ... ]
tests:
  result: pass | fail | not-run
  command: <what was run>
stop_reason: <present only when status: stopped>
```

- `status: amended` requires the commit was amended (`amended_sha` set) and
  `tests.result: pass`.
- Non-empty `needs_direction[]` ⇒ always `status: stopped`.

## Hard stops — surface, never swallow

The flow stops (`status: stopped`, `amended_sha: null`, one-line `stop_reason`)
when: the change can't be resolved (zero / >1 subject match), reviewer feedback
conflicts, feedback is genuinely ambiguous (→ `needs_direction[]`), tests fail
after changes, or a `gerry` command fails.

- **Return the `stop_reason` plus the `comments` classification gathered so far**
  (especially `needs_direction[]`). Do not swallow it.
- **Do not auto-retry.** Do not push on a stop.
