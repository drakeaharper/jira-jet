package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

// Row kinds for the standup view.
const (
	rowSectionHeader = iota
	rowDateHeader
	rowIssue
	rowEmpty
)

type standupRow struct {
	kind  int
	label string
	issue *jira.Issue
}

// StandupModel displays a standup report with completed and in-progress tickets.
type StandupModel struct {
	rows         []standupRow
	cursor       int
	completed    []jira.Issue
	wip          []jira.Issue
	days         int
	loading      bool
	spinner      spinner.Model
	width        int
	height       int
	scrollOffset int
}

// NewStandupModel creates a new standup model.
func NewStandupModel(days int) StandupModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	return StandupModel{
		days:    days,
		loading: true,
		spinner: s,
		cursor:  -1,
	}
}

func (m StandupModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m StandupModel) SetSize(width, height int) StandupModel {
	m.width = width
	m.height = height
	m.ensureVisible()
	return m
}

// SetData stores fetched issues and builds the row layout.
func (m StandupModel) SetData(completed, wip []jira.Issue) StandupModel {
	m.completed = completed
	m.wip = wip
	m.loading = false
	m.buildRows()
	return m
}

func (m *StandupModel) buildRows() {
	m.rows = nil

	// Completed section
	m.rows = append(m.rows, standupRow{
		kind:  rowSectionHeader,
		label: fmt.Sprintf("Completed (%d)", len(m.completed)),
	})

	grouped := groupByDate(m.completed)
	dates := dateRange(m.days)

	for _, dateStr := range dates {
		m.rows = append(m.rows, standupRow{
			kind:  rowDateHeader,
			label: formatDate(dateStr),
		})

		if issues, ok := grouped[dateStr]; ok {
			for i := range issues {
				m.rows = append(m.rows, standupRow{
					kind:  rowIssue,
					issue: &issues[i],
				})
			}
		} else {
			m.rows = append(m.rows, standupRow{
				kind:  rowEmpty,
				label: "No tickets closed",
			})
		}
	}

	// In Progress section
	m.rows = append(m.rows, standupRow{
		kind:  rowSectionHeader,
		label: fmt.Sprintf("In Progress (%d)", len(m.wip)),
	})

	if len(m.wip) > 0 {
		for i := range m.wip {
			m.rows = append(m.rows, standupRow{
				kind:  rowIssue,
				issue: &m.wip[i],
			})
		}
	} else {
		m.rows = append(m.rows, standupRow{
			kind:  rowEmpty,
			label: "None",
		})
	}

	// Set cursor to first issue row
	m.cursor = -1
	for i, row := range m.rows {
		if row.kind == rowIssue {
			m.cursor = i
			break
		}
	}
	m.scrollOffset = 0
	m.ensureVisible()
}

func (m *StandupModel) nextIssueRow() {
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if m.rows[i].kind == rowIssue {
			m.cursor = i
			return
		}
	}
}

func (m *StandupModel) prevIssueRow() {
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].kind == rowIssue {
			m.cursor = i
			return
		}
	}
}

func (m *StandupModel) ensureVisible() {
	if m.cursor < 0 || m.height <= 0 {
		return
	}
	// Each issue row takes 2 visual lines, others take 1, section headers take 2 (blank + header).
	// For simplicity, map cursor row index to approximate visual line.
	visualLine := m.cursorVisualLine()
	visible := m.height - 1 // leave room for bottom
	if visualLine < m.scrollOffset {
		m.scrollOffset = visualLine
	}
	if visualLine >= m.scrollOffset+visible {
		m.scrollOffset = visualLine - visible + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// cursorVisualLine computes the visual line number of the cursor row.
func (m *StandupModel) cursorVisualLine() int {
	line := 0
	for i := 0; i < m.cursor && i < len(m.rows); i++ {
		line += m.rowHeight(i)
	}
	return line
}

func (m *StandupModel) rowHeight(i int) int {
	switch m.rows[i].kind {
	case rowSectionHeader:
		return 2 // blank line + header
	case rowIssue:
		return 2 // key+summary line + details line
	default:
		return 1
	}
}

func (m StandupModel) Update(msg tea.Msg, client *jira.Client) (StandupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "j" || msg.String() == "down":
			m.nextIssueRow()
			m.ensureVisible()
			return m, nil
		case msg.String() == "k" || msg.String() == "up":
			m.prevIssueRow()
			m.ensureVisible()
			return m, nil
		case msg.String() == "enter":
			if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].issue != nil {
				issueKey := m.rows[m.cursor].issue.Key
				return m, func() tea.Msg { return navigateToDetailMsg{key: issueKey} }
			}
		case msg.String() == "r":
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, fetchStandupData(client, m.days))
		case key.Matches(msg, globalKeys.Back):
			return m, func() tea.Msg { return goBackMsg{} }
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m StandupModel) View() string {
	if m.loading && len(m.rows) == 0 {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.spinner.View()+" Loading standup...",
		)
	}

	var b strings.Builder

	// Title
	title := titleStyle.Render("Standup Report")
	b.WriteString(title + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("━", min(m.width, 74))) + "\n")

	// Render rows, applying scroll offset
	visualLine := 0
	visible := m.height - 3 // title + separator + buffer

	for i, row := range m.rows {
		h := m.rowHeight(i)

		// Skip rows before scroll offset
		if visualLine+h <= m.scrollOffset {
			visualLine += h
			continue
		}
		// Stop if past visible area
		if visualLine-m.scrollOffset >= visible {
			break
		}

		switch row.kind {
		case rowSectionHeader:
			icon := "✅"
			style := lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
			if strings.HasPrefix(row.label, "In Progress") {
				icon = "🔄"
				style = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
			}
			b.WriteString("\n")
			b.WriteString(style.Render(fmt.Sprintf("%s %s", icon, row.label)) + "\n")

		case rowDateHeader:
			b.WriteString(headerStyle.Render(fmt.Sprintf("  %s", row.label)) + "\n")

		case rowIssue:
			isSelected := i == m.cursor
			m.renderIssueRow(&b, row.issue, isSelected)

		case rowEmpty:
			b.WriteString(dimStyle.Render(fmt.Sprintf("    %s", row.label)) + "\n")
		}

		visualLine += h
	}

	// Pad remaining height
	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-1 {
		b.WriteString("\n")
		rendered++
	}

	return b.String()
}

func (m StandupModel) renderIssueRow(b *strings.Builder, issue *jira.Issue, selected bool) {
	cursor := "  "
	keyStyle := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	summaryStyle := lipgloss.NewStyle().Foreground(colorWhite)

	if selected {
		cursor = "> "
		bg := lipgloss.Color("236")
		keyStyle = keyStyle.Background(bg)
		summaryStyle = summaryStyle.Background(bg)
	}

	maxSummary := m.width - len(issue.Key) - 8
	summary := issue.Fields.Summary
	if maxSummary > 3 && len(summary) > maxSummary {
		summary = summary[:maxSummary-3] + "..."
	}

	b.WriteString(fmt.Sprintf("    %s%s %s\n", cursor, keyStyle.Render(issue.Key), summaryStyle.Render(summary)))

	// Detail line: status | type
	status := StatusStyle(issue.Fields.Status.Name).Render(issue.Fields.Status.Name)
	parts := []string{status}
	if issue.Fields.IssueType.Name != "" {
		parts = append(parts, IssueTypeStyle(issue.Fields.IssueType.Name).Render(issue.Fields.IssueType.Name))
	}
	b.WriteString(fmt.Sprintf("      %s\n", strings.Join(parts, dimStyle.Render(" | "))))
}

// groupByDate groups issues by the date portion of ResolutionDate.
func groupByDate(issues []jira.Issue) map[string][]jira.Issue {
	grouped := make(map[string][]jira.Issue)
	for _, issue := range issues {
		dateKey := "Unknown"
		if len(issue.Fields.ResolutionDate) >= 10 {
			dateKey = issue.Fields.ResolutionDate[:10]
		}
		grouped[dateKey] = append(grouped[dateKey], issue)
	}
	return grouped
}

// dateRange returns date strings from today back through days, most recent first.
func dateRange(days int) []string {
	today := time.Now()
	dates := make([]string, 0, days+1)
	for i := 0; i <= days; i++ {
		dates = append(dates, today.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	return dates
}

// formatDate converts "2006-01-02" to "January 2, 2006".
func formatDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("January 2, 2006")
}
