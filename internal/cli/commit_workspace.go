package cli

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// isDesignPage is whether a store path (design/x.md) is a design page.
func isDesignPage(rel string) bool { return path.Dir(rel) == "design" && strings.HasSuffix(rel, ".md") }

// commitWorkspace is pm commit in a worktree with a records workspace (the record-drafts page, Writes and journal
// publish): follow; refuse a named file with conflict markers, naming the records commits that touched it; put the
// named design pages into the workspace's draft commit, each tagged with its sprint; commit the named journals and the
// draft's pages on the records branch in the store, refusing a file the store has uncommitted edits to; follow again.
// Until publish at sprint close is built, the draft's pages publish here with the journals (the sprint's decision).
func (e *env) commitWorkspace(w store.Workspace, message string, p *Parsed) (string, error) {
	followed, err := store.Follow(e.records, w)
	if err != nil {
		return "", err
	}
	for _, rel := range followed.Conflicts {
		fmt.Fprintf(e.stdout, "records/%s: the records branch changed it under your uncommitted edit; resolve the "+
			"conflict markers there\n", rel)
	}
	dirty, err := w.Dirty()
	if err != nil {
		return "", err
	}
	if len(dirty) == 0 {
		return "", refuse("nothing to commit in %s", w.Dir)
	}
	names := p.Values["paths"]
	if len(names) == 0 {
		listing := make([]string, len(dirty))
		for i, rel := range dirty {
			listing[i] = "  records/" + rel
		}
		return "", refuse("name the records you edited: pm commit -m \"…\" <path>…. Uncommitted in %s:\n%s", w.Dir,
			strings.Join(listing, "\n"))
	}
	rels, err := w.Named(e.here, names, dirty)
	if err != nil {
		return "", err
	}
	texts := map[string]*string{}
	changes := map[string]*string{}
	for _, rel := range rels {
		p := filepath.Join(w.Dir, filepath.FromSlash(rel))
		if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", refuse("records/%s is a symlink; a record is a file", rel)
		}
		if strings.HasSuffix(rel, ".summary.json") && fsutil.Exists(p) {
			if _, err := records.ReadSummary(p); err != nil {
				return "", err
			}
		}
		b, err := os.ReadFile(p)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil {
			text := string(b)
			if store.HasMarkers(text) {
				return "", e.markersRefusal(rel, text)
			}
			texts[rel] = &text
		}
		changes[filepath.Join(e.records, filepath.FromSlash(rel))] = texts[rel]
	}
	pages, err := e.checkWorkspaceCommit(w, rels, changes, p.Get("sprint"))
	if err != nil {
		return "", err
	}
	return e.publishWorkspace(w, message, rels, texts, pages)
}

// markersRefusal refuses a file with conflict markers, naming the records commits that changed it under the edit.
func (e *env) markersRefusal(rel, text string) error {
	commits, err := store.MarkerCommits(e.records, rel, text)
	if err != nil {
		return err
	}
	by := ""
	if len(commits) > 0 {
		by = ", which records changed under your edit in " + strings.Join(commits, "; ")
	}
	return refuse("records/%s has conflict markers%s: keep both edits between the <<<<<<< and >>>>>>> lines, remove "+
		"the markers, then pm commit again", rel, by)
}

// checkWorkspaceCommit checks the named files as the records branch would hold them, and gives each named design
// page's sprint; it refuses a file the store has uncommitted edits to, which the commit would overwrite.
func (e *env) checkWorkspaceCommit(w store.Workspace, rels []string, changes map[string]*string,
	sprint string) (map[string]string, error) {
	for p, text := range changes {
		if text == nil {
			if err := records.LinkToNothing(fsutil.Resolved(e.records), p); err != nil {
				return nil, err
			}
		}
	}
	recs, err := store.Committed(e.records, changes)
	if err != nil {
		return nil, err
	}
	ws, err := e.work()
	if err != nil {
		return nil, err
	}
	items, err := ws.Items()
	if err != nil {
		return nil, err
	}
	x := records.NewItems(items)
	if _, err := site.Check(recs, x, filepath.Base(w.Code), nil); err != nil {
		return nil, err
	}
	pages := map[string]string{}
	for _, rel := range rels {
		if isDesignPage(rel) {
			if pages[rel], err = designSprint(recs, x, rel, sprint); err != nil {
				return nil, err
			}
		}
	}
	paths := make([]string, 0, len(changes))
	for p := range changes {
		paths = append(paths, p)
	}
	held, err := store.Uncommitted(e.records, paths)
	if err != nil {
		return nil, err
	}
	if len(held) > 0 {
		return nil, refuse("records/%s has uncommitted edits in the shared store %s, a main-checkout session's; "+
			"this commit would overwrite them: commit or revert them there first", held[0], e.records)
	}
	return pages, nil
}

// designSprint is the sprint a design page's draft belongs to: the open sprint whose Design pages link it and in which
// this session holds an open task; given names one of several. It refuses when none does.
func designSprint(recs []*records.Record, x *records.Items, rel, given string) (string, error) {
	slug := strings.TrimSuffix(path.Base(rel), ".md")
	session := currentSession()
	var linking, mine []string
	for _, r := range recs {
		it := x.Get(r.Bead())
		if r.Type() != "sprint" || it == nil || it.Status == work.Closed || !slices.Contains(records.DesignLinks(r), slug) {
			continue
		}
		linking = append(linking, it.ID)
		for _, t := range x.All() {
			if t.Holder != nil && session != "" && t.Holder.Session == session && t.Status != work.Closed &&
				slices.Contains(x.Ancestors(t.ID), it.ID) {
				mine = append(mine, it.ID)
				break
			}
		}
	}
	switch {
	case given != "" && slices.Contains(linking, given):
		return given, nil
	case given != "":
		return "", refuse("--sprint %s: that sprint is not open or does not link records/%s in its Design pages; the "+
			"open sprints that do: %s", given, rel, orNone(linking))
	case len(mine) == 1:
		return mine[0], nil
	case len(mine) == 0:
		return "", refuse("records/%s: no open sprint in which this session holds a task links it in its Design pages; "+
			"link it there first (the sprint's ## Design pages: [title](../design/%s.md)), then pm commit again; open "+
			"sprints that link it: %s", rel, slug, orNone(linking))
	default:
		return "", refuse("records/%s is linked by several open sprints in which this session holds a task (%s); "+
			"name one with --sprint ID", rel, strings.Join(mine, ", "))
	}
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

// publishWorkspace puts the design pages into the draft commit, then commits the journals and the draft's pages on
// the records branch, each page's sprint in the message; a failed commit puts the draft back. Then it follows.
func (e *env) publishWorkspace(w store.Workspace, message string, rels []string, texts map[string]*string,
	pages map[string]string) (string, error) {
	old := ""
	if len(pages) > 0 {
		var err error
		if old, err = store.CommitDraft(e.records, w, pages); err != nil {
			return "", err
		}
	}
	drafted, tags, err := store.DraftPages(e.records, w)
	if err != nil {
		return "", err
	}
	var writes []store.Write
	add := func(rel string, text *string) {
		wr := store.Write{Path: filepath.Join(e.records, filepath.FromSlash(rel)), Remove: text == nil}
		if text != nil {
			wr.Text = *text
		}
		writes = append(writes, wr)
	}
	for _, rel := range rels {
		if _, ok := pages[rel]; !ok {
			add(rel, texts[rel])
		}
	}
	trailer := ""
	for _, rel := range slices.Sorted(maps.Keys(drafted)) {
		add(rel, drafted[rel])
		trailer += fmt.Sprintf("\nSprint: %s %s", rel, tags[rel])
	}
	if trailer != "" {
		message += "\n" + trailer
	}
	if err := store.Apply(e.records, writes, message, "", ""); err != nil {
		if old != "" {
			err = errors.Join(err, store.ResetDraft(e.records, w, old))
		}
		return "", err
	}
	if _, err := store.Follow(e.records, w); err != nil {
		return "", err
	}
	head, err := store.Git(e.records, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	named := make([]string, len(rels))
	for i, rel := range rels {
		named[i] = "records/" + rel
	}
	return fmt.Sprintf("committed %s as %s: %s", strings.Join(named, ", "), head, strings.SplitN(message, "\n", 2)[0]),
		nil
}
