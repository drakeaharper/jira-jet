package prs

import (
	"testing"

	"jet/internal/gerrit"
	"jet/internal/github"
)

func boolp(b bool) *bool { return &b }

func TestGerritReviewable(t *testing.T) {
	rules := map[string]int{"Code-Review": -1, "Lint-Review": -2}

	label := func(name string, votes ...int) map[string]interface{} {
		all := make([]interface{}, len(votes))
		for i, v := range votes {
			all[i] = map[string]interface{}{"value": float64(v)}
		}
		return map[string]interface{}{name: map[string]interface{}{"all": all}}
	}

	tests := []struct {
		name       string
		ch         gerrit.Change
		wantReview bool
		wantReason string
	}{
		{"clean", gerrit.Change{}, true, ""},
		{"cr-1 blocks", gerrit.Change{Labels: label("Code-Review", -1)}, false, "CR-1"},
		{"cr+2 ok", gerrit.Change{Labels: label("Code-Review", 2)}, true, ""},
		{"lint-1 ok (threshold -2)", gerrit.Change{Labels: label("Lint-Review", -1)}, true, ""},
		{"lint-2 blocks", gerrit.Change{Labels: label("Lint-Review", -2)}, false, "LR-2"},
		{"merge conflict blocks", gerrit.Change{Mergeable: boolp(false)}, false, "merge conflict"},
		{"mergeable ok", gerrit.Change{Mergeable: boolp(true)}, true, ""},
	}
	for _, tc := range tests {
		gotReview, gotReason := gerritReviewable(tc.ch, true, rules)
		if gotReview != tc.wantReview || gotReason != tc.wantReason {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.name, gotReview, gotReason, tc.wantReview, tc.wantReason)
		}
	}
}

func TestFromGitHubReviewability(t *testing.T) {
	tests := []struct {
		name       string
		p          github.PR
		wantReview bool
		wantReason string
	}{
		{"clean needs review", github.PR{ReviewDecision: "REVIEW_REQUIRED", Mergeable: "MERGEABLE"}, true, ""},
		{"approved", github.PR{ReviewDecision: "APPROVED"}, true, ""},
		{"draft blocks", github.PR{IsDraft: true}, false, "draft"},
		{"changes requested blocks", github.PR{ReviewDecision: "CHANGES_REQUESTED"}, false, "changes requested"},
		{"conflict blocks", github.PR{Mergeable: "CONFLICTING"}, false, "merge conflict"},
	}
	for _, tc := range tests {
		got := fromGitHub(tc.p)
		if got.Reviewable != tc.wantReview || got.BlockReason != tc.wantReason {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.name, got.Reviewable, got.BlockReason, tc.wantReview, tc.wantReason)
		}
	}
}

func TestGroupBySourceRepo(t *testing.T) {
	list := []PR{
		{Source: SourceGitHub, Repo: "org/b", Number: 1, Reviewable: false, Updated: "2026-01-03"},
		{Source: SourceGerrit, Repo: "canvas", Number: 2, Reviewable: false, Updated: "2026-01-05"},
		{Source: SourceGerrit, Repo: "canvas", Number: 3, Reviewable: true, Updated: "2026-01-01"},
		{Source: SourceGitHub, Repo: "org/a", Number: 4, Reviewable: true, Updated: "2026-01-02"},
	}
	groups := GroupBySourceRepo(list)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// Gerrit group first.
	if groups[0].Source != SourceGerrit || groups[0].Repo != "canvas" {
		t.Errorf("first group should be gerrit/canvas, got %s/%s", groups[0].Source, groups[0].Repo)
	}
	// Within gerrit/canvas, reviewable (#3) before blocked (#2).
	if groups[0].PRs[0].Number != 3 {
		t.Errorf("reviewable PR should sort first, got #%d", groups[0].PRs[0].Number)
	}
	// GitHub groups sorted by repo name: org/a before org/b.
	if groups[1].Repo != "org/a" || groups[2].Repo != "org/b" {
		t.Errorf("github repos should be alphabetical, got %s then %s", groups[1].Repo, groups[2].Repo)
	}
}
