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

## Foundation rules honored

- **One flow each, no chaining.** Push, release, and routing belong to composite
  workflows — **except** `base-canvas-parallel-env-auto`, whose whole job is the
  claim/push/release lifecycle primitive.
- **Release invariant:** release an env only *after* a push (pushed = resumable).
  Foundation flows create/update Gerrit *changes* (pre-merge); none merges/submits.
- **Machine block is the contract.** Each workflow emits its flow's block
  verbatim — never parse prose.
- **Hard stops surface, never swallow.** On `status: stopped` / hard stop:
  return the `stop_reason` + partial output; **no auto-retry**.
- `base-comments-and-votes-auto` keeps the vote `action_level` a parameter,
  default `recommend-only` (never auto-escalate outward actions).

## Mapping: workflow → flow → inputs → result fields

| Base workflow | Flow (command/skill) | Inputs (where from) | Result block & gate fields |
|---------------|----------------------|---------------------|----------------------------|
| `base-canvas-parallel-env-auto` | `canvas-parallel-env-auto` skill | `mode` (claim\|review), `ticket`\|`change`, `--base-ref`, `--reset-db`, `--focus` — all from instruction box | `## Env Result`: `status`, `env_name`, `url`, `code_path`, `change_url`, `stop_reason` |
| `base-start-ticket-auto` | `/canvas-lms-common:start-ticket-auto` | ticket key (jet auto-appends) | `## Ticket Result`: **`status: committed\|stopped`**, **`commit_sha`**, `branch`, `files[]`, `tests`, `assumptions`, `stop_reason` |
| `base-review-auto` | `/canvas-lms-common:review-auto` | change # or HEAD; `--focus`; `ticket_context` (key+AC, optional) — instruction box | `## Review Summary`: **`verdict: pass\|changes-requested`**, **`ac_status: met\|partial\|unmet\|n/a`**, `ac_gaps[]`, `tickets[]`, `critical_count`, `suggestion_count`, **`critical[]`** |
| `base-address-feedback-auto` | `/canvas-lms-common:address-feedback-auto` | numeric change # (instruction box) | `## Feedback Result`: **`status: amended\|stopped`**, **`amended_sha`**, `comments.{applied,skipped,needs_direction}[]`, `tests`, `stop_reason` |
| `base-setup-test-auto` | `/canvas-lms-common:setup-test-auto` | ticket key (jet auto-appends) | `## Test Plan`: `env_url`, `course_url`, `feature_flag`, **`logins.{teacher,student}.{unique_id,password}`**, **`steps[]`**, **`expected[]`** |
| `base-qa-auto` | `/canvas-lms-common:qa-auto` | Test Plan block (instruction box); ticket fallback | `## QA Result`: **`verdict: pass\|fail`**, `steps_run`, `screenshots[]`, **`findings[].likely_owner`** |
| `base-comments-and-votes-auto` | `/canvas-lms-common:comments-and-votes-auto` | findings + change # + `action_level` (instruction box; default `recommend-only`) | `## Comments & Vote`: **`recommended_cr`**, `action_level`, `rationale`, `comments_count`, **`posted_comments`**, **`cast_vote`** |
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
`/canvas-lms-common:<flow>` commands. **The composite owns claim / push / release**
and runs the inner flows push-free (per ORCHESTRATION.md's lifecycle table).

## `pipeline-canvas-ticket` — TICKET LANE

**Sequence:** claim env → `start-ticket-auto` → `setup-test-auto` → `qa-auto` →
push → release (with a QA fix loop).

```
claim env → start-ticket-auto → setup-test-auto → qa-auto → push → release
                  ▲ (stopped→HALT, leave claimed)        │
                  └──────────── fix loop ◄───────────────┘
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| after start-ticket-auto | `status` | `stopped` → **HALT**, surface `stop_reason`, **leave env claimed**; `committed` (`commit_sha` set) → setup-test-auto |
| after qa-auto | `verdict` | `pass` → push → release; `fail` → route by `findings[].likely_owner` |
| qa fail routing | `findings[].likely_owner` | `code-bug` → start-ticket-auto re-fix → setup-test+qa; `data-setup` → setup-test+qa; `flag-off`/`unknown` → **HALT**, leave claimed |

- **Loop cap:** `MAX_FIX_ITERATIONS` (parameter, default `3`). Exhaustion → HALT,
  leave env claimed.
- **Lifecycle / release:** composite owns claim/push/release. Push
  (`HEAD:refs/for/master`) happens **only** after `qa-auto: pass`; release happens
  **only after push succeeds**. Any halt before push → **env left claimed**. Push
  fails → do not release, leave claimed. Never merge/submit.

## `pipeline-canvas-review` — REVIEW LANE

**Sequence:** claim env (review mode) → `review-auto` → `comments-and-votes-auto`
→ release (always).

```
claim env (review) → review-auto → comments-and-votes-auto → release (always)
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| after claim | `gerry fetch` ok? | fail → release immediately + HALT |
| after review-auto | `verdict` | `changes-requested` → comments-and-votes with `critical[]`+suggestions (rubric → CR-1/CR+1); `pass` → comments-and-votes with empty/nit findings (rubric → CR+2) |
| comments-and-votes | `action_level` | acts only at the authorized level; default `recommend-only` posts nothing |

- **Parameters:** change number (instruction box), `action_level` (default
  `recommend-only`, never auto-escalated), optional `--focus`.
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

### Dry / safe run (do this first)

Run the **review lane** with `action_level: recommend-only` (the default): it
claims an env, reviews the change, and **posts nothing / casts no vote / never
pushes** — it only surfaces the recommended comments + CR and the exact `gerry`
commands, then releases. That exercises the full claim→review→release path with
zero outward writes. Escalate to `post-comments` / `post-and-vote` only when
you've confirmed the recommendation.

(The ticket lane always pushes on `qa-auto: pass`; there is no no-push mode for
it. To rehearse without a push, stop after `qa-auto` and inspect — or use the
review lane for a true read-only dry run.)

## Source of truth

These templates mirror the contracts in
`claude-code-plugins/plugins/canvas-lms-common/`:
`docs/autonomous-flows/ORCHESTRATION.md`, the per-flow files `01`–`07`, and the
authoritative `commands/<flow>.md` / `skills/canvas-parallel-env-auto/SKILL.md`.
Do not edit those command/skill files to change a workflow — edit the template.
