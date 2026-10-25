package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func cmdProjectOpen(e *env, p *Parsed) (string, error) {
	name := p.Get("project")
	if !slugRE.MatchString(name) {
		return "", refuse("project name %s must be lowercase words joined by '-'", strconv.Quote(name))
	}
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	goal := p.Get("text")
	if goal == "" {
		return "", refuse("Goal is empty; pass the project's goal with %s", textForms)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.records, "projects", name+".md")
	taken := fsutil.Exists(path)
	for _, rec := range r.recs {
		taken = taken || rec.Type() == "project" && rec.Name() == name
	}
	if taken {
		return "", refuse("project %s already exists (%s)", strconv.Quote(name), r.rel(path))
	}
	const plannedID = "pm-planned-project"
	text := func(id string) string { return records.ProjectText(records.YAMLStr(title), id, goal) }
	if err := r.checkPlanned([]store.Write{{Path: path, Text: text(plannedID)}},
		r.with(planned(plannedID, work.Project, "", title))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Project, Title: title})
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: text(made.ID)}}
	if err := r.checkPlanned(w, r.with(made)); err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	out, err := r.apply(w, fmt.Sprintf("opened project %s: epic %s, record %s", name, made.ID, r.rel(path)), "pm: ")
	if err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	return out, nil
}

func cmdProjectClose(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	name := p.Get("project")
	prec, err := r.project(name)
	if err != nil {
		return "", err
	}
	epicID := prec.Bead()
	epic := r.item(epicID)
	if epic == nil {
		return "", refuse("project %s has bead %s, which is not in the work store", name, epicID)
	}
	if epic.Status == work.Closed {
		return "", refuse("project %s (%s) is already closed", name, epicID)
	}
	head, err := r.requireCommitted(prec.Path)
	if err != nil {
		return "", err
	}
	text := records.SectionText(prec.Body, "Outcome")
	if text == "" || text == records.NotClosed {
		return "", refuse("%s: the committed ## Outcome is still '%s'; write it and commit", r.rel(prec.Path),
			records.NotClosed)
	}
	var open []string
	for _, k := range r.x.Children(epicID) {
		if r.x.Proposal(k) {
			open = append(open, fmt.Sprintf("%s (%s; a proposal: pm sprint close %s --dropped --reason \"…\")", k.ID,
				k.Title, k.ID))
		} else if k.Status != work.Closed {
			open = append(open, fmt.Sprintf("%s (%s)", k.ID, k.Title))
		}
	}
	if open != nil {
		return "", refuse("open sprints in the project: %s; close them with pm sprint close first", strings.Join(open, ", "))
	}
	reason := fmt.Sprintf("%s (commit %s)", records.FirstSentence(text, 200), head)
	if err := r.checkPlanned(nil, r.with(closedCopy(epic, reason))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(epicID, reason, work.Done, closer(epic)); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed %s: %s", epicID, reason), nil
}

// cmdProjectReopen sets a closed project open again, as pm sprint reopen does a sprint (reopen).
func cmdProjectReopen(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	rec, err := r.project(p.Get("project"))
	if err != nil {
		return "", err
	}
	return r.reopen(e, p, rec, "project", fmt.Sprintf("project %s (%s)", rec.Name(), rec.Bead()), "Outcome")
}

// reopen sets a closed project or sprint open again, as pm sprint edit renames one: the work store first, then one
// records commit that puts the close-time section back to "Not closed yet." (records.Reopened), so the next close needs
// a fresh one, and adds the reason as a decision of the record; a failed records step closes the item again with its
// old close reason, resolution and closer, at a new close time.
func (r *repo) reopen(e *env, p *Parsed, rec *records.Record, kind, label, section string) (string, error) {
	id := rec.Bead()
	it := r.item(id)
	if it == nil {
		return "", refuse("%s is not in the work store", label)
	}
	if it.Status != work.Closed {
		return "", refuse("%s is open; only a closed %s is reopened", label, kind)
	}
	reason := p.Get("text")
	if err := decisionBody(reason); err != nil {
		return "", err
	}
	source := "agent"
	if p.Get("confirmed") != "" {
		source = "owner"
	}
	text, err := records.Reopened(rec.Text, kind)
	if err == nil {
		text, err = records.InsertEntry(text, "Decisions", decisionBlock(source,
			fmt.Sprintf("Reopened the %s, closed as \"%s\": %s", kind, it.CloseReason, reason)))
	}
	if err != nil {
		return "", err
	}
	open := *it
	open.Status, open.Resolution, open.CloseReason, open.ClosedBy, open.ClosedAt = work.Open, "", "", "", time.Time{}
	w := []store.Write{{Path: rec.Path, Text: text}}
	if err := r.checkPlanned(w, r.with(open)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Reopen(id); err != nil {
		return "", err
	}
	out, err := r.apply(w, fmt.Sprintf("reopened %s %s; its %s is '%s' again, and %s holds the reason as a %s decision",
		kind, id, section, records.NotClosed, r.rel(rec.Path), kind), "pm: ")
	if err != nil {
		if cerr := ws.Close(id, it.CloseReason, it.Resolution, it.ClosedBy); cerr != nil {
			return "", fmt.Errorf("%w; closing %s again failed too: %v", err, id, cerr)
		}
		return "", fmt.Errorf("%w; the work-store step is undone: %s is closed again", err, id)
	}
	return out, nil
}

// ---------------------------------------------------------------- tasks
