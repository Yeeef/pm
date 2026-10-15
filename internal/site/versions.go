package site

import (
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// Versions: each draft (store.Draft) is served beside the published records as a version, the published texts with the
// draft's files laid over them. A version renders in full under DraftsDir, so a record's relative links stay in it:
// /drafts/<draft>/<record>.html. Its diff is /drafts/<draft>/<record>.diff.html, against the published text, or with
// ?vs=<other draft> against that draft's. A version is built on its first request and kept for the Served's life, one
// read of the records and the items; a version that does not parse or check fails only its own URLs. A page that a
// draft changes, and the overview while any draft exists, carries the version bar.

// DraftsDir is the directory of the versions' pages.
const DraftsDir = "drafts/"

// version is one draft served as a version: its site, built on its first request.
type version struct {
	draft  store.Draft
	once   sync.Once
	served *Served
	err    error
}

// AddDrafts serves each draft as a version beside s: root is the records store and texts the published record texts
// that s was made from, by resolved path, as store.Published gives them.
func (s *Served) AddDrafts(root string, texts map[string]string, drafts []store.Draft) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if err := reserved(s.recs); err != nil {
		return err
	}
	s.root, s.texts = root, texts
	for _, d := range drafts {
		if d.Name == "" || strings.ContainsAny(d.Name, "/?#%") || s.version(d.Name) != nil {
			return fmt.Errorf("draft name %q is empty, holds a URL delimiter or names two drafts", d.Name)
		}
		s.versions = append(s.versions, &version{draft: d})
	}
	return nil
}

// reserved fails a record under DraftsDir, whose page the versions' URLs would hide.
func reserved(recs []*Record) error {
	for _, r := range recs {
		if strings.HasPrefix(r.Out(), DraftsDir) {
			return errorf("%s: the site serves the drafts' versions under %s; move the record out of it", r.Rel,
				DraftsDir)
		}
	}
	return nil
}

func (s *Served) version(name string) *version {
	for _, v := range s.versions {
		if v.draft.Name == name {
			return v
		}
	}
	return nil
}

// build is v's site: the published texts with the draft's files laid over them, parsed and checked as Serve checks
// the published ones. A design page the draft changes or adds is updated today; the others keep their dates.
func (s *Served) build(v *version) (*Served, error) {
	v.once.Do(func() {
		texts := maps.Clone(s.texts)
		for rel, text := range v.draft.Files {
			p := filepath.Join(s.root, filepath.FromSlash(rel))
			if text == nil {
				delete(texts, p)
			} else {
				texts[p] = *text
			}
		}
		recs, err := records.ParseAll(s.root, texts)
		if err == nil {
			err = reserved(recs)
		}
		if err != nil {
			v.err = err
			return
		}
		var dates Dates
		if s.dates != nil {
			dates = Dates{}
			today := store.Today()
			for _, r := range recs {
				if r.Type() != "design" {
					continue
				}
				d, published := s.dates[r.Rel]
				if _, changed := v.draft.Files[r.Rel+".md"]; changed || !published {
					d = store.Dates{Created: cmp.Or(d.Created, today), Updated: today}
				}
				dates[r.Rel] = d
			}
		}
		v.served, v.err = Serve(recs, s.items, s.name, dates, s.summaries)
	})
	return v.served, v.err
}

// versionPage is the page at rest under DraftsDir: <draft>/<page>, a version's page with its bar, or
// <draft>/<record>.diff.html[?vs=<draft>], a diff.
func (s *Served) versionPage(rest string) (string, bool, error) {
	name, path, _ := strings.Cut(rest, "/")
	v := s.version(name)
	if v == nil || path == "" {
		return "", false, nil
	}
	path, vs, hasVS := strings.Cut(path, "?vs=")
	if rel, ok := strings.CutSuffix(path, ".diff.html"); ok {
		return s.diffPage(v, rel, vs)
	}
	if hasVS {
		return "", false, nil
	}
	site, err := s.build(v)
	if err != nil {
		return "", true, fmt.Errorf("the version %s does not render: %w", name, err)
	}
	text, found, err := site.page(path)
	if !found || err != nil {
		return text, found, err
	}
	return s.withBar(text, strings.TrimSuffix(path, ".html"), v, 2)
}

// withBar is page with the version bar after its title, when any draft changes the record at rel ("index", the
// overview: every draft); current is the version the page shows (nil: the published one), depth how many directories
// deeper than the published page it is served.
func (s *Served) withBar(page, rel string, current *version, depth int) (string, bool, error) {
	bar := s.bar(rel, current, strings.Repeat("../", strings.Count(rel, "/")+depth))
	if bar == "" {
		return page, true, nil
	}
	const title = "</h1>\n"
	i := strings.Index(page, title)
	if i < 0 {
		return "", true, fmt.Errorf("the page of %s has no title to put the version bar under", rel)
	}
	i += len(title)
	return page[:i] + bar + page[i:], true, nil
}

// bar is the version bar of the page of rel, its links relative to up (the site's root from the page): the published
// version, then each draft that changes rel, and the current one, with its label, its state and links to its render
// and its diffs; "" on a published page that no draft changes.
func (s *Served) bar(rel string, current *version, up string) string {
	var lis []string
	for _, v := range s.versions {
		text, changes := v.draft.Files[rel+".md"]
		if rel != "index" && !changes && v != current { // a version's page names its version, changed or not
			continue
		}
		at := up + DraftsDir + v.draft.Name + "/"
		name := `<a href="` + esc(at+rel+".html") + `">` + esc(v.draft.Name) + "</a>"
		if v == current {
			name = "<b>" + esc(v.draft.Name) + "</b>"
		}
		state := []string{v.draft.Label, v.draft.State}
		switch {
		case changes && text == nil:
			state = append(state, "removes this record")
		case !s.renders(v, rel):
			state = append(state, "does not render")
		}
		li := name + `<span class="k">` + esc(strings.Join(state, "; ")) + "</span>"
		if rel == "index" {
			var changed []string
			for _, f := range slices.Sorted(maps.Keys(v.draft.Files)) {
				r := strings.TrimSuffix(f, ".md")
				changed = append(changed, `<a href="`+esc(at+r+".diff.html")+`">`+esc(r)+"</a>")
			}
			li += `<span class="k">changes ` + strings.Join(changed, ", ") + "</span>"
		} else {
			li += `<a href="` + esc(at+rel+".diff.html") + `">diff against published</a>`
			if current != nil && v != current {
				li += `<a href="` + esc(at+rel+".diff.html?vs="+current.draft.Name) + `">diff against ` +
					esc(current.draft.Name) + "</a>"
			}
		}
		lis = append(lis, "<li>"+li+"</li>")
	}
	if lis == nil {
		return ""
	}
	published := `<a href="` + esc(up+rel+".html") + `">published</a>`
	switch {
	case current == nil:
		published = "<b>published</b>"
	case !s.published(rel):
		published = `published<span class="k">not published</span>`
	}
	return `<div class="note draft versions"><p class="cap">VERSIONS</p><ul class="list"><li>` + published + "</li>" +
		strings.Join(lis, "") + "</ul></div>\n"
}

// published is whether the published site has the page of rel.
func (s *Served) published(rel string) bool {
	if rel == "index" {
		return true
	}
	_, ok := s.texts[filepath.Join(s.root, filepath.FromSlash(rel)+".md")]
	return ok
}

// renders is whether v builds and renders the page of rel.
func (s *Served) renders(v *version, rel string) bool {
	site, err := s.build(v)
	if err != nil {
		return false
	}
	_, _, err = site.page(rel + ".html")
	return err == nil
}

// text is the record file of rel in v (nil: the published one), and whether it has one.
func (s *Served) text(v *version, rel string) (string, bool) {
	if v != nil {
		if t, ok := v.draft.Files[rel+".md"]; ok {
			if t == nil {
				return "", false
			}
			return *t, true
		}
	}
	t, ok := s.texts[filepath.Join(s.root, filepath.FromSlash(rel)+".md")]
	return t, ok
}

// diffPage is the diff of the record at rel in v against the published text, or against draft vs's: by section
// (diff.go). It reads only the two texts, so a version that does not render still has its diff.
func (s *Served) diffPage(v *version, rel, vs string) (string, bool, error) {
	var base *version
	against := "published"
	if vs != "" {
		if base = s.version(vs); base == nil {
			return "", false, nil
		}
		against = "draft " + vs
	}
	now, inNow := s.text(v, rel)
	was, inWas := s.text(base, rel)
	if !inNow && !inWas {
		return "", false, nil
	}
	meta := `<p class="meta"><span>draft ` + esc(v.draft.Name) + "</span><span>against " + esc(against) + "</span></p>"
	frm := &Record{Rel: DraftsDir + v.draft.Name + "/" + rel}
	text, err := page(frm, "diff", rel+".md", meta+sectionDiff(was, now), &ctx{}, "")
	if err != nil {
		return "", true, err
	}
	return s.withBar(text, rel, v, 2)
}
