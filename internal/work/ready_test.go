package work

import (
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

func closed(it *Item) { it.Status, it.Resolution, it.ClosedAt = Closed, Done, at }

// askOf is an ask of two questions: a single choice of two options, and a multiple choice of three.
func askOf() *AskInfo {
	return &AskInfo{Call: "toolu_01", Questions: []AskQuestion{
		{Text: "Ship it today?", Header: "Ship", Options: []AskOption{{Label: "Yes", Detail: "merge now"},
			{Label: "No", Detail: "wait for CI"}}},
		{Text: "Which checks?", Header: "Checks", Multi: true, Options: []AskOption{{Label: "lint"},
			{Label: "go", Detail: "the Go tests"}, {Label: "py", Detail: "the harness"}}},
	}}
}

// question makes a need a question a session raised.
func question(it *Item) {
	it.Need = &NeedInfo{Kind: Question, RaisedBy: &RaisedBy{Session: "s-ask"}, Ask: askOf()}
}
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
