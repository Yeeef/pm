package setup

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/store"
	"io"
	"strings"
)

// GitPostCheckout is pm hook git-post-checkout: in a new worktree (previous HEAD all zeros), pm init's clone and
// worktree half, all but the pm service. The store's own checkout, which that half itself makes, is skipped. A failure
// is printed and exits 1, which git reports without undoing the checkout.
func GitPostCheckout(p Arguments, here string, stdout, stderr io.Writer) error {
	args := p.ValuesOf("git_args")
	if len(args) == 0 || strings.Trim(args[0], "0") != "" || args[0] == "" {
		return nil
	}
	err := func() error {
		records, err := store.PathOf(here)
		if err != nil {
			return err
		}
		top, err := install.Git(here, "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		if fsutil.Resolved(top) == fsutil.Resolved(records) {
			return nil
		}
		c, err := config.Load(here)
		if err != nil {
			return err
		}
		done, err := install.SetupClone(here, c.Remote)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, done)
		return err
	}()
	if err != nil {
		fmt.Fprintf(stderr, "pm hook git-post-checkout: %s\n", err)
		return ExitError{1}
	}
	return nil
}

// hookGitPreCommit is pm hook git-pre-commit, retired with the main branch's records/ copy, whose guard it was: it does
// nothing. An earlier pm's section in the main checkout's .pm/hooks/pre-commit runs it in every worktree of the clone
// until pm upgrade's removal of that section reaches the main checkout, so a worktree already pinned to this pm must
// still commit.
func GitPreCommit() error { return nil }
