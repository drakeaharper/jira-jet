# Composite: canvas-review-pipeline (REVIEW LANE)

End-to-end **orchestration** of the review lane from ORCHESTRATION.md (1.5.0):

```
[resolve-from-ticket]? → cpe-auto(review) → review-auto → release
                                            (review + POST comments + VOTE)
```

**1.5.0 key point:** `review-auto` **posts the inline comments and casts the CR
vote itself** (its Step 5 → `comments-and-votes-auto`, default `post-and-vote`).
This composite does **not** orchestrate posting separately and must **not**
suppress it — it only owns **claim, release, and (optionally) resolving a ticket
to its change**. The posting is the flow's job; let it run.

## Non-negotiable rules

- **Unattended run — NEVER prompt.** Runs headless via jet; no human to answer.
  Do not ask for `action_level` or anything else, and do not treat "no answer"
  as a dry-run signal. This is a live autonomous run, not template authoring.
- **Run regardless of starting directory.** You may be launched from any cwd
  (even the jet tool's own source repo). Step 1 claims a **fresh Canvas env** and
  `cd`s into it — the starting directory is irrelevant and is **never** a reason
  to pause, second-guess "posture," or skip execution.
- **Let the flow post + vote.** `review-auto` posts comments and casts the CR
  vote by default (`post-and-vote`). Do **not** pass `recommend-only` or
  otherwise hold back the posting unless the instruction box explicitly says so.
- **Branch only on gate fields** (`verdict`, `ac_status`). Never parse prose.
- **Always release at the end** (success or hard stop). Review lane never pushes;
  the change already lives on Gerrit. **Never merge/submit.**
- **No auto-retry of a hard stop.** Surface the stop reason, then still release.

## Parameters

| Parameter | Source | Default | Notes |
|-----------|--------|---------|-------|
| change number **or** ticket key | **instruction box** / auto-appended ticket | — | A numeric change # goes straight to review. A ticket key triggers resolve-from-ticket first. |
| `--focus "<text>"` | **instruction box** (optional) | empty | Review emphasis; passed through to `review-auto`. |
| `action_level` | **instruction box** (optional) | **`post-and-vote`** | Only to opt *down* (`post-comments` = comments no vote; `recommend-only` = dry run). Never escalate above post-and-vote. |

## Diagram

```
ticket key (auto-appended)  OR  change # (instruction box)   [+ --focus]
        │
        ▼
[0. resolve-from-ticket]  (only if entered from a ticket, no change # given)
        │  jet view <TICKET> --format json | jq … newest gerritbot /+/<n>
        │  capture summary + acceptance criteria → ticket_context
        ├─ no gerritbot comment ──► ✋ HALT (nothing to review)
        ▼ change_number (+ ticket_context)
[1. claim env, review mode]  cpe claim review-<change> --no-checkout → cd code_path → gerry fetch <change>
        │ gerry fetch ok?
        ├─ fail ──► [cpe release] immediately; ✋ HALT + report
        ▼ ok  (HEAD detached at change tip)
[2. review-auto --focus "<focus>" [--ticket KEY + AC]]
        │  reviews, then Step 5 POSTS inline comments + CASTS CR vote
        │  (comments-and-votes-auto, default post-and-vote;
        │   rubric: blocker/ac!=met → CR-1 ; open-question → CR+1 ; nits/clean → CR+2)
        │ verdict (informational — posting already happened):
        ├─ changes-requested / ac!=met → comments posted, CR-1/CR+1 cast
        └─ pass                        → CR+2 cast, zero/﻿nit comments
        ▼
[3. cpe release]   ← ALWAYS (success OR hard stop)            ✅ DONE
```

## Steps

### 0. Resolve change from ticket (only when entered from a ticket)
If the instruction box gave a numeric change #, skip to Step 1. Otherwise resolve
the appended ticket key to its change via the newest gerritbot comment:
```bash
jet view <TICKET> --format json | jq -r '
  [ .fields.comment.comments[] | select(.author.emailAddress=="gerritbot@canvas.net") ]
  | sort_by(.created) | reverse | .[0].body | capture("/\\+/(?<n>[0-9]+)").n'
```
Also capture the ticket `summary` + acceptance criteria (→ `ticket_context` for a
ticket-rooted review). Zero gerritbot comments → **HALT** (nothing to review).
Multiple distinct changes → default newest; honor an explicit change-# override.

### 1. Claim the env (review mode)
```bash
command -v cpe; cpe doctor --json | jq -r '.checks|to_entries|map(select(.key!="flock" and .value!="ok"))|length'   # !=0 → HALT
cpe free --json | jq '.envs | length'            # 0 → HALT (do NOT cold-build)
CLAIM=$(cpe claim review-<CHANGE> --no-checkout --json)
ENV_NAME=$(jq -r .name <<< "$CLAIM"); CODE_PATH=$(jq -r .code_path <<< "$CLAIM")
cd "$CODE_PATH"
gerry fetch <CHANGE>      # HEAD → change tip (detached)
```
`gerry fetch` fails → `cpe release <ENV_NAME>` immediately, then HALT.

### 2. review-auto (reviews AND posts AND votes)
Invoke **`/canvas-lms-common:review-auto`** — append `--focus "<text>"` and the
ticket context (`--ticket KEY` + AC) from Step 0 if present. It reviews, then its
**Step 5 posts the inline comments and casts the CR vote** at the effective
`action_level` (default **`post-and-vote`**). Read the `## Review Summary` and the
`## Comments & Vote` blocks it emits.
- Hard stop (no change / `gerry fetch` failed / HEAD not a Gerrit change / `gerry`
  post fails) → surface stop reason, go to Step 3 (release), HALT. Do not retry.

### 3. Release (always)
```bash
cpe release <ENV_NAME>   # idempotent
```
Always run — success or hard stop. Then report.

## Final report
End with: `verdict`, `ac_status`, `recommended_cr`, **`posted_comments` /
`cast_vote`** (proof the feedback landed), the `action_level` used, the resolved
change # (and ticket if rooted), confirmation the env was released, and any
`stop_reason`.
