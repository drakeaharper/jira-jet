# Composite (pipeline) jet workflows

Read-only **starting templates** for the Canvas autonomous dev lanes. Each
pipeline is a single markdown file that orchestrates the dragon-canvas
`/dragon-canvas:*` autonomous commands end-to-end — claim an env, run the flows
in sequence, branch on their machine-readable gate fields, and release.

There are no per-command "base" wrapper templates. A single `-auto` command is
invoked directly as a slash command (`/dragon-canvas:qa --auto`, etc.); only the
**multi-step orchestration** is worth shipping as a jet template, and that lives
here in the two pipelines.

## How these are used

These files are **embedded in the jet binary** (`//go:embed baseflows/*.md`),
not written to `~/.jet/workflows/`. Consequences:

- They are **read-only** and **do not appear** in the Claude-task launcher
  (`C` / resume), which only lists on-disk workflows from `~/.jet/workflows/`.
- They are offered **only** when you **create a new workflow** in the editor
  (`W` → "Create new"): pick a pipeline template, it pre-loads into the preview,
  refine it with Claude, then save under a **new** name. The saved copy is what
  becomes launchable.

## The dragon-canvas commands own the contract

The pipelines **do not redeclare** each command's machine-readable output
schema, mode behavior, or hard-stop conditions — the `/dragon-canvas:<flow>`
command/skill is authoritative for all of that. The pipeline only references the
**gate field names** it branches on. So when a dragon-canvas command's output
evolves, the pipelines inherit it with no edit here.

The commands' relevant contracts (for reference only):

| Command | Gate fields the pipeline branches on |
|---------|--------------------------------------|
| `/dragon-canvas:start-ticket --auto` | `status` (pushed\|stopped), `commit_sha`, `gerrit_change` |
| `/dragon-canvas:setup-test --auto` | the whole `## Test Plan` block (incl. `logins`) → fed to qa |
| `/dragon-canvas:qa --auto` | `verdict` (pass\|fail), `findings[].likely_owner` |
| `/dragon-canvas:address-feedback --auto` | `status` (pushed\|stopped), `amended_sha` |
| `/dragon-canvas:review --auto` | `verdict`, `ac_status`; posts comments + votes itself |
| `canvas-parallel-env-auto` skill | env lifecycle (claim + release) only |

## jet has no chaining engine — how a pipeline works

A jet workflow is **one markdown file → one headless `claude -p` run**. jet has
**no step/DAG engine and no conditional-branching primitive**. So a pipeline is
a single file whose **prose** drives the one Claude agent: it invokes each
`-auto` command in sequence and **branches on the next flow's machine-block gate
fields** (`status`, `commit_sha`, `verdict`, `findings[].likely_owner`). The
agent does the branching; jet just supplies the prompt. **The pipeline owns
claim, release, and sequencing; the inner flows own their own push/post** — the
pipeline adds no push or posting step.

## `pipeline-canvas-ticket` — TICKET LANE

**Sequence:** claim env → `start-ticket --auto` (commit + **push**) →
`setup-test --auto` → `qa --auto` → release (with a QA fix loop). No separate
push node — the flows push themselves.

```
claim env → start-ticket --auto → setup-test --auto → qa --auto → release
            (commit + PUSH)                          │
                  ▲ (stopped→HALT, leave claimed)     │
                  └─ fix: address-feedback --auto ◄─────┘
                     (amend + PUSH new patchset)
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| after start-ticket --auto | `status` | `stopped` → **HALT**, surface `stop_reason`, **leave env claimed** (nothing pushed); `pushed` (`commit_sha`+`gerrit_change` set) → setup-test --auto |
| after qa --auto | `verdict` | `pass` → release (already pushed); `fail` → route by `findings[].likely_owner` |
| qa fail routing | `findings[].likely_owner` | `code-bug` → `address-feedback --auto` (amend+push) or start-ticket re-fix → setup-test+qa; `data-setup` → setup-test+qa; `flag-off`/`unknown` → **HALT** (change pushed → release) |

- **Loop cap:** `MAX_FIX_ITERATIONS` (parameter, default `3`). Exhaustion → HALT.
- **Lifecycle / release:** pipeline owns claim + release + sequencing; the
  **flows push themselves** (`start-ticket --auto`, `address-feedback --auto`).
  Release after `qa --auto: pass` (the change is already on Gerrit). A
  `start-ticket --auto: stopped` (pre-push) → **leave env claimed**. Never
  merge/submit.

## `pipeline-canvas-review` — REVIEW LANE

**Sequence:** `[resolve change from ticket]?` → claim env (review mode) →
`review --auto` (reviews **and posts comments + votes**) → release (always).
`review --auto` owns the posting via its Step 5 — the pipeline does **not** add a
separate comments-and-votes step or suppress it.

```
[resolve from ticket]? → claim env (review) → review --auto → release (always)
                                              (review + POST + VOTE)
```

| Branch point | Gate field | Routing |
|--------------|-----------|---------|
| (entry) | change # vs ticket key | numeric change # → claim directly; ticket key → resolve newest gerritbot change first (capture AC) |
| after claim | `gerry fetch` ok? | fail → release immediately + HALT |
| review --auto Step 5 | `action_level` | posts inline comments + casts CR vote at the effective level; default **`post-and-vote`** |
| (informational) | `verdict` / `ac_status` | `changes-requested`/`ac!=met` → CR-1/CR+1 already cast; `pass` → CR+2, zero/nit comments |

- **Resolve-from-ticket** is inlined in Step 0 (`jet view --format json` + `jq`
  on the newest gerritbot comment) — there is no separate helper template.
- **Parameters:** change # **or** ticket key (instruction box / auto-appended),
  `--focus` (optional), `action_level` (default **`post-and-vote`**; opt down to
  `post-comments` or `recommend-only` for a dry run — never escalate above).
- **Lifecycle / release:** read-only on the repo, **no push**; **always
  releases** at the end (success or hard stop). Never merge/submit.

## Invoking a pipeline end-to-end

1. `jet tui` → `W` → "Create new" → pick `pipeline-canvas-ticket` (or
   `pipeline-canvas-review`) → refine if desired → `ctrl+s`, save as e.g.
   `ticket-pipeline`.
2. Select a ticket (ticket lane) → press `C` → pick `ticket-pipeline` → in the
   instruction box add optional params (`MAX_FIX_ITERATIONS=2`, `--reset-db`) →
   `ctrl+s`.
3. Review lane: press `C` → pick `review-pipeline` → instruction box: the numeric
   Gerrit change # (+ optional `--focus`, `action_level`) → `ctrl+s`.

### Dry / safe run

The review lane **defaults to `post-and-vote`** — it will post comments and cast
the CR vote on Gerrit. For a zero-write rehearsal, explicitly pass
`action_level: recommend-only`: it claims an env, reviews the change, **posts
nothing / casts no vote / never pushes** — only surfaces the recommended comments
+ CR and the exact `gerry` commands, then releases. Use `post-comments` for a
middle ground (comments, no vote).

(The ticket lane's `start-ticket --auto` **pushes a patchset as soon as it
commits** — before QA — so there is no no-push mode for it. For a true read-only
dry run, use the review lane with `recommend-only`.)

## Source of truth

These pipelines orchestrate the contracts defined in
`dragon-marketplace/plugins/dragon-canvas/`: the authoritative
`commands/<flow>.md` and `skills/canvas-parallel-env-auto/SKILL.md`. The command
files own each flow's behavior and output schema; the pipelines own only
claim/release/sequencing. To change a flow's behavior, edit the command; to
change how the lanes chain those flows, edit the pipeline.
