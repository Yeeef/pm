package proc

import "testing"

func TestGitHubRepoReadsEachRemoteForm(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/Owner/repo.git":         "Owner/repo",
		"https://github.com/owner/repo\n":           "owner/repo",
		"https://token@github.com/owner/re.po/":     "owner/re.po",
		"git@github.com:owner/repo.git":             "owner/repo",
		"ssh://git@github.com/owner/repo":           "owner/repo",
		"ssh://git@GitHub.com:22/owner/repo.git":    "owner/repo",
		"https://gitlab.com/owner/repo.git":         "",
		"git@gitlab.example:owner/repo.git":         "",
		"/tmp/origin.git":                           "",
		"https://github.com/owner/repo/pull/3":      "",
		"https://github.example.com/owner/repo.git": "",
	} {
		if got := GitHubRepo(url); got != want {
			t.Errorf("GitHubRepo(%q) = %q, want %q", url, got, want)
		}
	}
}
