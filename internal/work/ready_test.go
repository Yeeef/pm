package work

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// Ready, blocked and cycles, from the work-store page's "Ready and blocked" definitions:
//
//	open_blockers(i) = { b ∈ blocked_by(j) : j ∈ {i} ∪ ancestors(i), status(b) = open }
//	blocked(i)       = status(i) = open  ∧  open_blockers(i) ≠ ∅
//	ready(i)         = type(i) = task ∧ status(i) = open ∧ ¬live(holder(i)) ∧ ¬blocked(i) ∧ every ancestor open

var at = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// item is a valid item for the tree tests: open unless closed is given.
func item(id string, typ Type, parent string, opts ...func(*Item)) Item {
	it := Item{ID: id, Type: typ, Parent: parent, Title: id, Status: Open, CreatedAt: at, UpdatedAt: at}
	if typ == Need {
		it.Need = &NeedInfo{Kind: Decision}
	}
	for _, o := range opts {
		o(&it)
	}
	return it
}

func closed(it *Item)                     { it.Status, it.Resolution, it.ClosedAt = Closed, Done, at }
func number(n int) func(*Item)            { return func(it *Item) { it.Number = n } }
func blockedBy(ids ...string) func(*Item) { return func(it *Item) { it.BlockedBy = ids } }
func heldBy(s string) func(*Item) {
	return func(it *Item) { it.Holder = &Holder{Session: s, Host: "mac", ClaimedAt: at} }
}

const P = "d-pro"

// tree: two sprints of one project, the second blocked by the first, with tasks of every kind the rules name.
func tree() []Item {
	return []Item{
		item(P, Project, ""),
		item(P+".1", Sprint, P, number(1)),
		item(P+".2", Sprint, P, number(2), blockedBy(P+".1")),
		item(P+".4", Sprint, P, number(4), closed),
		item(P+".1.1", Task, P+".1"),                                   // ready
		item(P+".1.1.1", Task, P+".1.1"),                               // ready: a child is not blocked by its open parent
		item(P+".1.2", Task, P+".1", blockedBy(P+".1.1")),              // blocked by its own blocker
		item(P+".1.3", Task, P+".1", heldBy("live")),                   // held by a live session
		item(P+".1.4", Task, P+".1", heldBy("gone")),                   // a stale holder: ready
		item(P+".1.5", Task, P+".1", closed),                           // closed
		item(P+".1.6", Need, P+".1"),                                   // a need is never ready
		item(P+".1.7", Task, P+".1", blockedBy(P+".1.5")),              // its blocker is closed: ready
		item(P+".2.1", Task, P+".2"),                                   // blocked through its sprint
		item(P+".3", Task, P),                                          // under the project: ready, ordered last
		item(P+".4.1", Task, P+".4"),                                   // under a closed sprint: not ready
		item(P+".1.10", Task, P+".1"),                                  // ready; orders after .1.7
		item(P+".1.2.1", Task, P+".1.2"),                               // blocked through its parent task
		item(P+".1.8", Task, P+".1", blockedBy(P+".2")),                // blocked by an open sprint
		item(P+".1.9", Task, P+".1", blockedBy(P+".1.6"), heldBy("x")), // a need blocks it; a stale holder
	}
}

func live(s string) bool { return s == "live" }

func index(t *testing.T, items []Item) *Index {
	t.Helper()
	x, err := NewIndex(items)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestReadyTasksInTheirOrder(t *testing.T) {
	x := index(t, tree())
	var got []string
	for _, it := range x.ReadyTasks(live) {
		got = append(got, it.ID)
	}
	want := []string{P + ".1.1", P + ".1.1.1", P + ".1.4", P + ".1.7", P + ".1.10", P + ".3"}
	if !slices.Equal(got, want) {
		t.Errorf("ready:\n got %v\nwant %v", got, want)
	}
}

func TestBlockedAndItsReasons(t *testing.T) {
	x := index(t, tree())
	for id, want := range map[string][]Blocker{
		P + ".1.2":   {{P + ".1.1", ""}},
		P + ".1.2.1": {{P + ".1.1", P + ".1.2"}},
		P + ".2.1":   {{P + ".1", P + ".2"}},
		P + ".2":     {{P + ".1", ""}},
		P + ".1.8":   {{P + ".2", ""}},
		P + ".1.9":   {{P + ".1.6", ""}},
		P + ".1.7":   nil, // its blocker is closed
		P + ".1.1.1": nil, // a parent never blocks its child
		P + ".1":     nil, // and a child never blocks its parent
	} {
		got := x.OpenBlockers(id)
		if !slices.Equal(got, want) {
			t.Errorf("%s: open blockers %v, want %v", id, got, want)
		}
		if x.Blocked(id) != (len(want) > 0) {
			t.Errorf("%s: blocked %v", id, x.Blocked(id))
		}
	}
	// A closed item is not blocked, whatever its blockers.
	items := tree()
	closed(&items[slices.IndexFunc(items, func(i Item) bool { return i.ID == P+".1.2" })])
	if index(t, items).Blocked(P + ".1.2") {
		t.Error("a closed item is blocked")
	}
}

func TestClosingTheBlockerFreesTheTasks(t *testing.T) {
	items := tree()
	for i := range items {
		if items[i].ID == P+".1" || items[i].ID == P+".1.1" {
			closed(&items[i])
		}
	}
	x := index(t, items)
	if !x.Ready(P+".2.1", live) {
		t.Error("the sprint blocking .2 closed, yet .2.1 is not ready")
	}
	if x.Ready(P+".1.2", live) {
		t.Error(".1.2's blocker closed, but its sprint is closed: not ready")
	}
}

func TestCyclesIncludeAncestors(t *testing.T) {
	for _, c := range []struct {
		name  string
		items []Item
		cycle string
	}{
		{"none", tree(), ""},
		{"two tasks", []Item{item(P, Project, ""), item(P+".1", Task, P, blockedBy(P+".2")),
			item(P+".2", Task, P, blockedBy(P+".1"))}, P + ".1 -> " + P + ".2 -> " + P + ".1"},
		{"a task blocks its own sprint", []Item{item(P, Project, ""), item(P+".1", Sprint, P, number(1),
			blockedBy(P+".1.1")), item(P+".1.1", Task, P+".1")}, P + ".1.1 -> " + P + ".1.1"}, // .1.1 waits on .1's blockers: itself
		{"through a sprint", []Item{item(P, Project, ""), item(P+".1", Sprint, P, number(1), blockedBy(P+".2.1")),
			item(P+".2", Sprint, P, number(2)), item(P+".2.1", Task, P+".2", blockedBy(P+".1.1")),
			item(P+".1.1", Task, P+".1")}, P + ".2.1 -> " + P + ".1.1 -> " + P + ".2.1"},
		{"closed items too", []Item{item(P, Project, ""), item(P+".1", Task, P, closed, blockedBy(P+".2")),
			item(P+".2", Task, P, closed, blockedBy(P+".1"))}, P + ".1 -> " + P + ".2 -> " + P + ".1"},
		// A task blocked by its own ancestor is no cycle: a parent never waits for its children.
		{"a task blocked by its sprint", []Item{item(P, Project, ""), item(P+".1", Sprint, P, number(1)),
			item(P+".1.1", Task, P+".1", blockedBy(P+".1"))}, ""},
	} {
		got := strings.Join(index(t, c.items).Cycle(), " -> ")
		if got != c.cycle {
			t.Errorf("%s: cycle %q, want %q", c.name, got, c.cycle)
		}
		err := Check(c.items)
		if (err != nil) != (c.cycle != "") || err != nil && !strings.Contains(err.Error(), "blocked_by cycle") {
			t.Errorf("%s: Check: %v", c.name, err)
		}
	}
}
