# Foundation (base) jet workflows

Read-only **starting templates** for the Canvas autonomous dev flows — one
foundation workflow per flow, plus one **resolver helper**
(`base-resolve-change-from-ticket`). Each wraps **exactly one** Claude
command/skill (the helper instead shells out to `jet`/`gerry`), declares its
inputs, and emits a machine-readable output block verbatim so a later
**composite** workflow can branch on the gate fields.

## How these are used

These files are **embedded in the jet binary** (`//go:embed baseflows/*.md`),
not written to `~/.jet/workflows/`. Consequences:

- They are **read-only** and **do not appear** in the Claude-task launcher
  (`C` / resume), which only lists on-disk workflows from `~/.jet/workflows/`.
- They are offered **only** when you **create a new workflow** in the editor
  (`W` → "Create new"): pick a base template, it pre-loads into the preview,
  refine it with Claude, then save under a **new** name. The saved copy is what
  becomes launchable.

## Foundation rules honored (1.5.0 model)

- **One flow each, no cross-flow chaining/routing** — that belongs to composites.
- **Each `-auto` flow owns its own outward action.** `start-ticket-auto` and
  `address-feedback-auto` commit **and push** (`status: pushed`); `review-auto`
  reviews **and posts comments + casts the CR vote** (via its Step 5 →
  `comments-and-votes-auto`). `canvas-parallel-env-auto` owns **only** the env
  lifecycle (claim + release); it does not push or post.
- **Release invariant:** release only after the flow has pushed/posted (a pushed
  change is resumable). A ticket flow that hard-stops before pushing → leave the
  env claimed. Flows create/update Gerrit *changes* (pre-merge); none merges/submits.
- **Machine block is the contract.** Each workflow emits its flow's block
  verbatim — never parse prose.
- **Hard stops surface, never swallow.** On `status: stopped` / hard stop:
  return the `stop_reason` + partial output; **no auto-retry**.
- `base-comments-and-votes-auto` exposes `action_level` as a parameter; the
  flow's default is **`post-and-vote`** (posts + votes). `recommend-only` is the
  opt-in dry run. Never escalate *above* the level given.

## Mapping: workflow → flow → inputs → result fields

| Base workflow | Flow (command/skill) | Inputs (where from) | Result block & gate fields |
|---------------|----------------------|---------------------|----------------------------|
| `base-canvas-parallel-env-auto` | `canvas-parallel-env-auto` skill (claim/release only; inner flow pushes/posts) | `mode` (claim\|review), `ticket`\|`change`, `--base-ref`, `--reset-db`, `--focus` — all from instruction box | `## Env Result`: **`status: released\|claimed\|stopped`**, `mode`, `env_name`, `url`, `code_path`, `inner_status`, `stop_reason` |
| `base-start-ticket-auto` | `/dragon-canvas:start-ticket --auto` (commits **and pushes**) | ticket key (jet auto-appends) | `## Ticket Result`: **`status: pushed\|stopped`**, **`commit_sha`**, **`gerrit_change`**, `branch`, `files[]`, `tests`, `assumptions`, `stop_reason` |
| `base-review-auto` | `/dragon-canvas:review --auto` (reviews **and posts comments + votes**) | change # or HEAD; `--focus`; `ticket_context` (key+AC, optional); `action_level` (default post-and-vote) — instruction box | `## Review Summary`: **`verdict: pass\|changes-requested`**, **`ac_status: met\|partial\|unmet\|n/a`**, `ac_gaps[]`, `tickets[]`, `critical[]`, `suggestions[]{kind}` + the Step-5 `## Comments & Vote` block (`posted_comments`, `cast_vote`) |
| `base-address-feedback-auto` | `/dragon-canvas:address-feedback --auto` (amends **and pushes** a patchset) | numeric change # (instruction box) | `## Feedback Result`: **`status: pushed\|stopped`**, **`amended_sha`**, `comments.{applied,skipped,needs_direction}[]`, `tests`, `stop_reason` |
| `base-setup-test-auto` | `/dragon-canvas:setup-test --auto` | ticket key (jet auto-appends) | `## Test Plan`: `env_url`, `course_url`, `feature_flag`, **`logins.{teacher,student}.{unique_id,password}`**, **`steps[]`**, **`expected[]`** |
| `base-qa-auto` | `/dragon-canvas:qa --auto` | Test Plan block (instruction box); ticket fallback | `## QA Result`: **`verdict: pass\|fail`**, `steps_run`, `screenshots[]`, **`findings[].likely_owner`** |
| `base-comments-and-votes-auto` | `/dragon-canvas:comments-and-votes --auto` (posts + votes) | findings + change # + `action_level` (instruction box; **default `post-and-vote`**) | `## Comments & Vote`: **`recommended_cr`**, `action_level`, `rationale`, `comments_count`, **`posted_comments`**, **`cast_vote`** |
| `base-resolve-change-from-ticket` (HELPER) | `jet view --format json` + `jq` (gerritbot comment) + optional `gerry details` | ticket key (jet auto-appends); change-# override (instruction box) | `## Resolved Change`: **`change_number`**, `change_id`, `ticket_context.{key,summary,acceptance_criteria}`, `candidates[]`, `status`, `stop_reason` |

## Invoking one

1. `jet tui`, select a ticket.
2. Press `C`, pick the workflow you created from a base template.
3. In the instruction box, supply any inputs that don't come from the ticket
   (e.g. a Gerrit change number, `--focus "..."`, or `action_level: post-comments`).
4. `ctrl+s` to launch.

(The ticket key is auto-appended by jet; flows whose unit of work is a Gerrit
change ignore that incidental ticket context.)

---

# Composite (pipeline) workflows

Two composites chain the foundation flows end-to-end per ORCHESTRATION.md's two
lanes. They ship as the same kind of read-only template (same embed loader, same
"Create new" picker) — distinguished by the `pipeline-` prefix (vs the `base-`
prefix on foundation flows), so they group together in the picker.

### jet has no chaining engine — how a composite works

A jet workflow is **one markdown file → one headless `claude -p` run**. jet has
**no step/DAG engine and no conditional-branching primitive**. So a composite is
a single file whose **prose** drives the one Claude agent: it invokes each
`-auto` Claude command in sequence and **branches on the next flow's machine-block
gate fields** (`status`, `commit_sha`, `verdict`, `findings[].likely_owner`,
`recommended_cr`). The agent does the branching; jet just supplies the prompt.
A composite cannot literally "call" a foundation `base-*.md` file (no include
mechanism) — it inlines the orchestration, invoking the same underlying
`/dragon-canvas:<flow>` commands. **The composite owns claim, release, and
sequencing; the inner flows own their own push/post** (1.5.0 lifecycle table) —
the composite does not add a push or a posting step.

## `pipeline-canvas-ticket` — TICKET LANE

**Sequence:** claim env → `start-ticket-auto` (commit + **push**) →
`setup-test-auto` → `qa-auto` → release (with a QA fix loop). No separate push
node — the flows push themselves.

```
claim env → start-ticket-auto → setup-test-auto → qa-auto → release
            (commit + PUSH)                          │
                  ▲ (stopped→HALT, leave claimed)     │
                  └─ fix: address-feedback-auto ◄─────┘
                     (amend + PUSH new patchset)
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| after start-ticket-auto | `status` | `stopped` → **HALT**, surface `stop_reason`, **leave env claimed** (nothing pushed); `pushed` (`commit_sha`+`gerrit_change` set) → setup-test-auto |
| after qa-auto | `verdict` | `pass` → release (already pushed); `fail` → route by `findings[].likely_owner` |
| qa fail routing | `findings[].likely_owner` | `code-bug` → `address-feedback-auto` (amend+push) or start-ticket re-fix → setup-test+qa; `data-setup` → setup-test+qa; `flag-off`/`unknown` → **HALT** (change pushed → release) |

- **Loop cap:** `MAX_FIX_ITERATIONS` (parameter, default `3`). Exhaustion → HALT.
- **Lifecycle / release:** composite owns claim + release + sequencing; the
  **flows push themselves** (`start-ticket-auto`, `address-feedback-auto`). Release
  after `qa-auto: pass` (the change is already on Gerrit). A `start-ticket-auto:
  stopped` (pre-push) → **leave env claimed**. Never merge/submit.

## `pipeline-canvas-review` — REVIEW LANE

**Sequence:** `[resolve-from-ticket]?` → claim env (review mode) → `review-auto`
(reviews **and posts comments + votes**) → release (always). `review-auto` owns
the posting via its Step 5 — the composite does **not** add a separate
comments-and-votes step or suppress it.

```
[resolve-from-ticket]? → claim env (review) → review-auto → release (always)
                                              (review + POST + VOTE)
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| (entry) | change # vs ticket key | numeric change # → claim directly; ticket key → resolve newest gerritbot change first (capture AC) |
| after claim | `gerry fetch` ok? | fail → release immediately + HALT |
| review-auto Step 5 | `action_level` | posts inline comments + casts CR vote at the effective level; default **`post-and-vote`** |
| (informational) | `verdict` / `ac_status` | `changes-requested`/`ac!=met` → CR-1/CR+1 already cast; `pass` → CR+2, zero/nit comments |

- **Parameters:** change # **or** ticket key (instruction box / auto-appended),
  `--focus` (optional), `action_level` (default **`post-and-vote`**; opt down to
  `post-comments` or `recommend-only` for a dry run — never escalate above).
- **Lifecycle / release:** read-only on the repo, **no push**; **always releases**
  at the end (success or hard stop). Never merge/submit.

## Invoking a composite end-to-end

1. `jet tui` → `W` → "Create new" → pick `pipeline-canvas-ticket` (or
   `pipeline-canvas-review`) → refine if desired → `ctrl+s`, save as e.g.
   `ticket-pipeline`.
2. Select a ticket (ticket lane) → press `C` → pick `ticket-pipeline` → in the
   instruction box add optional params (`MAX_FIX_ITERATIONS=2`, `--reset-db`) →
   `ctrl+s`.
3. Review lane: press `C` → pick `review-pipeline` → instruction box: the numeric
   Gerrit change # (+ optional `--focus`, `action_level`) → `ctrl+s`.

### Dry / safe run

The review lane now **defaults to `post-and-vote`** — it will post comments and
cast the CR vote on Gerrit. For a zero-write rehearsal, explicitly pass
`action_level: recommend-only`: it claims an env, reviews the change, **posts
nothing / casts no vote / never pushes** — only surfaces the recommended comments
+ CR and the exact `gerry` commands, then releases. Use `post-comments` for a
middle ground (comments, no vote).

(The ticket lane's `start-ticket-auto` **pushes a patchset as soon as it commits**
— before QA — so there is no no-push mode for it. For a true read-only dry run,
use the review lane with `recommend-only`.)

## Source of truth

These templates mirror the contracts in
`dragon-marketplace/plugins/dragon-canvas/`:
`docs/autonomous-flows/ORCHESTRATION.md`, the per-flow files `01`–`07`, and the
authoritative `commands/<flow>.md` / `skills/canvas-parallel-env-auto/SKILL.md`.
Do not edit those command/skill files to change a workflow — edit the template.
