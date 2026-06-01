# Foundation: setup-test-auto

Foundation (base) workflow. Wraps **exactly one** Canvas autonomous flow:
`/canvas-lms-common:setup-test-auto`. It generates and runs the idempotent Ruby
script that builds a manual-testing course for a ticket, provisions login
credentials, and emits a machine-readable **Test Plan**. It does **not** run QA,
push, or chain — the Test Plan is consumed by a separate `qa-auto` step in a
composite.

> Starting template. Refine for your repo, save under a new name. Base
> templates are read-only and never run directly.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| ticket key | **auto-appended by jet** (selected ticket context below), or the current branch name | yes | e.g. `LX-1234`. The test scenario is derived from the ticket. |

## What to do

1. Identify the ticket key from the appended ticket context (or current branch).
2. Invoke **`/canvas-lms-common:setup-test-auto <TICKET_KEY>`**. It is idempotent
   — re-running reuses the existing course rather than duplicating, and creates
   (or resets) the test users with a known dev-only password.
3. Emit the Test Plan as the result. Do not start a browser or run QA here.

## Output (the workflow's result — emit verbatim)

End with the flow's machine-readable block exactly as `/setup-test-auto` defines
it. A composite passes this **whole block (including `logins` with passwords)**
to `qa-auto`:

```
## Test Plan (machine-readable)
ticket: <TICKET>
env_url: <http://canvas-web.docker or the cpe env URL>
course_url: <env_url>/courses/<id>
feature_flag: <flag name or none>
logins:
  teacher: { unique_id: <ticket>_teacher@qa.test, password: <QA_PASSWORD> }
  student: { unique_id: <ticket>_student@qa.test, password: <QA_PASSWORD> }
steps:
  - as: <teacher|student>
    action: <navigate/click/etc — one concrete UI action>
    url: <optional direct URL>
  - ...
expected:
  - <observable expected behavior 1>
  - <observable expected behavior 2>
```

## Hard stops — surface, never swallow

The flow halts (no Test Plan) when: the ticket key can't be determined, the
script fails to execute (Rails error, env down), or the ticket has no testable
scenario. (A missing test user is **not** a stop — the script creates it.)

- **Report the stop reason verbatim**, with whatever was set up so far.
- **Do not auto-retry.** Do not chain into `qa-auto` on a stop.
