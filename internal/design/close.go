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
}

// PlanClose plans, writing nothing, the publish of the sprint's design-page drafts from every draft of the clone, less
// discard (slugs, each with its reason), after the close's other checks; here is where the close runs, dir the records
// store, root the code checkout. It refuses while the closing workspace (here's; none in the main checkout) has uncommitted edits to a page the sprint links and does not discard; a discard of a page the
// sprint has no draft of, edit of or waiver for; an overlap (store.PlanPublish); and a covers pattern a page to publish
// adds or changes that matches no file on the remote's main, fetched (MissingOnMain).
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
	drafted, err := store.Drafted(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, slug := range slices.Sorted(maps.Keys(discard)) {
		page := "design/" + slug + ".md"
		_, waived := Waivers(sprint)[slug]
		if !drafted[id][page] && !slices.Contains(dirty, page) && !waived {
			return nil, fmt.Errorf("--discard %s: sprint %s has no draft of the design page %s to drop", slug, id, slug)
		}
		paths = append(paths, page)
		if slices.Contains(dirty, page) {
			c.revert = append(c.revert, page)
		}
	}
	for _, slug := range records.DesignLinks(sprint) {
		if _, dropped := discard[slug]; !dropped && slices.Contains(dirty, "design/"+slug+".md") {
			page := "design/" + slug + ".md"
			return nil, fmt.Errorf("records/%s has uncommitted edits in this worktree's records workspace %s, and sprint "+
				"%s links it: pm commit it, which drafts it for the sprint, or drop the edits with --discard %s "+
				"--reason \"…\"; then pm sprint close %s again", page, w.Dir, id, slug, id)
		}
	}
	if c.Publish, err = store.PlanPublish(dir, id, paths); err != nil {
		return nil, err
	}
	added := map[string][]string{}
	for _, page := range slices.Sorted(maps.Keys(c.Pages)) {
		if c.Pages[page] == nil {
			continue
		}
		now, err := records.CoversOf(*c.Pages[page], page)
		if err != nil {
			return nil, err
		}
		var was []string
		if c.Was[page] != nil {
			if was, err = records.CoversOf(*c.Was[page], page); err != nil {
				return nil, fmt.Errorf("the page as the records branch holds it: %w", err)
			}
		}
		added[strings.TrimSuffix(page, ".md")] = slices.DeleteFunc(now, func(p string) bool {
			return slices.Contains(was, p)
		})
	}
	missing, err := MissingOnMain(root, remote, branch, added)
	if err != nil {
		return nil, fmt.Errorf("sprint %s: %w", id, err)
	}
	if missing != nil {
		return nil, fmt.Errorf("sprint %s: the design pages it publishes add covers patterns that name files %s/%s "+
			"does not have: %s. Fix each pattern to the file main has in the workspace that drafts the page and pm "+
			"commit it there, or merge the PR that adds the file first; then pm sprint close %s again", id, remote,
			branch, strings.Join(missing, ", "), id)
	}
	return c, nil
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
	return strings.Join(parts, "; ")
}

// Waive is the sprint record's text with a waiver decision (WaiverText), as pm design unchanged records it, for each
// page of discard (slug and reason) it does not waive yet, in slug order; block makes the decision block of a body.
func Waive(sprint *records.Record, discard map[string]string, block func(string) string) (string, error) {
	text, waived := sprint.Text, Waivers(sprint)
	for _, slug := range slices.Sorted(maps.Keys(discard)) {
		if _, ok := waived[slug]; ok {
			continue
		}
		var err error
		if text, err = records.InsertEntry(text, "Decisions", block(WaiverText(slug, discard[slug]))); err != nil {
			return "", err
		}
	}
	return text, nil
}
