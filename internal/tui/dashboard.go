package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

// issueItem wraps a jira.Issue for the bubbles list.
type issueItem struct {
	issue jira.Issue
}

func (i issueItem) FilterValue() string {
	return i.issue.Key + " " + i.issue.Fields.Summary
}

// issueDelegate renders each issue in the list.
type issueDelegate struct{}

func (d issueDelegate) Height() int                             { return 2 }
func (d issueDelegate) Spacing() int                            { return 1 }
func (d issueDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d issueDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(issueItem)
	if !ok {
		return
	}

	issue := i.issue
	isSelected := index == m.Index()

	// Line 1: Key + Summary
	keyStyle := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	summaryStyle := lipgloss.NewStyle().Foreground(colorWhite)
	if isSelected {
		keyStyle = keyStyle.Background(lipgloss.Color("236"))
		summaryStyle = summaryStyle.Background(lipgloss.Color("236"))
	}

	maxSummaryWidth := m.Width() - len(issue.Key) - 4
	summary := issue.Fields.Summary
	if len(summary) > maxSummaryWidth && maxSummaryWidth > 3 {
		summary = summary[:maxSummaryWidth-3] + "..."
	}

	line1 := keyStyle.Render(issue.Key) + " " + summaryStyle.Render(summary)

	// Line 2: Status | Type | Priority | Assignee
	status := StatusStyle(issue.Fields.Status.Name).Render(issue.Fields.Status.Name)
	parts := []string{status}

	if issue.Fields.IssueType.Name != "" {
		parts = append(parts, dimStyle.Render(issue.Fields.IssueType.Name))
	}
	if issue.Fields.Priority.Name != "" {
		parts = append(parts, PriorityStyle(issue.Fields.Priority.Name).Render(issue.Fields.Priority.Name))
	}

	assignee := "Unassigned"
	if issue.Fields.Assignee != nil {
		assignee = issue.Fields.Assignee.DisplayName
		if assignee == "" {
			assignee = issue.Fields.Assignee.Name
		}
	}
	parts = append(parts, dimStyle.Render(assignee))

	line2 := "  " + strings.Join(parts, dimStyle.Render(" | "))

	cursor := "  "
	if isSelected {
		cursor = "> "
	}

	fmt.Fprintf(w, "%s%s\n%s", cursor, line1, line2)
}

// promptMode tracks what the text input at the bottom is being used for.
type promptMode int

const (
	promptNone promptMode = iota
	promptOpenTicket
	promptEpic
	promptClaude
)

// DashboardModel is the model for the dashboard view.
type DashboardModel struct {
	list       list.Model
	jql        string // the "home" JQL for my tickets
	currentJQL string // what's currently loaded (may differ when viewing epic)
	loading    bool
	spinner    spinner.Model
	total      int

	// Prompt input for open/epic (single-line)
	prompt textinput.Model
	// Prompt for claude instructions (multi-line)
	claudePrompt       textarea.Model
	promptMode         promptMode
	pendingClaudeIssue *jira.Issue // captured when C is pressed, used on ctrl+s

	// Epic viewing state
	viewingEpic  string // non-empty when viewing an epic's children
	epicShowAll  bool   // when true, show closed tickets in epic view
	allEpicItems []jira.Issue // unfiltered epic children for toggle
}

// NewDashboardModel creates a new dashboard model.
func NewDashboardModel(jql string) DashboardModel {
	delegate := issueDelegate{}
	l := list.New([]list.Item{}, delegate, 80, 24)
	l.Title = "Jet Dashboard"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	ti := textinput.New()
	ti.CharLimit = 128

	ta := textarea.New()
	ta.SetHeight(4)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // no limit

	return DashboardModel{
		list:         l,
		jql:          jql,
		currentJQL:   jql,
		loading:      true,
		spinner:      s,
		prompt:       ti,
		claudePrompt: ta,
	}
}

func (d DashboardModel) Init() tea.Cmd {
	return d.spinner.Tick
}

func (d DashboardModel) SetSize(width, height int) DashboardModel {
	h := height
	if d.promptMode == promptClaude {
		h -= 6 // reserve space for textarea + label
	} else if d.promptMode != promptNone {
		h -= 2 // reserve space for single-line prompt
	}
	d.list.SetSize(width, h)
	d.claudePrompt.SetWidth(width - 2)
	return d
}

func (d DashboardModel) SetIssues(issues []jira.Issue, total int) DashboardModel {
	items := make([]list.Item, len(issues))
	for i, issue := range issues {
		items[i] = issueItem{issue: issue}
	}
	d.list.SetItems(items)
	d.loading = false
	d.total = total
	d.updateTitle()
	return d
}

func (d *DashboardModel) updateTitle() {
	if d.viewingEpic != "" {
		label := "open"
		if d.epicShowAll {
			label = "all"
		}
		d.list.Title = fmt.Sprintf("Epic %s (%d %s tickets)", d.viewingEpic, d.total, label)
	} else {
		d.list.Title = fmt.Sprintf("Jet Dashboard (%d tickets)", d.total)
	}
}

func (d DashboardModel) SetEpicChildren(issues []jira.Issue, epicKey string) DashboardModel {
	d.viewingEpic = epicKey
	d.allEpicItems = issues
	d.loading = false
	d = d.applyEpicFilter()
	return d
}

// applyEpicFilter sets the list items based on epicShowAll toggle.
func (d DashboardModel) applyEpicFilter() DashboardModel {
	var filtered []jira.Issue
	if d.epicShowAll {
		filtered = d.allEpicItems
	} else {
		for _, issue := range d.allEpicItems {
			status := strings.ToLower(issue.Fields.Status.Name)
			if status != "closed" && status != "done" && status != "resolved" {
				filtered = append(filtered, issue)
			}
		}
	}
	items := make([]list.Item, len(filtered))
	for i, issue := range filtered {
		items[i] = issueItem{issue: issue}
	}
	d.list.SetItems(items)
	d.total = len(filtered)
	d.updateTitle()
	return d
}

func (d DashboardModel) selectedIssue() *jira.Issue {
	item := d.list.SelectedItem()
	if item == nil {
		return nil
	}
	i, ok := item.(issueItem)
	if !ok {
		return nil
	}
	return &i.issue
}

func (d DashboardModel) Update(msg tea.Msg, client *jira.Client) (DashboardModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle prompt input mode
		if d.promptMode != promptNone {
			// Claude prompt uses textarea — separate handling
			if d.promptMode == promptClaude {
				switch msg.String() {
				case "esc":
					d.promptMode = promptNone
					d.pendingClaudeIssue = nil
					d.claudePrompt.Blur()
					d.claudePrompt.Reset()
					return d, nil
				case "ctrl+s":
					instruction := strings.TrimSpace(d.claudePrompt.Value())
					d.promptMode = promptNone
					d.claudePrompt.Blur()
					d.claudePrompt.Reset()
					if d.pendingClaudeIssue != nil {
						issue := d.pendingClaudeIssue
						d.pendingClaudeIssue = nil
						return d, func() tea.Msg {
							return launchClaudeTaskMsg{issue: issue, instruction: instruction}
						}
					}
					return d, nil
				}
				var cmd tea.Cmd
				d.claudePrompt, cmd = d.claudePrompt.Update(msg)
				return d, cmd
			}

			// Single-line prompts (open ticket, epic)
			switch msg.String() {
			case "esc":
				d.promptMode = promptNone
				d.prompt.Blur()
				return d, nil
			case "enter":
				rawValue := strings.TrimSpace(d.prompt.Value())
				value := strings.ToUpper(rawValue)
				mode := d.promptMode
				d.promptMode = promptNone
				d.prompt.Blur()
				d.prompt.SetValue("")
				if value == "" {
					return d, nil
				}
				switch mode {
				case promptOpenTicket:
					return d, func() tea.Msg { return navigateToDetailMsg{key: value} }
				case promptEpic:
					d.loading = true
					return d, tea.Batch(d.spinner.Tick, fetchEpicChildren(client, value))
				}
				return d, nil
			}
			var cmd tea.Cmd
			d.prompt, cmd = d.prompt.Update(msg)
			return d, cmd
		}

		// Don't handle custom keys when filtering
		if d.list.SettingFilter() {
			break
		}

		switch {
		case key.Matches(msg, dashboardKeys.Enter):
			if issue := d.selectedIssue(); issue != nil {
				return d, func() tea.Msg { return navigateToDetailMsg{key: issue.Key} }
			}

		case key.Matches(msg, dashboardKeys.Open):
			d.promptMode = promptOpenTicket
			d.prompt.Placeholder = "Enter ticket key (e.g. PROJ-123)"
			d.prompt.SetValue("")
			d.prompt.Focus()
			return d, d.prompt.Focus()

		case key.Matches(msg, dashboardKeys.Epic):
			d.promptMode = promptEpic
			d.prompt.Placeholder = "Enter epic key (e.g. PROJ-100)"
			d.prompt.SetValue("")
			d.prompt.Focus()
			return d, d.prompt.Focus()

		case key.Matches(msg, dashboardKeys.BackToMine):
			if d.viewingEpic != "" {
				d.viewingEpic = ""
				d.allEpicItems = nil
				d.epicShowAll = false
				d.loading = true
				d.currentJQL = d.jql
				return d, tea.Batch(d.spinner.Tick, fetchIssues(client, d.jql, 50))
			}

		case key.Matches(msg, dashboardKeys.ToggleAll):
			if d.viewingEpic != "" {
				d.epicShowAll = !d.epicShowAll
				d = d.applyEpicFilter()
				return d, nil
			}

		case key.Matches(msg, dashboardKeys.Claude):
			if issue := d.selectedIssue(); issue != nil {
				issueCopy := *issue
				d.pendingClaudeIssue = &issueCopy
				d.promptMode = promptClaude
				d.claudePrompt.SetWidth(d.list.Width() - 2)
				d.claudePrompt.Placeholder = fmt.Sprintf("Additional instructions for %s (ctrl+s to submit, esc to cancel)", issue.Key)
				d.claudePrompt.Reset()
				return d, d.claudePrompt.Focus()
			}

		case key.Matches(msg, dashboardKeys.Tasks):
			return d, func() tea.Msg { return navigateToTaskViewerMsg{} }

		case key.Matches(msg, dashboardKeys.Create):
			return d, func() tea.Msg { return navigateToFormMsg{issue: nil} }

		case key.Matches(msg, dashboardKeys.Edit):
			if issue := d.selectedIssue(); issue != nil {
				return d, func() tea.Msg { return navigateToFormMsg{issue: issue} }
			}

		case key.Matches(msg, dashboardKeys.Transition):
			if issue := d.selectedIssue(); issue != nil {
				return d, func() tea.Msg { return navigateToTransitionMsg{key: issue.Key} }
			}

		case key.Matches(msg, dashboardKeys.Start):
			if issue := d.selectedIssue(); issue != nil {
				d.loading = true
				return d, quickTransition(client, issue.Key, []string{"in progress", "in development", "progress"})
			}

		case key.Matches(msg, dashboardKeys.Close):
			if issue := d.selectedIssue(); issue != nil {
				d.loading = true
				return d, quickTransition(client, issue.Key, []string{"done", "closed", "resolved", "complete"})
			}

		case key.Matches(msg, dashboardKeys.Grab):
			if issue := d.selectedIssue(); issue != nil {
				d.loading = true
				return d, grabIssueCmd(client, issue.Key)
			}

		case key.Matches(msg, dashboardKeys.Refresh):
			d.loading = true
			if d.viewingEpic != "" {
				return d, tea.Batch(d.spinner.Tick, fetchEpicChildren(client, d.viewingEpic))
			}
			return d, tea.Batch(d.spinner.Tick, fetchIssues(client, d.jql, 50))
		}

	case spinner.TickMsg:
		if d.loading {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Pass through to the list
	var cmd tea.Cmd
	d.list, cmd = d.list.Update(msg)
	cmds = append(cmds, cmd)

	return d, tea.Batch(cmds...)
}

func (d DashboardModel) View() string {
	if d.loading && len(d.list.Items()) == 0 {
		return lipgloss.Place(
			d.list.Width(), d.list.Height(),
			lipgloss.Center, lipgloss.Center,
			d.spinner.View()+" Loading tickets...",
		)
	}

	view := d.list.View()

	// Show prompt if active
	if d.promptMode == promptClaude {
		issueKey := ""
		if d.pendingClaudeIssue != nil {
			issueKey = d.pendingClaudeIssue.Key
		}
		label := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(
			fmt.Sprintf("Claude task for %s:", issueKey),
		)
		taView := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorCyan).
			Render(d.claudePrompt.View())
		hint := dimStyle.Render("ctrl+s: submit  esc: cancel")
		view = lipgloss.JoinVertical(lipgloss.Left, view, label, taView, hint)
	} else if d.promptMode != promptNone {
		var label string
		switch d.promptMode {
		case promptOpenTicket:
			label = "Open ticket: "
		case promptEpic:
			label = "Epic key: "
		}
		promptLine := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(label) + d.prompt.View()
		view = lipgloss.JoinVertical(lipgloss.Left, view, promptLine)
	}

	return view
}
