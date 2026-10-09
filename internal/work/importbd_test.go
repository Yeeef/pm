package work

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// The import from bd, by the work-store page's "Field mapping" table: testdata/bd-export.jsonl has one issue per row,
// and each expected item below is written from the table, not from the importer.

func fixtureItems(t *testing.T) map[string]Item {
	t.Helper()
	recs := must(ReadBDRecords("testdata/records"))
	items := must(FromBD(bytes.NewReader(must(os.ReadFile("testdata/bd-export.jsonl"))), recs))
	out := map[string]Item{}
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

func TestImportMapsEachRowOfTheTable(t *testing.T) {
	got := fixtureItems(t)
	t1 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC)
	const P = "demo-p1x"
	base := func(id string, typ Type, parent, title string) Item {
		return Item{ID: id, Type: typ, Parent: parent, Title: title, Status: Open, CreatedAt: t1, UpdatedAt: t2}
	}
	shut := func(it Item, reason string, r Resolution, by string) Item {
		it.Status, it.CloseReason, it.Resolution, it.ClosedAt, it.ClosedBy = Closed, reason, r, t2, by
		return it
	}
	want := []Item{
		func() Item { it := base(P, Project, "", "Demo project"); it.Description = "The goal."; return it }(),
		// a sprint: number from its title, confirmed by its record demo-1
		func() Item {
			it := shut(base(P+".1", Sprint, P, "Sprint 1: First"), "done", Done, "")
			it.Number = 1
			return it
		}(),
		// closed: claimed_by becomes closed_by; comment authors kept; metadata probe dropped
		func() Item {
			it := shut(base(P+".1.1", Task, P+".1", "Closed task"), "shipped", Done, "s-old")
			it.Comments = []Comment{{ID: "c-0001", Kind: Note, Author: "Yeeef", Text: "a note", CreatedAt: t2}}
			return it
		}(),
		func() Item {
			it := base(P+".2", Sprint, P, "Sprint 2: Second")
			it.Number, it.BlockedBy = 2, []string{P + ".1"}
			return it
		}(),
		// in_progress: open with a holder of unknown host; notes under a Notes heading; the site's reply author
		func() Item {
			it := base(P+".2.1", Task, P+".2", "Held task")
			it.Description, it.Labels, it.StartedAt = "Do it.\n\n## Notes\n\nMind X.", []string{"x"}, t1
			it.Holder = &Holder{Session: "s-1", ClaimedAt: t1}
			it.Comments = []Comment{{ID: "c-0002", Kind: Reply, Author: "owner", Text: "go ahead", CreatedAt: t2},
				{ID: "c-0003", Kind: Note, Author: "yeeef", Text: "on it", CreatedAt: t2}}
			return it
		}(),
		func() Item {
			it := base(P+".2.1.1", Task, P+".2.1", "Sub-task")
			it.Description = "## Notes\n\nOnly notes."
			return it
		}(),
		// label human: a need; decision without action or review; raised_by and delivered from the metadata
		func() Item {
			it := base(P+".2.2", Need, P+".2", "Decision?")
			it.Need = &NeedInfo{Kind: Decision, Delivered: 2,
				RaisedBy: &RaisedBy{Session: "s-1", Inbox: "/tmp/s-1.sock", Host: "mac"}}
			return it
		}(),
		// label action: an action need; Responded…: answered
		func() Item {
			it := shut(base(P+".2.3", Need, P+".2", "Run this"), "Responded: done it", Answered, "")
			it.Need = &NeedInfo{Kind: Action, RaisedBy: &RaisedBy{Session: "s-2"}}
			return it
		}(),
		// metadata.review: a review need, over the action label; external_ref equals the PR
		func() Item {
			it := shut(base(P+".2.4", Need, P+".2", "Review PR #9"), "merged as abc123", Done, "")
			it.Need = &NeedInfo{Kind: Review, Delivered: 1, RaisedBy: &RaisedBy{Session: "s-3"},
				Review: &ReviewInfo{PR: "https://github.com/o/r/pull/9", Sprints: []string{P + ".2", P + ".1"},
					Designs: []string{"pm-go"}, Focus: "the parser", Merged: "abc123", MergeReported: "abc123"}}
			return it
		}(),
		// label no-decision wins over Responded…
		func() Item {
			it := shut(base(P+".2.5", Need, P+".2", "Which?"), "Responded: neither", NoDecision, "")
			it.Need = &NeedInfo{Kind: Decision}
			return it
		}(),
		// Dismissed…: dismissed; other labels kept
		func() Item {
			it := shut(base(P+".2.6", Need, P+".2", "[TEST] probe"), "Dismissed: [TEST]", Dismissed, "")
			it.Labels, it.Need = []string{"[TEST]"}, &NeedInfo{Kind: Decision}
			return it
		}(),
		// a bug: a task labelled bug; under a project is allowed
		func() Item {
			it := base(P+".3", Task, P, "A bug under the project")
			it.Labels, it.BlockedBy = []string{"bug"}, []string{P + ".2.1"}
			return it
		}(),
		// closed with no parent: allowed
		func() Item {
			it := shut(base("demo-zzz", Task, "", "A closed bug with no parent"), "fixed", Done, "s-2")
			it.Labels = []string{"bug"}
			return it
		}(),
	}
	if len(got) != len(want) {
		t.Fatalf("%d items, want %d", len(got), len(want))
	}
	for _, w := range want {
		g, ok := got[w.ID]
		if !ok {
			t.Errorf("%s not imported", w.ID)
			continue
		}
		gj, wj := string(must(json.Marshal(g))), string(must(json.Marshal(w)))
		if gj != wj {
			t.Errorf("%s:\n got %s\nwant %s", w.ID, gj, wj)
		}
	}
}

func TestImportFailsHardOnWhatItCannotMap(t *testing.T) {
	raw := string(must(os.ReadFile("testdata/bd-export.jsonl")))
	recs := must(ReadBDRecords("testdata/records"))
	// edit applies f to the issue with this id, as JSON.
	edit := func(id string, f func(m map[string]any)) string {
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
			var m map[string]any
			must(0, json.Unmarshal([]byte(line), &m))
			if m["id"] == id {
				f(m)
			}
			out = append(out, string(must(json.Marshal(m))))
		}
		return strings.Join(out, "\n")
	}
	meta := func(m map[string]any) map[string]any {
		if m["metadata"] == nil {
			m["metadata"] = map[string]any{}
		}
		return m["metadata"].(map[string]any)
	}
	deps := func(m map[string]any) []any { d, _ := m["dependencies"].([]any); return d }
	const P = "demo-p1x"
	for _, c := range []struct {
		name, input, want string
	}{
		{"an unknown key", edit(P+".2.1", func(m map[string]any) { m["defer_until"] = "x" }), `unknown field "defer_until"`},
		{"an unknown metadata key", edit(P+".2.1", func(m map[string]any) { meta(m)["lease"] = 1 }), `unknown field "lease"`},
		{"an unknown review key", edit(P+".2.4", func(m map[string]any) { meta(m)["review"].(map[string]any)["who"] = 1 }),
			`unknown field "who"`},
		{"an unknown type", edit(P+".2.1", func(m map[string]any) { m["issue_type"] = "chore" }), `issue_type "chore"`},
		{"an unknown status", edit(P+".3", func(m map[string]any) { m["status"] = "blocked" }), `status "blocked"`},
		{"an unknown dependency type", edit(P+".3", func(m map[string]any) {
			deps(m)[1].(map[string]any)["type"] = "related"
		}), `dependency type "related"`},
		{"another record kind", edit(P+".3", func(m map[string]any) { m["_type"] = "memory" }), `_type "memory"`},
		{"a dangling parent", edit(P+".3", func(m map[string]any) { deps(m)[0].(map[string]any)["depends_on_id"] = P + ".9" }),
			"not in the store"},
		{"a dangling blocker", edit(P+".3", func(m map[string]any) { deps(m)[1].(map[string]any)["depends_on_id"] = P + ".9" }),
			"not in the store"},
		{"an open task with no parent", edit(P+".3", func(m map[string]any) { m["dependencies"] = deps(m)[1:] }),
			"open task with no parent"},
		{"a blocked_by cycle", edit(P+".2.1", func(m map[string]any) {
			m["dependencies"] = append(deps(m), map[string]any{"issue_id": P + ".2.1", "depends_on_id": P + ".3",
				"type": "blocks", "created_at": "2026-10-01T12:00:00Z", "created_by": "yeeef", "metadata": "{}"})
		}), "blocked_by cycle"},
		{"a non-UTC timestamp", edit(P+".3", func(m map[string]any) { m["updated_at"] = "2026-10-02T11:30:00+02:00" }),
			"not a UTC timestamp"},
		{"a fractional timestamp", edit(P+".3", func(m map[string]any) { m["created_at"] = "2026-10-01T12:00:00.5Z" }),
			"not a UTC timestamp"},
		{"a sprint title without its number", edit(P+".2", func(m map[string]any) { m["title"] = "Second" }),
			`does not start "Sprint N: "`},
		{"a sprint whose record has another number", edit(P+".2", func(m map[string]any) { m["title"] = "Sprint 3: S" }),
			"its record is demo-2"},
		{"a sprint with no record", strings.TrimSpace(raw) + "\n" +
			`{"_type":"issue","id":"demo-p1x.5","title":"Sprint 5: X","status":"open","issue_type":"epic",` +
			`"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z","dependencies":[{"issue_id":` +
			`"demo-p1x.5","depends_on_id":"demo-p1x","type":"parent-child"}]}`, "no sprint record"},
		{"an epic under a sprint", strings.TrimSpace(raw) + "\n" +
			`{"_type":"issue","id":"demo-p1x.2.9","title":"Sprint 9: X","status":"open","issue_type":"epic",` +
			`"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z","dependencies":[{"issue_id":` +
			`"demo-p1x.2.9","depends_on_id":"demo-p1x.2","type":"parent-child"}]}`, "which is no project"},
		{"external_ref not the review's PR", edit(P+".2.4", func(m map[string]any) { m["external_ref"] = "https://x/9" }),
			"is not the review's PR"},
		{"review data on a decision", edit(P+".2.2", func(m map[string]any) { meta(m)["merged"] = "abc" }),
			"review data on a decision need"},
		{"need metadata on a task", edit(P+".3", func(m map[string]any) { meta(m)["session"] = "s" }),
			"need metadata or external_ref on a task"},
		{"in_progress without a claim", edit(P+".2.1", func(m map[string]any) { m["metadata"] = nil }),
			"in_progress without claimed_by"},
		{"open but claimed", edit(P+".3", func(m map[string]any) {
			meta(m)["claimed_by"], meta(m)["claimed_at"] = "s", "2026-10-01T12:00:00Z"
		}), "open but claimed_by"},
		{"open with close fields", edit(P+".3", func(m map[string]any) { m["close_reason"] = "x" }), "close fields"},
		{"a partial export", edit(P+".2.1", func(m map[string]any) { m["comment_count"] = 3 }), "a partial export"},
		{"a design field", edit(P+".3", func(m map[string]any) { m["design"] = "x" }), "design or acceptance_criteria"},
		{"human on an epic", edit(P+".2", func(m map[string]any) { m["labels"] = []string{"human"} }), "label human"},
		{"a negative picked_up", edit(P+".2.2", func(m map[string]any) { meta(m)["picked_up"] = -1 }), "picked_up -1"},
		{"an inbox without its session", edit(P+".2.2", func(m map[string]any) { delete(meta(m), "session") }),
			"inbox without the session"},
		{"an id twice", raw + edit(P+".3", func(map[string]any) {}), "appears twice"},
	} {
		_, err := FromBD(strings.NewReader(c.input), recs)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.name, c.want, err)
		}
	}
	if _, err := FromBD(strings.NewReader(raw), recs); err != nil {
		t.Fatalf("the fixture itself: %v", err)
	}
}
