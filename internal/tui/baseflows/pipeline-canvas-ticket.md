# Composite: canvas-ticket-pipeline (TICKET LANE)

End-to-end **orchestration** of the ticket lane from ORCHESTRATION.md (1.5.0):

```
claim env → start-ticket --auto → setup-test --auto → qa --auto → release
            (commit + PUSH)                          │
                  ▲                                   │
                  └─ fix loop: address-feedback --auto ◄┘
                     (amend + PUSH new patchset)
```

**1.5.0 key point:** the inner flows own their own push — `start-ticket --auto`
commits **and pushes** a patchset, `address-feedback --auto` amends **and pushes**.
There is **no separate push-gate node**. This composite owns **only claim,
release, and sequencing** (setup-test → qa → fix loop). QA runs against the
already-pushed change; release happens after `qa --auto: pass`.

## Non-negotiable rules

- **Unattended run — NEVER prompt.** Headless via jet; no human to answer. This
  is a live autonomous run, not a template-authoring exercise — execute it.
- **Run regardless of starting directory.** You may be launched from any cwd
  (even the jet tool's own source repo). Step 1 claims a **fresh Canvas env** and
  `cd`s into it — the starting directory is irrelevant and is **never** a reason
  to pause, second-guess "posture," or skip execution.
- **Branch only on gate fields** (`status`, `commit_sha`, `verdict`,
  `findings[].likely_owner`). Never parse prose.
- **The flows push; the composite does not.** Do not add a `git push` step.
- **Release invariant:** release only after the change is safely pushed.
  `start-ticket --auto: pushed` then `qa --auto: pass` → release. **If
  `start-ticket --auto` hard-stops before pushing (`status: stopped`), LEAVE THE
  ENV CLAIMED** (nothing pushed → releasing orphans work). Never merge/submit.
- **No auto-retry of a hard stop.** Fix loop is capped at `MAX_FIX_ITERATIONS`.

## Parameters

| Parameter | Source | Default | Notes |
|-----------|--------|---------|-------|
| ticket key | **auto-appended by jet** | — | e.g. `LX-1234`. Identify it first. |
| `MAX_FIX_ITERATIONS` | **instruction box** (optional) | `3` | Max QA→fix→QA passes before halting. |
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
[start-ticket --auto]  commits + PUSHES the change itself
        │ status?
        ├─ stopped ─────────────────► ✋ HALT: surface stop_reason + assumptions; LEAVE ENV CLAIMED
        └─ pushed  (commit_sha + gerrit_change set, tests pass, on Gerrit)
                │
                ▼
        [setup-test --auto] ──► ## Test Plan (env_url, course_url, logins{teacher,student}, steps[], expected[])
                │
                ▼
        [qa --auto]  (consume the WHOLE Test Plan block, incl. logins+passwords)
                │ verdict?
                ├─ pass ───────────► [cpe release]   ✅ DONE  (change already pushed by start-ticket --auto)
                └─ fail → for each finding, route on findings[].likely_owner:
                        ├─ code-bug    → [address-feedback --auto <change>] (amend + PUSH new patchset) ┐
                        │                 (or start-ticket --auto re-fix; the change is already pushed)  │
                        ├─ data-setup  → ─────────────────────────────────────────────────────────── ┤ fix loop
                        │                                                                              ▼
                        │                                         back to [setup-test --auto] → [qa --auto]
                        │                                         (increment counter; cap MAX_FIX_ITERATIONS)
                        ├─ flag-off    → ✋ HALT + report (human decides flag); env was pushed → release
                        └─ unknown     → ✋ HALT + report (human triage); env was pushed → release

  loop exhausted (counter == MAX_FIX_ITERATIONS) ──► ✋ HALT + report; change pushed → release
```

## Steps

### 0. Identify the ticket
Read the appended ticket context; extract the ticket key.

### 1. Claim the env (composite owns claim)
Preflight, then claim — env only (bare `cpe claim`, not the full cpe-auto skill,
which would run start-ticket and release before setup-test/qa):
```bash
command -v cpe; cpe doctor --json | jq -r '.checks|to_entries|map(select(.key!="flock" and .value!="ok"))|length'   # !=0 → HALT
cpe free --json | jq '.envs | length'            # 0 → HALT (do NOT cold-build)
CLAIM=$(cpe claim <TICKET> --base-ref <BASE_REF> --json)   # add --reset-db before --json if requested
ENV_NAME=$(jq -r .name <<< "$CLAIM"); CODE_PATH=$(jq -r .code_path <<< "$CLAIM"); URL=$(jq -r .url <<< "$CLAIM")
cd "$CODE_PATH"
```
Surface `URL`.

### 2. start-ticket --auto (commits AND pushes)
Invoke **`/dragon-canvas:start-ticket --auto <TICKET>`** (skip its branch-setup
step — `cpe claim` already made the branch). It commits and **pushes** the change.
Read its `## Ticket Result` block.
- `status: stopped` → **HALT**: surface `stop_reason` + `assumptions`; **leave env
  claimed**; stop.
- `status: pushed` (`commit_sha` + `gerrit_change` set) → continue. The change is
  already on Gerrit.

### 3. setup-test --auto
Invoke **`/dragon-canvas:setup-test --auto <TICKET>`**. Capture the entire
`## Test Plan` block (incl. `logins` with passwords — keep it out of verbose logs).
Hard stop → HALT + report; the change is pushed, so release.

### 4. qa --auto
Invoke **`/dragon-canvas:qa --auto`** with the whole Test Plan block. Read the
`## QA Result` block.
- `verdict: pass` → Step 5 (release).
- `verdict: fail` → Step 6 (route findings), unless the loop cap is hit.
- Hard stop → HALT + report; change pushed → release.

### 5. Release (on `verdict: pass`)
```bash
cpe release <ENV_NAME>   # change was already pushed by start-ticket --auto / address-feedback --auto
```
Report the `gerrit_change` URL + that the env was released. **DONE.** Never merge/submit.

### 6. Fix loop (on `verdict: fail`, counter < MAX_FIX_ITERATIONS)
Route each `findings[].likely_owner`:
- **`code-bug`** → the change is already pushed, so invoke
  **`/dragon-canvas:address-feedback --auto <change>`** (amends + **pushes** a
  new patchset) — or `start-ticket --auto <TICKET>` re-fix — passing the QA
  `findings[]` as the defect. Then **re-run Step 3 → Step 4**.
- **`data-setup`** → skip the code fix; **re-run Step 3 → Step 4** (re-provision).
- **`flag-off`** → **HALT + report** (human decides flag); change pushed → release.
- **`unknown`** → **HALT + report** (human triage); change pushed → release.

Increment the counter each pass. `counter == MAX_FIX_ITERATIONS` without a pass →
**HALT + report**; the change is pushed, so release. Never loop forever.

## Final report
End with: which step ended the run, the gate fields, the `gerrit_change` URL,
whether the env was released or left claimed, and any `stop_reason` + partial
output (`assumptions`, `findings[]`).
