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
