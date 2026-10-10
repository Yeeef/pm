package records

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/work"
)

// Items is the work store's items by id, in the order the store gave them: what records and pages read status from.
type Items struct {
	byID  map[string]*work.Item
	order []*work.Item
}

// NewItems indexes items.
func NewItems(items []work.Item) *Items {
	x := &Items{byID: map[string]*work.Item{}}
	for i := range items {
		it := &items[i]
		x.byID[it.ID] = it
		x.order = append(x.order, it)
	}
	return x
}

// Get is the item with the id, or nil.
func (x *Items) Get(id string) *work.Item { return x.byID[id] }

// All is every item, in the store's order.
func (x *Items) All() []*work.Item { return x.order }

// Parent is the item's parent id; "" for an unknown item or one without a parent.
func (x *Items) Parent(id string) string {
	if it := x.byID[id]; it != nil {
		return it.Parent
	}
	return ""
}

// Ancestors is the item's parent, grandparent, …, nearest first.
func (x *Items) Ancestors(id string) []string {
	var out []string
	for p := x.Parent(id); p != "" && !contains(out, p); p = x.Parent(p) {
		out = append(out, p)
	}
	return out
}

// Children is the items whose parent is the id, in id order (work.CompareIDs).
func (x *Items) Children(parent string) []*work.Item {
	var out []*work.Item
	for _, it := range x.order {
		if it.Parent == parent {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return work.CompareIDs(out[i].ID, out[j].ID) < 0 })
	return out
}

// ProjectOf is the project record a record belongs to: by the item tree for sprints and docs with a bead, by its
// sprint for a postmortem with one, by `project:` otherwise; nil when there is none.
func ProjectOf(rec *Record, recs []*Record, items *Items) *Record {
	var projects []*Record
	for _, r := range recs {
		if r.Type() == "project" {
			projects = append(projects, r)
		}
	}
	byBead := func(id string) *Record {
		for _, p := range projects {
			if b, ok := p.ID("bead"); ok && b == id {
				return p
			}
		}
		return nil
	}
	switch t := rec.Type(); {
	case t == "project":
		return rec
	case t == "sprint":
		bead, ok := rec.ID("bead")
		if !ok || items.Get(bead) == nil || items.Parent(bead) == "" {
			return nil
		}
		return byBead(items.Parent(bead))
	case t == "doc" && rec.Has("bead"):
		bead, ok := rec.ID("bead")
		if !ok {
			return nil
		}
		for _, b := range append([]string{bead}, items.Ancestors(bead)...) {
			if p := byBead(b); p != nil {
				return p
			}
		}
		return nil
	case t == "postmortem" && rec.Has("sprint"):
		sprint := SprintRecord(recs, rec.Meta["sprint"])
		if sprint == nil {
			return nil
		}
		return ProjectOf(sprint, recs, items)
	}
	name, ok := rec.ID("project")
	if !ok {
		return nil
	}
	for _, p := range projects {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// SprintRecord is the first sprint record whose bead equals the value, or nil.
func SprintRecord(recs []*Record, bead Value) *Record {
	for _, r := range recs {
		if v, ok := r.Meta["bead"]; ok && r.Type() == "sprint" && v.Kind == bead.Kind && v.Str == bead.Str {
			return r
		}
	}
	return nil
}

// CheckGenerated fails on hand-written text in a generated section: a non-prompt line in Progress, or a heading the
// renderer adds itself.
func CheckGenerated(rec *Record) error {
	lines := strings.Split(rec.Body, "\n")
	line0 := strings.Count(rec.Text, "\n") - strings.Count(rec.Body, "\n") // the file line of the body's first, 0-based
	for _, s := range SectionRanges(rec.Body, 2) {
		if contains(GeneratedBody[rec.Type()], s.Name) {
			for n := s.Start + 1; n < s.End; n++ {
				if Strip(lines[n]) != "" && !strings.HasPrefix(lines[n], ">") {
					return errorf("%s.md:%d: hand-written text in '## %s', which is generated from the work store "+
						"when the page is rendered; move it to Findings or Decisions, or remove it", rec.Rel, line0+n+1, s.Name)
				}
			}
		}
		if contains(GeneratedHeadings[rec.Type()], s.Name) {
			return errorf("%s.md:%d: '## %s' is a section the page generates; remove the heading and move its text "+
				"to a section written by hand", rec.Rel, line0+s.Start+1, s.Name)
		}
	}
	return nil
}

// Validate checks a record against the others and the items: its bead exists, it has a project, its sections are
// there, and it writes nothing in a generated section.
func Validate(rec *Record, recs []*Record, items *Items) error {
	t := rec.Type()
	if (t == "project" || t == "sprint" || t == "doc") && rec.Has("bead") {
		if bead, ok := rec.ID("bead"); !ok || items.Get(bead) == nil {
			return errorf("%s: bead %s not found in the work store", rec.Rel, rec.Meta["bead"].Str)
		}
	}
	if t == "postmortem" && rec.Has("sprint") && SprintRecord(recs, rec.Meta["sprint"]) == nil {
		return errorf("%s: no sprint record has bead %s", rec.Rel, rec.Meta["sprint"].Str)
	}
	if t != "day" && ProjectOf(rec, recs, items) == nil { // days are repo-wide
		return errorf("%s: no project record found", rec.Rel)
	}
	for _, sec := range Sections[t] {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(sec) + `$`).MatchString(rec.Body) {
			return errorf("%s: %s record needs a '%s' section", rec.Rel, t, sec)
		}
	}
	return CheckGenerated(rec)
}

// Summary is a day's generated summary, as pm day summarize writes it.
type Summary struct {
	Date        string `json:"date"`
	GeneratedAt string `json:"generated_at"`
	Digest      string `json:"digest"`
	Text        string `json:"text"`
}

// SummaryPath is where pm day summarize stores a day's summary.
func SummaryPath(root, day string) string { return filepath.Join(root, "days", day+".summary.json") }

// ReadSummaries is every day summary under root, by date. A summary listed but gone when it is read (git rewrites a
// file by unlinking it) is left out, as it is no longer there.
func ReadSummaries(root string) (map[string]*Summary, error) {
	paths, err := filepath.Glob(filepath.Join(root, "days", "*.summary.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := map[string]*Summary{}
	for _, p := range paths {
		s, err := ReadSummary(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[strings.SplitN(filepath.Base(p), ".", 2)[0]] = s
	}
	return out, nil
}

// ReadSummary is one day summary; an error naming the file when it is not one, since every page reads them.
func ReadSummary(path string) (*Summary, error) {
	base := filepath.Base(path)
	name := "days/" + base
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := pyjson.Loads(string(b))
	if err != nil {
		return nil, errorf("%s: not valid JSON (%v); fix it or delete it and run pm day summarize", name, err)
	}
	o, ok := v.(*pyjson.Object)
	fields := map[string]string{}
	for _, k := range []string{"date", "generated_at", "digest", "text"} {
		s, isStr := any(nil), false
		if ok {
			s = o.Get(k)
			_, isStr = s.(string)
		}
		if !isStr {
			return nil, errorf("%s: a summary is a JSON object with string fields date, generated_at, digest and "+
				"text; fix it or delete it and run pm day summarize", name)
		}
		fields[k] = s.(string)
	}
	if day := strings.SplitN(base, ".", 2)[0]; fields["date"] != day {
		return nil, errorf("%s: its date is %s, not the date in its name", name, pyjson.StrRepr(fields["date"]))
	}
	if _, err := ParseISO(fields["generated_at"]); err != nil {
		return nil, errorf("%s: generated_at %s is not an ISO timestamp", name, pyjson.StrRepr(fields["generated_at"]))
	}
	return &Summary{fields["date"], fields["generated_at"], fields["digest"], fields["text"]}, nil
}

var isoRE = regexp.MustCompile(`^(\d{4})-?(\d{2})-?(\d{2})(?:[T ](\d{2})(?::?(\d{2})(?::?(\d{2})(?:[.,](\d{1,9}))?)?)?` +
	`(Z|[+-]\d{2}(?::?\d{2}(?::?\d{2})?)?)?)?$`)

// ParseISO is Python's datetime.fromisoformat() for the forms pm and its summaries write: a date, or a date and a time
// with an optional fraction and offset. A time without an offset is local, as Python's astimezone() reads a naive one.
func ParseISO(s string) (time.Time, error) {
	m := isoRE.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("Invalid isoformat string: %s", pyjson.StrRepr(s))
	}
	layout, value := "2006-01-02", m[1]+"-"+m[2]+"-"+m[3]
	if m[4] != "" {
		layout, value = layout+"T15", value+"T"+m[4]
		if m[5] != "" {
			layout, value = layout+":04", value+":"+m[5]
		}
		if m[6] != "" {
			layout, value = layout+":05", value+":"+m[6]
		}
		if m[7] != "" {
			layout, value = layout+".000000000", value+"."+(m[7] + "000000000")[:9]
		}
	}
	loc := time.Local
	if tz := m[8]; tz != "" {
		if tz == "Z" {
			loc = time.UTC
		} else {
			digits := strings.ReplaceAll(tz[1:], ":", "")
			secs := atoiPad(digits[0:2])*3600 + atoiPad(digits[2:min(4, len(digits))])*60
			if len(digits) > 4 {
				secs += atoiPad(digits[4:])
			}
			if tz[0] == '-' {
				secs = -secs
			}
			loc = time.FixedZone("", secs)
		}
	}
	t, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("Invalid isoformat string: %s", pyjson.StrRepr(s))
	}
	return t, nil
}

func atoiPad(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
