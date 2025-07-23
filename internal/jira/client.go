package jira

import (
	"bytes"
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
	Summary     string      `json:"summary"`
	Description string      `json:"description"`
	Status      Status      `json:"status"`
	IssueType   IssueType   `json:"issuetype"`
	Priority    Priority    `json:"priority"`
	Project     Project     `json:"project"`
	Assignee    *User       `json:"assignee"`
	Reporter    *User       `json:"reporter"`
	Labels      []string    `json:"labels"`
	Components  []Component `json:"components"`
	FixVersions []Version   `json:"fixVersions"`
	Created     string      `json:"created"`
	Updated     string      `json:"updated"`
	Comment     CommentList `json:"comment"`
	Attachment  []Attachment `json:"attachment"`
	Parent      *IssueLink  `json:"parent"`
	EpicLink    *EpicLink   `json:"customfield_10014"`
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
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
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

func NewClient(baseURL, email, username, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Email:      email,
		Username:   username,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
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

	return &issue, nil
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: failed to create issue - %s", resp.StatusCode, string(body))
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issue, nil
}