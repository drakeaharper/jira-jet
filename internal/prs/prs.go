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
	Source  Source `json:"source"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Repo    string `json:"repo"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	Status  string `json:"status"`  // human-readable review/merge state
	Draft   bool   `json:"draft"`
	Updated string `json:"updated"` // raw upstream timestamp
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
				for _, ch := range changes {
					out = append(out, fromChange(ch, base))
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

func fromChange(ch gerrit.Change, webBase string) PR {
	return PR{
		Source:  SourceGerrit,
		Number:  ch.Number,
		Title:   ch.Subject,
		Repo:    ch.Project,
		Author:  ch.Owner.DisplayName(),
		URL:     fmt.Sprintf("%s/c/%s/+/%d", webBase, ch.Project, ch.Number),
		Status:  gerritStatus(ch),
		Updated: ch.Updated,
	}
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
	return PR{
		Source:  SourceGitHub,
		Number:  p.Number,
		Title:   p.Title,
		Repo:    p.Repo,
		Author:  p.Author.Login,
		URL:     p.URL,
		Status:  status,
		Draft:   p.IsDraft,
		Updated: p.UpdatedAt,
	}
}

// gerritWebBase derives the browser URL base (drops the /a/ auth path and port).
func gerritWebBase(cfg *gerry.Config) string {
	return "https://" + cfg.Server
}
