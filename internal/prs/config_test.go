package prs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, ".jira_config")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNormalizeRepo(t *testing.T) {
	cases := map[string]string{
		"owner/repo":                        "owner/repo",
		"  owner/repo  ":                    "owner/repo",
		"https://github.com/owner/repo":     "owner/repo",
		"https://github.com/owner/repo.git": "owner/repo",
		"owner/repo/":                       "owner/repo",
	}
	for in, want := range cases {
		got, err := NormalizeRepo(in)
		if err != nil || got != want {
			t.Errorf("NormalizeRepo(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "noslash", "a/b/c", "/repo", "owner/"} {
		if _, err := NormalizeRepo(bad); err == nil {
			t.Errorf("NormalizeRepo(%q) should error", bad)
		}
	}
}

func TestAddPreservesOtherSections(t *testing.T) {
	path := writeConfig(t, "[jira]\nurl = https://x\ntoken = secret\n\n[prs]\ngerrit_filter = ownerin:lx\ngithub_repos = owner/a\n")

	repos, err := AddRepo("owner/b")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(repos, ",") != "owner/a,owner/b" {
		t.Errorf("got repos %v", repos)
	}
	out := read(t, path)
	for _, want := range []string{"[jira]", "token = secret", "gerrit_filter = ownerin:lx", "github_repos = owner/a,owner/b"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAddDedup(t *testing.T) {
	writeConfig(t, "[prs]\ngithub_repos = owner/a\n")
	if _, err := AddRepo("OWNER/A"); err == nil {
		t.Error("expected dedup error for case-insensitive match")
	}
}

func TestRemoveRepo(t *testing.T) {
	path := writeConfig(t, "[prs]\ngithub_repos = owner/a,owner/b,owner/c\n")
	repos, err := RemoveRepo("owner/b")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(repos, ",") != "owner/a,owner/c" {
		t.Errorf("got %v", repos)
	}
	if strings.Contains(read(t, path), "owner/b") {
		t.Error("owner/b should be gone from file")
	}
	if _, err := RemoveRepo("owner/z"); err == nil {
		t.Error("removing absent repo should error")
	}
}

func TestAddCreatesSectionWhenMissing(t *testing.T) {
	path := writeConfig(t, "[jira]\nurl = https://x\n")
	if _, err := AddRepo("owner/a"); err != nil {
		t.Fatal(err)
	}
	out := read(t, path)
	if !strings.Contains(out, "[prs]") || !strings.Contains(out, "github_repos = owner/a") {
		t.Errorf("should create [prs] section:\n%s", out)
	}
	if !strings.Contains(out, "[jira]") {
		t.Errorf("must preserve [jira]:\n%s", out)
	}
}

func TestAddKeyWhenSectionExistsWithoutKey(t *testing.T) {
	path := writeConfig(t, "[prs]\ngerrit_filter = ownerin:lx\n")
	if _, err := AddRepo("owner/a"); err != nil {
		t.Fatal(err)
	}
	out := read(t, path)
	if !strings.Contains(out, "gerrit_filter = ownerin:lx") || !strings.Contains(out, "github_repos = owner/a") {
		t.Errorf("should keep gerrit_filter and add github_repos:\n%s", out)
	}
}

func TestAddWhenNoConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if _, err := AddRepo("owner/a"); err != nil {
		t.Fatal(err)
	}
	out := read(t, filepath.Join(dir, ".jira_config"))
	if !strings.Contains(out, "[prs]") || !strings.Contains(out, "github_repos = owner/a") {
		t.Errorf("should create config file:\n%s", out)
	}
}

func TestLoadConfigTeamKeys(t *testing.T) {
	writeConfig(t, `[jira]
url = https://example.atlassian.net

[prs]
gerrit_filter = ownerin:lx
github_repos = org/a,org/b
github_team = org/learning-experience
github_authors = alice, bob
`)
	// Clear env so the file is what's under test.
	for _, k := range []string{"JET_PRS_GERRIT_FILTER", "JET_PRS_GITHUB_REPOS", "JET_PRS_GITHUB_TEAM", "JET_PRS_GITHUB_AUTHORS"} {
		t.Setenv(k, "")
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHubTeam != "org/learning-experience" {
		t.Errorf("github_team = %q", cfg.GitHubTeam)
	}
	if strings.Join(cfg.GitHubAuthors, ",") != "alice,bob" {
		t.Errorf("github_authors = %v (want trimmed alice,bob)", cfg.GitHubAuthors)
	}
	if !cfg.TeamAuthorsConfigured() {
		t.Error("TeamAuthorsConfigured() should be true")
	}
	if (&Config{}).TeamAuthorsConfigured() {
		t.Error("empty config should not report a team")
	}
}

func TestLoadConfigTeamEnvOverride(t *testing.T) {
	writeConfig(t, "[prs]\ngithub_team = org/from-file\n")
	t.Setenv("JET_PRS_GITHUB_TEAM", "org/from-env")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHubTeam != "org/from-env" {
		t.Errorf("env should win: got %q", cfg.GitHubTeam)
	}
}
