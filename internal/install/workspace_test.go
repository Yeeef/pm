package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A git too old for workspaces fails the workspace step for this worktree alone: pm init goes on, the worktree's
// records/ is the link to the shared store, and the output says so loudly; a new enough git makes the workspace.
func TestAnOldGitLeavesTheWorktreeOnTheLinkAndSaysSo(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "absent")) // no Claude Code settings to write
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "repo")
	run(t, dir, "git", "init", "-q", "-b", "main", main)
	run(t, main, "git", "commit", "-q", "--allow-empty", "-m", "code")
	records := filepath.Join(main, ".pm/store/records")
	run(t, main, "git", "worktree", "add", "-q", "--detach", records)
	run(t, records, "git", "checkout", "-q", "--orphan", "records")
	run(t, records, "git", "commit", "-q", "--allow-empty", "-m", "records")
	for _, name := range []string{"old", "new"} {
		run(t, main, "git", "worktree", "add", "-q", "-b", name, filepath.Join(dir, name))
	}

	saved := checkGit
	t.Cleanup(func() { checkGit = saved })
	checkGit = func() error { return errors.New("git 2.39 is older than 2.40, which records workspaces need") }
	old := filepath.Join(dir, "old")
	said, err := setupRecords(old, main, records)
	if err != nil {
		t.Fatalf("setupRecords with an old git: %v", err)
	}
	if text := strings.Join(said, "\n"); !strings.Contains(text, "WARNING: no records workspace for "+old+": git 2.39 "+
		"is older than 2.40") || !strings.Contains(text, "its records/ is the link to the shared store") {
		t.Errorf("said %q", text)
	}
	if target, err := os.Readlink(filepath.Join(old, "records")); err != nil || target != records {
		t.Errorf("records/ is no link to the store: %q, %v", target, err)
	}

	checkGit = saved
	if _, err := setupRecords(filepath.Join(dir, "new"), main, records); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(filepath.Join(dir, "new", "records", ".git")); err != nil || !st.Mode().IsRegular() {
		t.Errorf("a new enough git made no workspace: %v", err)
	}
}
