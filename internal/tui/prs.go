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

// prRow is one rendered line: either a group header or a PR.
type prRow struct {
	header bool
	source prs.Source
	repo   string
	count  int
	pr     prs.PR
}

// PRsModel displays open pull requests aggregated across Gerrit and GitHub,
// grouped by source and repo, reviewable PRs first.
type PRsModel struct {
	scope        string // "mine" or "team"
	rows         []prRow
	cursor       int // index into rows; always points at a non-header row when possible
	total        int
	reviewable   int
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

// SetData stores fetched PRs (grouping them) and per-source warnings.
func (m PRsModel) SetData(list []prs.PR, warnings []string) PRsModel {
	m.warnings = warnings
	m.loading = false
	m.total = len(list)
	m.reviewable = 0
	m.rows = nil
	for _, g := range prs.GroupBySourceRepo(list) {
		m.rows = append(m.rows, prRow{header: true, source: g.Source, repo: g.Repo, count: len(g.PRs)})
		for _, p := range g.PRs {
			if p.Reviewable {
				m.reviewable++
			}
			m.rows = append(m.rows, prRow{pr: p})
		}
	}
	m.cursor = m.firstSelectable()
	m.scrollOffset = 0
	m.ensureVisible()
	return m
}

func (m *PRsModel) firstSelectable() int {
	for i, r := range m.rows {
		if !r.header {
			return i
		}
	}
	return 0
}

func (m *PRsModel) nextSelectable() {
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if !m.rows[i].header {
			m.cursor = i
			return
		}
	}
}

func (m *PRsModel) prevSelectable() {
	for i := m.cursor - 1; i >= 0; i-- {
		if !m.rows[i].header {
			m.cursor = i
			return
		}
	}
}

func (m *PRsModel) ensureVisible() {
	if m.height <= 0 || len(m.rows) == 0 {
		return
	}
	// Header lines above the content: title + separator + warnings.
	visible := m.height - 3 - len(m.warnings)
	if visible < 1 {
		visible = 1
	}
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

func (m PRsModel) selectedPR() (prs.PR, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) && !m.rows[m.cursor].header {
		return m.rows[m.cursor].pr, true
	}
	return prs.PR{}, false
}

func (m PRsModel) Update(msg tea.Msg, _ interface{}) (PRsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "j" || msg.String() == "down":
			m.nextSelectable()
			m.ensureVisible()
			return m, nil
		case msg.String() == "k" || msg.String() == "up":
			m.prevSelectable()
			m.ensureVisible()
			return m, nil
		case msg.String() == "enter" || msg.String() == "o":
			if p, ok := m.selectedPR(); ok {
				openURL(p.URL)
			}
			return m, nil
		case msg.String() == "tab":
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

	if m.loading && len(m.rows) == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.spinner.View()+" Loading "+heading+"...")
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s — %d total, %d reviewable", heading, m.total, m.reviewable)) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("━", min(m.width, 74))) + "\n")

	for _, w := range m.warnings {
		b.WriteString(errorStyle.Render("! "+w) + "\n")
	}

	if len(m.rows) == 0 {
		b.WriteString(dimStyle.Render("  No PRs found.") + "\n")
	}

	visible := m.height - 3 - len(m.warnings)
	if visible < 1 {
		visible = 1
	}
	end := m.scrollOffset + visible
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.scrollOffset; i < end; i++ {
		m.renderRow(&b, m.rows[i], i == m.cursor)
	}

	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-1 {
		b.WriteString("\n")
		rendered++
	}
	return b.String()
}

func (m PRsModel) renderRow(b *strings.Builder, row prRow, selected bool) {
	if row.header {
		src := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("gerrit")
		if row.source == prs.SourceGitHub {
			src = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render("github")
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n",
			src,
			dimStyle.Render("·"),
			lipgloss.NewStyle().Foreground(colorBlue).Render(fmt.Sprintf("%s (%d)", row.repo, row.count))))
		return
	}

	p := row.pr
	cursor := "  "
	if selected {
		cursor = "> "
	}

	id := fmt.Sprintf("!%d", p.Number)
	if p.Source == prs.SourceGitHub {
		id = fmt.Sprintf("#%d", p.Number)
	}

	title := p.Title
	maxTitle := m.width - 34
	if maxTitle > 3 && len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	idStyle := lipgloss.NewStyle().Foreground(colorYellow)
	titleStyle := lipgloss.NewStyle().Foreground(colorWhite)
	authorStyle := dimStyle
	marker := ""
	if !p.Reviewable {
		// Blocked: dim everything and append the reason.
		idStyle = dimStyle
		titleStyle = dimStyle
		marker = "  " + dimStyle.Render("(blocked: "+p.BlockReason+")")
	}
	if selected {
		bg := lipgloss.Color("236")
		idStyle = idStyle.Background(bg)
		titleStyle = titleStyle.Background(bg)
		authorStyle = authorStyle.Background(bg)
	}

	b.WriteString(fmt.Sprintf("  %s%s  %s  %s%s\n",
		cursor,
		idStyle.Render(fmt.Sprintf("%-8s", id)),
		titleStyle.Render(title),
		authorStyle.Render(p.Author),
		marker,
	))
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
