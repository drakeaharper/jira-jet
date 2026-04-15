package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jet/internal/jira"
)

const stateFile = ".jet/tasks/state.json"

// GlobalWorkflowDir returns the global workflow directory (~/.jet/workflows/).
// Falls back to local .jet/workflows if the home directory cannot be resolved.
func GlobalWorkflowDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".jet/workflows"
	}
	return filepath.Join(home, ".jet", "workflows")
}

// Workflow represents a discovered workflow file from ~/.jet/workflows/.
type Workflow struct {
	Name    string // filename without .md extension
	Path    string // full path to the .md file
	Content string // raw file content
}

// DiscoverWorkflows finds all .md workflow files in ~/.jet/workflows/.
// Returns an empty slice (not error) if the directory does not exist.
func DiscoverWorkflows() ([]Workflow, error) {
	dir := GlobalWorkflowDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var workflows []Workflow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read workflow file %s: %v\n", path, err)
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		workflows = append(workflows, Workflow{
			Name:    name,
			Path:    path,
			Content: string(content),
		})
	}

	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows, nil
}

// MigrateLocalWorkflows copies workflow files from local .jet/workflows/ to the
// global ~/.jet/workflows/ directory. Existing global files are not overwritten.
// Returns the names of successfully migrated workflows.
func MigrateLocalWorkflows() ([]string, error) {
	const localDir = ".jet/workflows"
	entries, err := os.ReadDir(localDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	globalDir := GlobalWorkflowDir()
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return nil, err
	}

	var migrated []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		dest := filepath.Join(globalDir, e.Name())
		if _, err := os.Stat(dest); err == nil {
			continue // global file already exists, skip
		}
		src := filepath.Join(localDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			continue
		}
		migrated = append(migrated, strings.TrimSuffix(e.Name(), ".md"))
	}

	return migrated, nil
}

// TaskStatus represents the lifecycle state of a background task.
type TaskStatus int

const (
	TaskRunning TaskStatus = iota
	TaskCompleted
	TaskFailed
)

// ClaudeOutput is the structured output from the claude CLI.
type ClaudeOutput struct {
	Plan         string   `json:"plan"`
	Steps        []string `json:"steps"`
	Conclusion   string   `json:"conclusion"`
	FilesChanged []string `json:"files_changed"`
	PRUrl        string   `json:"pr_url"`
	Repo         string   `json:"repo"`
	WorktreePath string   `json:"worktree_path"`
}

// claudeResponse is the top-level --output-format json envelope.
type claudeResponse struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	DurationMs       int64           `json:"duration_ms"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
}

// Task represents a single background Claude task.
type Task struct {
	IssueKey     string
	Summary      string
	Status       TaskStatus
	StartedAt    time.Time
	EndedAt      time.Time
	Output       *ClaudeOutput
	Error        error
	OutputFile   string
	Cost         float64
	Duration     time.Duration
	Branch       string
	WorktreePath string
	PRCreated    bool
	PID          int
	Reconnected  bool // true if this task was recovered from a previous session

	// For cancellation and live output
	cancel context.CancelFunc
	stderr *bytes.Buffer
	mu     sync.Mutex // protects stderr reads
}

// Elapsed returns how long the task has been running.
func (t *Task) Elapsed() time.Duration {
	if t.Status == TaskRunning {
		return time.Since(t.StartedAt)
	}
	return t.Duration
}

// StderrSnapshot returns the current stderr output (safe for concurrent reads).
func (t *Task) StderrSnapshot() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stderr == nil {
		return ""
	}
	return t.stderr.String()
}

// TaskManager tracks all running and completed background tasks.
type TaskManager struct {
	mu      sync.RWMutex
	tasks   []*Task
	program *tea.Program
}

// persistedTask is the JSON-serializable form of a task for state.json.
type persistedTask struct {
	IssueKey     string        `json:"issue_key"`
	Summary      string        `json:"summary"`
	Status       string        `json:"status"`
	StartedAt    time.Time     `json:"started_at"`
	EndedAt      time.Time     `json:"ended_at,omitempty"`
	OutputFile   string        `json:"output_file,omitempty"`
	Cost         float64       `json:"cost,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	Branch       string        `json:"branch"`
	PRCreated    bool          `json:"pr_created,omitempty"`
	PRUrl        string        `json:"pr_url,omitempty"`
	PID          int           `json:"pid,omitempty"`
	ErrorMsg     string        `json:"error,omitempty"`
}

// NewTaskManager creates a new task manager and loads persisted state.
func NewTaskManager() *TaskManager {
	tm := &TaskManager{}
	tm.loadState()
	return tm
}

// SetProgram sets the tea.Program reference for sending messages from goroutines.
func (tm *TaskManager) SetProgram(p *tea.Program) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.program = p
}

// Tasks returns a copy of the task list.
func (tm *TaskManager) Tasks() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]*Task, len(tm.tasks))
	copy(result, tm.tasks)
	return result
}

// RunningCount returns the number of currently running tasks.
func (tm *TaskManager) RunningCount() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	count := 0
	for _, t := range tm.tasks {
		if t.Status == TaskRunning {
			count++
		}
	}
	return count
}

// KillTask cancels a running task by issue key.
func (tm *TaskManager) KillTask(issueKey string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, t := range tm.tasks {
		if t.IssueKey == issueKey && t.Status == TaskRunning {
			if t.cancel != nil {
				// Live task from this session — use context cancellation
				t.cancel()
				return true
			}
			if t.PID > 0 && t.Reconnected {
				// Reconnected task — kill by PID
				if proc, err := os.FindProcess(t.PID); err == nil {
					proc.Signal(syscall.SIGTERM)
					t.Status = TaskFailed
					t.Error = fmt.Errorf("task killed by user (PID %d)", t.PID)
					t.EndedAt = time.Now()
					t.Duration = t.EndedAt.Sub(t.StartedAt)
					go tm.saveState()
					return true
				}
			}
		}
	}
	return false
}

// IsRunning checks if a task is already running for the given issue key.
func (tm *TaskManager) IsRunning(issueKey string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, t := range tm.tasks {
		if t.IssueKey == issueKey && t.Status == TaskRunning {
			return true
		}
	}
	return false
}

// loadState reads persisted task state from disk and recovers tasks.
func (tm *TaskManager) loadState() {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return // no state file, fresh start
	}

	var persisted []persistedTask
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}

	for _, pt := range persisted {
		task := &Task{
			IssueKey:   pt.IssueKey,
			Summary:    pt.Summary,
			StartedAt:  pt.StartedAt,
			EndedAt:    pt.EndedAt,
			OutputFile: pt.OutputFile,
			Cost:       pt.Cost,
			Duration:   pt.Duration,
			Branch:     pt.Branch,
			PRCreated:  pt.PRCreated,
			PID:        pt.PID,
		}

		if pt.ErrorMsg != "" {
			task.Error = fmt.Errorf("%s", pt.ErrorMsg)
		}

		switch pt.Status {
		case "completed":
			task.Status = TaskCompleted
		case "failed":
			task.Status = TaskFailed
		case "running":
			// Check if the process is still alive
			if pt.PID > 0 && processAlive(pt.PID) {
				task.Status = TaskRunning
				task.Reconnected = true
			} else {
				// Process is gone — check if output file exists
				if pt.OutputFile != "" {
					if _, err := os.Stat(pt.OutputFile); err == nil {
						task.Status = TaskCompleted
					} else {
						task.Status = TaskFailed
						task.Error = fmt.Errorf("process exited (previous session)")
					}
				} else {
					task.Status = TaskFailed
					task.Error = fmt.Errorf("process exited (previous session)")
				}
				task.EndedAt = time.Now()
				task.Duration = task.EndedAt.Sub(task.StartedAt)
			}
		default:
			task.Status = TaskFailed
		}

		tm.tasks = append(tm.tasks, task)
	}
}

// saveState persists the current task list to disk.
func (tm *TaskManager) saveState() {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	persisted := make([]persistedTask, len(tm.tasks))
	for i, t := range tm.tasks {
		pt := persistedTask{
			IssueKey:   t.IssueKey,
			Summary:    t.Summary,
			StartedAt:  t.StartedAt,
			EndedAt:    t.EndedAt,
			OutputFile: t.OutputFile,
			Cost:       t.Cost,
			Duration:   t.Duration,
			Branch:     t.Branch,
			PRCreated:  t.PRCreated,
			PID:        t.PID,
		}

		switch t.Status {
		case TaskRunning:
			pt.Status = "running"
		case TaskCompleted:
			pt.Status = "completed"
		case TaskFailed:
			pt.Status = "failed"
		}

		if t.Error != nil {
			pt.ErrorMsg = t.Error.Error()
		}
		if t.Output != nil && t.Output.PRUrl != "" {
			pt.PRUrl = t.Output.PRUrl
		}

		persisted[i] = pt
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(stateFile), 0755)
	os.WriteFile(stateFile, data, 0644)
}

// processAlive checks if a process with the given PID is still running.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 doesn't kill — just checks if process exists
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// ClearTask removes a specific failed/completed task by issue key.
// Returns false if the task is still running (can't be cleared).
func (tm *TaskManager) ClearTask(issueKey string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tasks {
		if t.IssueKey == issueKey {
			if t.Status == TaskRunning {
				return false
			}
			if t.OutputFile != "" {
				os.Remove(t.OutputFile)
			}
			tm.tasks = append(tm.tasks[:i], tm.tasks[i+1:]...)
			go tm.saveState()
			return true
		}
	}
	return false
}

// ClearNonRunningTasks removes all completed and failed tasks and their output files.
func (tm *TaskManager) ClearNonRunningTasks() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	filtered := tm.tasks[:0]
	for _, t := range tm.tasks {
		if t.Status != TaskRunning {
			if t.OutputFile != "" {
				os.Remove(t.OutputFile)
			}
		} else {
			filtered = append(filtered, t)
		}
	}
	tm.tasks = filtered
	go tm.saveState()
}

// LaunchTask starts a background Claude task for the given issue.
// Claude is responsible for finding the right repo, creating a worktree,
// doing the work, committing, pushing, and creating a PR.
func (tm *TaskManager) LaunchTask(issue *jira.Issue, customInstruction string, workflowContent string) error {
	claudePath, err := findClaudeBinary()
	if err != nil {
		return err
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	branchName := strings.ToLower(issue.Key)

	ctx, cancel := context.WithCancel(context.Background())

	var logBuf bytes.Buffer
	task := &Task{
		IssueKey:  issue.Key,
		Summary:   issue.Fields.Summary,
		Status:    TaskRunning,
		StartedAt: time.Now(),
		Branch:    branchName,
		cancel:    cancel,
		stderr:    &logBuf,
	}

	tm.mu.Lock()
	// Remove any previous completed/failed entries for this issue key
	filtered := tm.tasks[:0]
	for _, t := range tm.tasks {
		if t.IssueKey != issue.Key {
			filtered = append(filtered, t)
		}
	}
	tm.tasks = append(filtered, task)
	program := tm.program
	tm.mu.Unlock()

	prompt := buildPrompt(issue, customInstruction, workDir, workflowContent)
	schema := `{"type":"object","properties":{"plan":{"type":"string","description":"A concise description of what you did"},"steps":{"type":"array","items":{"type":"string"},"description":"Steps you took"},"conclusion":{"type":"string","description":"Summary of what was done and any remaining items"},"files_changed":{"type":"array","items":{"type":"string"},"description":"File paths that were modified"},"pr_url":{"type":"string","description":"URL of the created pull request, empty if not created"},"repo":{"type":"string","description":"Path of the repo you worked in"},"worktree_path":{"type":"string","description":"Path of the worktree you created, empty if not applicable"}},"required":["plan","steps","conclusion","files_changed","pr_url","repo","worktree_path"]}`

	go func() {
		cmd := exec.CommandContext(ctx, claudePath,
			"-p",
			"--verbose",
			"--output-format", "stream-json",
			"--json-schema", schema,
			"--permission-mode", "bypassPermissions",
		)
		cmd.Stdin = strings.NewReader(prompt)
		cmd.Dir = workDir

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			task.Status = TaskFailed
			task.Error = fmt.Errorf("failed to create stdout pipe: %w", err)
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			task.Status = TaskFailed
			task.Error = fmt.Errorf("failed to start claude: %w", err)
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		// Store PID and persist state now that the process is running
		task.PID = cmd.Process.Pid
		tm.saveState()

		// Read streaming JSON events from stdout
		var lastResultLine []byte
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // up to 10MB per line
		for scanner.Scan() {
			line := scanner.Bytes()

			// Format event for live log display
			formatted := formatStreamEvent(line)
			if formatted != "" {
				task.mu.Lock()
				task.stderr.WriteString(formatted)
				task.mu.Unlock()
			}

			// Capture the result event for parsing later
			var peek struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line, &peek) == nil && peek.Type == "result" {
				lastResultLine = make([]byte, len(line))
				copy(lastResultLine, line)
			}
		}

		err = cmd.Wait()

		task.EndedAt = time.Now()
		task.Duration = task.EndedAt.Sub(task.StartedAt)

		if err != nil {
			task.Status = TaskFailed
			if ctx.Err() == context.Canceled {
				task.Error = fmt.Errorf("task cancelled by user")
			} else {
				task.Error = fmt.Errorf("claude exited with error: %w", err)
			}
			writeRawTaskFile(task, stderrBuf.String())
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		if lastResultLine == nil {
			task.Status = TaskFailed
			task.Error = fmt.Errorf("no result event in stream output")
			writeRawTaskFile(task, stderrBuf.String())
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		// Parse the result event (same structure as non-streaming envelope)
		var resp claudeResponse
		if err := json.Unmarshal(lastResultLine, &resp); err != nil {
			task.Status = TaskFailed
			task.Error = fmt.Errorf("failed to parse claude result: %w", err)
			writeRawTaskFile(task, string(lastResultLine))
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		task.Cost = resp.TotalCostUSD

		// Check for API-level errors (e.g. overloaded, rate limited)
		if resp.IsError {
			task.Status = TaskFailed
			task.Error = fmt.Errorf("API error: %s", resp.Result)
			writeRawTaskFile(task, string(lastResultLine))
			tm.saveState()
			if program != nil {
				program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task, err: task.Error})
			}
			return
		}

		// Parse structured output
		var co ClaudeOutput
		if err := json.Unmarshal(resp.StructuredOutput, &co); err != nil {
			co = ClaudeOutput{
				Plan:       resp.Result,
				Conclusion: "Structured output parsing failed; raw result saved.",
			}
		}

		task.Output = &co
		task.Status = TaskCompleted
		if co.PRUrl != "" {
			task.PRCreated = true
		}
		if co.WorktreePath != "" {
			task.WorktreePath = co.WorktreePath
		}

		// Best-effort worktree cleanup if Claude reported one
		if co.WorktreePath != "" && co.Repo != "" {
			remove := exec.Command("git", "worktree", "remove", co.WorktreePath)
			remove.Dir = co.Repo
			remove.Run()
		}

		// Write markdown file
		if err := writeTaskMarkdown(task); err != nil {
			task.Error = fmt.Errorf("output saved but failed to write file: %w", err)
		}

		tm.saveState()

		if program != nil {
			program.Send(claudeTaskDoneMsg{issueKey: task.IssueKey, task: task})
		}
	}()

	return nil
}

// findClaudeBinary locates the claude CLI binary.
func findClaudeBinary() (string, error) {
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".asdf/shims/claude"),
		"/usr/local/bin/claude",
		filepath.Join(home, ".npm/bin/claude"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("claude binary not found in PATH")
}

// buildPrompt constructs the prompt from issue fields.
// If workflowContent is non-empty, it replaces the hardcoded instructions section.
func buildPrompt(issue *jira.Issue, customInstruction string, workDir string, workflowContent string) string {
	branchName := strings.ToLower(issue.Key)
	var b strings.Builder

	if workflowContent != "" {
		b.WriteString(workflowContent)
		b.WriteString("\n\n")
	} else {
		b.WriteString("You are implementing a Jira ticket. Read the ticket context below, ")
		b.WriteString("then make the necessary code changes.\n\n")

		b.WriteString("## Git Workflow\n\n")
		b.WriteString(fmt.Sprintf("Your working directory is `%s`. ", workDir))
		b.WriteString("This directory may contain multiple git repositories as subdirectories. ")
		b.WriteString("First, explore the directory structure and read any relevant README or project files to understand which repo(s) are relevant to this ticket.\n\n")
		b.WriteString("Once you identify the correct repo:\n")
		b.WriteString(fmt.Sprintf("1. `cd` into the repo and create a git worktree: `git worktree add -b %s .jet/worktrees/%s`\n", branchName, branchName))
		b.WriteString(fmt.Sprintf("2. `cd` into the worktree at `.jet/worktrees/%s` and do ALL your work there\n", branchName))
		b.WriteString("3. Implement the changes described in the ticket\n")
		b.WriteString(fmt.Sprintf("4. Stage and commit with a message referencing %s\n", issue.Key))
		b.WriteString("5. Push the branch to the remote\n")
		b.WriteString(fmt.Sprintf("6. Create a pull request using `gh pr create` with the title prefixed by %s\n\n", issue.Key))
		b.WriteString("If the worktree branch already exists, you can use `git worktree add .jet/worktrees/" + branchName + " " + branchName + "` instead.\n")
		b.WriteString("Report the repo path and worktree path in your structured output so it can be cleaned up.\n\n")
	}

	b.WriteString(fmt.Sprintf("## Ticket: %s\n", issue.Key))
	b.WriteString(fmt.Sprintf("**Summary:** %s\n", issue.Fields.Summary))
	b.WriteString(fmt.Sprintf("**Status:** %s\n", issue.Fields.Status.Name))
	b.WriteString(fmt.Sprintf("**Type:** %s\n", issue.Fields.IssueType.Name))
	if issue.Fields.Priority.Name != "" {
		b.WriteString(fmt.Sprintf("**Priority:** %s\n", issue.Fields.Priority.Name))
	}
	b.WriteString(fmt.Sprintf("**Project:** %s (%s)\n", issue.Fields.Project.Name, issue.Fields.Project.Key))

	if len(issue.Fields.Labels) > 0 {
		b.WriteString(fmt.Sprintf("**Labels:** %s\n", strings.Join(issue.Fields.Labels, ", ")))
	}
	if len(issue.Fields.Components) > 0 {
		var names []string
		for _, c := range issue.Fields.Components {
			names = append(names, c.Name)
		}
		b.WriteString(fmt.Sprintf("**Components:** %s\n", strings.Join(names, ", ")))
	}

	if issue.Fields.DescriptionText != "" {
		b.WriteString(fmt.Sprintf("\n## Description\n%s\n", issue.Fields.DescriptionText))
	}

	if len(issue.Fields.IssueLinks) > 0 {
		b.WriteString("\n## Linked Issues\n")
		for _, link := range issue.Fields.IssueLinks {
			if link.OutwardIssue != nil {
				b.WriteString(fmt.Sprintf("- %s %s: %s\n", link.Type.Outward, link.OutwardIssue.Key, link.OutwardIssue.Fields.Summary))
			} else if link.InwardIssue != nil {
				b.WriteString(fmt.Sprintf("- %s %s: %s\n", link.Type.Inward, link.InwardIssue.Key, link.InwardIssue.Fields.Summary))
			}
		}
	}

	comments := issue.Fields.Comment.Comments
	if len(comments) > 0 {
		b.WriteString("\n## Recent Comments\n")
		start := 0
		if len(comments) > 5 {
			start = len(comments) - 5
		}
		for _, c := range comments[start:] {
			author := c.Author.DisplayName
			if author == "" {
				author = c.Author.Name
			}
			b.WriteString(fmt.Sprintf("**%s:** %s\n\n", author, c.Body))
		}
	}

	if customInstruction != "" {
		b.WriteString(fmt.Sprintf("\n## Additional Instructions\n%s\n", customInstruction))
	}

	return b.String()
}

// writeTaskMarkdown writes structured task output to a markdown file.
func writeTaskMarkdown(task *Task) error {
	dir := ".jet/tasks"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%d.md", task.IssueKey, task.StartedAt.Unix())
	task.OutputFile = filepath.Join(dir, filename)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s — Claude Task\n\n", task.IssueKey))
	b.WriteString(fmt.Sprintf("**Summary:** %s\n", task.Summary))
	b.WriteString(fmt.Sprintf("**Branch:** `%s`\n", task.Branch))
	b.WriteString(fmt.Sprintf("**Started:** %s\n", task.StartedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Duration:** %s\n", task.Duration.Round(time.Second)))
	b.WriteString(fmt.Sprintf("**Cost:** $%.4f\n", task.Cost))
	if task.Output != nil && task.Output.PRUrl != "" {
		b.WriteString(fmt.Sprintf("**PR:** %s\n", task.Output.PRUrl))
	}
	b.WriteString("\n")

	if task.Output != nil {
		b.WriteString("## Plan\n\n")
		b.WriteString(task.Output.Plan + "\n\n")

		if len(task.Output.Steps) > 0 {
			b.WriteString("## Steps\n\n")
			for i, step := range task.Output.Steps {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
			}
			b.WriteString("\n")
		}

		if len(task.Output.FilesChanged) > 0 {
			b.WriteString("## Files Changed\n\n")
			for _, f := range task.Output.FilesChanged {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			b.WriteString("\n")
		}

		b.WriteString("## Conclusion\n\n")
		b.WriteString(task.Output.Conclusion + "\n")
	}

	return os.WriteFile(task.OutputFile, []byte(b.String()), 0644)
}

// writeRawTaskFile saves raw output when structured parsing fails.
func writeRawTaskFile(task *Task, raw string) {
	dir := ".jet/tasks"
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%s-%d-raw.md", task.IssueKey, task.StartedAt.Unix())
	task.OutputFile = filepath.Join(dir, filename)

	content := fmt.Sprintf("# %s — Claude Task (Raw Output)\n\n**Warning:** Structured output parsing failed.\n\n```\n%s\n```\n", task.IssueKey, raw)
	os.WriteFile(task.OutputFile, []byte(content), 0644)
}

// streamContentBlock represents a content block in a streaming assistant message.
type streamContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// formatStreamEvent converts a stream-json line into a human-readable log entry.
func formatStreamEvent(line []byte) string {
	var event struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype,omitempty"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message,omitempty"`
	}
	if json.Unmarshal(line, &event) != nil {
		return ""
	}

	switch event.Type {
	case "system":
		if event.Subtype == "init" {
			return "  Session started\n"
		}
	case "assistant":
		var blocks []streamContentBlock
		if json.Unmarshal(event.Message.Content, &blocks) != nil {
			return ""
		}
		var b strings.Builder
		for _, block := range blocks {
			switch block.Type {
			case "text":
				text := strings.TrimSpace(block.Text)
				if text == "" {
					continue
				}
				lines := strings.SplitN(text, "\n", 4)
				if len(lines) > 3 {
					lines = append(lines[:3], "...")
				}
				for _, l := range lines {
					if len(l) > 120 {
						l = l[:120] + "..."
					}
					b.WriteString("  " + l + "\n")
				}
			case "tool_use":
				b.WriteString(fmt.Sprintf("  ► %s\n", formatToolSummary(block.Name, block.Input)))
			}
		}
		return b.String()
	case "result":
		return ""
	}
	return ""
}

// formatToolSummary returns a concise description of a tool invocation.
func formatToolSummary(name string, rawInput json.RawMessage) string {
	var input map[string]interface{}
	if json.Unmarshal(rawInput, &input) != nil {
		return name
	}

	if fp, ok := input["file_path"].(string); ok {
		return fmt.Sprintf("%s %s", name, fp)
	}
	if cmd, ok := input["command"].(string); ok {
		cmd = strings.SplitN(cmd, "\n", 2)[0]
		if len(cmd) > 80 {
			cmd = cmd[:80] + "..."
		}
		return fmt.Sprintf("%s: %s", name, cmd)
	}
	if p, ok := input["pattern"].(string); ok {
		return fmt.Sprintf("%s %s", name, p)
	}
	if p, ok := input["path"].(string); ok {
		return fmt.Sprintf("%s %s", name, p)
	}
	return name
}
