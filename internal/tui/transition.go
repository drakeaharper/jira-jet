package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

// transitionItem wraps a jira.Transition for the list.
type transitionItem struct {
	transition jira.Transition
}

func (t transitionItem) FilterValue() string { return t.transition.Name }

// transitionDelegate renders each transition.
type transitionDelegate struct{}

func (d transitionDelegate) Height() int                             { return 1 }
func (d transitionDelegate) Spacing() int                            { return 0 }
func (d transitionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d transitionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	t, ok := item.(transitionItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	name := t.transition.Name
	target := t.transition.To.Name

	cursor := "  "
	style := dimStyle
	if isSelected {
		cursor = "> "
		style = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	}

	fmt.Fprintf(w, "%s%s %s", cursor, style.Render(name), dimStyle.Render("→ "+target))
}

// TransitionModel is the model for the transition picker overlay.
type TransitionModel struct {
	list     list.Model
	issueKey string
	loading  bool
	spinner  spinner.Model
	width    int
	height   int
}

func NewTransitionModel(issueKey string) TransitionModel {
	delegate := transitionDelegate{}
	l := list.New([]list.Item{}, delegate, 50, 10)
	l.Title = fmt.Sprintf("Transition %s", issueKey)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	return TransitionModel{
		list:     l,
		issueKey: issueKey,
		loading:  true,
		spinner:  s,
	}
}

func (t TransitionModel) Init() tea.Cmd {
	return t.spinner.Tick
}

func (t TransitionModel) SetSize(width, height int) TransitionModel {
	t.width = width
	t.height = height
	// Keep the list compact
	listWidth := min(60, width-4)
	listHeight := min(20, height-4)
	t.list.SetSize(listWidth, listHeight)
	return t
}

func (t TransitionModel) SetTransitions(transitions []jira.Transition, issueKey string) TransitionModel {
	items := make([]list.Item, len(transitions))
	for i, tr := range transitions {
		items[i] = transitionItem{transition: tr}
	}
	t.list.SetItems(items)
	t.loading = false
	t.issueKey = issueKey
	return t
}

func (t TransitionModel) Update(msg tea.Msg, client *jira.Client) (TransitionModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, globalKeys.Back):
			return t, func() tea.Msg { return goBackMsg{} }

		case msg.String() == "enter":
			item := t.list.SelectedItem()
			if item != nil {
				tr := item.(transitionItem).transition
				t.loading = true
				return t, transitionIssueCmd(client, t.issueKey, tr.ID, tr.To.Name)
			}
		}

	case spinner.TickMsg:
		if t.loading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(msg)
			cmds = append(cmds, cmd)
			return t, tea.Batch(cmds...)
		}
	}

	var cmd tea.Cmd
	t.list, cmd = t.list.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t TransitionModel) View() string {
	var content string
	if t.loading && len(t.list.Items()) == 0 {
		content = t.spinner.View() + " Loading transitions..."
	} else {
		content = t.list.View()
	}

	box := overlayStyle.Width(min(60, t.width-4)).Render(content)

	return lipgloss.Place(t.width, t.height, lipgloss.Center, lipgloss.Center, box)
}
