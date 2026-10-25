package proc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GHTimeout bounds one gh call.
const GHTimeout = 60 * time.Second

// GH runs gh in dir and returns what it prints; a gh that fails or runs past GHTimeout is an error naming why.
func GH(dir string, args ...string) (string, error) {
	cmd := "gh " + strings.Join(args, " ")
	res, err := Run(append([]string{"gh"}, args...), Options{Cwd: &dir, Timeout: GHTimeout,
		TimeoutText: strconv.Itoa(int(GHTimeout.Seconds()))})
	switch {
	case err != nil:
		return "", fmt.Errorf("%s did not run: %v", cmd, err)
	case res.Code != 0:
		why := strings.TrimSpace(res.Stderr)
		if why == "" {
			why = fmt.Sprintf("exit %d", res.Code)
		}
		return "", fmt.Errorf("%s failed: %s", cmd, why)
	}
	return res.Stdout, nil
}

// GHJSON runs gh in dir and decodes the JSON it prints into v; a gh that fails, runs past GHTimeout or prints no JSON
// is an error naming why.
func GHJSON(dir string, v any, args ...string) error {
	out, err := GH(dir, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("gh %s printed no JSON: %v", strings.Join(args, " "), err)
	}
	return nil
}

var (
	urlRemote  = regexp.MustCompile(`^([a-z][a-z0-9+.-]*)://(?:[^@/]+@)?([^/:@]+)(?::[0-9]*)?/(.*)$`)
	scpRemote  = regexp.MustCompile(`^(?:[^@/:]+@)?([^/:@]+):(.*)$`)
	repoPath   = regexp.MustCompile(`^/?([\w.-]+/[\w.-]+?)(?:\.git)?/?$`)
	githubPull = regexp.MustCompile(`^https://github\.com/([\w.-]+/[\w.-]+)/pull/[0-9]+$`)
	repoCommit = regexp.MustCompile(`^([\w.-]+/[\w.-]+)@([0-9a-fA-F]{4,40})$`)
)

// SSHHostName is the host ssh connects to for host, an alias ~/.ssh/config may map (ssh -G's hostname line); "" when
// ssh cannot tell. A variable so a test can resolve an alias without ssh.
var SSHHostName = func(host string) string {
	res, err := Run([]string{"ssh", "-G", host}, Options{Timeout: 10 * time.Second, TimeoutText: "10"})
	if err != nil || res.Code != 0 {
		return ""
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if name, ok := strings.CutPrefix(line, "hostname "); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

func isGitHub(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || host == "www.github.com"
}

// GitHubRepo is the OWNER/REPO a git remote URL names on GitHub, in its URL form (https, ssh, git) or scp-like one
// ([user@]host:path); an ssh host other than github.com counts when it is an alias ssh resolves to github.com
// (SSHHostName). "" for a URL elsewhere, and for an alias ssh cannot resolve.
func GitHubRepo(url string) string {
	url = strings.TrimSpace(url)
	var scheme, host, path string
	if m := urlRemote.FindStringSubmatch(url); m != nil {
		scheme, host, path = strings.ToLower(m[1]), m[2], m[3]
	} else if m := scpRemote.FindStringSubmatch(url); m != nil {
		scheme, host, path = "ssh", m[1], m[2]
	}
	p := repoPath.FindStringSubmatch(path)
	if p == nil {
		return ""
	}
	ssh := scheme == "ssh" || scheme == "git+ssh" || scheme == "ssh+git"
	if !isGitHub(host) && !(ssh && isGitHub(SSHHostName(host))) {
		return ""
	}
	return p[1]
}

// OtherRepoPR is the OWNER/REPO of pr when it is another repo's pull request than dir's: a GitHub PR URL whose
// OWNER/REPO (ignoring case) is not the one remote's configured URL names, or any GitHub PR URL when that URL is not on
// GitHub. "" for this repo's PR, for a URL that is no GitHub PR, and when dir has no such remote, which leaves no other
// repo to tell apart: this repo's checks then run, and those that need the remote refuse without it.
func OtherRepoPR(dir, remote, pr string) (string, error) {
	other, _, err := otherRepoPR(dir, remote, pr)
	return other, err
}

// otherRepoPR is OtherRepoPR, and the OWNER/REPO the remote's URL names ("" off GitHub).
func otherRepoPR(dir, remote, pr string) (other, here string, err error) {
	m := githubPull.FindStringSubmatch(pr)
	if m == nil {
		return "", "", nil
	}
	res, err := Run([]string{"git", "config", "--get", "remote." + remote + ".url"}, Options{Cwd: &dir})
	switch {
	case err != nil:
		return "", "", fmt.Errorf("git config remote.%s.url did not run: %v", remote, err)
	case res.Code == 1: // no such remote
		return "", "", nil
	case res.Code != 0:
		return "", "", fmt.Errorf("git config remote.%s.url failed: %s", remote, strings.TrimSpace(res.Stderr))
	}
	if here = GitHubRepo(res.Stdout); here != "" && strings.EqualFold(here, m[1]) {
		return "", here, nil
	}
	return m[1], here, nil
}

// PR is a GitHub pull request's merge as gh gives it: its state (OPEN, CLOSED, MERGED), the branch it merges into,
// its repo's default branch and, once merged, its merge commit's full sha.
type PR struct{ State, Base, Default, Merge string }

// MergedToDefault is whether the PR merged into its repo's default branch: a PR merged into a stacked base did not.
func (p PR) MergedToDefault() bool {
	return p.State == "MERGED" && p.Merge != "" && p.Base == p.Default
}

// ViewPR is the merge of pr, a pull request of the GitHub repo slug (OWNER/REPO), read with gh in dir; a gh that fails
// is an error naming why.
func ViewPR(dir, slug, pr string) (PR, error) {
	var got struct {
		State       string `json:"state"`
		Base        string `json:"baseRefName"`
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
	}
	if err := GHJSON(dir, &got, "pr", "view", pr, "--json", "state,mergeCommit,baseRefName"); err != nil {
		return PR{}, err
	}
	var repo struct {
		Default *struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := GHJSON(dir, &repo, "repo", "view", slug, "--json", "defaultBranchRef"); err != nil {
		return PR{}, err
	}
	if repo.Default == nil || repo.Default.Name == "" {
		return PR{}, fmt.Errorf("gh repo view %s gave no default branch", slug)
	}
	p := PR{State: got.State, Base: got.Base, Default: repo.Default.Name}
	if got.MergeCommit != nil {
		p.Merge = got.MergeCommit.OID
	}
	return p, nil
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// PRReachesMain is nil unless pr is a GitHub PR whose base is not main, the main branch of dir's repo (whose remote is
// remote), or for another repo's PR (OtherRepoPR) that repo's default branch (OtherBase); else why, as a refusal says
// it: a stacked PR merges into another PR's branch, so its review could never close as merged. For another repo's PR,
// note says so, naming both repos, for the raise to print.
func PRReachesMain(dir, remote, main, pr string) (note string, err error) {
	if !githubPull.MatchString(pr) {
		return "", nil
	}
	other, here, err := otherRepoPR(dir, remote, pr)
	switch {
	case err != nil:
		return "", err
	case other != "":
		mine := "this repo is " + here
		if here == "" {
			mine = "this repo's remote is not on GitHub"
		}
		return fmt.Sprintf("; %s is another repo's PR, of %s (%s): it is held to %s's default branch, and this "+
			"repo's design-page check does not apply", pr, other, mine, other), OtherBase(dir, other, pr)
	}
	var got struct {
		Base string `json:"baseRefName"`
	}
	if err := GHJSON(dir, &got, "pr", "view", pr, "--json", "baseRefName"); err != nil {
		return "", fmt.Errorf("--pr %s: pm reads its base branch from GitHub, and cannot: %v", pr, err)
	}
	if got.Base != main {
		return "", fmt.Errorf("--pr %s merges into %s, not %s, so its merge does not deliver the sprint; raise the review "+
			"on the PR that reaches %s, or retarget this one first (gh pr edit %s --base %s)", pr,
			strconv.Quote(got.Base), main, main, pr, main)
	}
	return "", nil
}

// OtherBase is nil when pr, a PR of the GitHub repo slug, merges into that repo's default branch; else why not, as a
// refusal says it: a stacked PR's merge does not deliver its sprint.
func OtherBase(dir, slug, pr string) error {
	got, err := ViewPR(dir, slug, pr)
	if err != nil {
		return fmt.Errorf("--pr %s: pm reads its base branch and %s's default branch from GitHub, and cannot: %v", pr,
			slug, err)
	}
	if got.Base != got.Default {
		return fmt.Errorf("--pr %s merges into %s, not %s, %s's default branch, so its merge does not deliver the "+
			"sprint; raise the review on the PR that reaches %s, or retarget this one first (gh pr edit %s --base %s)",
			pr, strconv.Quote(got.Base), got.Default, slug, got.Default, pr, got.Default)
	}
	return nil
}

// OtherMerge is the full sha of the merge commit of pr, a PR of the GitHub repo slug, when gh gives it merged into
// that repo's default branch with a merge commit sha names (a prefix of it); else why not, as a refusal says it, given
// naming sha (--merged, merged as). Another repo's commit is not in this clone, so gh, not git, tells.
func OtherMerge(dir, slug, pr, given, sha string) (string, error) {
	if !shaRE.MatchString(sha) {
		return "", fmt.Errorf("%s %s is not a commit sha: 7 to 40 hex digits", given, strconv.Quote(sha))
	}
	got, err := ViewPR(dir, slug, pr)
	switch {
	case err != nil:
		return "", fmt.Errorf("%s %s: pm reads the merge of %s, a PR of %s, from GitHub, and cannot: %v", given, sha,
			pr, slug, err)
	case got.State != "MERGED" || got.Merge == "":
		return "", fmt.Errorf("%s %s: %s is %s on GitHub, not merged; give its merge commit once it is merged into %s",
			given, sha, pr, got.State, got.Default)
	case got.Base != got.Default:
		return "", fmt.Errorf("%s %s: %s merged into %s, not %s, %s's default branch; give the merge commit once its "+
			"work is on %s", given, sha, pr, strconv.Quote(got.Base), got.Default, slug, got.Default)
	case !strings.HasPrefix(got.Merge, sha):
		return "", fmt.Errorf("%s %s is not the merge commit of %s: GitHub gives %s", given, sha, pr, got.Merge)
	}
	return got.Merge, nil
}

// GitHubRef is how a close names the work ref gives on GitHub, resolved with gh in dir: "commit OWNER/REPO@<short>" for
// OWNER/REPO@SHA and "PR <url>" (with its merge commit once merged) for a PR URL; "" for a ref of neither form. A ref
// that does not resolve, or a PR closed without merging, is an error naming why.
func GitHubRef(dir, ref string) (string, error) {
	if m := repoCommit.FindStringSubmatch(ref); m != nil {
		var c struct {
			SHA string `json:"sha"`
		}
		if err := GHJSON(dir, &c, "api", "repos/"+m[1]+"/commits/"+m[2]); err != nil {
			return "", fmt.Errorf("--commit %s is not a commit of %s on GitHub: %v", ref, m[1], err)
		}
		if len(c.SHA) < 7 {
			return "", fmt.Errorf("--commit %s is not a commit of %s on GitHub: gh api gave no sha", ref, m[1])
		}
		return "commit " + m[1] + "@" + c.SHA[:7], nil
	}
	if m := githubPull.FindStringSubmatch(strings.TrimSuffix(ref, "/")); m != nil {
		ref = strings.TrimSuffix(ref, "/")
		var pr struct {
			State       string `json:"state"`
			MergeCommit *struct {
				OID string `json:"oid"`
			} `json:"mergeCommit"`
		}
		if err := GHJSON(dir, &pr, "pr", "view", ref, "--json", "state,mergeCommit"); err != nil {
			return "", fmt.Errorf("--commit %s is not a pull request on GitHub: %v", ref, err)
		}
		switch {
		case pr.State == "MERGED" && pr.MergeCommit != nil && len(pr.MergeCommit.OID) >= 7:
			return "PR " + ref + ", merged as " + m[1] + "@" + pr.MergeCommit.OID[:7], nil
		case pr.State == "CLOSED":
			return "", fmt.Errorf("--commit %s was closed without merging, so it holds no work; name the commit or "+
				"PR that does, or close the task with --dropped", ref)
		}
		return "PR " + ref, nil
	}
	return "", nil
}
