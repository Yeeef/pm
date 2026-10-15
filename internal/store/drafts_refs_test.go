package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// The expected outcomes follow from the record-drafts page's Draft refs: each clone pushes its draft commits to its
// own namespace with a lease, never forced; the other clones list them as committed drafts with no session, and a
// close in any clone publishes them; the owning clone deletes a ref once its draft is empty or published.

// twoClones is clone A (root a, store sa) with design/p.md published, its bare origin, and clone B of that origin
// (store sb) with its store as pm init makes it; each has its clone id.
func twoClones(t *testing.T) (a, sa, sb string) {
	t.Helper()
	a, sa = clone(t)
	publish(t, sa, "design/p.md", lines)
	origin := filepath.Join(filepath.Dir(a), "origin.git")
	sh(t, a, "git clone -q --bare . "+origin, "git remote add origin "+origin, "git fetch -q origin")
	b := filepath.Join(filepath.Dir(a), "b")
	sh(t, filepath.Dir(a), "git clone -q "+origin+" b")
	sh(t, b, "git worktree add -q .pm/store/records records")
	sb = filepath.Join(b, ".pm/store/records")
	for _, dir := range []string{a, b} {
		if said, err := MakeCloneID(dir); err != nil || said == "" {
			t.Fatalf("MakeCloneID %s: %q, %v", dir, said, err)
		}
	}
	return a, sa, sb
}

func id(t *testing.T, dir string) string {
	t.Helper()
	id, err := CloneID(dir)
	if err != nil || id == "" {
		t.Fatalf("CloneID %s: %q, %v", dir, id, err)
	}
	return id
}

// syncDrafts is one sync's drafts step in the clone of store: its draft refs pushed, the other clones' fetched.
func syncDrafts(t *testing.T, store string) string {
	t.Helper()
	said, err := PushDrafts(store, "origin", id(t, store))
	if err != nil {
		t.Fatal(err)
	}
	if err := FetchDrafts(store, "origin", id(t, store)); err != nil {
		t.Fatal(err)
	}
	return said
}

// remoteDraftRefs is the origin's draft refs, as git ls-remote prints them.
func remoteDraftRefs(t *testing.T, store string) string {
	t.Helper()
	return sh(t, store, "git ls-remote origin 'refs/pm/drafts/*'")
}

func TestAnotherClonesDraftIsListedPublishedByACloseHereAndItsRefDeletedByItsClone(t *testing.T) {
	a, sa, sb := twoClones(t)
	ws := workspace(t, a, sa, "wa")
	draft(t, sa, ws, "design/p.md", "c\n", "C in clone A\n", "pm-1")
	if said := syncDrafts(t, sa); said != "pushed 1 and deleted 0 draft refs" {
		t.Errorf("A's sync said %q", said)
	}
	syncDrafts(t, sb)
	if got := sh(t, sa, "git for-each-ref refs/pm/fetched"); got != "" {
		t.Errorf("A fetched its own draft back: %s", got)
	}

	idA := id(t, sa)
	var listed *Draft
	for _, d := range (Clone{Store: sb}).Drafts() {
		if d.Name == idA+"~wa" {
			listed = &d
		}
	}
	want := "clone " + idA + ", worktree wa, sprint pm-1, no session"
	if listed == nil || listed.Err != nil || listed.Label != want || listed.State != "committed" ||
		listed.Files["design/p.md"] == nil || !strings.Contains(*listed.Files["design/p.md"], "C in clone A\n") {
		t.Fatalf("B lists A's draft as %+v; want it committed, labelled %q, with A's text", listed, want)
	}
	drafted, warnings, err := Drafted(sb)
	if err != nil || warnings != nil || !drafted["pm-1"]["design/p.md"] {
		t.Errorf("B's Drafted = %v, %v, %v; want A's page as pm-1's draft", drafted, warnings, err)
	}

	p, err := PlanPublish(sb, "pm-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := p.Pages["design/p.md"]; text == nil || !strings.Contains(*text, "C in clone A\n") || len(p.Others) != 1 {
		t.Fatalf("B's close publishes %v, notes %v; want A's page and a note naming A's draft", p.Pages, p.Others)
	}
	if err := Apply(sb, []Write{{Path: filepath.Join(sb, "design/p.md"), Text: *p.Pages["design/p.md"]}},
		"close\n\n"+p.Trailer("pm-1"), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Drop(sb); err != nil {
		t.Fatal(err)
	}
	if again, err := PlanPublish(sb, "pm-1", nil); err != nil || len(again.Pages) != 0 {
		t.Errorf("a second close from B publishes %v (%v); want nothing", again, err)
	}
	for _, d := range (Clone{Store: sb}).Drafts() {
		if d.Name == idA+"~wa" && len(d.Files) > 0 {
			t.Errorf("B still lists A's published page as a draft: %v", d.Files)
		}
	}
	if remoteDraftRefs(t, sb) == "" {
		t.Fatal("B's close wrote A's namespace: A's ref is gone before A's sync")
	}

	sh(t, sb, "git push -q origin records")
	sh(t, sa, "git fetch -q origin records") // the records step's fetch
	if said := syncDrafts(t, sa); said != "pushed 0 and deleted 1 draft refs" {
		t.Errorf("A's sync after the close said %q", said)
	}
	if got := remoteDraftRefs(t, sa); got != "" {
		t.Errorf("A's ref is still on the remote after its sync: %s", got)
	}
	syncDrafts(t, sb)
	if got := sh(t, sb, "git for-each-ref refs/pm/fetched"); got != "" {
		t.Errorf("B keeps a fetched ref the remote no longer has: %s", got)
	}
}

func TestALeaseThatFailsIsReportedAndNeverForced(t *testing.T) {
	a, sa, sb := twoClones(t)
	ws := workspace(t, a, sa, "wa")
	draft(t, sa, ws, "design/p.md", "c\n", "C one\n", "pm-1")
	syncDrafts(t, sa)
	ref := "refs/pm/drafts/" + id(t, sa) + "/wa"
	sh(t, sb, "git commit-tree -m other records^{tree} > other.sha", "git push -q origin +$(cat other.sha):"+ref)
	other := sh(t, sb, "cat other.sha")

	draft(t, sa, ws, "design/p.md", "a\n", "A two\n", "pm-1")
	_, err := PushDrafts(sa, "origin", id(t, sa))
	if err == nil || !strings.Contains(err.Error(), ref+" on origin is not at ") ||
		!strings.Contains(err.Error(), "so pm did not force it") {
		t.Fatalf("PushDrafts over another writer: %v; want the lease's failure", err)
	}
	if got := strings.Fields(remoteDraftRefs(t, sa)); len(got) != 2 || got[0] != other {
		t.Errorf("the remote ref is %v; want the other writer's %s kept", got, other)
	}
}

func TestADraftEmptiedOrGoneDeletesItsRemoteRef(t *testing.T) {
	a, sa, _ := twoClones(t)
	one, two := workspace(t, a, sa, "one"), workspace(t, a, sa, "two")
	draft(t, sa, one, "design/p.md", "c\n", "C one\n", "pm-1")
	draft(t, sa, two, "design/p.md", "a\n", "A two\n", "pm-1")
	if said := syncDrafts(t, sa); said != "pushed 2 and deleted 0 draft refs" {
		t.Fatalf("the sync said %q", said)
	}
	draft(t, sa, one, "design/p.md", "C one\n", "c\n", "pm-1") // the page back to its published text: an empty draft
	sh(t, a, "git worktree remove --force --force "+two.Code, "git worktree prune")
	if err := two.DeleteBranch(sa); err != nil {
		t.Fatal(err)
	}
	if said := syncDrafts(t, sa); said != "pushed 0 and deleted 2 draft refs" {
		t.Errorf("the sync said %q", said)
	}
	if got := remoteDraftRefs(t, sa) + sh(t, sa, "git for-each-ref refs/pm/pushed"); got != "" {
		t.Errorf("refs left: %s", got)
	}
}

func TestAnotherClonesDraftThatOverlapsWhatRecordsPublishedIsRefusedNamingItsClone(t *testing.T) {
	a, sa, sb := twoClones(t)
	ws := workspace(t, a, sa, "wa")
	draft(t, sa, ws, "design/p.md", "c\n", "C in clone A\n", "pm-1")
	syncDrafts(t, sa)
	syncDrafts(t, sb)
	published := publish(t, sb, "design/p.md", strings.Replace(lines, "c\n", "C in clone B\n", 1))
	tip := sh(t, sb, "git rev-parse records")
	_, err := PlanPublish(sb, "pm-1", nil)
	if err == nil || !strings.Contains(err.Error(), "records/design/p.md on clone "+id(t, sa)+"'s drafts/wa overlaps "+
		"what the records branch published since its base, in "+published) {
		t.Fatalf("PlanPublish: %v; want the overlap with %s named, and A's draft", err, published)
	}
	if got := sh(t, sb, "git rev-parse records"); got != tip {
		t.Errorf("a refused plan moved records")
	}
}
