package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The expected texts below follow from the record-drafts page's Follow section, not from this code: disjoint hunks
// merge, overlapping ones leave git merge-file's markers in the following workspace only, a conflicting draft page
// leaves the draft commit as an uncommitted edit, and a follow that fails changes nothing.

// lines is a record body of numbered paragraphs, each a hunk of its own: one unchanged line apart, they merge cleanly.
const lines = "a\n\nb\n\nc\n\nd\n\ne\n"

// workspace is a code worktree of root on a new branch, with its records workspace made as pm init makes it.
func workspace(t *testing.T, root, store, name string) Workspace {
	t.Helper()
	code := filepath.Join(filepath.Dir(root), name)
	sh(t, root, "git worktree add -q -b "+name+" "+code)
	if said, err := AddWorkspace(store, code); err != nil || said == "" {
		t.Fatalf("AddWorkspace %s: %q, %v", name, said, err)
	}
	w, err := WorkspaceAt(code)
	if err != nil || w == nil {
		t.Fatalf("WorkspaceAt %s: %v, %v", code, w, err)
	}
	return *w
}

// publish commits text at rel on the records branch from the store, as pm commit and the one-shot writes do.
func publish(t *testing.T, store, rel, text string) string {
	t.Helper()
	if err := Apply(store, []Write{{Path: filepath.Join(store, rel), Text: text}}, "publish "+rel, "", ""); err != nil {
		t.Fatal(err)
	}
	return sh(t, store, "git rev-parse --short HEAD")
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func follow(t *testing.T, store string, w Workspace) Followed {
	t.Helper()
	f, err := Follow(store, w)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// clean is whether the workspace sits on the records tip with no uncommitted file and no merge or rebase in progress.
func clean(t *testing.T, store string, w Workspace) {
	t.Helper()
	if head, tip := sh(t, store, "git rev-parse "+w.Ref()), sh(t, store, "git rev-parse records"); head != tip {
		t.Errorf("%s is at %s, not the records tip %s", w.Branch(), head, tip)
	}
	if dirty, err := w.Dirty(); err != nil || dirty != nil {
		t.Errorf("%s has uncommitted files %v (%v)", w.Dir, dirty, err)
	}
	midOperation(t, w)
}

// midOperation fails when git holds a merge, rebase, cherry-pick or stash state for the workspace.
func midOperation(t *testing.T, w Workspace) {
	t.Helper()
	gitDir := sh(t, w.Dir, "git rev-parse --absolute-git-dir")
	for _, f := range []string{"MERGE_HEAD", "REBASE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD"} {
		if _, err := os.Stat(filepath.Join(gitDir, f)); err == nil {
			t.Errorf("%s holds %s: left mid-operation", w.Dir, f)
		}
	}
	if stash := sh(t, w.Dir, "git stash list"); stash != "" {
		t.Errorf("a stash: %s", stash)
	}
}

func TestAWorkspaceTakesThePlaceOfTheLinkAndTheMainCheckoutKeepsIt(t *testing.T) {
	root, store := clone(t)
	if err := os.Symlink(store, filepath.Join(root, "records")); err != nil {
		t.Fatal(err)
	}
	if w, err := WorkspaceAt(root); err != nil || w != nil {
		t.Errorf("the main checkout: %v, %v; want no workspace", w, err)
	}
	if _, err := AddWorkspace(store, root); err == nil {
		t.Error("the main checkout was given a workspace")
	}
	code := filepath.Join(filepath.Dir(root), "wt")
	sh(t, root, "git worktree add -q -b wt "+code)
	if err := os.Symlink(store, filepath.Join(code, "records")); err != nil {
		t.Fatal(err)
	}
	if w, err := WorkspaceAt(code); err != nil || w != nil {
		t.Fatalf("a worktree with the link: %v, %v; want no workspace yet", w, err)
	}
	said, err := AddWorkspace(store, code)
	if err != nil || said != "made "+filepath.Join(code, "records")+" a records workspace on branch drafts/wt" {
		t.Fatalf("AddWorkspace: %q, %v", said, err)
	}
	w, err := WorkspaceAt(code)
	if err != nil || w == nil || w.Branch() != "drafts/wt" {
		t.Fatalf("WorkspaceAt: %v, %v", w, err)
	}
	if got := read(t, w.Dir, "projects/demo.md"); got != project {
		t.Errorf("the workspace holds %q, not the records tip's text", got)
	}
	if said, err := AddWorkspace(store, code); err != nil || said != "" {
		t.Errorf("a second AddWorkspace: %q, %v; want nothing done", said, err)
	}
	inside, err := WorkspaceOf(filepath.Join(w.Dir, "projects"))
	if err != nil || inside == nil || inside.Dir != w.Dir {
		t.Errorf("WorkspaceOf from inside it: %v, %v", inside, err)
	}
	if got, err := CodeRoot(w.Dir, store); err != nil || got != code {
		t.Errorf("CodeRoot from the workspace: %q, %v; want its code worktree", got, err)
	}
	clean(t, store, *w)
}

func TestFollowMergesDisjointEditsAndMarksAnOverlapOnlyInTheSecondWorkspace(t *testing.T) {
	root, store := clone(t)
	publish(t, store, "sprints/s.md", lines)
	one, two := workspace(t, root, store, "one"), workspace(t, root, store, "two")

	// disjoint: one changes a, two changes e; one publishes first
	write(t, one.Dir, "sprints/s.md", strings.Replace(lines, "a\n", "A one\n", 1))
	write(t, two.Dir, "sprints/s.md", strings.Replace(lines, "e\n", "E two\n", 1))
	first := publish(t, store, "sprints/s.md", read(t, one.Dir, "sprints/s.md"))
	if f := follow(t, store, one); f.Conflicts != nil {
		t.Fatalf("one's own edit conflicts: %v", f.Conflicts)
	}
	clean(t, store, one)
	f := follow(t, store, two)
	both := "A one\n\nb\n\nc\n\nd\n\nE two\n"
	if got := read(t, two.Dir, "sprints/s.md"); got != both || f.Conflicts != nil {
		t.Fatalf("two after the follow: %q, conflicts %v; want both edits %q", got, f.Conflicts, both)
	}
	if f.Tip != sh(t, store, "git rev-parse records") || !strings.HasPrefix(sh(t, store, "git rev-parse "+first), f.Tip[:7]) {
		t.Errorf("Followed = %+v; want the tip %s", f, first)
	}
	publish(t, store, "sprints/s.md", both)
	follow(t, store, two)
	clean(t, store, two)
	midOperation(t, two)

	// overlap: both change c; one publishes first, so two's follow leaves markers in two alone
	write(t, one.Dir, "sprints/s.md", strings.Replace(both, "c\n", "C one\n", 1))
	write(t, two.Dir, "sprints/s.md", strings.Replace(both, "c\n", "C two\n", 1))
	first = publish(t, store, "sprints/s.md", read(t, one.Dir, "sprints/s.md"))
	follow(t, store, one)
	f = follow(t, store, two)
	got := read(t, two.Dir, "sprints/s.md")
	if !reflect.DeepEqual(f.Conflicts, []string{"sprints/s.md"}) || !HasMarkers(got) ||
		!strings.Contains(got, "<<<<<<< workspace\nC two\n=======\nC one\n>>>>>>> records ") {
		t.Fatalf("two after an overlapping follow: conflicts %v, text %q", f.Conflicts, got)
	}
	for dir, what := range map[string]string{one.Dir: "one", store: "the store"} {
		if HasMarkers(read(t, dir, "sprints/s.md")) {
			t.Errorf("%s holds conflict markers", what)
		}
	}
	commits, err := MarkerCommits(store, "sprints/s.md", got)
	if err != nil || len(commits) != 1 || !strings.HasPrefix(commits[0], first+" ") {
		t.Errorf("MarkerCommits = %v, %v; want one's commit %s", commits, err, first)
	}
	if dirty := dirtyOf(t, two); !reflect.DeepEqual(dirty, []string{"sprints/s.md"}) {
		t.Errorf("two's markers are not its uncommitted edit: %v", dirty)
	}
	midOperation(t, two)
}

func TestAConflictingDraftPageLeavesTheDraftCommitAsAnUncommittedEdit(t *testing.T) {
	root, store := clone(t)
	publish(t, store, "design/p.md", lines)
	publish(t, store, "design/q.md", lines)
	w := workspace(t, root, store, "wt")
	write(t, w.Dir, "design/p.md", strings.Replace(lines, "c\n", "C draft\n", 1))
	write(t, w.Dir, "design/q.md", strings.Replace(lines, "a\n", "A draft\n", 1))
	if _, err := CommitDraft(store, w, map[string]string{"design/p.md": "pm-1.2", "design/q.md": "pm-1.3"}); err != nil {
		t.Fatal(err)
	}
	pages, tags, err := DraftPages(store, w)
	if err != nil || len(pages) != 2 || !reflect.DeepEqual(tags, map[string]string{"design/p.md": "pm-1.2",
		"design/q.md": "pm-1.3"}) {
		t.Fatalf("DraftPages = %v, %v, %v", pages, tags, err)
	}
	if dirty := dirtyOf(t, w); dirty != nil {
		t.Fatalf("the draft commit left uncommitted files: %v", dirty)
	}

	// records changes p's c (an overlap) and q's e (disjoint from the draft's a)
	publish(t, store, "design/p.md", strings.Replace(lines, "c\n", "C published\n", 1))
	publish(t, store, "design/q.md", strings.Replace(lines, "e\n", "E published\n", 1))
	f := follow(t, store, w)
	if !reflect.DeepEqual(f.Conflicts, []string{"design/p.md"}) {
		t.Fatalf("conflicts = %v; want design/p.md", f.Conflicts)
	}
	pages, tags, err = DraftPages(store, w)
	wantQ := "A draft\n\nb\n\nc\n\nd\n\nE published\n"
	if err != nil || len(pages) != 1 || pages["design/q.md"] == nil || *pages["design/q.md"] != wantQ ||
		tags["design/q.md"] != "pm-1.3" {
		t.Fatalf("the draft after the follow: %v, %v, %v; want q alone, merged", pages, tags, err)
	}
	if got := read(t, w.Dir, "design/q.md"); got != wantQ {
		t.Errorf("q's file: %q", got)
	}
	if got := read(t, w.Dir, "design/p.md"); !strings.Contains(got, "<<<<<<< workspace\nC draft\n=======\nC published\n") {
		t.Errorf("p's file holds no merged markers: %q", got)
	}
	if dirty := dirtyOf(t, w); !reflect.DeepEqual(dirty, []string{"design/p.md"}) {
		t.Errorf("uncommitted after the follow: %v; want p alone", dirty)
	}
	if base := sh(t, store, "git rev-parse "+w.Ref()+"^"); base != sh(t, store, "git rev-parse records") {
		t.Errorf("the draft commit is on %s, not the records tip", base)
	}
	midOperation(t, w)
}

func TestAFollowThatFailsChangesNothing(t *testing.T) {
	root, store := clone(t)
	publish(t, store, "sprints/s.md", lines)
	w := workspace(t, root, store, "wt")
	mine := strings.Replace(lines, "e\n", "E mine\n", 1)
	write(t, w.Dir, "sprints/s.md", mine)
	write(t, w.Dir, "docs/new.md", "new")
	publish(t, store, "sprints/s.md", strings.Replace(lines, "a\n", "A published\n", 1))
	publish(t, store, "projects/demo.md", project+"more\n")
	head := sh(t, store, "git rev-parse "+w.Ref())
	status := sh(t, w.Dir, "git status --porcelain")
	// git cannot move a ref another process holds the lock of
	lock := filepath.Join(sh(t, root, "git rev-parse --path-format=absolute --git-common-dir"), "refs/heads/drafts/wt.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Follow(store, w); err == nil {
		t.Fatal("the follow passed with its branch locked")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if got := read(t, w.Dir, "sprints/s.md"); got != mine {
		t.Errorf("the uncommitted edit changed: %q", got)
	}
	if got := read(t, w.Dir, "projects/demo.md"); got != project {
		t.Errorf("a committed file was refreshed: %q", got)
	}
	if got := sh(t, store, "git rev-parse "+w.Ref()); got != head {
		t.Errorf("the branch moved to %s", got)
	}
	if got := sh(t, w.Dir, "git status --porcelain"); got != status {
		t.Errorf("git status %q, was %q", got, status)
	}
	midOperation(t, w)

	f := follow(t, store, w)
	if got := read(t, w.Dir, "sprints/s.md"); got != "A published\n\nb\n\nc\n\nd\n\nE mine\n" || f.Conflicts != nil {
		t.Errorf("the follow once the lock went: %q, %v", got, f.Conflicts)
	}
	if got := read(t, w.Dir, "projects/demo.md"); got != project+"more\n" {
		t.Errorf("a committed file was not refreshed: %q", got)
	}
	if dirty := dirtyOf(t, w); !reflect.DeepEqual(dirty, []string{"sprints/s.md", "docs/new.md"}) { // tracked first
		t.Errorf("uncommitted after the follow: %v", dirty)
	}
}

func TestAHandCommitInAWorkspaceIsRefused(t *testing.T) {
	root, store := clone(t)
	w := workspace(t, root, store, "wt")
	write(t, w.Dir, "docs/x.md", "x")
	sh(t, w.Dir, "git add -A", "git commit -qm by-hand")
	publish(t, store, "projects/demo.md", project+"more\n")
	if _, err := Follow(store, w); err == nil || !strings.Contains(err.Error(), "neither a records commit nor a draft") {
		t.Errorf("Follow over a hand commit: %v", err)
	}
}

func dirtyOf(t *testing.T, w Workspace) []string {
	t.Helper()
	dirty, err := w.Dirty()
	if err != nil {
		t.Fatal(err)
	}
	return dirty
}
