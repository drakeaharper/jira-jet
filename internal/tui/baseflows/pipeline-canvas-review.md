# Composite: canvas-review-pipeline (REVIEW LANE)

End-to-end **orchestration** of the review lane from ORCHESTRATION.md:

```
claim env (review mode) → review-auto → comments-and-votes-auto → release
```

This composite **is the orchestrator**. There is no jet step/branch engine — you
(the single Claude agent running this workflow) execute the steps in sequence and
**branch only on each flow's machine-block gate fields**. The review lane is
**read-only on the repo**: it performs **no push** and the detached HEAD is
disposable, so it **always releases** the env at the end.

> Starting template (composite). Refine for your repo, save under a new name,
> then launch. Base templates are read-only and never run directly.

## Non-negotiable rules

- **Branch only on gate fields** (`verdict`, `critical[]`, `recommended_cr`).
  Never parse prose to decide routing.
- **Always release at the end** (success or hard stop). Review lane never pushes;
  the change already lives on Gerrit, so releasing can't orphan anything. **Never
  merge/submit.**
- **`action_level` is a parameter and is never auto-escalated.** Default
  `recommend-only` (posts nothing — this is the dry/safe run). Honor exactly the
  level given.
- **No auto-retry of a hard stop.** Surface the stop reason + partial results,
  then still release.

## Parameters

| Parameter | Source | Default | Notes |
|-----------|--------|---------|-------|
| change number | **instruction box** | — | Numeric Gerrit change #, e.g. `407569`. Required. |
| `action_level` | **instruction box** (optional) | **`recommend-only`** | `recommend-only` \| `post-comments` \| `post-and-vote`. Never escalate beyond this. |
| `--focus "<text>"` | **instruction box** (optional) | empty | Review emphasis; passed through to `review-auto`. |

The ticket context jet auto-appends is **incidental** here — the unit of work is
a Gerrit change. Use the change number from the instruction box.

## Diagram

```
change number (instruction box)  [+ --focus]  [+ action_level, default recommend-only]
        │
        ▼
[claim env, review mode]  cpe claim review-<change> --no-checkout → cd code_path → gerry fetch <change>
        │ gerry fetch ok?
        ├─ fail ──► [cpe release] immediately; HALT + report
        └─ ok  (HEAD detached at change tip)
                │
                ▼
        [review-auto --focus "<focus>"]   (read-only; never edits/pushes)
                │ verdict?
                ├─ changes-requested → findings = critical[] (+ suggestions)
                └─ pass              → findings = empty / nits only
                          │
                          ▼
        [comments-and-votes-auto <change> --action-level <LEVEL>]
                  CR rubric: any blocker → CR-1 ; open questions → CR+1 ; nits/empty → CR+2
                  posts comments / casts vote ONLY at <LEVEL>
                  (recommend-only ⇒ posts nothing, just surfaces the gerry commands)
                          │
                          ▼
                  [cpe release]   ← ALWAYS (success OR hard stop)        ✅ DONE
```

## Steps

### 1. Claim the env (review mode) — composite owns claim/release
```bash
command -v cpe                                   # missing → HALT (install cpe)
cpe doctor --json | jq -r '.checks|to_entries|map(select(.key!="flock" and .value!="ok"))|length'   # !=0 → HALT
cpe free --json | jq '.envs | length'            # 0 → HALT (do NOT cold-build)
CLAIM=$(cpe claim review-<CHANGE> --no-checkout --json)
ENV_NAME=$(jq -r .name <<< "$CLAIM"); CODE_PATH=$(jq -r .code_path <<< "$CLAIM"); URL=$(jq -r .url <<< "$CLAIM")
cd "$CODE_PATH"
gerry fetch <CHANGE>      # HEAD becomes the change tip (detached)
```
If **`gerry fetch` fails** (auth, missing change): `cpe release <ENV_NAME>`
**immediately**, then HALT + report. Never leave the env half-claimed.

### 2. review-auto (read-only)
Invoke **`/canvas-lms-common:review-auto`** (append `--focus "<text>"` if given).
Read its `## Review Summary` block.
- `verdict: changes-requested` → findings = `critical[]` + suggestions (→ Step 3).
- `verdict: pass` → findings = empty / nits only (→ Step 3; rubric yields CR+2).
- Hard stop (no change / `gerry fetch` failed / HEAD not a Gerrit change) →
  surface stop reason, go to Step 4 (release), HALT.

### 3. comments-and-votes-auto (at the authorized level only)
Invoke **`/canvas-lms-common:comments-and-votes-auto <CHANGE> --action-level
<LEVEL>`** (omit the flag ⇒ `recommend-only`). Pass the review findings
(`critical[]` + suggestions) as input. The CR score is rubric-driven
(blocker→CR-1, open-question→CR+1, nits/empty→CR+2); only Code-Review is set.
Read its `## Comments & Vote` block (`recommended_cr`, `posted_comments`,
`cast_vote`).
- Hard stop (no findings / change unresolvable / `gerry` post fails) → surface
  the stop reason; **do not retry the gerry post**; go to Step 4 (release), HALT.
- **Never escalate `action_level`** to recover from a stop.

### 4. Release (always)
```bash
cpe release <ENV_NAME>   # idempotent
```
Always run this — on success or after any hard stop in Steps 2–3 (review lane has
nothing unpushed to protect). Then report.

## Final report
End with: `verdict`, `recommended_cr`, the `action_level` used, whether comments
were posted / a vote cast (`posted_comments` / `cast_vote`), the exact `gerry`
commands if `recommend-only`, confirmation the env was released, and any
`stop_reason`.
