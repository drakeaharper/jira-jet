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

func TestValidateScope(t *testing.T) {
	for _, ok := range []string{"", ScopeRequested, ScopeAll} {
		if err := ValidateScope(ok); err != nil {
			t.Errorf("scope %q should be valid, got %v", ok, err)
		}
	}
	if err := ValidateScope("everyone"); err == nil {
		t.Error("scope \"everyone\" should be rejected")
	}
}

func TestTeamGerritQuery(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		scope   string
		want    string
		wantErr bool
	}{
		{
			name:  "requested scope keeps reviewer clause",
			scope: ScopeRequested,
			want:  "is:open -is:wip -is:ignored -owner:self (reviewer:self OR cc:self)",
		},
		{
			name:   "requested scope ands the filter",
			filter: "ownerin:lx",
			scope:  ScopeRequested,
			want:   "(is:open -is:wip -is:ignored -owner:self (reviewer:self OR cc:self)) ownerin:lx",
		},
		{
			name:   "all scope drops reviewer clause",
			filter: "ownerin:lx",
			scope:  ScopeAll,
			want:   "(is:open -is:wip -is:ignored -owner:self) ownerin:lx",
		},
		{
			name:    "all scope without filter is refused",
			scope:   ScopeAll,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		got, err := teamGerritQuery(&Config{GerritFilter: tc.filter}, Options{Scope: tc.scope})
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error, got query %q", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestTeamGitHubAuthors(t *testing.T) {
	// Explicit authors win and require no API call.
	cfg := &Config{GitHubAuthors: []string{"alice", "bob"}, GitHubTeam: "org/team"}
	got, err := teamGitHubAuthors(cfg, Options{Scope: ScopeAll})
	if err != nil || len(got) != 2 || got[0] != "alice" {
		t.Errorf("explicit authors should win: got %v, err %v", got, err)
	}

	// Nothing configured → empty list (means "all but mine"), no error.
	got, err = teamGitHubAuthors(&Config{}, Options{Scope: ScopeAll})
	if err != nil || got != nil {
		t.Errorf("unconfigured team should yield (nil, nil): got %v, err %v", got, err)
	}

	// Gerrit-only source must not trigger a GitHub team lookup.
	got, err = teamGitHubAuthors(cfg, Options{Scope: ScopeAll, Source: "gerrit"})
	if err != nil || got != nil {
		t.Errorf("gerrit-only should skip author resolution: got %v, err %v", got, err)
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

func TestWithoutSelf(t *testing.T) {
	in := []string{"alice", "DrakeAHarper", " bob "}
	got := withoutSelf(in, "drakeaharper")
	if len(got) != 2 || got[0] != "alice" || got[1] != " bob " {
		t.Errorf("self should be dropped case-insensitively, got %v", got)
	}
	// Unresolvable login must not silently empty the list.
	if len(withoutSelf(in, "")) != 3 {
		t.Error("empty login should leave the list untouched")
	}
}
