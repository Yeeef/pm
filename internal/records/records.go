// Package records parses and validates records: Markdown files with a YAML header under records/. A record's type
// decides its required header fields and sections; fenced-div blocks are limited to the names in Blocks; sections carry
// line ranges so a command can insert into a named section and leave the rest of the file untouched. Python source:
// records.py, and the record templates in cli.py.
package records

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/pyjson"
)

// Types is every record type in the order Python's REQUIRED lists them, which a refusal names.
var Types = []string{"project", "sprint", "day", "design", "doc", "postmortem"}

// Required is each type's required header fields; a doc also names exactly one of bead or project, a postmortem
// exactly one of sprint or project.
var Required = map[string][]string{
	"project":    {"title", "bead"},
	"sprint":     {"title", "bead"},
	"day":        {"date"},
	"design":     {"title", "project"},
	"doc":        {"title", "date"},
	"postmortem": {"title", "date"},
}

// Sections is each type's required sections. Day records are no longer written; older ones keep their hand-written
// ## Today. Prior art is the design template's one optional section, so it is not listed.
var Sections = map[string][]string{
	"project": {"## Goal", "## Progress", "## Decisions", "## Design pages", "## Outcome"},
	"sprint": {"## Goal", "## Scope", "## Done when", "## Design pages", "## Progress", "## Decisions", "## Findings",
		"## Delivery report"},
	"day": {},
	"design": {"## Problem", "## Goals and non-goals", "## Constraints and key facts", "## Design",
		"## Alternatives considered", "## Open questions"},
	"postmortem": {"## Summary", "## Timeline", "## Cost", "## Root cause", "## What changed",
		"## What would have caught it earlier"},
}

// GeneratedBody is the sections the renderer fills: a record keeps them empty, prompt lines only.
var GeneratedBody = map[string][]string{"project": {"Progress"}, "sprint": {"Progress"}}

// GeneratedHeadings is the headings the renderer adds; a record never holds one, since its text would sit beside or
// under the generated content.
var GeneratedHeadings = map[string][]string{
	"project": {"Docs", "Postmortems"},
	"sprint":  {"Decisions await you", "Actions await you", "Docs", "Postmortems"},
	"day":     {"Decisions await you", "Actions await you", "Sprints", "Docs"},
}

const (
	NotClosed = "Not closed yet." // placeholder body of a close-time section
	NoneYet   = "None yet."       // placeholder body of an empty list section
)

// ReportParts is the delivery report's subsections.
var ReportParts = []string{"Outcome", `Against "Done when"`}

// Blocks is the fenced-div blocks a record may use, and BlockAttrs the attributes each requires.
var (
	Blocks     = []string{"decision", "note", "result"} // sorted, as a refusal lists them
	BlockAttrs = map[string][]string{"decision": {"source", "date"}, "result": {"title"}}
)

// Error is a record that does not parse or validate: Python's RecordError.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func errorf(format string, a ...any) *Error { return &Error{fmt.Sprintf(format, a...)} }

// Record is one record file, or a day page generated without one.
type Record struct {
	Path    string   // the resolved file path
	Rel     string   // path under records/, without .md
	Meta    Meta     // the header
	Body    string   // the text after the header
	Text    string   // the whole file; empty for a day page generated without a file
	Summary *Summary // a day's generated summary, set by site.WithDays
}

// Out is the page's path under the site.
func (r *Record) Out() string { return r.Rel + ".html" }

// Type is the record's type.
func (r *Record) Type() string { return r.Meta["type"].Str }

// Title is the title, else the date, else the path: Python's str(meta.get("title") or meta.get("date") or rel).
func (r *Record) Title() string {
	for _, k := range []string{"title", "date"} {
		if v, ok := r.Meta[k]; ok && v.Truthy {
			return v.Str
		}
	}
	return r.Rel
}

// Name is the file name without .md: the project name of a project record.
func (r *Record) Name() string { return r.Rel[strings.LastIndex(r.Rel, "/")+1:] }

// ID is a header field that names an item (bead, sprint): its text when the header gives a string, else "" with false,
// since a typed value never equals an item id.
func (r *Record) ID(key string) (string, bool) {
	v, ok := r.Meta[key]
	if !ok || v.Kind != KindStr {
		return "", false
	}
	return v.Str, true
}

// Bead is the bead field's id; ""when the header has none or a typed one.
func (r *Record) Bead() string { id, _ := r.ID("bead"); return id }

// Has says whether the header names the key.
func (r *Record) Has(key string) bool { _, ok := r.Meta[key]; return ok }

// Attrs is a block's attributes from its info string: key=value or key="value with spaces".
func Attrs(info string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRE.FindAllStringSubmatchIndex(info, -1) {
		key := info[m[2]:m[3]]
		if m[6] >= 0 {
			out[key] = info[m[6]:m[7]]
		} else {
			out[key] = info[m[4]:m[5]]
		}
	}
	return out
}

// Python's \w, \s and \d match Unicode; Go's match ASCII. These classes are Python's for the characters records hold.
const (
	pyW = `[\p{L}\p{N}_]`
	pyS = `[\t\n\v\f\r \x1c-\x1f\x85\p{Z}]`

	// PyS is Python's \s for other packages' patterns.
	PyS = pyS
)

var (
	attrRE     = regexp.MustCompile(`(` + pyW + `+)=("([^"]*)"|[^\t\n\v\f\r \x1c-\x1f\x85\p{Z}}]+)`)
	decisionRE = regexp.MustCompile(`(?ms)^::: decision(.*?)\n(.*?)^:::` + pyS + `*$`)
	blockRE    = regexp.MustCompile(`^:::` + pyS + `*(` + pyW + `+)(.*)$`)
	dayRE      = regexp.MustCompile(`^\p{Nd}{4}-\p{Nd}{2}-\p{Nd}{2}$`)
)

// Decision is one ::: decision block: its attributes and its body.
type Decision struct {
	Attrs map[string]string
	Body  string
}

// Decisions is every ::: decision block in a record's text.
func Decisions(text string) []Decision {
	var out []Decision
	for _, m := range decisionRE.FindAllStringSubmatch(text, -1) {
		out = append(out, Decision{Attrs(m[1]), m[2]})
	}
	return out
}

// CheckBlocks fails on a ::: line naming an unknown block or missing a required attribute; it reads every line, those
// in code fences included.
func CheckBlocks(text, rel string) error {
	for _, line := range SplitLines(text) {
		m := blockRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !contains(Blocks, m[1]) {
			return errorf("%s: unknown block '::: %s' (allowed: %s)", rel, m[1], strings.Join(Blocks, ", "))
		}
		a := Attrs(m[2])
		for _, req := range BlockAttrs[m[1]] {
			if _, ok := a[req]; !ok {
				return errorf("%s: ::: %s needs '%s'", rel, m[1], req)
			}
		}
	}
	return nil
}

// dated is the folder of a doc and a postmortem, and the key it names besides project.
var dated = map[string][2]string{"doc": {"docs", "bead"}, "postmortem": {"postmortems", "sprint"}}

func checkDatedHeader(meta Meta, rel string) error {
	kind := meta["type"].Str
	folder, key := dated[kind][0], dated[kind][1]
	_, hasKey := meta[key]
	_, hasProject := meta["project"]
	if hasKey == hasProject {
		return errorf("%s: a %s names exactly one of '%s' or 'project' in its header", rel, kind, key)
	}
	day := meta["date"].Str
	if !dayRE.MatchString(day) {
		return errorf("%s: date must be YYYY-MM-DD, got %s", rel, pyjson.StrRepr(day))
	}
	if !regexp.MustCompile(`^` + folder + `/` + day + `-[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(rel) {
		return errorf("%s: a %s dated %s lives at %s/%s-<slug>.md (slug: lowercase words joined by '-')", rel, kind,
			day, folder, day)
	}
	return nil
}

// header is the YAML header's text and where the body starts: Python's HEADER_RE, ---\n(.*?)\n---\n at the start.
func header(text string) (string, int, bool) {
	if !strings.HasPrefix(text, "---\n") {
		return "", 0, false
	}
	i := strings.Index(text[4:], "\n---\n")
	if i < 0 {
		return "", 0, false
	}
	return text[4 : 4+i], 4 + i + 5, true
}

// Parse parses one record file and checks its header and blocks.
func Parse(path, rel, text string) (*Record, error) {
	head, end, ok := header(text)
	if !ok {
		return nil, errorf("%s: missing YAML header", rel)
	}
	meta, err := parseHeader(head, rel)
	if err != nil {
		return nil, err
	}
	kind, ok := meta["type"]
	if _, known := Required[kind.Str]; !ok || kind.Kind != KindStr || !known {
		got := "None"
		if ok {
			got = kind.Repr()
		}
		return nil, errorf("%s: type must be one of %s, got %s", rel, strings.Join(Types, ", "), got)
	}
	var missing []string
	for _, k := range Required[kind.Str] {
		if !meta[k].Truthy {
			missing = append(missing, k)
		}
	}
	if missing != nil {
		return nil, errorf("%s: missing header fields: %s", rel, strings.Join(missing, ", "))
	}
	if _, ok := dated[kind.Str]; ok {
		if err := checkDatedHeader(meta, rel); err != nil {
			return nil, err
		}
	}
	if err := CheckBlocks(text, rel); err != nil {
		return nil, err
	}
	return &Record{Path: path, Rel: rel, Meta: meta, Body: text[end:], Text: text}, nil
}

// Texts is the text of every .md file under root, by resolved path.
func Texts(root string) (map[string]string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(d.Name(), ".md") || d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[p] = string(b)
		return nil
	})
	return out, err
}

// ParseAll is the records of texts (resolved path to text, as Texts gives), in path order: by path parts, as Python
// sorts Path objects, so "a/b.md" comes before "a-c.md".
func ParseAll(root string, texts map[string]string) ([]*Record, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(texts))
	for p := range texts {
		paths = append(paths, p)
	}
	SortPaths(paths)
	out := make([]*Record, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("%s is not under %s", p, root)
		}
		rec, err := Parse(p, strings.TrimSuffix(filepath.ToSlash(rel), ".md"), texts[p])
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// SortPaths sorts slash paths by their parts, as Python orders Path objects.
func SortPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool { return lessParts(paths[i], paths[j]) })
}

func lessParts(a, b string) bool {
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// Read is every record under root; overrides maps a path to planned text, existing or new.
func Read(root string, overrides map[string]string) ([]*Record, error) {
	texts, err := Texts(root)
	if err != nil {
		return nil, err
	}
	for p, t := range overrides {
		abs, err := resolve(p)
		if err != nil {
			return nil, err
		}
		texts[abs] = t
	}
	return ParseAll(root, texts)
}

// resolve is Python's Path.resolve() for a path that may not exist yet: its existing parent resolved.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r, nil
	}
	dir, err := resolve(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(abs)), nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
