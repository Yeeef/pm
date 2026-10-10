package work

import (
	"strings"
	"testing"
	"time"
)

// The merge table of the work-store page's Data model ("Concurrent writers"), one test per row whose field lives in the
// items row: base, ours and theirs are the three versions of a row both clones changed, as Dolt's
// dolt_conflicts_items gives them. The rows for list fields (comments, labels, blocked_by) are rows of their own
// tables, which Dolt merges; their tests are in sync_test.go, through two clones and a remote.

var t0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func sec(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

// row is an open task as the base has it.
func row() Item {
	return Item{ID: "demo-abc.1.1", Type: Task, Parent: "demo-abc.1", Title: "T", Description: "d", Status: Open,
		CreatedAt: sec(0), UpdatedAt: sec(0)}
}

// sides is the base and two copies of it for each side to change, the updated_at of ours and theirs set.
func sides(ours, theirs int) (Item, Item, Item) {
	b, o, t := row(), row(), row()
	o.UpdatedAt, t.UpdatedAt = sec(ours), sec(theirs)
	return b, o, t
}

func merged(t *testing.T, b, o, th Item) (Item, *Override) {
	t.Helper()
	m, over, err := mergeItem(b, o, th)
	if err != nil {
		t.Fatal(err)
	}
	return m, over
}

func refused(t *testing.T, b, o, th Item, field string) {
	t.Helper()
	_, _, err := mergeItem(b, o, th)
	if err == nil || !strings.Contains(err.Error(), "field "+field) || !strings.Contains(err.Error(), b.ID) ||
		!strings.Contains(err.Error(), "no merge rule settles it") {
		t.Fatalf("want a failure naming %s and field %s, got %v", b.ID, field, err)
	}
}

func TestMergeAFieldOneSideChangedTakesThatSide(t *testing.T) {
	b, o, th := sides(5, 6)
	o.Title = "ours"          // only ours changed the title
	th.Description = "theirs" // only theirs changed the description
	m, _ := merged(t, b, o, th)
	if m.Title != "ours" || m.Description != "theirs" || m.UpdatedAt != sec(6) {
		t.Fatalf("%+v", m)
	}
}

func TestMergeTypeNeverChangesAndNumberChangedOnBothSidesFails(t *testing.T) {
	b, o, th := sides(5, 6)
	th.Type = Need
	refused(t, b, o, th, "type")
	b, o, th = sides(5, 6)
	for _, it := range []*Item{&b, &o, &th} {
		it.Type, it.Number, it.Parent = Sprint, 7, "demo-abc"
	}
	o.Number, th.Number = 8, 9 // both sides moved it
	refused(t, b, o, th, "number")
}

// A sprint moved on one side (parent, number, title) while the other edited it: the move holds, and the title the
// later side gave keeps the moved number.
func TestMergeASprintMovedOnOneSideKeepsTheMove(t *testing.T) {
	b, o, th := sides(5, 6)
	for _, it := range []*Item{&b, &o, &th} {
		it.Type, it.Number, it.Parent, it.Title = Sprint, 1, "demo-abc", "Sprint 1: S"
	}
	o.Number, o.Parent, o.Title = 4, "demo-xyz", "Sprint 4: S" // ours moved it, earlier
	th.Title, th.Description = "Sprint 1: S renamed", "d"      // theirs edited it, later
	m, _ := merged(t, b, o, th)
	if m.Number != 4 || m.Parent != "demo-xyz" || m.Title != "Sprint 4: S renamed" || m.Description != "d" {
		t.Fatalf("%+v", m)
	}
}

func TestMergeStatusClosedWins(t *testing.T) {
	for _, closer := range []string{"ours", "theirs"} {
		b, o, th := sides(5, 6)
		c := &o
		if closer == "theirs" {
			c = &th
		}
		c.Status, c.Resolution, c.CloseReason, c.ClosedAt, c.ClosedBy = Closed, Done, "shipped", sec(4), "s1"
		m, _ := merged(t, b, o, th)
		if m.Status != Closed || m.Resolution != Done || m.CloseReason != "shipped" || m.ClosedAt != sec(4) ||
			m.ClosedBy != "s1" {
			t.Fatalf("%s closed: %+v", closer, m)
		}
	}
}

func TestMergeCloseFieldsFromTheEarlierClose(t *testing.T) {
	b, o, th := sides(5, 6)
	o.Status, o.Resolution, o.CloseReason, o.ClosedAt, o.ClosedBy = Closed, Done, "ours", sec(5), "s1"
	th.Status, th.Resolution, th.CloseReason, th.ClosedAt, th.ClosedBy = Closed, Done, "theirs", sec(3), "s2"
	m, _ := merged(t, b, o, th)
	if m.CloseReason != "theirs" || m.ClosedAt != sec(3) || m.ClosedBy != "s2" {
		t.Fatalf("%+v", m)
	}
	// Both closed in the same second, differently: no winner.
	th.ClosedAt = sec(5)
	refused(t, b, o, th, "closed_at, closed_by, close_reason")
}

func TestMergeResolutionNotNullThenLaterUpdatedAt(t *testing.T) {
	// A closed need: one side marks it no-decision after the close (pm decision close), the other leaves it; then
	// both set it, and the later updated_at wins.
	b, o, th := sides(5, 9)
	for _, it := range []*Item{&b, &o, &th} {
		it.Status, it.Resolution, it.CloseReason, it.ClosedAt = Closed, Answered, "Responded", sec(1)
	}
	th.Resolution = NoDecision
	if m, _ := merged(t, b, o, th); m.Resolution != NoDecision {
		t.Fatalf("one side set it: %+v", m)
	}
	o.Resolution = Dismissed // both set it: theirs is later (9 > 5)
	if m, _ := merged(t, b, o, th); m.Resolution != NoDecision {
		t.Fatalf("both set it: %+v", m)
	}
	o.UpdatedAt = sec(10)
	if m, _ := merged(t, b, o, th); m.Resolution != Dismissed {
		t.Fatalf("ours later: %+v", m)
	}
	o.UpdatedAt = sec(9)
	refused(t, b, o, th, "resolution")
}

func TestMergeHolderCloseBeatsClaim(t *testing.T) {
	b, o, th := sides(5, 6)
	o.Status, o.Resolution, o.ClosedAt = Closed, Done, sec(5)
	th.Holder, th.StartedAt = &Holder{Session: "s2", ClaimedAt: sec(6)}, sec(6)
	m, over := merged(t, b, o, th)
	if m.Status != Closed || m.Holder != nil || over != nil {
		t.Fatalf("%+v %+v", m, over)
	}
}

func TestMergeHolderAClaimBeatsARelease(t *testing.T) {
	b, o, th := sides(5, 6)
	b.Holder = &Holder{Session: "s1", ClaimedAt: sec(1)}
	o.Holder = nil                                        // ours released
	th.Holder = &Holder{Session: "s2", ClaimedAt: sec(6)} // theirs took the claim over
	m, over := merged(t, b, o, th)
	if m.Holder == nil || m.Holder.Session != "s2" || over != nil {
		t.Fatalf("%+v %+v", m.Holder, over)
	}
}

func TestMergeHolderTheLaterClaimWinsAndIsReported(t *testing.T) {
	b, o, th := sides(5, 6)
	o.Holder = &Holder{Session: "s1", Host: "a", ClaimedAt: sec(5)}
	th.Holder = &Holder{Session: "s2", Host: "b", ClaimedAt: sec(4)}
	m, over := merged(t, b, o, th)
	if m.Holder == nil || m.Holder.Session != "s1" || over == nil || over.Lost.Session != "s2" ||
		over.Kept.Session != "s1" || over.ID != b.ID {
		t.Fatalf("%+v %+v", m.Holder, over)
	}
	th.Holder.ClaimedAt = sec(5)
	refused(t, b, o, th, "holder")
}

func TestMergeStartedAtTheEarliest(t *testing.T) {
	b, o, th := sides(5, 6)
	o.StartedAt, th.StartedAt = sec(5), sec(3)
	if m, _ := merged(t, b, o, th); m.StartedAt != sec(3) {
		t.Fatalf("%v", m.StartedAt)
	}
}

func TestMergeUpdatedAtTheLatest(t *testing.T) {
	b, o, th := sides(8, 6)
	if m, _ := merged(t, b, o, th); m.UpdatedAt != sec(8) {
		t.Fatalf("%v", m.UpdatedAt)
	}
}

func TestMergeDeliveredTheMax(t *testing.T) {
	b, o, th := sides(5, 6)
	for _, it := range []*Item{&b, &o, &th} {
		it.Type, it.Need = Need, &NeedInfo{Kind: Decision, RaisedBy: &RaisedBy{Session: "s1"}, Delivered: 1}
	}
	o.Need.Delivered, th.Need.Delivered = 3, 2
	if m, _ := merged(t, b, o, th); m.Need.Delivered != 3 {
		t.Fatalf("%+v", m.Need)
	}
}

func TestMergeOtherScalarsTheLaterUpdatedAt(t *testing.T) {
	b, o, th := sides(5, 6)
	for _, it := range []*Item{&b, &o, &th} {
		it.Type, it.Need = Need, &NeedInfo{Kind: Review, Review: &ReviewInfo{PR: "u/1", Focus: "f"}}
	}
	o.Title, th.Title = "ours", "theirs"
	o.Description, th.Description = "ours", "theirs"
	o.Parent, th.Parent = "demo-abc.2", "demo-abc.3"
	o.Need.Review.Merged, th.Need.Review.Merged = "aaa", "bbb"
	o.Need.Review.Focus, th.Need.Review.Focus = "fo", "ft"
	m, _ := merged(t, b, o, th)
	if m.Title != "theirs" || m.Description != "theirs" || m.Parent != "demo-abc.3" || m.Need.Review.Merged != "bbb" ||
		m.Need.Review.Focus != "ft" || m.Need.Review.PR != "u/1" {
		t.Fatalf("%+v %+v", m, m.Need.Review)
	}
	th.UpdatedAt = sec(5) // the same second on both: no winner
	refused(t, b, o, th, "title")
}
