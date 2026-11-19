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

// QuickFilter is a board quick filter and its JQL.
type QuickFilter struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	JQL         string `json:"jql"`
	Description string `json:"description"`
}

// BoardQuickFilters returns every quick filter on a board.
func (c *Client) BoardQuickFilters(boardID string) ([]QuickFilter, error) {
	var all []QuickFilter
	for start := 0; ; {
		body, err := c.getRaw(fmt.Sprintf("/rest/agile/1.0/board/%s/quickfilter?startAt=%d", url.PathEscape(boardID), start), "board quick filters")
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []QuickFilter `json:"values"`
			IsLast bool          `json:"isLast"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		all = append(all, page.Values...)
		if page.IsLast || len(page.Values) == 0 {
			return all, nil
		}
		start += len(page.Values)
	}
}

// BoardFilterJQL returns the JQL of the saved filter behind a board.
func (c *Client) BoardFilterJQL(boardID string) (string, error) {
	body, err := c.getRaw(fmt.Sprintf("/rest/agile/1.0/board/%s/configuration", url.PathEscape(boardID)), "board configuration")
	if err != nil {
		return "", err
	}
	var cfg struct {
		Filter struct {
			ID string `json:"id"`
		} `json:"filter"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	body, err = c.getRaw("/rest/api/3/filter/"+url.PathEscape(cfg.Filter.ID), "board filter")
	if err != nil {
		return "", err
	}
	var f struct {
		JQL string `json:"jql"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return f.JQL, nil
}
