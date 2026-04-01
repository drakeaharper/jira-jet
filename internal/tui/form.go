package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"os/exec"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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

type formPane int

const (
	formPaneChat formPane = iota
	formPaneFields
)

type FormModel struct {
	// Form fields (right pane)
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

	// Split-screen state
	activePane formPane

	// Chat pane (left)
	chatHistory  []chatEntry
	chatViewport viewport.Model
	chatInput    textarea.Model
	leftWidth    int
	rightWidth   int

	// Claude state
	claudeLoading bool
	loadingStart  time.Time
	claudeSpinner spinner.Model
	cancelClaude  context.CancelFunc
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
	da.SetHeight(10)
	da.ShowLineNumbers = false

	chatInput := textarea.New()
	chatInput.Placeholder = "Ask Claude to help fill in fields..."
	chatInput.SetHeight(3)
	chatInput.ShowLineNumbers = false
	chatInput.CharLimit = 0

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	m := FormModel{
		inputs:        inputs,
		descArea:      da,
		editing:       issue,
		activePane:    formPaneFields,
		chatInput:     chatInput,
		claudeSpinner: s,
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

		m.chatHistory = append(m.chatHistory, chatEntry{
			role:    "system",
			content: fmt.Sprintf("Editing %s. Ask Claude to help update fields.", issue.Key),
		})
	} else {
		m.chatHistory = append(m.chatHistory, chatEntry{
			role:    "system",
			content: "Creating a new ticket. Describe what you need, or fill in the fields on the right.",
		})
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
	f.leftWidth = width/2 - 1
	f.rightWidth = width - f.leftWidth - 1
	f.descArea.SetWidth(f.rightWidth - 4)
	f.chatInput.SetWidth(f.leftWidth - 2)

	// Dynamic description height: total height minus fixed form elements
	// header(1) + sep(1) + newline(1) + project(1) + summary(1) + desc label(1) + desc border(2) + spacing(2) + type(1) + epic(1) = 12
	descHeight := height - 12
	if descHeight < 4 {
		descHeight = 4
	}
	f.descArea.SetHeight(descHeight)

	chatVPHeight := height - 7
	if chatVPHeight < 1 {
		chatVPHeight = 1
	}
	f.chatViewport = viewport.New(f.leftWidth, chatVPHeight)
	f.chatViewport.SetContent(f.renderChatHistory())

	return f
}

func (f FormModel) Update(msg tea.Msg, client *jira.Client) (FormModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Tab to switch panes
		if msg.String() == "ctrl+p" {
			if f.activePane == formPaneChat {
				f.activePane = formPaneFields
				f.chatInput.Blur()
				// Re-focus current form field
				if f.focusIndex == fieldCount {
					return f, f.descArea.Focus()
				}
				f.inputs[f.focusIndex].Focus()
				return f, nil
			} else {
				f.activePane = formPaneChat
				// Blur form fields
				if f.focusIndex == fieldCount {
					f.descArea.Blur()
				} else {
					f.inputs[f.focusIndex].Blur()
				}
				return f, f.chatInput.Focus()
			}
		}

		if f.activePane == formPaneChat {
			switch msg.String() {
			case "esc":
				if f.cancelClaude != nil {
					f.cancelClaude()
					f.cancelClaude = nil
				}
				return f, func() tea.Msg { return goBackMsg{} }

			case "enter":
				if !f.claudeLoading {
					text := strings.TrimSpace(f.chatInput.Value())
					if text == "" {
						return f, nil
					}
					f.chatInput.Reset()
					f.chatHistory = append(f.chatHistory, chatEntry{role: "user", content: text})
					f.claudeLoading = true
					f.loadingStart = time.Now()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
					f.cancelClaude = cancel
					f.chatViewport.SetContent(f.renderChatHistory())
					f.chatViewport.GotoBottom()
					return f, tea.Batch(
						f.claudeSpinner.Tick,
						sendFormClaudeMessage(ctx, f.chatHistory, f.currentFields(), f.editing),
					)
				}
				return f, nil

			case "ctrl+d":
				f.chatViewport.HalfViewDown()
				return f, nil
			case "ctrl+u":
				f.chatViewport.HalfViewUp()
				return f, nil
			}

			// Pass to chat textarea
			var cmd tea.Cmd
			f.chatInput, cmd = f.chatInput.Update(msg)
			return f, cmd
		}

		// === Fields pane ===
		switch {
		case msg.String() == "esc":
			if f.focusIndex == fieldCount {
				f.focusIndex = fieldDescription
				f.descArea.Blur()
				f.inputs[fieldDescription].Focus()
				return f, nil
			}
			if f.cancelClaude != nil {
				f.cancelClaude()
				f.cancelClaude = nil
			}
			return f, func() tea.Msg { return goBackMsg{} }

		case key.Matches(msg, formKeys.Submit):
			return f.submit(client)

		case key.Matches(msg, formKeys.NextField):
			return f.nextField()

		case key.Matches(msg, formKeys.PrevField):
			return f.prevField()
		}

		// Update focused component
		if f.focusIndex == fieldCount {
			var cmd tea.Cmd
			f.descArea, cmd = f.descArea.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			f.inputs[f.focusIndex], cmd = f.inputs[f.focusIndex].Update(msg)
			cmds = append(cmds, cmd)
		}

		return f, tea.Batch(cmds...)

	case formClaudeResponseMsg:
		f.claudeLoading = false
		f.cancelClaude = nil
		if msg.err != nil {
			f.chatHistory = append(f.chatHistory, chatEntry{
				role: "system", content: fmt.Sprintf("Error: %s", msg.err),
			})
		} else {
			f.chatHistory = append(f.chatHistory, chatEntry{
				role: "assistant", content: msg.chatMessage,
			})
			// Auto-populate fields from Claude's response
			f.applyClaudeFields(msg.fields)
		}
		f.chatViewport.SetContent(f.renderChatHistory())
		f.chatViewport.GotoBottom()
		return f, nil

	case spinner.TickMsg:
		if f.claudeLoading {
			var cmd tea.Cmd
			f.claudeSpinner, cmd = f.claudeSpinner.Update(msg)
			f.chatViewport.SetContent(f.renderChatHistory())
			cmds = append(cmds, cmd)
		}
	}

	return f, tea.Batch(cmds...)
}

func (f *FormModel) applyClaudeFields(fields formClaudeFields) {
	if fields.Project != "" {
		f.inputs[fieldProject].SetValue(fields.Project)
	}
	if fields.Summary != "" {
		f.inputs[fieldSummary].SetValue(fields.Summary)
	}
	if fields.Description != "" {
		f.descArea.SetValue(fields.Description)
	}
	if fields.IssueType != "" {
		f.inputs[fieldType].SetValue(fields.IssueType)
	}
	if fields.Epic != "" {
		f.inputs[fieldEpic].SetValue(fields.Epic)
	}
}

func (f FormModel) currentFields() formClaudeFields {
	return formClaudeFields{
		Project:     strings.TrimSpace(f.inputs[fieldProject].Value()),
		Summary:     strings.TrimSpace(f.inputs[fieldSummary].Value()),
		Description: strings.TrimSpace(f.descArea.Value()),
		IssueType:   strings.TrimSpace(f.inputs[fieldType].Value()),
		Epic:        strings.TrimSpace(f.inputs[fieldEpic].Value()),
	}
}

func (f FormModel) nextField() (FormModel, tea.Cmd) {
	if f.focusIndex == fieldCount {
		f.descArea.Blur()
	} else {
		f.inputs[f.focusIndex].Blur()
	}

	if f.focusIndex == fieldDescription {
		f.focusIndex = fieldCount
		f.descArea.Focus()
		return f, f.descArea.Focus()
	} else if f.focusIndex == fieldCount {
		f.focusIndex = fieldType
	} else {
		f.focusIndex++
		if f.focusIndex > fieldEpic {
			f.focusIndex = fieldProject
		}
		if f.focusIndex == fieldDescription {
			f.focusIndex = fieldCount
			f.descArea.Focus()
			return f, f.descArea.Focus()
		}
	}

	if f.editing != nil && f.focusIndex == fieldProject {
		f.focusIndex = fieldSummary
	}

	f.inputs[f.focusIndex].Focus()
	return f, nil
}

func (f FormModel) prevField() (FormModel, tea.Cmd) {
	if f.focusIndex == fieldCount {
		f.descArea.Blur()
		f.focusIndex = fieldSummary
		f.inputs[f.focusIndex].Focus()
		return f, nil
	}

	f.inputs[f.focusIndex].Blur()

	if f.focusIndex == fieldType {
		f.focusIndex = fieldCount
		f.descArea.Focus()
		return f, f.descArea.Focus()
	}

	f.focusIndex--
	if f.focusIndex < fieldProject {
		f.focusIndex = fieldEpic
	}

	if f.focusIndex == fieldDescription {
		f.focusIndex = fieldSummary
	}

	if f.editing != nil && f.focusIndex == fieldProject {
		f.focusIndex = fieldEpic
	}

	f.inputs[f.focusIndex].Focus()
	return f, nil
}

func (f FormModel) submit(client *jira.Client) (FormModel, tea.Cmd) {
	if f.editing != nil {
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
	// === Left pane: Chat ===
	chatHeaderStyle := dimStyle
	if f.activePane == formPaneChat {
		chatHeaderStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	}
	chatHeader := chatHeaderStyle.Render("  Claude")
	chatSep := dimStyle.Render(strings.Repeat("─", f.leftWidth))

	inputBorderColor := colorGray
	if f.activePane == formPaneChat {
		inputBorderColor = colorCyan
	}
	chatInputView := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(inputBorderColor).
		Width(f.leftWidth - 2).
		Render(f.chatInput.View())

	leftPane := lipgloss.JoinVertical(lipgloss.Left,
		chatHeader, chatSep,
		f.chatViewport.View(),
		chatInputView,
	)

	// === Right pane: Form fields ===
	formHeaderStyle := dimStyle
	if f.activePane == formPaneFields {
		formHeaderStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	}

	title := "Create Ticket"
	if f.editing != nil {
		title = "Edit " + f.editing.Key
	}
	formHeader := formHeaderStyle.Render("  " + title)
	formSep := dimStyle.Render(strings.Repeat("─", f.rightWidth))

	var formContent strings.Builder
	formContent.WriteString("\n")

	for i := range f.inputs {
		if i == fieldDescription {
			label := "Description: "
			if f.focusIndex == fieldCount && f.activePane == formPaneFields {
				label = lipgloss.NewStyle().Foreground(colorCyan).Render(label)
			} else {
				label = dimStyle.Render(label)
			}
			formContent.WriteString(label + "\n")
			style := lipgloss.NewStyle()
			if f.focusIndex == fieldCount && f.activePane == formPaneFields {
				style = style.Border(lipgloss.NormalBorder()).BorderForeground(colorCyan)
			} else {
				style = style.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("236"))
			}
			formContent.WriteString(style.Render(f.descArea.View()) + "\n\n")
			continue
		}

		if f.editing != nil && i == fieldProject {
			formContent.WriteString(dimStyle.Render(f.inputs[i].Prompt) + dimStyle.Render(f.inputs[i].Value()+" (read-only)") + "\n")
			continue
		}

		formContent.WriteString(f.inputs[i].View() + "\n")
	}

	if f.loading {
		formContent.WriteString("\n" + dimStyle.Render("  Submitting..."))
	}

	rightPane := lipgloss.JoinVertical(lipgloss.Left,
		formHeader, formSep,
		formContent.String(),
	)

	// Separator column
	sep := lipgloss.NewStyle().Foreground(colorGray).
		Render(strings.Repeat("│\n", f.height))

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(f.leftWidth).Render(leftPane),
		sep,
		lipgloss.NewStyle().Width(f.rightWidth).Render(rightPane),
	)
}

func (f FormModel) renderChatHistory() string {
	w := f.leftWidth - 2
	if w < 10 {
		w = 10
	}
	var b strings.Builder
	for _, entry := range f.chatHistory {
		switch entry.role {
		case "user":
			b.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("You") + "\n")
			b.WriteString(wrapText(entry.content, w) + "\n\n")
		case "assistant":
			b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("Claude") + "\n")
			b.WriteString(wrapText(entry.content, w) + "\n\n")
		case "system":
			b.WriteString(dimStyle.Render(wrapText(entry.content, w)) + "\n\n")
		}
	}
	if f.claudeLoading {
		elapsed := time.Since(f.loadingStart).Round(time.Second)
		b.WriteString(fmt.Sprintf("%s Claude is thinking... (%s)\n", f.claudeSpinner.View(), elapsed))
	}
	return b.String()
}

// sendFormClaudeMessage calls Claude to help fill in ticket fields.
func sendFormClaudeMessage(ctx context.Context, history []chatEntry, current formClaudeFields, editing *jira.Issue) tea.Cmd {
	historyCopy := make([]chatEntry, len(history))
	copy(historyCopy, history)
	cur := current
	isEdit := editing != nil
	editKey := ""
	if editing != nil {
		editKey = editing.Key
	}

	return func() tea.Msg {
		claudePath, err := findClaudeBinary()
		if err != nil {
			return formClaudeResponseMsg{err: fmt.Errorf("claude not found: %w", err)}
		}

		prompt := buildFormClaudePrompt(historyCopy, cur, isEdit, editKey)
		return callClaudeForForm(ctx, claudePath, prompt)
	}
}

func buildFormClaudePrompt(history []chatEntry, current formClaudeFields, isEdit bool, editKey string) string {
	var b strings.Builder

	if isEdit {
		b.WriteString(fmt.Sprintf("You are helping edit Jira ticket %s. ", editKey))
	} else {
		b.WriteString("You are helping create a new Jira ticket. ")
	}
	b.WriteString("Based on the user's request, fill in or update the ticket fields.\n\n")
	b.WriteString("Respond with:\n")
	b.WriteString("- \"message\": a brief conversational reply\n")
	b.WriteString("- \"fields\": an object with ticket fields to set. Only include fields you want to change. ")
	b.WriteString("Fields: project (key like PROJ), summary, description, issue_type (Story/Bug/Task/etc), epic (key like PROJ-100)\n\n")
	b.WriteString("For description, write clear acceptance criteria and context. Keep summaries concise.\n\n")

	b.WriteString("## Current field values:\n")
	b.WriteString(fmt.Sprintf("- project: %q\n", current.Project))
	b.WriteString(fmt.Sprintf("- summary: %q\n", current.Summary))
	b.WriteString(fmt.Sprintf("- description: %q\n", current.Description))
	b.WriteString(fmt.Sprintf("- issue_type: %q\n", current.IssueType))
	b.WriteString(fmt.Sprintf("- epic: %q\n\n", current.Epic))

	if len(history) > 0 {
		b.WriteString("## Conversation:\n")
		start := 0
		if len(history) > 20 {
			start = len(history) - 20
		}
		for _, entry := range history[start:] {
			switch entry.role {
			case "user":
				b.WriteString(fmt.Sprintf("User: %s\n\n", entry.content))
			case "assistant":
				b.WriteString(fmt.Sprintf("Assistant: %s\n\n", entry.content))
			}
		}
	}

	return b.String()
}

// formClaudeOutput is the structured output from Claude for form assistance.
type formClaudeOutput struct {
	Message string           `json:"message"`
	Fields  formClaudeFields `json:"fields"`
}

func callClaudeForForm(ctx context.Context, claudePath, prompt string) tea.Msg {
	schema := `{"type":"object","properties":{"message":{"type":"string","description":"Your conversational reply"},"fields":{"type":"object","properties":{"project":{"type":"string"},"summary":{"type":"string"},"description":{"type":"string"},"issue_type":{"type":"string"},"epic":{"type":"string"}},"description":"Ticket fields to set or update. Only include fields you want to change."}},"required":["message","fields"]}`

	workDir, _ := os.Getwd()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, claudePath,
		"-p",
		"--output-format", "json",
		"--json-schema", schema,
		"--max-turns", "3",
	)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = workDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.Canceled {
			return formClaudeResponseMsg{err: fmt.Errorf("cancelled")}
		}
		if ctx.Err() == context.DeadlineExceeded {
			return formClaudeResponseMsg{err: fmt.Errorf("timed out")}
		}
		return formClaudeResponseMsg{err: fmt.Errorf("claude failed: %w", err)}
	}

	var resp claudeResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return formClaudeResponseMsg{err: fmt.Errorf("failed to parse response: %w", err)}
	}

	if resp.IsError {
		return formClaudeResponseMsg{err: fmt.Errorf("API error: %s", resp.Result)}
	}

	var out formClaudeOutput
	if err := json.Unmarshal(resp.StructuredOutput, &out); err != nil {
		return formClaudeResponseMsg{chatMessage: resp.Result}
	}

	return formClaudeResponseMsg{
		chatMessage: out.Message,
		fields:      out.Fields,
	}
}
