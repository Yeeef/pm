// Package designindex is the design-page index: the design pages a sprint and its project list in their Design pages
// sections, printed under the line of `pm sprint open` and of a session's first `pm task claim` in a sprint (the
// work-context design page, Design-page index), so an agent starting work sees once where its area's current state is
// written.
package designindex

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
)

// Block is the index build gives, as a block that starts on a new line; when build fails (the site's URL or the
// records cannot be read), one line naming the error instead, since the open or claim it follows is already done.
func Block(build func() (string, error)) string {
	block, err := build()
	if err != nil {
		return "\ndesign pages not listed: " + err.Error()
	}
	return block
}

// Of is the pages the records from list in their Design pages sections, the first record's first, without repeats: a
// heading, then one line per page with its slug, title and link (the site's URL + the page's Out(), as pm show links a
// record), or `missing` for a slug that names no design record in recs. "" when they list none; url, the site's URL,
// is resolved only when there is a page to print.
func Of(recs []*records.Record, url func() (string, error), from ...*records.Record) (string, error) {
	var slugs []string
	for _, rec := range from {
		for _, s := range records.DesignLinks(rec) {
			if !slices.Contains(slugs, s) {
				slugs = append(slugs, s)
			}
		}
	}
	if slugs == nil {
		return "", nil
	}
	site, err := url()
	if err != nil {
		return "", err
	}
	pages := map[string]*records.Record{}
	for _, rec := range recs {
		if rec.Type() == "design" {
			pages[rec.Name()] = rec
		}
	}
	slugWidth, titleWidth := 0, 0
	for _, s := range slugs {
		slugWidth = max(slugWidth, len([]rune(s)))
		if p := pages[s]; p != nil {
			titleWidth = max(titleWidth, len([]rune(p.Title())))
		}
	}
	lines := []string{"", "design pages (read the ones your task touches):"}
	for _, s := range slugs {
		line := "  " + pad(s, slugWidth) + "  "
		if p := pages[s]; p != nil {
			line += pad(p.Title(), titleWidth) + "  " + site + "/" + p.Out()
		} else {
			line += "missing"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// Claim is the index session sid's claim of task id prints: the pages of the task's sprint, then of its project, when
// no other task of that sprint is held by sid or was closed by it. A task directly under a project has the project's
// pages, and the same test runs over the project's tasks. "" when this is not the session's first claim there. x is
// the store as it was before the claim.
func Claim(recs []*records.Record, x *records.Items, id, sid string, url func() (string, error)) (string, error) {
	var sprint, project string
	for _, a := range x.Ancestors(id) {
		it := x.Get(a)
		switch {
		case it == nil:
		case it.Type == item.Sprint && sprint == "":
			sprint = a
		case it.Type == item.Project:
			project = a
		}
	}
	scope := sprint
	if scope == "" {
		scope = project
	}
	if scope == "" {
		return "", nil
	}
	for _, it := range x.All() {
		if it.ID == id || it.Type != item.Task || !slices.Contains(x.Ancestors(it.ID), scope) {
			continue
		}
		if (it.Holder != nil && it.Holder.Session == sid) || it.ClosedBy == sid {
			return "", nil
		}
	}
	var from []*records.Record
	for _, bead := range []string{sprint, project} {
		if bead == "" {
			continue
		}
		rec := byBead(recs, bead)
		if rec == nil {
			return "", fmt.Errorf("no sprint or project record has bead %s", bead)
		}
		from = append(from, rec)
	}
	return Of(recs, url, from...)
}

// byBead is the sprint or project record whose bead is id, or nil.
func byBead(recs []*records.Record, id string) *records.Record {
	for _, rec := range recs {
		if (rec.Type() == "sprint" || rec.Type() == "project") && rec.Bead() == id {
			return rec
		}
	}
	return nil
}

// pad is s padded with spaces to n runes.
func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-len([]rune(s))))
}
