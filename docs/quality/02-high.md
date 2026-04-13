# High — Performance & DRY

## 4. Regex Compiled on Every Call

**Status:** DONE  
**Files:** `cmd/view.go:413-494`

**Problem:** `formatHTMLContent()` calls `regexp.MustCompile()` on ~15 patterns every invocation.
This is called per-ticket in list views — unnecessary allocation and compile cost.

**Plan:**
- Hoist all `regexp.MustCompile()` calls to package-level `var` block at top of `cmd/view.go`
- Name them descriptively: `reHeading`, `reBold`, `reLink`, etc.
- Reference the pre-compiled vars in `formatHTMLContent()`

---

## 5. Duplicated TLS Configuration

**Status:** DONE  
**Files:** `internal/jira/client.go:218-230`, `internal/confluence/client.go:95-108`

**Problem:** Identical TLS config (MinVersion, CipherSuites, PreferServerCipherSuites) is
copy-pasted in both client constructors.

**Plan:**
- Create `internal/httpclient/httpclient.go` with a `New(timeout time.Duration) *http.Client`
  function that returns a client with the standard TLS config
- Replace both inline configs with a call to this shared constructor
- Both clients keep their own `http.Client` field — only the construction is shared

---

## 6. Duplicated HTML Entity Decoding

**Status:** DONE  
**Files:** `cmd/confluence.go:243-248`, `cmd/confluence.go:316-321`, `cmd/view.go`

**Problem:** The same `strings.NewReplacer` for `&amp;`, `&lt;`, `&gt;`, `&nbsp;`, `&quot;`,
`&#39;` appears in 3+ locations.

**Plan:**
- Create a shared `decodeHTMLEntities(s string) string` in a small `internal/textutil` package
  (or as a private function in `cmd/` if all callers are there)
- Replace all inline replacer blocks with this single function

---

## 7. Duplicated HTTP Status Code Handling

**Status:** DONE  
**Files:** `internal/jira/client.go`, `internal/confluence/client.go`

**Problem:** 401/403/404 status checks with human-friendly error messages are repeated 16+ times
per client, with slight message variations.

**Plan:**
- Add a `checkResponse(resp *http.Response, resourceDesc string) error` method to each client
  (or shared in `internal/httpclient`)
- Centralizes status-to-error mapping: 401 -> auth error, 403 -> permission error, 404 -> not found
- Each call site becomes: `if err := c.checkResponse(resp, "issue PROJ-123"); err != nil { return err }`

---

## 8. Inconsistent Error Wrapping

**Status:** NOT AN ISSUE  
**Files:** Throughout codebase (~222 `fmt.Errorf` calls, ~50/50 split on `%w` usage)

**Problem:** Some errors wrap with `%w` (preserving the chain), others use `%v` or no verb at all.
`errors.Is()` / `errors.As()` won't work reliably.

**Plan:**
- Establish convention: use `%w` when the error has a cause worth preserving, plain string when
  it's a validation/sentinel message with no underlying error
- Audit all `fmt.Errorf` calls — fix the ones that have an `err` argument but use `%v` instead of `%w`
- Low-risk sweep since it only changes error chain behavior, not control flow
