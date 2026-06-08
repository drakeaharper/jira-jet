# Foundation: comments-and-votes-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:comments-and-votes-auto`. It turns a review's findings into
**inline Gerrit comments** and a **Code-Review (CR) vote** following the rubric,
and **posts + votes via the `gerry` CLI — that is its job**. It does not chain or
release.

> 1.5.0 model: this flow **posts and votes by default** (`post-and-vote`).
> `recommend-only` is now an opt-in dry run, not the default.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| findings | **instruction box** — paste the `## Review Summary` block from `review-auto`, or an equivalent findings list | yes | Drives the comment set and the CR rubric. |
| change number | **instruction box** — numeric Gerrit change #, else inferred from HEAD subject match | yes (arg preferred) | Needed for `gerry comments add` / `gerry vote`; a `Change-Id` is not accepted there. |
| `action_level` | **instruction box** (optional) | no — flow **default `post-and-vote`** | `recommend-only` \| `post-comments` \| `post-and-vote`. Lower it to opt down; **never escalate above** `post-and-vote`. |

### action_level

| Level | Behavior |
|-------|----------|
| `post-and-vote` (**default**) | Post the inline comments **and** cast the CR vote per the rubric. |
| `post-comments` | Post the inline comments only. **Do not vote.** |
| `recommend-only` | Opt-in dry run: produce comment set + recommended CR + rationale, **post nothing**, surface the exact `gerry` commands. |

If no level is provided in the instruction box, use **`post-and-vote`**.

## What to do

1. Take the findings and change number from the instruction box (resolve the
   change from HEAD's subject only if no numeric arg was given).
2. Invoke **`/canvas-lms-common:comments-and-votes-auto <CHANGE_NUMBER>
   --action-level <LEVEL>`** (omit the flag ⇒ default `post-and-vote`).
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
- A present-but-**empty** findings set (clean review) is **not** a stop → CR+2
  with zero inline comments.
- **Do not auto-escalate** `action_level` above the level given.
