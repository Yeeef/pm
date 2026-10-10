// Package site renders records joined with live work-store status: the pages the pm service serves and pm check
// renders. Page assembly uses html/template, and fragments are escaped with the standard library's html.EscapeString;
// golden_test.go holds every page of testdata/constructs to its frozen copy.
package site

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	pm "github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// Style is the one stylesheet every page gets.
var Style = pm.Style

type (
	Record = records.Record
	Items  = records.Items
	Item   = work.Item
)

// Dates is the design pages' dates by rel; nil for a render that only validates, whose pages show none.
type Dates = map[string]store.Dates

func errorf(format string, a ...any) error { return &records.Error{Msg: fmt.Sprintf(format, a...)} }

var pills = map[string][2]string{"done": {"done", "DONE"}, "running": {"run", "RUNNING"},
	"blocked": {"blocked", "BLOCKED"}, "ready": {"queued", "READY"}}

func pill(st string) string {
	p := pills[st]
	return `<span class="pill ` + p[0] + `">` + p[1] + "</span>"
}

// ---------------------------------------------------------------- item status

func isEpic(it *Item) bool { return it.Type == work.Project || it.Type == work.Sprint }

// State is one of done, running, blocked and ready: done once closed, running while held or, for a project or sprint,
// once any child has started or finished; blocked while a blocker is not closed.
func State(it *Item, items *Items) string {
	switch {
	case it.Status == work.Closed:
		return "done"
	case it.Holder != nil:
		return "running"
	}
	if isEpic(it) {
		for _, k := range items.All() {
			if k.Parent == it.ID && (k.Holder != nil || k.Status == work.Closed) {
				return "running"
			}
		}
	}
	for _, b := range it.BlockedBy {
		if x := items.Get(b); x == nil || x.Status != work.Closed {
			return "blocked"
		}
	}
	return "ready"
}

// Kind is what a need waits for: an action by the owner (a PR review is one), else a decision.
func Kind(it *Item) string {
	if it.Need != nil && (it.Need.Kind == work.Action || it.Need.Kind == work.Review) {
		return "action"
	}
	return "decision"
}

// OwnerTasks is the open needs under a project item: what awaits the owner, newest first.
func OwnerTasks(items *Items, under string) []*Item {
	var out []*Item
	for _, it := range items.All() {
		if it.Status != work.Closed && it.Type == work.Need && contains(items.Ancestors(it.ID), under) {
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
	return childrenWhere(items, project, func(it *Item) bool { return it.Type == work.Sprint })
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
	sort.SliceStable(sorted, func(i, j int) bool { return work.CompareIDs(sorted[i].ID, sorted[j].ID) < 0 })
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
		"  classDef running fill:#e3edf8,stroke:#1d5fa8,color:#1d2321",
		"  classDef ready fill:#f6efd9,stroke:#8a6a12,color:#1d2321",
		"  classDef blocked fill:#f7e3e1,stroke:#a8322d,color:#1d2321")
	legend := `<p class="meta">` + pill("done") + " " + pill("running") + " " + pill("ready") + " " + pill("blocked") +
		"<span>arrows: must finish first</span></p>\n"
	return `<pre class="mermaid">` + esc(strings.Join(lines, "\n")) + "</pre>\n" + legend
}

// inline is one line of Markdown, with .md links pointing at rendered pages.
func inline(text string) string {
	return mdHref.ReplaceAllString(renderInline(inlineHTML, text), `href="$1.html"`)
}

func progress(project *Record, recs []*Record, items *Items, frm *Record) (string, error) {
	root := project.Bead()
	graph := TaskGraph(childrenWhere(items, root, nil), items)
	var sprints []*Record
	for _, r := range recs {
		if r.Type() == "sprint" && records.ProjectOf(r, recs, items) == project {
			sprints = append(sprints, r)
		}
	}
	sort.SliceStable(sprints, func(i, j int) bool { return work.CompareIDs(sprints[i].Bead(), sprints[j].Bead()) < 0 })
	var rows []string
	for _, r := range sprints {
		b := items.Get(r.Bead())
		if b == nil {
			return "", errorf("%s: bead %s not found in the work store", r.Rel, r.Bead())
		}
		report, err := records.Outcome(r)
		if err != nil {
			return "", err
		}
		out := "<span class=empty>not closed</span>"
		if report != "" {
			out = inline(report)
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
	return graph + table, nil
}

type unsprintedItem struct {
	it      *Item
	project *Record
}

// unsprinted is each open task or need filed directly under a project epic or with no parent, with its project (nil
// for none), by id. A need under a project is left out: the await-you sections show it.
func unsprinted(recs []*Record, items *Items) []unsprintedItem {
	projects := map[string]*Record{}
	for _, r := range recs {
		if r.Type() == "project" {
			projects[r.Bead()] = r
		}
	}
	var out []unsprintedItem
	for _, it := range items.All() {
		if it.Status == work.Closed || isEpic(it) {
			continue
		}
		if p := projects[it.Parent]; it.Parent == "" || (p != nil && it.Type != work.Need) {
			out = append(out, unsprintedItem{it, p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return work.CompareIDs(out[i].it.ID, out[j].it.ID) < 0 })
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
	sorted := append([]*Record{}, docs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		di, dj := sorted[i].Meta["date"].Str, sorted[j].Meta["date"].Str
		if di != dj {
			return di > dj
		}
		return sorted[i].Rel > sorted[j].Rel
	})
	var lis strings.Builder
	for _, d := range sorted {
		lis.WriteString(`<li><a href="` + link(frm, d.Out()) + `">` + esc(d.Title()) + "</a>" +
			`<span class="k">` + esc(d.Meta["date"].Str) + "</span></li>")
	}
	return "<" + heading + ` id="` + strings.ToLower(label) + `">` + label + "</" + heading + `><ul class="list">` +
		lis.String() + "</ul>\n"
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
		if LocalDay(sp.CreatedAt) == day {
			rows = append(rows, `<li><span class="pill queued">OPENED</span> this sprint</li>`)
		}
		moved := false
		for _, t := range items.Children(sp.ID) {
			for _, v := range dayVerbs {
				if LocalDay(v.at(t)) == day {
					rows = append(rows, `<li><span class="pill `+v.class+`">`+strings.ToUpper(v.verb)+"</span> "+
						esc(t.Title)+` <span class="k">`+esc(t.ID)+"</span></li>")
					moved = true
					break
				}
			}
		}
		if moved || LocalDay(sp.CreatedAt) == day {
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

var designLink = regexp.MustCompile(`\(\.\./design/([a-z0-9-]+)\.md[)#]`)

func reviewDesigns(review *work.ReviewInfo, recs []*Record) []*Record {
	slugs := append([]string{}, review.Designs...)
	for _, r := range recs {
		if b, ok := r.ID("bead"); ok && r.Type() == "sprint" && contains(review.Sprints, b) {
			for _, m := range designLink.FindAllStringSubmatch(records.SectionText(r.Body, "Design pages"), -1) {
				slugs = append(slugs, m[1])
			}
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

func reviewContext(review *work.ReviewInfo, items *Items, recs []*Record, frm *Record, md func(string) (string, error)) (string, error) {
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

// requestPlace is the sprint and the task a request sits under in the project, from its parent chain: the sprint is
// the ancestor just below the project, the task the request's own parent when that is below the sprint.
func requestPlace(it *Item, items *Items, project string) (sprint, task string) {
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
	sid, tid := requestPlace(it, items, proj.Bead())
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

// NotDelivered is a site reply's state before it reaches the asking session.
const NotDelivered = "not delivered yet: the session that asked is not running, or the push failed and the pm service " +
	"tries again; the next session sees it"

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
		if c.Kind == work.Reply {
			author = "you, on the site"
			if n < seen {
				status = ` · <span class="picked">delivered to the agent&#x27;s session</span>`
			} else {
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
	if it.Need != nil && it.Need.Review != nil {
		desc, err = reviewContext(it.Need.Review, items, recs, frm, md)
	} else {
		desc, err = md(it.Description)
		if desc == "" {
			desc = `<p class="empty">No details given.</p>`
		}
	}
	if err != nil {
		return "", err
	}
	how := "Reply below with the evidence once done; the agent checks it and closes this."
	if Kind(it) == "decision" {
		how = "Reply below, or answer with <code>pm reply add " + esc(it.ID) + "</code>; the agent that asked " +
			"records your answer."
	}
	return `<div class="card need" id="need-` + esc(it.ID) + `"><h4>` + pill(State(it, items)) + esc(it.Title) +
		`</h4><p class="k">` + where + " · " + esc(it.ID) + "</p>" + desc + `<p class="k">` + how + "</p>" + thread(it) +
		ReplySlot(it.ID, Kind(it)) + "</div>", nil
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
			which := "decision needs"
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
<nav class="topbar"><a href="{{.Home}}">Home</a>{{.Crumbs}}</nav>
<div class="eyebrow">{{.Kind}}</div>
<h1>{{.Title}}</h1>
{{.Body}}
</div>{{.Mermaid}}</body></html>
`))

const mermaidScript = `
<script type="module">
import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";
const dark = matchMedia("(prefers-color-scheme: dark)").matches;
mermaid.initialize({ startOnLoad: true, theme: dark ? "dark" : "default" });
</script>
`

func page(rec *Record, kind, title, body string, c *ctx, crumbs string) (string, error) {
	mermaid := ""
	if c.mermaid {
		mermaid = mermaidScript
	}
	var b bytes.Buffer
	err := pageTemplate.Execute(&b, map[string]any{
		"Status": template.HTML(StatusSlot), "Title": title, "CSS": link(rec, "style.css"), "Home": link(rec, "index.html"),
		"Crumbs": template.HTML(crumbs), "Kind": kind, "Body": template.HTML(body), "Mermaid": template.HTML(mermaid),
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

var (
	progressRE  = regexp.MustCompile(`(## Progress\n(?:\n?>.*\n)*)`)
	todayRE     = regexp.MustCompile(`(## Today\n(?:\n?>.*\n)*)`)
	decisionsRE = regexp.MustCompile(`(?m)^## Decisions$`)
)

// RenderRecord is one record's page.
func RenderRecord(rec *Record, recs []*Record, items *Items, dates Dates) (string, error) {
	c := &ctx{rel: rec.Rel}
	md := recordMD(c)
	render := func(s string) (string, error) { return render(md, s) }
	m := rec.Meta
	if err := records.Validate(rec, recs, items); err != nil {
		return "", err
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
	case "sprint":
		b := items.Get(rec.Bead())
		pre = `<p class="meta">` + pill(State(b, items)) + "<span>" + esc(rec.Bead()) + "</span>" +
			"<span>opened " + esc(utcDate(b.CreatedAt)) + "</span></p>"
		c.mermaid = true
		generated := TaskGraph([]*Item{b}, items)
		body = replaceFirst(progressRE, body, func(g string) string { return g + "\n" + generated + "\n" })
		var cards []card
		for _, it := range OwnerTasks(items, rec.Bead()) {
			h, err := ownerCard(it, cardWhere(it, proj, items, recs, rec), items, render, recs, rec)
			if err != nil {
				return "", err
			}
			cards = append(cards, card{it, h})
		}
		needs := awaiting(cards, true)
		body = replaceFirst(decisionsRE, body, func(l string) string { return needs + "\n\n" + l })
		body = withDocs(body, "## Delivery report", sprintDocs(rec, recs, items), sprintPostmortems(rec, recs), rec)
	case "day":
		day := m["date"].Str
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
			pre = `<p class="meta">` + designDatesMeta(rec, dates) + "</p>"
		}
	}
	source := []byte(skipFrontMatter(body))
	doc := md.Parser().Parse(text.NewReader(source))
	var content bytes.Buffer
	if err := md.Renderer().Render(&content, source, doc); err != nil {
		return "", err
	}
	nav := ""
	if rec.Type() == "design" {
		nav = toc(doc, source)
	}
	return page(rec, rec.Type(), rec.Title(), pre+nav+content.String(), c, crumbs)
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

// RenderIndex is the root page: the overview of everything open, across all projects. Design pages are listed by
// last update, newest first (by title within a day), with both dates; without dates, by title.
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
	out := []string{awaiting(cards, false)}

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
		out = append(out, `<h2 id="days">Days</h2><ul class="list">`+lis.String()+"</ul>")
	}
	out = append(out, unsprintedTable(unsprinted(recs, items), nil, "h2"))

	for _, p := range projects {
		out = append(out, `<h2><a href="`+p.Out()+`">`+esc(p.Title())+"</a></h2>")
		sprintRecs := map[string]*Record{}
		for _, r := range recs {
			if b, ok := r.ID("bead"); ok && r.Type() == "sprint" {
				sprintRecs[b] = r
			}
		}
		sprints := sprintsOf(items, p.Bead())
		var done []*Item
		for _, sp := range sprints {
			if sp.Status == work.Closed {
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
			if sp.Status != work.Closed || shown[sp.ID] {
				tasks := items.Children(sp.ID)
				n := 0
				for _, t := range tasks {
					if t.Status == work.Closed {
						n++
					}
				}
				title := esc(sp.Title)
				if r := sprintRecs[sp.ID]; r != nil {
					title = `<a href="` + r.Out() + `">` + esc(sp.Title) + "</a>"
				}
				rows = append(rows, "<li>"+pill(State(sp, items))+" "+title+
					fmt.Sprintf(`<span class="k">%d of %d tasks done</span></li>`, n, len(tasks)))
			}
		}
		if hidden := len(done) - DoneSprintsShown; hidden > 0 {
			rows = append(rows, fmt.Sprintf(`<li><span class="k">%d older done sprints not shown; `+
				`<a href="%s">the project page</a> lists every sprint</span></li>`, hidden, p.Out()))
		}
		if rows != nil {
			out = append(out, `<h3>Sprints</h3><ul class="list">`+strings.Join(rows, "")+"</ul>")
		}
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
			out = append(out, `<h3>Design pages</h3><ul class="list">`+lis.String()+"</ul>")
		}
		out = append(out, docList(projectDocs(p, recs, items, "doc"), nil, "h3", "Docs"))
		out = append(out, docList(projectDocs(p, recs, items, "postmortem"), nil, "h3", "Postmortems"))
	}
	return page(nil, "overview", siteName, strings.Join(out, ""), c, "")
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
	sort.SliceStable(all, func(i, j int) bool { return work.CompareIDs(all[i].ID, all[j].ID) < 0 })
	for _, it := range all {
		if ids != nil && !ids[it.ID] {
			continue
		}
		if it.Status != work.Closed || it.Type != work.Need || Kind(it) != "decision" ||
			it.Resolution == work.NoDecision || it.Resolution == work.Dismissed {
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
				"decision add --need %s, or, if the answer sets no rule, close it with pm decision close %s --reason "+
				"\"<why>\"", it.ID, it.Title, it.ID, it.ID)
		}
	}
	return nil
}

// RenderPages is every page of the site by its path; an invalid record fails it. dates are the design pages' dates; a
// render that only validates leaves them out. A day page is rendered for every date with activity, with its summary.
func RenderPages(recs []*Record, items *Items, siteName string, dates Dates, summaries map[string]*records.Summary) (map[string]string, error) {
	if err := CheckNeedsAnswered(recs, items, nil); err != nil {
		return nil, err
	}
	recs = WithDays(recs, items, summaries)
	pages := map[string]string{}
	for _, r := range recs {
		p, err := RenderRecord(r, recs, items, dates)
		if err != nil {
			return nil, err
		}
		pages[r.Out()] = p
	}
	index, err := RenderIndex(recs, items, siteName, dates)
	if err != nil {
		return nil, err
	}
	pages["index.html"] = index
	return pages, nil
}

// Check is pm check's and pm commit's check: every page renders and every answered decision need is cited or marked
// no-decision. It writes nothing; it returns the number of pages.
func Check(recs []*Record, items *Items, siteName string, summaries map[string]*records.Summary) (int, error) {
	pages, err := RenderPages(recs, items, siteName, nil, summaries)
	return len(pages), err
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

func firstRune(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }

func lastRune(s string) rune { r, _ := utf8.DecodeLastRuneInString(s); return r }
