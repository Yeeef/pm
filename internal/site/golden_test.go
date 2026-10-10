package site

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The site on testdata/constructs, a fixture for each construct a page renders: its records, committed on a fixed
// date so the design pages' dates are fixed, and its work-store items (items.json). Every page must equal its frozen
// copy under testdata/constructs/pages; after an intended change to the site, rewrite them with
//
//	go test -tags gms_pure_go ./internal/site -run Golden -update
//
// and review the diff of the pages in the PR.

var update = flag.Bool("update", false, "rewrite testdata/constructs/pages from the pages rendered now")

const constructs = "testdata/constructs"

// zone is the local time zone the pages render in, far from UTC so that a UTC date where a local one belongs (day
// pages, summaries) shows.
var zone = time.FixedZone("UTC+13", 13*3600)

func inZone(t *testing.T) {
	saved := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = saved })
}

// constructsStore is a records store holding the constructs' records, as edited: a path to new text, or to "" to
// remove the file; committed on branch records on 2026-10-05.
func constructsStore(t *testing.T, edits map[string]string) string {
	dir := filepath.Join(t.TempDir(), "records")
	src := filepath.Join(constructs, "records")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(dir, rel), string(b))
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel, text := range edits {
		path := filepath.Join(dir, rel)
		if text == "" {
			err = os.Remove(path)
		} else {
			err = writeFile(path, text)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE=2026-10-05T12:00:00+00:00", "GIT_COMMITTER_DATE=2026-10-05T12:00:00+00:00")
	for _, args := range [][]string{{"init", "-q", "-b", "records"}, {"add", "-A"}, {"commit", "-qm", "records"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// constructsItems is the constructs' items, plus extra ones as JSON.
func constructsItems(t *testing.T, extra ...string) *records.Items {
	b, err := os.ReadFile(filepath.Join(constructs, "items.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []work.Item
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	for _, e := range extra {
		var it work.Item
		if err := json.Unmarshal([]byte(e), &it); err != nil {
			t.Fatal(err)
		}
		items = append(items, it)
	}
	return records.NewItems(items)
}

// renderStore renders the store as pm's service does: the working store, design dates from git, the day summaries.
func renderStore(dir string, items *records.Items) (map[string]string, error) {
	recs, err := records.Read(dir, nil)
	if err != nil {
		return nil, err
	}
	dates, err := store.DesignDates(dir, recs, store.Today())
	if err != nil {
		return nil, err
	}
	summaries, err := records.ReadSummaries(dir)
	if err != nil {
		return nil, err
	}
	return RenderPages(recs, items, "demo", dates, summaries)
}

func TestPagesEqualTheGoldenPages(t *testing.T) {
	inZone(t)
	pages, err := renderStore(constructsStore(t, nil), constructsItems(t))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join(constructs, "pages")
	if *update {
		if err := os.RemoveAll(golden); err != nil {
			t.Fatal(err)
		}
		for p, text := range pages {
			if err := writeFile(filepath.Join(golden, p), text); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	want := map[string]string{}
	err = filepath.WalkDir(golden, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(golden, p)
		want[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range union(want, pages) {
		w, inWant := want[p]
		g, inGot := pages[p]
		switch {
		case !inWant || !inGot:
			t.Errorf("page %s: golden %v, rendered %v", p, inWant, inGot)
		case g != w:
			t.Errorf("page %s differs from its golden copy (- golden, + rendered):\n%s", p, Diff(w, g))
		}
	}
	t.Logf("%d pages compared", len(pages))
}

// pm check on the constructs with one thing broken: the error it gives.
func TestCheckRefusesEachBrokenRecord(t *testing.T) {
	inZone(t)
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(constructs, "records", rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	sprint, design := read("sprints/demo-1.md"), read("design/parser.md")
	uncited := `{"id": "demo.1.9", "type": "need", "title": "Pick a parser", "description": "", "status": "closed", ` +
		`"resolution": "answered", "close_reason": "Responded: a", "number": null, "parent": "demo.1", ` +
		`"blocked_by": [], "labels": [], "holder": null, "started_at": null, "created_at": "2026-10-01T12:00:00Z", ` +
		`"updated_at": null, "closed_at": "2026-10-01T13:00:00Z", "closed_by": null, "comments": [], ` +
		`"need": {"kind": "decision", "raised_by": null, "delivered": 0, "review": null}}`
	cases := []struct {
		name  string
		edits map[string]string
		extra []string
		want  string
	}{
		{"no-header", map[string]string{"sprints/demo-1.md": strings.SplitN(sprint, "---\n", 3)[2]}, nil,
			"sprints/demo-1: missing YAML header"},
		{"unknown-type", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "type: sprint", "type: epic", 1)}, nil,
			"sprints/demo-1: type must be one of project, sprint, day, design, doc, postmortem, got 'epic'"},
		{"typed-type", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "type: sprint", "type: 5", 1)}, nil,
			"sprints/demo-1: type must be one of project, sprint, day, design, doc, postmortem, got 5"},
		{"missing-field", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "title: First\n", "", 1)}, nil,
			"sprints/demo-1: missing header fields: title"},
		{"false-title", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "title: First", "title: no", 1)}, nil,
			"sprints/demo-1: missing header fields: title"},
		{"unknown-block", map[string]string{"sprints/demo-1.md": sprint + "\n::: warning\nx\n:::\n"}, nil,
			"sprints/demo-1: unknown block '::: warning' (allowed: decision, note, result)"},
		{"block-attr", map[string]string{"sprints/demo-1.md": sprint + "\n::: result\nx\n:::\n"}, nil,
			"sprints/demo-1: ::: result needs 'title'"},
		{"unknown-bead", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "bead: demo.1", "bead: demo.99", 1)}, nil,
			"postmortems/2026-10-03-outage: no sprint record has bead demo.1"},
		{"hand-progress", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "> Do not write here.\n",
			"> Do not write here.\n\nMine.\n", 1)}, nil,
			"sprints/demo-1.md:42: hand-written text in '## Progress', which is generated from the work store when the page is rendered; move it to Findings or Decisions, or remove it"},
		{"generated-heading", map[string]string{"sprints/demo-1.md": sprint + "\n## Docs\n\nx\n"}, nil,
			"sprints/demo-1.md:81: '## Docs' is a section the page generates; remove the heading and move its text to a section written by hand"},
		{"missing-section", map[string]string{"design/parser.md": strings.Replace(design, "## Open questions", "## Questions", 1)}, nil,
			"design/parser: design record needs a '## Open questions' section"},
		{"bad-outcome", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "Not closed yet.\n\n### Against",
			"Maybe.\n\n### Against", 1)}, nil,
			"sprints/demo-1: Delivery report Outcome must start with done, partial or voided"},
		{"no-outcome-part", map[string]string{"sprints/demo-1.md": strings.Replace(sprint, "### Outcome", "### Result", 1)}, nil,
			"sprints/demo-1: Delivery report needs a '### Outcome' subsection"},
		{"doc-two-keys", map[string]string{"docs/2026-10-02-notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10-02\n" +
			"bead: demo.1\nproject: demo\n---\n\nx\n"}, nil,
			"docs/2026-10-02-notes: a doc names exactly one of 'bead' or 'project' in its header"},
		{"feedback-with-project", map[string]string{"docs/pm-feedback.md": "---\ntype: doc\ntitle: pm feedback\n" +
			"date: 2026-10-02\nproject: demo\n---\n\nx\n"}, nil,
			"docs/pm-feedback: the pm feedback doc is the repo's, about any project; its header names neither 'bead' nor 'project'"},
		{"second-feedback-doc", map[string]string{"docs/2026-10-03-demo-feedback.md": "---\ntype: doc\ntitle: pm feedback\n" +
			"date: 2026-10-03\nproject: demo\n---\n\nx\n"}, nil,
			"docs/2026-10-03-demo-feedback: a repo keeps one pm feedback doc, records/docs/pm-feedback.md; move this doc's entries into it in time order, each tagged with its project (About project `<name>`.), delete this doc and commit both with pm commit"},
		{"doc-bad-date", map[string]string{"docs/2026-10-02-notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10\n" +
			"project: demo\n---\n\nx\n"}, nil,
			"docs/2026-10-02-notes: date must be YYYY-MM-DD, got '2026-10'"},
		{"doc-bad-path", map[string]string{"docs/notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10-02\nproject: demo\n" +
			"---\n\nx\n"}, nil,
			"docs/notes: a doc dated 2026-10-02 lives at docs/2026-10-02-<slug>.md (slug: lowercase words joined by '-')"},
		{"postmortem-no-sprint", map[string]string{"postmortems/2026-10-03-outage.md": "",
			"postmortems/2026-10-03-lost.md": "---\ntype: postmortem\ntitle: Lost\ndate: 2026-10-03\nsprint: demo.42\n" +
				"---\n\nx\n"}, nil,
			"postmortems/2026-10-03-lost: no sprint record has bead demo.42"},
		{"no-project", map[string]string{"design/parser.md": strings.Replace(design, "project: demo", "project: nowhere", 1)}, nil,
			"design/parser: no project record found"},
		{"uncited-need", nil, []string{uncited},
			`need demo.1.9 (Pick a parser) is closed but no decision cites it; record the owner's answer with pm decision add --need demo.1.9, or, if the answer sets no rule, close it with pm decision close demo.1.9 --reason "<why>"`},
		{"bad-summary", map[string]string{"days/2026-10-01.summary.json": `{"date": "2026-10-02", "generated_at": "x", "digest": "d", "text": "t"}`}, nil,
			"days/2026-10-01.summary.json: its date is '2026-10-02', not the date in its name"},
		{"summary-not-object", map[string]string{"days/2026-10-01.summary.json": "[1]"}, nil,
			"days/2026-10-01.summary.json: a summary is a JSON object with string fields date, generated_at, digest and text; fix it or delete it and run pm day summarize"},
		{"summary-bad-time", map[string]string{"days/2026-10-01.summary.json": `{"date": "2026-10-01", "generated_at": "noon", "digest": "d", "text": "t"}`}, nil,
			"days/2026-10-01.summary.json: generated_at 'noon' is not an ISO timestamp"},
	}
	for _, c := range cases {
		dir := constructsStore(t, c.edits)
		got := "no error"
		recs, err := records.Read(dir, nil)
		if err == nil {
			var summaries map[string]*records.Summary
			if summaries, err = records.ReadSummaries(dir); err == nil {
				_, err = Check(recs, constructsItems(t, c.extra...), "demo", summaries)
			}
		}
		if err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("%s: pm check gives %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCitesMatchesTheIDAlone(t *testing.T) {
	for _, c := range []struct {
		text string
		want bool
	}{
		{"see demo.1.2 here", true}, {"demo.1.2", true}, {"demo.1.23", false}, {"xdemo.1.2", false},
		{"demo.1.2.3", false}, {"demo.1.2.", true}, {"(demo.1.2)", true}, {"a-demo.1.2", false}, {"demo.1.2-x", false},
		{"demo.1.2 demo.1.2x", true}, {".demo.1.2", false}, {"éxdemo.1.2", false}, {"demo.1.2é", false},
		{"demo.1.2.é", false}, {"demo.1.2,", true}, {"", false},
	} {
		if got := Cites(c.text, "demo.1.2"); got != c.want {
			t.Errorf("Cites(%q, demo.1.2) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestIndexListsEverySprintNotDoneAndOnlyTheLatestClosedDoneOnes(t *testing.T) {
	project, err := records.Parse("p.md", "projects/p", "---\ntype: project\nbead: p\ntitle: P\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	items := []work.Item{{ID: "p", Type: work.Project, Title: "P", Status: work.Open, CreatedAt: at, UpdatedAt: at}}
	for n := 0; n < 12; n++ { // done sprints done-00..done-11, done-11 closed last
		closed := at.AddDate(0, 0, n)
		items = append(items, work.Item{ID: fmt.Sprintf("p.%d", n+1), Type: work.Sprint, Parent: "p", Number: n + 1,
			Title: fmt.Sprintf("done-%02d", n), Status: work.Closed, Resolution: work.Done, CreatedAt: at,
			UpdatedAt: closed, ClosedAt: closed})
	}
	for n := 0; n < 3; n++ {
		items = append(items, work.Item{ID: fmt.Sprintf("p.%d", n+13), Type: work.Sprint, Parent: "p", Number: n + 13,
			Title: fmt.Sprintf("live-%d", n), Status: work.Open, CreatedAt: at, UpdatedAt: at})
	}
	page, err := RenderIndex([]*Record{project}, records.NewItems(items), "site", nil)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		if !strings.Contains(page, fmt.Sprintf("live-%d", n)) {
			t.Errorf("live-%d is not listed", n)
		}
	}
	var shown []int
	for n := 0; n < 12; n++ {
		if strings.Contains(page, fmt.Sprintf("done-%02d", n)) {
			shown = append(shown, n)
		}
	}
	if DoneSprintsShown != 8 || fmt.Sprint(shown) != "[4 5 6 7 8 9 10 11]" {
		t.Errorf("done sprints shown %v, DoneSprintsShown %d", shown, DoneSprintsShown)
	}
	if !strings.Contains(page, "4 older done sprints not shown") {
		t.Error("the index does not say 4 older done sprints are not shown")
	}
}

func union(a, b map[string]string) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Diff is the changed lines of b against a, each run with the line before it: "  " context, "- " a only, "+ " b only.
func Diff(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	mx, my := x[pre:len(x)-suf], y[pre:len(y)-suf]
	var ops []string
	if len(mx)*len(my) > 4_000_000 {
		for _, l := range mx {
			ops = append(ops, "- "+l)
		}
		for _, l := range my {
			ops = append(ops, "+ "+l)
		}
	} else {
		lcs := make([][]int, len(mx)+1)
		for i := range lcs {
			lcs[i] = make([]int, len(my)+1)
		}
		for i := len(mx) - 1; i >= 0; i-- {
			for j := len(my) - 1; j >= 0; j-- {
				if mx[i] == my[j] {
					lcs[i][j] = lcs[i+1][j+1] + 1
				} else {
					lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
				}
			}
		}
		i, j := 0, 0
		for i < len(mx) || j < len(my) {
			switch {
			case i < len(mx) && j < len(my) && mx[i] == my[j]:
				ops = append(ops, "  "+mx[i])
				i, j = i+1, j+1
			case j < len(my) && (i == len(mx) || lcs[i][j+1] >= lcs[i+1][j]):
				ops = append(ops, "+ "+my[j])
				j++
			default:
				ops = append(ops, "- "+mx[i])
				i++
			}
		}
	}
	var out []string
	if pre > 0 {
		out = append(out, "  "+x[pre-1])
	}
	for k, op := range ops {
		if strings.HasPrefix(op, "  ") && (k+1 >= len(ops) || strings.HasPrefix(ops[k+1], "  ")) {
			continue
		}
		out = append(out, op)
	}
	return strings.Join(out, "\n")
}
