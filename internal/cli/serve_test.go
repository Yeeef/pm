package cli

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
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
		if _, err := records.Texts(dir); err != nil {
			textsFailed++
			first = cmp.Or(first, err)
		}
	}
	t.Logf("%d checkouts, %d calls each: Stamp failed %d, Texts failed %d", checkouts, calls, stampFailed, textsFailed)
	if first != nil {
		t.Fatalf("Stamp or Texts failed while git rewrote the records; first: %v", first)
	}
}
