package install

import (
	"fmt"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
)

// git runs git in dir with input on stdin: its stripped stdout, or an Error naming the command and git's message.
func git(dir, input string, args ...string) (string, error) {
	res, err := proc.Run(append([]string{"git"}, args...), proc.Options{Cwd: &dir, Stdin: input})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		why := res.Stderr
		if why == "" {
			why = res.Stdout
		}
		return "", refuse("git %s failed in %s: %s", strings.Join(args, " "), dir, config.PyStrip(why))
	}
	return config.PyStrip(res.Stdout), nil
}

// Git is git in dir, its stdout stripped; a failure is an Error carrying git's message.
func Git(dir string, args ...string) (string, error) { return git(dir, "", args...) }

// gitOK is whether git in dir exits 0.
func gitOK(dir string, args ...string) bool {
	res, err := proc.Run(append([]string{"git"}, args...), proc.Options{Cwd: &dir})
	return err == nil && res.Code == 0
}

// GitConfig is a git config value in dir, "" when unset.
func GitConfig(dir, key string) (string, error) {
	return Git(dir, "config", "--get", "--default=", key)
}

// ---------------------------------------------------------------- a brand-new repo's records branch

// Layout is the records store's directories, which a new records branch holds empty.
var Layout = []string{"projects", "sprints", "days", "design", "docs", "postmortems"}

const keep = ".gitkeep" // git tracks no empty directory

// RemoteHasBranch is whether remote has refs/heads/<branch>; an unreachable remote is an Error.
func RemoteHasBranch(dir, remote, branch string) (bool, error) {
	res, err := proc.Run([]string{"git", "ls-remote", "--exit-code", "--heads", remote, "refs/heads/" + branch},
		proc.Options{Cwd: &dir})
	if err != nil {
		return false, err
	}
	switch res.Code {
	case 0:
		return true, nil
	case 2: // the remote answered and has no such branch
		return false, nil
	}
	why := res.Stderr
	if why == "" {
		why = res.Stdout
	}
	return false, refuse("git ls-remote %s failed: %s", remote, config.PyStrip(why))
}

// CreateRecordsBranch makes the store's empty layout an orphan commit, publishes it as <remote>/<branch>, then creates
// the local branch tracking it. The push comes first, so a rejected push (another clone created it meanwhile) leaves
// nothing local; pm push fetches <remote>/<branch> and so cannot publish a branch the remote lacks.
func CreateRecordsBranch(dir, remote, branch string) (string, error) {
	blob, err := git(dir, "", "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	sub, err := git(dir, "100644 blob "+blob+"\t"+keep+"\n", "mktree")
	if err != nil {
		return "", err
	}
	var tree strings.Builder
	for _, d := range Layout {
		tree.WriteString("040000 tree " + sub + "\t" + d + "\n")
	}
	root, err := git(dir, tree.String(), "mktree")
	if err != nil {
		return "", err
	}
	commit, err := Git(dir, "commit-tree", root, "-m", "pm: empty records store")
	if err != nil {
		return "", err
	}
	if _, err := Git(dir, "push", "--quiet", remote, commit+":refs/heads/"+branch); err != nil {
		return "", err
	}
	if _, err := Git(dir, "fetch", "--quiet", remote, fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", branch, remote, branch)); err != nil {
		return "", err
	}
	if _, err := Git(dir, "branch", "--quiet", "--track", branch, remote+"/"+branch); err != nil {
		return "", err
	}
	dirs := make([]string, len(Layout))
	for i, d := range Layout {
		dirs[i] = d + "/"
	}
	return fmt.Sprintf("created the %s branch with an empty store (%s) and pushed it to %s", branch,
		strings.Join(dirs, ", "), remote), nil
}
