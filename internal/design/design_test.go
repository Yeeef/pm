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

func TestCheckFailsOnADeadPatternAndListsUncoveredSources(t *testing.T) {
	tracked := map[string]bool{"internal/a/a.go": true, "internal/b/b.go": true, "prime.md": true, "README.md": true}
	recs := []*records.Record{page(t, "design/a", "covers: [internal/a/**]\n"), page(t, "design/none", "")}
	got, err := Check(recs, tracked, []string{"internal/**", "prime.md"})
	if err != nil || !reflect.DeepEqual(got, []string{"internal/b/b.go", "prime.md"}) {
		t.Errorf("Check = %v, %v; want the two uncovered sources", got, err)
	}
	if got, err := Check(recs, tracked, nil); err != nil || got != nil {
		t.Errorf("Check without sources = %v, %v; want nothing", got, err)
	}
	recs = append(recs, page(t, "design/dead", "covers: [internal/c/**, prime.md]\n"))
	_, err = Check(recs, tracked, []string{"internal/**"})
	want := `design/dead: covers pattern "internal/c/**" matches no tracked file; fix or remove it (a directory's files ` +
		`are dir/**)`
	if err == nil || err.Error() != want {
		t.Errorf("Check with a dead pattern: %v, want %q", err, want)
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
