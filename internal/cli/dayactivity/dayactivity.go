// Package dayactivity is a day's activity: the plain text of what happened on one local day, from the records and
// the work store, which pm day summarize gives the model and hashes into the summary's digest.
package dayactivity

import (
	"sort"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// Of is plain text of what happened on the local day, from the records in recordsDir (recs, as read) and the work
// store's items: per project, the sprints finished (with their
// Outcome) or opened, the tasks finished (with their close reason), started or opened, and the owner requests raised
// or closed that day; the docs dated that day; and the records committed that day, not counting the generated
// summaries. The summary's input and, hashed, its digest. It holds only events dated that day, never current state,
// so a day's digest changes only when that day's events do. Empty when nothing happened that day.
func Of(recordsDir string, recs []*records.Record, items []work.Item, day string) (string, error) {
	on := func(t time.Time) bool { return site.LocalDay(t) == day }
	var out []string
	for _, p := range recs {
		if p.Type() != "project" {
			continue
		}
		var lines []string
		for _, sp := range children(items, p.Bead(), func(it *work.Item) bool { return it.Type == work.Sprint }) {
			rows, err := sprintActivity(recs, items, sp, on)
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
	log, err := store.Git(recordsDir, "log", "--since="+day+"T00:00", "--until="+day+"T23:59:59", "--format=%x00%s",
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

// children is the items under parent that keep holds, in id order.
func children(items []work.Item, parent string, keep func(*work.Item) bool) []*work.Item {
	var out []*work.Item
	for i := range items {
		if it := &items[i]; it.Parent == parent && keep(it) {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return work.CompareIDs(out[i].ID, out[j].ID) < 0 })
	return out
}

// sprintActivity is the rows of what happened to sprint sp, and to its tasks and needs, on the day on holds.
func sprintActivity(recs []*records.Record, items []work.Item, sp *work.Item, on func(time.Time) bool) ([]string, error) {
	var rows []string
	if on(sp.ClosedAt) {
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
	if on(sp.CreatedAt) {
		rows = append(rows, "    sprint opened")
	}
	// a dismissed need is a [TEST] or replaced one; a dismissed task is a dropped one, which moved the day
	for _, t := range children(items, sp.ID, func(it *work.Item) bool {
		return it.Resolution != work.Dismissed || it.Type == work.Task
	}) {
		request := t.Type == work.Need
		noun := "task"
		if request {
			noun = "request to the owner (" + site.Kind(t) + ")"
		}
		if on(t.ClosedAt) {
			verb := "closed"
			if t.Resolution == work.Dismissed {
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
