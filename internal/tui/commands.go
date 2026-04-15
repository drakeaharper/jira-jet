package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jet/internal/jira"
)

// Navigation messages
type navigateToDetailMsg struct{ key string }
type navigateToFormMsg struct{ issue *jira.Issue } // nil = create, non-nil = edit
type navigateToTransitionMsg struct{ key string }
type goBackMsg struct{}
type refreshDashboardMsg struct{}

// API result messages
type issuesLoadedMsg struct {
	issues []jira.Issue
	total  int
}

type issueLoadedMsg struct {
	issue *jira.Issue
}

type transitionsLoadedMsg struct {
	transitions []jira.Transition
	issueKey    string
}

type issueCreatedMsg struct {
	issue *jira.Issue
}

type issueUpdatedMsg struct {
	issueKey string
}

type transitionDoneMsg struct {
	issueKey  string
	newStatus string
}

type commentAddedMsg struct {
	issueKey string
}

type grabDoneMsg struct {
	issueKey    string
	displayName string
}

type errMsg struct {
	err error
}

type epicChildrenLoadedMsg struct {
	issues  []jira.Issue
	epicKey string
}

// Claude task messages
type launchClaudeTaskMsg struct {
	issue           *jira.Issue
	instruction     string
	workflowContent string
}

type claudeTaskDoneMsg struct {
	issueKey string
	task     *Task
	err      error
}

type clearNotificationMsg struct{}

type cancelClaudeTaskMsg struct {
	issueKey string
}

// taskLogTickMsg is sent periodically to refresh the live logs viewport.
type taskLogTickMsg struct{}

func taskLogTick() tea.Cmd {
	return tea.Tick(750*time.Millisecond, func(time.Time) tea.Msg {
		return taskLogTickMsg{}
	})
}

type navigateToTaskViewerMsg struct{}
type navigateToWorkflowEditorMsg struct{}
type navigateToStandupMsg struct{ days int }

type workflowEditorResponseMsg struct {
	chatMessage     string
	workflowContent string
	err             error
}

type workflowSavedMsg struct {
	path string
}

type formClaudeResponseMsg struct {
	chatMessage string
	fields      formClaudeFields
	err         error
}

type formClaudeFields struct {
	Project     string `json:"project"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	IssueType   string `json:"issue_type"`
	Epic        string `json:"epic"`
}

type projectEpicsLoadedMsg struct {
	issues     []jira.Issue
	projectKey string
	total      int
}

type standupDataLoadedMsg struct {
	completed []jira.Issue
	wip       []jira.Issue
}

type clearErrMsg struct{}

// fetchIssues searches for issues matching the given JQL.
func fetchIssues(client *jira.Client, jql string, max int) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.SearchIssues(jql, max)
		if err != nil {
			return errMsg{err: err}
		}
		return issuesLoadedMsg{issues: resp.Issues, total: resp.Total}
	}
}

// fetchIssue gets a single issue by key.
func fetchIssue(client *jira.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		if err != nil {
			return errMsg{err: err}
		}
		return issueLoadedMsg{issue: issue}
	}
}

// fetchTransitions gets available transitions for an issue.
func fetchTransitions(client *jira.Client, key string) tea.Cmd {
	return func() tea.Msg {
		transitions, err := client.GetTransitions(key)
		if err != nil {
			return errMsg{err: err}
		}
		return transitionsLoadedMsg{transitions: transitions, issueKey: key}
	}
}

// createIssue creates a new Jira issue.
func createIssueCmd(client *jira.Client, project, summary, description, issueType, epic string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.CreateIssue(project, summary, description, issueType, epic)
		if err != nil {
			return errMsg{err: err}
		}
		return issueCreatedMsg{issue: issue}
	}
}

// updateIssueCmd updates fields on an existing issue.
func updateIssueCmd(client *jira.Client, key string, fields map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		err := client.UpdateIssue(key, fields)
		if err != nil {
			return errMsg{err: err}
		}
		return issueUpdatedMsg{issueKey: key}
	}
}

// transitionIssueCmd transitions an issue to a new status.
func transitionIssueCmd(client *jira.Client, key, transitionID, targetStatus string) tea.Cmd {
	return func() tea.Msg {
		err := client.TransitionIssue(key, transitionID)
		if err != nil {
			return errMsg{err: err}
		}
		return transitionDoneMsg{issueKey: key, newStatus: targetStatus}
	}
}

// addCommentCmd adds a comment to an issue.
func addCommentCmd(client *jira.Client, key, body string) tea.Cmd {
	return func() tea.Msg {
		err := client.AddComment(key, body)
		if err != nil {
			return errMsg{err: err}
		}
		return commentAddedMsg{issueKey: key}
	}
}

// grabIssueCmd assigns the issue to the current user.
func grabIssueCmd(client *jira.Client, key string) tea.Cmd {
	return func() tea.Msg {
		user, err := client.GetCurrentUser()
		if err != nil {
			return errMsg{err: err}
		}
		assigneeValue := user.AccountID
		if assigneeValue == "" {
			assigneeValue = user.Name
		}
		fields := map[string]interface{}{
			"assignee": map[string]string{"accountId": assigneeValue},
		}
		if user.AccountID == "" {
			fields = map[string]interface{}{
				"assignee": map[string]string{"name": assigneeValue},
			}
		}
		err = client.UpdateIssue(key, fields)
		if err != nil {
			return errMsg{err: err}
		}
		displayName := user.DisplayName
		if displayName == "" {
			displayName = user.Name
		}
		return grabDoneMsg{issueKey: key, displayName: displayName}
	}
}

// quickTransition finds a matching transition by name or target status keywords and executes it.
func quickTransition(client *jira.Client, key string, keywords []string) tea.Cmd {
	return func() tea.Msg {
		transitions, err := client.GetTransitions(key)
		if err != nil {
			return errMsg{err: err}
		}
		for _, t := range transitions {
			name := strings.ToLower(t.Name)
			targetStatus := strings.ToLower(t.To.Name)
			for _, kw := range keywords {
				if strings.Contains(name, kw) || strings.Contains(targetStatus, kw) {
					err := client.TransitionIssue(key, t.ID)
					if err != nil {
						return errMsg{err: err}
					}
					return transitionDoneMsg{issueKey: key, newStatus: t.To.Name}
				}
			}
		}
		return errMsg{err: fmt.Errorf("no matching transition found for %s", key)}
	}
}

// fetchEpicChildren gets all child issues of an epic.
func fetchEpicChildren(client *jira.Client, epicKey string) tea.Cmd {
	return func() tea.Msg {
		issues, err := client.GetEpicChildren(epicKey)
		if err != nil {
			return errMsg{err: err}
		}
		return epicChildrenLoadedMsg{issues: issues, epicKey: epicKey}
	}
}

// fetchProjectEpics searches for epics in a project.
func fetchProjectEpics(client *jira.Client, projectKey string) tea.Cmd {
	return func() tea.Msg {
		jql := fmt.Sprintf("project = \"%s\" AND issuetype = Epic ORDER BY updated DESC", projectKey)
		resp, err := client.SearchIssues(jql, 50)
		if err != nil {
			return errMsg{err: err}
		}
		return projectEpicsLoadedMsg{issues: resp.Issues, projectKey: projectKey, total: resp.Total}
	}
}

// fetchStandupData runs both completed and WIP queries for the standup view.
func fetchStandupData(client *jira.Client, days int) tea.Cmd {
	return func() tea.Msg {
		completedJQL := fmt.Sprintf(
			`assignee = currentUser() AND statusCategory = "Done" AND resolved >= -%dd ORDER BY resolved DESC`,
			days,
		)
		wipJQL := `assignee = currentUser() AND statusCategory = "In Progress" ORDER BY updated DESC`

		completedResp, err := client.SearchIssues(completedJQL, 50)
		if err != nil {
			return errMsg{err: fmt.Errorf("standup completed query failed: %w", err)}
		}

		wipResp, err := client.SearchIssues(wipJQL, 50)
		if err != nil {
			return errMsg{err: fmt.Errorf("standup WIP query failed: %w", err)}
		}

		return standupDataLoadedMsg{
			completed: completedResp.Issues,
			wip:       wipResp.Issues,
		}
	}
}

// clearErrAfter returns a command that clears the error after a delay.
func clearErrAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearErrMsg{}
	})
}

// clearNotificationAfter returns a command that clears the notification after a delay.
func clearNotificationAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearNotificationMsg{}
	})
}
