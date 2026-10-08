package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{"summary", func() (bool, string) { return false, "claude is missing" }},
		{"records", func() (bool, string) { return true, "up to date with origin/records" }}}
	code, out, err := Push(main, steps)
	want := "2026-10-08T10:00:00+00:00 work ok: pushed 2 commits\n2026-10-08T10:00:00+00:00 summary error: claude is " +
		"missing\n2026-10-08T10:00:00+00:00 records ok: up to date with origin/records"
	if err != nil || code != 1 || out != want {
		t.Fatalf("%d %v\n%s", code, err, out)
	}
	_, log, _ := Files(main)
	if data, _ := os.ReadFile(log); string(data) != want+"\n" {
		t.Fatalf("log %q", data)
	}
	flags, _ := Flags(main, "", "origin")
	if len(flags) != 1 || flags[0] != "summary step failed at 2026-10-08T10:00:00+00:00: claude is missing; log "+log {
		t.Fatal(flags)
	}
	banner, _ := Banner(main, "", "origin")
	if !strings.Contains(banner, "<li>summary step failed at") {
		t.Fatal(banner)
	}
	d, _ := Describe(main)
	if len(d) != 4 || d[1] != "push      summary error at 2026-10-08T10:00:00+00:00: claude is missing" {
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
	if len(flags) != 3 || !strings.HasPrefix(flags[0], "work push overdue: last successful push 2026-10-08T10:00:00+00:00, 31 min ago (the pm service pushes every 10 min)") {
		t.Fatal(flags)
	}
}

func TestANeverSuccessfulPushIsOverdueFromTheInstall(t *testing.T) {
	main := t.TempDir()
	at(t, "2026-10-08T10:00:00Z")
	if err := MarkInstalled(main); err != nil {
		t.Fatal(err)
	}
	if flags, _ := Flags(main, "", "origin"); len(flags) != 0 {
		t.Fatal(flags)
	}
	at(t, "2026-10-08T11:00:00Z")
	MarkInstalled(main) // keeps the first install's time
	flags, _ := Flags(main, "", "origin")
	if len(flags) != 3 || !strings.HasPrefix(flags[2], "records push overdue: no successful push since 2026-10-08T10:00:00+00:00, 60 min ago") {
		t.Fatal(flags)
	}
}

func TestASecondRunSkipsWhileOneHoldsTheLock(t *testing.T) {
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
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPushRecordsPushesRebasesAndReports(t *testing.T) {
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
	if ok, said := PushRecords(store, "origin"); ok { // origin has no records branch yet: fetch fails
		t.Fatal(said)
	}
	git(t, store, "push", "-q", "origin", "HEAD:"+Branch)
	if ok, said := PushRecords(store, "origin"); !ok || said != "up to date with origin/records" {
		t.Fatal(ok, said)
	}
	commit(store, "b.md")
	if ok, said := PushRecords(store, "origin"); !ok || said != "pushed 1 commit(s)" {
		t.Fatal(ok, said)
	}
	git(t, tmp, "clone", "-q", "-b", Branch, origin, other)
	commit(other, "c.md")
	git(t, other, "push", "-q", "origin", "HEAD:"+Branch)
	if ok, said := PushRecords(store, "origin"); !ok || said != "up to date with origin/records (1 behind; not pulled)" {
		t.Fatal(ok, said)
	}
	commit(store, "d.md")
	if ok, said := PushRecords(store, "origin"); !ok || said != "pushed 1 commit(s) after rebasing onto 1 new on origin/records" {
		t.Fatal(ok, said)
	}
}
