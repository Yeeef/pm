package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
	"github.com/Yeeef/yeeef-agents/pm/internal/work/worktest"
)

// A clone as conftest's repo fixture builds it: main checkout, config, the store on branch records holding one
// project and one sprint record.
func clone(t *testing.T) string {
	t.Helper()
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	cfg := filepath.Join(tmp, "gitconfig")
	os.WriteFile(cfg, []byte("[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := filepath.Join(tmp, "repo")
	os.MkdirAll(filepath.Join(root, ".pm"), 0o755)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/records\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".pm/config.toml"), []byte(`version = "9.9.9"
remote = "origin"
main_branch = "main"
port = 8000
`), 0o644)
	run(t, root, "git init -q", "git add .gitignore .pm", "git commit -qm code",
		"git worktree add -q --detach .pm/store/records")
	store := filepath.Join(root, ".pm/store/records")
	run(t, store, "git checkout -q --orphan records", "git rm -rqf .")
	for rel, text := range map[string]string{
		"projects/demo.md": "---\ntype: project\ntitle: Demo\nbead: demo\n---\n\n## Goal\n\nx\n\n## Progress\n\n" +
			"## Decisions\n\nNone yet.\n\n## Design pages\n\nNone yet.\n\n## Outcome\n\nNot closed yet.\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(store, rel)), 0o755)
		os.WriteFile(filepath.Join(store, rel), []byte(text), 0o644)
	}
	run(t, store, "git add -A", "git commit -qm records")
	return root
}

func run(t *testing.T, dir string, cmds ...string) {
	t.Helper()
	for _, c := range cmds {
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", c, err, out)
		}
	}
}

func TestCheckRendersEveryPageWithTheWorkStoresItems(t *testing.T) {
	root := clone(t)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake := worktest.New([]work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open,
		CreatedAt: at, UpdatedAt: at}})
	defer func(old func(string) (work.Store, error)) { OpenWork = old }(OpenWork)
	OpenWork = func(string) (work.Store, error) { return fake, nil }
	var out bytes.Buffer
	if err := cmdCheck(root, &out); err != nil {
		t.Fatal(err)
	}
	// Python: f"checked the records in {records}: all {n} pages render"; the project page and the index
	if want := "checked the records in " + filepath.Join(root, ".pm/store/records") + ": all 2 pages render\n"; out.String() != want {
		t.Errorf("pm check printed %q, want %q", out.String(), want)
	}

	OpenWork = func(string) (work.Store, error) { return worktest.New(nil), nil }
	if err := cmdCheck(root, &out); err == nil || err.Error() != "projects/demo: bead demo not found in Beads" {
		t.Errorf("a record whose item is missing: %v", err)
	}
}

func TestCheckReadsTheEmbeddedWorkStore(t *testing.T) {
	root := clone(t)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dir, run := work.Locations(root)
	d, err := work.CreateStore(work.Options{Dir: dir, RunDir: run})
	if err != nil {
		t.Fatal(err)
	}
	err = d.Import([]work.Item{{ID: "demo-a1b", Type: work.Project, Title: "Demo", Status: work.Open, CreatedAt: at,
		UpdatedAt: at}}, "seed")
	if err = errors.Join(err, d.Shutdown()); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, ".pm/store/records/projects/demo.md") // the store needs <prefix>-<root> ids
	b, _ := os.ReadFile(project)
	os.WriteFile(project, bytes.Replace(b, []byte("bead: demo\n"), []byte("bead: demo-a1b\n"), 1), 0o644)
	var out bytes.Buffer
	if err := cmdCheck(root, &out); err != nil {
		t.Fatal(err)
	}
	if want := "checked the records in " + filepath.Join(root, ".pm/store/records") + ": all 2 pages render\n"; out.String() != want {
		t.Errorf("pm check printed %q, want %q", out.String(), want)
	}
}

func TestCheckFailsWithoutAWorkStore(t *testing.T) {
	root := clone(t)
	defer func(v string) { buildinfo.Version = v }(buildinfo.Version)
	buildinfo.Version = "9.9.9"
	t.Chdir(root)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"check"}, nil, &stdout, &stderr)
	want := "error: work store: none at " + filepath.Join(root, ".pm/store/work") + "; pm init creates or clones it\n"
	if code != 1 || stderr.String() != want || stdout.Len() != 0 {
		t.Errorf("pm check: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
