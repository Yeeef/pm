package search

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// unit is a unit with its tokens, as Units builds it.
func unit(kind, date, address, text string) Unit {
	return Unit{Kind: kind, Date: date, State: "-", Address: address, Text: text, snips: []string{text},
		tokens: Tokens(text)}
}

// corpus is three units whose BM25 scores the tests compute by hand:
//
//	a: "alpha beta"               |a| = 2
//	b: "alpha alpha gamma delta"  |b| = 4
//	c: "beta gamma"               |c| = 2
//
// N = 3, avgdl = 8/3. alpha and beta are each in 2 units: idf = ln(1 + (3 − 2 + 0.5)/(2 + 0.5)) = ln 1.6 = 0.4700036.
// For alpha:
//
//	a: tf 1, K = 1.2·(0.25 + 0.75·2/(8/3)) = 0.975: 0.4700036 · 1·2.2/(1 + 0.975)  = 0.4700036 · 1.1139241 = 0.5235484
//	b: tf 2, K = 1.2·(0.25 + 0.75·4/(8/3)) = 1.65:  0.4700036 · 2·2.2/(2 + 1.65)   = 0.4700036 · 1.2054795 = 0.5665797
//
// For alpha beta only a holds both: 0.5235484 + 0.5235484 (beta in a is alpha's case again) = 1.0470968.
func corpus() []Unit {
	return []Unit{
		unit("doc", "2026-10-01", "docs/a", "alpha beta"),
		unit("sprint", "2026-10-02", "s-1 Goal", "alpha alpha gamma delta"),
		unit("doc", "2026-10-03", "docs/c", "beta gamma"),
	}
}

func scores(hits []Hit) map[string]float64 {
	out := map[string]float64{}
	for _, h := range hits {
		out[h.Address] = h.Score
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBM25ScoresAndOrderMatchTheHandComputedValues(t *testing.T) {
	hits := Rank(corpus(), Query{Terms: []string{"alpha"}, N: 10})
	if len(hits) != 2 || hits[0].Address != "s-1 Goal" || hits[1].Address != "docs/a" {
		t.Fatalf("alpha: got %v, want s-1 Goal then docs/a", scores(hits))
	}
	if !near(hits[0].Score, 0.5665797) || !near(hits[1].Score, 0.5235484) {
		t.Errorf("alpha: got %v, want 0.5665797 and 0.5235484", scores(hits))
	}
	hits = Rank(corpus(), Query{Terms: []string{"Alpha", "betas"}, N: 10})
	if len(hits) != 1 || hits[0].Address != "docs/a" || !near(hits[0].Score, 1.0470968) {
		t.Errorf("alpha beta: got %v, want docs/a alone at 1.0470968: a unit holds every word", scores(hits))
	}
	if hits := Rank(corpus(), Query{Terms: []string{"alpha", "epsilon"}, N: 10}); hits != nil {
		t.Errorf("a word no unit holds: got %v, want no hit", scores(hits))
	}
}

func TestFiltersKeepUnitsWithoutChangingTheirScores(t *testing.T) {
	all := scores(Rank(corpus(), Query{Terms: []string{"gamma"}, N: 10}))
	docs := Rank(corpus(), Query{Terms: []string{"gamma"}, Kinds: []string{"doc"}, N: 10})
	if len(docs) != 1 || docs[0].Address != "docs/c" || docs[0].Score != all["docs/c"] {
		t.Errorf("--kind doc: got %v, want docs/c alone at its unfiltered score %v", scores(docs), all["docs/c"])
	}
	units := corpus()
	units[0].Project, units[1].Project = "demo", "demo"
	demo := Rank(units, Query{Terms: []string{"alpha"}, Project: "demo", N: 10})
	other := Rank(units, Query{Terms: []string{"beta"}, Project: "other", N: 10})
	if len(demo) != 2 || !near(demo[0].Score, 0.5665797) || other != nil {
		t.Errorf("--project: got %v and %v, want both alpha units at their scores, and nothing for other",
			scores(demo), scores(other))
	}
}

func TestEqualScoresGoNewestFirstThenByAddress(t *testing.T) {
	units := []Unit{
		unit("doc", "2026-10-01", "docs/old", "same words"),
		unit("doc", "2026-10-05", "docs/b", "same words"),
		unit("doc", "2026-10-05", "docs/a", "same words"),
	}
	var got []string
	for _, h := range Rank(units, Query{Terms: []string{"same"}, N: 10}) {
		got = append(got, h.Address)
	}
	if want := []string{"docs/a", "docs/b", "docs/old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAPhraseOrAPathMustAppearAsWritten(t *testing.T) {
	units := []Unit{
		unit("doc", "-", "docs/path", "It moved `internal/cli/task.go` here."),
		unit("doc", "-", "docs/words", "The task list in internal tools, for cli users; go on."),
		unit("doc", "-", "docs/wrapped", "Each design\npage holds one area."),
		unit("doc", "-", "docs/apart", "A page about design."),
	}
	for terms, want := range map[string][]string{
		"internal/cli/task.go": {"docs/path"},
		"design page":          {"docs/wrapped"},
		"design|page":          {"docs/apart", "docs/wrapped"}, // two terms: words anywhere
	} {
		var got []string
		for _, h := range Rank(units, Query{Terms: strings.Split(terms, "|"), N: 10}) {
			got = append(got, h.Address)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", terms, got, want)
		}
	}
}

func TestOutputHasTheCountTheCappedAlignedLinesAndTheReadHint(t *testing.T) {
	units := []Unit{
		unit("decision", "2026-09-30", "pm-harness-112 Decisions", "Claims keep only the holder. Claim history is not kept."),
		unit("task", "2026-09-12", "pm-9.88.2", "Record every claim in an events table"),
		unit("doc", "2026-09-01", "docs/2026-09-01-x", "One claim here, in a much longer unit about other things."),
	}
	units[0].State, units[1].State = "done", "dropped"
	q := Query{Terms: []string{"claim"}, N: 2}
	want := "3 units hold all of: claim\n" +
		"decision  2026-09-30  done     pm-harness-112 Decisions  Claims keep only the holder.\n" +
		"task      2026-09-12  dropped  pm-9.88.2                 Record every claim in an events table\n" +
		"read one: pm show --record <record> --section <section>; an item: pm show <id>"
	if got := Format(q, Rank(units, q)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	q = Query{Terms: []string{"events"}, N: 10}
	if got := Format(q, Rank(units, q)); !strings.HasPrefix(got, "1 unit holds all of: events\ntask  ") {
		t.Errorf("one hit: got\n%s", got)
	}
	q = Query{Terms: []string{"claim", "nowhere"}, N: 10}
	if got, want := Format(q, Rank(units, q)), "no unit holds all of: claim, nowhere; try other words"; got != want {
		t.Errorf("no hit: got %q, want %q", got, want)
	}
}

func TestCheckRefusesAQueryWithNoWordAndAnNBelowOne(t *testing.T) {
	for _, q := range []Query{{N: 10}, {Terms: []string{"--", "./"}, N: 10}, {Terms: []string{"a"}, N: 0}} {
		if Check(q) == nil {
			t.Errorf("%+v: want a refusal", q)
		}
	}
	if err := Check(Query{Terms: []string{"a"}, N: 1}); err != nil {
		t.Errorf("want none, got %v", err)
	}
}

// demoStore is the records and items of a project demo (item p) with one closed sprint s1, a dropped task and an open
// need under it, a design page, a doc and a postmortem.
func demoStore(t *testing.T) ([]*records.Record, []item.Item, map[string]store.Dates) {
	t.Helper()
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	items := []item.Item{
		{ID: "p", Type: item.Project, Title: "Demo", Status: item.Open, CreatedAt: day(1)},
		{ID: "s1", Type: item.Sprint, Parent: "p", Status: item.Closed, CreatedAt: day(2), ClosedAt: day(5)},
		{ID: "s1.1", Type: item.Task, Parent: "s1", Title: "Lexer by hand", Description: "Write the lexer.",
			Status: item.Closed, Resolution: item.Dismissed, CloseReason: "A library lexes it.", CreatedAt: day(2),
			ClosedAt: day(4)},
		{ID: "s1.2", Type: item.Need, Parent: "s1", Title: "Pick a lexer", Status: item.Open, CreatedAt: day(3)},
		{ID: "q", Type: item.Project, Title: "Other", Status: item.Open, CreatedAt: day(1)},
	}
	texts := map[string]string{
		"projects/demo": "---\ntype: project\ntitle: Demo\nbead: p\n---\n\n## Goal\n\n> What standing goal?\n\nA lexer.\n\n" +
			"## Progress\n\nlexer progress\n\n## Decisions\n\n> What did we choose?\n\n" +
			"::: decision {source=owner date=2026-10-02}\nUse a lexer library.\nIt is maintained.\n:::\n\n" +
			"## Design pages\n\nNone yet.\n",
		"projects/other": "---\ntype: project\ntitle: Other\nbead: q\n---\n\n## Goal\n\nNothing.\n",
		"sprints/demo-1": "---\ntype: sprint\ntitle: Lexer\nbead: s1\n---\n\n## Goal\n\n> What should be true, prompted?\n\n" +
			"The lexer runs.\n\n## Findings\n\nNone yet.\n\n## Delivery report\n\n> Written at close.\n\n### Outcome\n\n" +
			"Not closed yet.\n\n### Against \"Done when\"\n\nNot closed yet.\n",
		"design/lexer": "---\ntype: design\ntitle: The lexer\nproject: demo\n---\n\n## Problem\n\nLexing.\n\n" +
			"## Design\n\nThe lexer in short.\n\n### Tokens\n\nA lexer token.\n\n#### Detail\n\nMore lexer.\n\n" +
			"### Errors\n\nThe lexer fails hard.\n\n## Open questions\n\nNone yet.\n",
		"docs/2026-10-03-notes": "---\ntype: doc\ntitle: Lexer notes\ndate: 2026-10-03\nbead: s1\n---\n\nThe lexer is fine.\n",
		"postmortems/2026-10-04-outage": "---\ntype: postmortem\ntitle: Lexer outage\ndate: 2026-10-04\nsprint: s1\n---\n\n" +
			"## Summary\n\nThe lexer broke.\n",
	}
	var recs []*records.Record
	for rel, text := range texts {
		rec, err := records.Parse(rel+".md", rel, text)
		if err != nil {
			t.Fatal(err)
		}
		recs = append(recs, rec)
	}
	return recs, items, map[string]store.Dates{"design/lexer": {Created: "2026-10-01", Updated: "2026-10-06"}}
}

// The units of the pm-search design page's table: ## sections but Progress and Decisions, each decision block,
// ### under a design page's Design (its lead keeping the heading Design), a doc's whole body, tasks and needs; prompt
// lines and placeholder bodies dropped; dates and states as the page's table gives them.
func TestUnitsFollowTheDesignPagesTable(t *testing.T) {
	recs, items, dates := demoStore(t)
	units := Units(recs, items, dates)
	var got []string
	for _, u := range units {
		got = append(got, strings.Join([]string{u.Kind, u.Date, u.State, u.Address, u.Project}, " | "))
	}
	want := []string{
		"decision | 2026-10-02 | running | demo Decisions | demo",
		"design | 2026-10-06 | - | lexer Design | demo",
		"design | 2026-10-06 | - | lexer Errors | demo",
		"design | 2026-10-06 | - | lexer Problem | demo",
		"design | 2026-10-06 | - | lexer Tokens | demo",
		"doc | 2026-10-03 | - | docs/2026-10-03-notes | demo",
		"need | 2026-10-03 | ready | s1.2 | demo",
		"postmortem | 2026-10-04 | - | postmortems/2026-10-04-outage Summary | demo",
		"project | 2026-10-01 | ready | other Goal | other",
		"project | 2026-10-01 | running | demo Goal | demo",
		"sprint | 2026-10-05 | done | demo-1 Goal | demo",
		"task | 2026-10-04 | dropped | s1.1 | demo",
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, u := range units {
		if strings.Contains(u.Text, "prompted") || strings.Contains(u.Text, "progress") {
			t.Errorf("%s indexes a prompt line or Progress: %q", u.Address, u.Text)
		}
		if u.Address == "lexer Tokens" && !strings.Contains(u.Text, "More lexer.") {
			t.Errorf("a #### stays in its ### unit: %q", u.Text)
		}
		if u.Kind == "decision" && u.Text != "Demo\nDecisions\nUse a lexer library.\nIt is maintained." {
			t.Errorf("decision text: %q", u.Text)
		}
	}
}

func TestRunRefusesAnUnknownProjectAndTakesAProjectsId(t *testing.T) {
	recs, items, dates := demoStore(t)
	_, err := Run(Query{Terms: []string{"lexer"}, Project: "nope", N: 10}, recs, items, dates)
	if err == nil || err.Error() != `no project "nope"; projects: demo, other` {
		t.Errorf("got %v", err)
	}
	byName, err := Run(Query{Terms: []string{"lexer"}, Project: "demo", N: 3}, recs, items, dates)
	if err != nil {
		t.Fatal(err)
	}
	byID, err := Run(Query{Terms: []string{"lexer"}, Project: "p", N: 3}, recs, items, dates)
	if err != nil || byID != byName || !strings.HasPrefix(byName, "11 units hold all of: lexer\n") {
		t.Errorf("by name:\n%s\nby id:\n%s %v", byName, byID, err)
	}
	if lines := strings.Split(byName, "\n"); len(lines) != 5 {
		t.Errorf("-n 3: got %d lines, want the header, 3 hits and the hint:\n%s", len(lines), byName)
	}
}

// An address pm show --record would refuse is not printed: a design page named as a project is given by its path, and
// a heading the record has twice by the record alone.
func TestAnAddressNamesWhatPmShowResolves(t *testing.T) {
	texts := map[string]string{
		"projects/demo": "---\ntype: project\ntitle: Demo\nbead: p\n---\n\n## Goal\n\nThe lexer.\n",
		"design/demo":   "---\ntype: design\ntitle: Demo design\nproject: demo\n---\n\n## Problem\n\nThe lexer.\n",
		"docs/2026-10-03-twice": "---\ntype: doc\ntitle: Twice\ndate: 2026-10-03\nproject: demo\n---\n\n" +
			"## Scope\n\nThe lexer.\n\n## Other\n\n### Scope\n\nMore.\n",
	}
	var recs []*records.Record
	for rel, text := range texts {
		rec, err := records.Parse(rel+".md", rel, text)
		if err != nil {
			t.Fatal(err)
		}
		recs = append(recs, rec)
	}
	var got []string
	for _, u := range Units(recs, []item.Item{{ID: "p", Type: item.Project}}, nil) {
		got = append(got, u.Address)
	}
	sort.Strings(got)
	want := []string{"design/demo Problem", "docs/2026-10-03-twice", "docs/2026-10-03-twice Other", "projects/demo Goal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
