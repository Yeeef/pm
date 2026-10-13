package design

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/records"
)

func page(t *testing.T, rel, header string) *records.Record {
	t.Helper()
	rec, err := records.Parse(rel+".md", rel, "---\ntype: design\ntitle: T\nproject: demo\n"+header+"---\n\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestCheckListsDeadPatternsAsWaitingAndUncoveredSources(t *testing.T) {
	tracked := map[string]bool{"internal/a/a.go": true, "internal/b/b.go": true, "prime.md": true, "README.md": true}
	recs := []*records.Record{page(t, "design/a", "covers: [internal/a/**]\n"), page(t, "design/none", "")}
	got, waiting, err := Check(recs, tracked, []string{"internal/**", "prime.md"})
	if err != nil || !reflect.DeepEqual(got, []string{"internal/b/b.go", "prime.md"}) || waiting != nil {
		t.Errorf("Check = %v, %v, %v; want the two uncovered sources", got, waiting, err)
	}
	if got, waiting, err := Check(recs, tracked, nil); err != nil || got != nil || waiting != nil {
		t.Errorf("Check without sources = %v, %v, %v; want nothing", got, waiting, err)
	}
	recs = append(recs, page(t, "design/dead", "covers: [internal/c/**, prime.md, internal/d.go]\n"))
	got, waiting, err = Check(recs, tracked, []string{"internal/**"})
	want := []string{`design/dead.md: covers pattern "internal/c/**"`, `design/dead.md: covers pattern "internal/d.go"`}
	if err != nil || !reflect.DeepEqual(waiting, want) || !reflect.DeepEqual(got, []string{"internal/b/b.go"}) {
		t.Errorf("Check with dead patterns = %v, %v, %v; want them waiting, not fatal", got, waiting, err)
	}
	_, _, err = Check(recs, tracked, []string{"internal/**", "lib/**"})
	if err == nil || err.Error() != `design_sources pattern "lib/**" matches no tracked file; fix or remove it` {
		t.Errorf("Check with a dead source pattern: %v, want it fatal", err)
	}
}

func TestMainFilesReadsTheTreeAtARefNotTheWorktree(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	for _, f := range []string{"a/x.go", "b.md"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-qm", "c")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "a/new.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "unmerged")
	files, err := MainFiles(filepath.Join(dir, "a"), "refs/remotes/origin/main")
	if err != nil || !reflect.DeepEqual(files, map[string]bool{"a/x.go": true, "b.md": true}) {
		t.Errorf("MainFiles = %v, %v; want the ref's two files", files, err)
	}
	recs := []*records.Record{page(t, "design/a", "covers: [a/**, a/new.go]\n")}
	if got := Dead(recs, files, (*records.Record).Covers); !reflect.DeepEqual(got, []string{`design/a.md: covers pattern "a/new.go"`}) {
		t.Errorf("Dead = %v; want the unmerged file's pattern", got)
	}
}

func TestWaiversReadWhatWaiverTextWrites(t *testing.T) {
	text := "---\ntype: sprint\ntitle: S\nbead: demo.1\n---\n\n## Decisions\n\n::: decision {source=agent date=2026-10-12}\n" +
		WaiverText("parser", "The change renames a test only.") + "\n:::\n\n::: decision {source=agent date=2026-10-12}\n" +
		"The design page lexer stays as it is.\nNot the fixed form.\n:::\n"
	rec, err := records.Parse("sprints/demo-1.md", "sprints/demo-1", text)
	if err != nil {
		t.Fatal(err)
	}
	if got := Waivers(rec); !reflect.DeepEqual(got, map[string]string{"parser": "The change renames a test only."}) {
		t.Errorf("Waivers = %v", got)
	}
}

// --merged without --pr: a merge commit's files are those it brought against its first parent; a commit with one
// parent (a squash or rebase merge, a direct commit) is refused, since its delivery's start is unknown.
func TestMergeFilesReadsAMergeCommitAndRefusesASingleParentOne(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("README.md")
	run("add", "-A")
	run("commit", "-qm", "root")
	run("checkout", "-q", "-b", "pr")
	for _, f := range []string{"src/a.go", "src/b.go"} { // two commits, as a rebase merge would land them
		write(f)
		run("add", "-A")
		run("commit", "-qm", f)
	}
	last := run("rev-parse", "HEAD")
	run("checkout", "-q", "main")
	write("other.md")
	run("add", "-A")
	run("commit", "-qm", "main moved on")
	run("merge", "-q", "--no-ff", "-m", "merge pr", "pr")
	got, err := MergeFiles(dir, run("rev-parse", "HEAD"))()
	if err != nil || !reflect.DeepEqual(got, []string{"src/a.go", "src/b.go"}) {
		t.Errorf("MergeFiles(merge) = %v, %v; want both of the branch's files", got, err)
	}
	_, err = MergeFiles(dir, last)()
	want := "--merged " + last[:7] + " is not a merge commit, so the design-page check cannot tell which commits the " +
		"delivery holds (a squash or a rebase merge leaves one commit per change); give the PR with --pr URL, whose " +
		"files GitHub lists"
	if err == nil || err.Error() != want {
		t.Errorf("MergeFiles(one parent): %v, want %q", err, want)
	}
}

func TestOwnIsThePatternsTheSprintAddedOrChangedSinceItOpened(t *testing.T) {
	dir, committer := t.TempDir(), ""
	commit := func(day, covers string) { // authored on day; committed on the day in committer, when set
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "design"), 0o755); err != nil {
			t.Fatal(err)
		}
		text := "---\ntype: design\nproject: demo\ncovers: " + covers + "\n---\n\nx\n" // no title: Parse refuses it
		if err := os.WriteFile(filepath.Join(dir, "design/p.md"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", day}} {
			cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+day+"T12:00:00Z", "GIT_COMMITTER_DATE="+committer+"T12:00:00Z")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	opened := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	committer = "2026-10-01" // authored before the sprint opened, rebased onto the remote's records after it
	commit("2026-09-20", "[a/**, pending/x.go, old.go]")
	committer = "2026-10-02"
	commit("2026-10-02", "[a/**, pending/x.go, new.go, a/y.go]") // in the sprint: old.go edited to new.go, a/y.go added
	now := page(t, "design/p", "covers: [a/**, pending/x.go, new.go, a/y.go]\n")
	got, err := Own(dir, now, opened)
	if err != nil || !reflect.DeepEqual(got, []string{"new.go", "a/y.go"}) {
		t.Errorf("Own = %v, %v; want the edited and the added pattern, not the ones the page held before (read from "+
			"a header Parse refuses, in a commit whose author date predates the sprint)", got, err)
	}
	if got, err := Own(dir, now, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)); err != nil || len(got) != 4 {
		t.Errorf("Own of a sprint opened before any commit = %v, %v; want every pattern", got, err)
	}
	created := page(t, "design/q", "covers: [a/**]\n") // a page made during the sprint: no design/q.md before it
	if got, err := Own(dir, created, opened); err != nil || !reflect.DeepEqual(got, []string{"a/**"}) {
		t.Errorf("Own of a page created in the sprint = %v, %v; want every pattern", got, err)
	}
}
