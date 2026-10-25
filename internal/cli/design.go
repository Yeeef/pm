package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"slices"
)

// designGate is the design-page gate (design.Gate) on the files a PR changes, for the sprints it delivers, each with
// the pages the clone's drafts hold for it. When no design page has covers, or pr is another repo's PR, there is
// nothing to gate, and files is not called.
func (r *repo) designGate(pr string, sprintIDs []string, files func() ([]string, error)) ([]design.Verdict, error) {
	// another repo's PR changes none of this repo's paths, which the covers name
	if other, err := otherRepo(r.root, pr); err != nil || other != "" || !design.Covered(r.recs) {
		return nil, err
	}
	drafted, warnings, err := store.Drafted(r.records)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings { // another worktree's bad branch never fails this gate
		fmt.Fprintln(r.warn, w)
	}
	var sprints []design.Sprint
	for _, id := range sprintIDs {
		rec, err := r.sprint(id)
		if err != nil {
			return nil, err
		}
		it := r.item(id)
		if it == nil {
			return nil, refuse("sprint %s is not in the work store, so the design-page check cannot tell when it opened", id)
		}
		sprints = append(sprints, design.Sprint{Record: rec, Opened: it.CreatedAt, Drafted: drafted[id]})
	}
	return design.Gate(r.records, pr, sprints, r.recs, files)
}

// designSlugs is the design pages' slugs, sorted.
func (r *repo) designSlugs() []string {
	var out []string
	for _, rec := range r.recs {
		if rec.Type() == "design" && !slices.Contains(out, rec.Name()) {
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
	if known := r.designSlugs(); !slices.Contains(known, slug) {
		return "", refuse("no design page %s; known: %s", strconv.Quote(slug), strings.Join(known, ", "))
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

// cmdDesignRename is pm design rename: a design page's slug or title or both, every link to it and every waiver of
// it (design.RenameCheck), in one records commit.
func cmdDesignRename(e *env, p *Parsed) (string, error) {
	return renameRecord(e, p, "design", design.RenameCheck)
}

// cmdDocRename is pm doc rename: a doc's slug, its date prefix kept, or its title, or both, and every link to it.
func cmdDocRename(e *env, p *Parsed) (string, error) { return renameRecord(e, p, "doc", nil) }

// renameCheck refuses a rename of the record name to the slug to ("" for none), or gives an extra rewrite of every
// record's text, or nil.
type renameCheck func(items []work.Item, name, to string) (func(string) string, error)

// renameRecord runs a rename of a record of kind (records.RenameTarget) as its flags give it, in one records commit;
// check, when not nil, runs before the plan.
func renameRecord(e *env, p *Parsed, kind string, check renameCheck) (string, error) {
	to, title := p.Get("to"), p.Get("title")
	if title, given := given(p, "title"); given && strip(title) == "" {
		return "", refuse("--title is empty")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	rn := records.Rename{To: to, Title: strip(title)}
	if rn.From, err = records.RenameTarget(r.recs, kind, p.Get("slug")); err != nil {
		return "", err
	}
	if check != nil {
		if rn.Rewrite, err = check(r.items, rn.From.Name(), to); err != nil {
			return "", err
		}
	}
	changes, msg, err := rn.Plan(r.recs)
	if err != nil {
		return "", err
	}
	w := make([]store.Write, len(changes))
	for i, c := range changes {
		w[i] = store.Write(c)
	}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, msg, "pm: ")
}
