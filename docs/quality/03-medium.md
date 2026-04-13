# Medium — Maintainability & Structure

## 9. Oversized Functions (200+ lines)

**Status:** DONE  
**Files:** Multiple TUI files, `cmd/view.go`

**Problem:** Several functions exceed 200 lines, making them hard to read and test:
- `dashboard.Update()` — 247 lines
- `app.Update()` — 232 lines
- `workfloweditor.Update()` — 237 lines
- `formatIssueReadable()` — 205 lines
- `detail.renderContent()` — 171 lines
- `taskmanager.LaunchTask()` — 157 lines

**Plan:**
- For Bubble Tea `Update()` methods: extract message-type handlers into named methods
  (`handleKeyMsg()`, `handleTickMsg()`, etc.) and dispatch from a lean switch
- For `formatIssueReadable()`: split into `formatHeader()`, `formatFields()`, `formatDescription()`,
  `formatComments()` sub-functions
- For `renderContent()`: similar decomposition by content section
- For `LaunchTask()`: separate setup, execution, and result-handling phases
- Target: no function over ~100 lines

---

## 10. Hardcoded Magic Durations

**Status:** DONE  
**Files:** `internal/tui/app.go`, `internal/tui/form.go`, `internal/tui/workfloweditor.go`,
`internal/jira/client.go`, `internal/confluence/client.go`

**Problem:** Timeouts and delays are scattered as inline literals:
`5*time.Minute`, `3*time.Minute`, `30*time.Second`, `5*time.Second`, `8*time.Second`, `10*time.Second`

**Plan:**
- Define named constants in each package (or a shared `internal/defaults` if cross-package):
  ```go
  const (
      APIRequestTimeout    = 30 * time.Second
      ClaudeProcessTimeout = 5 * time.Minute
      StatusMessageDelay   = 3 * time.Second
      // etc.
  )
  ```
- Replace all inline literals with the named constants

---

## 11. Silent Error in Workflow Discovery

**Status:** DONE  
**Files:** `internal/tui/taskmanager.go:50`

**Problem:** When `os.ReadFile()` fails during `DiscoverWorkflows()`, the error is silently
swallowed with `continue`. Workflows may partially load with no feedback to the user.

**Plan:**
- Log a warning (stderr or TUI status message) when a workflow file can't be read
- Include the filename in the message so the user can investigate
- Don't fail hard — partial loading is acceptable, but silent partial loading is not

---

## 12. Ignored defer Close() on Writes

**Status:** DONE  
**Files:** `cmd/init.go:140,172`, `cmd/confluence.go:93,619,734`, `cmd/view.go:93`,
`cmd/attachments.go:207`, `internal/config/config.go:107`

**Problem:** `defer file.Close()` ignores the error return. For read-only files this is fine,
but for files opened for writing, a `Close()` error can mean data wasn't flushed.

**Plan:**
- For write paths (`cmd/init.go`): use a named return + deferred close pattern:
  ```go
  defer func() {
      if cerr := file.Close(); cerr != nil && err == nil {
          err = cerr
      }
  }()
  ```
- For read-only paths: leave as-is (risk is negligible)
