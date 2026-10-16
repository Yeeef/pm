package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/store"
)

func at(t *testing.T, when string) {
	t.Helper()
	fixed, err := time.Parse(time.RFC3339, when)
	if err != nil {
		t.Fatal(err)
	}
	saved := Now
	Now = func() time.Time { return fixed }
	t.Cleanup(func() { Now = saved })
}

func TestARunRecordsEachStepAndFlagsAFailure(t *testing.T) {
	main := t.TempDir()
	at(t, "2026-10-08T10:00:00Z")
	if d, _ := Describe(main); len(d) != 1 || !strings.HasSuffix(d[0], "push.log  no push recorded yet") {
		t.Fatal(d)
	}
	steps := []Step{{"work", func() (bool, string) { return true, "pushed 2 commits" }},
		{"summary", func() (bool, string) { panic("claude is missing") }},
		{"records", func() (bool, string) { return true, "up to date with origin/records" }},
		{"drafts", func() (bool, string) { return true, "pushed 0 and deleted 0 draft refs" }}}
	code, out, err := Push(main, steps)
	want := "2026-10-08T10:00:00+00:00 work ok: pushed 2 commits\n2026-10-08T10:00:00+00:00 summary error: panic: claude is " +
		"missing\n2026-10-08T10:00:00+00:00 records ok: up to date with origin/records\n2026-10-08T10:00:00+00:00 drafts ok: " +
		"pushed 0 and deleted 0 draft refs"
	if err != nil || code != 1 || out != want {
		t.Fatalf("%d %v\n%s", code, err, out)
	}
	_, log, _ := Files(main)
	if data, _ := os.ReadFile(log); string(data) != want+"\n" {
		t.Fatalf("log %q", data)
	}
	flags, _ := Flags(main, "", "origin")
	if len(flags) != 1 || flags[0] != "summary step failed at 2026-10-08T10:00:00+00:00: panic: claude is missing; log "+log {
		t.Fatal(flags)
	}
	banner, _ := Banner(main, "", "origin")
	if !strings.Contains(banner, "<li>summary step failed at") {
		t.Fatal(banner)
	}
	d, _ := Describe(main)
	if len(d) != 5 || d[1] != "push      summary error at 2026-10-08T10:00:00+00:00: panic: claude is missing" {
		t.Fatal(d)
	}
	steps[1].Run = func() (bool, string) { return true, "summarized" }
	if code, _, _ := Push(main, steps); code != 0 {
		t.Fatal(code)
	}
	if flags, _ := Flags(main, "", "origin"); len(flags) != 0 {
		t.Fatal(flags)
	}
	at(t, "2026-10-08T10:31:00Z") // past Overdue (30 min) since the last success
	flags, _ = Flags(main, "", "origin")
	if len(flags) != 4 || !strings.HasPrefix(flags[0], "work push overdue: last successful push 2026-10-08T10:00:00+00:00, 31 min ago (the pm service pushes every 10 min)") {
		t.Fatal(flags)
	}
	t.Run("ASecondRunSkipsWhileOneHoldsTheLock", func(t *testing.T) {
		main := t.TempDir()
		release := make(chan struct{})
		started := make(chan struct{})
		first := make(chan struct{})
		go func() {
			Push(main, []Step{{"work", func() (bool, string) { close(started); <-release; return true, "" }}})
			close(first)
		}()
		<-started
		code, out, err := Push(main, nil)
		close(release)
		<-first
		if code != 0 || out != "another pm push holds the lock; skipped" || err != nil {
			t.Fatal(code, out, err)
		}
	})
}

func TestANeverSuccessfulPushIsOverdueFromTheInstall(t *testing.T) {
	main := t.TempDir()
	at(t, "2026-10-08T10:00:00Z")
	if err := StartClocks(main); err != nil {
		t.Fatal(err)
	}
	if flags := flagged(t, main); len(flags) != 0 {
		t.Fatal(flags)
	}
	if d := described(t, main); len(d) != 1 || !strings.HasSuffix(d[0], "no push recorded yet") {
		t.Fatal(d)
	}
	at(t, "2026-10-08T11:00:00Z")
	if err := StartClocks(main); err != nil { // a later start keeps each step's first
		t.Fatal(err)
	}
	flags := flagged(t, main)
	if len(flags) != 4 || !strings.HasPrefix(flags[2], "records push overdue: no successful push since 2026-10-08T10:00:00+00:00, 60 min ago") {
		t.Fatal(flags)
	}
}

// A step that a later pm adds (drafts, in 0.15.1) is counted from the start of the first service that runs it, not
// from the clone's install: a state written by an older pm has installed_at and the other steps, no drafts.
func TestAStepALaterPmAddsIsOverdueFromItsFirstStart(t *testing.T) {
	main := t.TempDir()
	ok := "2026-10-16T09:55:00+00:00"
	done := map[string]any{"at": ok, "ok": true, "message": "done", "last_ok": ok, "first": "2026-10-01T00:00:00+00:00"}
	if err := WriteState(main, State{"installed_at": "2026-10-01T00:00:00+00:00", "work": done, "summary": done, "records": done}); err != nil {
		t.Fatal(err)
	}
	at(t, "2026-10-16T10:00:00Z")
	if flags := flagged(t, main); len(flags) != 0 { // the new pm reads it before its service started
		t.Fatal(flags)
	}
	if err := StartClocks(main); err != nil {
		t.Fatal(err)
	}
	if flags := flagged(t, main); len(flags) != 0 {
		t.Fatal(flags)
	}
	at(t, "2026-10-16T10:31:00Z") // the others pushed at 09:55: the overdue clock runs for all four
	flags := flagged(t, main)
	if len(flags) != 4 || !strings.HasPrefix(flags[3], "drafts push overdue: no successful push since 2026-10-16T10:00:00+00:00, 31 min ago") {
		t.Fatal(flags)
	}
	steps := []Step{{"work", func() (bool, string) { return true, "" }}, {"summary", func() (bool, string) { return true, "" }},
		{"records", func() (bool, string) { return true, "" }}, {"drafts", func() (bool, string) { return false, "lease failed" }}}
	if _, _, err := Push(main, steps); err != nil {
		t.Fatal(err)
	}
	if s, _ := state(t, main).step("drafts"); s.First != "2026-10-16T10:00:00+00:00" || s.LastOK != nil { // the run keeps the first
		t.Fatal(s)
	}
	at(t, "2026-10-16T10:45:00Z")
	if flags := flagged(t, main); len(flags) != 1 || !strings.HasPrefix(flags[0], "drafts push failed at 2026-10-16T10:31:00+00:00: lease failed") {
		t.Fatal(flags)
	}
}

// StartClocks leaves the state to a push under way, which records every step itself.
func TestStartClocksLeavesTheStateToAPushUnderWay(t *testing.T) {
	main := t.TempDir()
	unlock, ok, err := lock(main)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := StartClocks(main); err != nil {
		t.Fatal(err)
	}
	if s := state(t, main); len(s) != 0 {
		t.Fatal(s)
	}
	unlock()
	if err := StartClocks(main); err != nil {
		t.Fatal(err)
	}
	if s := state(t, main); len(s) != 4 {
		t.Fatal(s)
	}
}

func flagged(t *testing.T, main string) []string {
	t.Helper()
	flags, err := Flags(main, "", "origin")
	if err != nil {
		t.Fatal(err)
	}
	return flags
}

func described(t *testing.T, main string) []string {
	t.Helper()
	d, err := Describe(main)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func state(t *testing.T, main string) State {
	t.Helper()
	s, err := ReadState(main)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestPushRecordsPushesRebasesAndReports(t *testing.T) {
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t") // the rebase commits; a CI runner has no git identity
	}
	tmp := t.TempDir()
	origin, store, other := filepath.Join(tmp, "origin.git"), filepath.Join(tmp, "store"), filepath.Join(tmp, "other")
	git(t, tmp, "init", "-q", "--bare", origin)
	git(t, tmp, "clone", "-q", origin, store)
	git(t, store, "checkout", "-q", "-b", Branch)
	commit := func(dir, file string) {
		os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644)
		git(t, dir, "add", file)
		git(t, dir, "commit", "-q", "-m", file)
	}
	commit(store, "a.md")
	if ok, said := PushRecords(store, "origin", noneClosed); ok { // origin has no records branch yet: fetch fails
		t.Fatal(said)
	}
	git(t, store, "push", "-q", "origin", "HEAD:"+Branch)
	if ok, said := PushRecords(store, "origin", noneClosed); !ok || said != "up to date with origin/records" {
		t.Fatal(ok, said)
	}
	commit(store, "b.md")
	if ok, said := PushRecords(store, "origin", noneClosed); !ok || said != "pushed 1 commit(s)" {
		t.Fatal(ok, said)
	}
	git(t, tmp, "clone", "-q", "-b", Branch, origin, other)
	commit(other, "c.md")
	git(t, other, "push", "-q", "origin", "HEAD:"+Branch)
	if ok, said := PushRecords(store, "origin", noneClosed); !ok || said != "fast-forwarded 1 commit(s) from origin/records" {
		t.Fatal(ok, said)
	}
	if _, err := os.Stat(filepath.Join(store, "c.md")); err != nil {
		t.Fatal("the fast-forward did not bring c.md:", err)
	}
	put(t, filepath.Join(store, "e.md"), "mine") // untracked: the other clone adds it next
	commit(other, "e.md")
	git(t, other, "push", "-q", "origin", "HEAD:"+Branch)
	if ok, said := PushRecords(store, "origin", noneClosed); ok || !strings.Contains(said, "not fast-forwarded") {
		t.Fatal("a fast-forward over an uncommitted file:", ok, said)
	}
	if b := get(t, filepath.Join(store, "e.md")); b != "mine" {
		t.Fatalf("the refused fast-forward changed e.md: %q", b)
	}
	if err := os.Remove(filepath.Join(store, "e.md")); err != nil {
		t.Fatal(err)
	}
	commit(store, "d.md")
	if ok, said := PushRecords(store, "origin", noneClosed); !ok || said != "pushed 1 commit(s) after rebasing onto 1 new on origin/records" {
		t.Fatal(ok, said)
	}
}

func noneClosed() (store.SprintClosed, error) { return func(string) bool { return false }, nil }

// A store only behind is fast-forwarded only when it is safe: never with an autostash (the stash is every worktree's),
// never over a tracked edit, an untracked or ignored file the remote adds, or a HEAD off the records branch.
func TestAFastForwardRefusesWhatItWouldOverwriteOrStash(t *testing.T) {
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t")
	}
	tmp := t.TempDir()
	origin, store, other := filepath.Join(tmp, "origin.git"), filepath.Join(tmp, "store"), filepath.Join(tmp, "other")
	git(t, tmp, "init", "-q", "--bare", origin)
	git(t, tmp, "clone", "-q", origin, store)
	git(t, store, "checkout", "-q", "-b", Branch)
	git(t, store, "config", "merge.autoStash", "true")
	put(t, filepath.Join(store, "a.md"), "a\n")
	put(t, filepath.Join(store, ".gitignore"), "ignored.md\n")
	git(t, store, "add", "a.md", ".gitignore")
	git(t, store, "commit", "-q", "-m", "a")
	git(t, store, "push", "-q", "origin", "HEAD:"+Branch)
	git(t, tmp, "clone", "-q", "-b", Branch, origin, other)
	put(t, filepath.Join(other, "a.md"), "a from the other clone\n")
	put(t, filepath.Join(other, "ignored.md"), "theirs\n")
	git(t, other, "add", "-f", "a.md", "ignored.md")
	git(t, other, "commit", "-q", "-m", "other")
	git(t, other, "push", "-q", "origin", "HEAD:"+Branch)
	stashes := func() string {
		out, _ := exec.Command("git", "-C", store, "stash", "list").Output() //nolint:errcheck // none: empty
		return string(out)
	}

	put(t, filepath.Join(store, "a.md"), "my edit\n") // a tracked edit the remote's commit changes
	if ok, said := PushRecords(store, "origin", noneClosed); ok || !strings.Contains(said, "uncommitted changes") {
		t.Errorf("over a tracked edit: %v %s", ok, said)
	}
	if get(t, filepath.Join(store, "a.md")) != "my edit\n" || stashes() != "" {
		t.Errorf("the refused fast-forward moved the edit: %q, stash %q", get(t, filepath.Join(store, "a.md")), stashes())
	}
	git(t, store, "checkout", "--", "a.md")

	put(t, filepath.Join(store, "ignored.md"), "mine\n") // ignored here, added by the remote
	if ok, said := PushRecords(store, "origin", noneClosed); ok || !strings.Contains(said, "the remote adds ignored.md") {
		t.Errorf("over an ignored file: %v %s", ok, said)
	}
	if get(t, filepath.Join(store, "ignored.md")) != "mine\n" {
		t.Error("the refused fast-forward overwrote the ignored file")
	}
	if err := os.Remove(filepath.Join(store, "ignored.md")); err != nil {
		t.Fatal(err)
	}

	git(t, store, "checkout", "-q", "--detach")
	if ok, said := PushRecords(store, "origin", noneClosed); ok || !strings.Contains(said, "is not on the records branch") {
		t.Errorf("with HEAD detached: %v %s", ok, said)
	}
	git(t, store, "checkout", "-q", Branch)
	if ok, said := PushRecords(store, "origin", noneClosed); !ok || said != "fast-forwarded 1 commit(s) from origin/records" {
		t.Errorf("the fast-forward: %v %s", ok, said)
	}
	if stashes() != "" || get(t, filepath.Join(store, "a.md")) != "a from the other clone\n" {
		t.Errorf("after the fast-forward: stash %q, a.md %q", stashes(), get(t, filepath.Join(store, "a.md")))
	}
}
