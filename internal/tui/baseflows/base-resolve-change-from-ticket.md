# Foundation helper: resolve-change-from-ticket

Foundation (base) **helper** workflow. It resolves a Jira ticket to its Gerrit
change number, because there is **no gerry/jet ticket→change query** — the link
lives in **gerritbot comments** on the ticket. It also captures the ticket
context (key, summary, acceptance criteria) a ticket-rooted `review-auto` needs.

It is read-only: it queries Jira and (optionally) Gerrit, and does **not** check
out, push, review, or chain. A composite calls it first, then feeds
`change_number` + `ticket_context` into `canvas-parallel-env-auto` (review mode)
and `review-auto`.

## Inputs (parameters)

| Parameter | Source | Required | Notes |
|-----------|--------|----------|-------|
| ticket key | **auto-appended by jet** (selected ticket context below), or typed in the instruction box | yes | e.g. `LX-1234`. |
| change # override | **instruction box** (optional) | no | If the ticket links multiple distinct changes, pass the explicit number to use instead of "newest". |

## What to do

### 1. Fetch the ticket as JSON
```bash
jet view <TICKET> --format json
```

### 2. Resolve the change number from the newest gerritbot comment
The change number is the `/+/<n>` in the body of the **most recent gerritbot
comment**. The author filter is **mandatory** — never pick up a human-pasted
Gerrit URL.
```bash
jet view <TICKET> --format json | jq -r '
  [ .fields.comment.comments[]
    | select(.author.emailAddress=="gerritbot@canvas.net") ]
  | sort_by(.created) | reverse | .[0].body
  | capture("/\\+/(?<n>[0-9]+)").n'
```
- **Zero gerritbot comments** → **STOP** (nothing to review). Surface the stop
  reason; do not guess a change.
- **Multiple distinct change numbers** across gerritbot comments → default to the
  **newest**, but expose all of them in `candidates[]` and honor a change-#
  override from the instruction box if given.

### 3. Capture ticket context (from the same JSON)
- `summary` ← `.fields.summary`.
- `acceptance_criteria` ← extract from the ticket **description** (the JSON
  `.fields.description` is raw ADF; if easier, also run `jet view <TICKET>`
  readable to get rendered text and pull the Acceptance Criteria section).
  Best-effort: if the ticket has no explicit AC section, leave it empty.

### 4. (Optional) confirm the change is OPEN
```bash
gerry details <n>     # confirm the change exists and is OPEN
```
If it resolves but is not open, still report the number and note its state; let
the composite decide.

## Output (the workflow's result — emit verbatim)

End with this machine-readable block so a composite can gate on `change_number`
and pass `ticket_context` into a ticket-rooted review:

```
## Resolved Change (machine-readable)
status: resolved | stopped
ticket: <KEY>
change_number: <n | null>
change_id: <Gerrit Change-Id | null>
ticket_context:
  key: <KEY>
  summary: <ticket summary>
  acceptance_criteria: [ "<criterion>", ... ]   # empty if none found
candidates: [ <n1>, <n2>, ... ]   # present only when >1 distinct change linked
stop_reason: <present only when status: stopped>
```

## Hard stops — surface, never swallow

- **No gerritbot comment** on the ticket → `status: stopped`,
  `change_number: null`, `stop_reason: "no gerritbot comment links a change"`.
- `jet view` fails (auth, ticket not found) → `status: stopped` + `stop_reason`.
- **Do not auto-retry.** Still emit whatever `ticket_context` you gathered.
- Never select a change from a non-gerritbot (human-pasted) comment.
