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

// unit is a unit with its tokens, as Units builds it, a document of its own (an item's when kind is task or need).
func unit(kind, date, address, text string) Unit {
	return Unit{Kind: kind, Date: date, State: "-", Address: address, Text: text, snips: []string{text},
		tokens: Tokens(text), doc: address}
}

// corpus is three records of one unit each, whose BM25 scores the tests compute by hand:
//
//	a: "alpha beta"               |a| = 2
//	b: "alpha alpha gamma delta"  |b| = 4
//	c: "beta gamma"               |c| = 2
//
// N = 3, avgdl = 8/3. alpha and beta are each in 2 records: idf = ln(1 + (3 − 2 + 0.5)/(2 + 0.5)) = ln 1.6 = 0.4700036.
// For alpha:
//
//	a: tf 1, K = 1.2·(0.25 + 0.75·2/(8/3)) = 0.975: 0.4700036 · 1·2.2/(1 + 0.975)  = 0.4700036 · 1.1139241 = 0.5235484
//	b: tf 2, K = 1.2·(0.25 + 0.75·4/(8/3)) = 1.65:  0.4700036 · 2·2.2/(2 + 1.65)   = 0.4700036 · 1.2054795 = 0.5665797
//
// For alpha beta: a holds both, 0.5235484 + 0.5235484 (beta in a is alpha's case again) = 1.0470968; b holds alpha,
// 0.5665797; c holds beta, 0.5235484 (c's beta is a's alpha again).
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

func addresses(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Address)
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
	want := []string{"docs/a", "s-1 Goal", "docs/c"}
	if !reflect.DeepEqual(addresses(hits), want) || !near(hits[0].Score, 1.0470968) ||
		!near(hits[1].Score, 0.5665797) || !near(hits[2].Score, 0.5235484) {
		t.Fatalf("alpha beta: got %v, want %v at 1.0470968, 0.5665797, 0.5235484: any word matches", scores(hits), want)
	}
	if !hits[0].All || hits[1].All || hits[2].All {
		t.Errorf("alpha beta: only docs/a holds every word: got %v %v %v", hits[0].All, hits[1].All, hits[2].All)
	}
	if hits := Rank(corpus(), Query{Terms: []string{"epsilon", "zeta"}, N: 10}); hits != nil {
		t.Errorf("words no record holds: got %v, want no hit", scores(hits))
	}
}

// A record is one document, so it holds every word even when they are in different sections, and its line points at
// its best section. Records r (units "alpha gamma" and "beta beta") and s ("alpha delta"):
//
//	documents: |r| = 4, |s| = 2, N = 2, avgdl = 3; idf alpha = ln(1 + 0.5/2.5) = 0.1823216, beta = ln(1 + 1.5/1.5) = 0.6931472
//	r: alpha tf 1, K = 1.2·(0.25 + 0.75·4/3) = 1.5: 0.1823216 · 2.2/2.5 = 0.1604430;
//	   beta tf 2: 0.6931472 · 4.4/3.5 = 0.8713850; together 1.0318280
//	s: alpha tf 1, K = 1.2·(0.25 + 0.75·2/3) = 0.9: 0.1823216 · 2.2/1.9 = 0.2111092
//	units: N = 3, avgdl = 2; alpha's idf ln 1.6 = 0.4700036, beta's ln(1 + 2.5/1.5) = 0.9808293: "alpha gamma" scores
//	0.4700036, "beta beta" 0.9808293 · 4.4/3.2 = 1.3486403, so r's line is r Beta
func TestARecordIsOneDocumentShownAtItsBestUnit(t *testing.T) {
	units := []Unit{
		unit("sprint", "2026-10-01", "r Alpha", "alpha gamma"),
		unit("sprint", "2026-10-01", "r Beta", "beta beta"),
		unit("doc", "2026-10-02", "docs/s", "alpha delta"),
	}
	units[0].doc, units[1].doc = "sprints/r", "sprints/r"
	hits := Rank(units, Query{Terms: []string{"alpha", "beta"}, N: 10})
	if want := []string{"r Beta", "docs/s"}; !reflect.DeepEqual(addresses(hits), want) {
		t.Fatalf("got %v, want %v", addresses(hits), want)
	}
	if !near(hits[0].Score, 1.0318280) || !near(hits[1].Score, 0.2111092) || !hits[0].All || hits[1].All {
		t.Errorf("got %v, all %v %v; want r at 1.0318280 holding both, s at 0.2111092", scores(hits), hits[0].All,
			hits[1].All)
	}
	q := Query{Terms: []string{"alpha", "beta"}, N: 10}
	if got := strings.SplitN(Format(q, hits), "\n", 2)[0]; got != "1 record holds all of: alpha, beta; 2 hold some" {
		t.Errorf("header: got %q", got)
	}
}

// Records and items are each normalized by their own mean length, so a short item does not outrank a record for being
// short. Records R (40 tokens, word twice) and F (40, none), items I (4 tokens, word once) and G (4, none):
//
//	N = 4, df 2: idf = ln(1 + 2.5/2.5) = ln 2 = 0.6931472
//	R: K = 1.2·(0.25 + 0.75·40/40) = 1.2: 0.6931472 · 4.4/3.2 = 0.9530774
//	I: K = 1.2·(0.25 + 0.75·4/4) = 1.2:   0.6931472 · 2.2/2.2 = 0.6931472
//
// Against one mean of 22, I would score 0.6931472 · 2.2/1.4636 = 1.0418 and R 0.6931472 · 4.4/3.9364 = 0.7748.
func TestRecordsAndItemsEachKeepTheirOwnMeanLength(t *testing.T) {
	units := []Unit{
		unit("doc", "-", "docs/r", "word word"+strings.Repeat(" x", 38)),
		unit("doc", "-", "docs/f", strings.TrimSpace(strings.Repeat("y ", 40))),
		unit("task", "-", "p.1", "word ab cd ef"),
		unit("task", "-", "p.2", "gh ij kl mn"),
	}
	hits := Rank(units, Query{Terms: []string{"word"}, N: 10})
	if want := []string{"docs/r", "p.1"}; !reflect.DeepEqual(addresses(hits), want) ||
		!near(hits[0].Score, 0.9530774) || !near(hits[1].Score, 0.6931472) {
		t.Errorf("got %v, want docs/r at 0.9530774 then p.1 at 0.6931472", scores(hits))
	}
}

func TestFiltersKeepRecordsWithoutChangingTheirScores(t *testing.T) {
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
		t.Errorf("--project: got %v and %v, want both alpha records at their scores, and nothing for other",
			scores(demo), scores(other))
	}
	// --kind decision shows a record's decision that holds a word, though its Goal scores higher, and drops a record
	// whose decisions hold none.
	units = []Unit{
		unit("sprint", "-", "s-1 Goal", "lexer lexer lexer"),
		unit("decision", "2026-10-02", "s-1 Decisions", "Use a lexer library."),
		unit("sprint", "-", "s-2 Goal", "lexer"),
		unit("decision", "2026-10-03", "s-2 Decisions", "Use a parser."),
	}
	units[0].doc, units[1].doc, units[2].doc, units[3].doc = "s-1", "s-1", "s-2", "s-2"
	if got := addresses(Rank(units, Query{Terms: []string{"lexer"}, Kinds: []string{"decision"}, N: 10})); !reflect.DeepEqual(
		got, []string{"s-1 Decisions"}) {
		t.Errorf("--kind decision: got %v, want s-1 Decisions alone", got)
	}
}

func TestEqualScoresGoNewestFirstThenByAddress(t *testing.T) {
	units := []Unit{
		unit("doc", "2026-10-01", "docs/old", "same words"),
		unit("doc", "2026-10-05", "docs/b", "same words"),
		unit("doc", "2026-10-05", "docs/a", "same words"),
	}
	if got, want := addresses(Rank(units, Query{Terms: []string{"same"}, N: 10})), []string{"docs/a", "docs/b",
		"docs/old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAPhraseOrAPathMustAppearAsWritten(t *testing.T) {
	units := []Unit{
		unit("doc", "-", "docs/path", "It moved `internal/cli/task.go` here."),
		unit("doc", "-", "docs/words", "The task list in internal tools, for cli users; go on."),
		unit("doc", "-", "docs/wrapped", "Each design\npage holds one area."),
		unit("doc", "-", "docs/apart", "A page about design."),
		unit("doc", "-", "docs/none", "Nothing here."),
	}
	for terms, want := range map[string][]string{
		"internal/cli/task.go": {"docs/path"},
		"design page":          {"docs/wrapped"},
		"design|page":          {"docs/apart", "docs/wrapped"}, // two terms: words anywhere, any of them
	} {
		got := addresses(Rank(units, Query{Terms: strings.Split(terms, "|"), N: 10}))
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", terms, got, want)
		}
	}
}

func TestOutputHasTheCountsTheCappedAlignedLinesAndTheReadHint(t *testing.T) {
	units := []Unit{
		unit("decision", "2026-09-30", "pm-harness-112 Decisions", "Claims keep only the holder. Claim history is not kept."),
		unit("task", "2026-09-12", "pm-9.88.2", "Record every claim in an events table"),
		unit("doc", "2026-09-01", "docs/2026-09-01-x", "One claim here, in a much longer unit about other things."),
	}
	units[0].State, units[1].State = "done", "dropped"
	q := Query{Terms: []string{"claim"}, N: 2}
	want := "3 records hold all of: claim; 3 hold some\n" +
		"decision  2026-09-30  done     pm-harness-112 Decisions  Claims keep only the holder.\n" +
		"task      2026-09-12  dropped  pm-9.88.2                 Record every claim in an events table\n" +
		"read one: pm show --record <record> --section <section>; an item: pm show <id>"
	if got := Format(q, Rank(units, q)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	q = Query{Terms: []string{"events"}, N: 10}
	if got := Format(q, Rank(units, q)); !strings.HasPrefix(got, "1 record holds all of: events; 1 holds some\ntask  ") {
		t.Errorf("one hit: got\n%s", got)
	}
	q = Query{Terms: []string{"claim", "nowhere"}, N: 10}
	if got := Format(q, Rank(units, q)); !strings.HasPrefix(got, "0 records hold all of: claim, nowhere; 3 hold some\n") {
		t.Errorf("some words: got\n%s", got)
	}
	q = Query{Terms: []string{"nowhere", "never"}, N: 10}
	if got, want := Format(q, Rank(units, q)), "no record or item holds any of: nowhere, never; try other words"; got != want {
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
		if u.Kind == "decision" && u.Text != "Demo\nUse a lexer library.\nIt is maintained." {
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
	if err != nil || byID != byName || !strings.HasPrefix(byName, "7 records hold all of: lexer; 7 hold some\n") {
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

// texts is each unit's address and indexed text, for a record set given as rel → text.
func texts(t *testing.T, recs map[string]string, items ...item.Item) map[string]string {
	t.Helper()
	var parsed []*records.Record
	for rel, text := range recs {
		rec, err := records.Parse(rel+".md", rel, text)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, rec)
	}
	out := map[string]string{}
	for _, u := range Units(parsed, items, nil) {
		out[u.Address] = u.Text
	}
	return out
}

// A decision block indexes its record's title and its body, not the heading Decisions its address names; a sprint's
// Design pages, links to pages indexed themselves, is no unit.
func TestADecisionIndexesItsTitleAndBodyAndDesignPagesIsNoUnit(t *testing.T) {
	got := texts(t, map[string]string{"sprints/demo-1": "---\ntype: sprint\ntitle: One\nbead: s\n---\n\n" +
		"## Design pages\n\n- [Lexer](../design/lexer.md)\n\n## Decisions\n\n" +
		"::: decision {source=agent date=2026-10-02}\nUse a library.\n:::\n"}, item.Item{ID: "s", Type: item.Sprint})
	want := map[string]string{"demo-1 Decisions": "One\nUse a library."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A # heading starts a section as a ## does, so its text is indexed under it; a setext heading's underline is part of
// the heading, not of its body.
func TestALevelOneHeadingStartsASectionAndASetextUnderlineIsNoBody(t *testing.T) {
	got := texts(t, map[string]string{"docs/2026-10-03-notes": "---\ntype: doc\ntitle: Notes\ndate: 2026-10-03\n" +
		"project: demo\n---\n\n## Goal\n\nAlpha.\n\n# Appendix\n\nZeta.\n\nSetext\n------\n\nOmega.\n"})
	want := map[string]string{
		"docs/2026-10-03-notes Goal":     "Notes\nGoal\nAlpha.",
		"docs/2026-10-03-notes Appendix": "Notes\nAppendix\nZeta.",
		"docs/2026-10-03-notes Setext":   "Notes\nSetext\nOmega.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
