package store

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Publish at sprint close (the record-drafts page): pm sprint close merges the pages each draft commit tags with the
// sprint, from every drafts/<name> branch of the clone, onto the records tip, and commits them in one records commit;
// then it drops them from the draft commits in one ref transaction. A workspace's draft commit keeps its tree and
// loses the pages' tags, so the workspace's files stay as they are until its own next follow drops the pages; a branch
// with no worktree loses them from its tree, and is deleted with its recorded base once nothing is left.

// publishedTag starts a line of the close's records commit for each page it publishes: "Published: <path> <sprint>".
const publishedTag = "Published: "

// DraftBranches is every drafts/<name> branch of the clone, by name, each as a Workspace whose Code and Dir are set
// when a worktree that is not gone has it checked out, else "": a branch whose worktree is gone.
func DraftBranches(store string) ([]Workspace, error) {
	refs, err := Git(store, "for-each-ref", "--format=%(refname:lstrip=2)", "refs/heads/"+DraftsPrefix)
	if err != nil {
		return nil, err
	}
	trees, err := Git(store, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	at := map[string]string{} // each checked-out branch's worktree
	for _, block := range strings.Split(trees, "\x00\x00") {
		path, branch, gone := "", "", false
		for _, field := range strings.Split(block, "\x00") {
			key, value, _ := strings.Cut(field, " ")
			switch key {
			case "worktree":
				path = value
			case "branch":
				branch = strings.TrimPrefix(value, "refs/heads/")
			case "prunable":
				gone = true
			}
		}
		if branch != "" && !gone {
			at[branch] = path
		}
	}
	var out []Workspace
	for _, b := range strings.Split(refs, "\n") {
		if b == "" {
			continue
		}
		w := Workspace{Name: strings.TrimPrefix(b, DraftsPrefix)}
		if dir, ok := at[b]; ok {
			w.Dir, w.Code = dir, filepath.Dir(dir)
		}
		out = append(out, w)
	}
	return out, nil
}

// Drafted is the unpublished design pages the clone's draft commits hold or mark in conflict, by sprint: each page's store path
// (design/x.md).
func Drafted(store string) (map[string]map[string]bool, error) {
	branches, err := DraftBranches(store)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, w := range branches {
		pages, tags, err := DraftPages(store, w)
		if err != nil {
			return nil, err
		}
		d, err := readDraft(store, w)
		if err != nil {
			return nil, err
		}
		for p := range pages {
			d.conflicts[p] = tags[p]
		}
		for p, sprint := range d.conflicts { // a page in conflict is still the sprint's draft
			if out[sprint] == nil {
				out[sprint] = map[string]bool{}
			}
			out[sprint][p] = true
		}
	}
	return out, nil
}

// Orphans is pm where's line for each drafts/ branch whose worktree is gone: what it still holds.
func Orphans(store string) ([]string, error) {
	branches, err := DraftBranches(store)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, w := range branches {
		if w.Dir != "" {
			continue
		}
		said := "its worktree is gone and it holds no unpublished draft; the next pm sprint close deletes it"
		d, err := readDraft(store, w)
		var pages []string
		if err == nil {
			pages, err = d.pending(store)
		}
		tags := d.tags
		if err != nil {
			said = "unreadable: " + err.Error()
		} else if len(pages) > 0 {
			var sprints []string
			for _, p := range pages {
				if !slices.Contains(sprints, tags[p]) {
					sprints = append(sprints, tags[p])
				}
			}
			sort.Strings(sprints)
			said = fmt.Sprintf("its worktree is gone; a draft commit of %d design pages for sprint %s, published at "+
				"the sprint's close", len(pages), strings.Join(sprints, ", "))
		}
		out = append(out, fmt.Sprintf("drafts    %s  %s", w.Branch(), said))
	}
	return out, nil
}

// Publish is what pm sprint close publishes for one sprint, planned without a write: the pages the sprint's drafts
// change on the records tip, and the drafts to drop them from.
type Publish struct {
	Tip   string             // the records tip the drafts merged onto
	Pages map[string]*string // each page the merge changes on the tip (design/x.md): its text, nil when removed
	Was   map[string]*string // each of Pages as the tip holds it, nil when it has none
	drops []drop
}

// drop is a draft to take pages out of: the sprint's, published or discarded, and on a branch with no worktree also
// those an earlier close took; such a branch left with nothing is deleted.
type drop struct {
	w     Workspace
	d     draftState
	pages []string
}

// PlanPublish merges, from the records tip, the pages each draft commit tags with sprint, branch by branch, leaving
// out discard (store paths, design/x.md) and any page the records branch holds as a close of the sprint published it
// since the draft's base (a close stopped before it dropped the drafts). It changes nothing. An overlap, with what the
// records branch changed since a draft's base or with another draft of the sprint, is refused, naming for each page
// the commits or the branch, and the fix.
func PlanPublish(store, sprint string, discard []string) (*Publish, error) {
	tip, err := Git(store, "rev-parse", "refs/heads/"+Branch)
	if err != nil {
		return nil, err
	}
	branches, err := DraftBranches(store)
	if err != nil {
		return nil, err
	}
	p := &Publish{Tip: tip}
	acc, changedBy := tip, map[string][]string{} // the merge so far; each page, the branches whose draft changed it
	var overlaps []string
	for _, w := range branches {
		d, err := readDraft(store, w)
		if err != nil {
			return nil, err
		}
		x, take, err := sprintPages(store, w, d, sprint, discard)
		if err != nil {
			return nil, err
		}
		if len(x.pages) > 0 || (w.Dir == "" && !d.draft) {
			p.drops = append(p.drops, x)
		}
		overlaps = append(overlaps, conflicting(w, d, sprint, discard)...)
		if len(take) == 0 {
			continue
		}
		tree, err := withFiles(store, d.base, d.head, take) // the base with the sprint's pages from the draft
		if err != nil {
			return nil, err
		}
		mine, err := gitIn(store, nil, "pm: the pages of "+w.Branch()+" for "+sprint+"\n", "commit-tree", tree, "-p",
			d.base, "-F", "-")
		if err != nil {
			return nil, err
		}
		merged, conflicted, err := mergeTree(store, d.base, acc, mine)
		if err != nil {
			return nil, err
		}
		for _, path := range conflicted {
			why, err := overlap(store, w, d, tip, path, sprint, changedBy[path])
			if err != nil {
				return nil, err
			}
			overlaps = append(overlaps, why)
		}
		if len(conflicted) > 0 {
			continue
		}
		next, err := gitIn(store, nil, "pm: publish "+sprint+"\n", "commit-tree", merged, "-p", acc, "-F", "-")
		if err != nil {
			return nil, err
		}
		changed, err := changedPaths(store, acc, next)
		if err != nil {
			return nil, err
		}
		for _, path := range changed {
			changedBy[path] = append(changedBy[path], w.Branch())
		}
		acc = next
	}
	if overlaps != nil {
		return nil, errorf("sprint %s's design-page drafts cannot publish, so nothing is published: %s", sprint,
			strings.Join(overlaps, "; "))
	}
	paths, err := changedPaths(store, tip, acc)
	if err != nil {
		return nil, err
	}
	if p.Pages, err = blobs(store, acc, paths); err != nil {
		return nil, err
	}
	if p.Was, err = blobs(store, tip, paths); err != nil {
		return nil, err
	}
	return p, nil
}

// sprintPages is what the close takes from one draft: the drop (the sprint's pages; on a branch with no worktree also
// those an earlier close took), and the sprint's pages to publish, sorted.
func sprintPages(store string, w Workspace, d draftState, sprint string, discard []string) (drop, []string, error) {
	x := drop{w: w, d: d}
	var take []string
	for page, s := range d.conflicts {
		if s == sprint && slices.Contains(discard, page) {
			x.pages = append(x.pages, page)
		}
	}
	for page, s := range d.tags {
		if s != sprint {
			continue
		}
		x.pages = append(x.pages, page)
		if slices.Contains(discard, page) {
			continue
		}
		done, err := publishedSince(store, d.base, page, sprint)
		if err != nil {
			return drop{}, nil, err
		}
		if !done {
			take = append(take, page)
		}
	}
	if w.Dir == "" { // with no worktree to follow, the drop takes every published page out: untagged or named so
		closed, err := d.closed(store)
		if err != nil {
			return drop{}, nil, err
		}
		pending, err := d.pending(store)
		if err != nil {
			return drop{}, nil, err
		}
		for page, s := range d.tags {
			if s != sprint && !slices.Contains(pending, page) {
				closed = append(closed, page)
			}
		}
		x.pages = append(x.pages, closed...)
	}
	sort.Strings(x.pages)
	sort.Strings(take)
	return x, take, nil
}

// conflicting is why each page the draft marks as the sprint's, in conflict, holds the close, unless discarded: a
// follow left the edit with conflict markers, uncommitted, so the draft commit no longer holds it.
func conflicting(w Workspace, d draftState, sprint string, discard []string) []string {
	var out []string
	for _, page := range slices.Sorted(maps.Keys(d.conflicts)) {
		if d.conflicts[page] != sprint || slices.Contains(discard, page) {
			continue
		}
		slug := strings.TrimSuffix(path.Base(page), ".md")
		if w.Dir == "" {
			out = append(out, fmt.Sprintf("records/%s on %s holds the sprint's edit in conflict, uncommitted, and its "+
				"worktree is gone with the edit: redo it in a workspace of your own and pm commit --sprint %s it, or "+
				"drop it with --discard %s --reason \"…\"", page, w.Branch(), sprint, slug))
			continue
		}
		out = append(out, fmt.Sprintf("records/%s in worktree %s holds the sprint's edit with conflict markers a follow "+
			"left, uncommitted: resolve them there and pm commit --sprint %s it, or drop it with --discard %s "+
			"--reason \"…\"", page, w.Code, sprint, slug))
	}
	return out
}

// publishedSince is whether a records commit since base published page as sprint's (its "Published:" line).
func publishedSince(store, base, page, sprint string) (bool, error) {
	log, err := Git(store, "log", "--format=%B", base+"..refs/heads/"+Branch, "--", page)
	if err != nil {
		return false, err
	}
	return slices.Contains(strings.Split(log, "\n"), publishedTag+page+" "+sprint), nil
}

// pending is the draft commit's pages not yet published, sorted: each tagged page that no close of its sprint
// published since the draft's base.
func (d draftState) pending(store string) ([]string, error) {
	var out []string
	for page := range d.conflicts {
		out = append(out, page)
	}
	for page, sprint := range d.tags {
		done, err := publishedSince(store, d.base, page, sprint)
		if err != nil {
			return nil, err
		}
		if !done {
			out = append(out, page)
		}
	}
	sort.Strings(out)
	return out, nil
}

// overlap is why the draft on w cannot publish path, and the way out: the records commits that changed it since the
// draft's base, or the sprint's earlier drafts (others) that changed it in this merge.
func overlap(store string, w Workspace, d draftState, tip, path, sprint string, others []string) (string, error) {
	log, err := Git(store, "log", "--reverse", "--cherry-pick", "--right-only", "--format=%h %s", d.base+"..."+tip,
		"--", path)
	if err != nil {
		return "", err
	}
	var commits []string
	for _, l := range strings.Split(log, "\n") {
		if l != "" {
			commits = append(commits, l)
		}
	}
	page := fmt.Sprintf("records/%s on %s", path, w.Branch())
	switch {
	case len(commits) > 0 && w.Dir != "":
		return fmt.Sprintf("%s overlaps what the records branch published since its base, in %s: bring the records "+
			"branch into that workspace with any pm write in %s (pm commit with no path does, and lists what is "+
			"uncommitted), which leaves conflict markers in its records/%s; resolve them, pm commit --sprint %s it, "+
			"then pm sprint close %s again", page, strings.Join(commits, "; "), w.Code, path, sprint, sprint), nil
	case len(commits) > 0:
		return fmt.Sprintf("%s overlaps what the records branch published since its base, in %s, and its worktree is "+
			"gone: put its text (git -C %s show %s:%s) into records/%s of a worktree of your own, resolve it with the "+
			"published one and pm commit --sprint "+sprint+" it there, delete the old draft with git -C %s branch -D %s once you took "+
			"every page it holds, then pm sprint close %s again", page, strings.Join(commits, "; "), store,
			w.Branch(), path, path, store, w.Branch(), sprint), nil
	default:
		return fmt.Sprintf("%s overlaps the sprint's draft of it on %s: bring both edits into one workspace's "+
			"records/%s and pm commit --sprint %s it there, put the page back to its published text in the other and "+
			"pm commit --sprint %s it there, which drops it from that draft, then pm sprint close %s again", page,
			strings.Join(others, ", "), path, sprint, sprint, sprint), nil
	}
}

// Trailer is the close's records-commit lines naming each page it publishes as the sprint's.
func (p *Publish) Trailer(sprint string) string {
	var out []string
	for _, path := range slices.Sorted(maps.Keys(p.Pages)) {
		out = append(out, publishedTag+path+" "+sprint)
	}
	return strings.Join(out, "\n")
}

// Drop takes the published and discarded pages out of the drafts in one ref transaction, after the close's records
// commit: a workspace's draft commit keeps its tree and loses their tags (its next follow drops them), a branch with
// no worktree loses them from its tree, and one left with nothing is deleted with its recorded base.
func (p *Publish) Drop(store string) error {
	var script strings.Builder
	for _, x := range p.drops {
		old := x.d.head
		tags, marks := maps.Clone(x.d.tags), maps.Clone(x.d.conflicts)
		for _, page := range x.pages {
			delete(tags, page)
			delete(marks, page)
		}
		tree := old
		if x.w.Dir == "" && x.d.draft {
			var err error
			if tree, err = withFiles(store, old, x.d.base, x.pages); err != nil {
				return err
			}
		}
		paths, err := changedPaths(store, x.d.base, tree)
		if err != nil {
			return err
		}
		if x.w.Dir == "" && len(paths) == 0 && len(marks) == 0 {
			fmt.Fprintf(&script, "delete %s %s\ndelete %s\n", x.w.Ref(), old, x.w.BaseRef())
			continue
		}
		head, err := gitIn(store, nil, draftMessage(x.w, paths, tags, marks), "commit-tree", tree+"^{tree}", "-p", x.d.base,
			"-F", "-")
		if err != nil {
			return err
		}
		fmt.Fprintf(&script, "update %s %s %s\n", x.w.Ref(), head, old)
	}
	if script.Len() == 0 {
		return nil
	}
	_, err := gitIn(store, nil, "start\n"+script.String()+"prepare\ncommit\n", "update-ref", "-m",
		"pm: sprint close", "--stdin")
	return err
}

// Revert puts back each of rels, uncommitted in the workspace, to its text on the workspace's branch.
func (w Workspace) Revert(store string, rels []string) error {
	texts, err := blobs(store, w.Ref(), rels)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if err := w.put(rel, texts[rel]); err != nil {
			return err
		}
	}
	return nil
}
