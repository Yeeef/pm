package search

import (
	"regexp"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
)

// Unit is one thing a hit points at: a record section, a decision block, a task or a need.
type Unit struct {
	Kind    string // one of Kinds
	Date    string // YYYY-MM-DD, or "-" when the store has none
	State   string // the item's state (site.State), or "-"
	Address string // what pm show --record takes (the record, then the section, shell-quoted), or the item's id
	Project string // the name of the project record it belongs to; "" for none
	Text    string // what is indexed: the record's title, the heading and the body, or an item's texts
	snips   []string
	tokens  []string
}

// Kinds is the kinds of unit, as --kind takes them.
var Kinds = []string{"sprint", "project", "decision", "design", "doc", "postmortem", "task", "need"}

// skipped is the sprint and project sections that are no units: Progress is generated from the work store, and each
// decision block in Decisions is a unit of its own.
var skipped = map[string]bool{"Progress": true, "Decisions": true}

var datedName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// Units is every unit of the records and the items. dates holds the design pages' dates by rel (store.DesignDates).
func Units(recs []*records.Record, items []item.Item, dates map[string]store.Dates) []Unit {
	x := records.NewItems(items)
	named := map[string]int{} // how many sprint, project and design records have each name
	for _, rec := range recs {
		if t := rec.Type(); t == "sprint" || t == "project" || t == "design" {
			named[rec.Name()]++
		}
	}
	var out []Unit
	for _, rec := range recs {
		target := rec.Name() // pm show --record takes a sprint's, project's or design page's name when only one has it
		if t := rec.Type(); t == "doc" || t == "postmortem" || named[target] > 1 {
			target = rec.Rel
		}
		out = append(out, recordUnits(rec, target, recs, x, dates)...)
	}
	projects := map[string]string{} // a project item's id → its record's name
	for _, rec := range recs {
		if rec.Type() == "project" && rec.Bead() != "" {
			projects[rec.Bead()] = rec.Name()
		}
	}
	for _, it := range x.All() {
		if it.Type != item.Task && it.Type != item.Need {
			continue
		}
		project := ""
		for _, a := range x.Ancestors(it.ID) {
			if p, ok := projects[a]; ok {
				project = p
			}
		}
		texts := []string{it.Title, it.Description, it.CloseReason}
		out = append(out, Unit{Kind: string(it.Type), Date: itemDate(it), State: site.State(it, x), Address: it.ID,
			Project: project, Text: strings.Join(texts, "\n"), snips: texts})
	}
	for i := range out {
		out[i].tokens = Tokens(out[i].Text)
	}
	return out
}

// recordUnits is a record's units, none for a record of no unit kind (a day page); target is how pm show --record
// names the record.
func recordUnits(rec *records.Record, target string, recs []*records.Record, x *records.Items,
	dates map[string]store.Dates) []Unit {
	kind := rec.Type()
	date, state := "-", "-"
	switch kind {
	case "sprint", "project":
		if it := x.Get(rec.Bead()); it != nil {
			date, state = itemDate(it), site.State(it, x)
		}
	case "design":
		if d := dates[rec.Rel]; d.Updated != "" {
			date = d.Updated
		}
	case "doc", "postmortem":
		from := rec.Meta["date"].Str // a doc's header date; a postmortem's file name starts with its date
		if kind == "postmortem" {
			from = rec.Name()
		}
		if d := datedName.FindString(from); d != "" {
			date = d
		}
	default:
		return nil
	}
	project := ""
	if p := records.ProjectOf(rec, recs, x); p != nil {
		project = p.Name()
	}
	var out []Unit
	parts, headings := sections(rec.Body, kind)
	add := func(kind, date, heading, body string) {
		if body = clean(body); body == "" {
			return
		}
		address := records.ShellQuote(target)
		if heading != "" && headings[heading] == 1 { // pm show --section refuses a heading the record has twice
			address += " " + records.ShellQuote(heading)
		}
		out = append(out, Unit{Kind: kind, Date: date, State: state, Address: address, Project: project,
			Text: strings.Join([]string{rec.Title(), heading, body}, "\n"), snips: []string{body}})
	}
	for _, s := range parts {
		switch {
		case kind != "sprint" && kind != "project" || !skipped[s.heading]:
			add(kind, date, s.heading, s.body)
		case s.heading == "Decisions":
			for _, d := range records.Decisions(s.body) {
				day := d.Attrs["date"]
				if day == "" {
					day = "-"
				}
				add("decision", day, "Decisions", d.Body)
			}
		}
	}
	return out
}

// itemDate is an item's close date when it is closed, else its creation date.
func itemDate(it *item.Item) string {
	t := it.CreatedAt
	if it.Status == item.Closed && !it.ClosedAt.IsZero() {
		t = it.ClosedAt
	}
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.DateOnly)
}

// section is one part of a record body a unit may come from: its heading ("" for the text before the first heading
// that splits it) and its body.
type section struct{ heading, body string }

// sections splits a record body as its kind's units: at each ## heading; a design page's Design section also at each
// ### heading, its text before the first ### keeping the heading Design. The headings are the ones pm show --record
// parses (records.Headings), so a unit's address names a section it prints; with how many headings have each text.
func sections(body, kind string) ([]section, map[string]int) {
	lines := strings.Split(body, "\n")
	heads := records.Headings(body)
	count := map[string]int{}
	for _, h := range heads {
		count[h.Text]++
	}
	text := func(from, to int) string { return strings.Join(lines[from:to], "\n") }
	end := func(i int) int { // the line where heading i's section ends
		for _, h := range heads[i+1:] {
			if h.Level <= heads[i].Level {
				return h.Line
			}
		}
		return len(lines)
	}
	first := len(lines)
	for _, h := range heads {
		if h.Level == 2 {
			first = h.Line
			break
		}
	}
	out := []section{{"", text(0, first)}}
	for i, h := range heads {
		if h.Level != 2 {
			continue
		}
		if kind != "design" || h.Text != "Design" {
			out = append(out, section{h.Text, text(h.Line+1, end(i))})
			continue
		}
		lead := end(i)
		for _, s := range heads[i+1:] {
			if s.Line < end(i) && s.Level == 3 {
				lead = s.Line
				break
			}
		}
		out = append(out, section{h.Text, text(h.Line+1, lead)})
		for j, s := range heads[i+1:] {
			if s.Line < end(i) && s.Level == 3 {
				out = append(out, section{s.Text, text(s.Line+1, end(i+1+j))})
			}
		}
	}
	return out, count
}

// placeholders is the bodies of a section with nothing in it yet.
var placeholders = map[string]bool{records.NotClosed: true, records.NoneYet: true}

// clean is a section's body without its prompt lines and placeholder lines; "" when nothing but headings is left.
func clean(body string) string {
	var keep []string
	content := false
	for _, l := range strings.Split(records.StripPrompts(body), "\n") {
		t := strings.TrimSpace(l)
		if placeholders[t] {
			continue
		}
		if t != "" && !strings.HasPrefix(t, "#") {
			content = true
		}
		keep = append(keep, l)
	}
	if !content {
		return ""
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
