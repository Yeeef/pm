package work

import (
	"encoding/json"
	"testing"
	"time"
)

// The JSON field names are the work-store page's (Data model); pm export prints this shape.
func TestItemJSONUsesThePagesFieldNames(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	review := Item{ID: "demo-9va.84.4", Type: Need, Parent: "demo-9va.84", Title: "Review PR #80", Status: Closed,
		Resolution: Done, CloseReason: "merged as abc123", Labels: []string{"[TEST]"}, CreatedAt: at, UpdatedAt: at,
		ClosedAt: at, ClosedBy: "s2",
		Comments: []Comment{{ID: "c1", Kind: Reply, Author: "owner", Text: "ok", CreatedAt: at}},
		Need: &NeedInfo{Kind: Review, RaisedBy: &RaisedBy{Session: "s1", Inbox: "/tmp/s1.sock", Host: "mac"}, Delivered: 1,
			Review: &ReviewInfo{PR: "https://github.com/o/r/pull/80", Sprints: []string{"demo-9va.84"},
				Designs: []string{"pm-go"}, Focus: "the parser", Merged: "abc123", MergeReported: "abc123"}}}
	task := Item{ID: "demo-9va.84.1", Type: Task, Parent: "demo-9va.84", Title: "Do it", Status: Open,
		BlockedBy: []string{"demo-9va.83"}, Holder: &Holder{Session: "s1", Host: "mac", ClaimedAt: at}, StartedAt: at,
		CreatedAt: at, UpdatedAt: at}
	sprint := Item{ID: "demo-9va.86", Type: Sprint, Parent: "demo-9va", Title: "Sprint 77: Spike", Status: Open,
		Number: 77, CreatedAt: at, UpdatedAt: at}
	for _, c := range []struct {
		item Item
		want string
	}{
		{review, `{"id":"demo-9va.84.4","type":"need","parent":"demo-9va.84","title":"Review PR #80","status":"closed",` +
			`"resolution":"done","close_reason":"merged as abc123","labels":["[TEST]"],"created_at":"2026-10-01T12:00:00Z",` +
			`"updated_at":"2026-10-01T12:00:00Z","closed_at":"2026-10-01T12:00:00Z","closed_by":"s2",` +
			`"comments":[{"id":"c1","kind":"reply","author":"owner","text":"ok","created_at":"2026-10-01T12:00:00Z"}],` +
			`"need":{"kind":"review","raised_by":{"session":"s1","inbox":"/tmp/s1.sock","host":"mac"},"delivered":1,` +
			`"review":{"pr":"https://github.com/o/r/pull/80","sprints":["demo-9va.84"],"designs":["pm-go"],` +
			`"focus":"the parser","merged":"abc123","merge_reported":"abc123"}}}`},
		{task, `{"id":"demo-9va.84.1","type":"task","parent":"demo-9va.84","title":"Do it","status":"open",` +
			`"blocked_by":["demo-9va.83"],"holder":{"session":"s1","host":"mac","claimed_at":"2026-10-01T12:00:00Z"},` +
			`"started_at":"2026-10-01T12:00:00Z","created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`},
		{sprint, `{"id":"demo-9va.86","type":"sprint","parent":"demo-9va","title":"Sprint 77: Spike","status":"open",` +
			`"number":77,"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`},
	} {
		got, err := json.Marshal(c.item)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.item.ID, got, c.want)
		}
		var back Item
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatal(err)
		}
		if again, _ := json.Marshal(back); string(again) != c.want {
			t.Errorf("%s does not round-trip: %s", c.item.ID, again)
		}
	}
}
