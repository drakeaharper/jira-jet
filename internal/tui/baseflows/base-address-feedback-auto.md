# Foundation: address-feedback-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:address-feedback-auto`. It pulls Gerrit review comments,
applies the valid ones, amends the single commit, **and pushes the updated
patchset to Gerrit itself** (`status: pushed`). It does not re-review or chain.

> 1.5.0 model: the inner flow owns its own push. A composite re-runs `review-auto`
> after this returns `status: pushed`.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| change number | **instruction box** — numeric Gerrit change #, else inferred from HEAD's commit subject | yes (arg preferred) | e.g. `407569`. Subject match yielding zero or >1 open change → hard stop. |

The ticket context jet auto-appends is **incidental** — the unit of work is a
Gerrit change.

## What to do

1. Resolve the change number (instruction-box arg first; else HEAD subject match).
2. Invoke **`/canvas-lms-common:address-feedback-auto <CHANGE_NUMBER>`**. It
   classifies each reviewer comment `valid` / `invalid` / `needs-direction`,
   applies the valid ones, amends the single commit, re-verifies tests, **and
   pushes the updated patchset** (`git push origin HEAD:refs/for/master` — updates
   the existing change; pre-merge, never merge/submit).

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/address-feedback-auto`
defines it, so a composite can gate on `status` / `amended_sha`:

```
## Feedback Result (machine-readable)
status: pushed | stopped
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

- `status: pushed` requires the commit was amended (`amended_sha` set),
  `tests.result: pass`, and a successful push.
- Non-empty `needs_direction[]` ⇒ always `status: stopped`.

## Hard stops — surface, never swallow

`status: stopped` (with `amended_sha` as far as it got + one-line `stop_reason`)
when: the change can't be resolved (zero / >1 subject match), feedback conflicts,
feedback is genuinely ambiguous (→ `needs_direction[]`), tests fail, **or the
push fails**.

- **Return the `stop_reason` + the `comments` classification gathered so far**
  (especially `needs_direction[]`).
- **Do not auto-retry.**
