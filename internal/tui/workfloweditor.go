package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type chatEntry struct {
	role    string // "user" or "assistant" or "system"
	content string
}

type editorPane int

const (
	paneChat editorPane = iota
	panePreview
)

type editorPhase int

const (
	phaseWorkflowList editorPhase = iota
	phaseRepoPicker
	phaseChat
)

type editorSaveMode int

const (
	saveNone editorSaveMode = iota
	savePrompting
)

// repoEntry represents a discovered git repository.
type repoEntry struct {
	name string // display name (directory basename)
	path string // absolute path
}

// WorkflowEditorModel is the split-screen Claude-assisted workflow creator.
type WorkflowEditorModel struct {
	width  int
	height int
	phase  editorPhase

	// Repo picker state
	repos      []repoEntry
	repoCursor int

	// Selected repo
	repoPath string
	repoName string

	// Chat pane (left)
	chatHistory  []chatEntry
	chatViewport viewport.Model
	chatInput    textarea.Model

	// Preview pane (right)
	previewViewport viewport.Model
	workflowContent string

	// Focus / interaction
	activePane editorPane

	// Pane widths (set by recalcLayout)
	leftWidth  int
	rightWidth int

	// Claude state
	loading      bool
	loadingStart time.Time
	spinner      spinner.Model
	cancelClaude context.CancelFunc

	// Save state
	saveMode      editorSaveMode
	filenameInput textinput.Model

	// Workflow list state
	existingWorkflows []Workflow
	listCursor        int

	// Editing an existing workflow (non-empty = editing, pre-populates save name)
	editingPath string
	editingName string
}

// workflowEditorOutput is the structured output from Claude.
type workflowEditorOutput struct {
	Message         string `json:"message"`
	WorkflowContent string `json:"workflow_content"`
}

func NewWorkflowEditorModel(width, height int) WorkflowEditorModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	chatInput := textarea.New()
	chatInput.Placeholder = "Ask Claude about your workflow..."
	chatInput.SetHeight(3)
	chatInput.ShowLineNumbers = false
	chatInput.CharLimit = 0

	filenameInput := textinput.New()
	filenameInput.Placeholder = "workflow-name"
	filenameInput.CharLimit = 64

	m := WorkflowEditorModel{
		width:         width,
		height:        height,
		spinner:       s,
		chatInput:     chatInput,
		activePane:    paneChat,
		filenameInput: filenameInput,
	}

	// Check for existing workflows first
	existing, _ := DiscoverWorkflows()
	if len(existing) > 0 {
		m.existingWorkflows = existing
		m.phase = phaseWorkflowList
	} else {
		m.initRepoPicker()
	}

	m = m.recalcLayout()
	return m
}

// initRepoPicker sets up the repo picker or skips to chat if only one repo.
func (m *WorkflowEditorModel) initRepoPicker() {
	repos := discoverRepos()
	m.repos = repos

	if len(repos) == 1 {
		m.phase = phaseChat
		m.repoPath = repos[0].path
		m.repoName = repos[0].name
		m.chatHistory = append(m.chatHistory, chatEntry{
			role:    "system",
			content: fmt.Sprintf("Repository: %s\nType a message to start building your workflow.", repos[0].name),
		})
	} else if len(repos) == 0 {
		m.phase = phaseChat
		wd, _ := os.Getwd()
		m.repoPath = wd
		m.repoName = filepath.Base(wd)
		m.chatHistory = append(m.chatHistory, chatEntry{
			role:    "system",
			content: "No git repositories detected. Using current directory.\nType a message to start building your workflow.",
		})
	} else {
		m.phase = phaseRepoPicker
	}
}

// initEditWorkflow sets up the editor with an existing workflow pre-loaded.
func (m *WorkflowEditorModel) initEditWorkflow(wf Workflow) {
	m.editingPath = wf.Path
	m.editingName = wf.Name
	m.workflowContent = wf.Content

	repos := discoverRepos()
	m.repos = repos

	if len(repos) == 1 {
		m.repoPath = repos[0].path
		m.repoName = repos[0].name
	} else if len(repos) == 0 {
		wd, _ := os.Getwd()
		m.repoPath = wd
		m.repoName = filepath.Base(wd)
	}

	m.phase = phaseChat
	m.chatHistory = append(m.chatHistory, chatEntry{
		role:    "system",
		content: fmt.Sprintf("Editing workflow: %s\nThe current content is shown on the right. Chat with Claude to refine it, or ctrl+s to save.", wf.Name),
	})
}

func (m WorkflowEditorModel) Init() tea.Cmd {
	if m.phase == phaseChat {
		return m.chatInput.Focus()
	}
	return nil // phaseWorkflowList and phaseRepoPicker need no init cmd
}

// discoverRepos checks if the current directory is a git repo, and if not,
// checks immediate child directories for git repos.
func discoverRepos() []repoEntry {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}

	// Check if current directory is a git repo
	if isGitRepo(wd) {
		return []repoEntry{{name: filepath.Base(wd), path: wd}}
	}

	// Check immediate children
	entries, err := os.ReadDir(wd)
	if err != nil {
		return nil
	}

	var repos []repoEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childPath := filepath.Join(wd, e.Name())
		if isGitRepo(childPath) {
			repos = append(repos, repoEntry{name: e.Name(), path: childPath})
		}
	}
	return repos
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func (m WorkflowEditorModel) SetSize(width, height int) WorkflowEditorModel {
	m.width = width
	m.height = height
	return m.recalcLayout()
}

func (m WorkflowEditorModel) recalcLayout() WorkflowEditorModel {
	m.leftWidth = m.width/2 - 1 // -1 for separator
	m.rightWidth = m.width - m.leftWidth - 1

	// Chat pane: header(1) + separator(1) + viewport + input(5 = 3 lines + border)
	chatVPHeight := m.height - 7
	if chatVPHeight < 1 {
		chatVPHeight = 1
	}
	m.chatViewport = viewport.New(m.leftWidth, chatVPHeight)
	m.chatViewport.SetContent(m.renderChatHistory())

	m.chatInput.SetWidth(m.leftWidth - 2)

	// Preview pane: header(1) + separator(1) + viewport
	previewVPHeight := m.height - 2
	if previewVPHeight < 1 {
		previewVPHeight = 1
	}
	m.previewViewport = viewport.New(m.rightWidth, previewVPHeight)
	m.previewViewport.SetContent(m.renderPreview())

	return m
}

func (m WorkflowEditorModel) Update(msg tea.Msg) (WorkflowEditorModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Workflow list phase
		if m.phase == phaseWorkflowList {
			switch msg.String() {
			case "esc":
				return m, func() tea.Msg { return goBackMsg{} }
			case "j", "down":
				// +1 for the "Create new" option
				if m.listCursor < len(m.existingWorkflows) {
					m.listCursor++
				}
				return m, nil
			case "k", "up":
				if m.listCursor > 0 {
					m.listCursor--
				}
				return m, nil
			case "enter":
				if m.listCursor < len(m.existingWorkflows) {
					// Edit existing workflow
					m.initEditWorkflow(m.existingWorkflows[m.listCursor])
					m = m.recalcLayout()
					return m, m.chatInput.Focus()
				}
				// "Create new" selected
				m.initRepoPicker()
				m = m.recalcLayout()
				if m.phase == phaseChat {
					return m, m.chatInput.Focus()
				}
				return m, nil
			case "d":
				// Delete workflow
				if m.listCursor < len(m.existingWorkflows) {
					wf := m.existingWorkflows[m.listCursor]
					os.Remove(wf.Path)
					m.existingWorkflows = append(m.existingWorkflows[:m.listCursor], m.existingWorkflows[m.listCursor+1:]...)
					if m.listCursor >= len(m.existingWorkflows)+1 && m.listCursor > 0 {
						m.listCursor--
					}
					// If no workflows left, go to create flow
					if len(m.existingWorkflows) == 0 {
						m.initRepoPicker()
						m = m.recalcLayout()
						if m.phase == phaseChat {
							return m, m.chatInput.Focus()
						}
					}
				}
				return m, nil
			}
			return m, nil
		}

		// Repo picker phase
		if m.phase == phaseRepoPicker {
			switch msg.String() {
			case "esc":
				return m, func() tea.Msg { return goBackMsg{} }
			case "j", "down":
				if m.repoCursor < len(m.repos)-1 {
					m.repoCursor++
				}
				return m, nil
			case "k", "up":
				if m.repoCursor > 0 {
					m.repoCursor--
				}
				return m, nil
			case "enter":
				if m.repoCursor < len(m.repos) {
					m.repoPath = m.repos[m.repoCursor].path
					m.repoName = m.repos[m.repoCursor].name
					m.phase = phaseChat
					m.chatHistory = append(m.chatHistory, chatEntry{
						role:    "system",
						content: fmt.Sprintf("Repository: %s\nType a message to start building your workflow.", m.repoName),
					})
					m = m.recalcLayout()
					return m, m.chatInput.Focus()
				}
				return m, nil
			}
			return m, nil
		}

		// Save filename prompt
		if m.saveMode == savePrompting {
			switch msg.String() {
			case "esc":
				m.saveMode = saveNone
				m.filenameInput.Blur()
				return m, m.chatInput.Focus()
			case "enter":
				name := strings.TrimSpace(m.filenameInput.Value())
				if name == "" {
					name = "my-workflow"
				}
				m.saveMode = saveNone
				m.filenameInput.Blur()
				return m, saveWorkflowFile(name, m.workflowContent)
			}
			var cmd tea.Cmd
			m.filenameInput, cmd = m.filenameInput.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "esc":
			if m.cancelClaude != nil {
				m.cancelClaude()
				m.cancelClaude = nil
			}
			return m, func() tea.Msg { return goBackMsg{} }

		case "ctrl+s":
			if m.workflowContent == "" {
				m.chatHistory = append(m.chatHistory, chatEntry{
					role: "system", content: "No workflow content to save yet.",
				})
				m.chatViewport.SetContent(m.renderChatHistory())
				m.chatViewport.GotoBottom()
				return m, nil
			}
			m.saveMode = savePrompting
			if m.editingName != "" {
				m.filenameInput.SetValue(m.editingName)
			} else {
				m.filenameInput.SetValue("")
			}
			m.filenameInput.Focus()
			m.chatInput.Blur()
			return m, m.filenameInput.Focus()

		case "tab":
			if m.activePane == paneChat {
				m.activePane = panePreview
			} else {
				m.activePane = paneChat
			}
			return m, nil

		case "enter":
			if m.activePane == paneChat && !m.loading {
				text := strings.TrimSpace(m.chatInput.Value())
				if text == "" {
					return m, nil
				}
				m.chatInput.Reset()
				m.chatHistory = append(m.chatHistory, chatEntry{role: "user", content: text})
				m.loading = true
				m.loadingStart = time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), ClaudeWorkflowTimeout)
				m.cancelClaude = cancel
				m.chatViewport.SetContent(m.renderChatHistory())
				m.chatViewport.GotoBottom()
				return m, tea.Batch(
					m.spinner.Tick,
					sendWorkflowEditorMessage(ctx, m.chatHistory, m.workflowContent, m.repoPath),
				)
			}
			// In preview pane, enter does nothing special
			return m, nil
		}

		// Scrolling for the active pane
		if m.activePane == panePreview {
			switch msg.String() {
			case "j", "down":
				m.previewViewport.LineDown(1)
				return m, nil
			case "k", "up":
				m.previewViewport.LineUp(1)
				return m, nil
			case "ctrl+d":
				m.previewViewport.HalfViewDown()
				return m, nil
			case "ctrl+u":
				m.previewViewport.HalfViewUp()
				return m, nil
			}
			return m, nil
		}

		// Chat pane: scroll history with ctrl+u/ctrl+d, otherwise input gets keys
		switch msg.String() {
		case "ctrl+d":
			m.chatViewport.HalfViewDown()
			return m, nil
		case "ctrl+u":
			m.chatViewport.HalfViewUp()
			return m, nil
		}

		// Pass to textarea
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd

	case workflowEditorResponseMsg:
		m.loading = false
		m.cancelClaude = nil
		if msg.err != nil {
			m.chatHistory = append(m.chatHistory, chatEntry{
				role: "system", content: fmt.Sprintf("Error: %s", msg.err),
			})
		} else {
			m.chatHistory = append(m.chatHistory, chatEntry{
				role: "assistant", content: msg.chatMessage,
			})
			if msg.workflowContent != "" {
				m.workflowContent = msg.workflowContent
			}
		}
		m.chatViewport.SetContent(m.renderChatHistory())
		m.chatViewport.GotoBottom()
		m.previewViewport.SetContent(m.renderPreview())
		m.previewViewport.GotoTop()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			// Refresh chat to show spinner animation
			m.chatViewport.SetContent(m.renderChatHistory())
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m WorkflowEditorModel) View() string {
	if m.phase == phaseWorkflowList {
		return m.workflowListView()
	}
	if m.phase == phaseRepoPicker {
		return m.repoPickerView()
	}

	// === Left pane: Chat ===
	chatHeaderStyle := dimStyle
	if m.activePane == paneChat {
		chatHeaderStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	}
	chatHeader := chatHeaderStyle.Render("  Chat")
	chatSep := dimStyle.Render(strings.Repeat("─", m.leftWidth))

	inputBorder := lipgloss.NormalBorder()
	inputBorderColor := colorGray
	if m.activePane == paneChat {
		inputBorderColor = colorCyan
	}
	chatInputView := lipgloss.NewStyle().
		Border(inputBorder).
		BorderForeground(inputBorderColor).
		Width(m.leftWidth - 2).
		Render(m.chatInput.View())

	var leftPane string
	if m.saveMode == savePrompting {
		saveLabel := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("Save as: ")
		saveInput := m.filenameInput.View()
		saveSuffix := dimStyle.Render(".md")
		saveHint := dimStyle.Render("  enter: save  esc: cancel")
		saveLine := saveLabel + saveInput + saveSuffix
		leftPane = lipgloss.JoinVertical(lipgloss.Left,
			chatHeader, chatSep,
			m.chatViewport.View(),
			saveLine, saveHint,
		)
	} else {
		leftPane = lipgloss.JoinVertical(lipgloss.Left,
			chatHeader, chatSep,
			m.chatViewport.View(),
			chatInputView,
		)
	}

	// === Right pane: Preview ===
	previewHeaderStyle := dimStyle
	if m.activePane == panePreview {
		previewHeaderStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	}
	previewHeader := previewHeaderStyle.Render("  Workflow Preview")
	previewSep := dimStyle.Render(strings.Repeat("─", m.rightWidth))

	rightPane := lipgloss.JoinVertical(lipgloss.Left,
		previewHeader, previewSep,
		m.previewViewport.View(),
	)

	// Separator column
	sep := lipgloss.NewStyle().Foreground(colorGray).
		Render(strings.Repeat("│\n", m.height))

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.leftWidth).Render(leftPane),
		sep,
		lipgloss.NewStyle().Width(m.rightWidth).Render(rightPane),
	)
}

func (m WorkflowEditorModel) workflowListView() string {
	title := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).
		Render("Workflows")
	var items strings.Builder
	for i, w := range m.existingWorkflows {
		cursor := "  "
		style := dimStyle
		if i == m.listCursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
		}
		items.WriteString(cursor + style.Render(w.Name) + dimStyle.Render(".md") + "\n")
	}
	// "Create new" option at the end
	createIdx := len(m.existingWorkflows)
	cursor := "  "
	style := dimStyle
	if m.listCursor == createIdx {
		cursor = "> "
		style = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	}
	items.WriteString("\n" + cursor + style.Render("+ Create new workflow") + "\n")

	hint := dimStyle.Render("j/k: navigate  enter: edit/create  d: delete  esc: back")
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", items.String(), hint)
	box := overlayStyle.Width(min(60, m.width-4)).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m WorkflowEditorModel) repoPickerView() string {
	title := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).
		Render("Select a repository to build a workflow for:")
	var items strings.Builder
	for i, r := range m.repos {
		cursor := "  "
		style := dimStyle
		if i == m.repoCursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
		}
		items.WriteString(cursor + style.Render(r.name) + "\n")
		items.WriteString("    " + dimStyle.Render(r.path) + "\n")
	}
	hint := dimStyle.Render("j/k: navigate  enter: select  esc: back")
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", items.String(), hint)
	box := overlayStyle.Width(min(70, m.width-4)).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m WorkflowEditorModel) renderChatHistory() string {
	w := m.leftWidth - 2
	if w < 10 {
		w = 10
	}
	var b strings.Builder
	for _, entry := range m.chatHistory {
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
	if m.loading {
		elapsed := time.Since(m.loadingStart).Round(time.Second)
		b.WriteString(fmt.Sprintf("%s Claude is thinking... (%s)\n", m.spinner.View(), elapsed))
	}
	return b.String()
}

func (m WorkflowEditorModel) renderPreview() string {
	w := m.rightWidth - 2
	if w < 10 {
		w = 10
	}
	if m.workflowContent == "" {
		return dimStyle.Render("  No workflow content yet.\n\n  Chat with Claude to generate one.")
	}
	return wrapText(m.workflowContent, w)
}

// wrapText wraps long lines at word boundaries to fit within maxWidth.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= maxWidth {
			result.WriteString(line + "\n")
			continue
		}
		// Wrap at word boundaries
		remaining := line
		for len(remaining) > maxWidth {
			// Find last space within maxWidth
			breakAt := strings.LastIndex(remaining[:maxWidth], " ")
			if breakAt <= 0 {
				// No space found, hard break
				breakAt = maxWidth
			}
			result.WriteString(remaining[:breakAt] + "\n")
			remaining = remaining[breakAt:]
			// Trim leading space from next line
			remaining = strings.TrimLeft(remaining, " ")
		}
		if remaining != "" {
			result.WriteString(remaining + "\n")
		}
	}
	// Remove trailing newline we added to the last line
	s := result.String()
	if strings.HasSuffix(s, "\n") && !strings.HasSuffix(text, "\n") {
		s = s[:len(s)-1]
	}
	return s
}

// sendWorkflowEditorMessage sends a conversational message to Claude with full history.
func sendWorkflowEditorMessage(ctx context.Context, history []chatEntry, currentWorkflow string, repoPath string) tea.Cmd {
	// Capture values for the goroutine
	historyCopy := make([]chatEntry, len(history))
	copy(historyCopy, history)
	wf := currentWorkflow
	rp := repoPath

	return func() tea.Msg {
		claudePath, err := findClaudeBinary()
		if err != nil {
			return workflowEditorResponseMsg{err: fmt.Errorf("claude not found: %w", err)}
		}

		prompt := buildWorkflowEditorPrompt(historyCopy, wf, rp, "")
		return callClaudeForWorkflow(ctx, claudePath, rp, prompt)
	}
}

func buildWorkflowEditorPrompt(history []chatEntry, currentWorkflow, workDir, initialInstruction string) string {
	var b strings.Builder

	b.WriteString("You are a workflow editor assistant for jira-jet. Your job is to help the user ")
	b.WriteString("create a ~/.jet/workflows/*.md file that will serve as instructions for Claude ")
	b.WriteString("when working on Jira tickets in this repository.\n\n")
	b.WriteString("The workflow file you produce will be used as the ENTIRE instructions section of a prompt. ")
	b.WriteString("Ticket context (summary, description, comments, etc.) will be auto-appended separately, ")
	b.WriteString("so do NOT include placeholder ticket fields in the workflow.\n\n")
	b.WriteString(fmt.Sprintf("The repository is at: %s\n\n", workDir))
	b.WriteString("On each turn, respond with structured JSON containing:\n")
	b.WriteString("- \"message\": your conversational reply to the user\n")
	b.WriteString("- \"workflow_content\": the COMPLETE updated workflow markdown (always the full file, not a diff)\n\n")
	b.WriteString("If the user hasn't asked for changes to the workflow, return the current workflow_content unchanged.\n\n")

	if currentWorkflow != "" {
		b.WriteString("## Current workflow content:\n```markdown\n")
		b.WriteString(currentWorkflow)
		b.WriteString("\n```\n\n")
	}

	if len(history) > 0 {
		b.WriteString("## Conversation history:\n")
		// Keep last 20 entries to avoid token limits
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

	if initialInstruction != "" {
		b.WriteString(fmt.Sprintf("User: %s\n", initialInstruction))
	}

	return b.String()
}

func callClaudeForWorkflow(ctx context.Context, claudePath, workDir, prompt string) tea.Msg {
	schema := `{"type":"object","properties":{"message":{"type":"string","description":"Your conversational reply to the user"},"workflow_content":{"type":"string","description":"The complete updated workflow markdown file content"}},"required":["message","workflow_content"]}`

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, claudePath,
		"-p",
		"--output-format", "json",
		"--json-schema", schema,
		"--max-turns", "10",
		"--permission-mode", "bypassPermissions",
	)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = workDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.Canceled {
			return workflowEditorResponseMsg{err: fmt.Errorf("cancelled")}
		}
		if ctx.Err() == context.DeadlineExceeded {
			return workflowEditorResponseMsg{err: fmt.Errorf("timed out after 5 minutes")}
		}
		return workflowEditorResponseMsg{err: fmt.Errorf("claude failed: %w\nstderr: %s", err, stderr.String())}
	}

	var resp claudeResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return workflowEditorResponseMsg{err: fmt.Errorf("failed to parse claude response: %w", err)}
	}

	if resp.IsError {
		return workflowEditorResponseMsg{err: fmt.Errorf("API error: %s", resp.Result)}
	}

	var out workflowEditorOutput
	if err := json.Unmarshal(resp.StructuredOutput, &out); err != nil {
		// Fallback: use the result text as the message
		return workflowEditorResponseMsg{
			chatMessage:     resp.Result,
			workflowContent: "",
		}
	}

	return workflowEditorResponseMsg{
		chatMessage:     out.Message,
		workflowContent: out.WorkflowContent,
	}
}

// saveWorkflowFile writes the workflow content to ~/.jet/workflows/{name}.md.
func saveWorkflowFile(name, content string) tea.Cmd {
	return func() tea.Msg {
		// Sanitize filename
		name = strings.ToLower(name)
		name = strings.ReplaceAll(name, " ", "-")
		// Remove anything that isn't alphanumeric, hyphen, or underscore
		var clean strings.Builder
		for _, r := range name {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				clean.WriteRune(r)
			}
		}
		name = clean.String()
		if name == "" {
			name = "my-workflow"
		}

		dir := GlobalWorkflowDir()
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errMsg{err: fmt.Errorf("failed to create workflows directory: %w", err)}
		}

		path := filepath.Join(dir, name+".md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return errMsg{err: fmt.Errorf("failed to save workflow: %w", err)}
		}

		return workflowSavedMsg{path: path}
	}
}
