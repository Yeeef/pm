package site

import (
	"slices"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// A day's activity: what the day page shows of a sprint, and DayActivity, the text pm day summarize sends the model.

// sprintMarks is what a sprint itself did on the day, oldest first: "proposed" (a sprint labelled proposal) or
// "opened" on its create date; "started", for a proposal, on the date of its first task when that is a later day;
// "dropped" on the date a proposal nobody started closed. A finished sprint is the caller's: the page shows no row
// for it, the text its Outcome.
func sprintMarks(sp *Item, items *Items, on func(time.Time) bool) []string {
	var marks []string
	proposal := slices.Contains(sp.Labels, item.ProposalLabel)
	switch {
	case on(sp.CreatedAt) && proposal:
		marks = append(marks, "proposed")
	case on(sp.CreatedAt):
		marks = append(marks, "opened")
	}
	var first time.Time
	for _, t := range items.Children(sp.ID) {
		if at := item.Arrived(t, sp.ID); t.Type == item.Task && (first.IsZero() || at.Before(first)) {
			first = at
		}
	}
	if proposal && !first.IsZero() && on(first) && LocalDay(first) > LocalDay(sp.CreatedAt) {
		marks = append(marks, "started")
	}
	if sp.Status == item.Closed && items.Unstarted(sp) && on(sp.ClosedAt) {
		marks = append(marks, "dropped")
	}
	return marks
}

// DayActivity is plain text of what happened on the local day: per project, the sprints finished (with their
// Outcome), proposed, opened, started or dropped (sprintMarks), the tasks finished (with their close reason), started
// or opened, and the owner requests raised or closed that day; the docs dated that day; and the records committed that
// day in the records store at dir, not counting the generated summaries. The summary's input and, hashed, its digest.
// It holds only events dated that day, never current state, so a day's digest changes only when that day's events do.
// Empty when nothing happened that day.
func DayActivity(day, dir string, recs []*Record, items *Items) (string, error) {
	on := func(t time.Time) bool { return LocalDay(t) == day }
	var out []string
	for _, p := range recs {
		if p.Type() != "project" {
			continue
		}
		var lines []string
		for _, sp := range sprintsOf(items, p.Bead()) {
			rows, err := sprintActivity(sp, recs, items, on)
			if err != nil {
				return "", err
			}
			if rows != nil {
				lines = append(append(lines, "  sprint "+sp.Title), rows...)
			}
		}
		if lines != nil {
			out = append(append(out, "project "+p.Title()), lines...)
		}
	}
	for _, rec := range recs {
		if rec.Type() == "doc" && rec.Meta["date"].Str == day {
			out = append(out, "doc "+rec.Title()+" ("+rec.Rel+")")
		}
	}
	log, err := store.Git(dir, "log", "--since="+day+"T00:00", "--until="+day+"T23:59:59", "--format=%x00%s",
		"--name-only")
	if err != nil {
		return "", err
	}
	for _, chunk := range strings.Split(log, "\x00")[1:] {
		var lines []string
		for _, l := range strings.Split(chunk, "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			continue
		}
		var names []string
		for _, n := range lines[1:] {
			if !strings.HasSuffix(n, ".summary.json") {
				names = append(names, n)
			}
		}
		if names != nil {
			out = append(out, "records commit: "+lines[0]+" ["+strings.Join(names, ", ")+"]")
		}
	}
	return strings.Join(out, "\n"), nil
}

func sprintActivity(sp *Item, recs []*Record, items *Items, on func(time.Time) bool) ([]string, error) {
	var rows []string
	if on(sp.ClosedAt) && !items.Unstarted(sp) {
		row := "    sprint finished"
		for _, rec := range recs {
			if rec.Type() == "sprint" && rec.Bead() == sp.ID {
				o, err := records.Outcome(rec)
				if err != nil {
					return nil, err
				}
				if o != "" {
					row += ": " + o
				}
				break
			}
		}
		rows = append(rows, row)
	}
	for _, m := range sprintMarks(sp, items, on) {
		if m == "dropped" {
			rows = append(rows, "    proposal dropped ("+sp.CloseReason+")")
		} else {
			rows = append(rows, "    sprint "+m)
		}
	}
	// a dismissed need is a [TEST] or replaced one; a dismissed task is a dropped one, which moved the day
	for _, t := range childrenWhere(items, sp.ID, func(it *Item) bool {
		return it.Resolution != item.Dismissed || it.Type == item.Task
	}) {
		request := t.Type == item.Need
		noun := "task"
		if request {
			noun = "request to the owner (" + Kind(t) + ")"
		}
		if on(t.ClosedAt) {
			verb := "closed"
			if t.Resolution == item.Dismissed {
				verb = "dropped"
			}
			row := "    " + noun + " " + verb + ": " + t.Title
			if t.CloseReason != "" {
				row += " (" + t.CloseReason + ")"
			}
			rows = append(rows, row)
		} else if on(t.StartedAt) {
			rows = append(rows, "    "+noun+" started: "+t.Title)
		}
		if on(t.CreatedAt) {
			if request {
				rows = append(rows, "    "+noun+" raised: "+t.Title)
			} else {
				rows = append(rows, "    task opened: "+t.Title)
			}
		}
	}
	return rows, nil
}
