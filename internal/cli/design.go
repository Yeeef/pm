package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// designGate is the design-page gate (design.Gate) on the files a PR changes, for the sprints it delivers. When no
// design page has covers there is nothing to gate, and files is not called.
func (r *repo) designGate(pr string, sprintIDs []string, files func() ([]string, error)) ([]design.Verdict, error) {
	if !design.Covered(r.recs) {
		return nil, nil
	}
	var sprints []design.Sprint
	for _, id := range sprintIDs {
		rec, err := r.sprint(id)
		if err != nil {
			return nil, err
		}
		sprints = append(sprints, design.Sprint{Record: rec, Opened: r.item(id).CreatedAt})
	}
	return design.Gate(r.records, pr, sprints, r.recs, files)
}

// designSlugs is the design pages' slugs, sorted.
func (r *repo) designSlugs() []string {
	var out []string
	for _, rec := range r.recs {
		if rec.Type() == "design" && !contains(out, rec.Name()) {
			out = append(out, rec.Name())
		}
	}
	sort.Strings(out)
	return out
}

// cmdDesignUnchanged is pm design unchanged: a sprint decision recording why a design page stays as it is although the
// sprint's PR changes code it covers, in the form the gate reads (design.WaiverText).
func cmdDesignUnchanged(e *env, p *Parsed) (string, error) {
	reason, err := decisionLine("--reason", p.Get("reason"))
	if err != nil {
		return "", err
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id, slug := p.Get("sprint"), p.Get("slug")
	rec, err := r.sprint(id)
	if err != nil {
		return "", err
	}
	if it := r.item(id); it == nil || it.Status == work.Closed {
		return "", refuse("sprint %s is closed or not in the work store; waive a page in an open sprint", id)
	}
	if known := r.designSlugs(); !contains(known, slug) {
		return "", refuse("no design page %s; known: %s", pyRepr(slug), strings.Join(known, ", "))
	}
	if _, ok := design.Waivers(rec)[slug]; ok {
		return "", refuse("%s already records that design page %s stays as it is", r.rel(rec.Path), slug)
	}
	updated, err := records.InsertEntry(rec.Text, "Decisions", decisionBlock("agent", design.WaiverText(slug, reason)))
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, fmt.Sprintf("recorded in %s that design page %s stays as it is", r.rel(rec.Path), slug), "pm: ")
}
