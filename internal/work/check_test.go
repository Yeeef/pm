package work

import (
	"strings"
	"testing"
	"time"
)

// Every invariant of the work-store page's Data model that Check enforces, one case each: a valid tree with one item
// changed fails, naming that item.

func TestTheTreeIsValid(t *testing.T) {
	if err := Check(tree()); err != nil {
		t.Fatal(err)
	}
}

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
}
