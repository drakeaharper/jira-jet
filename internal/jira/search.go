package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

// RawSearchPage is one page of /rest/api/3/search/jql with issues left as raw JSON.
type RawSearchPage struct {
	Issues        []json.RawMessage `json:"issues"`
	NextPageToken string            `json:"nextPageToken"`
	IsLast        bool              `json:"isLast"`
}

// SearchJQLRaw runs one page of a JQL search, passing params straight to the Jira search API.
func (c *Client) SearchJQLRaw(params url.Values) (*RawSearchPage, error) {
	body, err := c.getRaw("/rest/api/3/search/jql?"+params.Encode(), "issue search")
	if err != nil {
		return nil, err
	}
	var page RawSearchPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &page, nil
}

// ListFieldsRaw returns every field Jira knows, including custom fields.
func (c *Client) ListFieldsRaw() ([]byte, error) {
	return c.getRaw("/rest/api/3/field", "field list")
}

func (c *Client) getRaw(endpoint, resource string) ([]byte, error) {
	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode == 400 {
			return nil, fmt.Errorf("HTTP 400: %s", string(body))
		}
		return nil, checkResponse(resp, 200, resource)
	}
	return body, nil
}
