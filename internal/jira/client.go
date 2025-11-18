package jira

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

type Issue struct {
	Key    string `json:"key"`
	Fields Fields `json:"fields"`
}

type Fields struct {
	Summary            string          `json:"summary"`
	Description        json.RawMessage `json:"description"`
	DescriptionText    string          `json:"-"` // Parsed text version
	Status             Status          `json:"status"`
	IssueType          IssueType       `json:"issuetype"`
	Priority           Priority        `json:"priority"`
	Project            Project         `json:"project"`
	Assignee           *User           `json:"assignee"`
	Reporter           *User           `json:"reporter"`
	Labels             []string        `json:"labels"`
	Components         []Component     `json:"components"`
	FixVersions        []Version       `json:"fixVersions"`
	Created            string          `json:"created"`
	Updated            string          `json:"updated"`
	Comment            CommentList     `json:"comment"`
	Attachment         []Attachment    `json:"attachment"`
	Parent             *IssueLink      `json:"parent"`
	EpicLink           *EpicLink       `json:"customfield_10014"`
}

type Status struct {
	Name string `json:"name"`
}

type IssueType struct {
	Name string `json:"name"`
}

type Priority struct {
	Name string `json:"name"`
}

type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type User struct {
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

type Component struct {
	Name string `json:"name"`
}

type Version struct {
	Name string `json:"name"`
}

type CommentList struct {
	Comments []Comment `json:"comments"`
}

type Comment struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	Author  User   `json:"author"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"`
	Author   User   `json:"author"`
	Created  string `json:"created"`
}

type IssueLink struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}

type EpicLink struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}

type CreateIssueRequest struct {
	Fields CreateIssueFields `json:"fields"`
}

type CreateIssueFields struct {
	Project     ProjectRef `json:"project"`
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	IssueType   IssueTypeRef `json:"issuetype"`
	Parent      *IssueRef  `json:"parent,omitempty"`
}

type ProjectRef struct {
	Key string `json:"key"`
}

type IssueTypeRef struct {
	Name string `json:"name"`
}

type IssueRef struct {
	Key string `json:"key"`
}

type UpdateIssueRequest struct {
	Fields map[string]interface{} `json:"fields"`
}

type AddCommentRequest struct {
	Body string `json:"body"`
}

type SearchRequest struct {
	JQL        string   `json:"jql"`
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	Fields     []string `json:"fields"`
}

type SearchResponse struct {
	Issues     []Issue `json:"issues"`
	StartAt    int     `json:"startAt"`
	Total      int     `json:"total"`
	MaxResults int     `json:"maxResults"`
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name string `json:"name"`
	} `json:"to"`
}

type TransitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

type TransitionRequest struct {
	Transition TransitionRef `json:"transition"`
}

type TransitionRef struct {
	ID string `json:"id"`
}

type IssueLinkRequest struct {
	Type        IssueLinkType `json:"type"`
	InwardIssue IssueRef      `json:"inwardIssue"`
	OutwardIssue IssueRef     `json:"outwardIssue"`
	Comment     *Comment      `json:"comment,omitempty"`
}

type IssueLinkType struct {
	Name string `json:"name"`
}

func NewClient(baseURL, email, username, token string) *Client {
	// Configure secure TLS settings
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

	// Set authentication
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

func (c *Client) GetIssue(issueKey string) (*Issue, error) {
	params := url.Values{}
	params.Add("expand", "changelog,renderedFields")
	params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,issuetype,project,comment,attachment,parent,customfield_10014")

	endpoint := fmt.Sprintf("/rest/api/2/issue/%s?%s", issueKey, params.Encode())

	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("issue %s not found", issueKey)
	}
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view this issue")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: request failed", resp.StatusCode)
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse description
	issue.Fields.DescriptionText = parseDescription(issue.Fields.Description)

	return &issue, nil
}

// parseDescription converts the description field (which can be either a string or ADF object) to plain text
func parseDescription(desc json.RawMessage) string {
	if len(desc) == 0 {
		return ""
	}

	// Try to unmarshal as string first (API v2)
	var strDesc string
	if err := json.Unmarshal(desc, &strDesc); err == nil {
		return strDesc
	}

	// If that fails, it's probably an ADF object (API v3)
	// For now, just return a simplified text extraction
	// TODO: Implement full ADF parsing if needed
	var adfDoc map[string]interface{}
	if err := json.Unmarshal(desc, &adfDoc); err == nil {
		return extractTextFromADF(adfDoc)
	}

	return ""
}

// extractTextFromADF recursively extracts text content from Atlassian Document Format
func extractTextFromADF(node map[string]interface{}) string {
	var text strings.Builder

	// Extract text from this node
	if nodeText, ok := node["text"].(string); ok {
		text.WriteString(nodeText)
	}

	// Recursively process content array
	if content, ok := node["content"].([]interface{}); ok {
		for _, child := range content {
			if childMap, ok := child.(map[string]interface{}); ok {
				childText := extractTextFromADF(childMap)
				if childText != "" {
					text.WriteString(childText)
					text.WriteString(" ")
				}
			}
		}
	}

	return text.String()
}

func (c *Client) AddComment(issueKey, comment string) error {
	endpoint := fmt.Sprintf("/rest/api/2/issue/%s/comment", issueKey)
	
	reqBody := AddCommentRequest{Body: comment}
	
	resp, err := c.makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("issue %s not found", issueKey)
	}
	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied - you may not have permission to comment on this issue")
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("HTTP %d: failed to add comment", resp.StatusCode)
	}

	return nil
}

func (c *Client) UpdateIssue(issueKey string, fields map[string]interface{}) error {
	endpoint := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
	
	reqBody := UpdateIssueRequest{Fields: fields}
	
	resp, err := c.makeRequest("PUT", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("issue %s not found", issueKey)
	}
	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied - you may not have permission to update this issue")
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("HTTP %d: failed to update issue", resp.StatusCode)
	}

	return nil
}

func (c *Client) CreateIssue(projectKey, summary, description, issueType, epicKey string) (*Issue, error) {
	endpoint := "/rest/api/2/issue"
	
	reqBody := CreateIssueRequest{
		Fields: CreateIssueFields{
			Project:     ProjectRef{Key: projectKey},
			Summary:     summary,
			Description: description,
			IssueType:   IssueTypeRef{Name: issueType},
		},
	}

	// Add epic link if provided
	if epicKey != "" {
		reqBody.Fields.Parent = &IssueRef{Key: epicKey}
	}
	
	resp, err := c.makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to create issues in this project")
	}
	if resp.StatusCode != 201 {
		// Don't expose full response body as it might contain sensitive info
		return nil, fmt.Errorf("HTTP %d: failed to create issue", resp.StatusCode)
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issue, nil
}

func (c *Client) SearchIssues(jql string, maxResults int) (*SearchResponse, error) {
	return c.SearchIssuesWithPagination(jql, 0, maxResults)
}

func (c *Client) SearchIssuesWithPagination(jql string, startAt int, maxResults int) (*SearchResponse, error) {
	// Use GET request with query parameters for v3 API
	// The v3 API uses /rest/api/3/search/jql with GET method
	params := url.Values{}
	params.Add("jql", jql)
	params.Add("startAt", fmt.Sprintf("%d", startAt))
	params.Add("maxResults", fmt.Sprintf("%d", maxResults))
	params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,issuetype,project,parent,customfield_10014")

	endpoint := fmt.Sprintf("/rest/api/3/search/jql?%s", params.Encode())

	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to search issues")
	}
	if resp.StatusCode != 200 {
		// Read response body for more context
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: search failed - %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body to handle it properly
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// First unmarshal to a generic map to see what fields are present
	var rawResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var searchResp SearchResponse
	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// If total is 0 but we have issues, use the issue count
	// This handles API v3 which may not return total in all cases
	if searchResp.Total == 0 && len(searchResp.Issues) > 0 {
		searchResp.Total = len(searchResp.Issues)
	}

	// Parse descriptions for all issues
	for i := range searchResp.Issues {
		searchResp.Issues[i].Fields.DescriptionText = parseDescription(searchResp.Issues[i].Fields.Description)
	}

	return &searchResp, nil
}

func (c *Client) GetEpicChildren(epicKey string) ([]Issue, error) {
	// Use the Agile API with pagination to get all epic children
	var allIssues []Issue
	startAt := 0
	maxResults := 50

	for {
		params := url.Values{}
		params.Add("startAt", fmt.Sprintf("%d", startAt))
		params.Add("maxResults", fmt.Sprintf("%d", maxResults))
		params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,issuetype,project,parent,customfield_10014")

		endpoint := fmt.Sprintf("/rest/agile/1.0/epic/%s/issue?%s", epicKey, params.Encode())

		resp, err := c.makeRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 401 {
			return nil, fmt.Errorf("authentication failed - check your credentials")
		}
		if resp.StatusCode == 403 {
			return nil, fmt.Errorf("access denied - you may not have permission to view epic issues")
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("epic %s not found", epicKey)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d: failed to get epic children", resp.StatusCode)
		}

		var searchResp SearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		allIssues = append(allIssues, searchResp.Issues...)

		// Check if we've fetched all issues
		if startAt + len(searchResp.Issues) >= searchResp.Total {
			break
		}

		startAt += maxResults
	}

	return allIssues, nil
}

func (c *Client) getEpicChildrenViaSearch(epicKey string) ([]Issue, error) {
	jql := fmt.Sprintf("\"Epic Link\" = %s ORDER BY key ASC", epicKey)

	var allIssues []Issue
	startAt := 0
	maxResults := 100

	for {
		searchResp, err := c.SearchIssuesWithPagination(jql, startAt, maxResults)
		if err != nil {
			return nil, fmt.Errorf("failed to search for child issues: %w", err)
		}

		allIssues = append(allIssues, searchResp.Issues...)

		// Check if we've fetched all issues
		if startAt + len(searchResp.Issues) >= searchResp.Total {
			break
		}

		startAt += maxResults
	}

	return allIssues, nil
}


func (c *Client) GetCurrentUser() (*User, error) {
	endpoint := "/rest/api/2/myself"
	
	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view user information")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: failed to get current user", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &user, nil
}

// GetTransitions retrieves available transitions for an issue
func (c *Client) GetTransitions(issueKey string) ([]Transition, error) {
	endpoint := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueKey)

	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("issue %s not found", issueKey)
	}
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied - you may not have permission to view transitions")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: failed to get transitions", resp.StatusCode)
	}

	var transResp TransitionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&transResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return transResp.Transitions, nil
}

// TransitionIssue transitions an issue to a new status
func (c *Client) TransitionIssue(issueKey, transitionID string) error {
	endpoint := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueKey)

	reqBody := TransitionRequest{
		Transition: TransitionRef{ID: transitionID},
	}

	resp, err := c.makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("issue %s not found", issueKey)
	}
	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied - you may not have permission to transition this issue")
	}
	if resp.StatusCode == 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid transition - %s", string(bodyBytes))
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("HTTP %d: failed to transition issue", resp.StatusCode)
	}

	return nil
}

// LinkIssues creates a link between two issues
func (c *Client) LinkIssues(inwardIssue, outwardIssue, linkType string, isInward bool) error {
	endpoint := "/rest/api/2/issueLink"

	// If the relationship is inward (e.g., "is-blocked-by"), swap the issues
	// because the API always expects the relationship from the outward perspective
	if isInward {
		inwardIssue, outwardIssue = outwardIssue, inwardIssue
	}

	reqBody := IssueLinkRequest{
		Type: IssueLinkType{
			Name: linkType,
		},
		InwardIssue: IssueRef{
			Key: inwardIssue,
		},
		OutwardIssue: IssueRef{
			Key: outwardIssue,
		},
	}

	resp, err := c.makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed - check your credentials")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied - you may not have permission to link issues")
	}
	if resp.StatusCode == 404 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("issue not found - %s", string(bodyBytes))
	}
	if resp.StatusCode == 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid link request - %s", string(bodyBytes))
	}
	if resp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: failed to create link - %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}