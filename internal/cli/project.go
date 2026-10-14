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
)

func cmdProjectOpen(e *env, p *Parsed) (string, error) {
	name := p.Get("name")
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
	name := p.Get("name")
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
		if k.Status != work.Closed {
			open = append(open, fmt.Sprintf("%s (%s)", k.ID, k.Title))
		}
	}
	if open != nil {
		return "", refuse("open sprints in the project: %s; close them with pm sprint close first", strings.Join(open, ", "))
	}
	reason := fmt.Sprintf("%s (commit %s)", firstSentence(text, 200), head)
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

// ---------------------------------------------------------------- tasks
