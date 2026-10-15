package records

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Rename is a design page's or a doc's rename, as pm design rename and pm doc rename plan it: the record, its new name
// ("" keeps it; a doc's date prefix is kept either way), its new title ("" keeps it), and an extra rewrite of every
// record's text, or nil.
type Rename struct {
	From    *Record
	To      string
	Title   string
	Rewrite func(text string) string
}

// Change is one file a rename writes: its path and new text, or its removal.
type Change struct {
	Path, Text string
	Remove     bool
}

var (
	docDate = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-`) // the <date>- a doc's name starts with
	slug    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)     // a new name: lowercase words joined by '-'
)

// kindNoun is how a rename's messages name the record's kind.
func kindNoun(rec *Record) string {
	if rec.Type() == "design" {
		return "design page"
	}
	return "doc"
}

// RenameTarget is the record a rename names: a design page (kind "design") by its slug, or a doc (kind "doc") by its
// path (records/ and .md optional), its <date>-<slug>, or its slug alone when one doc has it.
func RenameTarget(recs []*Record, kind, arg string) (*Record, error) {
	name := strings.TrimSuffix(strings.TrimPrefix(arg, "records/"), ".md")
	name = strings.TrimPrefix(name, map[string]string{"design": "design/", "doc": "docs/"}[kind])
	var found, all []string
	var rec *Record
	for _, x := range recs {
		if x.Type() != kind {
			continue
		}
		all = append(all, x.Name())
		if x.Name() == name || kind == "doc" && docDate.ReplaceAllString(x.Name(), "") == name {
			found, rec = append(found, "records/"+x.Rel+".md"), x
		}
	}
	switch {
	case len(found) > 1:
		return nil, fmt.Errorf("%s names %d docs: %s; give the doc's path", strconv.Quote(arg), len(found),
			strings.Join(found, ", "))
	case rec == nil && kind == "design":
		return nil, fmt.Errorf("no design page %s; known: %s", strconv.Quote(arg), strings.Join(all, ", "))
	case rec == nil:
		return nil, fmt.Errorf("no doc %s under records/docs", strconv.Quote(arg))
	case IsFeedbackDoc(rec):
		return nil, fmt.Errorf("records/%s.md is the repo's feedback doc, which pm feedback add writes at that path; "+
			"it is not renamed", rec.Rel)
	}
	return rec, nil
}

// Plan is the rename's changes, the renamed record first at its new path with its new title, then every record whose
// links to it (RenameLinks) or whose rn.Rewrite change, and the message that names them. It fails on a rename that
// changes nothing, a new name not lowercase words joined by '-' and a new name whose file exists.
func (rn Rename) Plan(recs []*Record) ([]Change, string, error) {
	from, to := rn.From, rn.From.Rel
	switch {
	case rn.To == "" && rn.Title == "":
		return nil, "", fmt.Errorf("give --to NEW_SLUG, --title \"…\" or both; a rename with neither changes nothing")
	case rn.To != "" && !slug.MatchString(rn.To):
		return nil, "", fmt.Errorf("%s slug %s must be lowercase words joined by '-'", from.Type(), strconv.Quote(rn.To))
	case rn.To != "":
		to = path.Dir(from.Rel) + "/" + docDate.FindString(from.Name()) + rn.To
	}
	retitled := rn.Title != "" && rn.Title != from.Title()
	newPath := filepath.Join(filepath.Dir(from.Path), path.Base(to)+".md")
	if to == from.Rel && !retitled {
		return nil, "", fmt.Errorf("%s is named %s and titled %s already; nothing to rename", kindNoun(from),
			from.Name(), strconv.Quote(from.Title()))
	}
	if _, err := os.Stat(newPath); to != from.Rel && err == nil {
		return nil, "", fmt.Errorf("records/%s.md already exists; choose another slug", to)
	}
	rewrite := func(x *Record) string {
		dir, _, _ := strings.Cut(x.Rel, "/")
		text, _ := RenameLinks(x.Text, dir, from.Rel, to, from.Title(), rn.Title)
		if rn.Rewrite != nil {
			text = rn.Rewrite(text)
		}
		return text
	}
	text := rewrite(from)
	if retitled {
		var err error
		if text, err = WithTitle(text, from.Rel, rn.Title); err != nil {
			return nil, "", err
		}
	}
	changes := []Change{{Path: newPath, Text: text}}
	if newPath != from.Path {
		changes = append(changes, Change{Path: from.Path, Remove: true})
	}
	var linked []string
	for _, x := range recs {
		if text := rewrite(x); x != from && text != x.Text {
			changes = append(changes, Change{Path: x.Path, Text: text})
			linked = append(linked, "records/"+x.Rel+".md")
		}
	}
	msg := fmt.Sprintf("retitled %s %s %s", kindNoun(from), from.Name(), strconv.Quote(rn.Title))
	if to != from.Rel {
		msg = fmt.Sprintf("renamed %s %s to %s", kindNoun(from), from.Name(), path.Base(to))
		if retitled {
			msg += ", titled " + strconv.Quote(rn.Title)
		}
	}
	if len(linked) > 0 {
		msg += "; rewrote its links in " + strings.Join(linked, ", ")
	}
	return changes, msg, nil
}

// RenameLinks is text, a record in the records directory dir (design, docs, sprints, …), with each Markdown link to
// the record from (a Rel, such as design/parser) pointing at to instead, its anchor kept: ../design/parser.md from
// any directory, and parser.md or ./parser.md from a record beside it. A link whose text is fromTitle takes
// toTitle when toTitle is not "". It says whether text changed.
func RenameLinks(text, dir, from, to, fromTitle, toTitle string) (string, bool) {
	kind, name := path.Split(from)
	prefix := `\.\./` + regexp.QuoteMeta(kind)
	if strings.TrimSuffix(kind, "/") == dir {
		prefix = `(?:` + prefix + `|\./)?`
	}
	link := regexp.MustCompile(`\[([^\]\n]*)\]\((` + prefix + `)` + regexp.QuoteMeta(name) + `\.md([#)])`)
	out := link.ReplaceAllStringFunc(text, func(m string) string {
		g := link.FindStringSubmatch(m)
		label := g[1]
		if toTitle != "" && label == fromTitle {
			label = toTitle
		}
		return "[" + label + "](" + g[2] + path.Base(to) + ".md" + g[3]
	})
	return out, out != text
}
