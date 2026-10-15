package store

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"regexp"
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

// BranchDraft is one drafts/ branch as the clone's drafts read it, or another clone's (Clone set: its id, and Name
// its worktree's there): its unpublished pages' texts (nil: removed) and sprints, and the pages it marks in conflict,
// with theirs.
type BranchDraft struct {
	Workspace
	Clone     string
	Pages     map[string]*string
	Tags      map[string]string
	Conflicts map[string]string
}

// AllDrafts is every drafts/ branch of the clone that pm can read, then every other clone's draft as last fetched,
// and a warning naming each one it cannot (a commit made by hand, a missing base ref) with its fix, left out: one
// workspace's bad state fails only its own worktree's pm commands, never another's check or gate.
func AllDrafts(store string) ([]BranchDraft, []string, error) {
	branches, err := DraftBranches(store)
	if err != nil {
		return nil, nil, err
	}
	var out []BranchDraft
	var warnings []string
	for _, w := range branches {
		pages, tags, err := DraftPages(store, w)
		var d draftState
		if err == nil {
			d, err = readDraft(store, w)
		}
		if err != nil {
			warnings = append(warnings, unreadable(w, err))
			continue
		}
		out = append(out, BranchDraft{Workspace: w, Pages: pages, Tags: tags, Conflicts: d.conflicts})
	}
	fetched, err := FetchedDrafts(store)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range fetched {
		var pages map[string]*string
		err := f.Err
		if err == nil {
			pages, err = f.Pages(store)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("warning: %s cannot be read, so pm leaves it out here: %v",
				f.Label(), err))
			continue
		}
		out = append(out, BranchDraft{Workspace: Workspace{Name: f.Name}, Clone: f.Clone, Pages: pages,
			Tags: f.Tags(), Conflicts: f.Conflicts()})
	}
	return out, warnings, nil
}

func unreadable(w Workspace, err error) string {
	return fmt.Sprintf("warning: %s cannot be read, so pm leaves its drafts out here: %v", w.Branch(), err)
}

// Drafted is the unpublished design pages the clone's draft commits hold or mark in conflict, by sprint: each page's
// store path (design/x.md); and AllDrafts' warnings.
func Drafted(store string) (map[string]map[string]bool, []string, error) {
	drafts, warnings, err := AllDrafts(store)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]map[string]bool{}
	add := func(sprint, page string) {
		if out[sprint] == nil {
			out[sprint] = map[string]bool{}
		}
		out[sprint][page] = true
	}
	for _, b := range drafts {
		for p := range b.Pages {
			add(b.Tags[p], p)
		}
		for p, sprint := range b.Conflicts { // a page in conflict is still the sprint's draft
			add(sprint, p)
		}
	}
	return out, warnings, nil
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
		out = append(out, fmt.Sprintf("drafts    %s  %s", w.Branch(), orphaned(store, w)))
	}
	return out, nil
}

// orphaned is what a branch whose worktree is gone still holds: unpublished pages, and edits lost in conflict.
func orphaned(store string, w Workspace) string {
	d, err := readDraft(store, w)
	var pages []string
	if err == nil {
		pages, err = d.pending(store)
	}
	if err != nil {
		return "unreadable: " + err.Error()
	}
	var said, sprints []string
	var drafted int
	for _, p := range pages {
		if sprint, ok := d.tags[p]; ok {
			drafted++
			if !slices.Contains(sprints, sprint) {
				sprints = append(sprints, sprint)
			}
		}
	}
	if drafted > 0 {
		sort.Strings(sprints)
		said = append(said, fmt.Sprintf("a draft commit of %d design pages for sprint %s, published at the sprint's "+
			"close", drafted, strings.Join(sprints, ", ")))
	}
	for _, p := range slices.Sorted(maps.Keys(d.conflicts)) {
		said = append(said, fmt.Sprintf("its edit of records/%s for sprint %s was left in conflict and is lost with the "+
			"worktree; the mark holds that sprint's close until pm sprint close %s --discard %s", p, d.conflicts[p],
			d.conflicts[p], strings.TrimSuffix(path.Base(p), ".md")))
	}
	if said == nil {
		return "its worktree is gone and it holds no unpublished draft; the next pm sprint close deletes it"
	}
	return "its worktree is gone; " + strings.Join(said, "; ")
}

// Publish is what pm sprint close publishes for one sprint, planned without a write: the pages the sprint's drafts
// change on the records tip, and the drafts to drop them from.
type Publish struct {
	Tip   string             // the records tip the drafts merged onto
	Pages map[string]*string // each page the merge changes on the tip (design/x.md): its text, nil when removed
	Was   map[string]*string // each of Pages as the tip holds it, nil when it has none
	// Skipped is a warning for each draft pm cannot read and no commit of which names the sprint
	Skipped []string
	// Others is a note for each other clone's draft the close publishes pages from
	Others []string
	drops  []drop
}

// drop is a draft to take pages out of: the sprint's, published or discarded, and on a branch with no worktree also
// those an earlier close took; such a branch left with nothing is deleted.
type drop struct {
	w     Workspace
	d     draftState
	pages []string
}

// PlanPublish merges, from the records tip, the pages each draft commit tags with sprint, branch by branch, then each
// other clone's as last fetched, leaving out discard (store paths, design/x.md) and any page the records branch holds
// as a close of the sprint published it since the draft's base (a close stopped before it dropped the drafts). It
// changes nothing. An overlap, with what the records branch changed since a draft's base or with another draft of the
// sprint, is refused, naming for each page the commits or the draft, and the fix.
func PlanPublish(store, sprint string, discard []string) (*Publish, error) {
	tip, err := Git(store, "rev-parse", "refs/heads/"+Branch)
	if err != nil {
		return nil, err
	}
	branches, err := DraftBranches(store)
	if err != nil {
		return nil, err
	}
	fetched, err := FetchedDrafts(store)
	if err != nil {
		return nil, err
	}
	p := &Publish{Tip: tip}
	m := &merging{store: store, sprint: sprint, acc: tip, changedBy: map[string][]string{}}
	for _, w := range branches {
		d, err := readDraft(store, w)
		if err != nil {
			if mine, err2 := mayHold(store, w, sprint); err2 != nil || mine {
				return nil, errors.Join(errorf("%s may hold sprint %s's design pages, and pm cannot read it, so "+
					"nothing is published: %v", w.Branch(), sprint, err), err2)
			}
			p.Skipped = append(p.Skipped, unreadable(w, err))
			continue
		}
		x, take, err := sprintPages(store, w, d, sprint, discard)
		if err != nil {
			return nil, err
		}
		if len(x.pages) > 0 || (w.Dir == "" && !d.draft) {
			p.drops = append(p.drops, x)
		}
		m.overlaps = append(m.overlaps, conflicting(w, d, sprint, discard)...)
		if err := m.merge(w.Branch(), d, take, func(path string, others []string) (string, error) {
			return overlap(store, w, d, tip, path, sprint, others)
		}); err != nil {
			return nil, err
		}
	}
	for _, f := range fetched {
		if f.Err != nil {
			if f.mayHold(sprint) {
				return nil, errorf("%s may hold sprint %s's design pages, and pm cannot read it, so nothing is "+
					"published: %v", f.Label(), sprint, f.Err)
			}
			p.Skipped = append(p.Skipped, fmt.Sprintf("warning: %s cannot be read, so pm leaves it out here: %v",
				f.Label(), f.Err))
			continue
		}
		take, err := f.takes(store, sprint, discard)
		if err != nil {
			return nil, err
		}
		m.overlaps = append(m.overlaps, f.conflicting(sprint, discard)...)
		before := m.acc
		if err := m.merge(f.Label(), f.d, take, func(path string, others []string) (string, error) {
			return f.overlap(store, tip, path, sprint, others)
		}); err != nil {
			return nil, err
		}
		if m.acc != before {
			p.Others = append(p.Others, fmt.Sprintf("note: published %s from %s; that clone drops them from its "+
				"draft when it follows onto a records branch with this close", strings.Join(take, ", "), f.Label()))
		}
	}
	if m.overlaps != nil {
		return nil, errorf("sprint %s's design-page drafts cannot publish, so nothing is published: %s", sprint,
			strings.Join(m.overlaps, "; "))
	}
	paths, err := changedPaths(store, tip, m.acc)
	if err != nil {
		return nil, err
	}
	if p.Pages, err = blobs(store, m.acc, paths); err != nil {
		return nil, err
	}
	if p.Was, err = blobs(store, tip, paths); err != nil {
		return nil, err
	}
	return p, nil
}

// merging is PlanPublish's merge so far: acc, the records tip with the drafts merged so far; changedBy, each page with
// the drafts whose merge changed it; overlaps, why each page that overlaps cannot publish.
type merging struct {
	store, sprint, acc string
	changedBy          map[string][]string
	overlaps           []string
}

// merge merges take, pages of the draft d (named label), onto acc from the draft's base: an overlap is noted with
// why, which says why the draft cannot publish a page and the way out, and leaves acc as it was.
func (m *merging) merge(label string, d draftState, take []string, why func(string, []string) (string, error)) error {
	if len(take) == 0 {
		return nil
	}
	tree, err := withFiles(m.store, d.base, d.head, take) // the base with the sprint's pages from the draft
	if err != nil {
		return err
	}
	mine, err := gitIn(m.store, nil, "pm: the pages of "+label+" for "+m.sprint+"\n", "commit-tree", tree, "-p",
		d.base, "-F", "-")
	if err != nil {
		return err
	}
	merged, conflicted, err := mergeTree(m.store, d.base, m.acc, mine)
	if err != nil {
		return err
	}
	for _, path := range conflicted {
		said, err := why(path, m.changedBy[path])
		if err != nil {
			return err
		}
		m.overlaps = append(m.overlaps, said)
	}
	if len(conflicted) > 0 {
		return nil
	}
	next, err := gitIn(m.store, nil, "pm: publish "+m.sprint+"\n", "commit-tree", merged, "-p", m.acc, "-F", "-")
	if err != nil {
		return err
	}
	changed, err := changedPaths(m.store, m.acc, next)
	if err != nil {
		return err
	}
	for _, path := range changed {
		m.changedBy[path] = append(m.changedBy[path], label)
	}
	m.acc = next
	return nil
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

// mayHold is whether a commit of w's branch not on the records branch names sprint in a "Sprint:" or "Conflict:"
// line: an unreadable branch that may hold the sprint's pages.
func mayHold(store string, w Workspace, sprint string) (bool, error) {
	log, err := Git(store, "log", "--format=%B", "refs/heads/"+Branch+".."+w.Ref())
	if err != nil {
		return false, err
	}
	for _, l := range strings.Split(log, "\n") {
		for _, tag := range []*regexp.Regexp{sprintTag, conflictTag} {
			if m := tag.FindStringSubmatch(l); len(m) == 3 && m[2] == sprint {
				return true, nil
			}
		}
	}
	return false, nil
}

// PublishedBy is whether a close of sprint published page on the records branch (its "Published:" line), ever.
func PublishedBy(store, page, sprint string) (bool, error) {
	log, err := Git(store, "log", "--format=%B", "refs/heads/"+Branch, "--", page)
	if err != nil {
		return false, err
	}
	return slices.Contains(strings.Split(log, "\n"), publishedTag+page+" "+sprint), nil
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
	return publishedOn(store, "refs/heads/"+Branch, base, page, sprint)
}

// publishedOn is whether a commit of rev since base published page as sprint's (its "Published:" line).
func publishedOn(store, rev, base, page, sprint string) (bool, error) {
	log, err := Git(store, "log", "--format=%B", base+".."+rev, "--", page)
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
