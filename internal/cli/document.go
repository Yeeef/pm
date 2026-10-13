package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"os"
	"path/filepath"
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// newRecord is the checks every new-record command makes first: the slug's form (badSlug the refusal, with the slug
// for its %s) and a non-empty title.
func newRecord(badSlug, slug, title string) error {
	if !slugRE.MatchString(slug) {
		return refuse(badSlug, pyRepr(slug))
	}
	if strip(title) == "" {
		return refuse("--title is empty")
	}
	return nil
}

func (r *repo) refuseExisting(path string) error {
	if exists(path) {
		return refuse("%s already exists; edit it by hand or choose another slug", r.rel(path))
	}
	return nil
}

func cmdDocNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("doc slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	body := p.Get("text")
	if body == "" {
		return "", refuse("the doc body is empty; pass it with %s", textForms)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var target string
	if bead, ok := given(p, "bead"); ok {
		if r.item(bead) == nil {
			return "", refuse("--bead %s is not in the work store", bead)
		}
		inside := false
		chain := append([]string{bead}, r.x.Ancestors(bead)...)
		for _, rec := range r.recs {
			if rec.Type() == "project" && contains(chain, rec.Bead()) {
				inside = true
			}
		}
		if !inside {
			return "", refuse("--bead %s is not inside a project with a record", bead)
		}
		target = "bead: " + bead
	} else {
		prec, err := r.project(p.Get("project"))
		if err != nil {
			return "", err
		}
		target = "project: " + prec.Name()
	}
	day := store.Today()
	path := filepath.Join(r.records, "docs", day+"-"+slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	text := fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n%s\n---\n\n%s\n", records.YAMLStr(strip(title)), day, target, body)
	w := []store.Write{{Path: path, Text: text}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "created "+r.rel(path), "pm: ")
}

func cmdDesignNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("design slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	prec, err := r.project(p.Get("project"))
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.records, "design", slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: records.DesignText(records.YAMLStr(strip(title)), prec.Name())}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, fmt.Sprintf("created %s; fill in its sections by hand in the store, then pm commit -m \"…\" %s",
		r.rel(path), r.rel(path)), "pm: ")
}

func cmdPostmortemNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("postmortem slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var target string
	if sprintID, ok := given(p, "sprint"); ok {
		rec, err := r.sprint(sprintID)
		if err != nil {
			return "", err
		}
		target = "sprint: " + rec.Bead()
	} else {
		prec, err := r.project(p.Get("project"))
		if err != nil {
			return "", err
		}
		target = "project: " + prec.Name()
	}
	day := store.Today()
	path := filepath.Join(r.records, "postmortems", day+"-"+slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: records.PostmortemText(records.YAMLStr(strip(title)), day, target)}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, fmt.Sprintf("created %s; fill in its sections by hand in the store, then pm commit -m \"…\" %s",
		r.rel(path), r.rel(path)), "pm: ")
}

// ---------------------------------------------------------------- projects and sprints
