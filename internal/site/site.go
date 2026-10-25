// Package site renders records joined with live work-store status: the pages the pm service serves and pm check
// renders. Page assembly uses html/template, and fragments are escaped with the standard library's html.EscapeString;
// golden_test.go holds every page of testdata/constructs to its frozen copy.
//
// serve.go holds what only the service's pages have: the reply forms and the status line. versions.go serves each
// draft beside the published records as a version, with the version bar and the URLs under DraftsDir; diff.go is a
// version's diff by section. activity.go is a day's activity: the sprint rows a day page shows and the text pm day
// summarize sends the model. The pages load Mermaid at one exact version, MermaidVersion. A page change shows in the
// golden pages (testdata/constructs/pages); a construct the fixture lacks gets a record or item in testdata/constructs.
package site

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	pm "github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// Style is the one stylesheet every page gets.
var Style = pm.Style

type (
	Record = records.Record
	Items  = records.Items
	Item   = item.Item
)

// Dates is the design pages' dates by rel; nil for a render that only validates, whose pages show none.
type Dates = map[string]store.Dates

func errorf(format string, a ...any) error { return &records.Error{Msg: fmt.Sprintf(format, a...)} }

var pills = map[string][2]string{"done": {"done", "DONE"}, "dropped": {"dropped", "DROPPED"},
	"proposed": {"proposed", "PROPOSED"}, "running": {"run", "RUNNING"}, "blocked": {"blocked", "BLOCKED"}, "ready": {"queued", "READY"}}

func pill(st string) string {
	p := pills[st]
	return `<span class="pill ` + p[0] + `">` + p[1] + "</span>"
}

// ---------------------------------------------------------------- item status

func isEpic(it *Item) bool { return it.Type == item.Project || it.Type == item.Sprint }

// State is one of done, dropped, proposed, running, blocked and ready: done once closed, dropped for a task closed as
// dismissed (pm task close --dropped: not done) and for a closed proposal nobody started, proposed for an open one
// (Items.Unstarted), running while held or, for a project or sprint, once any child has started or finished; blocked
// while a blocker is not closed.
func State(it *Item, items *Items) string {
	switch {
	case it.Status == item.Closed && it.Type == item.Task && it.Resolution == item.Dismissed:
		return "dropped"
	case it.Status == item.Closed && items.Unstarted(it):
		return "dropped"
	case it.Status == item.Closed:
		return "done"
	case items.Unstarted(it):
		return "proposed"
	case it.Holder != nil:
		return "running"
	}
	if isEpic(it) {
		for _, k := range items.All() {
			if k.Parent == it.ID && (k.Holder != nil || k.Status == item.Closed && !items.Unstarted(k)) {
				return "running"
			}
		}
	}
	for _, b := range it.BlockedBy {
		if x := items.Get(b); x == nil || x.Status != item.Closed {
			return "blocked"
		}
	}
	return "ready"
}

// Kind is what a need waits for: an action by the owner (a PR review is one), the answer to a session's question,
// else a decision.
func Kind(it *Item) string {
	if it.Need != nil && (it.Need.Kind == item.Action || it.Need.Kind == item.Review) {
		return "action"
	}
	if it.Need != nil && it.Need.Kind == item.Question {
		return "question"
	}
	return "decision"
}

// OwnerTasks is the open needs under a project item: what awaits the owner, newest first. Under "", it is the open
// needs with no parent: questions from sessions that held no task.
func OwnerTasks(items *Items, under string) []*Item {
	var out []*Item
	for _, it := range items.All() {
		if it.Status != item.Closed && it.Type == item.Need && (contains(items.Ancestors(it.ID), under) ||
			under == "" && it.Parent == "") {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func childrenWhere(items *Items, parent string, keep func(*Item) bool) []*Item {
	var out []*Item
	for _, it := range items.Children(parent) {
		if keep == nil || keep(it) {
			out = append(out, it)
		}
	}
	return out
}

func sprintsOf(items *Items, project string) []*Item {
	return childrenWhere(items, project, func(it *Item) bool { return it.Type == item.Sprint })
}

func utcDate(t time.Time) string { return t.UTC().Format("2006-01-02") }

// LocalDay is the local calendar date of a timestamp; "" for none.
func LocalDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}

// ---------------------------------------------------------------- links and lists

func link(frm *Record, to string) string {
	if frm == nil {
		return to
	}
	return strings.Repeat("../", strings.Count(frm.Rel, "/")) + to
}

var (
	nonWord = regexp.MustCompile(`[^\p{L}\p{N}_]`)
	mdHref  = regexp.MustCompile(`href="([^":]+?)\.md"`)
)

// TaskGraph is a Mermaid graph: one box per sprint holding its tasks, coloured by state, arrows for blockers.
func TaskGraph(sprints []*Item, items *Items) string {
	nid := func(id string) string { return nonWord.ReplaceAllString(id, "_") }
	label := func(s string) string { return strings.ReplaceAll(s, `"`, "#quot;") }
	sorted := append([]*Item{}, sprints...)
	sort.SliceStable(sorted, func(i, j int) bool { return item.CompareIDs(sorted[i].ID, sorted[j].ID) < 0 })
	lines, edges := []string{"flowchart LR"}, []string{}
	for _, sp := range sorted {
		lines = append(lines, fmt.Sprintf(`  subgraph %s["%s"]`, nid(sp.ID), label(sp.Title)), "    direction TB")
		tasks := items.Children(sp.ID)
		for _, t := range tasks {
			lines = append(lines, fmt.Sprintf(`    %s["%s"]:::%s`, nid(t.ID), label(t.Title), State(t, items)))
			for _, b := range t.BlockedBy {
				edges = append(edges, fmt.Sprintf("  %s --> %s", nid(b), nid(t.ID)))
			}
		}
		if len(tasks) == 0 {
			lines = append(lines, fmt.Sprintf(`    %s_empty["no tasks yet"]`, nid(sp.ID)))
		}
		lines = append(lines, "  end")
	}
	lines = append(lines, edges...)
	lines = append(lines,
		"  classDef done fill:#e4f2e7,stroke:#2c7a3f,color:#1d2321",
		"  classDef dropped fill:#eceeed,stroke:#5d6763,color:#5d6763,stroke-dasharray:4 3",
		"  classDef running fill:#e3edf8,stroke:#1d5fa8,color:#1d2321",
		"  classDef ready fill:#f6efd9,stroke:#8a6a12,color:#1d2321",
		"  classDef blocked fill:#f7e3e1,stroke:#a8322d,color:#1d2321")
	legend := `<p class="meta">` + pill("done") + " " + pill("dropped") + " " + pill("running") + " " + pill("ready") +
		" " + pill("blocked") +
		"<span>arrows: must finish first</span></p>\n"
	return `<pre class="mermaid">` + esc(strings.Join(lines, "\n")) + "</pre>\n" + legend
}

// inline is one line of Markdown, with .md links pointing at rendered pages.
func inline(text string) string {
	return mdHref.ReplaceAllString(renderInline(inlineHTML, text), `href="$1.html"`)
}

func progress(project *Record, recs []*Record, items *Items, frm *Record) (string, error) {
	root := project.Bead()
	graph := TaskGraph(childrenWhere(items, root, func(it *Item) bool { return !items.Unstarted(it) }), items)
	var sprints []*Record
	for _, r := range recs {
		if r.Type() == "sprint" && records.ProjectOf(r, recs, items) == project {
			sprints = append(sprints, r)
		}
	}
	sort.SliceStable(sprints, func(i, j int) bool { return item.CompareIDs(sprints[i].Bead(), sprints[j].Bead()) < 0 })
	var rows []string
	for _, r := range sprints {
		b := items.Get(r.Bead())
		if b == nil {
			return "", errorf("%s: bead %s not found in the work store", r.Rel, r.Bead())
		}
		if items.Proposal(b) { // the Proposed section lists it
			continue
		}
		report, err := records.Outcome(r)
		if err != nil {
			return "", err
		}
		out := "<span class=empty>not closed</span>"
		if report != "" {
			out = inline(report)
		} else if items.Unstarted(b) {
			out = "dropped: " + esc(b.CloseReason)
		}
		rows = append(rows, `<tr><td><a href="`+link(frm, r.Out())+`">`+esc(r.Title())+"</a></td>"+
			"<td>"+esc(utcDate(b.CreatedAt))+"</td>"+
			"<td>"+inline(records.FirstPara(records.SectionText(r.Body, "Goal")))+"</td>"+
			"<td>"+out+"</td><td>"+pill(State(b, items))+"</td></tr>")
	}
	table := ""
	if rows != nil {
		table = `<div class="tbl"><table><tr><th>Sprint</th><th>Opened</th><th>Goal</th>` +
			"<th>Outcome</th><th>Status</th></tr>" + strings.Join(rows, "") + "</table></div>\n"
	}
	return graph + table + proposedSection(childrenWhere(items, root, nil), recs, items, frm), nil
}

// OpenSprintRows is pm sprint propose's duplicate list: a line per open sprint under project but except, proposals
// included, with its state, id, title and its goal's first sentence.
func OpenSprintRows(project, except string, recs []*Record, items *Items) []string {
	var out []string
	for _, sp := range sprintsOf(items, project) {
		if sp.Status == item.Closed || sp.ID == except {
			continue
		}
		line := fmt.Sprintf("  %-8s  %s  %s", State(sp, items), sp.ID, sp.Title)
		for _, r := range recs {
			if b, ok := r.ID("bead"); ok && b == sp.ID && r.Type() == "sprint" {
				line += "  " + records.FirstSentence(records.SectionText(r.Body, "Goal"), 110)
			}
		}
		out = append(out, line)
	}
	return out
}

// proposedSection is the collapsed Proposed section: a row per open proposal among sprints, with its pill, its title
// linking its record, and its goal's first sentence; empty when there is none.
func proposedSection(sprints []*Item, recs []*Record, items *Items, frm *Record) string {
	var rows []string
	for _, sp := range sprints {
		if !items.Proposal(sp) {
			continue
		}
		title, goal := esc(sp.Title), ""
		for _, r := range recs {
			if b, ok := r.ID("bead"); ok && b == sp.ID && r.Type() == "sprint" {
				title = `<a href="` + link(frm, r.Out()) + `">` + title + "</a>"
				goal = `<span class="k">` + inline(records.FirstSentence(records.SectionText(r.Body, "Goal"), 200)) +
					"</span>"
			}
		}
		rows = append(rows, "<li>"+pill("proposed")+" "+title+goal+"</li>")
	}
	if rows == nil {
		return ""
	}
	return fmt.Sprintf(`<details class="proposals"><summary>Proposed (%d)</summary><ul class="list">%s</ul></details>`+"\n",
		len(rows), strings.Join(rows, ""))
}

type unsprintedItem struct {
	it      *Item
	project *Record
}

// unsprinted is each open task or need filed directly under a project epic or with no parent, with its project (nil
// for none), by id. A need under a project, and a question with no parent, is left out: the await-you sections show
// it.
func unsprinted(recs []*Record, items *Items) []unsprintedItem {
	projects := map[string]*Record{}
	for _, r := range recs {
		if r.Type() == "project" {
			projects[r.Bead()] = r
		}
	}
	var out []unsprintedItem
	for _, it := range items.All() {
		if it.Status == item.Closed || isEpic(it) {
			continue
		}
		if p := projects[it.Parent]; it.Parent == "" && Kind(it) != "question" || (p != nil && it.Type != item.Need) {
			out = append(out, unsprintedItem{it, p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return item.CompareIDs(out[i].it.ID, out[j].it.ID) < 0 })
	return out
}

// unsprintedTable is the generated 'Not in a sprint' table; empty when there are none. The type is bd's issue type,
// which the work store keeps as the label bug.
func unsprintedTable(list []unsprintedItem, frm *Record, heading string) string {
	if len(list) == 0 {
		return ""
	}
	var rows strings.Builder
	for _, u := range list {
		typ, project := "task", "—"
		if contains(u.it.Labels, "bug") {
			typ = "bug"
		}
		if u.project != nil {
			project = `<a href="` + link(frm, u.project.Out()) + `">` + esc(u.project.Title()) + "</a>"
		}
		rows.WriteString("<tr><td>" + esc(u.it.ID) + "</td><td>" + typ + "</td><td>" + esc(u.it.Title) + "</td><td>" +
			project + "</td></tr>")
	}
	return "<" + heading + ` id="not-in-a-sprint">Not in a sprint</` + heading + ">\n" + `<div class="tbl"><table><tr>` +
		"<th>Item</th><th>Type</th><th>Title</th><th>Project</th></tr>" + rows.String() + "</table></div>\n"
}

// docList is a generated list of dated records (docs or postmortems), newest first, linked relative to frm; empty
// when there are none.
func docList(docs []*Record, frm *Record, heading, label string) string {
	if len(docs) == 0 {
		return ""
	}
	var lis strings.Builder
	for _, d := range newestFirst(docs) {
		lis.WriteString(`<li><a href="` + link(frm, d.Out()) + `">` + esc(d.Title()) + "</a>" +
			`<span class="k">` + esc(d.Meta["date"].Str) + "</span></li>")
	}
	return "<" + heading + ` id="` + strings.ToLower(label) + `">` + label + "</" + heading + `><ul class="list">` +
		lis.String() + "</ul>\n"
}

// newestFirst is dated records by date, newest first, then by path, last first.
func newestFirst(docs []*Record) []*Record {
	sorted := append([]*Record{}, docs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		di, dj := sorted[i].Meta["date"].Str, sorted[j].Meta["date"].Str
		if di != dj {
			return di > dj
		}
		return sorted[i].Rel > sorted[j].Rel
	})
	return sorted
}

func sprintDocs(sprint *Record, recs []*Record, items *Items) []*Record {
	sid := sprint.Bead()
	var out []*Record
	for _, r := range recs {
		if r.Type() != "doc" || !r.Has("bead") {
			continue
		}
		if b, ok := r.ID("bead"); ok && (b == sid || contains(items.Ancestors(b), sid)) {
			out = append(out, r)
		}
	}
	return out
}

func projectDocs(project *Record, recs []*Record, items *Items, kind string) []*Record {
	var out []*Record
	for _, r := range recs {
		if r.Type() == kind && records.ProjectOf(r, recs, items) == project {
			out = append(out, r)
		}
	}
	return out
}

func sprintPostmortems(sprint *Record, recs []*Record) []*Record {
	var out []*Record
	for _, r := range recs {
		if v, ok := r.Meta["sprint"]; ok && r.Type() == "postmortem" && v.Kind == sprint.Meta["bead"].Kind &&
			v.Str == sprint.Meta["bead"].Str {
			out = append(out, r)
		}
	}
	return out
}

// sprintTitle is a sprint's title, linked to its record when it has one.
func sprintTitle(id, title string, recs []*Record, frm *Record) string {
	for _, r := range recs {
		if b, ok := r.ID("bead"); ok && r.Type() == "sprint" && b == id {
			return `<a href="` + link(frm, r.Out()) + `">` + esc(title) + "</a>"
		}
	}
	return esc(title)
}

// itemTitle is the sprint title of an id: the item's, else the id itself.
func itemTitle(items *Items, id string, recs []*Record, frm *Record) string {
	if it := items.Get(id); it != nil {
		return sprintTitle(it.ID, it.Title, recs, frm)
	}
	return sprintTitle(id, id, recs, frm)
}

// ---------------------------------------------------------------- days

var dayVerbs = []struct {
	at          func(*Item) time.Time
	verb, class string
}{
	{func(it *Item) time.Time { return it.ClosedAt }, "closed", "done"},
	{func(it *Item) time.Time { return it.StartedAt }, "started", "run"},
	{func(it *Item) time.Time { return it.CreatedAt }, "opened", "queued"},
}

func dayActivity(day string, items *Items, under string, recs []*Record, frm *Record) string {
	var out []string
	for _, sp := range sprintsOf(items, under) {
		var rows []string
		for _, m := range sprintMarks(sp, items, func(t time.Time) bool { return LocalDay(t) == day }) {
			row := `<li><span class="pill queued">OPENED</span> this sprint</li>`
			switch m {
			case "proposed":
				row = "<li>" + pill("proposed") + " this sprint</li>"
			case "started":
				row = `<li><span class="pill run">STARTED</span> this proposal: its first task</li>`
			case "dropped":
				row = "<li>" + pill("dropped") + ` this proposal <span class="k">` + esc(sp.CloseReason) + "</span></li>"
			}
			rows = append(rows, row)
		}
		for _, t := range items.Children(sp.ID) {
			for _, v := range dayVerbs {
				if LocalDay(v.at(t)) == day {
					if v.verb == "closed" && State(t, items) == "dropped" {
						v.verb, v.class = "dropped", "dropped"
					}
					rows = append(rows, `<li><span class="pill `+v.class+`">`+strings.ToUpper(v.verb)+"</span> "+
						esc(t.Title)+` <span class="k">`+esc(t.ID)+"</span></li>")
					break
				}
			}
		}
		if rows != nil {
			out = append(out, `<section class="card"><h4>`+pill(State(sp, items))+sprintTitle(sp.ID, sp.Title, recs, frm)+
				`</h4><ul class="list">`+strings.Join(rows, "")+"</ul></section>")
		}
	}
	if out == nil {
		return `<p class="empty">No sprint activity in the work store on this day.</p>`
	}
	return strings.Join(out, "")
}

// activityDays is every date a day page shows: a sprint opened or a task opened, started or closed in a project, or a
// doc dated that day.
func activityDays(recs []*Record, items *Items) map[string]bool {
	days := map[string]bool{}
	for _, r := range recs {
		if r.Type() == "doc" {
			days[r.Meta["date"].Str] = true
		}
	}
	for _, p := range recs {
		if p.Type() != "project" {
			continue
		}
		for _, sp := range sprintsOf(items, p.Bead()) {
			days[LocalDay(sp.CreatedAt)] = true
			if items.Unstarted(sp) { // a dropped proposal's day; a started one's is its first task's
				days[LocalDay(sp.ClosedAt)] = true
			}
			for _, t := range items.Children(sp.ID) {
				for _, v := range dayVerbs {
					days[LocalDay(v.at(t))] = true
				}
			}
		}
	}
	delete(days, "")
	return days
}

// WithDays is recs plus a day record without a file for each date with activity or a summary but no day record;
// every day carries its generated summary if it has one.
func WithDays(recs []*Record, items *Items, summaries map[string]*records.Summary) []*Record {
	have := map[string]bool{}
	out := make([]*Record, 0, len(recs))
	for _, r := range recs {
		if r.Type() == "day" {
			have[r.Meta["date"].Str] = true
			c := *r
			c.Summary = summaries[r.Meta["date"].Str]
			r = &c
		}
		out = append(out, r)
	}
	days := activityDays(recs, items)
	for d := range summaries {
		days[d] = true
	}
	var add []string
	for d := range days {
		if !have[d] {
			add = append(add, d)
		}
	}
	sort.Strings(add)
	for _, d := range add {
		out = append(out, &Record{Path: "days/" + d + ".md", Rel: "days/" + d,
			Meta: records.Meta{"type": {Kind: records.KindStr, Str: "day", Truthy: true},
				"date": {Kind: records.KindStr, Str: d, Truthy: true}},
			Body: "## Today\n", Summary: summaries[d]})
	}
	return out
}

var blankLines = regexp.MustCompile(`\n[\t\n\v\f\r \x1c-\x1f\x85\p{Z}]*\n`)

func summaryMD(rec *Record) (string, error) {
	s := rec.Summary
	if s == nil {
		return "", nil
	}
	at, err := records.ParseISO(s.GeneratedAt)
	if err != nil {
		return "", err
	}
	text := blankLines.ReplaceAllString(mustRender(commentMD, records.Strip(s.Text)), "\n")
	return `<p class="meta"><span>generated at ` + at.In(time.Local).Format("15:04") + "</span></p>\n" + text, nil
}

func dayToday(rec *Record) string {
	if rec.Summary != nil {
		return records.SummaryLine(rec.Summary.Text)
	}
	return records.FirstPara(records.SectionText(rec.Body, "Today"))
}

// ---------------------------------------------------------------- requests

var awaitingKinds = []struct{ kind, anchor, heading, what, empty string }{
	{"question", "sessions-ask-you", "Sessions ask you", "An agent session asks; your answer steers that session " +
		"and records no decision.", "No session questions waiting on the owner."},
	{"decision", "decisions-await-you", "Decisions await you", "The owner chooses; the agent records the answer.",
		"No decisions waiting on the owner."},
	{"action", "actions-await-you", "Actions await you", "The owner does something; the agent closes it once it " +
		"sees it done.", "No actions waiting on the owner."},
}

var prNumber = regexp.MustCompile(`/pull/(\d+)`)

// PRLabel is "PR #<n>" for a pull request URL.
func PRLabel(url string) string {
	if m := prNumber.FindStringSubmatch(url); m != nil {
		return "PR #" + m[1]
	}
	return "PR"
}

func reviewDesigns(review *item.ReviewInfo, recs []*Record) []*Record {
	slugs := append([]string{}, review.Designs...)
	for _, r := range recs {
		if b, ok := r.ID("bead"); ok && r.Type() == "sprint" && contains(review.Sprints, b) {
			slugs = append(slugs, records.DesignLinks(r)...)
		}
	}
	bySlug := map[string]*Record{}
	for _, r := range recs {
		if r.Type() == "design" {
			bySlug[r.Name()] = r
		}
	}
	var out []*Record
	seen := map[string]bool{}
	for _, s := range slugs {
		if r, ok := bySlug[s]; ok && !seen[s] {
			out = append(out, r)
		}
		seen[s] = true
	}
	return out
}

func reviewContext(review *item.ReviewInfo, items *Items, recs []*Record, frm *Record, md func(string) (string, error)) (string, error) {
	pr := esc(review.PR)
	var titles []string
	for _, s := range review.Sprints {
		titles = append(titles, itemTitle(items, s, recs, frm))
	}
	sprints := strings.Join(titles, ", ")
	var written []string
	for _, r := range recs {
		if b, ok := r.ID("bead"); !ok || r.Type() != "sprint" || !contains(review.Sprints, b) {
			continue
		}
		o, err := records.Outcome(r)
		if err != nil {
			return "", err
		}
		if o != "" {
			written = append(written, `<a href="`+link(frm, r.Out())+`#delivery-report">delivery report: `+esc(r.Title())+"</a>")
		}
	}
	if written != nil {
		sprints += " · " + strings.Join(written, ", ")
	}
	var designs []string
	for _, r := range reviewDesigns(review, recs) {
		designs = append(designs, `<a href="`+link(frm, r.Out())+`">`+esc(r.Title())+"</a>")
	}
	ds := strings.Join(designs, ", ")
	if ds == "" {
		ds = `<span class="empty">none listed</span>`
	}
	focus, err := md(review.Focus)
	if err != nil {
		return "", err
	}
	return `<ul class="list review"><li>Pull request: <a href="` + pr + `">` + pr + "</a></li><li>Delivers: " + sprints +
		"</li><li>Design pages: " + ds + `</li></ul><p class="m-sec">Focus</p>` + focus, nil
}

// RequestPlace is the sprint and the task a request sits under in the project, from its parent chain: the sprint is
// the ancestor just below the project, the task the request's own parent when that is below the sprint; "" where
// absent.
func RequestPlace(it *Item, items *Items, project string) (sprint, task string) {
	chain := items.Ancestors(it.ID)
	var below []string
	for i, a := range chain {
		if a == project {
			below = chain[:i]
			break
		}
	}
	if len(below) > 0 {
		sprint = below[len(below)-1]
	}
	if len(below) > 1 {
		task = below[0]
	}
	return sprint, task
}

func cardWhere(it *Item, proj *Record, items *Items, recs []*Record, frm *Record) string {
	sid, tid := RequestPlace(it, items, proj.Bead())
	parts := []string{`<a href="` + link(frm, proj.Out()) + `">` + esc(proj.Title()) + "</a>"}
	if sid != "" {
		parts = append(parts, itemTitle(items, sid, recs, frm))
	}
	if tid != "" {
		title := tid
		if t := items.Get(tid); t != nil {
			title = t.Title
		}
		parts = append(parts, "task "+esc(title))
	}
	return strings.Join(parts, " · ")
}

// NotDelivered is a site reply's state before it reaches the asking session; NotPushed for a session that stores no
// inbox, which the pm service cannot push into (a Claude Code session has its messaging socket, a Codex session its
// CODEX_HOME to queue the reply under).
const (
	NotDelivered = "not delivered yet: the session that asked is not running, or the push failed and the pm service " +
		"tries again; the next session sees it"
	NotPushed = "not delivered yet: the session that asked has no inbox pm can push to; pm show flags the reply for " +
		"it, and pm reply read prints it"
)

var replyMark = regexp.MustCompile(`[\t\n\v\f\r \x1c-\x1f\x85\p{Z}]*<!-- pm-reply [A-Za-z0-9-]+ -->[\t\n\v\f\r \x1c-\x1f\x85\p{Z}]*$`)

// ReplyBody is a site reply's text as the owner wrote it, without its reply-id mark.
func ReplyBody(text string) string { return replyMark.ReplaceAllString(text, "") }

// thread is an open request's comments, oldest first, each with its author, time and whether it reached the asking
// session.
func thread(it *Item) string {
	if len(it.Comments) == 0 {
		return ""
	}
	seen, n := 0, 0
	if it.Need != nil {
		seen = it.Need.Delivered
	}
	var out strings.Builder
	for _, c := range it.Comments {
		author := c.Author
		if author == "" {
			author = "unknown"
		}
		status := ""
		if c.Kind == item.Reply {
			author = "you, on the site"
			switch need := it.Need; {
			case n < seen:
				status = ` · <span class="picked">delivered to the agent&#x27;s session</span>`
			case need != nil && need.RaisedBy != nil && need.RaisedBy.Session != "" && need.RaisedBy.Inbox == "":
				status = ` · <span class="replied">` + NotPushed + "</span>"
			default:
				status = ` · <span class="replied">` + NotDelivered + "</span>"
			}
			n++
		}
		when := ""
		if !c.CreatedAt.IsZero() {
			when = c.CreatedAt.UTC().Format("2006-01-02 15:04:05")
		}
		out.WriteString(`<div class="reply-item"><p class="k">` + esc(author) + " · " + esc(when) + " UTC" + status +
			"</p>" + mustRender(commentMD, ReplyBody(c.Text)) + "</div>")
	}
	return `<div class="replies"><p class="k">Replies</p>` + out.String() + "</div>"
}

// ReplySlot is where a card's reply form goes; the pm service fills it on every page it serves.
func ReplySlot(id, kind string) string { return "<!--pm-reply " + esc(id) + " " + kind + "-->" }

// StatusSlot is where the line stating the age of a page's data goes; the pm service fills it.
const StatusSlot = "<!--pm-status-->"

func ownerCard(it *Item, where string, items *Items, md func(string) (string, error), recs []*Record, frm *Record) (string, error) {
	var desc string
	var err error
	title, cls := esc(it.Title), ""
	switch {
	case it.Need != nil && it.Need.Review != nil:
		desc, err = reviewContext(it.Need.Review, items, recs, frm, md)
	case it.Need != nil && it.Need.Ask != nil:
		title, cls = "Session "+esc(item.ShortSession(it.Need.RaisedBy.Session))+" asks you", " question"
		desc = askFields(it.ID, *it.Need.Ask)
	default:
		desc, err = md(it.Description)
		if desc == "" {
			desc = `<p class="empty">No details given.</p>`
		}
	}
	if err != nil {
		return "", err
	}
	how := "Reply below with the evidence once done; the agent checks it and closes this."
	switch Kind(it) {
	case "decision":
		how = "Reply below, or answer with <code>pm reply add " + esc(it.ID) + "</code>; the agent that asked " +
			"records your answer."
	case "question":
		how = "Answer each question and send it, or answer with <code>pm reply add " + esc(it.ID) + "</code>; your " +
			"answer goes to the session that asked."
	}
	return `<div class="card need` + cls + `" id="need-` + esc(it.ID) + `"><h4>` + pill(State(it, items)) + title +
		`</h4><p class="k">` + where + " · " + esc(it.ID) + "</p>" + desc + `<p class="k">` + how + "</p>" + thread(it) +
		ReplySlot(it.ID, Kind(it)) + "</div>", nil
}

// askFields is a question card's questions, each with its header and its options: radio buttons, checkboxes when it
// is multi-select, and an "Other" text for the owner's own answer. Each input belongs to the reply form the pm service
// puts in the card's slot (form="reply-<id>"), and posts under the names item.AnswerText reads.
func askFields(id string, ask item.AskInfo) string {
	form := `form="reply-` + esc(id) + `"`
	var b strings.Builder
	for i, q := range ask.Questions {
		kind, choose := "radio", ""
		if q.Multi {
			kind, choose = "checkbox", ` <span class="k">choose one or more</span>`
		}
		b.WriteString(`<fieldset class="ask"><legend>`)
		if q.Header != "" {
			b.WriteString(`<span class="pill queued">` + esc(q.Header) + "</span> ")
		}
		b.WriteString(esc(q.Text) + choose + "</legend>")
		for _, o := range q.Options {
			detail := ""
			if o.Detail != "" {
				detail = ` <span class="k">` + esc(o.Detail) + "</span>"
			}
			b.WriteString(`<label><input type="` + kind + `" name="` + item.AnswerField(i) + `" value="` + esc(o.Label) +
				`" ` + form + "> " + esc(o.Label) + detail + "</label>")
		}
		other, hint := "Other", "your own answer, instead of an option"
		switch {
		case len(q.Options) == 0:
			other, hint = "Answer", "your answer"
		case q.Multi:
			hint = "your own answer, added to the options you pick"
		}
		b.WriteString(`<label class="other">` + other + ` <input type="text" name="` + item.OtherField(i) +
			`" placeholder="` + hint + `" ` + form + "></label></fieldset>")
	}
	return b.String()
}

type card struct {
	item *Item
	html string
}

func awaiting(cards []card, prompt bool) string {
	var out []string
	for _, k := range awaitingKinds {
		var body strings.Builder
		for _, c := range cards {
			if Kind(c.item) == k.kind {
				body.WriteString(c.html)
			}
		}
		b := body.String()
		if b == "" {
			b = `<p class="empty">` + k.empty + "</p>"
		}
		quote := ""
		if prompt {
			which := k.kind + " needs"
			if k.kind == "action" {
				which = "action and PR review needs"
			}
			quote = "<blockquote><p>" + k.what + " Generated from the work store: open " + which +
				".</p></blockquote>\n"
		}
		out = append(out, `<h2 id="`+k.anchor+`">`+k.heading+"</h2>\n"+quote+b)
	}
	return strings.Join(out, "\n")
}

func cardsFor(items *Items, proj *Record, recs []*Record, frm *Record, md func(string) (string, error)) ([]card, error) {
	var out []card
	for _, it := range OwnerTasks(items, proj.Bead()) {
		h, err := ownerCard(it, cardWhere(it, proj, items, recs, frm), items, md, recs, frm)
		if err != nil {
			return nil, err
		}
		out = append(out, card{it, h})
	}
	return out, nil
}

// ---------------------------------------------------------------- pages

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=IBM+Plex+Mono:wght@400;500&family=Fraunces:opsz,wght@9..144,600&display=swap">
<link rel="stylesheet" href="{{.CSS}}">
</head><body><div class="wrap">{{.Status}}
<nav class="topbar"><a href="{{.Home}}">Home</a>{{.Crumbs}}<a class="docs" href="{{.Docs}}">Docs</a></nav>
<div class="eyebrow">{{.Kind}}</div>
<h1>{{.Title}}</h1>
{{.Body}}
</div>{{.Mermaid}}{{.Math}}</body></html>
`))

// MermaidVersion is the exact Mermaid release the pages load: a pinned URL is immutable, so a browser that loaded it
// once keeps it, and a second CDN serves it when the first fails.
const MermaidVersion = "11.17.2"

// mermaidScript draws each pre.mermaid at its natural width (the pre scrolls sideways; useMaxWidth would shrink a wide
// diagram's text below reading size on a phone), and draws them again from their source when the colour scheme
// changes: the system's, or the page's data-theme.
var mermaidScript = strings.ReplaceAll(`
<script type="module">
const at = ["https://cdn.jsdelivr.net/npm/mermaid@VERSION/dist/mermaid.esm.min.mjs",
  "https://unpkg.com/mermaid@VERSION/dist/mermaid.esm.min.mjs"];
let mermaid;
try { mermaid = (await import(at[0])).default; } catch { mermaid = (await import(at[1])).default; }
const nodes = [...document.querySelectorAll("pre.mermaid")];
const sources = nodes.map(n => n.textContent);
const scheme = matchMedia("(prefers-color-scheme: dark)");
// Every diagram type that Mermaid's defaults give a useMaxWidth draws at natural width.
const types = Object.entries(mermaid.mermaidAPI.defaultConfig).filter(([, v]) => v && typeof v === "object" && "useMaxWidth" in v);
const wide = Object.fromEntries(types.map(([k]) => [k, { useMaxWidth: false }]));
let shown, queue = Promise.resolve();
async function draw() {
  const set = document.documentElement.dataset.theme;
  const theme = (set ? set === "dark" : scheme.matches) ? "dark" : "default";
  if (theme === shown) return;
  shown = theme;
  nodes.forEach((n, i) => { n.removeAttribute("data-processed"); n.textContent = sources[i]; });
  mermaid.initialize({ startOnLoad: false, theme, ...wide });
  await mermaid.run({ nodes });
}
const redraw = () => { queue = queue.then(draw).catch(console.error); }; // one failed draw does not stop the next
scheme.addEventListener("change", redraw);
new MutationObserver(redraw).observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
redraw();
</script>
`, "VERSION", MermaidVersion)

func page(rec *Record, kind, title, body string, c *ctx, crumbs string) (string, error) {
	mermaid, math := "", ""
	if c.mermaid {
		mermaid = mermaidScript
	}
	if c.math {
		math = katexScript
	}
	var b bytes.Buffer
	err := pageTemplate.Execute(&b, map[string]any{
		"Status": template.HTML(StatusSlot), "Title": title, "CSS": link(rec, "style.css"), "Home": link(rec, "index.html"),
		"Docs": link(rec, DocsIndex), "Crumbs": template.HTML(crumbs), "Kind": kind, "Body": template.HTML(body),
		"Mermaid": template.HTML(mermaid), "Math": template.HTML(math),
	})
	return b.String(), err
}

func withDocs(body, before string, docs, postmortems []*Record, frm *Record) string {
	listing := docList(docs, frm, "h2", "Docs") + docList(postmortems, frm, "h2", "Postmortems")
	if listing == "" {
		return body
	}
	return replaceFirst(regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(before)+`$`), body,
		func(m string) string { return listing + "\n" + m })
}

// replaceFirst replaces the first match of re in s with f(match): Python's re.sub(…, count=1) with a function.
func replaceFirst(re *regexp.Regexp, s string, f func(string) string) string {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return s
	}
	return s[:loc[0]] + f(s[loc[0]:loc[1]]) + s[loc[1]:]
}

func designDatesMeta(rec *Record, dates Dates) string {
	if dates == nil {
		return ""
	}
	d := dates[rec.Rel]
	return "<span>created " + esc(d.Created) + "</span><span>updated " + esc(d.Updated) + "</span>"
}

var mergedStamp = regexp.MustCompile(`Merged as ([0-9a-f]{7,40})\b`)

// codeChanged is the line a design page shows when the code its covers name changed after its last edit, or why that
// was not checked; "" when it was and nothing changed.
// A commit is left out when a sprint that updated the page (links it in Design pages) or waived it (pm design
// unchanged) stamped it as its merge: that sprint's page edit or waiver already answers for it, though the merge
// lands after the edit.
func codeChanged(page *Record, recs []*Record, d store.Dates) string {
	if d.Unchecked != "" {
		return `<p class="note draft">Code changes not checked: ` + esc(d.Unchecked) + ".</p>"
	}
	commits := d.Code
	var merges []string
	for _, r := range recs {
		if _, waived := design.Waivers(r)[page.Name()]; r.Type() == "sprint" &&
			(waived || contains(records.DesignLinks(r), page.Name())) {
			for _, m := range mergedStamp.FindAllStringSubmatch(r.Body, -1) {
				merges = append(merges, m[1])
			}
		}
	}
	var left []store.CodeCommit
	for _, c := range commits {
		if !slices.ContainsFunc(merges, func(m string) bool { return strings.HasPrefix(c.SHA, m) }) {
			left = append(left, c)
		}
	}
	if left == nil {
		return ""
	}
	latest := left[0].Day
	for _, c := range left {
		latest = max(latest, c.Day)
	}
	n := "1 commit"
	if len(left) != 1 {
		n = fmt.Sprintf("%d commits", len(left))
	}
	return `<p class="note draft">The code this page covers changed after its last edit: ` + n +
		" on the main branch, the latest on " + esc(latest) + ".</p>"
}

var (
	progressRE  = regexp.MustCompile(`(## Progress\n(?:\n?>.*\n)*)`)
	todayRE     = regexp.MustCompile(`(## Today\n(?:\n?>.*\n)*)`)
	decisionsRE = regexp.MustCompile(`(?m)^## Decisions$`)
)

// RenderRecord is one record's page.
func RenderRecord(rec *Record, recs []*Record, items *Items, dates Dates) (string, error) {
	p, _, err := renderRecord(rec, recs, items, dates)
	return p, err
}

// renderRecord is one record's page and each link of its own text that names no record (resolveLink).
func renderRecord(rec *Record, recs []*Record, items *Items, dates Dates) (string, []string, error) {
	c := &ctx{rel: rec.Rel, recs: recs}
	md := recordMD(c)
	render := func(s string) (string, error) { return render(md, s) }
	m := rec.Meta
	if err := records.Validate(rec, recs, items); err != nil {
		return "", nil, err
	}
	var proj *Record
	if rec.Type() != "day" {
		proj = records.ProjectOf(rec, recs, items)
	}
	crumbs := ""
	if proj != nil && rec.Type() != "project" {
		crumbs = ` / <a href="` + link(rec, proj.Out()) + `">` + esc(proj.Title()) + "</a>"
	}
	pre, body := "", rec.Body
	switch rec.Type() {
	case "project":
		c.mermaid = true
		var err error
		body, err = projectBody(rec, recs, items)
		if err != nil {
			return "", nil, err
		}
	case "sprint":
		c.mermaid = true
		var err error
		pre, body, err = sprintBody(rec, proj, recs, items, render)
		if err != nil {
			return "", nil, err
		}
	case "day":
		var err error
		body, err = dayBody(rec, recs, items, render)
		if err != nil {
			return "", nil, err
		}
	case "doc":
		where := ""
		if rec.Has("bead") {
			where = "<span>" + esc(m["bead"].Str) + "</span>"
		}
		pre = `<p class="meta"><span>` + esc(m["date"].Str) + "</span>" + where + "</p>"
	case "postmortem":
		where := ""
		if rec.Has("sprint") {
			where = "<span>" + itemTitle(items, m["sprint"].Str, recs, rec) + "</span>"
		}
		pre = `<p class="meta"><span>` + esc(m["date"].Str) + "</span>" + where + "</p>"
	case "design":
		if dates != nil {
			pre = `<p class="meta">` + designDatesMeta(rec, dates) + "</p>" + codeChanged(rec, recs, dates[rec.Rel])
		}
	}
	source := []byte(skipFrontMatter(body))
	c.links = true // the links of the record's text; a request's description renders above with none checked
	doc := md.Parser().Parse(text.NewReader(source))
	var content bytes.Buffer
	if err := md.Renderer().Render(&content, source, doc); err != nil {
		return "", nil, err
	}
	nav := ""
	if rec.Type() == "design" {
		nav = toc(doc, source)
	}
	p, err := page(rec, rec.Type(), rec.Title(), pre+nav+content.String(), c, crumbs)
	return p, c.broken, err
}

func projectBody(rec *Record, recs []*Record, items *Items) (string, error) {
	body := rec.Body
	generated, err := progress(rec, recs, items, rec)
	if err != nil {
		return "", err
	}
	var mine []unsprintedItem
	for _, u := range unsprinted(recs, items) {
		if u.project == rec {
			mine = append(mine, u)
		}
	}
	generated += unsprintedTable(mine, rec, "h3")
	body = replaceFirst(progressRE, body, func(g string) string { return g + "\n" + generated + "\n" })
	body = withDocs(body, "## Outcome", projectDocs(rec, recs, items, "doc"), projectDocs(rec, recs, items, "postmortem"), rec)
	return body, nil
}

func sprintBody(rec, proj *Record, recs []*Record, items *Items, render func(string) (string, error)) (string, string, error) {
	body := rec.Body
	b := items.Get(rec.Bead())
	pre := `<p class="meta">` + pill(State(b, items)) + "<span>" + esc(rec.Bead()) + "</span>" +
		"<span>opened " + esc(utcDate(b.CreatedAt)) + "</span></p>"
	generated := `<p class="empty">No tasks yet.</p>` // a chart of one empty box says no more
	if len(items.Children(b.ID)) > 0 {
		generated = TaskGraph([]*Item{b}, items)
	}
	body = replaceFirst(progressRE, body, func(g string) string { return g + "\n" + generated + "\n" })
	var cards []card
	for _, it := range OwnerTasks(items, rec.Bead()) {
		h, err := ownerCard(it, cardWhere(it, proj, items, recs, rec), items, render, recs, rec)
		if err != nil {
			return "", "", err
		}
		cards = append(cards, card{it, h})
	}
	needs := awaiting(cards, true)
	body = replaceFirst(decisionsRE, body, func(l string) string { return needs + "\n\n" + l })
	body = withDocs(body, "## Delivery report", sprintDocs(rec, recs, items), sprintPostmortems(rec, recs), rec)
	return pre, body, nil
}

func dayBody(rec *Record, recs []*Record, items *Items, render func(string) (string, error)) (string, error) {
	body := rec.Body
	day := rec.Meta["date"].Str
	var cards []card
	var projects []*Record
	for _, p := range recs {
		if p.Type() == "project" {
			projects = append(projects, p)
			cs, err := cardsFor(items, p, recs, rec, render)
			if err != nil {
				return "", err
			}
			cards = append(cards, cs...)
		}
	}
	generated := awaiting(cards, true)
	generated += "\n<h2 id=\"sprints\">Sprints</h2>\n<blockquote><p>Which sprints moved today, and what changed? " +
		"Generated from the work store: tasks opened, started or closed on this date.</p></blockquote>\n"
	for _, p := range projects {
		generated += "<h3>" + esc(p.Title()) + "</h3>" + dayActivity(day, items, p.Bead(), recs, rec)
	}
	var docs []*Record
	for _, r := range recs {
		if r.Type() == "doc" && r.Meta["date"].Str == day {
			docs = append(docs, r)
		}
	}
	generated += docList(docs, rec, "h2", "Docs")
	switch {
	case rec.Summary != nil: // the generated summary opens Today, above an older day's hand-written paragraph
		s, err := summaryMD(rec)
		if err != nil {
			return "", err
		}
		body = replaceFirst(todayRE, body, func(g string) string { return g + "\n" + s + "\n" })
	case rec.Text == "":
		body += "\nNo summary has been generated for this day.\n"
	}
	body = strings.TrimRight(body, "\n") + "\n\n" + generated + "\n"
	return body, nil
}

var numbered = regexp.MustCompile(`^[0-9]+\. `)

// toc is a design page's table of contents: its h2 and h3 headings, each linked to its id.
func toc(doc ast.Node, source []byte) string {
	var items []string
	openSub := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		if h.Level != 2 && h.Level != 3 {
			return ast.WalkSkipChildren, nil
		}
		title := esc(headingSource(h, source))
		href := ""
		if id, ok := h.AttributeString("id"); ok {
			href = string(id.([]byte))
		}
		if h.Level == 2 {
			switch {
			case openSub:
				items = append(items, "</ul></li>")
			case items != nil:
				items = append(items, "</li>")
			default:
				items = append(items, "")
			}
			openSub = false
			items = append(items, `<li><a href="#`+href+`">`+numbered.ReplaceAllString(title, "")+"</a>")
		} else {
			if !openSub {
				items = append(items, "<ul>")
				openSub = true
			}
			items = append(items, `<li><a href="#`+href+`">`+title+"</a></li>")
		}
		return ast.WalkSkipChildren, nil
	})
	if items == nil {
		return ""
	}
	if openSub {
		items = append(items, "</ul></li>")
	} else {
		items = append(items, "</li>")
	}
	return `<nav class="toc"><p class="cap">CONTENTS</p><ol>` + strings.Join(items, "") + "</ol></nav>"
}

// DoneSprintsShown is how many done sprints the overview lists, latest closed first; it lists every sprint not done.
const DoneSprintsShown = 8

// RenderIndex is the root page: the overview of everything open, across all projects. The open projects come first;
// the closed ones follow in one collapsed section. Design pages are listed by last update, newest first (by title
// within a day), with both dates; without dates, by title.
func RenderIndex(recs []*Record, items *Items, siteName string, dates Dates) (string, error) {
	c := &ctx{}
	md := func(s string) (string, error) { return render(plainMD, s) }
	var projects []*Record
	for _, r := range recs {
		if r.Type() == "project" {
			projects = append(projects, r)
		}
	}
	var cards []card
	for _, p := range projects {
		cs, err := cardsFor(items, p, recs, nil, md)
		if err != nil {
			return "", err
		}
		cards = append(cards, cs...)
	}
	for _, it := range OwnerTasks(items, "") { // a question from a session that holds no task sits under no project
		h, err := ownerCard(it, "asked outside any task", items, md, recs, nil)
		if err != nil {
			return "", err
		}
		cards = append(cards, card{it, h})
	}
	out := []string{awaiting(cards, false)}

	out = append(out, indexDays(recs))
	out = append(out, unsprintedTable(unsprinted(recs, items), nil, "h2"))
	var feedback []*Record // the repo's pm feedback doc, which no project lists
	for _, r := range recs {
		if records.IsFeedbackDoc(r) {
			feedback = append(feedback, r)
		}
	}
	out = append(out, docList(feedback, nil, "h2", "Feedback"))

	var closed []string
	for _, p := range projects {
		section := `<h2><a href="` + p.Out() + `">` + esc(p.Title()) + "</a></h2>" + indexSprints(p, recs, items) +
			indexDesigns(p, recs, items, dates) + docList(projectDocs(p, recs, items, "doc"), nil, "h3", "Docs") +
			docList(projectDocs(p, recs, items, "postmortem"), nil, "h3", "Postmortems")
		if it := items.Get(p.Bead()); it != nil && it.Status == item.Closed {
			closed = append(closed, section)
		} else {
			out = append(out, section)
		}
	}
	if closed != nil {
		out = append(out, fmt.Sprintf(`<details class="closed-projects"><summary>Closed projects (%d)</summary>%s</details>`,
			len(closed), strings.Join(closed, "")))
	}
	return page(nil, "overview", siteName, strings.Join(out, ""), c, "")
}

// DocsIndex is the path of the docs index, which every page's navigation links.
const DocsIndex = "docs/index.html"

// RenderDocsIndex is the docs index: every doc, newest first, with its date and the project it belongs to, and its
// bead when it names one.
func RenderDocsIndex(recs []*Record, items *Items) (string, error) {
	frm := &Record{Rel: strings.TrimSuffix(DocsIndex, ".html")}
	var docs []*Record
	for _, r := range recs {
		if r.Type() == "doc" {
			docs = append(docs, r)
		}
	}
	body := `<p class="empty">No docs yet.</p>`
	if docs != nil {
		var lis strings.Builder
		for _, d := range newestFirst(docs) {
			where := ""
			if p := records.ProjectOf(d, recs, items); p != nil {
				where = `<a href="` + link(frm, p.Out()) + `">` + esc(p.Title()) + "</a>"
			} else if records.IsFeedbackDoc(d) {
				where = "the repo's pm feedback, about any project"
			}
			if d.Has("bead") {
				where += " · " + esc(d.Meta["bead"].Str)
			}
			lis.WriteString(`<li><a href="` + link(frm, d.Out()) + `">` + esc(d.Title()) + "</a>" +
				`<span class="k">` + esc(d.Meta["date"].Str) + `</span><span class="k">` + where + "</span></li>")
		}
		body = `<ul class="list">` + lis.String() + "</ul>"
	}
	return page(frm, "docs", "Docs", body, &ctx{}, "")
}

func indexDays(recs []*Record) string {
	var days []*Record
	for _, r := range recs {
		if r.Type() == "day" {
			days = append(days, r)
		}
	}
	sort.SliceStable(days, func(i, j int) bool { return days[i].Meta["date"].Str > days[j].Meta["date"].Str })
	if len(days) > 7 {
		days = days[:7]
	}
	if days != nil {
		var lis strings.Builder
		for _, r := range days {
			today := inline(dayToday(r))
			if r.Summary != nil {
				today = renderInline(inlineText, dayToday(r))
			}
			lis.WriteString(`<li><a href="` + r.Out() + `">` + esc(r.Title()) + `</a><span class="k">` + today + "</span></li>")
		}
		return `<h2 id="days">Days</h2><ul class="list">` + lis.String() + "</ul>"
	}
	return ""
}

func indexSprints(p *Record, recs []*Record, items *Items) string {
	sprintRecs := map[string]*Record{}
	for _, r := range recs {
		if b, ok := r.ID("bead"); ok && r.Type() == "sprint" {
			sprintRecs[b] = r
		}
	}
	sprints := sprintsOf(items, p.Bead())
	var done []*Item
	for _, sp := range sprints {
		if sp.Status == item.Closed {
			done = append(done, sp)
		}
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].ClosedAt.After(done[j].ClosedAt) })
	shown := map[string]bool{}
	for i, sp := range done {
		if i < DoneSprintsShown {
			shown[sp.ID] = true
		}
	}
	var rows []string
	for _, sp := range sprints {
		if (sp.Status != item.Closed || shown[sp.ID]) && !items.Proposal(sp) {
			n, m := 0, 0
			for _, t := range items.Children(sp.ID) {
				switch State(t, items) {
				case "dropped": // not done, and no longer to do
				case "done":
					n, m = n+1, m+1
				default:
					m++
				}
			}
			title := esc(sp.Title)
			if r := sprintRecs[sp.ID]; r != nil {
				title = `<a href="` + r.Out() + `">` + esc(sp.Title) + "</a>"
			}
			count := fmt.Sprintf("%d of %d tasks done", n, m)
			if items.Unstarted(sp) {
				count = "dropped: " + esc(sp.CloseReason)
			}
			rows = append(rows, "<li>"+pill(State(sp, items))+" "+title+`<span class="k">`+count+"</span></li>")
		}
	}
	if hidden := len(done) - DoneSprintsShown; hidden > 0 {
		rows = append(rows, fmt.Sprintf(`<li><span class="k">%d older done sprints not shown; `+
			`<a href="%s">the project page</a> lists every sprint</span></li>`, hidden, p.Out()))
	}
	list := ""
	if rows != nil {
		list = `<h3>Sprints</h3><ul class="list">` + strings.Join(rows, "") + "</ul>"
	}
	return list + proposedSection(sprints, recs, items, nil)
}

func indexDesigns(p *Record, recs []*Record, items *Items, dates Dates) string {
	var designs []*Record
	for _, r := range recs {
		if r.Type() == "design" && records.ProjectOf(r, recs, items) == p {
			designs = append(designs, r)
		}
	}
	sort.SliceStable(designs, func(i, j int) bool { return designs[i].Title() < designs[j].Title() })
	if dates != nil {
		sort.SliceStable(designs, func(i, j int) bool { return dates[designs[i].Rel].Updated > dates[designs[j].Rel].Updated })
	}
	if designs != nil {
		var lis strings.Builder
		for _, r := range designs {
			lis.WriteString(`<li><a href="` + r.Out() + `">` + esc(r.Title()) + `</a><span class="k">` +
				designDatesMeta(r, dates) + "</span></li>")
		}
		return `<h3>Design pages</h3><ul class="list">` + lis.String() + "</ul>"
	}
	return ""
}

// Cites says whether text names the id as a whole id, not as part of a longer one: Python's
// (?<![\w.-])<id>(?![\w-]|\.\w), whose lookarounds RE2 cannot express.
func Cites(text, id string) bool {
	if id == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(text[from:], id)
		if i < 0 {
			return false
		}
		i += from
		from = i + 1
		if i > 0 {
			prev := lastRune(text[:i])
			if isWord(prev) || prev == '.' || prev == '-' {
				continue
			}
		}
		rest := text[i+len(id):]
		if rest != "" {
			next := firstRune(rest)
			if isWord(next) || next == '-' {
				continue
			}
			if next == '.' && len(rest) > 1 && isWord(firstRune(rest[1:])) {
				continue
			}
		}
		return true
	}
}

// CheckNeedsAnswered fails on an answered decision need (a closed need inside a project, not an action, not
// dismissed, not marked no-decision) that no decision in a project or sprint record cites: a rule the owner set lives
// in the records. ids, when not nil, limits the check to those items.
func CheckNeedsAnswered(recs []*Record, items *Items, ids map[string]bool) error {
	epics := map[string]bool{}
	var bodies []string
	for _, r := range recs {
		if r.Type() == "project" {
			epics[r.Bead()] = true
		}
		if r.Type() == "project" || r.Type() == "sprint" {
			for _, d := range records.Decisions(r.Text) {
				bodies = append(bodies, d.Body)
			}
		}
	}
	all := append([]*Item{}, items.All()...)
	sort.SliceStable(all, func(i, j int) bool { return item.CompareIDs(all[i].ID, all[j].ID) < 0 })
	for _, it := range all {
		if ids != nil && !ids[it.ID] {
			continue
		}
		if it.Status != item.Closed || it.Type != item.Need || Kind(it) != "decision" ||
			it.Resolution == item.NoDecision || it.Resolution == item.Dismissed {
			continue
		}
		inside := false
		for _, a := range items.Ancestors(it.ID) {
			inside = inside || epics[a]
		}
		cited := false
		for _, b := range bodies {
			cited = cited || Cites(b, it.ID)
		}
		if inside && !cited {
			return errorf("need %s (%s) is closed but no decision cites it; record the owner's answer with pm "+
				"decision add --need %s, or, if the answer sets no rule, close it with pm need close %s --reason "+
				"\"<why>\"", it.ID, it.Title, it.ID, it.ID)
		}
	}
	return nil
}

// RenderPages is every page of the site by its path; an invalid record fails it. dates are the design pages' dates; a
// render that only validates leaves them out. A day page is rendered for every date with activity, with its summary.
func RenderPages(recs []*Record, items *Items, siteName string, dates Dates, summaries map[string]*records.Summary) (map[string]string, error) {
	pages, _, err := renderPages(recs, items, siteName, dates, summaries)
	return pages, err
}

// renderPages is RenderPages and each link of a record's text that names no record.
func renderPages(recs []*Record, items *Items, siteName string, dates Dates, summaries map[string]*records.Summary) (map[string]string, []string, error) {
	if err := CheckNeedsAnswered(recs, items, nil); err != nil {
		return nil, nil, err
	}
	recs = WithDays(recs, items, summaries)
	pages := map[string]string{}
	var broken []string
	for _, r := range recs {
		p, b, err := renderRecord(r, recs, items, dates)
		if err != nil {
			return nil, nil, err
		}
		pages[r.Out()] = p
		broken = append(broken, b...)
	}
	index, err := RenderIndex(recs, items, siteName, dates)
	if err != nil {
		return nil, nil, err
	}
	pages["index.html"] = index
	if pages[DocsIndex], err = RenderDocsIndex(recs, items); err != nil {
		return nil, nil, err
	}
	return pages, broken, nil
}

// Check is pm check's and pm commit's check: every page renders, every answered decision need is cited or marked
// no-decision, and every relative .md link of a record's text that stays in the store names a record (a dated one by
// its file name or its slug), and no record's raw HTML holds JSX or a blank line inside an <svg> (jsx.go). It writes
// nothing; it returns the number of pages. A link that names no record, and JSX, still render: the site shows the
// page, and only this check refuses it.
func Check(recs []*Record, items *Items, siteName string, summaries map[string]*records.Summary) (int, error) {
	pages, broken, err := renderPages(recs, items, siteName, nil, summaries)
	var once []string // a link a record gives twice is named once
	for _, b := range broken {
		if !slices.Contains(once, b) {
			once = append(once, b)
		}
	}
	var errs []string
	if once != nil {
		errs = append(errs, strings.Join(once, "\n"),
			"fix each link to name a record's file, or a doc's or postmortem's slug, or remove it")
	}
	for _, r := range recs {
		if r.Text != "" {
			errs = append(errs, jsxErrors(r.Rel, r.Body, strings.Count(r.Text[:len(r.Text)-len(r.Body)], "\n")+1)...)
		}
	}
	if err == nil && errs != nil {
		err = errorf("%s", strings.Join(errs, "\n"))
	}
	return len(pages), err
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

func firstRune(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }

func lastRune(s string) rune { r, _ := utf8.DecodeLastRuneInString(s); return r }
