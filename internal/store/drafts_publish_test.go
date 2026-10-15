package store

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The expected outcomes follow from the record-drafts page's Publish at sprint close: an overlap changes nothing and
// names what overlaps; a close stopped after its records commit finds nothing left to publish.

// draft commits the edit of rel in w's workspace into its draft commit for sprint.
func draft(t *testing.T, store string, w Workspace, rel, old, now, sprint string) {
	t.Helper()
	write(t, w.Dir, rel, strings.Replace(read(t, w.Dir, rel), old, now, 1))
	if _, err := CommitDraft(store, w, map[string]string{rel: sprint}); err != nil {
		t.Fatal(err)
	}
}

func TestTwoDraftsOfTheSprintThatOverlapAreRefusedNamingTheOtherBranch(t *testing.T) {
	root, store := clone(t)
	publish(t, store, "design/p.md", lines)
	one, two := workspace(t, root, store, "one"), workspace(t, root, store, "two")
	draft(t, store, one, "design/p.md", "c\n", "C one\n", "pm-1")
	draft(t, store, two, "design/p.md", "c\n", "C two\n", "pm-1")
	draft(t, store, two, "design/p.md", "a\n", "A two\n", "pm-1")
	tip := sh(t, store, "git rev-parse records")
	_, err := PlanPublish(store, "pm-1", nil)
	if err == nil || !strings.Contains(err.Error(), "records/design/p.md on drafts/two overlaps the sprint's draft of it "+
		"on drafts/one") {
		t.Fatalf("PlanPublish: %v; want the overlap with drafts/one named", err)
	}
	if got := sh(t, store, "git rev-parse records"); got != tip {
		t.Errorf("a refused plan moved records")
	}
	// another sprint's draft of the page is not the sprint's: it neither publishes nor overlaps
	p, err := PlanPublish(store, "pm-2", nil)
	if err != nil || len(p.Pages) != 0 {
		t.Errorf("PlanPublish of a sprint with no draft: %v, %v", p, err)
	}
}

func TestACloseStoppedBeforeItDroppedTheDraftsPublishesNothingAgain(t *testing.T) {
	root, store := clone(t)
	publish(t, store, "design/p.md", lines)
	publish(t, store, "design/q.md", lines)
	one, two := workspace(t, root, store, "one"), workspace(t, root, store, "two")
	draft(t, store, one, "design/p.md", "a\n", "A one\n", "pm-1")
	draft(t, store, two, "design/p.md", "e\n", "E two\n", "pm-1")
	draft(t, store, two, "design/q.md", "b\n", "B two\n", "pm-2")
	p, err := PlanPublish(store, "pm-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "A one\n\nb\n\nc\n\nd\n\nE two\n"
	if len(p.Pages) != 1 || p.Pages["design/p.md"] == nil || *p.Pages["design/p.md"] != want {
		t.Fatalf("Pages = %v; want p with both drafts' edits", p.Pages)
	}
	err = Apply(store, []Write{{Path: filepath.Join(store, "design/p.md"), Text: want}}, "close\n\n"+p.Trailer("pm-1"),
		"", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := PlanPublish(store, "pm-1", nil)
	if err != nil || len(again.Pages) != 0 {
		t.Fatalf("the plan after the records commit: %v, %v; want nothing to publish", again, err)
	}
	if err := again.Drop(store); err != nil {
		t.Fatal(err)
	}
	if pages, tags, err := DraftPages(store, two); err != nil || len(pages) != 1 ||
		!reflect.DeepEqual(tags, map[string]string{"design/q.md": "pm-2"}) {
		t.Errorf("two's draft after the drop: %v, %v, %v; want q for pm-2 alone", pages, tags, err)
	}
	// two's next follow drops p from its tree: its file takes the published text, q stays drafted
	follow(t, store, two)
	if got := read(t, two.Dir, "design/p.md"); got != want {
		t.Errorf("two's p after the follow: %q", got)
	}
	if dirty := dirtyOf(t, two); dirty != nil {
		t.Errorf("two has uncommitted %v", dirty)
	}
	if pages, _, err := DraftPages(store, two); err != nil || len(pages) != 1 || pages["design/q.md"] == nil {
		t.Errorf("two's draft after the follow: %v, %v", pages, err)
	}
}

// A rename on records of a page a draft commit holds: the follow carries the page's sprint to its new path, where
// git's merge puts the draft's edit, and does not refuse.
func TestFollowCarriesADraftPagesSprintAcrossARenameOnRecords(t *testing.T) {
	root, store := clone(t)
	text := "---\ntype: design\ntitle: P\n---\n\n" + lines
	publish(t, store, "design/p.md", text)
	w := workspace(t, root, store, "wt")
	draft(t, store, w, "design/p.md", "c\n", "C draft\n", "pm-1")
	renamed := strings.Replace(text, "title: P", "title: R", 1)
	if err := Apply(store, []Write{{Path: filepath.Join(store, "design/p.md"), Remove: true},
		{Path: filepath.Join(store, "design/r.md"), Text: renamed}}, "rename p to r", "", ""); err != nil {
		t.Fatal(err)
	}
	follow(t, store, w)
	pages, tags, err := DraftPages(store, w)
	if err != nil || len(pages) != 1 || pages["design/r.md"] == nil || tags["design/r.md"] != "pm-1" {
		t.Fatalf("the draft after the follow: %v, %v, %v; want r for pm-1", pages, tags, err)
	}
	if want := strings.Replace(renamed, "c\n", "C draft\n", 1); *pages["design/r.md"] != want ||
		read(t, w.Dir, "design/r.md") != want {
		t.Errorf("r in the draft: %q; want the rename with the draft's edit", *pages["design/r.md"])
	}
	if dirty := dirtyOf(t, w); dirty != nil {
		t.Errorf("uncommitted after the follow: %v", dirty)
	}
}
