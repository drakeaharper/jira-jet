// Package prs aggregates open pull requests across Gerrit and GitHub into a
// single unified model for jet's `prs` commands.
package prs

import (
	"fmt"
	"sort"

	"jet/internal/gerrit"
	"jet/internal/gerry"
	"jet/internal/github"
)

// Source identifies where a PR lives.
type Source string

const (
	SourceGerrit Source = "gerrit"
	SourceGitHub Source = "github"
)

// PR is a system-agnostic pull request / change.
type PR struct {
	Source      Source `json:"source"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Repo        string `json:"repo"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Status      string `json:"status"` // human-readable review/merge state
	Draft       bool   `json:"draft"`
	Updated     string `json:"updated"`               // raw upstream timestamp
	Reviewable  bool   `json:"reviewable"`            // false when blocked (see BlockReason)
	BlockReason string `json:"block_reason,omitempty"` // why not reviewable, e.g. "CR-1", "draft"
}

// Group is a set of PRs sharing a source and repo, ordered reviewable-first.
type Group struct {
	Source Source `json:"source"`
	Repo   string `json:"repo"`
	PRs    []PR   `json:"prs"`
}

// GroupBySourceRepo buckets PRs by (source, repo). Groups are ordered gerrit
// before github, then by repo name; within a group, reviewable PRs come first,
// then most-recently-updated.
func GroupBySourceRepo(list []PR) []Group {
	index := map[string]*Group{}
	var order []string
	for _, p := range list {
		key := string(p.Source) + "\x00" + p.Repo
		g, ok := index[key]
		if !ok {
			g = &Group{Source: p.Source, Repo: p.Repo}
			index[key] = g
			order = append(order, key)
		}
		g.PRs = append(g.PRs, p)
	}

	groups := make([]Group, 0, len(order))
	for _, key := range order {
		g := index[key]
		sort.SliceStable(g.PRs, func(i, j int) bool {
			if g.PRs[i].Reviewable != g.PRs[j].Reviewable {
				return g.PRs[i].Reviewable // reviewable first
			}
			return g.PRs[i].Updated > g.PRs[j].Updated
		})
		groups = append(groups, *g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Source != groups[j].Source {
			return groups[i].Source == SourceGerrit // gerrit groups first
		}
		return groups[i].Repo < groups[j].Repo
	})
	return groups
}

// Options controls which sources are queried and how many results per source.
type Options struct {
	Source string // "all", "gerrit", or "github"
	Limit  int
}

func (o Options) wantGerrit() bool { return o.Source == "" || o.Source == "all" || o.Source == "gerrit" }
func (o Options) wantGitHub() bool { return o.Source == "" || o.Source == "all" || o.Source == "github" }

// Mine returns your open PRs across the configured sources.
func Mine(cfg *Config, opts Options) ([]PR, []error) {
	return collect(cfg, opts, "owner:self is:open -is:wip", github.Authored)
}

// Team returns open PRs awaiting your review across the configured sources.
func Team(cfg *Config, opts Options) ([]PR, []error) {
	q := fmt.Sprintf("is:open -is:wip -is:ignored -owner:self (reviewer:self OR cc:self)")
	if cfg.GerritFilter != "" {
		q = fmt.Sprintf("(%s) %s", q, cfg.GerritFilter)
	}
	return collect(cfg, opts, q, github.ReviewRequested)
}

// collect runs the gerrit query and the github lister, merging into unified PRs.
func collect(cfg *Config, opts Options, gerritQuery string, ghList func([]string, int) ([]github.PR, error)) ([]PR, []error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}
	var out []PR
	var errs []error

	if opts.wantGerrit() {
		gcfg, err := gerry.Load()
		if err != nil {
			errs = append(errs, fmt.Errorf("gerrit: %w", err))
		} else {
			changes, err := gerrit.NewClient(gcfg).ListChanges(gerritQuery, limit)
			if err != nil {
				errs = append(errs, fmt.Errorf("gerrit: %w", err))
			} else {
				base := gerritWebBase(gcfg)
				blockConflict, blockingLabels := gcfg.ReviewabilityRules()
				for _, ch := range changes {
					out = append(out, fromChange(ch, base, blockConflict, blockingLabels))
				}
			}
		}
	}

	if opts.wantGitHub() {
		if !github.Available() {
			errs = append(errs, fmt.Errorf("github: gh CLI not found on PATH"))
		} else if len(cfg.GitHubRepos) == 0 {
			errs = append(errs, fmt.Errorf("github: no repos configured (set github_repos in [prs])"))
		} else {
			ghPRs, err := ghList(cfg.GitHubRepos, limit)
			if err != nil {
				errs = append(errs, fmt.Errorf("github: %w", err))
			}
			for _, p := range ghPRs {
				out = append(out, fromGitHub(p))
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out, errs
}

func fromChange(ch gerrit.Change, webBase string, blockConflict bool, blockingLabels map[string]int) PR {
	reviewable, reason := gerritReviewable(ch, blockConflict, blockingLabels)
	return PR{
		Source:      SourceGerrit,
		Number:      ch.Number,
		Title:       ch.Subject,
		Repo:        ch.Project,
		Author:      ch.Owner.DisplayName(),
		URL:         fmt.Sprintf("%s/c/%s/+/%d", webBase, ch.Project, ch.Number),
		Status:      gerritStatus(ch),
		Updated:     ch.Updated,
		Reviewable:  reviewable,
		BlockReason: reason,
	}
}

// gerritReviewable applies gerry's reviewability rules: a merge conflict or a
// blocking negative vote makes a change not reviewable. Unknown mergeability
// does not disqualify.
func gerritReviewable(ch gerrit.Change, blockConflict bool, blockingLabels map[string]int) (bool, string) {
	if blockConflict {
		if known, mergeable := ch.MergeableState(); known && !mergeable {
			return false, "merge conflict"
		}
	}
	for label, threshold := range blockingLabels {
		if hasVote, min := ch.MinLabelVote(label); hasVote && min <= threshold {
			return false, labelReason(label, min)
		}
	}
	return true, ""
}

// labelReason renders a compact reason like "CR-1" from a label + vote.
func labelReason(label string, vote int) string {
	abbr := label
	switch label {
	case "Code-Review":
		abbr = "CR"
	case "QA-Review":
		abbr = "QR"
	case "Lint-Review":
		abbr = "LR"
	case "Verified":
		abbr = "V"
	}
	return fmt.Sprintf("%s%+d", abbr, vote)
}

func gerritStatus(ch gerrit.Change) string {
	cr := ch.LabelVote("Code-Review")
	switch {
	case cr >= 2:
		return "CR+2"
	case cr == 1:
		return "CR+1"
	case cr < 0:
		return "CR" + fmt.Sprintf("%d", cr)
	default:
		return "needs review"
	}
}

func fromGitHub(p github.PR) PR {
	status := "needs review"
	switch p.ReviewDecision {
	case "APPROVED":
		status = "approved"
	case "CHANGES_REQUESTED":
		status = "changes requested"
	case "REVIEW_REQUIRED", "":
		status = "needs review"
	}

	// A GitHub PR is not reviewable when the author still owes work: it's a
	// draft, changes were requested, or it has a merge conflict.
	reviewable, reason := true, ""
	switch {
	case p.IsDraft:
		reviewable, reason = false, "draft"
	case p.ReviewDecision == "CHANGES_REQUESTED":
		reviewable, reason = false, "changes requested"
	case p.Mergeable == "CONFLICTING":
		reviewable, reason = false, "merge conflict"
	}

	return PR{
		Source:      SourceGitHub,
		Number:      p.Number,
		Title:       p.Title,
		Repo:        p.Repo,
		Author:      p.Author.Login,
		URL:         p.URL,
		Status:      status,
		Draft:       p.IsDraft,
		Updated:     p.UpdatedAt,
		Reviewable:  reviewable,
		BlockReason: reason,
	}
}

// gerritWebBase derives the browser URL base (drops the /a/ auth path and port).
func gerritWebBase(cfg *gerry.Config) string {
	return "https://" + cfg.Server
}
