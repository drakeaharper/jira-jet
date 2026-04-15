package jira

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
	ResolutionDate     string          `json:"resolutiondate"`
	Comment            CommentList     `json:"comment"`
	Attachment         []Attachment    `json:"attachment"`
	Parent             *IssueLink      `json:"parent"`
	EpicLink           *EpicLink       `json:"customfield_10014"`
	IssueLinks         []IssueLinkItem `json:"issuelinks"`
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

type IssueLinkItem struct {
	ID           string              `json:"id"`
	Type         IssueLinkTypeDetail `json:"type"`
	InwardIssue  *LinkedIssue        `json:"inwardIssue,omitempty"`
	OutwardIssue *LinkedIssue        `json:"outwardIssue,omitempty"`
}

type IssueLinkTypeDetail struct {
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

type LinkedIssue struct {
	Key    string       `json:"key"`
	Fields LinkedFields `json:"fields"`
}

type LinkedFields struct {
	Summary   string    `json:"summary"`
	Status    Status    `json:"status"`
	IssueType IssueType `json:"issuetype"`
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

// EscapeString escapes special characters in a JQL string literal.
// Use this when interpolating user input into JQL queries inside double quotes.
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

// checkResponse maps common HTTP error codes to user-friendly errors.
// Returns nil when resp.StatusCode == successCode.
func checkResponse(resp *http.Response, successCode int, resource string) error {
	if resp.StatusCode == successCode {
		return nil
	}
	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("authentication failed - check your credentials")
	case 403:
		return fmt.Errorf("access denied to %s", resource)
	case 404:
		return fmt.Errorf("%s not found", resource)
	default:
		return fmt.Errorf("HTTP %d: request failed for %s", resp.StatusCode, resource)
	}
}

func (c *Client) GetIssue(issueKey string) (*Issue, error) {
	params := url.Values{}
	params.Add("expand", "changelog,renderedFields")
	params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,resolutiondate,issuetype,project,comment,attachment,parent,customfield_10014,issuelinks")

	endpoint := fmt.Sprintf("/rest/api/2/issue/%s?%s", issueKey, params.Encode())

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, 200, "issue "+issueKey); err != nil {
		return nil, err
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
	
	resp, err := c.makeRequest(context.Background(), "POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkResponse(resp, 201, "issue "+issueKey)
}

func (c *Client) UpdateIssue(issueKey string, fields map[string]interface{}) error {
	endpoint := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
	
	reqBody := UpdateIssueRequest{Fields: fields}
	
	resp, err := c.makeRequest(context.Background(), "PUT", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkResponse(resp, 204, "issue "+issueKey)
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
	
	resp, err := c.makeRequest(context.Background(), "POST", endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, 201, "issue creation"); err != nil {
		return nil, err
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
	params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,resolutiondate,issuetype,project,parent,customfield_10014")

	endpoint := fmt.Sprintf("/rest/api/3/search/jql?%s", params.Encode())

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
			return nil, checkResponse(resp, 200, "issue search")
		}
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
		params.Add("fields", "summary,description,status,assignee,reporter,priority,labels,components,fixVersions,created,updated,resolutiondate,issuetype,project,parent,customfield_10014")

		endpoint := fmt.Sprintf("/rest/agile/1.0/epic/%s/issue?%s", epicKey, params.Encode())

		resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if err := checkResponse(resp, 200, "epic "+epicKey); err != nil {
			return nil, err
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
	
	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, 200, "current user"); err != nil {
		return nil, err
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

	resp, err := c.makeRequest(context.Background(), "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, 200, "issue "+issueKey+" transitions"); err != nil {
		return nil, err
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

	resp, err := c.makeRequest(context.Background(), "POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid transition - %s", string(bodyBytes))
	}
	return checkResponse(resp, 204, "issue "+issueKey+" transition")
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

	resp, err := c.makeRequest(context.Background(), "POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 || resp.StatusCode == 404 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: link request failed - %s", resp.StatusCode, string(bodyBytes))
	}
	return checkResponse(resp, 201, "issue link")
}