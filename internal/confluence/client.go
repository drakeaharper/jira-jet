package confluence

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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

func NewClient(baseURL, email, username, token string) *Client {
	// Configure secure TLS settings (same as JIRA)
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Email:      email,
		Username:   username,
		Token:      token,
		HTTPClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) makeRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	url := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	req, err := http.NewRequest(method, url, reqBody)
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

	resp, err := c.makeRequest("GET", endpoint, nil)
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

	resp, err := c.makeRequest("GET", endpoint, nil)
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
	cql := fmt.Sprintf("type=page AND text~\"%s\"", searchText)
	return c.SearchPages(cql, limit)
}

// SearchBySpace searches for pages within a specific space
func (c *Client) SearchBySpace(spaceKey string, limit int) (*SearchResponse, error) {
	cql := fmt.Sprintf("type=page AND space=\"%s\"", spaceKey)
	return c.SearchPages(cql, limit)
}

// GetSpace retrieves space information by space key
func (c *Client) GetSpace(spaceKey string) (*Space, error) {
	endpoint := fmt.Sprintf("/wiki/api/v2/spaces?keys=%s", spaceKey)

	resp, err := c.makeRequest("GET", endpoint, nil)
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

	resp, err := c.makeRequest("POST", endpoint, createReq)
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
