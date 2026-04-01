package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

type viewID int

const (
	viewDashboard viewID = iota
	viewDetail
	viewForm
	viewTransition
	viewTaskViewer
)

// App is the top-level Bubble Tea model.
type App struct {
	client *jira.Client
	width  int
	height int

	activeView viewID
	viewStack  []viewID

	dashboard  DashboardModel
	detail     DetailModel
	form       FormModel
	transition TransitionModel
	taskViewer TaskViewerModel

	taskManager  *TaskManager
	notification string

	err    error
	errMsg string
}

// NewApp creates a new App model.
func NewApp(client *jira.Client, initialJQL string, tm *TaskManager) App {
	return App{
		client:      client,
		activeView:  viewDashboard,
		dashboard:   NewDashboardModel(initialJQL),
		taskManager: tm,
	}
}

// Run launches the Bubble Tea program.
func Run(client *jira.Client, initialJQL string) error {
	tm := NewTaskManager()
	app := NewApp(client, initialJQL, tm)
	p := tea.NewProgram(app, tea.WithAltScreen())
	tm.SetProgram(p)
	_, err := p.Run()
	return err
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.dashboard.Init(),
		fetchIssues(a.client, a.dashboard.jql, 50),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Reserve 2 lines for status/help bar
		contentHeight := a.height - 2
		a.dashboard = a.dashboard.SetSize(a.width, contentHeight)
		// Only resize views that are currently active (avoids nil pointer on uninitialized models)
		switch a.activeView {
		case viewDetail:
			a.detail = a.detail.SetSize(a.width, contentHeight)
		case viewForm:
			a.form = a.form.SetSize(a.width, contentHeight)
		case viewTransition:
			a.transition = a.transition.SetSize(a.width, contentHeight)
		case viewTaskViewer:
			a.taskViewer = a.taskViewer.SetSize(a.width, contentHeight)
		}
		return a, nil

	case tea.KeyMsg:
		// Global quit from dashboard only (not when a prompt is active)
		if a.activeView == viewDashboard && msg.String() == "q" && !a.dashboard.list.SettingFilter() && a.dashboard.promptMode == promptNone {
			return a, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

	case errMsg:
		a.err = msg.err
		a.errMsg = msg.err.Error()
		cmds = append(cmds, clearErrAfter(5*time.Second))
		return a, tea.Batch(cmds...)

	case clearErrMsg:
		a.err = nil
		a.errMsg = ""
		return a, nil

	// Navigation messages
	case navigateToDetailMsg:
		a.viewStack = append(a.viewStack, a.activeView)
		a.activeView = viewDetail
		a.detail = NewDetailModel()
		a.detail = a.detail.SetSize(a.width, a.height-2)
		return a, fetchIssue(a.client, msg.key)

	case navigateToFormMsg:
		a.viewStack = append(a.viewStack, a.activeView)
		a.activeView = viewForm
		a.form = NewFormModel(msg.issue)
		a.form = a.form.SetSize(a.width, a.height-2)
		return a, a.form.Init()

	case navigateToTransitionMsg:
		a.viewStack = append(a.viewStack, a.activeView)
		a.activeView = viewTransition
		a.transition = NewTransitionModel(msg.key)
		a.transition = a.transition.SetSize(a.width, a.height-2)
		return a, tea.Batch(a.transition.Init(), fetchTransitions(a.client, msg.key))

	case goBackMsg:
		if len(a.viewStack) > 0 {
			a.activeView = a.viewStack[len(a.viewStack)-1]
			a.viewStack = a.viewStack[:len(a.viewStack)-1]
		}
		return a, nil

	case refreshDashboardMsg:
		if a.dashboard.viewingEpic != "" {
			return a, fetchEpicChildren(a.client, a.dashboard.viewingEpic)
		}
		return a, fetchIssues(a.client, a.dashboard.jql, 50)

	// API result messages that may need routing
	case epicChildrenLoadedMsg:
		a.dashboard = a.dashboard.SetEpicChildren(msg.issues, msg.epicKey)
		return a, nil

	case issuesLoadedMsg:
		a.dashboard = a.dashboard.SetIssues(msg.issues, msg.total)
		return a, nil

	case issueLoadedMsg:
		a.detail = a.detail.SetIssue(msg.issue)
		return a, nil

	case transitionsLoadedMsg:
		a.transition = a.transition.SetTransitions(msg.transitions, msg.issueKey)
		return a, nil

	case issueCreatedMsg:
		a.errMsg = ""
		a.err = nil
		a.viewStack = a.viewStack[:len(a.viewStack)-1]
		a.activeView = viewDashboard
		return a, a.refreshDashboard()

	case issueUpdatedMsg:
		a.viewStack = a.viewStack[:len(a.viewStack)-1]
		a.activeView = viewDashboard
		return a, a.refreshDashboard()

	case transitionDoneMsg:
		// Pop back to previous view
		if len(a.viewStack) > 0 {
			a.activeView = a.viewStack[len(a.viewStack)-1]
			a.viewStack = a.viewStack[:len(a.viewStack)-1]
		}
		return a, a.refreshDashboard()

	case commentAddedMsg:
		// Refresh the detail view
		if a.detail.issue != nil {
			return a, fetchIssue(a.client, a.detail.issue.Key)
		}
		return a, nil

	case grabDoneMsg:
		return a, a.refreshDashboard()

	// Claude task messages
	case launchClaudeTaskMsg:
		if a.taskManager.IsRunning(msg.issue.Key) {
			a.errMsg = fmt.Sprintf("[%s] Task already running", msg.issue.Key)
			cmds = append(cmds, clearErrAfter(3*time.Second))
			return a, tea.Batch(cmds...)
		}
		if err := a.taskManager.LaunchTask(msg.issue, msg.instruction); err != nil {
			a.errMsg = fmt.Sprintf("Failed to launch task: %s", err)
			cmds = append(cmds, clearErrAfter(5*time.Second))
			return a, tea.Batch(cmds...)
		}
		a.notification = fmt.Sprintf("[%s] Claude task launched", msg.issue.Key)
		cmds = append(cmds, clearNotificationAfter(3*time.Second))
		return a, tea.Batch(cmds...)

	case claudeTaskDoneMsg:
		if msg.err != nil {
			a.errMsg = fmt.Sprintf("[%s] Claude task failed: %s", msg.issueKey, msg.err)
			cmds = append(cmds, clearErrAfter(8*time.Second))
		} else {
			a.notification = fmt.Sprintf("[%s] Claude task completed ($%.4f)", msg.issueKey, msg.task.Cost)
			cmds = append(cmds, clearNotificationAfter(10*time.Second))
		}
		return a, tea.Batch(cmds...)

	case cancelClaudeTaskMsg:
		if a.taskManager.KillTask(msg.issueKey) {
			a.notification = fmt.Sprintf("[%s] Task cancelled", msg.issueKey)
			cmds = append(cmds, clearNotificationAfter(5*time.Second))
		} else {
			a.errMsg = fmt.Sprintf("[%s] No running task to cancel", msg.issueKey)
			cmds = append(cmds, clearErrAfter(3*time.Second))
		}
		return a, tea.Batch(cmds...)

	case clearNotificationMsg:
		a.notification = ""
		return a, nil

	case navigateToTaskViewerMsg:
		a.viewStack = append(a.viewStack, a.activeView)
		a.activeView = viewTaskViewer
		a.taskViewer = NewTaskViewerModel(a.taskManager, a.width, a.height-2)
		return a, nil
	}

	// Delegate to active view
	var cmd tea.Cmd
	switch a.activeView {
	case viewDashboard:
		a.dashboard, cmd = a.dashboard.Update(msg, a.client)
		cmds = append(cmds, cmd)
	case viewDetail:
		a.detail, cmd = a.detail.Update(msg, a.client)
		cmds = append(cmds, cmd)
	case viewForm:
		a.form, cmd = a.form.Update(msg, a.client)
		cmds = append(cmds, cmd)
	case viewTransition:
		a.transition, cmd = a.transition.Update(msg, a.client)
		cmds = append(cmds, cmd)
	case viewTaskViewer:
		a.taskViewer, cmd = a.taskViewer.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a App) View() string {
	var content string
	switch a.activeView {
	case viewDashboard:
		content = a.dashboard.View()
	case viewDetail:
		content = a.detail.View()
	case viewForm:
		content = a.form.View()
	case viewTaskViewer:
		content = a.taskViewer.View()
	case viewTransition:
		// Render transition overlay on top of the previous view
		var bg string
		if len(a.viewStack) > 0 {
			switch a.viewStack[len(a.viewStack)-1] {
			case viewDashboard:
				bg = a.dashboard.View()
			case viewDetail:
				bg = a.detail.View()
			default:
				bg = a.dashboard.View()
			}
		} else {
			bg = a.dashboard.View()
		}
		_ = bg
		content = a.transition.View()
	}

	// Status bar at the bottom
	var statusBar string
	if a.errMsg != "" {
		statusBar = errorStyle.Render(fmt.Sprintf(" Error: %s ", a.errMsg))
	} else if a.notification != "" {
		statusBar = successStyle.Render(fmt.Sprintf(" %s ", a.notification))
	} else {
		statusBar = a.helpBar()
	}

	// Compose full view
	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

func (a App) refreshDashboard() tea.Cmd {
	if a.dashboard.viewingEpic != "" {
		return fetchEpicChildren(a.client, a.dashboard.viewingEpic)
	}
	return fetchIssues(a.client, a.dashboard.jql, 50)
}

func (a App) helpBar() string {
	var prefix string
	if count := a.taskManager.RunningCount(); count > 0 {
		prefix = lipgloss.NewStyle().Foreground(colorYellow).Bold(true).
			Render(fmt.Sprintf(" [%d task(s) running] ", count))
	}

	var bar string
	switch a.activeView {
	case viewDashboard:
		if a.dashboard.promptMode == promptClaude {
			return prefix + helpBarStyle.Render(" enter:new line  ctrl+s:submit  esc:cancel")
		}
		if a.dashboard.promptMode != promptNone {
			return prefix + helpBarStyle.Render(" enter:confirm  esc:cancel")
		}
		base := " enter:view  o:open  x:epic  C:claude  T:tasks  c:create  e:edit  t:transition  s:start  d:done  g:grab  r:refresh  q:quit"
		if a.dashboard.viewingEpic != "" {
			base = " enter:view  m:my tickets  a:show/hide closed  C:claude  T:tasks  o:open  x:epic  e:edit  t:transition  s:start  d:done  g:grab  r:refresh  q:quit"
		}
		bar = helpBarStyle.Render(base)
	case viewDetail:
		bar = helpBarStyle.Render(" j/k:scroll  e:edit  t:transition  c:comment  C:claude  g:grab  u:back  q:quit")
	case viewForm:
		bar = helpBarStyle.Render(" tab:next field  shift+tab:prev  ctrl+s:submit  esc:cancel")
	case viewTransition:
		bar = helpBarStyle.Render(" j/k:navigate  enter:select  u:back")
	case viewTaskViewer:
		switch a.taskViewer.mode {
		case taskViewList:
			bar = helpBarStyle.Render(" j/k:navigate  enter:view output  l:live logs  f:files  K:kill  x:clear error  X:clear all errors  r:refresh  u:back")
		case taskViewFiles:
			bar = helpBarStyle.Render(" j/k:navigate  enter:view  x:delete file  X:delete all  r:refresh  u:back")
		case taskViewLogs:
			bar = helpBarStyle.Render(" j/k:scroll  r:refresh logs  u:back")
		default:
			bar = helpBarStyle.Render(" j/k:scroll  u:back")
		}
	}
	return prefix + bar
}
