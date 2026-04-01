package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type taskViewMode int

const (
	taskViewList taskViewMode = iota
	taskViewOutput
	taskViewLogs
	taskViewFiles
	taskViewFileContent
)

// fileEntry is a discovered output file on disk.
type fileEntry struct {
	path    string
	name    string
	modTime time.Time
	size    int64
}

// TaskViewerModel displays background Claude tasks and their output.
type TaskViewerModel struct {
	taskManager  *TaskManager
	tasks        []*Task
	selected     int
	mode         taskViewMode
	prevListMode taskViewMode // which list we came from (tasks or files)
	viewport     viewport.Model
	width        int
	height       int

	// File browser state
	files         []fileEntry
	fileSelected  int
}

// NewTaskViewerModel creates a new task viewer.
func NewTaskViewerModel(tm *TaskManager, width, height int) TaskViewerModel {
	vp := viewport.New(width, height)
	tv := TaskViewerModel{
		taskManager: tm,
		tasks:       tm.Tasks(),
		viewport:    vp,
		width:       width,
		height:      height,
	}
	tv.files = discoverOutputFiles()
	return tv
}

func (tv TaskViewerModel) SetSize(width, height int) TaskViewerModel {
	tv.width = width
	tv.height = height
	tv.viewport = viewport.New(width, height)
	return tv
}

// discoverOutputFiles scans .jet/tasks/ for all .md files, newest first.
func discoverOutputFiles() []fileEntry {
	entries, err := os.ReadDir(".jet/tasks")
	if err != nil {
		return nil
	}
	var files []fileEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			path:    filepath.Join(".jet/tasks", e.Name()),
			name:    e.Name(),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	// Sort newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

func (tv TaskViewerModel) Update(msg tea.Msg) (TaskViewerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Viewport modes — output, logs, file content
		if tv.mode == taskViewOutput || tv.mode == taskViewLogs || tv.mode == taskViewFileContent {
			switch {
			case key.Matches(msg, globalKeys.Back):
				tv.mode = tv.prevListMode
				return tv, nil
			case msg.String() == "j" || msg.String() == "down":
				tv.viewport.LineDown(1)
				return tv, nil
			case msg.String() == "k" || msg.String() == "up":
				tv.viewport.LineUp(1)
				return tv, nil
			case msg.String() == "r" && tv.mode == taskViewLogs:
				tv.viewport.SetContent(tv.liveLogsContent())
				tv.viewport.GotoBottom()
				return tv, nil
			}
			var cmd tea.Cmd
			tv.viewport, cmd = tv.viewport.Update(msg)
			return tv, cmd
		}

		// File browser list
		if tv.mode == taskViewFiles {
			switch {
			case key.Matches(msg, globalKeys.Back):
				tv.mode = taskViewList
				return tv, nil
			case msg.String() == "j" || msg.String() == "down":
				if tv.fileSelected < len(tv.files)-1 {
					tv.fileSelected++
				}
				return tv, nil
			case msg.String() == "k" || msg.String() == "up":
				if tv.fileSelected > 0 {
					tv.fileSelected--
				}
				return tv, nil
			case msg.String() == "enter":
				if tv.fileSelected < len(tv.files) {
					tv.prevListMode = taskViewFiles
					tv.mode = taskViewFileContent
					tv.viewport = viewport.New(tv.width, tv.height)
					tv.viewport.SetContent(tv.fileContent(tv.files[tv.fileSelected].path))
					return tv, nil
				}
			case msg.String() == "x":
				if tv.fileSelected < len(tv.files) {
					os.Remove(tv.files[tv.fileSelected].path)
					tv.files = discoverOutputFiles()
					if tv.fileSelected >= len(tv.files) {
						tv.fileSelected = max(0, len(tv.files)-1)
					}
				}
				return tv, nil
			case msg.String() == "X":
				for _, f := range tv.files {
					os.Remove(f.path)
				}
				tv.files = discoverOutputFiles()
				tv.fileSelected = 0
				return tv, nil
			case msg.String() == "r":
				tv.files = discoverOutputFiles()
				if tv.fileSelected >= len(tv.files) {
					tv.fileSelected = max(0, len(tv.files)-1)
				}
				return tv, nil
			}
			return tv, nil
		}

		// Task list
		switch {
		case key.Matches(msg, globalKeys.Back):
			return tv, func() tea.Msg { return goBackMsg{} }
		case msg.String() == "j" || msg.String() == "down":
			if tv.selected < len(tv.tasks)-1 {
				tv.selected++
			}
			return tv, nil
		case msg.String() == "k" || msg.String() == "up":
			if tv.selected > 0 {
				tv.selected--
			}
			return tv, nil
		case msg.String() == "enter":
			if tv.selected < len(tv.tasks) {
				task := tv.tasks[tv.selected]
				if task.OutputFile != "" {
					tv.prevListMode = taskViewList
					tv.mode = taskViewOutput
					tv.viewport = viewport.New(tv.width, tv.height)
					tv.viewport.SetContent(tv.taskOutputContent())
					return tv, nil
				}
				// Running task with no output file yet — show live logs
				if task.Status == TaskRunning {
					tv.prevListMode = taskViewList
					tv.mode = taskViewLogs
					tv.viewport = viewport.New(tv.width, tv.height)
					tv.viewport.SetContent(tv.liveLogsContent())
					tv.viewport.GotoBottom()
					return tv, nil
				}
			}
		case msg.String() == "l":
			if tv.selected < len(tv.tasks) {
				tv.prevListMode = taskViewList
				tv.mode = taskViewLogs
				tv.viewport = viewport.New(tv.width, tv.height)
				tv.viewport.SetContent(tv.liveLogsContent())
				tv.viewport.GotoBottom()
				return tv, nil
			}
		case msg.String() == "K":
			if tv.selected < len(tv.tasks) {
				task := tv.tasks[tv.selected]
				if task.Status == TaskRunning {
					issueKey := task.IssueKey
					return tv, func() tea.Msg { return cancelClaudeTaskMsg{issueKey: issueKey} }
				}
			}
			return tv, nil
		case msg.String() == "x":
			if tv.selected < len(tv.tasks) {
				task := tv.tasks[tv.selected]
				if task.Status == TaskFailed {
					tv.taskManager.ClearTask(task.IssueKey)
					tv.tasks = tv.taskManager.Tasks()
					if tv.selected >= len(tv.tasks) {
						tv.selected = max(0, len(tv.tasks)-1)
					}
				}
			}
			return tv, nil
		case msg.String() == "X":
			tv.taskManager.ClearFailedTasks()
			tv.tasks = tv.taskManager.Tasks()
			tv.selected = 0
			return tv, nil
		case msg.String() == "f":
			tv.mode = taskViewFiles
			tv.files = discoverOutputFiles()
			tv.fileSelected = 0
			return tv, nil
		case msg.String() == "r":
			tv.tasks = tv.taskManager.Tasks()
			return tv, nil
		}

	case claudeTaskDoneMsg:
		tv.tasks = tv.taskManager.Tasks()
		tv.files = discoverOutputFiles()
		return tv, nil
	}

	return tv, nil
}

func (tv TaskViewerModel) taskOutputContent() string {
	if tv.selected >= len(tv.tasks) {
		return ""
	}
	task := tv.tasks[tv.selected]
	if task.OutputFile == "" {
		return dimStyle.Render("No output file available.")
	}
	return tv.fileContent(task.OutputFile)
}

func (tv TaskViewerModel) fileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return errorStyle.Render(fmt.Sprintf("Error reading file: %s", err))
	}
	return string(data)
}

func (tv TaskViewerModel) liveLogsContent() string {
	if tv.selected >= len(tv.tasks) {
		return ""
	}
	task := tv.tasks[tv.selected]

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("  Logs: %s", task.IssueKey)) + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Status: %s  Elapsed: %s",
		taskStatusString(task.Status),
		task.Elapsed().Round(time.Second))) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(tv.width, 60))) + "\n\n")

	stderr := task.StderrSnapshot()
	if stderr == "" {
		b.WriteString(dimStyle.Render("  No output yet...") + "\n")
	} else {
		b.WriteString(stderr)
	}

	return b.String()
}

func taskStatusString(s TaskStatus) string {
	switch s {
	case TaskRunning:
		return "running"
	case TaskCompleted:
		return "completed"
	case TaskFailed:
		return "failed"
	}
	return "unknown"
}

func (tv TaskViewerModel) View() string {
	// Viewport modes
	if tv.mode == taskViewOutput || tv.mode == taskViewLogs || tv.mode == taskViewFileContent {
		return tv.viewport.View()
	}

	// File browser
	if tv.mode == taskViewFiles {
		return tv.filesView()
	}

	// Task list
	return tv.tasksView()
}

func (tv TaskViewerModel) filesView() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  Output Files") + "  " + dimStyle.Render("(.jet/tasks/)") + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(tv.width, 60))) + "\n\n")

	if len(tv.files) == 0 {
		b.WriteString(dimStyle.Render("  No output files found in .jet/tasks/") + "\n")
		return b.String()
	}

	for i, f := range tv.files {
		cursor := "  "
		if i == tv.fileSelected {
			cursor = "> "
		}

		nameStyle := valueStyle
		if i == tv.fileSelected {
			nameStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
		}

		sizeStr := formatFileSize(f.size)
		timeStr := f.modTime.Format("2006-01-02 15:04")

		b.WriteString(fmt.Sprintf("%s%s\n", cursor, nameStyle.Render(f.name)))
		b.WriteString(fmt.Sprintf("    %s\n\n", dimStyle.Render(fmt.Sprintf("%s  %s", timeStr, sizeStr))))
	}

	return b.String()
}

func (tv TaskViewerModel) tasksView() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  Claude Tasks") + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(tv.width, 60))) + "\n\n")

	if len(tv.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks yet. Press Shift+C on a ticket to launch one.") + "\n")
		b.WriteString(dimStyle.Render("  Press f to browse output files.") + "\n")
		return b.String()
	}

	for i, task := range tv.tasks {
		cursor := "  "
		if i == tv.selected {
			cursor = "> "
		}

		var icon string
		var iconStyle lipgloss.Style
		switch task.Status {
		case TaskRunning:
			icon = "●"
			iconStyle = lipgloss.NewStyle().Foreground(colorYellow)
		case TaskCompleted:
			icon = "✓"
			iconStyle = lipgloss.NewStyle().Foreground(colorGreen)
		case TaskFailed:
			icon = "✗"
			iconStyle = lipgloss.NewStyle().Foreground(colorRed)
		}

		line := fmt.Sprintf("%s%s %s  %s",
			cursor,
			iconStyle.Render(icon),
			lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(task.IssueKey),
			dimStyle.Render(task.Summary),
		)
		b.WriteString(line + "\n")

		var details []string
		if task.Branch != "" {
			details = append(details, lipgloss.NewStyle().Foreground(colorMagenta).Render(task.Branch))
		}
		if task.Status == TaskRunning {
			elapsed := task.Elapsed().Round(time.Second)
			label := fmt.Sprintf("running %s", elapsed)
			if task.Reconnected {
				label += " (reconnected)"
			}
			details = append(details, lipgloss.NewStyle().Foreground(colorYellow).Render(label))
		} else {
			details = append(details, task.StartedAt.Format("15:04:05"))
			details = append(details, fmt.Sprintf("duration: %s", task.Duration.Round(time.Second)))
		}
		if task.Cost > 0 {
			details = append(details, fmt.Sprintf("$%.4f", task.Cost))
		}
		if task.PRCreated {
			details = append(details, lipgloss.NewStyle().Foreground(colorGreen).Render("PR created"))
		}
		if task.Error != nil {
			details = append(details, errorStyle.Render(task.Error.Error()))
		}
		b.WriteString("    " + dimStyle.Render(strings.Join(details, "  ")) + "\n\n")
	}

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
