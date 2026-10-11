package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// clone is a main checkout on main with its store at .pm/store/records on branch records, as pm init leaves a clone
// (and conftest's repo fixture builds it).
func clone(t *testing.T) (root, store string) {
	t.Helper()
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "gitconfig")
	os.WriteFile(cfg, []byte("[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, _ = filepath.EvalSymlinks(tmp)
	root = filepath.Join(root, "repo")
	os.MkdirAll(root, 0o755)
	sh(t, root, "git init -q")
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/records\n/.pm/store\n"), 0o644)
	sh(t, root, "git add .gitignore", "git commit -qm code", "git worktree add -q --detach .pm/store/records")
	store = filepath.Join(root, ".pm/store/records")
	sh(t, store, "git checkout -q --orphan records", "git rm -rqf .")
	write(t, store, "projects/demo.md", project)
	sh(t, store, "git add -A", "git commit -qm records")
	return root, store
}

const project = "---\ntype: project\ntitle: Demo\nbead: demo\n---\n\n## Goal\n\nx\n"

func sh(t *testing.T, dir string, cmds ...string) string {
	t.Helper()
	var out []byte
	for _, c := range cmds {
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		var err error
		if out, err = cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s in %s: %v\n%s", c, dir, err, out)
		}
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, rel, text string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCommitCommitsOnlyThePathsNamed(t *testing.T) {
	root, store := clone(t)
	mine := write(t, store, "docs/a.md", "a")
	write(t, store, "docs/other.md", "another session's")
	if _, err := Commit(store, "pm: a", []string{mine}); err != nil {
		t.Fatal(err)
	}
	if got := sh(t, store, "git log -1 --format=%s --name-only"); got != "pm: a\n\ndocs/a.md" {
		t.Errorf("commit = %q", got)
	}
	dirty, err := Uncommitted(store, []string{mine, filepath.Join(store, "docs/other.md")})
	if err != nil || !reflect.DeepEqual(dirty, []string{"docs/other.md"}) {
		t.Errorf("Uncommitted = %v, %v", dirty, err)
	}
	if _, err := Commit(store, "x", nil); err == nil || err.Error() != "no records named to commit" {
		t.Errorf("no paths: %v", err)
	}
	// on the same clone, once the commits above are done: it moves the store off its branch, then away
	t.Run("FindChecksTheStore", func(t *testing.T) {
		got, err := Find(root)
		if err != nil || got != store {
			t.Fatalf("Find = %q, %v", got, err)
		}
		if got, _ := Find(filepath.Join(store, "projects")); got != store {
			t.Errorf("Find from inside the store = %q", got)
		}
		if code, _ := CodeRoot(store, store); code != root {
			t.Errorf("CodeRoot from the store = %q, want the main checkout", code)
		}
		if code, _ := CodeRoot(root, store); code != root {
			t.Errorf("CodeRoot = %q", code)
		}
		sh(t, store, "git checkout -q -b other")
		if _, err := Find(root); err == nil || err.Error() != store+" is not a worktree on branch records; move it away and run pm init" {
			t.Errorf("a store on another branch: %v", err)
		}
		os.RemoveAll(filepath.Join(root, ".pm"))
		if _, err := Find(root); err == nil || err.Error() != "no records store at "+store+"; set it up with pm init" {
			t.Errorf("no store: %v", err)
		}
	})
}

func TestApplyCommitsOrRestores(t *testing.T) {
	_, store := clone(t)
	p := filepath.Join(store, "projects/demo.md")
	if err := Apply(store, []Write{{Path: p, Text: project + "more\n"}}, "edit", "", "pm: "); err != nil {
		t.Fatal(err)
	}
	if got := sh(t, store, "git log -1 --format=%s"); got != "pm: edit" {
		t.Errorf("Apply committed %q", got)
	}
	if got := sh(t, store, "git status --porcelain"); got != "" {
		t.Errorf("after Apply the store is dirty: %q", got)
	}
	gitdir := sh(t, store, "git rev-parse --absolute-git-dir")
	write(t, gitdir, "index.lock", "") // git add now fails
	err := Apply(store, []Write{{Path: p, Text: "changed"}, {Path: filepath.Join(store, "docs/new.md"), Text: "new"}}, "edit", "bd delete x", "pm: ")
	if err == nil || !strings.HasPrefix(err.Error(), "committing failed: git add -A -- projects/demo.md docs/new.md failed in ") ||
		!strings.HasSuffix(err.Error(), "; restored records/projects/demo.md, records/docs/new.md to the state before this "+
			"write; undo the work-store step with: bd delete x") {
		t.Errorf("Apply with a failing commit: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != project+"more\n" {
		t.Errorf("not restored: %q", b)
	}
	if _, err := os.Stat(filepath.Join(store, "docs/new.md")); !os.IsNotExist(err) {
		t.Errorf("a new file stays after a failed write: %v", err)
	}
	t.Run("LockIsExclusive", func(t *testing.T) {
		dir := t.TempDir()
		unlock, err := Lock(dir)
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan bool)
		go func() {
			u, err := Lock(dir)
			if err == nil {
				u()
			}
			got <- true
		}()
		select {
		case <-got:
			t.Fatal("a second Lock did not wait")
		case <-time.After(100 * time.Millisecond):
		}
		unlock()
		select {
		case <-got:
		case <-time.After(5 * time.Second):
			t.Fatal("the second Lock did not get the lock after the first let go")
		}
	})
}

func TestApplyNeverOverwritesARecordItCannotRead(t *testing.T) {
	_, store := clone(t)
	p := filepath.Join(store, "projects/demo.md")
	os.Chmod(p, 0)
	defer os.Chmod(p, 0o644)
	err := Apply(store, []Write{{Path: p, Text: "changed"}}, "edit", "", "pm: ")
	if err == nil || !strings.HasSuffix(err.Error(), "; no record was changed") {
		t.Errorf("Apply on an unreadable record: %v", err)
	}
	os.Chmod(p, 0o644)
	if b, _ := os.ReadFile(p); string(b) != project {
		t.Errorf("the record changed: %q", b)
	}
}
