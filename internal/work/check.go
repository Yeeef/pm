package work

import (
	"fmt"
	"slices"
	"time"
)

// The invariants pm checks in code on every write and on every load (the work-store page, Storage: Read), on top of
// what the schema enforces. A failure names the item and fails hard: the store holds no state these rules forbid.

// itemError is an invariant failure on one item.
func itemError(id, format string, a ...any) error {
	return fmt.Errorf("work store: %s %s", id, fmt.Sprintf(format, a...))
}

// Check checks every item's own fields, then the tree, the references and the blocked_by cycles across them.
func Check(items []Item) error {
	for i := range items {
		if err := checkItem(&items[i]); err != nil {
			return err
		}
	}
	x, err := NewIndex(items)
	if err != nil {
		return err
	}
	for i := range items {
		if err := x.checkTree(&items[i]); err != nil {
			return err
		}
	}
	if c := x.Cycle(); c != nil {
		return fmt.Errorf("work store: a blocked_by cycle, ancestors included: %s", cycleText(c))
	}
	return nil
}

// checkStamp is nil for a set UTC timestamp in whole seconds, the store's form (YYYY-MM-DDTHH:MM:SSZ).
func checkStamp(id, field string, t time.Time) error {
	if t.IsZero() {
		return itemError(id, "has no %s", field)
	}
	if t.Location() != time.UTC || t.Nanosecond() != 0 {
		return itemError(id, "has %s %s, not UTC in whole seconds", field, t.Format(time.RFC3339Nano))
	}
	return nil
}

// checkItem checks the rules one item holds alone: its id, enums, the type's fields present and no other type's,
// the close fields exactly when closed, no holder on a closed item.
func checkItem(it *Item) error {
	id := it.ID
	if err := CheckID(id); err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	switch it.Type {
	case Project, Sprint, Task, Need:
	default:
		return itemError(id, "has type %q, not project, sprint, task or need", it.Type)
	}
	if it.Title == "" {
		return itemError(id, "has no title")
	}
	for _, s := range []struct {
		name string
		t    time.Time
	}{{"created_at", it.CreatedAt}, {"updated_at", it.UpdatedAt}} {
		if err := checkStamp(id, s.name, s.t); err != nil {
			return err
		}
	}
	if !it.StartedAt.IsZero() {
		if err := checkStamp(id, "started_at", it.StartedAt); err != nil {
			return err
		}
	}
	switch it.Status {
	case Open:
		if it.Resolution != "" || it.CloseReason != "" || !it.ClosedAt.IsZero() || it.ClosedBy != "" {
			return itemError(id, "is open but has close fields (resolution, close_reason, closed_at or closed_by)")
		}
	case Closed:
		switch it.Resolution {
		case Done, Answered, NoDecision, Dismissed:
		default:
			return itemError(id, "is closed with resolution %q, not done, answered, no-decision or dismissed",
				it.Resolution)
		}
		if err := checkStamp(id, "closed_at", it.ClosedAt); err != nil {
			return err
		}
		if it.Holder != nil {
			return itemError(id, "is closed but held by %s", it.Holder.Session)
		}
	default:
		return itemError(id, "has status %q, not open or closed", it.Status)
	}
	if (it.Type == Sprint) != (it.Number > 0) {
		if it.Type == Sprint {
			return itemError(id, "is a sprint without a number")
		}
		return itemError(id, "is a %s with a sprint number", it.Type)
	}
	if it.Holder != nil {
		if it.Holder.Session == "" {
			return itemError(id, "has a holder without a session")
		}
		if err := checkStamp(id, "holder claimed_at", it.Holder.ClaimedAt); err != nil {
			return err
		}
	}
	for i, l := range it.Labels {
		if l == "" || slices.Contains(it.Labels[:i], l) {
			return itemError(id, "has an empty or repeated label %q", l)
		}
	}
	for i, b := range it.BlockedBy {
		if b == id || slices.Contains(it.BlockedBy[:i], b) {
			return itemError(id, "is blocked by itself or twice by %s", b)
		}
	}
	for i, c := range it.Comments {
		if c.ID == "" || slices.ContainsFunc(it.Comments[:i], func(o Comment) bool { return o.ID == c.ID }) {
			return itemError(id, "has a comment with an empty or repeated id %q", c.ID)
		}
		if c.Kind != Reply && c.Kind != Note {
			return itemError(id, "has comment %s of kind %q, not reply or note", c.ID, c.Kind)
		}
		if c.Author == "" {
			return itemError(id, "has comment %s without an author", c.ID)
		}
		if err := checkStamp(id, "comment "+c.ID+" created_at", c.CreatedAt); err != nil {
			return err
		}
	}
	return checkNeed(it)
}

// checkNeed checks the need fields: present exactly on a need, review data exactly on a review.
func checkNeed(it *Item) error {
	id, n := it.ID, it.Need
	if (it.Type == Need) != (n != nil) {
		if n == nil {
			return itemError(id, "is a need without need fields")
		}
		return itemError(id, "is a %s with need fields", it.Type)
	}
	if n == nil {
		return nil
	}
	switch n.Kind {
	case Decision, Action, Review:
	default:
		return itemError(id, "is a need of kind %q, not decision, action or review", n.Kind)
	}
	if (n.Kind == Review) != (n.Review != nil) {
		if n.Review == nil {
			return itemError(id, "is a review need without review fields")
		}
		return itemError(id, "is a %s need with review fields", n.Kind)
	}
	if n.Review != nil && n.Review.PR == "" {
		return itemError(id, "is a review need without a PR")
	}
	if n.Review != nil {
		for _, list := range [][]string{n.Review.Sprints, n.Review.Designs} {
			for i, v := range list {
				if v == "" || slices.Contains(list[:i], v) {
					return itemError(id, "has an empty or repeated review sprint or design %q", v)
				}
			}
		}
	}
	if n.RaisedBy != nil && n.RaisedBy.Session == "" {
		return itemError(id, "is raised by no session but has raised_by")
	}
	if n.Delivered < 0 {
		return itemError(id, "has delivered %d, below 0", n.Delivered)
	}
	return nil
}

// checkTree checks an item's place: the parent exists and has the type the item's type allows, the ancestors repeat
// nothing, and every blocker exists.
func (x *Index) checkTree(it *Item) error {
	id := it.ID
	var parent *Item
	if it.Parent != "" {
		if parent = x.byID[it.Parent]; parent == nil {
			return itemError(id, "has parent %s, which is not in the store", it.Parent)
		}
	}
	switch it.Type {
	case Project:
		if parent != nil {
			return itemError(id, "is a project with parent %s; a project has none", it.Parent)
		}
	case Sprint:
		if parent == nil || parent.Type != Project {
			return itemError(id, "is a sprint whose parent %q is not a project", it.Parent)
		}
	case Task:
		if parent != nil && parent.Type != Sprint && parent.Type != Task && parent.Type != Project {
			return itemError(id, "is a task under %s %s; a task sits under a sprint, a task or a project",
				parent.Type, parent.ID)
		}
	}
	if parent == nil && (it.Type == Task || it.Type == Need) && it.Status == Open {
		return itemError(id, "is an open %s with no parent", it.Type)
	}
	seen := map[string]bool{id: true}
	for p := parent; p != nil; p = x.byID[p.Parent] {
		if seen[p.ID] {
			return itemError(id, "has a parent cycle through %s", p.ID)
		}
		seen[p.ID] = true
	}
	for _, b := range it.BlockedBy {
		if x.byID[b] == nil {
			return itemError(id, "is blocked by %s, which is not in the store", b)
		}
	}
	return nil
}
