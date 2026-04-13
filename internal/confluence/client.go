package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"jet/internal/httpclient"
)

type Client struct {
	BaseURL    string
	Email      string
	Username   string
	Token      string
	HTTPClient *http.Client
}

type Page struct {
	ID      string      `json:"id"`
	Status  string      `json:"status"`
	Title   string      `json:"title"`
	SpaceID string      `json:"spaceId,omitempty"`
	Body    *PageBody   `json:"body,omitempty"`
	Version *Version    `json:"version,omitempty"`
	Links   *PageLinks  `json:"_links,omitempty"`
}

type PageBody struct {
	Storage *BodyContent `json:"storage,omitempty"`
	View    *BodyContent `json:"view,omitempty"`
}

type BodyContent struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

type Version struct {
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
}

type PageLinks struct {
	WebUI   string `json:"webui,omitempty"`
	TinyUI  string `json:"tinyui,omitempty"`
	EditUI  string `json:"editui,omitempty"`
	Base    string `json:"base,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Start   int            `json:"start"`
	Limit   int            `json:"limit"`
	Size    int            `json:"size"`
}

type SearchResult struct {
	Content      ContentInfo  `json:"content,omitempty"`
	Title        string       `json:"title"`
	Excerpt      string       `json:"excerpt,omitempty"`
	URL          string       `json:"url,omitempty"`
	LastModified string       `json:"lastModified,omitempty"`
}

type ContentInfo struct {
	ID      string     `json:"id"`
	Type    string     `json:"type"`
	Status  string     `json:"status"`
	Title   string     `json:"title"`
	Space   SpaceInfo  `json:"space,omitempty"`
	Links   PageLinks  `json:"_links,omitempty"`
}

type SpaceInfo struct {
	ID   string `json:"id,omitempty"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Space struct {
	ID          string     `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
}

// EscapeString escapes special characters in a CQL string literal.
// Use this when interpolating user input into CQL queries inside double quotes.
func EscapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func NewClient(baseURL, email, username, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Email:      email,
		Username:   username,
		Token:      token,
		HTTPClient: httpclient.New(httpclient.DefaultTimeout),
	}
}

func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	url := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication (same as JIRA)
	if c.Email != "" && c.Token != "" {
		req.SetBasicAuth(c.Email, c.Token)
	} else if c.Username != "" && c.Token != "" {
		req.SetBasicAuth(c.Username, c.Token)
	} else {
		return nil, fmt.Errorf("authentication credentials not provided")
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "jira-jet/1.0 (Security-Enhanced)")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// GetPage retrieves a Confluence page by ID
func (c *Client) GetPage(pageID string) (*Page, error) {
	params := url.Values{}
	params.Add("body-format", "storage")

	endpoint := fmt.Sprintf("/wiki/api/v2/pages/%s?%s", pageID, params.Encode())

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("page %s not found", pageID)
	}
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view this page")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: request failed", resp.StatusCode)
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// SearchPages searches for Confluence pages using CQL
func (c *Client) SearchPages(cql string, limit int) (*SearchResponse, error) {
	params := url.Values{}
	params.Add("cql", cql)
	params.Add("limit", fmt.Sprintf("%d", limit))

	endpoint := fmt.Sprintf("/wiki/rest/api/search?%s", params.Encode())

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to search")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: search failed", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResp, nil
}

// SearchByText is a convenience method for simple text searches
func (c *Client) SearchByText(searchText string, limit int) (*SearchResponse, error) {
	cql := fmt.Sprintf("type=page AND text~\"%s\"", EscapeString(searchText))
	return c.SearchPages(cql, limit)
}

// SearchBySpace searches for pages within a specific space
func (c *Client) SearchBySpace(spaceKey string, limit int) (*SearchResponse, error) {
	cql := fmt.Sprintf("type=page AND space=\"%s\"", EscapeString(spaceKey))
	return c.SearchPages(cql, limit)
}

// GetSpace retrieves space information by space key
func (c *Client) GetSpace(spaceKey string) (*Space, error) {
	endpoint := fmt.Sprintf("/wiki/api/v2/spaces?keys=%s", spaceKey)

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("space %s not found", spaceKey)
	}
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view this space")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: request failed", resp.StatusCode)
	}

	var result struct {
		Results []Space `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("space %s not found", spaceKey)
	}

	return &result.Results[0], nil
}

// CreatePageRequest represents the request to create a new page
type CreatePageRequest struct {
	SpaceID  string             `json:"spaceId"`
	Status   string             `json:"status"`
	Title    string             `json:"title"`
	ParentID string             `json:"parentId,omitempty"`
	Body     CreatePageBody     `json:"body"`
}

type CreatePageBody struct {
	Representation string `json:"representation"`
	Value          string `json:"value"`
}

// UpdatePageRequest represents the request to update an existing page
type UpdatePageRequest struct {
	ID       string        `json:"id"`
	Status   string        `json:"status"`
	Title    string        `json:"title"`
	SpaceID  string        `json:"spaceId"`
	ParentID string        `json:"parentId,omitempty"`
	Body     CreatePageBody `json:"body"`
	Version  UpdateVersion  `json:"version"`
}

type UpdateVersion struct {
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
}

// CreatePage creates a new Confluence page
func (c *Client) CreatePage(spaceID, title, content string, parentID string) (*Page, error) {
	// Build the create request
	createReq := CreatePageRequest{
		SpaceID: spaceID,
		Status:  "current",
		Title:   title,
		Body: CreatePageBody{
			Representation: "storage",
			Value:          content,
		},
	}

	if parentID != "" {
		createReq.ParentID = parentID
	}

	endpoint := "/wiki/api/v2/pages"

	resp, err := c.makeRequest(context.Background(), "POST", endpoint, createReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body for better error messages
	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to create pages in this space")
	}
	if resp.StatusCode == 400 {
		return nil, fmt.Errorf("invalid request: %s", string(bodyBytes))
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("HTTP %d: failed to create page: %s", resp.StatusCode, string(bodyBytes))
	}

	var page Page
	if err := json.Unmarshal(bodyBytes, &page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// UpdatePage updates an existing Confluence page
func (c *Client) UpdatePage(pageID, title, content, spaceID string, version int, parentID, versionMessage string) (*Page, error) {
	// Build the update request
	updateReq := UpdatePageRequest{
		ID:      pageID,
		Status:  "current",
		Title:   title,
		SpaceID: spaceID,
		Body: CreatePageBody{
			Representation: "storage",
			Value:          content,
		},
		Version: UpdateVersion{
			Number:  version,
			Message: versionMessage,
		},
	}

	if parentID != "" {
		updateReq.ParentID = parentID
	}

	endpoint := fmt.Sprintf("/wiki/api/v2/pages/%s", pageID)

	resp, err := c.makeRequest(context.Background(), "PUT", endpoint, updateReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body for better error messages
	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to update this page")
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("page %s not found", pageID)
	}
	if resp.StatusCode == 409 {
		return nil, fmt.Errorf("version conflict - page was modified by another user. Please retry the command")
	}
	if resp.StatusCode == 400 {
		return nil, fmt.Errorf("invalid request: %s", string(bodyBytes))
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("HTTP %d: failed to update page: %s", resp.StatusCode, string(bodyBytes))
	}

	var page Page
	if err := json.Unmarshal(bodyBytes, &page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// ChildPagesResponse represents the response from getting child pages
type ChildPagesResponse struct {
	Results []ChildPage `json:"results"`
	Links   struct {
		Next string `json:"next,omitempty"`
	} `json:"_links"`
}

type ChildPage struct {
	ID      string     `json:"id"`
	Type    string     `json:"type"`
	Status  string     `json:"status"`
	Title   string     `json:"title"`
	Version *Version   `json:"version,omitempty"`
	Links   *PageLinks `json:"_links,omitempty"`
}

// GetChildPages retrieves direct children of a page
func (c *Client) GetChildPages(pageID string, limit int) (*ChildPagesResponse, error) {
	params := url.Values{}
	params.Add("limit", fmt.Sprintf("%d", limit))

	endpoint := fmt.Sprintf("/wiki/api/v2/pages/%s/children?%s", pageID, params.Encode())

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("page %s not found", pageID)
	}
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view this page")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: failed to get page children", resp.StatusCode)
	}

	var childrenResp ChildPagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&childrenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &childrenResp, nil
}
