package design

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func TestCheckListsUncoveredSourcesAndNotDeadPatterns(t *testing.T) {
	tracked := map[string]bool{"internal/a/a.go": true, "internal/b/b.go": true, "prime.md": true, "README.md": true}
	recs := []*records.Record{page(t, "design/a", "covers: [internal/a/**]\n"), page(t, "design/none", "")}
	got, err := Check(recs, tracked, []string{"internal/**", "prime.md"})
	if err != nil || !reflect.DeepEqual(got, []string{"internal/b/b.go", "prime.md"}) {
		t.Errorf("Check = %v, %v; want the two uncovered sources", got, err)
	}
	if got, err := Check(recs, tracked, nil); err != nil || got != nil {
		t.Errorf("Check without sources = %v, %v; want nothing", got, err)
	}
	recs = append(recs, page(t, "design/dead", "covers: [internal/c/**, prime.md, internal/d.go]\n"))
	got, err = Check(recs, tracked, []string{"internal/**"})
	if err != nil || !reflect.DeepEqual(got, []string{"internal/b/b.go"}) {
		t.Errorf("Check with dead patterns = %v, %v; want the uncovered source alone (Hold holds the patterns)", got, err)
	}
	_, err = Check(recs, tracked, []string{"internal/**", "lib/**"})
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
	if got := Missing(map[string][]string{"design/a": {"a/**", "a/new.go"}}, files); !reflect.DeepEqual(got,
		[]string{`design/a.md: covers pattern "a/new.go"`}) {
		t.Errorf("Missing = %v; want the unmerged file's pattern", got)
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

func TestRenameWaiversMovesOnlyTheRenamedPagesWaivers(t *testing.T) {
	text := "::: decision {source=agent date=2026-10-12}\n" + WaiverText("parser", "Why.") + "\n:::\n" +
		"::: decision {source=agent date=2026-10-12}\n" + WaiverText("parser-old", "Other.") + "\n:::\n" +
		"Prose: The design page parser stays as it is for this sprint's code changes.\n"
	want := "::: decision {source=agent date=2026-10-12}\n" + WaiverText("lexer", "Why.") + "\n:::\n" +
		"::: decision {source=agent date=2026-10-12}\n" + WaiverText("parser-old", "Other.") + "\n:::\n" +
		"Prose: The design page parser stays as it is for this sprint's code changes.\n"
	if got := RenameWaivers(text, "parser", "lexer"); got != want {
		t.Errorf("RenameWaivers =\n%s\nwant\n%s", got, want)
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

// MissingOnMain reads the remote-tracking ref, and fetches it when a pattern matches nothing there: a page published
// once its code merged names a file that a stale ref lacks.
func TestMissingOnMainFetchesAStaleRefBeforeItFails(t *testing.T) {
	tmp := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	origin, root, other := filepath.Join(tmp, "origin.git"), filepath.Join(tmp, "root"), filepath.Join(tmp, "other")
	git(tmp, "init", "-q", "--bare", "-b", "main", origin)
	add := func(dir, f string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(dir, "add", "-A")
		git(dir, "commit", "-qm", f)
		git(dir, "push", "-q", "origin", "HEAD:main")
	}
	git(tmp, "clone", "-q", origin, root)
	add(root, "a/x.go")
	git(root, "fetch", "-q", "origin")
	git(tmp, "clone", "-q", origin, other)
	add(other, "a/new.go") // merged elsewhere: root's origin/main lacks it
	patterns := map[string][]string{"design/a": {"a/**", "a/new.go"}, "design/b": {"b/**"}}
	got, err := MissingOnMain(root, "origin", "main", patterns)
	if err != nil || !reflect.DeepEqual(got, []string{`design/b.md: covers pattern "b/**"`}) {
		t.Errorf("MissingOnMain = %v, %v; want only the pattern main lacks after the fetch", got, err)
	}
	if got, err := MissingOnMain(root, "nowhere", "main", map[string][]string{"design/a": nil}); err != nil || got != nil {
		t.Errorf("MissingOnMain with no pattern = %v, %v; want nothing read", got, err)
	}
	if _, err := MissingOnMain(root, "nowhere", "main", patterns); err == nil ||
		!strings.Contains(err.Error(), "pm cannot see nowhere/main") {
		t.Errorf("MissingOnMain with no such remote: %v; want it to fail naming the ref", err)
	}
}
