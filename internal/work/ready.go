package work

import (
	"slices"
	"strings"
)

// Ready and blocked, as the work-store page defines them over the items of one store copy:
//
//	ancestors(i)     = parent(i), parent(parent(i)), …                       nearest first
//	open_blockers(i) = { b ∈ blocked_by(j) : j ∈ {i} ∪ ancestors(i), status(b) = open }
//	blocked(i)       = status(i) = open  ∧  open_blockers(i) ≠ ∅
//	ready(i)         = type(i) = task  ∧  status(i) = open  ∧  ¬live(holder(i))  ∧  ¬blocked(i)
//	                   ∧  every ancestor of i is open
//
// The whole computation runs in process over one read of the store; no index is kept in it.

// Index is a store copy's items by id, for the computations that walk the tree.
type Index struct {
	items []Item
	byID  map[string]*Item
}

// NewIndex indexes items; an id that appears twice is an error.
func NewIndex(items []Item) (*Index, error) {
	x := &Index{items: items, byID: make(map[string]*Item, len(items))}
	for i := range items {
		if _, dup := x.byID[items[i].ID]; dup {
			return nil, itemError(items[i].ID, "appears twice")
		}
		x.byID[items[i].ID] = &items[i]
	}
	return x, nil
}

// Item is the item with this id, or nil.
func (x *Index) Item(id string) *Item { return x.byID[id] }

// Ancestors is id's parent, its parent's parent, and so on, nearest first. It stops at a missing parent or a repeat
// (Check fails both).
func (x *Index) Ancestors(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	for it := x.byID[id]; it != nil && it.Parent != ""; it = x.byID[it.Parent] {
		if seen[it.Parent] {
			break
		}
		seen[it.Parent] = true
		out = append(out, it.Parent)
	}
	return out
}

// Blocker is one open blocker of an item, and the ancestor whose blocked_by holds it ("" when the item's own).
type Blocker struct {
	ID  string
	Via string
}

// OpenBlockers is open_blockers(id), each with where it came from: the item's own first, then each ancestor's,
// nearest first; a blocker reached twice is listed once, from the nearest.
func (x *Index) OpenBlockers(id string) []Blocker {
	var out []Blocker
	seen := map[string]bool{}
	for _, j := range append([]string{id}, x.Ancestors(id)...) {
		it := x.byID[j]
		if it == nil {
			continue
		}
		via := j
		if j == id {
			via = ""
		}
		for _, b := range it.BlockedBy {
			if bi := x.byID[b]; bi != nil && bi.Status == Open && !seen[b] {
				seen[b] = true
				out = append(out, Blocker{ID: b, Via: via})
			}
		}
	}
	return out
}

// Blocked is blocked(id): the item is open and has an open blocker, its own or an ancestor's.
func (x *Index) Blocked(id string) bool {
	it := x.byID[id]
	return it != nil && it.Status == Open && len(x.OpenBlockers(id)) > 0
}

// Ready is ready(id): an open task that no live session holds, not blocked, with every ancestor open. live says
// whether a session is live; a holder that is not live is a stale holder, and the task is ready.
func (x *Index) Ready(id string, live func(session string) bool) bool {
	it := x.byID[id]
	if it == nil || it.Type != Task || it.Status != Open {
		return false
	}
	if it.Holder != nil && live(it.Holder.Session) {
		return false
	}
	if x.Blocked(id) {
		return false
	}
	for _, a := range x.Ancestors(id) {
		if p := x.byID[a]; p == nil || p.Status != Open { // a missing ancestor: Check fails such a store
			return false
		}
	}
	return true
}

// ReadyTasks is every ready task in pm task ready's order: by the number of the sprint it sits under (its nearest
// sprint ancestor), then by id; tasks under no sprint (directly under a project) come after every sprint's tasks.
func (x *Index) ReadyTasks(live func(session string) bool) []Item {
	var out []Item
	for _, it := range x.items {
		if x.Ready(it.ID, live) {
			out = append(out, it)
		}
	}
	sprintOf := func(id string) int {
		for _, a := range x.Ancestors(id) {
			if s := x.byID[a]; s != nil && s.Type == Sprint {
				return s.Number
			}
		}
		return -1
	}
	slices.SortStableFunc(out, func(a, b Item) int {
		sa, sb := sprintOf(a.ID), sprintOf(b.ID)
		switch {
		case sa != sb && sa == -1:
			return 1
		case sa != sb && sb == -1:
			return -1
		case sa != sb:
			return sa - sb
		}
		return CompareIDs(a.ID, b.ID)
	})
	return out
}

// waits is what id waits on directly: its own blockers and every ancestor's, open or closed. A parent never waits for
// its children.
func (x *Index) waits(id string) []string {
	var out []string
	for _, j := range append([]string{id}, x.Ancestors(id)...) {
		if it := x.byID[j]; it != nil {
			out = append(out, it.BlockedBy...)
		}
	}
	return out
}

// Cycle is the ids on one blocked_by cycle, ancestors included (an item waits on its ancestors' blockers too), in
// order with the first repeated at the end; nil when there is none. So a task that blocks its own sprint is a cycle.
func (x *Index) Cycle() []string {
	const (
		unseen = iota
		onPath
		done
	)
	state := make(map[string]int, len(x.items))
	var path []string
	var visit func(id string) []string
	visit = func(id string) []string {
		state[id] = onPath
		path = append(path, id)
		for _, b := range x.waits(id) {
			if x.byID[b] == nil {
				continue
			}
			switch state[b] {
			case onPath:
				start := slices.Index(path, b)
				return append(slices.Clone(path[start:]), b)
			case unseen:
				if c := visit(b); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[id] = done
		return nil
	}
	ids := make([]string, 0, len(x.items))
	for _, it := range x.items {
		ids = append(ids, it.ID)
	}
	slices.SortFunc(ids, CompareIDs)
	for _, id := range ids {
		if state[id] == unseen {
			if c := visit(id); c != nil {
				return c
			}
		}
	}
	return nil
}

// cycleText is a cycle for an error: "a -> b -> a".
func cycleText(c []string) string { return strings.Join(c, " -> ") }
