package cli

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
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

	s, err := newServedSite(dir, "t")
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

// Load reads every record lock-free while a records sync may rewrite them; a file that went mid-read is left out of
// that read, so the read must not pass for the whole store: Load says which went (a service.Partial), and the service
// reads again under the records lock, which the sync's rebase holds. A Load that succeeds has every record.
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

	s, err := newServedSite(dir, "t")
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
}
