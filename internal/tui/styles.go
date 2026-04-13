package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	colorCyan    = lipgloss.Color("51")
	colorYellow  = lipgloss.Color("226")
	colorGreen   = lipgloss.Color("46")
	colorRed     = lipgloss.Color("196")
	colorBlue    = lipgloss.Color("33")
	colorMagenta = lipgloss.Color("201")
	colorGray    = lipgloss.Color("241")
	colorWhite   = lipgloss.Color("255")
)

// Layout styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorYellow)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	helpBarStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorCyan).
			Padding(1, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
)

// StatusStyle returns a lipgloss style for a given Jira status name.
func StatusStyle(status string) lipgloss.Style {
	switch strings.ToLower(status) {
	case "open", "to do", "new", "backlog":
		return lipgloss.NewStyle().Foreground(colorRed)
	case "in progress", "in development":
		return lipgloss.NewStyle().Foreground(colorYellow)
	case "done", "closed", "resolved":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "in review", "code review", "in validation":
		return lipgloss.NewStyle().Foreground(colorCyan)
	default:
		return lipgloss.NewStyle().Foreground(colorWhite)
	}
}

// IssueTypeStyle returns a lipgloss style for a given Jira issue type name.
func IssueTypeStyle(issueType string) lipgloss.Style {
	switch strings.ToLower(issueType) {
	case "epic":
		return lipgloss.NewStyle().Foreground(colorMagenta)
	case "bug":
		return lipgloss.NewStyle().Foreground(colorRed)
	case "story":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "task":
		return lipgloss.NewStyle().Foreground(colorBlue)
	case "sub-task", "subtask":
		return lipgloss.NewStyle().Foreground(colorGray)
	default:
		return lipgloss.NewStyle().Foreground(colorWhite)
	}
}

// PriorityStyle returns a lipgloss style for a given Jira priority name.
func PriorityStyle(priority string) lipgloss.Style {
	switch strings.ToLower(priority) {
	case "highest", "critical":
		return lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	case "high":
		return lipgloss.NewStyle().Foreground(colorRed)
	case "medium":
		return lipgloss.NewStyle().Foreground(colorYellow)
	case "low":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "lowest":
		return lipgloss.NewStyle().Foreground(colorGray)
	default:
		return lipgloss.NewStyle().Foreground(colorWhite)
	}
}
