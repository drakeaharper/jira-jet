# Composite: canvas-ticket-pipeline (TICKET LANE)

End-to-end **orchestration** of the ticket lane from ORCHESTRATION.md:

```
claim env → start-ticket-auto → setup-test-auto → qa-auto → push → release
                  ▲                                   │
                  └──────── fix loop ◄────────────────┘
```

This composite **is the orchestrator**. There is no jet step/branch engine — you
(the single Claude agent running this workflow) execute the steps in sequence and
**branch only on each flow's machine-block gate fields**. This composite **owns
the env lifecycle (claim / push / release)** and runs the inner `-auto` flows in
their **push-free** form, performing the single push itself once QA is green.

> Starting template (composite). Refine for your repo, save under a new name,
> then launch. Base templates are read-only and never run directly.

## Non-negotiable rules

- **Branch only on gate fields** (`status`, `commit_sha`, `verdict`,
  `findings[].likely_owner`). Never parse prose to decide routing.
- **Release invariant — release only ever AFTER a push.** A pushed commit is
  resumable; an unpushed one is not. So: release **only** after `git push`
  succeeds. **If anything halts before the push, LEAVE THE ENV CLAIMED** (do not
  release — it would orphan local work).
- **Push creates a Gerrit change** (`HEAD:refs/for/master`, pre-merge,
  abandonable). **Never merge/submit** — that's a human action.
- **No auto-retry of a hard stop.** On `status: stopped` / any hard stop:
  surface the `stop_reason` + partial results and stop the run.
- **Fix loop is capped** at `MAX_FIX_ITERATIONS`. On exhaustion → halt + report,
  leave env claimed.

## Parameters

| Parameter | Source | Default | Notes |
|-----------|--------|---------|-------|
| ticket key | **auto-appended by jet** (selected ticket context below) | — | e.g. `LX-1234`. Identify it first. |
| `MAX_FIX_ITERATIONS` | **instruction box** (optional) | `3` | Max QA→fix→QA loop passes before halting. |
| `--base-ref` | **instruction box** (optional) | `origin/master` | Branch base for the claim. |
| `--reset-db` | **instruction box** (optional) | off | Reset env DB before checkout. |

## Diagram

```
ticket key (auto-appended)
        │
        ▼
[claim env]  cpe doctor → cpe free → cpe claim <ticket> [--base-ref] [--reset-db] → cd code_path
        │                                                              (env CLAIMED)
        ▼
[start-ticket-auto]  (push-free, single commit)
        │ status?
        ├─ stopped ─────────────────► HALT: surface stop_reason + assumptions; LEAVE ENV CLAIMED
        └─ committed  (commit_sha set, tests pass)
                │
                ▼
        [setup-test-auto] ──► ## Test Plan (env_url, course_url, logins{teacher,student}, steps[], expected[])
                │
                ▼
        [qa-auto]  (consume the WHOLE Test Plan block, incl. logins+passwords)
                │ verdict?
                ├─ pass ───────────► [git push HEAD:refs/for/master] ──► [cpe release]  ✅ DONE
                │                         │ push fails? ──► HALT; do NOT release; LEAVE CLAIMED
                └─ fail → for each finding, route on findings[].likely_owner:
                        ├─ code-bug    → [start-ticket-auto] (re-fix; pass findings[] as the defect)  ┐
                        ├─ data-setup  → ─────────────────────────────────────────────────────────── ┤ fix loop
                        │                                                                              ▼
                        │                                         back to [setup-test-auto] → [qa-auto]
                        │                                         (increment counter; cap MAX_FIX_ITERATIONS)
                        ├─ flag-off    → HALT + report (human decides flag enablement); LEAVE CLAIMED
                        └─ unknown     → HALT + report (human triage); LEAVE CLAIMED

  loop exhausted (counter == MAX_FIX_ITERATIONS) ──► HALT + report; LEAVE ENV CLAIMED
```

## Steps

### 0. Identify the ticket
Read the appended ticket context; extract the ticket key (e.g. `LX-1234`).

### 1. Claim the env (composite owns claim)
Preflight then claim — env only, **no inner flow, no push** (the standalone
`canvas-parallel-env-auto` claim mode bundles inner-flow+push+release; this
orchestrator unbundles it and owns push/release itself):
```bash
command -v cpe                                   # missing → HALT (install cpe)
cpe doctor --json | jq -r '.checks|to_entries|map(select(.key!="flock" and .value!="ok"))|length'   # !=0 → HALT
cpe free --json | jq '.envs | length'            # 0 → HALT (do NOT cold-build)
CLAIM=$(cpe claim <TICKET> --base-ref <BASE_REF> --json)   # add --reset-db before --json if requested
ENV_NAME=$(jq -r .name <<< "$CLAIM"); CODE_PATH=$(jq -r .code_path <<< "$CLAIM"); URL=$(jq -r .url <<< "$CLAIM")
cd "$CODE_PATH"
```
Surface `URL` (where the user verifies the running Canvas). The env is now claimed.

### 2. start-ticket-auto (push-free)
Invoke **`/canvas-lms-common:start-ticket-auto <TICKET>`** (tell it to skip its
branch-setup step — `cpe claim` already created/checked out the branch). Read its
`## Ticket Result` block.
- `status: stopped` → **HALT**: surface `stop_reason` + `assumptions`; leave env claimed; stop.
- `status: committed` (with `commit_sha`) → continue. (No push.)

### 3. setup-test-auto
Invoke **`/canvas-lms-common:setup-test-auto <TICKET>`**. Capture the entire
`## Test Plan` block (including `logins` with passwords).
- Hard stop (no testable scenario / script fails) → HALT + report, leave claimed.

### 4. qa-auto
Invoke **`/canvas-lms-common:qa-auto`**, passing the whole Test Plan block. Read
its `## QA Result` block.
- `verdict: pass` → go to Step 5 (push + release).
- `verdict: fail` → go to Step 6 (route findings), unless the loop cap is hit.
- Hard stop (env unreachable, login fails, no browser) → HALT + report, leave claimed.

### 5. Push then release (only on `verdict: pass`)
```bash
git -C "$CODE_PATH" log --oneline origin/master..HEAD   # expect exactly 1 commit; >1 → squash; 0 → HALT
git -C "$CODE_PATH" push origin HEAD:refs/for/master     # creates/updates the Gerrit change
```
- Push **fails** → surface the error, **do NOT release**, leave env claimed, HALT.
- Push **succeeds** → capture the Gerrit change URL, then release:
```bash
cpe release <ENV_NAME>
```
Report change URL + that the env was released. **DONE.** (Never merge/submit.)

### 6. Fix loop (only on `verdict: fail`, counter < MAX_FIX_ITERATIONS)
Classify each `findings[].likely_owner` and route:
- **`code-bug`** → invoke **`/canvas-lms-common:start-ticket-auto <TICKET>`**
  again, passing the QA `findings[]` as the defect to fix (it amends the single
  commit). Then **re-run Step 3 (setup-test-auto) → Step 4 (qa-auto)**.
- **`data-setup`** → skip the code fix; **re-run Step 3 → Step 4** (regenerate the
  scenario / re-provision).
- **`flag-off`** → **HALT + report** (a human decides flag enablement); leave claimed.
- **`unknown`** → **HALT + report** (human triage); leave claimed.

Increment the fix counter each loop pass. When `counter == MAX_FIX_ITERATIONS`
without a `verdict: pass` → **HALT + report**, leave env claimed. Never loop forever.

## Final report
Always end with: which step ended the run, the relevant gate fields, the Gerrit
change URL (if pushed), whether the env was released or left claimed, and any
`stop_reason` + partial output (`assumptions`, `findings[]`).
