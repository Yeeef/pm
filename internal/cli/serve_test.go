package cli

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// A records sync that rebases rewrites record files: git unlinks each one and writes it anew, and drops a directory
// whose files all went. The service's per-second Stamp, and the Load that reads the records after it, walk the store
// meanwhile, so a file or directory listed by the walk can be gone when it is read. Neither may fail for that.
func TestStampAndTextsSurviveGitRewritingTheRecordsMidWalk(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) error {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v\n%s", args, err, out)
		}
		return nil
	}
	run := func(args ...string) {
		t.Helper()
		if err := git(args...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, text string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Commit a: 200 records and a directory of 20 more. Commit b: the 200 rewritten, the directory gone.
	run("init", "-q", "-b", store.Branch)
	for i := range 200 {
		write(fmt.Sprintf("sprints/s-%03d.md", i), fmt.Sprintf("# sprint %d\n\nversion a\n", i))
	}
	for i := range 20 {
		write(fmt.Sprintf("gone/g-%02d.md", i), "# gone\n")
	}
	run("add", "-A")
	run("commit", "-qm", "a")
	run("tag", "a")
	run("rm", "-rq", "gone")
	for i := range 200 {
		write(fmt.Sprintf("sprints/s-%03d.md", i), fmt.Sprintf("# sprint %d\n\nversion b, longer\n", i))
	}
	run("add", "-A")
	run("commit", "-qm", "b")
	run("tag", "b")
	run("checkout", "-q", "--detach", "a") // a's tree, with HEAD detached so each checkout below moves only files

	s, err := newServedSite(dir, "t", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	const checkouts = 40
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range checkouts {
			if err := git("checkout", "-q", "--detach", []string{"b", "a"}[i%2]); err != nil {
				t.Error(err) // not Fatal: this is not the test's goroutine
				return
			}
		}
	}()
	calls, stampFailed, textsFailed := 0, 0, 0
	var first error
	for walking := true; walking; {
		select {
		case <-done:
			walking = false
		default:
		}
		calls++
		if _, err := s.Stamp(); err != nil {
			stampFailed++
			first = cmp.Or(first, err)
		}
		if _, _, err := records.Texts(dir); err != nil {
			textsFailed++
			first = cmp.Or(first, err)
		}
	}
	t.Logf("%d checkouts, %d calls each: Stamp failed %d, Texts failed %d", checkouts, calls, stampFailed, textsFailed)
	if first != nil {
		t.Fatalf("Stamp or Texts failed while git rewrote the records; first: %v", first)
	}
}

// Load reads lock-free while a records sync may rewrite the store's worktree: the published records come from the
// records branch, so a Load that succeeds has every record; drafts read while git rewrote the worktree make it a
// service.Partial, and the service reads again under the records lock, which the sync's rebase holds.
func TestLoadNeverPassesARecordSetMissingAFileGitRewrote(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) error {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v\n%s", args, err, out)
		}
		return nil
	}
	run := func(args ...string) {
		t.Helper()
		if err := git(args...); err != nil {
			t.Fatal(err)
		}
	}
	const docs = 100
	write := func(version string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		for i := range docs {
			text := fmt.Sprintf("---\ntype: doc\ntitle: Doc %d\ndate: 2026-10-02\nproject: demo\n---\n\n# Doc %d\n\n%s\n", i, i,
				version)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("docs/2026-10-02-d%03d.md", i)), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	run("init", "-q", "-b", store.Branch)
	project := "---\ntype: project\ntitle: Demo\nbead: demo\n---\n\n## Goal\n\nA goal.\n\n## Progress\n\n## Decisions\n\n" +
		"None yet.\n\n## Design pages\n\nNone yet.\n\n## Outcome\n\nNot closed yet.\n"
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "projects/demo.md"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	items := []work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open}}
	write("version a")
	run("add", "-A")
	run("commit", "-qm", "a")
	run("tag", "a")
	write("version b, longer")
	run("commit", "-qam", "b")
	run("tag", "b")
	run("checkout", "-q", "--detach", "a")

	s, err := newServedSite(dir, "t", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	const checkouts = 40
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range checkouts {
			if err := git("checkout", "-q", "--detach", []string{"b", "a"}[i%2]); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	t.Cleanup(func() { <-done })
	loads, failed, partial, short := 0, 0, 0, 0
	for walking := true; walking; {
		select {
		case <-done:
			walking = false
		default:
		}
		loads++
		pages, err := s.Load(items)
		if err != nil { // the service reads again under the records lock: a file that went, or one git is still writing
			var p *service.Partial
			if errors.As(err, &p) {
				partial++
			}
			failed++
			continue
		}
		for i := range docs {
			if _, found, err := pages.Page(fmt.Sprintf("docs/2026-10-02-d%03d.html", i)); err != nil || !found {
				short++
				break
			}
		}
	}
	t.Logf("%d checkouts, %d loads: %d failed (%d a file that went), %d passed without every record", checkouts, loads,
		failed, partial, short)
	if short > 0 {
		t.Fatalf("%d of %d loads passed without every record while git rewrote them", short, loads)
	}
	t.Run("CheckRendersEveryPageWithTheWorkStoresItems", func(t *testing.T) {
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
		// Python: f"checked the records in {records}: all {n} pages render"; the project page, the index and the docs index
		if want := "checked the records in " + filepath.Join(root, ".pm/store/records") + ": all 3 pages render\n"; out.String() != want {
			t.Errorf("pm check printed %q, want %q", out.String(), want)
		}

		OpenWork = func(string) (work.Store, error) { return worktest.New(nil), nil }
		if err := cmdCheck(root, &out); err == nil || err.Error() != "projects/demo: bead demo not found in the work store" {
			t.Errorf("a record whose item is missing: %v", err)
		}
	})
	t.Run("CheckReadsTheWorkStoreTheServiceHolds", func(t *testing.T) {
		root := clone(t)
		at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		_, d := worktest.ServeAt(t, root, work.Ops{})
		err := d.Import([]work.Item{{ID: "demo-a1b", Type: work.Project, Title: "Demo", Status: work.Open, CreatedAt: at,
			UpdatedAt: at}}, "seed")
		if err := errors.Join(err, d.Shutdown()); err != nil {
			t.Fatal(err)
		}
		project := filepath.Join(root, ".pm/store/records/projects/demo.md") // the store needs <prefix>-<root> ids
		b, _ := os.ReadFile(project)
		os.WriteFile(project, bytes.Replace(b, []byte("bead: demo\n"), []byte("bead: demo-a1b\n"), 1), 0o644)
		var out bytes.Buffer
		if err := cmdCheck(root, &out); err != nil {
			t.Fatal(err)
		}
		if want := "checked the records in " + filepath.Join(root, ".pm/store/records") + ": all 3 pages render\n"; out.String() != want {
			t.Errorf("pm check printed %q, want %q", out.String(), want)
		}
	})
	t.Run("CheckFailsWithoutTheServiceOrAWorkStore", func(t *testing.T) {
		root := clone(t)
		defer func(v string) { buildinfo.Version = v }(buildinfo.Version)
		buildinfo.Version = "9.9.9"
		t.Chdir(root)
		check := func(want string) {
			t.Helper()
			var stdout, stderr bytes.Buffer
			if code := Execute([]string{"check"}, nil, &stdout, &stderr); code != 1 || stderr.String() != want ||
				stdout.Len() != 0 {
				t.Errorf("pm check: exit %d, stdout %q, stderr %q; want %q", code, stdout.String(), stderr.String(), want)
			}
		}
		check("error: the pm service does not answer on " + work.Sock(root) + "; pm reaches the work store only " +
			"through it: run pm service restart\n")
		h, err := work.NewHost(work.HostOptions{Main: root, Version: buildinfo.Version})
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		check("error: this clone has no work store yet: run pm init\n")
	})
}

// A clone as conftest's repo fixture builds it: main checkout, config, the store on branch records holding one
// project and one sprint record.
func clone(t *testing.T) string {
	t.Helper()
	tmp, err := os.MkdirTemp("", "pm") //nolint:usetesting // short: the clone's service socket must fit the kernel's limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	tmp, _ = filepath.EvalSymlinks(tmp)
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

// A commit of a hand edit moves only the records branch, the working tree already holding its text: the stamp moves
// with the branch's ref, and the next look publishes the edit and drops the draft.
func TestACommittedEditIsPublishedAndLeavesNoDraftAtTheNextLook(t *testing.T) {
	root := clone(t)
	records := filepath.Join(root, ".pm/store/records")
	s, err := newServedSite(records, "t", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	items := []work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open}}
	project := filepath.Join(records, "projects/demo.md")
	b, err := os.ReadFile(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, bytes.Replace(b, []byte("## Goal\n\nx\n"), []byte("## Goal\n\nShip it.\n"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	page := func(pages service.Pages, path string) (string, bool) {
		t.Helper()
		text, found, err := pages.Page(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return text, found
	}
	pages, err := s.Load(items)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := page(pages, "projects/demo.html"); strings.Contains(text, "Ship it.") {
		t.Fatal("an uncommitted edit is published")
	}
	if text, found := page(pages, "drafts/store/projects/demo.html"); !found || !strings.Contains(text, "Ship it.") {
		t.Fatalf("the edit is no version: found %v", found)
	}
	before, err := s.Stamp()
	if err != nil {
		t.Fatal(err)
	}
	run(t, records, "git commit -qam goal")
	if after, err := s.Stamp(); err != nil || after == before {
		t.Fatalf("the stamp did not move with the records branch: %v", err)
	}
	if pages, err = s.Load(items); err != nil {
		t.Fatal(err)
	}
	if text, _ := page(pages, "projects/demo.html"); !strings.Contains(text, "Ship it.") {
		t.Error("the committed edit is not published at the next look")
	}
	if _, found := page(pages, "drafts/store/projects/demo.html"); found {
		t.Error("the committed edit is still a draft")
	}
}
