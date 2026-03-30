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

	err    error
	errMsg string
}

// NewApp creates a new App model.
func NewApp(client *jira.Client, initialJQL string) App {
	return App{
		client:     client,
		activeView: viewDashboard,
		dashboard:  NewDashboardModel(initialJQL),
	}
}

// Run launches the Bubble Tea program.
func Run(client *jira.Client, initialJQL string) error {
	p := tea.NewProgram(NewApp(client, initialJQL), tea.WithAltScreen())
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
	switch a.activeView {
	case viewDashboard:
		if a.dashboard.promptMode != promptNone {
			return helpBarStyle.Render(" enter:confirm  esc:cancel")
		}
		base := " enter:view  o:open  x:epic  c:create  e:edit  t:transition  s:start  d:done  g:grab  r:refresh  /:filter  q:quit"
		if a.dashboard.viewingEpic != "" {
			base = " enter:view  m:my tickets  a:show/hide closed  o:open  x:epic  e:edit  t:transition  s:start  d:done  g:grab  r:refresh  q:quit"
		}
		return helpBarStyle.Render(base)
	case viewDetail:
		return helpBarStyle.Render(" j/k:scroll  e:edit  t:transition  c:comment  g:grab  u:back  q:quit")
	case viewForm:
		return helpBarStyle.Render(" tab:next field  shift+tab:prev  ctrl+s:submit  esc:cancel")
	case viewTransition:
		return helpBarStyle.Render(" j/k:navigate  enter:select  u:back")
	}
	return ""
}
