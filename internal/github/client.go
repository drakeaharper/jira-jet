// Package github wraps the `gh` CLI to list pull requests as structured data.
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PR is the subset of gh's pull-request JSON that jet displays.
type PR struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	Mergeable      string `json:"mergeable"` // MERGEABLE, CONFLICTING, or UNKNOWN
	UpdatedAt      string `json:"updatedAt"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Repo string `json:"-"` // filled in by caller (gh doesn't return it per-item here)
}

const jsonFields = "number,title,url,state,isDraft,reviewDecision,mergeable,updatedAt,author"

// Available reports whether the gh CLI is installed.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// RepoExists reports whether the given owner/repo is visible to the
// authenticated gh user.
func RepoExists(repo string) bool {
	return exec.Command("gh", "repo", "view", repo, "--json", "name").Run() == nil
}

// listRepo runs `gh pr list` for one repo with an optional extra --search filter.
func listRepo(repo, search string, limit int) ([]PR, error) {
	args := []string{
		"pr", "list",
		"--repo", repo,
		"--state", "open",
		"--json", jsonFields,
		"--limit", fmt.Sprintf("%d", limit),
	}
	if search != "" {
		args = append(args, "--search", search)
	}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list failed for %s: %s", repo, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gh pr list failed for %s: %w", repo, err)
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output for %s: %w", repo, err)
	}
	for i := range prs {
		prs[i].Repo = repo
	}
	return prs, nil
}

// Authored returns open PRs you authored across the given repos.
func Authored(repos []string, limit int) ([]PR, error) {
	return listAcross(repos, "author:@me", limit)
}

// ReviewRequested returns open PRs where your review is requested across the repos.
func ReviewRequested(repos []string, limit int) ([]PR, error) {
	return listAcross(repos, "review-requested:@me", limit)
}

// NotAuthored returns every open PR across the repos except your own. Used for
// team-wide scope, where a review request is not required to surface a PR.
func NotAuthored(repos []string, limit int) ([]PR, error) {
	return listAcross(repos, "-author:@me", limit)
}

// ByAuthors returns open PRs across the repos written by any of the given
// logins. Callers should have already removed their own login from the list.
//
// GitHub search ANDs repeated qualifiers, so "author:a author:b" matches
// nothing — a PR has exactly one author. The alternatives must be OR'd
// explicitly. Note this means "-author:@me" cannot be appended: it would bind
// to the last OR branch only.
func ByAuthors(repos, authors []string, limit int) ([]PR, error) {
	terms := make([]string, 0, len(authors))
	for _, a := range authors {
		if a = strings.TrimSpace(a); a != "" {
			terms = append(terms, "author:"+a)
		}
	}
	if len(terms) == 0 {
		return NotAuthored(repos, limit)
	}
	return listAcross(repos, strings.Join(terms, " OR "), limit)
}

// CurrentLogin returns the authenticated gh user's login.
func CurrentLogin() (string, error) {
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve current gh user: %w", ghErr(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// TeamMembers expands an "org/team-slug" into member logins via the GitHub API.
func TeamMembers(team string) ([]string, error) {
	org, slug, ok := strings.Cut(strings.TrimSpace(team), "/")
	if !ok || org == "" || slug == "" {
		return nil, fmt.Errorf("expected org/team-slug, got %q", team)
	}
	path := fmt.Sprintf("orgs/%s/teams/%s/members", org, slug)
	out, err := exec.Command("gh", "api", "--paginate", path, "--jq", ".[].login").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list members of %s: %w", team, ghErr(err))
	}
	var logins []string
	for _, line := range strings.Split(string(out), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			logins = append(logins, l)
		}
	}
	if len(logins) == 0 {
		return nil, fmt.Errorf("team %s has no visible members (does your gh token have read:org?)", team)
	}
	return logins, nil
}

// ghErr unwraps an exec failure into the stderr text gh produced, which is far
// more useful than "exit status 1".
func ghErr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
	}
	return err
}

func listAcross(repos []string, search string, limit int) ([]PR, error) {
	var all []PR
	var errs []string
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		prs, err := listRepo(repo, search, limit)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		all = append(all, prs...)
	}
	if len(all) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return all, nil
}
