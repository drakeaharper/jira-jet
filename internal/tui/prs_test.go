package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jet/internal/prs"
)

func samplePRs() []prs.PR {
	return []prs.PR{
		{Source: prs.SourceGitHub, Number: 634, Title: "Align widget_dashboard toolbar", Repo: "org/platform-ui", Author: "drakeaharper", URL: "https://github.com/org/platform-ui/pull/634", Status: "approved"},
		{Source: prs.SourceGerrit, Number: 417266, Title: "collapse widget_dashboard learner shell", Repo: "canvas-lms", Author: "Drake Harper", URL: "https://gerrit.example.com/c/canvas-lms/+/417266", Status: "needs review"},
	}
}

func TestPRsModelRendersData(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)

	view := m.View()
	for _, want := range []string{"Your open PRs (2)", "platform-ui", "#634", "canvas-lms", "!417266", "approved"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestPRsModelTeamHeading(t *testing.T) {
	m := NewPRsModel("team").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	if !strings.Contains(m.View(), "PRs awaiting your review") {
		t.Errorf("team heading missing:\n%s", m.View())
	}
}

func TestPRsModelWarningsShown(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(nil, []string{"github: gh CLI not found on PATH"})
	view := m.View()
	if !strings.Contains(view, "gh CLI not found") || !strings.Contains(view, "No PRs found") {
		t.Errorf("expected warning + empty state:\n%s", view)
	}
}

func TestPRsModelNavigation(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	if m.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, nil)
	if m.cursor != 1 {
		t.Errorf("j should move cursor to 1, got %d", m.cursor)
	}
	// Should clamp at the last item.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, nil)
	if m.cursor != 1 {
		t.Errorf("cursor should clamp at 1, got %d", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, nil)
	if m.cursor != 0 {
		t.Errorf("k should move cursor back to 0, got %d", m.cursor)
	}
}

func TestPRsModelTabTogglesScope(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if cmd == nil {
		t.Fatal("tab should emit a navigate command")
	}
	msg := cmd()
	nav, ok := msg.(navigateToPRsMsg)
	if !ok {
		t.Fatalf("expected navigateToPRsMsg, got %T", msg)
	}
	if nav.scope != "team" {
		t.Errorf("tab from mine should switch to team, got %q", nav.scope)
	}
}
