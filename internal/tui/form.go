package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

const (
	fieldProject = iota
	fieldSummary
	fieldDescription
	fieldType
	fieldEpic
	fieldCount
)

type FormModel struct {
	inputs     [fieldCount]textinput.Model
	descArea   textarea.Model
	focusIndex int
	editing    *jira.Issue // nil = create, non-nil = edit
	width      int
	height     int
	loading    bool

	// Store original values for edit mode to diff
	origSummary string
	origDesc    string
}

func NewFormModel(issue *jira.Issue) FormModel {
	var inputs [fieldCount]textinput.Model

	for i := range inputs {
		t := textinput.New()
		t.CharLimit = 256
		switch i {
		case fieldProject:
			t.Placeholder = "Project key (e.g. PROJ)"
			t.Prompt = "Project:     "
		case fieldSummary:
			t.Placeholder = "Ticket summary"
			t.Prompt = "Summary:     "
		case fieldDescription:
			t.Placeholder = "(use tab to skip to description area below)"
			t.Prompt = "Description: "
		case fieldType:
			t.Placeholder = "Story"
			t.Prompt = "Type:        "
		case fieldEpic:
			t.Placeholder = "Epic key (optional)"
			t.Prompt = "Epic:        "
		}
		inputs[i] = t
	}

	da := textarea.New()
	da.Placeholder = "Enter description..."
	da.SetHeight(6)
	da.ShowLineNumbers = false

	m := FormModel{
		inputs:   inputs,
		descArea: da,
		editing:  issue,
	}

	if issue != nil {
		// Edit mode: pre-populate
		inputs[fieldProject].SetValue(issue.Fields.Project.Key)
		inputs[fieldSummary].SetValue(issue.Fields.Summary)
		da.SetValue(issue.Fields.DescriptionText)
		inputs[fieldType].SetValue(issue.Fields.IssueType.Name)
		if issue.Fields.EpicLink != nil {
			inputs[fieldEpic].SetValue(issue.Fields.EpicLink.Key)
		}
		m.origSummary = issue.Fields.Summary
		m.origDesc = issue.Fields.DescriptionText
		m.inputs = inputs
		m.descArea = da
	}

	// Focus first editable field
	if issue != nil {
		m.focusIndex = fieldSummary
		m.inputs[fieldSummary].Focus()
	} else {
		m.focusIndex = fieldProject
		m.inputs[fieldProject].Focus()
	}

	return m
}

func (f FormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (f FormModel) SetSize(width, height int) FormModel {
	f.width = width
	f.height = height
	f.descArea.SetWidth(width - 16)
	return f
}

func (f FormModel) Update(msg tea.Msg, client *jira.Client) (FormModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "esc":
			// In the description textarea, esc exits the textarea
			if f.focusIndex == fieldCount {
				f.focusIndex = fieldDescription
				f.descArea.Blur()
				f.inputs[fieldDescription].Focus()
				return f, nil
			}
			// Otherwise, esc leaves the form
			return f, func() tea.Msg { return goBackMsg{} }

		case key.Matches(msg, formKeys.Submit):
			return f.submit(client)

		case key.Matches(msg, formKeys.NextField):
			return f.nextField()

		case key.Matches(msg, formKeys.PrevField):
			return f.prevField()
		}
	}

	// Update focused component
	if f.focusIndex == fieldCount {
		// Description textarea is focused
		var cmd tea.Cmd
		f.descArea, cmd = f.descArea.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		f.inputs[f.focusIndex], cmd = f.inputs[f.focusIndex].Update(msg)
		cmds = append(cmds, cmd)
	}

	return f, tea.Batch(cmds...)
}

func (f FormModel) nextField() (FormModel, tea.Cmd) {
	// Blur current
	if f.focusIndex == fieldCount {
		f.descArea.Blur()
	} else {
		f.inputs[f.focusIndex].Blur()
	}

	// Advance
	if f.focusIndex == fieldDescription {
		// Jump to textarea
		f.focusIndex = fieldCount
		f.descArea.Focus()
		return f, f.descArea.Focus()
	} else if f.focusIndex == fieldCount {
		// Jump from textarea to Type
		f.focusIndex = fieldType
	} else {
		f.focusIndex++
		if f.focusIndex > fieldEpic {
			f.focusIndex = fieldProject
		}
		// Skip description input (we use textarea instead)
		if f.focusIndex == fieldDescription {
			f.focusIndex = fieldCount
			f.descArea.Focus()
			return f, f.descArea.Focus()
		}
	}

	// Skip project field in edit mode
	if f.editing != nil && f.focusIndex == fieldProject {
		f.focusIndex = fieldSummary
	}

	f.inputs[f.focusIndex].Focus()
	return f, nil
}

func (f FormModel) prevField() (FormModel, tea.Cmd) {
	// Blur current
	if f.focusIndex == fieldCount {
		f.descArea.Blur()
		f.focusIndex = fieldSummary
		f.inputs[f.focusIndex].Focus()
		return f, nil
	}

	f.inputs[f.focusIndex].Blur()

	if f.focusIndex == fieldType {
		// Jump back to textarea
		f.focusIndex = fieldCount
		f.descArea.Focus()
		return f, f.descArea.Focus()
	}

	f.focusIndex--
	if f.focusIndex < fieldProject {
		f.focusIndex = fieldEpic
	}

	// Skip description input
	if f.focusIndex == fieldDescription {
		f.focusIndex = fieldSummary
	}

	// Skip project in edit mode
	if f.editing != nil && f.focusIndex == fieldProject {
		f.focusIndex = fieldEpic
	}

	f.inputs[f.focusIndex].Focus()
	return f, nil
}

func (f FormModel) submit(client *jira.Client) (FormModel, tea.Cmd) {
	if f.editing != nil {
		// Edit mode: only send changed fields
		fields := make(map[string]interface{})
		summary := strings.TrimSpace(f.inputs[fieldSummary].Value())
		if summary != "" && summary != f.origSummary {
			fields["summary"] = summary
		}
		desc := strings.TrimSpace(f.descArea.Value())
		if desc != f.origDesc {
			fields["description"] = desc
		}
		if len(fields) == 0 {
			return f, func() tea.Msg { return goBackMsg{} }
		}
		f.loading = true
		return f, updateIssueCmd(client, f.editing.Key, fields)
	}

	// Create mode
	project := strings.TrimSpace(f.inputs[fieldProject].Value())
	summary := strings.TrimSpace(f.inputs[fieldSummary].Value())
	if project == "" || summary == "" {
		return f, func() tea.Msg {
			return errMsg{err: fmt.Errorf("project and summary are required")}
		}
	}

	desc := strings.TrimSpace(f.descArea.Value())
	issueType := strings.TrimSpace(f.inputs[fieldType].Value())
	if issueType == "" {
		issueType = "Story"
	}
	epic := strings.TrimSpace(f.inputs[fieldEpic].Value())

	f.loading = true
	return f, createIssueCmd(client, project, summary, desc, issueType, epic)
}

func (f FormModel) View() string {
	var b strings.Builder

	title := "Create Ticket"
	if f.editing != nil {
		title = "Edit " + f.editing.Key
	}

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  "+title) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(f.width, 60))) + "\n\n")

	for i := range f.inputs {
		if i == fieldDescription {
			// Show label, then textarea below
			label := "Description: "
			if f.focusIndex == fieldCount {
				label = lipgloss.NewStyle().Foreground(colorCyan).Render(label)
			} else {
				label = dimStyle.Render(label)
			}
			b.WriteString(label + "\n")
			style := lipgloss.NewStyle()
			if f.focusIndex == fieldCount {
				style = style.Border(lipgloss.NormalBorder()).BorderForeground(colorCyan)
			} else {
				style = style.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("236"))
			}
			b.WriteString(style.Render(f.descArea.View()) + "\n\n")
			continue
		}

		// In edit mode, show project as read-only
		if f.editing != nil && i == fieldProject {
			b.WriteString(dimStyle.Render(f.inputs[i].Prompt) + dimStyle.Render(f.inputs[i].Value()+" (read-only)") + "\n")
			continue
		}

		b.WriteString(f.inputs[i].View() + "\n")
	}

	if f.loading {
		b.WriteString("\n" + dimStyle.Render("  Submitting..."))
	}

	return b.String()
}
