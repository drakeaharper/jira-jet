# Low — Defensive Improvements

## 13. Incomplete .gitignore

**Status:** DONE  
**Files:** `.gitignore`

**Problem:** Only ignores `.jet/` and `jet` binary. Missing standard patterns.

**Plan:**
- Add: `.env`, `.env.*`, `*.swp`, `*.swo`, `*~`, `.idea/`, `.vscode/`, `dist/`, `*.test`
- Not urgent — config lives in `~/.jira_config` (outside repo), so no current leak risk

---

## 14. Plaintext Token Storage

**Status:** TODO  
**Files:** `cmd/init.go`, `internal/config/config.go`

**Problem:** API tokens stored as plaintext in `~/.jira_config`. File permissions are enforced
(0600) and the CLI warns the user, which is standard for CLIs.

**Plan (optional, nice-to-have):**
- Investigate OS keychain integration (`keyring` package) for macOS Keychain / Linux secret-service
- Fall back to file-based storage when keychain unavailable
- This is a bigger effort and may not be worth it for a personal CLI tool

---

## 15. No Custom Error Types

**Status:** TODO  
**Files:** Throughout

**Problem:** All errors are string-based `fmt.Errorf`. No sentinel errors or typed errors for
programmatic handling.

**Plan (optional):**
- Define sentinels for common cases: `ErrNotFound`, `ErrAuthFailed`, `ErrForbidden`
- Use in the centralized `checkResponse()` from task #7
- Enables callers to do `if errors.Is(err, jira.ErrNotFound)` instead of string matching
- Only worth doing if the codebase grows to need it
