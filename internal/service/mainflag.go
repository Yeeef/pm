package service

import (
	"errors"
	"fmt"
	"html"
	"os/exec"
	"strconv"
	"strings"
)

// MainFlag is the line that flags this clone's main checkout, which hooks, rules and links read, when it is off the
// main branch or behind <remote>/<main branch> as of the last fetch, with the command that fixes it; "" when it is
// on the main branch and current, or when no remote-tracking ref exists yet. The service never pulls it itself.
func (t Texts) MainFlag() (string, error) {
	quoted := shellQuote(t.Main)
	out, err := exec.Command("git", "-C", t.Main, "symbolic-ref", "--quiet", "HEAD").Output()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return fmt.Sprintf("main checkout %s has a detached HEAD, not %s, so hooks and rules read another tree; run "+
			"git -C %s switch %s", t.Main, t.MainBranch, quoted, t.MainBranch), nil
	case err != nil:
		return "", fmt.Errorf("main checkout %s: git symbolic-ref HEAD: %v", t.Main, err)
	}
	// the full ref: --short prints heads/<main> when a tag of that name exists too
	if on := strings.TrimSpace(string(out)); on != "refs/heads/"+t.MainBranch {
		return fmt.Sprintf("main checkout %s is on branch %s, not %s, so hooks and rules read another tree; run "+
			"git -C %s switch %s", t.Main, strings.TrimPrefix(on, "refs/heads/"), t.MainBranch, quoted, t.MainBranch), nil
	}
	ref := "refs/remotes/" + t.Remote + "/" + t.MainBranch
	if exec.Command("git", "-C", t.Main, "rev-parse", "--verify", "--quiet", ref).Run() != nil {
		return "", nil // never fetched: nothing to count against
	}
	out, err = exec.Command("git", "-C", t.Main, "rev-list", "--left-right", "--count", "HEAD..."+ref).Output()
	if err != nil {
		return "", fmt.Errorf("main checkout %s: git rev-list --left-right --count HEAD...%s: %v", t.Main, ref, err)
	}
	counts := strings.Fields(string(out))
	if len(counts) != 2 {
		return "", fmt.Errorf("main checkout %s: git rev-list --left-right --count printed %q", t.Main, out)
	}
	ahead, err1 := strconv.Atoi(counts[0])
	behind, err2 := strconv.Atoi(counts[1])
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("main checkout %s: git rev-list --left-right --count printed %q", t.Main, out)
	}
	switch {
	case behind == 0:
		return "", nil
	case ahead > 0: // pull --ff-only would fail
		return fmt.Sprintf("main checkout %s has diverged from %s/%s (%d commit(s) behind, %d ahead), so hooks and "+
			"rules run old code; inspect its own commits (git -C %s log %s/%s..HEAD), then rebase or reset it onto "+
			"%s/%s", t.Main, t.Remote, t.MainBranch, behind, ahead, quoted, t.Remote, t.MainBranch, t.Remote,
			t.MainBranch), nil
	}
	return fmt.Sprintf("main checkout %s is %d commit(s) behind %s/%s, so hooks and rules run old code; run %s",
		t.Main, behind, t.Remote, t.MainBranch, t.pullMain()), nil
}

// mainBanner is the site's warning for MainFlag, beside the push banner; empty when there is no flag.
func (t Texts) mainBanner() string {
	line, err := t.MainFlag()
	if err != nil {
		line = err.Error()
	}
	if line == "" {
		return ""
	}
	return `<div class="note draft push"><p><strong>Main checkout</strong> — needs attention:</p><ul><li>` +
		html.EscapeString(line) + `</li></ul></div>`
}
