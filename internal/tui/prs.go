package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/prs"
)

// PRsModel displays open pull requests aggregated across Gerrit and GitHub.
type PRsModel struct {
	scope        string // "mine" or "team"
	prs          []prs.PR
	cursor       int
	loading      bool
	spinner      spinner.Model
	width        int
	height       int
	scrollOffset int
	warnings     []string
}

// NewPRsModel creates a PR view for the given scope ("mine" or "team").
func NewPRsModel(scope string) PRsModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)
	return PRsModel{scope: scope, loading: true, spinner: s}
}

func (m PRsModel) Init() tea.Cmd { return m.spinner.Tick }

func (m PRsModel) SetSize(width, height int) PRsModel {
	m.width = width
	m.height = height
	m.ensureVisible()
	return m
}

// SetData stores fetched PRs and per-source warnings.
func (m PRsModel) SetData(list []prs.PR, warnings []string) PRsModel {
	m.prs = list
	m.warnings = warnings
	m.loading = false
	if m.cursor >= len(list) {
		m.cursor = len(list) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
	return m
}

func (m *PRsModel) ensureVisible() {
	if m.height <= 0 {
		return
	}
	visible := m.visibleRows()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+visible {
		m.scrollOffset = m.cursor - visible + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// visibleRows is how many PR rows fit; each row is 2 lines, minus header (3).
func (m PRsModel) visibleRows() int {
	rows := (m.height - 3 - len(m.warnings)) / 2
	if rows < 1 {
		return 1
	}
	return rows
}

func (m PRsModel) Update(msg tea.Msg, _ interface{}) (PRsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "j" || msg.String() == "down":
			if m.cursor < len(m.prs)-1 {
				m.cursor++
				m.ensureVisible()
			}
			return m, nil
		case msg.String() == "k" || msg.String() == "up":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
			return m, nil
		case msg.String() == "enter" || msg.String() == "o":
			if m.cursor >= 0 && m.cursor < len(m.prs) {
				openURL(m.prs[m.cursor].URL)
			}
			return m, nil
		case msg.String() == "tab":
			// Toggle between mine and team.
			next := "team"
			if m.scope == "team" {
				next = "mine"
			}
			return m, func() tea.Msg { return navigateToPRsMsg{scope: next} }
		case msg.String() == "r":
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, fetchPRs(m.scope))
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

func (m PRsModel) View() string {
	heading := "Your open PRs"
	if m.scope == "team" {
		heading = "PRs awaiting your review"
	}

	if m.loading && len(m.prs) == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.spinner.View()+" Loading "+heading+"...")
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s (%d)", heading, len(m.prs))) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("━", min(m.width, 74))) + "\n")

	for _, w := range m.warnings {
		b.WriteString(errorStyle.Render("! "+w) + "\n")
	}

	if len(m.prs) == 0 {
		b.WriteString(dimStyle.Render("  No PRs found.") + "\n")
	}

	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.prs) {
		end = len(m.prs)
	}
	for i := m.scrollOffset; i < end; i++ {
		m.renderPR(&b, m.prs[i], i == m.cursor)
	}

	// Pad to full height so the status bar stays anchored.
	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-1 {
		b.WriteString("\n")
		rendered++
	}
	return b.String()
}

func (m PRsModel) renderPR(b *strings.Builder, p prs.PR, selected bool) {
	cursor := "  "
	titleStyle := lipgloss.NewStyle().Foreground(colorWhite)
	if selected {
		cursor = "> "
		titleStyle = titleStyle.Background(lipgloss.Color("236"))
	}

	// Source badge + repo/number.
	var badge string
	var id string
	if p.Source == prs.SourceGitHub {
		badge = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render("github")
		id = fmt.Sprintf("#%d", p.Number)
	} else {
		badge = lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("gerrit")
		id = fmt.Sprintf("!%d", p.Number)
	}

	title := p.Title
	if p.Draft {
		title = "[draft] " + title
	}
	maxTitle := m.width - 20
	if maxTitle > 3 && len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	repoID := lipgloss.NewStyle().Foreground(colorBlue).Render(fmt.Sprintf("%s %s", p.Repo, id))
	b.WriteString(fmt.Sprintf("%s%s %s  %s\n", cursor, badge, repoID, titleStyle.Render(title)))
	b.WriteString(fmt.Sprintf("      %s  %s\n",
		prStatusStyle(p.Status).Render(p.Status),
		dimStyle.Render(p.Author),
	))
}

func prStatusStyle(s string) lipgloss.Style {
	switch s {
	case "approved", "CR+2":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "changes requested":
		return lipgloss.NewStyle().Foreground(colorRed)
	case "CR+1":
		return lipgloss.NewStyle().Foreground(colorGreen)
	default:
		return lipgloss.NewStyle().Foreground(colorYellow)
	}
}

// openURL opens a URL in the default browser (best-effort, non-blocking).
func openURL(url string) {
	if url == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
