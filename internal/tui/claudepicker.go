package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// claudePickerPhase tracks where in the picker flow we are.
type claudePickerPhase int

const (
	pickerInactive claudePickerPhase = iota
	pickerWorkflow
	pickerPrompt
)

// pickerMode distinguishes how the picker labels itself (launch vs resume).
type pickerMode int

const (
	pickerModeLaunch pickerMode = iota
	pickerModeResume
)

// ClaudePicker is the shared workflow + instruction picker used by the
// dashboard, detail, and task viewer screens.
type ClaudePicker struct {
	phase     claudePickerPhase
	mode      pickerMode
	workflows []Workflow
	cursor    int
	selected  string
	prompt    textarea.Model
	targetKey string
	width     int
}

// NewClaudePicker constructs a fresh picker.
func NewClaudePicker() ClaudePicker {
	ta := textarea.New()
	ta.SetHeight(4)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	return ClaudePicker{
		phase:  pickerInactive,
		prompt: ta,
	}
}

// Active reports whether the picker is currently capturing input.
func (p ClaudePicker) Active() bool {
	return p.phase != pickerInactive
}

// InWorkflowPhase reports whether the picker is currently showing the
// workflow list.
func (p ClaudePicker) InWorkflowPhase() bool {
	return p.phase == pickerWorkflow
}

// InPromptPhase reports whether the picker is currently capturing the
// instruction textarea.
func (p ClaudePicker) InPromptPhase() bool {
	return p.phase == pickerPrompt
}

// SetWidth propagates the available width to the textarea.
func (p ClaudePicker) SetWidth(w int) ClaudePicker {
	p.width = w
	if w > 2 {
		p.prompt.SetWidth(w - 2)
	}
	return p
}

// Start begins the picker for a given target. Resume mode skips the workflow
// picker entirely — the original session already carries the workflow's
// intent, so a re-pick is rarely useful. Launch mode skips it only when no
// workflows exist on disk.
func (p ClaudePicker) Start(targetKey string, mode pickerMode) (ClaudePicker, tea.Cmd) {
	p.targetKey = targetKey
	p.mode = mode
	p.cursor = 0
	p.selected = ""
	p.prompt.Reset()

	if mode == pickerModeResume {
		p.workflows = nil
		p.phase = pickerPrompt
		p.prompt.Placeholder = p.promptPlaceholder()
		return p, p.prompt.Focus()
	}

	wfs, _ := DiscoverWorkflows()
	p.workflows = wfs
	if len(wfs) == 0 {
		p.phase = pickerPrompt
		p.prompt.Placeholder = p.promptPlaceholder()
		return p, p.prompt.Focus()
	}
	p.phase = pickerWorkflow
	return p, nil
}

func (p ClaudePicker) promptPlaceholder() string {
	verb := "instructions"
	if p.mode == pickerModeResume {
		verb = "follow-up"
	}
	if p.targetKey != "" {
		return fmt.Sprintf("Additional %s for %s (ctrl+s to submit, esc to cancel)", verb, p.targetKey)
	}
	return fmt.Sprintf("Additional %s (ctrl+s to submit, esc to cancel)", verb)
}

// Height returns the vertical space the picker occupies, for host sizing.
func (p ClaudePicker) Height() int {
	switch p.phase {
	case pickerWorkflow:
		// items + label + hint + padding
		return len(p.workflows) + 3
	case pickerPrompt:
		// label (1) + bordered textarea (height 4 + 2 border) + hint (1)
		return 8
	}
	return 0
}

// reset clears all picker state and returns to inactive.
func (p ClaudePicker) reset() ClaudePicker {
	p.phase = pickerInactive
	p.workflows = nil
	p.cursor = 0
	p.selected = ""
	p.targetKey = ""
	p.prompt.Blur()
	p.prompt.Reset()
	return p
}

// Update advances the picker state machine. When the user submits or cancels,
// the returned cmd produces a claudePickerResultMsg.
func (p ClaudePicker) Update(msg tea.Msg) (ClaudePicker, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Forward non-key messages (e.g., paste, blink) to textarea while in prompt phase
		if p.phase == pickerPrompt {
			var cmd tea.Cmd
			p.prompt, cmd = p.prompt.Update(msg)
			return p, cmd
		}
		return p, nil
	}

	switch p.phase {
	case pickerWorkflow:
		switch keyMsg.String() {
		case "esc":
			target := p.targetKey
			p = p.reset()
			return p, emitPickerResult(claudePickerResultMsg{cancelled: true, targetKey: target})
		case "j", "down":
			if p.cursor < len(p.workflows)-1 {
				p.cursor++
			}
			return p, nil
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case "enter":
			if p.cursor < len(p.workflows) {
				p.selected = p.workflows[p.cursor].Content
			}
			p.phase = pickerPrompt
			p.prompt.Placeholder = p.promptPlaceholder()
			return p, p.prompt.Focus()
		}
		return p, nil

	case pickerPrompt:
		switch keyMsg.String() {
		case "esc":
			target := p.targetKey
			p = p.reset()
			return p, emitPickerResult(claudePickerResultMsg{cancelled: true, targetKey: target})
		case "ctrl+s":
			result := claudePickerResultMsg{
				instruction:     strings.TrimSpace(p.prompt.Value()),
				workflowContent: p.selected,
				targetKey:       p.targetKey,
			}
			p = p.reset()
			return p, emitPickerResult(result)
		case "enter":
			if strings.TrimSpace(p.prompt.Value()) == "" {
				result := claudePickerResultMsg{
					instruction:     "",
					workflowContent: p.selected,
					targetKey:       p.targetKey,
				}
				p = p.reset()
				return p, emitPickerResult(result)
			}
		}
		var cmd tea.Cmd
		p.prompt, cmd = p.prompt.Update(msg)
		return p, cmd
	}

	return p, nil
}

// View renders the picker's current phase.
func (p ClaudePicker) View() string {
	switch p.phase {
	case pickerWorkflow:
		labelText := "Select workflow"
		if p.mode == pickerModeResume {
			labelText = "Select workflow for resume"
		}
		if p.targetKey != "" {
			labelText = fmt.Sprintf("%s for %s:", labelText, p.targetKey)
		} else {
			labelText += ":"
		}
		label := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(labelText)
		var items strings.Builder
		for i, w := range p.workflows {
			cursor := "  "
			style := dimStyle
			if i == p.cursor {
				cursor = "> "
				style = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			}
			items.WriteString(cursor + style.Render(w.Name) + "\n")
		}
		hint := dimStyle.Render("j/k: navigate  enter: select  esc: cancel")
		return lipgloss.JoinVertical(lipgloss.Left, label, items.String(), hint)

	case pickerPrompt:
		labelText := "Claude task"
		if p.mode == pickerModeResume {
			labelText = "Resume Claude session"
		}
		if p.targetKey != "" {
			labelText = fmt.Sprintf("%s for %s:", labelText, p.targetKey)
		} else {
			labelText += ":"
		}
		label := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(labelText)
		taView := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorCyan).
			Render(p.prompt.View())
		hint := dimStyle.Render("ctrl+s: submit  enter: skip  esc: cancel")
		return lipgloss.JoinVertical(lipgloss.Left, label, taView, hint)
	}
	return ""
}

func emitPickerResult(r claudePickerResultMsg) tea.Cmd {
	return func() tea.Msg { return r }
}
