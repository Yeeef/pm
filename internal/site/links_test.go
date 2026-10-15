package site

import (
	"cmp"
	"path"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/records"
)

// linkRecs is a record at each rel, a doc dated by its name; only the doc index reads more than the rel.
func linkRecs(rels ...string) []*Record {
	str := func(s string) records.Value { return records.Value{Kind: records.KindStr, Str: s, Truthy: true} }
	var out []*Record
	for _, rel := range rels {
		date := cmp.Or(datePrefix.FindString(path.Base(rel)), "2026-10-15-")
		out = append(out, &Record{Rel: rel, Path: rel + ".md", Body: "\nx\n", Meta: records.Meta{
			"type": str("doc"), "title": str("T"), "date": str(date[:10]), "project": str("p")}})
	}
	return out
}

// A relative link to a record points at its page: by its file name, or for a dated record by its slug alone; a link
// into the store that names no record is reported, one that leaves the store or names no .md file is only rewritten.
func TestALinkToARecordResolvesToItsPage(t *testing.T) {
	recs := linkRecs("docs/2026-10-15-note-cfr", "docs/2026-10-01-twice", "docs/2026-10-02-twice",
		"design/parser", "postmortems/2026-10-03-outage")
	cases := []struct{ from, dest, want, broken string }{
		{"docs/2026-10-15-x", "./note-cfr.md", "./2026-10-15-note-cfr.html", ""},
		{"docs/2026-10-15-x", "note-cfr.md#why", "2026-10-15-note-cfr.html#why", ""},
		{"docs/2026-10-15-x", "2026-10-15-note-cfr.md", "2026-10-15-note-cfr.html", ""},
		{"sprints/p-1", "../docs/note-cfr.md", "../docs/2026-10-15-note-cfr.html", ""},
		{"docs/2026-10-15-x", "../design/parser.md#a", "../design/parser.html#a", ""},
		{"design/parser", "../postmortems/outage.md", "../postmortems/2026-10-03-outage.html", ""},
		{"docs/2026-10-15-x", "../../README.md", "../../README.html", ""},
		{"docs/2026-10-15-x", "fig.png", "fig.png", ""},
		{"docs/2026-10-15-x", "./03-mccfr.md", "./03-mccfr.html",
			"docs/2026-10-15-x.md: the link ./03-mccfr.md names records/docs/03-mccfr.md, which does not exist"},
		{"docs/2026-10-15-x", "../pdfs.md", "../pdfs.html",
			"docs/2026-10-15-x.md: the link ../pdfs.md names records/pdfs.md, which does not exist"},
		{"docs/2026-10-15-x", "twice.md", "twice.html", "docs/2026-10-15-x.md: the link twice.md names a slug that " +
			"2 records have: 2026-10-01-twice, 2026-10-02-twice"},
	}
	for _, c := range cases {
		got, broken := resolveLink(c.from, c.dest, recs)
		if got != c.want || broken != c.broken {
			t.Errorf("%s in %s: got %q %q, want %q %q", c.dest, c.from, got, broken, c.want, c.broken)
		}
	}
}

// A link that names no record fails pm check, naming the file and the link, but the page still renders; a link in
// code is no link.
func TestCheckReportsALinkToNoRecord(t *testing.T) {
	recs := linkRecs("docs/2026-10-15-x")
	recs[0].Body += "\n[a](./gone.md) `[b](./code.md)` [c](x.md)\n"
	c := &ctx{rel: recs[0].Rel, recs: recs, links: true}
	out, err := render(recordMD(c), recs[0].Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<a href="./gone.html">a</a>`) {
		t.Errorf("the link to no record does not render as a link: %s", out)
	}
	want := []string{"docs/2026-10-15-x.md: the link ./gone.md names records/docs/gone.md, which does not exist"}
	if strings.Join(c.broken, "|") != strings.Join(want, "|") {
		t.Errorf("broken links %q, want %q", c.broken, want)
	}
}
