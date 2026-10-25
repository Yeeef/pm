package proc

import "testing"

func TestGitHubRepoReadsEachRemoteForm(t *testing.T) {
	orig := SSHHostName
	t.Cleanup(func() { SSHHostName = orig })
	SSHHostName = func(host string) string { // the ssh config of the test: two aliases, one of them GitHub's
		return map[string]string{"github-work": "github.com", "gitlab-work": "gitlab.com"}[host]
	}
	for url, want := range map[string]string{
		"https://github.com/Owner/repo.git":         "Owner/repo",
		"https://github.com/owner/repo\n":           "owner/repo",
		"https://token@github.com/owner/re.po/":     "owner/re.po",
		"https://www.github.com/owner/repo":         "owner/repo",
		"git@github.com:owner/repo.git":             "owner/repo",
		"github.com:owner/repo.git":                 "owner/repo",
		"git@github.com:/owner/repo.git":            "owner/repo",
		"ssh://git@github.com/owner/repo":           "owner/repo",
		"ssh://git@GitHub.com:22/owner/repo.git":    "owner/repo",
		"git@github-work:owner/repo.git":            "owner/repo",
		"ssh://git@github-work/owner/repo.git":      "owner/repo",
		"git@gitlab-work:owner/repo.git":            "",
		"git@unknown-alias:owner/repo.git":          "",
		"https://github-work/owner/repo.git":        "", // an ssh alias means nothing to https
		"https://gitlab.com/owner/repo.git":         "",
		"/tmp/origin.git":                           "",
		"/tmp/a/b":                                  "",
		"https://github.com/owner/repo/pull/3":      "",
		"https://github.example.com/owner/repo.git": "",
	} {
		if got := GitHubRepo(url); got != want {
			t.Errorf("GitHubRepo(%q) = %q, want %q", url, got, want)
		}
	}
}
