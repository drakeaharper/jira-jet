# Foundation: comments-and-votes-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:comments-and-votes-auto`. It turns a review's findings into
**inline Gerrit comment recommendations** and a **Code-Review (CR) vote
recommendation** following the rubric — and posts/votes **only at the level you
explicitly authorize**. It does not chain or release.

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| findings | **instruction box** — paste the `## Review Summary` block from `review-auto`, or an equivalent findings list | yes | Drives the comment set and the CR rubric. |
| change number | **instruction box** — numeric Gerrit change #, else inferred from HEAD subject match | yes (arg preferred) | Needed for `gerry comments add` / `gerry vote`; a `Change-Id` is not accepted there. |
| `action_level` | **instruction box** (optional) | no — **default `recommend-only`** | `recommend-only` \| `post-comments` \| `post-and-vote`. **Never escalate beyond what was requested.** |

### action_level (do not auto-escalate)

| Level | Behavior |
|-------|----------|
| `recommend-only` (**default**) | Produce comment set + recommended CR + rationale. **Post nothing.** Surface the exact `gerry` commands for the user. |
| `post-comments` | Post the inline comments to Gerrit. **Do not vote.** |
| `post-and-vote` | Post the inline comments **and** cast the CR vote per the rubric. |

If no level is provided in the instruction box, use **`recommend-only`**.

## What to do

1. Take the findings and change number from the instruction box (resolve the
   change from HEAD's subject only if no numeric arg was given).
2. Invoke **`/canvas-lms-common:comments-and-votes-auto <CHANGE_NUMBER>
   --action-level <LEVEL>`** (omit the flag to default to `recommend-only`).
3. The CR score is always rubric-driven (blocker → CR-1, open question → CR+1,
   only nits → CR+2). Never cast a score the rubric doesn't support, and never
   set labels other than Code-Review.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as
`/comments-and-votes-auto` defines it:

```
## Comments & Vote (machine-readable)
change: <number>
action_level: <recommend-only | post-comments | post-and-vote>
recommended_cr: <-1 | +1 | +2>
rationale: <one paragraph naming the determining findings>
comments_count: <n>
posted_comments: <true|false>
cast_vote: <true|false>
```

## Hard stops — surface, never swallow

The flow halts when: no findings input, the change number can't be resolved
(zero / >1 subject match), or a `gerry` command fails when posting/voting.

- **Report the stop reason verbatim.** Do not retry a failed `gerry` post blindly.
- **Do not auto-escalate** `action_level` to recover from a stop.
