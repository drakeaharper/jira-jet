# Foundation: canvas-parallel-env-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow: the
**`canvas-lms-common:canvas-parallel-env-auto`** skill. Unlike the other six
foundation workflows (which wrap a single inner flow and never push/release),
**this one is the env-lifecycle primitive**: its whole job is claim → run an
inner flow → (claim mode, on success) push → release. It honors the
**release invariant**: an env is released only *after* a push, because a pushed
commit is resumable and can't be orphaned. It creates/updates a Gerrit *change*
(pre-merge, abandonable); it **never merges/submits**.

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

All come from the **instruction box** at launch — never prompted, never guessed.

| Parameter | Required | Default | Notes |
|-----------|----------|---------|-------|
| `mode` | yes | — | `claim` (ticket work) or `review` (Gerrit change). |
| `ticket` | claim mode | — | Jira key, e.g. `LX-3975`. (May also be the jet-appended ticket key in claim mode.) |
| `change` | review mode | — | Gerrit change number, e.g. `407569`. |
| `--base-ref` | no | `origin/master` | Branch base for claim mode. |
| `--reset-db` | no | `false` | Reset env DB before checkout (claim mode). |
| `--focus` | no | empty | Free-text review focus (review mode); passed through to `review-auto`. |

If a **required** input for the chosen mode is missing, **stop and report** — do
not guess, do not prompt.

## What to do

Invoke the **`canvas-lms-common:canvas-parallel-env-auto`** skill with the
inputs above and let it run its lifecycle end to end:

- **Preflight** — `cpe` on `$PATH`, `cpe doctor` clean, a free pool env exists.
- **Claim** — claim an isolated env; `cd` into its `code_path`.
- **Inner flow** — claim mode runs `/canvas-lms-common:start-ticket-auto
  <ticket>`; review mode runs `/canvas-lms-common:review-auto --focus "<focus>"`
  against the fetched change.
- **Push & release** (the invariant):
  - **review mode** — always `cpe release` at the end (the change already lives
    on Gerrit; detached HEAD is disposable).
  - **claim mode, success** — `git push origin HEAD:refs/for/master`, **then**
    `cpe release`.
  - **claim mode, failure** — **leave the env claimed** (nothing pushed →
    releasing would orphan local work). Report `env_name` + `code_path`.

Only `cpe`, `jq`, `gerry fetch` (review), and the two owned git ops
(`git push …refs/for/master`, `git log`/`git status`) are run here. Never merge,
never submit, never `cpe create` a cold env in autonomous mode.

## Output (the workflow's result — emit verbatim)

End with a machine-readable block carrying the env identity (so a composite can
use `code_path` as the working dir and `env_name` as the release target) plus
the lifecycle outcome:

```
## Env Result (machine-readable)
status: claimed | pushed-and-released | released | stopped
mode: claim | review
env_name: <name | null>
url: <env URL | null>
code_path: <absolute path to checkout | null>
change_url: <Gerrit change URL | null>   # claim-mode success only
stop_reason: <present only when status: stopped>
```

## Hard stops — surface, never swallow

The skill halts when: `cpe` is not on `$PATH`, `cpe doctor` reports a required
check missing, no free pool env exists, a required arg is missing, or `cpe`
exits non-zero.

- **Report the stop reason verbatim** with `env_name`/`code_path` if an env was
  claimed.
- **Do not auto-retry**, and do not cold-build (`cpe create`) — that's a human
  decision.
- **Release rule on failure:** review mode releases; claim mode with nothing
  pushed leaves the env claimed for inspection. If a push fails, **do not
  release**.
