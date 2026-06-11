# Foundation: canvas-parallel-env-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow: the
**`dragon-canvas:canvas-parallel-env-auto`** skill — the env-lifecycle
primitive. Its job is **claim an isolated env → run an inner flow → release**.

> 1.5.0 model: the **inner flow owns its own outward action** — `start-ticket-auto`
> commits **and pushes**; `review-auto` reviews **and posts comments + votes**.
> The wrapper owns **only claim + release**; it does not push or post.

## Inputs (parameters)

All from the **instruction box** — never prompted, never guessed.

| Parameter | Required | Default | Notes |
|-----------|----------|---------|-------|
| `mode` | yes | — | `claim` (ticket work) or `review` (Gerrit change). |
| `ticket` | claim mode | — | Jira key, e.g. `LX-3975`. |
| `change` | review mode | — | Gerrit change number, e.g. `407569`. |
| `--base-ref` | no | `origin/master` | Branch base for claim mode. |
| `--reset-db` | no | `false` | Reset env DB before checkout (claim mode). |
| `--focus` | no | empty | Review emphasis (review mode); passed through to `review-auto`. |

Missing a required input for the mode → **stop and report** (no guess, no prompt).

## What to do

Invoke the **`dragon-canvas:canvas-parallel-env-auto`** skill with the inputs
above and let it run its lifecycle:

- **Preflight** — `cpe` on `$PATH`, `cpe doctor` clean, a free pool env exists.
- **Claim** — claim an isolated env; `cd` into its `code_path`
  (review mode: `--no-checkout` then `gerry fetch <change>`).
- **Inner flow** (owns its own push/post):
  - claim mode → `/dragon-canvas:start-ticket --auto <ticket>` (commits **and
    pushes** the change).
  - review mode → `/dragon-canvas:review --auto --focus "<focus>"` (reviews
    **and posts comments + casts the CR vote**).
- **Release** — the wrapper owns only this:
  - **inner flow succeeded** (ticket `status: pushed`, or review posted) → `cpe release`.
  - **ticket flow hard-stopped before pushing** (`status: stopped`) → **leave the
    env claimed** (nothing pushed → releasing would orphan local work).
  - **review mode** → release either way (detached HEAD is disposable; the change
    lives on Gerrit).

The wrapper runs only `cpe`, `jq`, `gerry fetch` (review), `git log`/`status`
reads — it never commits, pushes, posts, merges, or `cpe create`s a cold env.

## Output (the workflow's result — emit verbatim)

End with a machine-readable block carrying env identity + lifecycle outcome (a
composite uses `code_path` as the working dir, `env_name` as the release target):

```
## Env Result (machine-readable)
status: released | claimed | stopped
mode: claim | review
env_name: <name | null>
url: <env URL | null>
code_path: <absolute path to checkout | null>
inner_status: <the inner flow's status/verdict, e.g. pushed | stopped | posted | null>
stop_reason: <present only when status: stopped>
```

## Hard stops — surface, never swallow

The skill halts when: `cpe` not on `$PATH`, `cpe doctor` reports a required check
missing, no free pool env, a required arg is missing, or `cpe` exits non-zero.

- **Report the stop reason** with `env_name`/`code_path` if claimed.
- **Do not auto-retry**, and do not cold-build (`cpe create`).
- **Release rule:** review mode releases; claim mode with a pre-push hard stop
  leaves the env claimed for inspection.
