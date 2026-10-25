package designindex

import (
	"errors"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
)

// siteURL is the site's URL as the command resolves it.
func siteURL() (string, error) { return "https://pm.example.com", nil }

// brokenURL is a site URL that cannot be resolved, as with PORT=abc.
func brokenURL() (string, error) { return "", errors.New("PORT is not a number") }

// store is the records and the items of a test.
type store struct {
	recs []*records.Record
	x    *records.Items
}

// indexRepo is a project p with sprints s1 and s2; tasks a (with sub-task a1) and b in s1, c in s2, and l and m directly
// under p. The project lists work-context and records-store; s1 lists records-store and gone, which names no design
// record; s2 lists none.
func indexRepo(t *testing.T, change func(map[string]*item.Item)) *store {
	t.Helper()
	items := []item.Item{
		{ID: "p", Type: item.Project}, {ID: "s1", Type: item.Sprint, Parent: "p"},
		{ID: "s2", Type: item.Sprint, Parent: "p"}, {ID: "a", Type: item.Task, Parent: "s1"},
		{ID: "a1", Type: item.Task, Parent: "a"}, {ID: "b", Type: item.Task, Parent: "s1"},
		{ID: "c", Type: item.Task, Parent: "s2"}, {ID: "l", Type: item.Task, Parent: "p"},
		{ID: "m", Type: item.Task, Parent: "p"},
	}
	byID := map[string]*item.Item{}
	for i := range items {
		byID[items[i].ID] = &items[i]
	}
	if change != nil {
		change(byID)
	}
	pages := func(slugs ...string) string {
		out := "## Goal\n\nG.\n\n## Design pages\n\n> Where is the detail?\n\n"
		for _, s := range slugs {
			out += "- [" + s + "](../design/" + s + ".md)\n"
		}
		if slugs == nil {
			out += "None yet.\n"
		}
		return out + "\n## Decisions\n\nNone yet.\n"
	}
	texts := map[string]string{
		"projects/demo":        "---\ntype: project\ntitle: Demo\nbead: p\n---\n\n" + pages("work-context", "records-store"),
		"sprints/demo-1":       "---\ntype: sprint\ntitle: One\nbead: s1\n---\n\n" + pages("records-store", "gone"),
		"sprints/demo-2":       "---\ntype: sprint\ntitle: Two\nbead: s2\n---\n\n" + pages(),
		"design/work-context":  "---\ntype: design\ntitle: Work context\nproject: demo\n---\n\n## Problem\n\nP.\n",
		"design/records-store": "---\ntype: design\ntitle: Records store\nproject: demo\n---\n\n## Problem\n\nP.\n",
	}
	r := &store{x: records.NewItems(items)}
	for rel, text := range texts {
		rec, err := records.Parse(rel+".md", rel, text)
		if err != nil {
			t.Fatal(err)
		}
		r.recs = append(r.recs, rec)
	}
	return r
}

func claimIndexOf(t *testing.T, r *store, id string) string {
	t.Helper()
	out, err := Claim(r.recs, r.x, id, "me", siteURL)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var projectIndex = "\ndesign pages (read the ones your task touches):\n" +
	"  work-context   Work context   https://pm.example.com/design/work-context.html\n" +
	"  records-store  Records store  https://pm.example.com/design/records-store.html\n" +
	"prior work: pm search <topic>"

func TestFirstClaimInASprintPrintsTheSprintsPagesThenTheProjectsWithoutRepeats(t *testing.T) {
	want := "\ndesign pages (read the ones your task touches):\n" +
		"  records-store  Records store  https://pm.example.com/design/records-store.html\n" +
		"  gone           missing\n" +
		"  work-context   Work context   https://pm.example.com/design/work-context.html\n" +
		"prior work: pm search <topic>"
	if got := claimIndexOf(t, indexRepo(t, nil), "a"); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// another session's claim in the sprint, and this session's claim in another sprint, leave it the first
	r := indexRepo(t, func(x map[string]*item.Item) {
		x["b"].Holder = &item.Holder{Session: "other"}
		x["c"].Holder = &item.Holder{Session: "me"}
	})
	if got := claimIndexOf(t, r, "a"); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestASecondClaimInTheSameSprintPrintsNoIndex(t *testing.T) {
	for name, change := range map[string]func(map[string]*item.Item){
		"holds another":    func(x map[string]*item.Item) { x["a"].Holder = &item.Holder{Session: "me"} },
		"closed another":   func(x map[string]*item.Item) { x["a"].Status, x["a"].ClosedBy = item.Closed, "me" },
		"holds a sub-task": func(x map[string]*item.Item) { x["a1"].Holder = &item.Holder{Session: "me"} },
	} {
		if got := claimIndexOf(t, indexRepo(t, change), "b"); got != "" {
			t.Errorf("%s: got %q, want no index", name, got)
		}
	}
	// the claimed task itself held by the session (a subagent claims again) does not count
	r := indexRepo(t, func(x map[string]*item.Item) { x["a"].Holder = &item.Holder{Session: "me"} })
	if got := claimIndexOf(t, r, "a"); !strings.HasPrefix(got, "\ndesign pages") {
		t.Errorf("re-claim of the only held task: got %q, want the index", got)
	}
}

func TestATaskDirectlyUnderAProjectGetsTheProjectsPages(t *testing.T) {
	if got := claimIndexOf(t, indexRepo(t, nil), "l"); got != projectIndex {
		t.Fatalf("got %q\nwant %q", got, projectIndex)
	}
	r := indexRepo(t, func(x map[string]*item.Item) { x["m"].Holder = &item.Holder{Session: "me"} })
	if got := claimIndexOf(t, r, "l"); got != "" {
		t.Fatalf("second claim under the project: got %q, want no index", got)
	}
	// a sprint that lists no pages still gives its project's
	if got := claimIndexOf(t, indexRepo(t, nil), "c"); got != projectIndex {
		t.Fatalf("got %q\nwant %q", got, projectIndex)
	}
}

func TestSprintOpenPrintsTheProjectsPages(t *testing.T) {
	r := indexRepo(t, nil)
	if got, err := Of(r.recs, siteURL, byBead(r.recs, "p")); err != nil || got != projectIndex {
		t.Fatalf("got %q\nwant %q", got, projectIndex)
	}
	if got, err := Of(r.recs, siteURL, byBead(r.recs, "s2")); err != nil || got != "\nprior work: pm search <topic>" {
		t.Fatalf("no pages listed: got %q, want the prior-work line alone", got)
	}
}

func TestTheSiteURLIsResolvedOnlyWhenThereIsAPageToPrint(t *testing.T) {
	second := indexRepo(t, func(x map[string]*item.Item) { x["a"].Holder = &item.Holder{Session: "me"} })
	if got := Block(func() (string, error) { return Claim(second.recs, second.x, "b", "me", brokenURL) }); got != "" {
		t.Errorf("second claim: got %q, want nothing", got)
	}
	r := indexRepo(t, nil)
	if got := Block(func() (string, error) { return Of(r.recs, brokenURL, byBead(r.recs, "s2")) }); got != "\nprior work: pm search <topic>" {
		t.Errorf("no pages listed: got %q, want the prior-work line alone", got)
	}
	want := "\ndesign pages not listed: PORT is not a number"
	if got := Block(func() (string, error) { return Claim(r.recs, r.x, "a", "me", brokenURL) }); got != want {
		t.Errorf("first claim: got %q, want %q", got, want)
	}
}
