# Foundation: review --auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/dragon-canvas:review --auto`. It produces a read-only, expert-augmented
review of a Gerrit change **and then posts the inline comments + casts the
Code-Review vote itself** (its Step 5 hands the Review Summary to
`comments-and-votes --auto`, default `post-and-vote`). It never edits code and
never merges/submits.

> 1.5.0 model: "reviewing isn't done until the feedback is on the change."
> review --auto owns the posting + vote. This is NOT a read-only-and-stop flow.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| change number | **instruction box** — numeric Gerrit change #, OR HEAD already at the change tip | yes (one of the two) | e.g. `407569`. |
| `--focus "<text>"` | **instruction box** (optional) | no | Emphasis directive (security/perf/a11y/…). |
| `ticket_context` (key + acceptance criteria) | **instruction box** — `--ticket KEY` + AC; a composite supplies these from `resolve-change-from-ticket` | no | Present → **ticket-rooted review**: verifies each acceptance criterion (drives `ac_status`/`ac_gaps`). Absent → code-merits only (`ac_status: n/a`). |
| `action_level` | **instruction box** (optional) | no — flow default **`post-and-vote`** | Lower it only to override: `post-comments` (comments, no vote) or `recommend-only` (dry run, posts nothing). |

## What to do

1. Resolve the change (instruction-box number, else HEAD at the change tip).
2. Invoke **`/dragon-canvas:review --auto`** — append `--focus "<text>"` and/or
   ticket context (`--ticket KEY` + AC) if supplied. It runs the full review +
   expert synthesis, then **Step 5 posts the inline comments and casts the CR
   vote** (default `post-and-vote`) via `comments-and-votes --auto`.
3. Do not edit code. The CR vote is pre-merge feedback — never merge/submit.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/review --auto` defines it,
so a composite can branch on `verdict`/`ac_status`:

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
suggestions:
  - kind: <open-question | nit>
    location: <file:line>
    problem: <one-line>
    fix: <one-line>
```

- `verdict: pass` only when zero Critical Issues **and** `ac_status` is `met` or
  `n/a`. A clean review (empty `critical[]`+`suggestions[]`) still posts a CR+2
  with zero inline comments.
- The posting result appears in the `comments-and-votes --auto`
  `## Comments & Vote` block (`recommended_cr`, `posted_comments`, `cast_vote`)
  that Step 5 emits.

## Hard stops — surface, never swallow

The flow halts when: no change to review (neither arg nor a Gerrit commit at
HEAD), `gerry fetch` fails, HEAD is not a Gerrit change, or a `gerry` post/vote
fails in Step 5.

- **Report the stop reason verbatim.** Do not invent a verdict or a posted state.
- **Do not auto-retry**; do not silently skip the posting.
