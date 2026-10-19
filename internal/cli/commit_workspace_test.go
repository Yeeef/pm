package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/served"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// The expected outcomes follow from the record-drafts page (Writes and journal publish, Follow, Publish at sprint
// close's sprint rule), not from this code.

// goal is a sprint Goal of paragraphs, each a hunk of its own, so edits to two of them merge.
const goal = "a\n\nb\n\nc\n\nd\n\ne"

// workspaces is a clone holding the demo project, its sprints 1 and 2 (1 linking design/parser), the design page, and
// the code worktrees one and two with their records workspaces; this test's session holds a task of sprint 1.
func workspaces(t *testing.T) (root, storeDir string, one, two store.Workspace) {
	t.Helper()
	root = clone(t)
	storeDir = filepath.Join(root, ".pm/store/records")
	sprint := recordsSprint("One", "demo.1", "[Parser](../design/parser.md)")
	for rel, text := range map[string]string{"sprints/demo-1.md": sprint,
		"sprints/demo-2.md": recordsSprint("Two", "demo.2", "None yet."),
		"design/parser.md": strings.Replace(records.DesignText("Parser", "demo"), "## Problem\n",
			"## Problem\n\nP1\n\nP2\n", 1)} {
		writeFile(t, filepath.Join(storeDir, rel), text)
	}
	run(t, storeDir, "git add -A", "git commit -qm sprints")
	edit(t, filepath.Join(root, ".pm/config.toml"), `version = "9.9.9"`, `version = "`+buildinfo.Version+`"`) // no launch
	run(t, root, "git commit -qam pin")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	items := []work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open, CreatedAt: at, UpdatedAt: at},
		{ID: "demo.1", Type: work.Sprint, Parent: "demo", Title: "One", Number: 1, Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		{ID: "demo.2", Type: work.Sprint, Parent: "demo", Title: "Two", Number: 2, Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		{ID: "demo.1.1", Type: work.Task, Parent: "demo.1", Title: "T", Status: work.Open, CreatedAt: at,
			UpdatedAt: at, Holder: &work.Holder{Session: "s1", ClaimedAt: at}}}
	saved, savedInner := OpenWork, innermost
	t.Cleanup(func() { OpenWork, innermost = saved, savedInner })
	OpenWork = func(string) (work.Store, error) { return worktest.New(items), nil }
	innermost = func() string { return "" }
	t.Setenv("CLAUDE_CODE_SESSION_ID", "s1")
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CODEX_SESSION_ID", "")
	ws := make([]store.Workspace, 2)
	for i, name := range []string{"one", "two"} {
		code := filepath.Join(filepath.Dir(root), name)
		run(t, root, "git worktree add -q -b "+name+" "+code)
		if _, err := store.AddWorkspace(storeDir, code); err != nil {
			t.Fatal(err)
		}
		w, err := store.WorkspaceAt(code)
		if err != nil || w == nil {
			t.Fatalf("no workspace in %s: %v", code, err)
		}
		ws[i] = *w
	}
	return root, storeDir, ws[0], ws[1]
}

func recordsSprint(title, bead, design string) string {
	text := records.SprintText(title, bead, map[string]string{"Goal": goal, "Scope": "**In:** x. **Out:** y.",
		"Done when": "z."})
	return strings.Replace(text, "> Where is the detail?\n\nNone yet.", "> Where is the detail?\n\n"+design, 1)
}

// runPM runs pm argv in dir: its stdout, stderr and exit code.
func runPM(t *testing.T, dir string, argv ...string) (string, string, int) {
	t.Helper()
	t.Chdir(dir)
	var out, errb strings.Builder
	code := Execute(argv, strings.NewReader(""), &out, &errb)
	return out.String(), errb.String(), code
}

func edit(t *testing.T, path, old, now string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s holds no %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, now, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := store.Git(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Two worktrees edit one sprint record: disjoint edits both reach records; an overlap is refused at the second pm
// commit, naming the first commit, with markers only in the second workspace.
func TestPmCommitPublishesDisjointEditsAndRefusesAnOverlapNamingTheFirstCommit(t *testing.T) {
	_, records, one, two := workspaces(t)
	rel := "records/sprints/demo-1.md"
	edit(t, filepath.Join(one.Dir, "sprints/demo-1.md"), "\na\n", "\nA one\n")
	edit(t, filepath.Join(two.Dir, "sprints/demo-1.md"), "\ne\n", "\nE two\n")
	if out, errs, code := runPM(t, one.Code, "commit", "-m", "one's goal", rel); code != 0 {
		t.Fatalf("one's commit: exit %d\n%s%s", code, out, errs)
	}
	if out, errs, code := runPM(t, two.Code, "commit", "-m", "two's goal", rel); code != 0 {
		t.Fatalf("two's disjoint commit: exit %d\n%s%s", code, out, errs)
	}
	published := gitOut(t, records, "show", "records:sprints/demo-1.md")
	if !strings.Contains(published, "\nA one\n") || !strings.Contains(published, "\nE two\n") {
		t.Fatalf("records lacks an edit:\n%s", published)
	}
	if got := gitOut(t, records, "log", "-2", "--format=%s"); got != "two's goal\none's goal" {
		t.Errorf("records log: %q", got)
	}

	edit(t, filepath.Join(one.Dir, "sprints/demo-1.md"), "\nc\n", "\nC one\n")
	edit(t, filepath.Join(two.Dir, "sprints/demo-1.md"), "\nc\n", "\nC two\n")
	if _, errs, code := runPM(t, one.Code, "commit", "-m", "one's c", rel); code != 0 {
		t.Fatalf("one's commit: %s", errs)
	}
	first := gitOut(t, records, "rev-parse", "--short", "HEAD")
	_, errs, code := runPM(t, two.Code, "commit", "-m", "two's c", rel)
	if code != 1 || !strings.Contains(errs, "error: "+rel+" has conflict markers, which records changed under your edit in "+
		first+" one's c") {
		t.Fatalf("two's overlapping commit: exit %d %q; want refused naming %s", code, errs, first)
	}
	if got := gitOut(t, records, "rev-parse", "--short", "HEAD"); got != first {
		t.Errorf("the refused commit moved records to %s", got)
	}
	if !store.HasMarkers(fileText(t, filepath.Join(two.Dir, "sprints/demo-1.md"))) {
		t.Error("two's workspace holds no markers")
	}
	for dir, what := range map[string]string{one.Dir: "one's workspace", records: "the store"} {
		if store.HasMarkers(fileText(t, filepath.Join(dir, "sprints/demo-1.md"))) {
			t.Errorf("%s holds markers", what)
		}
	}
	// resolved, it commits
	conflict := regexp.MustCompile(`(?s)<<<<<<< workspace\nC two\n=======\nC one\n>>>>>>> records [^\n]*\n`)
	marked := fileText(t, filepath.Join(two.Dir, "sprints/demo-1.md"))
	if !conflict.MatchString(marked) {
		t.Fatalf("two's markers:\n%s", marked)
	}
	writeFile(t, filepath.Join(two.Dir, "sprints/demo-1.md"), conflict.ReplaceAllString(marked, "C one, C two\n"))
	if _, errs, code := runPM(t, two.Code, "commit", "-m", "both c", rel); code != 0 {
		t.Fatalf("the resolved commit: %s", errs)
	}
	if got := gitOut(t, records, "show", "records:sprints/demo-1.md"); !strings.Contains(got, "\nC one, C two\n") {
		t.Errorf("records after the resolution:\n%s", got)
	}
}

// A one-shot write from one worktree merges into the other workspace's uncommitted edit of the same file at that
// workspace's next pm write, and leaves neither workspace mid-operation.
func TestAOneShotWriteMergesIntoAnotherWorkspacesUncommittedEdit(t *testing.T) {
	_, records, one, two := workspaces(t)
	edit(t, filepath.Join(two.Dir, "sprints/demo-1.md"), "\nb\n", "\nB two\n")
	if out, errs, code := runPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "Followed in 3 ms."); code != 0 {
		t.Fatalf("finding add: exit %d %s%s", code, out, errs)
	}
	for _, w := range []store.Workspace{one, two} {
		if got := fileText(t, filepath.Join(w.Dir, "sprints/demo-1.md")); strings.Contains(got, "Followed in 3 ms.") != (w == one) {
			t.Errorf("%s after the finding: %q", w.Name, got)
		}
	}
	// two's next write follows: the finding merges into its uncommitted Goal edit
	if out, errs, code := runPM(t, two.Code, "finding", "add", "--sprint", "demo.1", "Merged."); code != 0 {
		t.Fatalf("two's finding: exit %d %s%s", code, out, errs)
	}
	got := fileText(t, filepath.Join(two.Dir, "sprints/demo-1.md"))
	if !strings.Contains(got, "\nB two\n") || !strings.Contains(got, "Followed in 3 ms.") || !strings.Contains(got, "Merged.") ||
		store.HasMarkers(got) {
		t.Fatalf("two's file after following both findings:\n%s", got)
	}
	tip := gitOut(t, records, "rev-parse", "records")
	for _, w := range []store.Workspace{one, two} {
		gitDir := gitOut(t, w.Dir, "rev-parse", "--absolute-git-dir")
		for _, f := range []string{"MERGE_HEAD", "REBASE_HEAD", "rebase-merge", "rebase-apply"} {
			if _, err := os.Stat(filepath.Join(gitDir, f)); err == nil {
				t.Errorf("%s holds %s", w.Dir, f)
			}
		}
	}
	if head := gitOut(t, records, "rev-parse", two.Ref()); head != tip {
		t.Errorf("two is at %s, not the records tip %s", head, tip)
	}
	if dirty := dirtyOf(t, two); len(dirty) != 1 || dirty[0] != "sprints/demo-1.md" {
		t.Errorf("two's uncommitted files: %v; want its Goal edit alone", dirty)
	}
}

// A design page commits into the draft commit, tagged with the sprint that links it and in which the session holds a
// task, and is not published; with no such sprint it is refused, and the main checkout refuses a design page.
func TestADesignPageIsTaggedWithItsSprintAndRefusedInTheMainCheckout(t *testing.T) {
	root, records, one, _ := workspaces(t)
	rel := "records/design/parser.md"
	tip := gitOut(t, records, "rev-parse", "records")
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1, drafted\n")
	if out, errs, code := runPM(t, one.Code, "commit", "-m", "parser", rel); code != 0 {
		t.Fatalf("commit: exit %d %s%s", code, out, errs)
	}
	if got := gitOut(t, records, "rev-parse", "records"); got != tip {
		t.Errorf("the draft moved records to %s", got)
	}
	if got := gitOut(t, records, "log", "-1", "--format=%B", one.Ref()); got != "pm draft: design pages of drafts/one"+
		"\n\nSprint: design/parser.md demo.1" {
		t.Errorf("the draft commit's message: %q", got)
	}
	if got := gitOut(t, records, "show", "records:design/parser.md"); strings.Contains(got, "P1, drafted") {
		t.Error("the page is published")
	}
	if dirty := dirtyOf(t, one); dirty != nil {
		t.Errorf("the draft left uncommitted files: %v", dirty)
	}

	t.Setenv("CLAUDE_CODE_SESSION_ID", "s2") // holds no task
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP2\n", "\nP2, drafted\n")
	_, errs, code := runPM(t, one.Code, "commit", "-m", "parser", rel)
	if code != 1 || !strings.Contains(errs, "no open sprint in which this session holds a task links it") {
		t.Errorf("no sprint: exit %d %q", code, errs)
	}
	if _, errs, code := runPM(t, one.Code, "commit", "-m", "parser", "--sprint", "demo.1", rel); code != 0 {
		t.Errorf("--sprint demo.1: %s", errs)
	}

	if err := os.Symlink(records, filepath.Join(root, "records")); err != nil {
		t.Fatal(err)
	}
	edit(t, filepath.Join(records, "design/parser.md"), "\nP1\n", "\nP1, in the store\n")
	_, errs, code = runPM(t, root, "commit", "-m", "parser", rel)
	if code != 1 || !strings.Contains(errs, "error: "+rel+" is a design page: design pages are edited and committed in "+
		"a code worktree's records workspace") {
		t.Errorf("the main checkout: exit %d %q", code, errs)
	}
}

// A commit on records that would overwrite the store's uncommitted edit of the file (a main-checkout session's) is
// refused, naming the file.
func TestACommitThatWouldOverwriteTheStoresUncommittedEditIsRefused(t *testing.T) {
	_, records, one, _ := workspaces(t)
	edit(t, filepath.Join(records, "sprints/demo-1.md"), "\nd\n", "\nD main\n")
	edit(t, filepath.Join(one.Dir, "sprints/demo-1.md"), "\na\n", "\nA one\n")
	_, errs, code := runPM(t, one.Code, "commit", "-m", "goal", "records/sprints/demo-1.md")
	if code != 1 || !strings.Contains(errs, "error: records/sprints/demo-1.md has uncommitted edits in the shared store") {
		t.Errorf("exit %d %q", code, errs)
	}
	if got := fileText(t, filepath.Join(records, "sprints/demo-1.md")); !strings.Contains(got, "D main") || strings.Contains(got, "A one") {
		t.Errorf("the store's edit was touched: %q", got)
	}
}

// The site lists each workspace that changes a page as a version of it, labelled with its worktree, its branch and
// its state; the stamp moves with a workspace's edit.
func TestTheSiteListsEachWorkspaceAsAVersionLabelledWithItsWorktree(t *testing.T) {
	_, storeDir, one, two := workspaces(t)
	ws, err := OpenWork("")
	if err != nil {
		t.Fatal(err)
	}
	items, err := ws.Items()
	if err != nil {
		t.Fatal(err)
	}
	// session s1, which holds a task of sprint demo.1, works in one: its transcript's entry names one's directory
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	entry := fmt.Sprintf(`{"type":"user","timestamp":%q,"cwd":%q,"sessionId":"s1","message":{"content":"go"}}`+"\n",
		time.Now().UTC().Format(time.RFC3339Nano), one.Code)
	writeFile(t, filepath.Join(claude, "projects/p/s1.jsonl"), entry)
	s, err := served.New(storeDir, "t", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Stamp()
	if err != nil {
		t.Fatal(err)
	}
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	edit(t, filepath.Join(two.Dir, "design/parser.md"), "\nP2\n", "\n<<<<<<< workspace\nP2 by two\n=======\nP2\n>>>>>>> records a..b\n")
	if after, err := s.Stamp(); err != nil || after == before {
		t.Fatalf("the stamp did not move with the workspaces' edits: %v", err)
	}
	pages, err := s.Load(items)
	if err != nil {
		t.Fatal(err)
	}
	page := func(path string) string {
		t.Helper()
		text, found, err := pages.Page(path)
		if err != nil || !found {
			t.Fatalf("%s: found %v, %v", path, found, err)
		}
		return text
	}
	published := page("design/parser.html")
	for _, want := range []string{">one</a>", "worktree one on one, sprint demo.1, session s1 live; uncommitted edits",
		">two</a>", "worktree two on two, no live session; uncommitted edits, conflict", `href="../drafts/one/design/parser.diff.html"`} {
		if !strings.Contains(published, want) {
			t.Errorf("the published page's version bar lacks %q", want)
		}
	}
	if strings.Contains(published, "P1 by one") {
		t.Error("a workspace's edit is published")
	}
	if !strings.Contains(page("drafts/one/design/parser.html"), "P1 by one") {
		t.Error("one's version lacks its edit")
	}
	if !strings.Contains(page("drafts/two/design/parser.diff.html"), "P2 by two") {
		t.Error("two's diff lacks its edit")
	}
}

func dirtyOf(t *testing.T, w store.Workspace) []string {
	t.Helper()
	dirty, err := w.Dirty()
	if err != nil {
		t.Fatal(err)
	}
	return dirty
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A figure the records branch changed under the workspace's uncommitted copy cannot merge: pm commit refuses it once,
// naming the commit, and a second pm commit replaces the published figure.
func TestPmCommitRefusesAFigureTheRecordsBranchChangedUnderIt(t *testing.T) {
	_, storeDir, one, _ := workspaces(t)
	writeFile(t, filepath.Join(storeDir, "docs/fig.png"), "\x89PNG\x00base")
	run(t, storeDir, "git add docs/fig.png", "git commit -qm figure")
	if _, errs, code := runPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "Follow."); code != 0 {
		t.Fatal(errs)
	}
	writeFile(t, filepath.Join(one.Dir, "docs/fig.png"), "\x89PNG\x00mine")
	writeFile(t, filepath.Join(storeDir, "docs/fig.png"), "\x89PNG\x00published")
	run(t, storeDir, "git commit -qam 'a newer figure'")
	newer := gitOut(t, storeDir, "rev-parse", "--short", "HEAD")
	_, errs, code := runPM(t, one.Code, "commit", "-m", "my figure", "records/docs/fig.png")
	if code != 1 || !strings.Contains(errs, "records/docs/fig.png is a binary file the records branch changed under your "+
		"uncommitted copy, in "+newer+" a newer figure") {
		t.Fatalf("exit %d %q", code, errs)
	}
	if got := gitOut(t, storeDir, "show", "records:docs/fig.png"); got != "\x89PNG\x00published" {
		t.Errorf("the published figure changed: %q", got)
	}
	if _, errs, code := runPM(t, one.Code, "commit", "-m", "my figure", "records/docs/fig.png"); code != 0 {
		t.Fatalf("the second commit: %s", errs)
	}
	if got := gitOut(t, storeDir, "show", "records:docs/fig.png"); got != "\x89PNG\x00mine" {
		t.Errorf("the figure was not replaced: %q", got)
	}
}
