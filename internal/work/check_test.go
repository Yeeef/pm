package work

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

// Every invariant of the work-store page's Data model that Check enforces, one case each: a valid tree with one item
// changed fails, naming that item.

func TestCheckRefusesEachImpossibleState(t *testing.T) {
	review := func(it *Item) {
		it.Need = &NeedInfo{Kind: Review, Review: &ReviewInfo{PR: "https://x/pull/1", Sprints: []string{P + ".1"}}}
	}
	for _, c := range []struct {
		id   string
		edit func(*Item)
		want string
	}{
		{P + ".1.1", func(it *Item) { it.ID = "nope" }, "not a work-store id"},
		{P + ".1.1", func(it *Item) { it.Type = "epic" }, `type "epic"`},
		{P + ".1.1", func(it *Item) { it.Title = "" }, "no title"},
		{P + ".1.1", func(it *Item) { it.Status = "in_progress" }, `status "in_progress"`},
		{P + ".1.1", func(it *Item) { it.CreatedAt = time.Time{} }, "no created_at"},
		{P + ".1.1", func(it *Item) { it.UpdatedAt = at.In(time.FixedZone("x", 3600)) }, "not UTC"},
		{P + ".1.1", func(it *Item) { it.StartedAt = at.Add(time.Millisecond) }, "whole seconds"},
		{P + ".1.1", func(it *Item) { it.Resolution = Done }, "open but has close fields"},
		{P + ".1.1", func(it *Item) { it.ClosedBy = "s" }, "open but has close fields"},
		{P + ".1.1", func(it *Item) { closed(it); it.Resolution = "" }, "resolution"},
		{P + ".1.1", func(it *Item) { closed(it); it.ClosedAt = time.Time{} }, "no closed_at"},
		{P + ".1.3", closed, "closed but held by live"},
		{P + ".1.3", func(it *Item) { it.Holder.Session = "" }, "holder without a session"},
		{P + ".1", func(it *Item) { it.Number = 0 }, "sprint without a number"},
		{P + ".1.1", func(it *Item) { it.Number = 3 }, "task with a sprint number"},
		{P + ".1.1", func(it *Item) { it.Labels = []string{"a", "a"} }, "repeated label"},
		{P + ".1.1", func(it *Item) { it.BlockedBy = []string{P + ".1.1"} }, "blocked by itself"},
		{P + ".1.1", func(it *Item) { it.BlockedBy = []string{P + ".9"} }, "not in the store"},
		{P + ".1.1", func(it *Item) {
			it.Comments = []Comment{{ID: "c", Kind: "reply", Author: "a", CreatedAt: at}, {ID: "c", Kind: Note,
				Author: "a", CreatedAt: at}}
		}, "repeated id"},
		{P + ".1.1", func(it *Item) { it.Comments = []Comment{{ID: "c", Kind: "answer", Author: "a", CreatedAt: at}} },
			`kind "answer"`},
		{P + ".1.1", func(it *Item) { it.Need = &NeedInfo{Kind: Decision} }, "task with need fields"},
		{P + ".1.6", func(it *Item) { it.Need = nil }, "need without need fields"},
		{P + ".1.6", func(it *Item) { it.Need.Kind = "question" }, `kind "question"`},
		{P + ".1.6", func(it *Item) { it.Need.Kind = Review }, "review need without review fields"},
		{P + ".1.6", func(it *Item) { review(it); it.Need.Kind = Action }, "action need with review fields"},
		{P + ".1.6", func(it *Item) { review(it); it.Need.Review.PR = "" }, "without a PR"},
		{P + ".1.6", func(it *Item) { review(it); it.Need.Review.Sprints = []string{P + ".1", P + ".1"} },
			"repeated review sprint"},
		{P + ".1.6", func(it *Item) { it.Need.RaisedBy = &RaisedBy{} }, "raised_by"},
		{P + ".1.6", func(it *Item) { it.Need.Delivered = -1 }, "below 0"},
		{P, func(it *Item) { it.Parent = P + ".1" }, "project with parent"},
		{P + ".1", func(it *Item) { it.Parent = P + ".2" }, "parent \"" + P + ".2\" is not a project"},
		{P + ".1.1", func(it *Item) { it.Parent = P + ".1.6" }, "task under need"},
		{P + ".1.1", func(it *Item) { it.Parent = "" }, "open task with no parent"},
		{P + ".1.6", func(it *Item) { it.Parent = "" }, "open need with no parent"},
		{P + ".1.1", func(it *Item) { it.Parent = P + ".9" }, "not in the store"},
		{P + ".1.1", func(it *Item) { it.Parent = P + ".1.1.1" }, "parent cycle"},
	} {
		items := tree()
		found := false
		for i := range items {
			if items[i].ID == c.id {
				c.edit(&items[i])
				found = true
			}
		}
		if !found {
			t.Fatalf("no %s in the tree", c.id)
		}
		err := Check(items)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s, want %q: %v", c.id, c.want, err)
		}
	}
	// A closed task or need with no parent is allowed: bd let three of them in before the import refused them.
	items := tree()
	items = append(items, item("d-old", Task, "", closed), item("d-ask", Need, "", closed))
	if err := Check(items); err != nil {
		t.Error(err)
	}

	t.Run("ReadyTasksInTheirOrder", func(t *testing.T) {
		x := index(t, tree())
		var got []string
		for _, it := range x.ReadyTasks(live) {
			got = append(got, it.ID)
		}
		want := []string{P + ".1.1", P + ".1.1.1", P + ".1.4", P + ".1.7", P + ".1.10", P + ".3"}
		if !slices.Equal(got, want) {
			t.Errorf("ready:\n got %v\nwant %v", got, want)
		}
	})

	t.Run("BlockedAndItsReasons", func(t *testing.T) {
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
	})

	t.Run("ClosingTheBlockerFreesTheTasks", func(t *testing.T) {
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
	})

	t.Run("CyclesIncludeAncestors", func(t *testing.T) {
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
	})

	t.Run("CheckIDTakesThePagesFormat", func(t *testing.T) {
		for _, id := range []string{"yeeef-agents-9va", "yeeef-agents-9va.41.2", "demo-abc", "demo-abcd1234", "a_b-x9z.10"} {
			if err := CheckID(id); err != nil {
				t.Errorf("%s: %v", id, err)
			}
		}
		for _, id := range []string{"9va", "-9va", "demo-ab", "demo-abcdefghi", "demo-9VA", "demo-9va.0", "demo-9va.01",
			"demo-9va.", "demo-9va..1", "demo-9va.x", "demo-9va.-1", "de mo-9va", "my.repo-9va"} {
			if err := CheckID(id); err == nil {
				t.Errorf("%s passed", id)
			}
		}
	})

	t.Run("NewRootMintsFourBase36CharactersAndALongerOneOnAHit", func(t *testing.T) {
		random := bytes.NewReader(bytes.Repeat([]byte{0, 1, 35, 36, 71}, 20))
		first := ""
		id, err := NewRoot("demo", func(id string) bool {
			if first == "" {
				first = id
				return true
			}
			return false
		}, random)
		if err != nil {
			t.Fatal(err)
		}
		if first != "demo-01z0" || id != "demo-z01z0" { // bytes mod 36: 0 1 35 36 → 0 1 z 0; then 71 0 1 35 36
			t.Errorf("first %s, then %s", first, id)
		}
		if _, err := NewRoot("demo", func(string) bool { return true }, random); err == nil ||
			!strings.Contains(err.Error(), "taken") {
			t.Errorf("every id taken: %v", err)
		}
		if _, err := NewRoot("", func(string) bool { return false }, random); err == nil {
			t.Error("minted without a prefix")
		}
	})

	t.Run("NextChildIsOneAboveTheHighestChildNumberEverMinted", func(t *testing.T) {
		p := "demo-9va"
		ids := []string{p, p + ".1", p + ".2", p + ".10", p + ".2.5", "demo-abc.11", p + ".1.10.1"}
		if got := NextChild(ids, p); got != p+".11" {
			t.Errorf("%s", got)
		}
		// A moved-away child keeps its id, so its number counts wherever it sits now: the ids are all that count.
		if got := NextChild(append(ids, p+".12"), p); got != p+".13" {
			t.Errorf("%s", got)
		}
		// p.1's children are p.1.<n>: p.10 and p.10.1 are not among them.
		if got := NextChild(append(ids, p+".10.1"), p+".1"); got != p+".1.1" {
			t.Errorf("%s", got)
		}
		if got := NextChild(ids, p+".2"); got != p+".2.6" {
			t.Errorf("%s", got)
		}
	})

	t.Run("IDsOrderByTheirNumbers", func(t *testing.T) {
		ids := []string{"x-9va.10", "x-9va.2", "x-9va", "x-9va.2.1", "x-abc", "x-9va.1"}
		slices.SortFunc(ids, CompareIDs)
		if got := strings.Join(ids, " "); got != "x-9va x-9va.1 x-9va.2 x-9va.2.1 x-9va.10 x-abc" {
			t.Error(got)
		}
		ids = []string{"x-9va.100", "x-9va.9.10", "x-9va.39", "x-9va.b", "x-9va.9.2", "x-9va.9", "x-9va.01", "x-9va.1"}
		slices.SortFunc(ids, CompareIDs)
		if got := strings.Join(ids, " "); got != "x-9va.01 x-9va.1 x-9va.9 x-9va.9.2 x-9va.9.10 x-9va.39 x-9va.100 x-9va.b" {
			t.Error(got)
		}
	})
}
