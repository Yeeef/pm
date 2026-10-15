package cli

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// The expected outcomes follow from the record-drafts page (Publish at sprint close, Covers, Failure modes), not from
// this code: a design page's draft publishes only at its sprint's close, from every worktree, in one records commit.

// closingStore is the work store with each close recorded.
type closingStore struct {
	*worktest.Store
	closed *[]string
}

func (s closingStore) Close(id, reason string, _ work.Resolution, _ string) error {
	*s.closed = append(*s.closed, id+": "+reason)
	return nil
}

// closeReady writes and commits sprint demo.1's delivery report, and closes its task: what pm sprint close needs.
func closeReady(t *testing.T, records string) *[]string {
	t.Helper()
	path := filepath.Join(records, "sprints/demo-1.md")
	edit(t, path, "\n\nNot closed yet.\n\n### Against", "\n\nDone: the drafts published.\n\n### Against")
	edit(t, path, "\n\nNot closed yet.\n", "\n\n- It publishes: met.\n")
	run(t, records, "git commit -qam 'delivery report'")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	items := []work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open, CreatedAt: at, UpdatedAt: at},
		{ID: "demo.1", Type: work.Sprint, Parent: "demo", Title: "One", Number: 1, Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		{ID: "demo.2", Type: work.Sprint, Parent: "demo", Title: "Two", Number: 2, Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		{ID: "demo.1.1", Type: work.Task, Parent: "demo.1", Title: "T", Status: work.Closed, CreatedAt: at,
			UpdatedAt: at}}
	closed := &[]string{}
	OpenWork = func(string) (work.Store, error) { return closingStore{worktest.New(items), closed}, nil }
	return closed
}

func mustPM(t *testing.T, dir string, argv ...string) string {
	t.Helper()
	out, errs, code := runPM(t, dir, argv...)
	if code != 0 {
		t.Fatalf("pm %s in %s: exit %d\n%s%s", strings.Join(argv, " "), dir, code, out, errs)
	}
	return out
}

func draftPages(t *testing.T, records string, w store.Workspace) map[string]string {
	t.Helper()
	pages, tags, err := store.DraftPages(records, w)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for p := range pages {
		out[p] = tags[p]
	}
	return out
}

// Two worktrees draft one page for one sprint: the published page is unchanged until the close, which publishes both
// drafts in one records commit and leaves no draft behind; run again, it publishes nothing.
func TestTheSprintClosePublishesTheDraftsOfTwoWorktreesInOneCommit(t *testing.T) {
	_, records, one, two := workspaces(t)
	rel := "records/design/parser.md"
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	edit(t, filepath.Join(two.Dir, "design/parser.md"), "\nP2\n", "\nP2 by two\n")
	published := gitOut(t, records, "show", "records:design/parser.md")
	before := gitOut(t, records, "rev-parse", "records")
	if out := mustPM(t, one.Code, "commit", "-m", "one's parser", rel); !strings.Contains(out,
		"drafted records/design/parser.md for sprint demo.1 on drafts/one: pm sprint close demo.1 publishes it") {
		t.Errorf("one's commit said %q", out)
	}
	mustPM(t, two.Code, "commit", "-m", "two's parser", rel)
	if got := gitOut(t, records, "rev-parse", "records"); got != before {
		t.Fatalf("pm commit of a design page moved records to %s", got)
	}
	for _, w := range []store.Workspace{one, two} {
		if got := draftPages(t, records, w); got["design/parser.md"] != "demo.1" || len(got) != 1 {
			t.Errorf("%s's draft: %v; want the parser page for demo.1", w.Name, got)
		}
	}

	closed := closeReady(t, records)
	if got := gitOut(t, records, "show", "records:design/parser.md"); got != published {
		t.Fatal("the page changed before the close")
	}
	before = gitOut(t, records, "rev-parse", "records")
	out := mustPM(t, one.Code, "sprint", "close", "demo.1")
	if !strings.Contains(out, "published design/parser.md") || len(*closed) != 1 {
		t.Errorf("the close said %q, closed %v", out, *closed)
	}
	if n := gitOut(t, records, "rev-list", "--count", before+"..records"); n != "1" {
		t.Errorf("the close made %s records commits; want one", n)
	}
	page := gitOut(t, records, "show", "records:design/parser.md")
	if !strings.Contains(page, "\nP1 by one\n") || !strings.Contains(page, "\nP2 by two\n") {
		t.Errorf("the published page lacks a draft:\n%s", page)
	}
	if msg := gitOut(t, records, "log", "-1", "--format=%B", "records"); !strings.Contains(msg,
		"Published: design/parser.md demo.1") {
		t.Errorf("the close's commit: %q", msg)
	}
	for _, w := range []store.Workspace{one, two} {
		if got := draftPages(t, records, w); len(got) != 0 {
			t.Errorf("%s still drafts %v", w.Name, got)
		}
	}
	// one followed at the close; two's next pm write drops the published page from its draft commit
	if head, tip := gitOut(t, records, "rev-parse", one.Ref()), gitOut(t, records, "rev-parse", "records"); head != tip {
		t.Errorf("one is at %s, not the records tip %s", head, tip)
	}
	mustPM(t, two.Code, "finding", "add", "--sprint", "demo.2", "Two follows.")
	if head, tip := gitOut(t, records, "rev-parse", two.Ref()), gitOut(t, records, "rev-parse", "records"); head != tip {
		t.Errorf("two is at %s, not the records tip %s", head, tip)
	}
	for _, w := range []store.Workspace{one, two} {
		if dirty := dirtyOf(t, w); dirty != nil {
			t.Errorf("%s has uncommitted %v", w.Name, dirty)
		}
	}

	before = gitOut(t, records, "rev-parse", "records")
	mustPM(t, one.Code, "sprint", "close", "demo.1") // the fake work store keeps the sprint open: a rerun
	if got := gitOut(t, records, "rev-parse", "records"); got != before {
		t.Errorf("the rerun close committed %s", gitOut(t, records, "log", "-1", "--format=%s", got))
	}
}

// A page another sprint published first overlaps the draft: the close is refused naming that commit and changes
// nothing; after the agent brings it in, resolves the markers and commits, the close passes.
func TestAnOverlapRefusesTheCloseNamingTheCommitUntilResolved(t *testing.T) {
	_, records, one, _ := workspaces(t)
	rel := "records/design/parser.md"
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	mustPM(t, one.Code, "commit", "-m", "one's parser", rel)
	edit(t, filepath.Join(records, "design/parser.md"), "\nP1\n", "\nP1 by sprint two\n")
	run(t, records, "git commit -qam 'sprint two published the parser'")
	other := gitOut(t, records, "rev-parse", "--short", "HEAD")
	closed := closeReady(t, records)
	before := gitOut(t, records, "rev-parse", "records")
	_, errs, code := runPM(t, one.Code, "sprint", "close", "demo.1")
	if code != 1 || !strings.Contains(errs, "records/design/parser.md on drafts/one overlaps what the records branch "+
		"published since its base, in "+other+" sprint two published the parser") {
		t.Fatalf("the close: exit %d %q; want refused naming %s", code, errs, other)
	}
	if got := gitOut(t, records, "rev-parse", "records"); got != before || len(*closed) != 0 {
		t.Fatalf("the refused close changed records to %s or closed %v", got, *closed)
	}
	if got := draftPages(t, records, one); got["design/parser.md"] != "demo.1" {
		t.Fatalf("the refused close dropped the draft: %v", got)
	}

	// any pm write brings the records branch in: the overlap becomes markers, resolved and committed
	runPM(t, one.Code, "commit", "-m", "follow")
	path := filepath.Join(one.Dir, "design/parser.md")
	conflict := regexp.MustCompile(`(?s)<<<<<<< workspace\nP1 by one\n=======\nP1 by sprint two\n>>>>>>> records [^\n]*\n`)
	if !conflict.MatchString(fileText(t, path)) {
		t.Fatalf("one's page after the follow:\n%s", fileText(t, path))
	}
	writeFile(t, path, conflict.ReplaceAllString(fileText(t, path), "P1 by both\n"))
	mustPM(t, one.Code, "commit", "-m", "both", "--sprint", "demo.1", rel) // its task is closed: --sprint names it
	mustPM(t, one.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/parser.md"); !strings.Contains(got, "\nP1 by both\n") {
		t.Errorf("the published page after the resolution:\n%s", got)
	}
}

// --discard drops the sprint's drafts of a page, puts back the closing workspace's edits to it, and records the
// reason as pm design unchanged does; it is refused for a page with no draft, or without its --reason.
func TestDiscardDropsTheDraftAndRecordsTheDecision(t *testing.T) {
	_, records, one, two := workspaces(t)
	rel := "records/design/parser.md"
	edit(t, filepath.Join(two.Dir, "design/parser.md"), "\nP2\n", "\nP2 by two\n")
	mustPM(t, two.Code, "commit", "-m", "two's parser", rel)
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one, not committed\n")
	published := gitOut(t, records, "show", "records:design/parser.md")
	closeReady(t, records)
	_, errs, code := runPM(t, one.Code, "sprint", "close", "demo.1")
	if code != 1 || !strings.Contains(errs, "records/design/parser.md has uncommitted edits in this worktree's records "+
		"workspace") {
		t.Fatalf("the close with an uncommitted page: exit %d %q", code, errs)
	}
	if _, errs, code := runPM(t, one.Code, "sprint", "close", "demo.1", "--discard", "parser"); code != 1 ||
		!strings.Contains(errs, "give one --reason with each --discard") {
		t.Errorf("--discard without --reason: exit %d %q", code, errs)
	}
	if _, errs, code := runPM(t, one.Code, "sprint", "close", "demo.1", "--discard", "lexer", "--reason", "R.",
		"--discard", "parser", "--reason", "R."); code != 1 || !strings.Contains(errs, "--discard lexer: sprint demo.1 "+
		"has no draft of the design page lexer") {
		t.Errorf("--discard of a page with no draft: exit %d %q", code, errs)
	}
	out := mustPM(t, one.Code, "sprint", "close", "demo.1", "--discard", "parser", "--reason",
		"The parser's design holds as it is.")
	if !strings.Contains(out, "discarded the drafts of parser") || strings.Contains(out, "; published") {
		t.Errorf("the close said %q", out)
	}
	if got := gitOut(t, records, "show", "records:design/parser.md"); got != published {
		t.Errorf("a discarded draft was published:\n%s", got)
	}
	sprint := gitOut(t, records, "show", "records:sprints/demo-1.md")
	if !strings.Contains(sprint, "The design page parser stays as it is for this sprint's code changes.\n"+
		"The parser's design holds as it is.\nIts drafts are discarded at the sprint's close, not published.\n:::") {
		t.Errorf("the sprint record lacks the decision:\n%s", sprint)
	}
	if got := fileText(t, filepath.Join(one.Dir, "design/parser.md")); got != published+"\n" && got != published {
		t.Errorf("one's uncommitted edit was not put back:\n%s", got)
	}
	for _, w := range []store.Workspace{one, two} {
		if got := draftPages(t, records, w); len(got) != 0 {
			t.Errorf("%s still drafts %v", w.Name, got)
		}
	}
}

// pm design rename on records of a page a workspace drafts moves the draft's sprint to the new path at the next
// follow, which does not wedge; the close publishes the draft at the new path.
func TestARenameOfADraftedPageCarriesItsSprint(t *testing.T) {
	root, records, one, _ := workspaces(t)
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	mustPM(t, one.Code, "commit", "-m", "one's parser", "records/design/parser.md")
	mustPM(t, root, "design", "rename", "parser", "--to", "grammar")
	mustPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "Renamed.")
	if got := draftPages(t, records, one); len(got) != 1 || got["design/grammar.md"] != "demo.1" {
		t.Fatalf("one's draft after the rename: %v; want the page at its new path, for demo.1", got)
	}
	if got := fileText(t, filepath.Join(one.Dir, "design/grammar.md")); !strings.Contains(got, "\nP1 by one\n") {
		t.Errorf("the renamed page lacks the draft's edit:\n%s", got)
	}
	closeReady(t, records)
	mustPM(t, one.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/grammar.md"); !strings.Contains(got, "\nP1 by one\n") {
		t.Errorf("the close did not publish the renamed draft:\n%s", got)
	}
}

// A worktree removed with its workspace leaves its drafts/ branch: pm where lists it, the close publishes from it, and
// the branch, left empty, is deleted.
func TestPmWhereListsAnOrphanedDraftsBranchAndTheClosePublishesIt(t *testing.T) {
	root, records, one, two := workspaces(t)
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	mustPM(t, one.Code, "commit", "-m", "one's parser", "records/design/parser.md")
	run(t, root, "git worktree remove --force --force "+one.Code)
	out := mustPM(t, root, "where")
	if !strings.Contains(out, "drafts    drafts/one  its worktree is gone; a draft commit of 1 design pages for "+
		"sprint demo.1, published at the sprint's close") || strings.Contains(out, "drafts/two") {
		t.Fatalf("pm where:\n%s", out)
	}
	closeReady(t, records)
	mustPM(t, two.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/parser.md"); !strings.Contains(got, "\nP1 by one\n") {
		t.Errorf("the orphan's draft was not published:\n%s", got)
	}
	if got := gitOut(t, records, "branch", "--list", "drafts/*"); strings.Contains(got, "drafts/one") {
		t.Errorf("the emptied orphan branch is kept: %q", got)
	}
}

// pm check holds a published page's covers to the remote's main, fetched when behind; in a workspace, a draft page's
// covers to the worktree's tracked files. The close holds a pattern a page to publish adds to main.
func TestCoversAreHeldToMainOnceAndToTheWorktreeWhileDrafted(t *testing.T) {
	root, records, one, _ := workspaces(t)
	origin := filepath.Join(filepath.Dir(root), "origin.git")
	run(t, root, "git init -q --bare "+origin, "git remote add origin "+origin, "git push -q origin main",
		"git fetch -q origin")
	edit(t, filepath.Join(records, "design/parser.md"), "project: demo\n", "project: demo\ncovers: [src/parser.go]\n")
	run(t, records, "git commit -qam 'the parser page names its file'")
	_, errs, code := runPM(t, root, "check")
	if code != 1 || !strings.Contains(errs, `covers patterns of published design pages match no file on origin/main: `+
		`design/parser.md: covers pattern "src/parser.go"`) {
		t.Fatalf("pm check with a published pattern main lacks: exit %d %q", code, errs)
	}
	writeFile(t, filepath.Join(root, "src/parser.go"), "package src\n")
	run(t, root, "git add src", "git commit -qm parser", "git push -q origin main") // origin/main is now behind
	mustPM(t, root, "check")

	// one's draft names a file only one's branch has: held there, not on main, until the close
	page := filepath.Join(one.Dir, "design/parser.md")
	mustPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "One follows.")
	run(t, one.Code, "git merge -q main")
	edit(t, page, "covers: [src/parser.go]", "covers: [src/parser.go, src/tables.go]")
	mustPM(t, one.Code, "commit", "-m", "tables", "records/design/parser.md")
	if _, errs, code := runPM(t, one.Code, "check"); code != 1 || !strings.Contains(errs, "covers patterns of this "+
		`workspace's draft pages match no file this worktree tracks: design/parser.md: covers pattern "src/tables.go"`) {
		t.Fatalf("pm check of a draft naming an untracked file: exit %d %q", code, errs)
	}
	writeFile(t, filepath.Join(one.Code, "src/tables.go"), "package src\n")
	run(t, one.Code, "git add src", "git commit -qm tables")
	mustPM(t, one.Code, "check")
	mustPM(t, root, "check") // the draft is not published
	closeReady(t, records)
	_, errs, code = runPM(t, one.Code, "sprint", "close", "demo.1")
	if code != 1 || !strings.Contains(errs, "the design pages it publishes add covers patterns that name files "+
		`origin/main does not have: design/parser.md: covers pattern "src/tables.go"`) {
		t.Fatalf("the close before the merge: exit %d %q", code, errs)
	}
	run(t, root, "git merge -q one", "git push -q origin main")
	mustPM(t, one.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/parser.md"); !strings.Contains(got, "src/tables.go") {
		t.Errorf("the page was not published:\n%s", got)
	}
}

// A PR that removes a file a published page covers merges before its sprint closes: while a draft of the page changes
// its covers, pm check lists the pattern as waiting on that sprint's close and passes; with no such draft it fails.
func TestAPublishedPatternMainLostWaitsOnTheDraftThatChangesIt(t *testing.T) {
	root, records, one, _ := workspaces(t)
	origin := filepath.Join(filepath.Dir(root), "origin.git")
	writeFile(t, filepath.Join(root, "src/parser.go"), "package src\n")
	run(t, root, "git add src", "git commit -qm parser", "git init -q --bare "+origin, "git remote add origin "+origin,
		"git push -q origin main", "git fetch -q origin")
	edit(t, filepath.Join(records, "design/parser.md"), "project: demo\n", "project: demo\ncovers: [src/parser.go]\n")
	run(t, records, "git commit -qam 'the parser page names its file'")
	mustPM(t, root, "check")

	mustPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "One follows.")
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "covers: [src/parser.go]", "covers: [src/grammar.go]")
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1, the grammar\n")
	mustPM(t, one.Code, "commit", "-m", "the grammar replaces the parser", "records/design/parser.md")
	run(t, root, "git rm -q src/parser.go", "git commit -qm 'the parser goes'", "git push -q origin main")
	out := mustPM(t, root, "check")
	if !strings.Contains(out, "1 covers patterns of published pages match no file on origin/main, waiting on the close "+
		"of the sprint whose draft changes the page's covers:\n  design/parser.md: covers pattern \"src/parser.go\" "+
		"(sprint demo.1)") {
		t.Errorf("pm check with a pending draft: %q", out)
	}

	// with the draft keeping the page's covers, the same pattern fails
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "covers: [src/grammar.go]", "covers: [src/parser.go]")
	mustPM(t, one.Code, "commit", "-m", "back to the parser", "records/design/parser.md")
	if got := draftPages(t, records, one); got["design/parser.md"] != "demo.1" {
		t.Fatalf("one's draft: %v; want the page still drafted", got)
	}
	if _, errs, code := runPM(t, root, "check"); code != 1 || !strings.Contains(errs, "covers patterns of published "+
		`design pages match no file on origin/main: design/parser.md: covers pattern "src/parser.go"`) {
		t.Errorf("pm check with no pending draft: exit %d %q", code, errs)
	}
}

// A close whose drafts step fails after its records commit has recorded the discard: run again without --discard, it
// still does not publish the page, and drops its draft.
func TestARerunAfterAFailedDropKeepsTheDiscard(t *testing.T) {
	_, records, one, two := workspaces(t)
	edit(t, filepath.Join(two.Dir, "design/parser.md"), "\nP2\n", "\nP2 by two\n")
	mustPM(t, two.Code, "commit", "-m", "two's parser", "records/design/parser.md")
	published := gitOut(t, records, "show", "records:design/parser.md")
	closed := closeReady(t, records)
	saved := closeDone
	t.Cleanup(func() { closeDone = saved })
	closeDone = func(*design.Closing, string) error { return errors.New("the drop stopped") }
	_, errs, code := runPM(t, one.Code, "sprint", "close", "demo.1", "--discard", "parser", "--reason", "It holds.")
	if code != 1 || !strings.Contains(errs, "the drop stopped; the close's records commit is") || len(*closed) != 0 {
		t.Fatalf("the close with a failing drop: exit %d %q, closed %v", code, errs, *closed)
	}
	closeDone = saved
	mustPM(t, one.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/parser.md"); got != published {
		t.Errorf("the rerun published a discarded draft:\n%s", got)
	}
	if got := draftPages(t, records, two); len(got) != 0 || len(*closed) != 1 {
		t.Errorf("after the rerun: two drafts %v, closed %v", got, *closed)
	}
}

// A follow that leaves a drafted page in conflict keeps its sprint: a close from another worktree refuses, naming the
// worktree and the page, until the edit is resolved and committed there; then it publishes it.
func TestACloseRefusesWhileADraftOfItsPageIsInConflictElsewhere(t *testing.T) {
	_, records, one, two := workspaces(t)
	rel := "records/design/parser.md"
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	mustPM(t, one.Code, "commit", "-m", "one's parser", rel)
	edit(t, filepath.Join(records, "design/parser.md"), "\nP1\n", "\nP1 by sprint two\n")
	run(t, records, "git commit -qam 'sprint two published the parser'")
	mustPM(t, one.Code, "finding", "add", "--sprint", "demo.1", "One follows into a conflict.")
	path := filepath.Join(one.Dir, "design/parser.md")
	if !store.HasMarkers(fileText(t, path)) {
		t.Fatal("the follow left no markers in one")
	}
	closeReady(t, records)
	before := gitOut(t, records, "rev-parse", "records")
	_, errs, code := runPM(t, two.Code, "sprint", "close", "demo.1")
	if code != 1 || !strings.Contains(errs, "records/design/parser.md in worktree "+one.Code+" holds the sprint's edit "+
		"with conflict markers") {
		t.Fatalf("the close from two: exit %d %q; want refused naming one", code, errs)
	}
	if got := gitOut(t, records, "rev-parse", "records"); got != before {
		t.Fatal("the refused close changed records")
	}
	conflict := regexp.MustCompile(`(?s)<<<<<<< workspace\nP1 by one\n=======\nP1 by sprint two\n>>>>>>> records [^\n]*\n`)
	writeFile(t, path, conflict.ReplaceAllString(fileText(t, path), "P1 by both\n"))
	mustPM(t, one.Code, "commit", "-m", "both", "--sprint", "demo.1", rel)
	mustPM(t, two.Code, "sprint", "close", "demo.1")
	if got := gitOut(t, records, "show", "records:design/parser.md"); !strings.Contains(got, "\nP1 by both\n") {
		t.Errorf("the published page:\n%s", got)
	}
}

// A page's draft in one workspace belongs to one sprint: pm commit refuses it for another open sprint while it is
// drafted there for the first.
func TestAPageDraftedForOneSprintIsRefusedForAnother(t *testing.T) {
	_, records, one, _ := workspaces(t)
	rel := "records/design/parser.md"
	edit(t, filepath.Join(records, "sprints/demo-2.md"), "> Where is the detail?\n\nNone yet.",
		"> Where is the detail?\n\n[Parser](../design/parser.md)")
	run(t, records, "git commit -qam 'sprint two links the parser'")
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP1\n", "\nP1 by one\n")
	mustPM(t, one.Code, "commit", "-m", "for one", "--sprint", "demo.1", rel)
	edit(t, filepath.Join(one.Dir, "design/parser.md"), "\nP2\n", "\nP2 for two\n")
	_, errs, code := runPM(t, one.Code, "commit", "-m", "for two", "--sprint", "demo.2", rel)
	if code != 1 || !strings.Contains(errs, "records/design/parser.md is drafted in this workspace for sprint demo.1") {
		t.Fatalf("a second sprint's commit: exit %d %q", code, errs)
	}
	if got := draftPages(t, records, one); got["design/parser.md"] != "demo.1" {
		t.Errorf("the draft after the refusal: %v", got)
	}
	mustPM(t, one.Code, "commit", "-m", "for one again", "--sprint", "demo.1", rel)
}
