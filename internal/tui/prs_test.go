package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jet/internal/prs"
)

func samplePRs() []prs.PR {
	return []prs.PR{
		{Source: prs.SourceGitHub, Number: 634, Title: "Align widget_dashboard toolbar", Repo: "org/platform-ui", Author: "drakeaharper", URL: "https://github.com/org/platform-ui/pull/634", Status: "approved", Reviewable: true},
		{Source: prs.SourceGitHub, Number: 335, Title: "EGG draft change", Repo: "org/platform-ui", Author: "vetraz", URL: "u2", Status: "needs review", Draft: true, Reviewable: false, BlockReason: "draft"},
		{Source: prs.SourceGerrit, Number: 417266, Title: "collapse widget_dashboard learner shell", Repo: "canvas-lms", Author: "Drake Harper", URL: "https://gerrit.example.com/c/canvas-lms/+/417266", Status: "needs review", Reviewable: true},
		{Source: prs.SourceGerrit, Number: 410474, Title: "auto-escape section name", Repo: "canvas-lms", Author: "Eric Saupe", URL: "u4", Status: "needs review", Reviewable: false, BlockReason: "CR-1"},
	}
}

func TestPRsModelRendersGroupedData(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)

	view := m.View()
	for _, want := range []string{
		"Your open PRs — 4 total, 2 reviewable",
		"gerrit", "canvas-lms (2)",
		"github", "org/platform-ui (2)",
		"!417266", "#634",
		"(blocked: CR-1)", "(blocked: draft)",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestPRsModelGerritGroupFirst(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	view := m.View()
	if strings.Index(view, "canvas-lms") > strings.Index(view, "platform-ui") {
		t.Errorf("gerrit group should render before github group:\n%s", view)
	}
}

func TestPRsModelReviewableFirstWithinGroup(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	view := m.View()
	// Within canvas-lms, the reviewable !417266 must appear before blocked !410474.
	if strings.Index(view, "!417266") > strings.Index(view, "!410474") {
		t.Errorf("reviewable PR should sort before blocked PR:\n%s", view)
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

func TestPRsModelCursorSkipsHeaders(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	// First selectable must be a PR row, not a header.
	if m.rows[m.cursor].header {
		t.Fatalf("cursor should start on a PR row, not a header")
	}
	if _, ok := m.selectedPR(); !ok {
		t.Fatal("selectedPR should resolve at start")
	}
	// Walk down through every selectable row; cursor must never land on a header.
	for i := 0; i < len(m.rows); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, nil)
		if m.rows[m.cursor].header {
			t.Fatalf("cursor landed on a header after %d moves", i+1)
		}
	}
}

func TestPRsModelTabTogglesScope(t *testing.T) {
	m := NewPRsModel("mine").SetSize(120, 40)
	m = m.SetData(samplePRs(), nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if cmd == nil {
		t.Fatal("tab should emit a navigate command")
	}
	nav, ok := cmd().(navigateToPRsMsg)
	if !ok {
		t.Fatalf("expected navigateToPRsMsg, got %T", cmd())
	}
	if nav.scope != "team" {
		t.Errorf("tab from mine should switch to team, got %q", nav.scope)
	}
}
