# Critical — Security & Correctness

## 1. JQL/CQL Injection

**Status:** DONE  
**Files:** `cmd/list.go:53,60,65,72`, `cmd/confluence.go:~190`

**Problem:** User input from `--assignee`, `--status`, `--project` flags is interpolated directly
into JQL/CQL query strings via `fmt.Sprintf`. Malicious input can break out of the quoted context.

**Plan:**
- Create a `jql.Escape(s string) string` helper that escapes special JQL characters
  (double quotes, backslashes, and reserved JQL operators)
- Apply it at every interpolation site in `cmd/list.go` and `cmd/confluence.go`
- Add test cases: normal input, input with quotes, input with backslashes, input with JQL keywords

---

## 2. No context.Context in API Clients

**Status:** DONE  
**Files:** `internal/jira/client.go:244`, `internal/confluence/client.go:122`

**Problem:** `makeRequest()` uses `http.NewRequest()` — no cancellation, no deadline propagation.
The TUI can't cancel in-flight requests on quit, and CLI commands can't respect OS signals.

**Plan:**
- Add `ctx context.Context` as the first parameter to `makeRequest()` in both clients
- Switch to `http.NewRequestWithContext(ctx, ...)`
- Thread `context.Background()` from CLI commands and proper cancellable contexts from TUI code
- Update all call sites (every exported client method)

---

## 3. os.Exit() in Command Handler

**Status:** DONE  
**Files:** `cmd/epic.go:48,56,85,96`

**Problem:** `epicCmd` calls `os.Exit(1)` directly instead of returning errors through Cobra's
`RunE` function. This bypasses deferred cleanup and breaks testability.

**Plan:**
- Change `Run` to `RunE` on the epic command
- Replace each `os.Exit(1)` with `return fmt.Errorf(...)`
- Verify no other commands use this pattern (initial audit found only `epic.go`)
