// Package gerrit is a minimal Gerrit REST client for listing changes.
// Ported down from gerrit-cli to only what jet's PR aggregation needs.
package gerrit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"jet/internal/gerry"
	"jet/internal/httpclient"
)

// Account is a Gerrit user.
type Account struct {
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
}

// DisplayName picks the best available label for an account.
func (a Account) DisplayName() string {
	switch {
	case a.Name != "":
		return a.Name
	case a.Username != "":
		return a.Username
	case a.Email != "":
		return a.Email
	default:
		return "unknown"
	}
}

// Change is the subset of a Gerrit change jet displays.
type Change struct {
	Number    int                    `json:"_number"`
	Project   string                 `json:"project"`
	Branch    string                 `json:"branch"`
	Subject   string                 `json:"subject"`
	Status    string                 `json:"status"`
	Updated   string                 `json:"updated,omitempty"`
	Owner     Account                `json:"owner"`
	Labels    map[string]interface{} `json:"labels,omitempty"`
	Mergeable *bool                  `json:"mergeable,omitempty"`
}

// MergeableState reports whether Gerrit supplied a mergeable value and what it
// was. known is false when the server did not compute it.
func (c Change) MergeableState() (known, mergeable bool) {
	if c.Mergeable == nil {
		return false, false
	}
	return true, *c.Mergeable
}

// MinLabelVote returns the lowest vote recorded for a label (from DETAILED_LABELS
// "all"), falling back to the "rejected" summary. hasVote is false when nobody
// has voted on the label.
func (c Change) MinLabelVote(label string) (hasVote bool, min int) {
	raw, ok := c.Labels[label]
	if !ok {
		return false, 0
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return false, 0
	}
	if all, ok := m["all"].([]interface{}); ok {
		for _, v := range all {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			score, ok := vm["value"].(float64)
			if !ok {
				continue
			}
			if !hasVote || int(score) < min {
				min = int(score)
			}
			hasVote = true
		}
	}
	if !hasVote {
		if rejected, ok := m["rejected"].(map[string]interface{}); ok {
			v := -1
			if value, ok := rejected["value"].(float64); ok {
				v = int(value)
			}
			return true, v
		}
	}
	return hasVote, min
}

// LabelVote returns the net vote for a label (e.g. "Code-Review"): the max
// positive or, if none, the min negative. 0 means no votes / label absent.
func (c Change) LabelVote(label string) int {
	raw, ok := c.Labels[label]
	if !ok {
		return 0
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return 0
	}
	if _, ok := m["approved"]; ok {
		return 2
	}
	if _, ok := m["rejected"]; ok {
		return -2
	}
	if _, ok := m["recommended"]; ok {
		return 1
	}
	if _, ok := m["disliked"]; ok {
		return -1
	}
	return 0
}

// Client talks to Gerrit's authenticated REST API.
type Client struct {
	cfg  *gerry.Config
	http *http.Client
}

// NewClient builds a Gerrit REST client from gerry's config.
func NewClient(cfg *gerry.Config) *Client {
	return &Client{cfg: cfg, http: httpclient.New(30 * time.Second)}
}

// ListChanges runs a Gerrit query and returns matching changes.
func (c *Client) ListChanges(query string, limit int) ([]Change, error) {
	// query is passed raw; Gerrit accepts spaces/operators here. url.QueryEscape
	// over-encodes some operators, so build the query string manually.
	path := fmt.Sprintf("changes/?q=%s&n=%d&o=DETAILED_LABELS&o=DETAILED_ACCOUNTS", urlQuery(query), limit)
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var changes []Change
	if err := json.Unmarshal(body, &changes); err != nil {
		return nil, fmt.Errorf("failed to parse changes: %w", err)
	}
	return changes, nil
}

func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.cfg.RESTURL(path), nil)
	if err != nil {
		return nil, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.cfg.User + ":" + c.cfg.HTTPPassword))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gerrit request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gerrit response: %w", err)
	}
	if resp.StatusCode >= 400 {
		switch resp.StatusCode {
		case 401:
			return nil, fmt.Errorf("gerrit auth failed (401) — check ~/.gerry/config.json http_password")
		case 403:
			return nil, fmt.Errorf("gerrit access forbidden (403)")
		default:
			return nil, fmt.Errorf("gerrit request failed with status %d: %s", resp.StatusCode, string(body))
		}
	}
	// Strip Gerrit's )]}' XSS-guard prefix.
	if bytes.HasPrefix(body, []byte(")]}'")) {
		body = body[4:]
	}
	return body, nil
}

// urlQuery percent-encodes only the characters that break a query string,
// leaving Gerrit operators (: - () +) intact.
func urlQuery(q string) string {
	var b bytes.Buffer
	for _, r := range q {
		switch r {
		case ' ':
			b.WriteByte('+')
		case '&', '#', '?', '%':
			b.WriteString(fmt.Sprintf("%%%02X", r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
