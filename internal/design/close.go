package design

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// Closing is what pm sprint close does with its sprint's design pages (the record-drafts page, Publish at sprint
// close): the pages its drafts publish (store.Publish), and the closing workspace's uncommitted edits to the pages it
// discards, put back once the close's records commit is made.
type Closing struct {
	*store.Publish
	w      *store.Workspace
	revert []string
	notes  []string // what the close leaves to others: unreadable drafts, other worktrees' edits of its pages
}

// PlanClose plans, writing nothing, the publish of the sprint's design-page drafts from every draft of the clone, less
// discard (slugs, each with its reason), after the close's other checks; here is where the close runs, dir the records
// store, root the code checkout. It refuses while the closing workspace (here's; none in the main checkout) has uncommitted edits to a page the sprint links and does not discard; a discard of a page the
// sprint has no draft of, edit of or waiver for, or that a close of the sprint published; an overlap or an unreadable
// branch that may hold the sprint's pages (store.PlanPublish); and a covers pattern of a page to publish that matches
// no file on the remote's main, fetched (MissingOnMain). It notes, without refusing, unreadable branches it leaves out
// and other worktrees' uncommitted edits of the pages it publishes.
func PlanClose(here, dir, root string, sprint *records.Record, discard map[string]string) (*Closing, error) {
	id := sprint.Bead()
	cfg, err := config.Load(here)
	if err != nil {
		return nil, err
	}
	remote, branch := cfg.Remote, cfg.MainBranch
	w, err := store.WorkspaceOf(here)
	if err != nil {
		return nil, err
	}
	c := &Closing{w: w}
	var dirty []string
	if w != nil {
		if dirty, err = w.Dirty(); err != nil {
			return nil, err
		}
	}
	drafted, _, err := store.Drafted(dir) // PlanPublish refuses an unreadable branch that may hold the sprint's pages
	if err != nil {
		return nil, err
	}
	for _, slug := range slices.Sorted(maps.Keys(discard)) {
		page := "design/" + slug + ".md"
		if done, err := store.PublishedBy(dir, page, id); err != nil {
			return nil, err
		} else if done && !Discarded(sprint)[slug] {
			return nil, fmt.Errorf("--discard %s: a close of sprint %s already published records/%s; it is no draft "+
				"to drop", slug, id, page)
		}
		_, waived := Waivers(sprint)[slug]
		if !drafted[id][page] && !slices.Contains(dirty, page) && !waived {
			return nil, fmt.Errorf("--discard %s: sprint %s has no draft of the design page %s to drop", slug, id, slug)
		}
	}
	dropped := Discarded(sprint) // what an earlier run of the close discarded holds on every rerun, whatever its flags
	for slug := range discard {
		dropped[slug] = true
	}
	var paths []string
	for _, slug := range slices.Sorted(maps.Keys(dropped)) {
		page := "design/" + slug + ".md"
		paths = append(paths, page)
		if slices.Contains(dirty, page) {
			c.revert = append(c.revert, page)
		}
	}
	for _, slug := range records.DesignLinks(sprint) {
		if !dropped[slug] && slices.Contains(dirty, "design/"+slug+".md") {
			page := "design/" + slug + ".md"
			return nil, fmt.Errorf("records/%s has uncommitted edits in this worktree's records workspace %s, and sprint "+
				"%s links it: pm commit it, which drafts it for the sprint, or drop the edits with --discard %s "+
				"--reason \"…\"; then pm sprint close %s again", page, w.Dir, id, slug, id)
		}
	}
	if c.Publish, err = store.PlanPublish(dir, id, paths); err != nil {
		return nil, err
	}
	held := map[string][]string{} // every pattern of a page to publish: once published, pm check holds them all to main
	for _, page := range slices.Sorted(maps.Keys(c.Pages)) {
		if c.Pages[page] == nil {
			continue
		}
		if held[strings.TrimSuffix(page, ".md")], err = records.CoversOf(*c.Pages[page], page); err != nil {
			return nil, err
		}
	}
	missing, err := MissingOnMain(root, remote, branch, held)
	if err != nil {
		return nil, fmt.Errorf("sprint %s: %w", id, err)
	}
	if missing != nil {
		return nil, fmt.Errorf("sprint %s: the design pages it publishes have covers patterns that name files %s/%s "+
			"does not have: %s. Fix each pattern to the file main has in the workspace that drafts the page and pm "+
			"commit it there, or merge the PR that adds the file first; then pm sprint close %s again", id, remote,
			branch, strings.Join(missing, ", "), id)
	}
	c.notes = append(c.notes, c.Skipped...)
	if err := c.othersEdits(dir); err != nil {
		return nil, err
	}
	return c, nil
}

// othersEdits notes each other worktree's uncommitted edit of a page the close publishes: the close does not wait on
// it, and once the sprint is closed it has no open sprint to be committed under.
func (c *Closing) othersEdits(dir string) error {
	branches, err := store.DraftBranches(dir)
	if err != nil {
		return err
	}
	for _, w := range branches {
		if w.Dir == "" || (c.w != nil && w.Dir == c.w.Dir) {
			continue
		}
		dirty, err := w.Dirty()
		if err != nil {
			return err
		}
		for _, page := range slices.Sorted(maps.Keys(c.Pages)) {
			if slices.Contains(dirty, page) {
				c.notes = append(c.notes, fmt.Sprintf("note: worktree %s has uncommitted edits to records/%s, which "+
					"this close published without them; commit them for an open sprint that links the page, or "+
					"revert them", w.Code, page))
			}
		}
	}
	return nil
}

// Writes is the close's writes of the pages it publishes, in the records store dir.
func (c *Closing) Writes(dir string) []store.Write {
	var w []store.Write
	for _, page := range slices.Sorted(maps.Keys(c.Pages)) {
		wr := store.Write{Path: filepath.Join(dir, filepath.FromSlash(page)), Remove: c.Pages[page] == nil}
		if !wr.Remove {
			wr.Text = *c.Pages[page]
		}
		w = append(w, wr)
	}
	return w
}

// Done finishes the close's drafts once its records commit is made: the published and discarded pages leave the
// drafts (store.Publish.Drop), and the closing workspace's uncommitted edits to a discarded page are put back.
func (c *Closing) Done(dir string) error {
	err := c.Drop(dir)
	if err == nil && c.revert != nil {
		err = c.w.Revert(dir, c.revert)
	}
	return err
}

// Said is the close's line on its pages: those it published and those it discarded; "" for none.
func (c *Closing) Said(discard map[string]string) string {
	var parts []string
	if len(c.Pages) > 0 {
		parts = append(parts, "published "+strings.Join(slices.Sorted(maps.Keys(c.Pages)), ", "))
	}
	if len(discard) > 0 {
		parts = append(parts, "discarded the drafts of "+strings.Join(slices.Sorted(maps.Keys(discard)), ", "))
	}
	said := strings.Join(parts, "; ")
	for _, n := range c.notes {
		said += "\n" + n
	}
	return said
}

// discardLine follows a waiver's reason when pm sprint close --discard recorded it: the record of the discard, which
// every later run of the close reads, so none publishes the page's draft.
const discardLine = "Its drafts are discarded at the sprint's close, not published."

// Discarded is the pages whose drafts the sprint's close discarded, by slug, as its record holds them.
func Discarded(sprint *records.Record) map[string]bool {
	out := map[string]bool{}
	for _, d := range records.Decisions(sprint.Text) {
		first, _, _ := strings.Cut(strings.TrimSpace(d.Body), "\n")
		if m := waiverRE.FindStringSubmatch(first); m != nil && strings.Contains(d.Body, "\n"+discardLine) {
			out[m[1]] = true
		}
	}
	return out
}

// Waive is the sprint record's text with a waiver decision (WaiverText) for each page of discard (slug and reason) it
// has not discarded yet, in slug order, its last line discardLine; block makes the decision block of a body.
func Waive(sprint *records.Record, discard map[string]string, block func(string) string) (string, error) {
	text, done := sprint.Text, Discarded(sprint)
	for _, slug := range slices.Sorted(maps.Keys(discard)) {
		if done[slug] {
			continue
		}
		var err error
		if text, err = records.InsertEntry(text, "Decisions", block(WaiverText(slug, discard[slug])+"\n"+
			discardLine)); err != nil {
			return "", err
		}
	}
	return text, nil
}
