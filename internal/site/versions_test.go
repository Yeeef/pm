package site

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// servedWith is the constructs' published site, as the pm service serves it, with drafts beside it, and its golden
// pages.
func servedWith(t *testing.T, drafts ...store.Draft) (*Served, map[string]string) {
	t.Helper()
	dir := constructsStore(t, nil)
	items := constructsItems(t)
	texts, sums, err := store.Published(dir)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := records.ParseAll(dir, texts)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := records.ParseSummaries(sums)
	if err != nil {
		t.Fatal(err)
	}
	dates, err := store.DesignDates(dir, recs)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CodeChanges(dir, "origin/main", recs, dates); err != nil {
		t.Fatal(err)
	}
	s, err := Serve(recs, items, "demo", dates, summaries)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddDrafts(dir, texts, drafts); err != nil {
		t.Fatal(err)
	}
	pages, err := renderStore(dir, items)
	if err != nil {
		t.Fatal(err)
	}
	return s, pages
}

func edited(t *testing.T, dir, rel string, from, to string) *string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), from) {
		t.Fatalf("%s holds no %q", rel, from)
	}
	text := strings.Replace(string(b), from, to, 1)
	return &text
}

func get(t *testing.T, s *Served, path string) string {
	t.Helper()
	text, found, err := s.Page(path)
	if err != nil || !found {
		t.Fatalf("%s: found %v, %v", path, found, err)
	}
	return text
}

var diffLine = regexp.MustCompile(`<span class="(add|del)">([+-]) (.*)</span>`)

// changedLines is each line a diff page marks added or removed, as "+ line" or "- line".
func changedLines(page string) []string {
	var out []string
	for _, m := range diffLine.FindAllStringSubmatch(page, -1) {
		out = append(out, m[2]+" "+m[3])
	}
	return out
}

// A draft's edit to a design page leaves the published page as it was, plus the version bar; the version renders the
// published set with the draft laid over it, its links staying in the version; its diff against published shows
// exactly the edited lines, in the one section they are in.
func TestAVersionIsThePublishedRecordsWithItsDraftLaidOver(t *testing.T) {
	inZone(t)
	dir := constructsStore(t, nil)
	draft := store.Draft{Name: "store", Label: "uncommitted edits in the shared store", State: "uncommitted edits",
		Files: map[string]*string{"design/parser.md": edited(t, dir, "design/parser.md", "Records need a parser.",
			"Records need a faster parser.\nIt streams.")}}
	s, golden := servedWith(t, draft)

	page := get(t, s, "design/parser.html")
	bar := regexp.MustCompile(`(?s)<div class="note draft versions">.*?</div>\n`).FindString(page)
	if bar == "" || strings.Replace(page, bar, "", 1) != golden["design/parser.html"] {
		t.Fatalf("the published page is not its golden page plus the version bar:\n%s", Diff(golden["design/parser.html"], page))
	}
	for _, want := range []string{`<b>published</b>`, `<a href="../drafts/store/design/parser.html">store</a>`,
		"uncommitted edits in the shared store; uncommitted edits", `<a href="../drafts/store/design/parser.diff.html">`} {
		if !strings.Contains(bar, want) {
			t.Errorf("the version bar lacks %q:\n%s", want, bar)
		}
	}
	if other := get(t, s, "design/empty.html"); other != golden["design/empty.html"] {
		t.Errorf("a page no draft changes is not its golden page:\n%s", Diff(golden["design/empty.html"], other))
	}
	if index := get(t, s, "index.html"); !strings.Contains(index, `<a href="drafts/store/design/parser.diff.html">design/parser</a>`) {
		t.Errorf("the overview does not list the draft's change:\n%s", index)
	}

	version := get(t, s, "drafts/store/design/parser.html")
	for _, want := range []string{"Records need a faster parser.", `<a href="../index.html">Home</a>`,
		`href="../sprints/demo-1.html"`, `<a href="../../../design/parser.html">published</a>`, `<b>store</b>`} {
		if !strings.Contains(version, want) {
			t.Errorf("the version lacks %q", want)
		}
	}
	if strings.Contains(version, "Records need a parser.") || strings.Contains(page, "faster") {
		t.Error("a version and the published page mix their texts")
	}
	if sprint := get(t, s, "drafts/store/sprints/demo-1.html"); !strings.Contains(sprint, "<b>store</b>") {
		t.Errorf("a page of the version that the draft does not change names no version:\n%s", sprint)
	}

	diff := get(t, s, "drafts/store/design/parser.diff.html")
	if got, want := changedLines(diff), []string{"- Records need a parser. See [sprint 1](../sprints/demo-1.md), its [goal](../sprints/demo-1.md#goal), the",
		"+ Records need a faster parser.", "+ It streams. See [sprint 1](../sprints/demo-1.md), its [goal](../sprints/demo-1.md#goal), the"}; !slices.Equal(got, want) {
		t.Errorf("the diff marks %q, want %q", got, want)
	}
	if !strings.Contains(diff, `<h3 class="diffhead">## Problem <span class="k">changed</span></h3>`) ||
		!strings.Contains(diff, `<details class="same"><summary>## Design <span class="k">unchanged</span>`) ||
		!strings.Contains(diff, "1 section changed") {
		t.Errorf("the diff is not by section:\n%s", diff)
	}
	for _, missing := range []string{"drafts/other/design/parser.html", "drafts/store/design/nope.html",
		"drafts/store/design/nope.diff.html", "drafts/store/design/parser.html?vs=store", "design/parser.html?vs=store"} {
		if _, found, err := s.Page(missing); found || err != nil {
			t.Errorf("%s: found %v, %v; want no such page", missing, found, err)
		}
	}
}

// Two drafts of one page: each version's diff against the other shows the lines where they differ.
func TestADraftsDiffAgainstAnotherDraftShowsWhereTheyDiffer(t *testing.T) {
	inZone(t)
	dir := constructsStore(t, nil)
	a := store.Draft{Name: "a", Label: "draft a", State: "uncommitted edits", Files: map[string]*string{
		"design/empty.md": edited(t, dir, "design/empty.md", "## Problem\n\n> What are we solving, and why now?\n\nNone yet.",
			"## Problem\n\n> What are we solving, and why now?\n\nSpeed.")}}
	b := store.Draft{Name: "b", Label: "draft b", State: "uncommitted edits", Files: map[string]*string{
		"design/empty.md": edited(t, dir, "design/empty.md", "## Open questions\n\n> What is still unresolved?\n\nNone yet.",
			"## Open questions\n\n> What is still unresolved?\n\nWhich cache.")}}
	s, _ := servedWith(t, a, b)
	diff := get(t, s, "drafts/a/design/empty.diff.html?vs=b")
	if got, want := changedLines(diff), []string{"- None yet.", "+ Speed.", "- Which cache.", "+ None yet."}; !slices.Equal(got, want) {
		t.Errorf("a against b marks %q, want %q", got, want)
	}
	if !strings.Contains(diff, "<span>against draft b</span>") || !strings.Contains(diff, "2 sections changed") {
		t.Errorf("the diff does not say what it is against:\n%s", diff)
	}
	version := get(t, s, "drafts/a/design/empty.html")
	if !strings.Contains(version, `<a href="../../../drafts/b/design/empty.diff.html?vs=a">diff against a</a>`) {
		t.Errorf("version a's bar does not link b's diff against it")
	}
	if _, found, err := s.Page("drafts/a/design/empty.diff.html?vs=c"); found || err != nil {
		t.Errorf("a diff against no draft: found %v, %v", found, err)
	}
}

// A draft that does not parse fails its own version's pages only: its diff still renders, and every published page,
// and every page of another version, renders as before.
func TestABrokenDraftFailsOnlyItsOwnVersion(t *testing.T) {
	inZone(t)
	dir := constructsStore(t, nil)
	broken := store.Draft{Name: "broken", Label: "a broken draft", State: "uncommitted edits", Files: map[string]*string{
		"design/parser.md": edited(t, dir, "design/parser.md", "type: design\n", "type: design\n  bad: [\n")}}
	good := store.Draft{Name: "good", Label: "a good draft", State: "uncommitted edits", Files: map[string]*string{
		"docs/2026-10-05-new.md": new("---\ntype: doc\ntitle: New\ndate: 2026-10-05\nproject: demo\n---\n\nA new doc.\n")}}
	s, golden := servedWith(t, broken, good)
	for _, path := range []string{"drafts/broken/design/parser.html", "drafts/broken/index.html"} {
		if _, found, err := s.Page(path); !found || err == nil || !strings.Contains(err.Error(), "the version broken does not render") {
			t.Errorf("%s: found %v, %v; want the version's error", path, found, err)
		}
	}
	if diff := get(t, s, "drafts/broken/design/parser.diff.html"); !slices.Equal(changedLines(diff), []string{"+   bad: ["}) {
		t.Errorf("the broken draft's diff marks %q", changedLines(diff))
	}
	for path, want := range golden {
		page := get(t, s, path)
		if path != "design/parser.html" && path != "index.html" && page != want {
			t.Errorf("%s changed:\n%s", path, Diff(want, page))
		}
	}
	if page := get(t, s, "design/parser.html"); !strings.Contains(page, "a broken draft; uncommitted edits; does not render") {
		t.Errorf("the bar does not say the broken version does not render:\n%s", page)
	}
	if doc := get(t, s, "drafts/good/docs/2026-10-05-new.html"); !strings.Contains(doc, "A new doc.") ||
		!strings.Contains(doc, "published<span class=\"k\">not published</span>") {
		t.Errorf("the good version's new doc:\n%s", doc)
	}
	if _, found, err := s.Page("docs/2026-10-05-new.html"); found || err != nil {
		t.Errorf("a draft's new record is published: found %v, %v", found, err)
	}
}

func TestSectionsCutAtHeadingsOutsideCodeFences(t *testing.T) {
	text := "---\ntype: design\n---\n\n## A\n\n```\n## not a heading\n```\n\n### A.1\n\nx\n\n## A\n\ny\n"
	var heads []string
	for _, s := range sections(text) {
		heads = append(heads, s.head)
	}
	if want := []string{"", "## A", "### A.1", "## A"}; !slices.Equal(heads, want) {
		t.Errorf("heads %q, want %q", heads, want)
	}
	if got := keys(sections(text)); got[1] == got[3] {
		t.Errorf("a repeated heading has one key: %q", got)
	}
	if diff := sectionDiff(text, text); !strings.Contains(diff, "0 sections changed") || strings.Contains(diff, "class=\"add\"") {
		t.Errorf("a text against itself:\n%s", diff)
	}
	if got := changedLines(sectionDiff("", "## A\n\nx\n")); !slices.Equal(got, []string{"+ ## A", "+ ", "+ x"}) {
		t.Errorf("a new record marks %q", got)
	}
}
