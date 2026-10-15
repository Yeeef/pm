package store

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Yeeef/pm/internal/item"
)

// Follow brings a workspace onto the records tip; the draft commit and pm commit's publish build on it. pm never runs
// git rebase, merge or stash in a workspace: it merges with git merge-tree --write-tree and git merge-file, writes
// commits with git commit-tree, moves the branch with git update-ref, resets the workspace's index with git read-tree
// and writes the files itself. Each runs under the store lock and completes or changes nothing.

// draftSubject starts a draft commit's message; each of its unpublished pages has a line "Sprint: <path> <sprint id>".
// A page the draft commit changes with no such line was published or discarded by its sprint's close
// (drafts_publish.go), which leaves the tree as it was: the next follow drops it. A page a follow took out of the
// draft commit with conflict markers keeps its sprint in a line "Conflict: <path> <sprint id>" until it is committed
// again or the workspace drops the edit, so the sprint's close refuses while it waits.
const draftSubject = "pm draft: "

var (
	sprintTag   = regexp.MustCompile(`(?m)^Sprint: (\S+) (\S+)$`)
	conflictTag = regexp.MustCompile(`(?m)^Conflict: (\S+) (\S+)$`)
)

// draftState is a workspace's branch: its commit, the records commit it is on, and its draft commit's pages' sprints.
type draftState struct {
	head, base string
	draft      bool              // head is a draft commit on base
	tags       map[string]string // each unpublished page of the draft commit (design/x.md): its sprint
	conflicts  map[string]string // each page a follow left with conflict markers, uncommitted: its sprint
}

// closed is the pages the draft commit changes with no sprint: published or discarded by their sprint's close.
func (d draftState) closed(store string) ([]string, error) {
	if !d.draft {
		return nil, nil
	}
	paths, err := changedPaths(store, d.base, d.head)
	return slices.DeleteFunc(paths, func(p string) bool { _, ok := d.tags[p]; return ok }), err
}

func readDraft(store string, w Workspace) (draftState, error) {
	base, err := Git(store, "rev-parse", "--verify", "--quiet", w.BaseRef())
	if err != nil {
		return draftState{}, errorf("%s has no recorded base (%s); remove the workspace with git worktree remove %s "+
			"and run %s in %s", w.Branch(), w.BaseRef(), w.Dir, Setup, w.Code)
	}
	out, err := Git(store, "log", "-1", "--format=%H%x00%P%x00%B", w.Ref())
	if err != nil {
		return draftState{}, err
	}
	parts := strings.SplitN(out, "\x00", 3)
	if len(parts) != 3 {
		return draftState{}, fmt.Errorf("git log -1 %s printed %q", w.Ref(), out)
	}
	d := draftState{head: parts[0], base: base, tags: map[string]string{}, conflicts: map[string]string{}}
	switch {
	case d.head == base:
	case strings.HasPrefix(parts[2], draftSubject) && parts[1] == base:
		d = draftOf(d.head, base, parts[2])
	default:
		return draftState{}, errorf("%s is at %s, a commit pm did not make on its base %s: a commit made by hand in %s? "+
			"pm commits there itself; undo it with git -C %s reset --soft %s, which keeps its edits uncommitted, then pm "+
			"commit them", w.Branch(), d.head, base, w.Dir, w.Dir, base)
	}
	return d, nil
}

// draftOf is the draft commit head on base, its pages' sprints and its conflict marks read from its message.
func draftOf(head, base, message string) draftState {
	d := draftState{head: head, base: base, draft: true, tags: map[string]string{}, conflicts: map[string]string{}}
	for _, m := range sprintTag.FindAllStringSubmatch(message, -1) {
		d.tags[m[1]] = m[2]
	}
	for _, m := range conflictTag.FindAllStringSubmatch(message, -1) {
		d.conflicts[m[1]] = m[2]
	}
	return d
}

// gitIn is git in dir with env added and stdin fed; stdout stripped, a failure a records error with git's message.
func gitIn(dir string, env []string, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env, cmd.Stdin = dir, append(os.Environ(), env...), strings.NewReader(stdin)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		return "", errorf("git %s failed in %s: %s", strings.Join(args, " "), dir, strings.TrimSpace(e.String()))
	}
	return strings.TrimSpace(o.String()), nil
}

// blobs is the text of each of paths at rev, nil for a path rev has no file at.
func blobs(store, rev string, paths []string) (map[string]*string, error) {
	out := map[string]*string{}
	for _, p := range paths {
		out[p] = nil
	}
	if len(paths) == 0 {
		return out, nil
	}
	list, err := Git(store, append([]string{"ls-tree", "-r", "-z", "--full-tree", rev, "--"}, paths...)...)
	if err != nil {
		return nil, err
	}
	for _, entry := range strings.Split(list, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		text, _, err := run(store, "cat-file", "blob", fields[2])
		if err != nil {
			return nil, err
		}
		out[path] = &text
	}
	return out, nil
}

// changedPaths is the paths whose file differs between the commits or trees a and b.
func changedPaths(store, a, b string) ([]string, error) {
	out, err := Git(store, "diff", "--name-only", "-z", "--no-renames", a, b, "--")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\x00"), "\x00"), nil
}

// mergeTree merges the draft commit onto tip from base: the merged tree, in which each conflicted path holds tip's
// file, and the conflicted paths.
func mergeTree(store, base, tip, draft string) (tree string, conflicted []string, err error) {
	out, errOut, err := run(store, "merge-tree", "--write-tree", "--name-only", "-z", "--no-messages",
		"--merge-base", base, tip, draft)
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", nil, errorf("git merge-tree failed in %s: %s", store, strings.TrimSpace(errOut))
	}
	fields := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	tree, conflicted = fields[0], fields[1:]
	if len(conflicted) == 0 {
		return tree, nil, nil
	}
	tree, err = withFiles(store, tree, tip, conflicted)
	return tree, conflicted, err
}

// withFiles is tree with each of paths as rev has it (absent where rev has none), through a scratch index.
func withFiles(store, tree, rev string, paths []string) (string, error) {
	tmp, err := os.MkdirTemp("", "pm-draft-index")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // a temp dir left behind breaks nothing
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitIn(store, env, "", "read-tree", tree); err != nil {
		return "", err
	}
	var info strings.Builder
	for _, p := range paths {
		entry, err := Git(store, "ls-tree", "--full-tree", rev, "--", p)
		if err != nil {
			return "", err
		}
		if entry == "" {
			entry = "0 0000000000000000000000000000000000000000\t" + p // mode 0 removes the path
		}
		info.WriteString(entry + "\n")
	}
	if _, err := gitIn(store, env, info.String(), "update-index", "--index-info"); err != nil {
		return "", err
	}
	return gitIn(store, env, "", "write-tree")
}

// mergeFile is git merge-file of the three texts (nil: no file, merged as empty), labelled for the reader: the result,
// and whether it holds conflict markers. theirs names the records commits the edit is merged with.
func mergeFile(base, ours, theirs *string, label string) (string, bool, error) {
	tmp, err := os.MkdirTemp("", "pm-merge")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // a temp dir left behind breaks nothing
	var files []string
	for i, t := range []*string{ours, base, theirs} {
		p := filepath.Join(tmp, fmt.Sprint(i))
		text := ""
		if t != nil {
			text = *t
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			return "", false, err
		}
		files = append(files, p)
	}
	cmd := exec.Command("git", "merge-file", "-p", "-L", "workspace", "-L", "base", "-L", label,
		files[0], files[1], files[2])
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return o.String(), false, nil
	case errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() < 128:
		return o.String(), true, nil
	default:
		return "", false, errorf("git merge-file failed: %s", strings.TrimSpace(e.String()))
	}
}

func same(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// Followed is what a follow did: the base it found, the records tip it moved to, and the files it left with conflict
// markers.
type Followed struct {
	Base, Tip string
	Conflicts []string
	Binaries  []string          // uncommitted binary files the records branch changed under: kept whole, noted for pm commit
	Stranded  map[string]string // draft pages whose sprint closed, with it: left the draft commit, uncommitted
}

// Follow brings the workspace onto the records tip, under the store lock the caller holds:
//
//  1. the draft commit, if any, less the pages it holds with no sprint (published or discarded at their sprint's close),
//     is merged onto the tip with git merge-tree --write-tree from its base; a page the records branch renamed since
//     the base keeps its sprint at its new path, where the merge puts the draft's edit;
//  2. each file whose HEAD text moves and that is uncommitted in the workspace is merged with git merge-file (old HEAD
//     text, working text, new HEAD text), conflict markers where hunks overlap;
//  3. a draft page that conflicts in 1 leaves the draft commit: its file keeps the agent's text merged with the tip's,
//     with markers, as an uncommitted edit;
//  4. a draft page, or a conflict mark, whose sprint closed (closed; another clone's close, which published what this
//     clone last pushed) leaves the draft commit too: its file keeps the draft's text as an uncommitted edit, noted as
//     stranded (Stranded), to commit under an open sprint or revert;
//  5. the branch moves to the tip plus the new draft commit; a file not uncommitted is refreshed from the new tree.
//
// The markers name the records range merged in (records <base>..<tip>), which pm commit reads to name the commits. A
// binary file is not merged: an uncommitted one stays as the workspace has it. It writes the files, then moves the
// branch and the index; a failure puts back what it changed.
func Follow(store string, w Workspace, sprints func() (SprintClosed, error)) (Followed, error) {
	tip, err := Git(store, "rev-parse", "refs/heads/"+Branch)
	if err != nil {
		return Followed{}, err
	}
	d, err := readDraft(store, w)
	if err != nil {
		return Followed{}, err
	}
	untagged, err := d.closed(store)
	if err != nil {
		return Followed{}, err
	}
	closed := func(string) bool { return false }
	if len(d.tags)+len(d.conflicts) > 0 { // the work store is read only for a draft that names a sprint
		if closed, err = sprints(); err != nil {
			return Followed{}, err
		}
	}
	f := Followed{Base: d.base, Tip: tip}
	if d.base == tip && len(untagged) == 0 && !d.closedSprint(closed) {
		return f, w.syncIndex() // a follow stopped between the branch and the index leaves the index behind
	}
	head, full, conflicted := tip, tip, []string(nil)
	var stranded map[string]string
	if d.draft {
		draft, err := withoutPages(store, d, untagged)
		if err != nil {
			return Followed{}, err
		}
		tags, err := renamedTags(store, d, tip)
		if err != nil {
			return Followed{}, err
		}
		var tree string
		if tree, conflicted, err = mergeTree(store, d.base, tip, draft); err != nil {
			return Followed{}, err
		}
		marks, err := w.conflictMarks(d, tags, conflicted)
		if err != nil {
			return Followed{}, err
		}
		if full, err = draftCommit(store, w, tree, tip, tags, marks); err != nil {
			return Followed{}, err
		}
		if head, stranded, err = strand(store, w, tree, tip, tags, marks, closed); err != nil {
			return Followed{}, err
		}
	}
	plan, err := followPlan(store, w, d, full, conflicted, &f) // the files as the draft holds them, stranded or not
	if err != nil {
		return Followed{}, err
	}
	if err := w.move(store, d.head, head, tip, plan); err != nil {
		return Followed{}, err
	}
	f.Stranded = stranded
	if err := w.noteStranded(stranded, nil); err != nil {
		return Followed{}, err
	}
	return f, w.noteBinaries(d.base, tip, f.Binaries)
}

// SprintClosed is whether the sprint with this id is closed in the work store.
type SprintClosed func(id string) bool

// ClosedSprints is SprintClosed over the work store's items.
func ClosedSprints(items []item.Item) SprintClosed {
	closed := map[string]bool{}
	for _, it := range items {
		if it.Type == item.Sprint && it.Status == item.Closed {
			closed[it.ID] = true
		}
	}
	return func(id string) bool { return closed[id] }
}

// closedSprint is whether a page or a conflict mark of the draft names a closed sprint.
func (d draftState) closedSprint(closed SprintClosed) bool {
	for _, m := range []map[string]string{d.tags, d.conflicts} {
		for _, sprint := range m {
			if closed(sprint) {
				return true
			}
		}
	}
	return false
}

// strand is the draft commit of tree on tip less each page whose sprint closed, and less each conflict mark that names
// one: the commit, and the stranded pages with their sprints. Their files keep the draft's text, uncommitted.
func strand(store string, w Workspace, tree, tip string, tags, marks map[string]string, closed SprintClosed) (string,
	map[string]string, error) {
	pages, err := changedPaths(store, tip, tree)
	if err != nil {
		return "", nil, err
	}
	stranded := map[string]string{}
	for _, p := range pages {
		if closed(tags[p]) {
			stranded[p] = tags[p]
		}
	}
	marks = maps.Clone(marks)
	for p, sprint := range marks {
		if closed(sprint) {
			stranded[p] = sprint
			delete(marks, p)
		}
	}
	if len(stranded) > 0 {
		if tree, err = withFiles(store, tree, tip, slices.Collect(maps.Keys(stranded))); err != nil {
			return "", nil, err
		}
	}
	head, err := draftCommit(store, w, tree, tip, tags, marks)
	return head, stranded, err
}

// conflictMarks is the conflict marks the draft commit keeps across a follow: each page the merge conflicts on, with
// its sprint, and each marked before whose file is still uncommitted (one reverted by hand dropped the edit).
func (w Workspace) conflictMarks(d draftState, tags map[string]string, conflicted []string) (map[string]string, error) {
	marks := map[string]string{}
	if len(d.conflicts) > 0 {
		dirty, err := w.Dirty()
		if err != nil {
			return nil, err
		}
		for p, sprint := range d.conflicts {
			if slices.Contains(dirty, p) {
				marks[p] = sprint
			}
		}
	}
	for _, p := range conflicted {
		if sprint := tags[p]; sprint != "" {
			marks[p] = sprint
		}
	}
	return marks, nil
}

// withoutPages is the draft commit with paths as its base has them: a commit on the base, or the base itself when
// nothing is left; the draft commit itself when paths is empty.
func withoutPages(store string, d draftState, paths []string) (string, error) {
	if len(paths) == 0 {
		return d.head, nil
	}
	tree, err := withFiles(store, d.head, d.base, paths)
	if err != nil {
		return "", err
	}
	if same, err := Git(store, "rev-parse", d.base+"^{tree}"); err != nil || same == tree {
		return d.base, err
	}
	return gitIn(store, nil, draftSubject+"less its closed pages\n", "commit-tree", tree, "-p", d.base, "-F", "-")
}

// renamedTags is the draft's tags, with each tagged page that the records branch renamed between the base and tip
// tagged at its new path too (git diff --find-renames, the rename detection git merge-tree runs).
func renamedTags(store string, d draftState, tip string) (map[string]string, error) {
	tags := maps.Clone(d.tags)
	if len(tags) == 0 {
		return tags, nil
	}
	out, err := Git(store, "diff", "--name-status", "-z", "--find-renames", d.base, tip, "--")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if !strings.HasPrefix(fields[i], "R") || i+2 >= len(fields) {
			continue
		}
		if sprint, ok := d.tags[fields[i+1]]; ok {
			tags[fields[i+2]] = sprint
		}
		i++ // a rename lists two paths
	}
	return tags, nil
}

// syncIndex resets the workspace's index to its HEAD's tree when they differ; the working tree is untouched.
func (w Workspace) syncIndex() error {
	if _, _, err := run(w.Dir, "diff-index", "--cached", "--quiet", "HEAD", "--"); err == nil {
		return nil
	}
	if _, err := Git(w.Dir, "read-tree", "HEAD"); err != nil {
		return err
	}
	run(w.Dir, "update-index", "-q", "--refresh") //nolint:errcheck // exits 1 while files differ, which they may
	return nil
}

// planned is a file Follow rewrites: the text it read there, and its new text (nil: no file).
type planned struct{ was, text *string }

// followPlan is each workspace file Follow rewrites, with the text it read and its new text (nil: removed), and
// f.Conflicts and f.Binaries set.
func followPlan(store string, w Workspace, d draftState, head string, conflicted []string, f *Followed) (
	map[string]planned, error) {
	moved, err := changedPaths(store, d.head, head)
	if err != nil {
		return nil, err
	}
	paths := slices.Sorted(maps.Keys(setOf(append(moved, conflicted...))))
	dirtyList, err := w.Dirty()
	if err != nil {
		return nil, err
	}
	dirty, isConflicted := setOf(dirtyList), setOf(conflicted)
	was, err := blobs(store, d.head, paths)
	if err != nil {
		return nil, err
	}
	now, err := blobs(store, head, paths)
	if err != nil {
		return nil, err
	}
	base, err := blobs(store, d.base, paths)
	if err != nil {
		return nil, err
	}
	from, err := Git(store, "rev-parse", "--short", d.base)
	if err != nil {
		return nil, err
	}
	to, err := Git(store, "rev-parse", "--short", f.Tip)
	if err != nil {
		return nil, err
	}
	label := "records " + from + ".." + to
	plan := map[string]planned{}
	for _, p := range paths {
		working, err := w.read(p)
		if err != nil {
			return nil, err
		}
		agent := was[p]
		if dirty[p] {
			agent = working
		}
		var text *string
		switch {
		case isConflicted[p]: // the tip's text in the new draft: the agent's text merged with it, uncommitted
			text, err = mergedText(base[p], agent, now[p], label, p, f)
		case dirty[p]:
			text, err = mergedText(was[p], agent, now[p], label, p, f)
		default:
			text = now[p]
		}
		if err != nil {
			return nil, err
		}
		if !same(text, working) {
			plan[p] = planned{was: working, text: text}
		}
	}
	return plan, nil
}

// mergedText is ours merged with theirs from base, nil when ours removed the file and the merge keeps nothing; a
// conflict is noted in f. A binary file (a figure) is never merged: the workspace's edit stands whole.
func mergedText(base, ours, theirs *string, label, p string, f *Followed) (*string, error) {
	for _, t := range []*string{base, ours, theirs} {
		if t != nil && strings.Contains(*t, "\x00") {
			f.Binaries = append(f.Binaries, p)
			return ours, nil
		}
	}
	text, conflict, err := mergeFile(base, ours, theirs, label)
	if err != nil {
		return nil, err
	}
	if conflict {
		f.Conflicts = append(f.Conflicts, p)
	}
	if ours == nil && text == "" && !conflict {
		return nil, nil
	}
	return &text, nil
}

func setOf(list []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		out[s] = true
	}
	return out
}

// draftCommit is the draft commit of tree on base, its pages tagged as tags give them and marks its conflict marks,
// or base when it holds neither a page nor a mark.
func draftCommit(store string, w Workspace, tree, base string, tags, marks map[string]string) (string, error) {
	pages, err := changedPaths(store, base, tree)
	if err != nil || (len(pages) == 0 && len(marks) == 0) {
		return base, err
	}
	for _, p := range pages {
		if tags[p] == "" {
			return "", errorf("the draft of %s holds %s with no sprint: the records branch moved a drafted page in a "+
				"way pm cannot follow. Keep the draft as uncommitted edits with git -C %s reset --soft %s, then pm "+
				"commit them", w.Branch(), p, w.Dir, w.BaseRef())
		}
	}
	return gitIn(store, nil, draftMessage(w, pages, tags, marks), "commit-tree", tree, "-p", base, "-F", "-")
}

// draftMessage is a draft commit's message: the subject, then a "Sprint:" line for each of pages that tags names, and
// a "Conflict:" line for each mark.
func draftMessage(w Workspace, pages []string, tags, marks map[string]string) string {
	var lines []string
	for _, p := range slices.Sorted(slices.Values(pages)) {
		if sprint := tags[p]; sprint != "" {
			lines = append(lines, "Sprint: "+p+" "+sprint)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(marks)) {
		lines = append(lines, "Conflict: "+p+" "+marks[p])
	}
	msg := draftSubject + "design pages of " + w.Branch() + "\n"
	if lines != nil {
		msg += "\n" + strings.Join(lines, "\n") + "\n"
	}
	return msg
}

// move writes plan's files into the workspace, each only while it still holds the text the plan read (an edit made
// meanwhile aborts the move), then moves its branch from old to head, its base to base and its index to head's tree;
// whatever fails puts back what it changed.
func (w Workspace) move(store, old, head, base string, plan map[string]planned) error {
	before := map[string]*string{}
	undo := func(err error) error {
		for p, text := range before {
			err = errors.Join(err, w.put(p, text))
		}
		return err
	}
	for _, p := range slices.Sorted(maps.Keys(plan)) {
		now, err := w.read(p)
		if err != nil {
			return undo(err)
		}
		if !same(now, plan[p].was) {
			return undo(errorf("records/%s changed in %s while pm followed the records branch; nothing was changed, so "+
				"run the pm command again", p, w.Dir))
		}
		before[p] = now
		if err := w.put(p, plan[p].text); err != nil {
			return undo(err)
		}
	}
	if head == old {
		return nil
	}
	if err := w.setBranch(store, old, head, base); err != nil {
		return undo(err)
	}
	return nil
}

// setBranch moves the branch from old to head and its recorded base to base in one ref transaction, then the
// workspace's index to head's tree, the working tree untouched; it moves both back when the index fails.
func (w Workspace) setBranch(store, old, head, base string) error {
	was, err := Git(store, "rev-parse", "--verify", "--quiet", w.BaseRef())
	if err != nil {
		return err
	}
	if err := w.setRefs(store, old, head, was, base, "pm: follow"); err != nil {
		return err
	}
	if _, err := Git(w.Dir, "read-tree", head); err != nil {
		return errors.Join(err, w.setRefs(store, head, old, base, was, "pm: follow undone"))
	}
	run(w.Dir, "update-index", "-q", "--refresh") //nolint:errcheck // exits 1 while files differ, which they may
	return nil
}

// setRefs moves the branch from old to head and the base from oldBase to base, both or neither.
func (w Workspace) setRefs(store, old, head, oldBase, base, why string) error {
	_, err := gitIn(store, nil, fmt.Sprintf("start\nupdate %s %s %s\nupdate %s %s %s\nprepare\ncommit\n", w.Ref(), head,
		old, w.BaseRef(), base, oldBase), "update-ref", "-m", why, "--stdin")
	return err
}

// put writes rel's text in the workspace, or removes the file for nil.
func (w Workspace) put(rel string, text *string) error {
	p := filepath.Join(w.Dir, filepath.FromSlash(rel))
	if text == nil {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return WriteAtomic(p, *text)
}

// CommitDraft puts pages, each a workspace file (design/x.md) with its sprint, into the workspace's draft commit, on
// the base a follow just moved to the records tip; it returns the branch's commit before, for ResetDraft.
func CommitDraft(store string, w Workspace, pages map[string]string) (string, error) {
	d, err := readDraft(store, w)
	if err != nil {
		return "", err
	}
	var info strings.Builder
	var committed []string
	for _, p := range slices.Sorted(maps.Keys(pages)) {
		text, err := w.read(p)
		if err != nil {
			return "", err
		}
		d.tags[p] = pages[p]
		delete(d.conflicts, p) // committed again: resolved
		committed = append(committed, p)
		if text == nil {
			info.WriteString("0 0000000000000000000000000000000000000000\t" + p + "\n")
			continue
		}
		oid, err := gitIn(store, nil, *text, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		info.WriteString("100644 " + oid + "\t" + p + "\n")
	}
	tmp, err := os.MkdirTemp("", "pm-draft-index")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // a temp dir left behind breaks nothing
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitIn(store, env, "", "read-tree", d.head); err != nil {
		return "", err
	}
	if _, err := gitIn(store, env, info.String(), "update-index", "--index-info"); err != nil {
		return "", err
	}
	tree, err := gitIn(store, env, "", "write-tree")
	if err != nil {
		return "", err
	}
	head, err := draftCommit(store, w, tree, d.base, d.tags, d.conflicts)
	if err != nil {
		return "", err
	}
	if err := w.setBranch(store, d.head, head, d.base); err != nil {
		return "", err
	}
	return d.head, w.noteStranded(nil, committed) // committed under an open sprint: no longer stranded
}

// ResetDraft moves the workspace's branch back to old, the commit CommitDraft returned, and its index with it.
func ResetDraft(store string, w Workspace, old string) error {
	d, err := readDraft(store, w)
	if err != nil || d.head == old {
		return err
	}
	return w.setBranch(store, d.head, old, d.base)
}

// Sprints is each page the workspace's draft commit holds unpublished or marks in conflict: its sprint, one per page.
func (w Workspace) Sprints(store string) (map[string]string, error) {
	d, err := readDraft(store, w)
	if err != nil {
		return nil, err
	}
	out := maps.Clone(d.conflicts)
	maps.Copy(out, d.tags)
	return out, nil
}

// DraftPages is the unpublished pages the workspace's draft commit holds, each with its text (nil: it removes the
// page), and their sprints; a page its sprint's close published or discarded is not one.
func DraftPages(store string, w Workspace) (map[string]*string, map[string]string, error) {
	d, err := readDraft(store, w)
	if err != nil || !d.draft {
		return nil, nil, err
	}
	paths, err := changedPaths(store, d.base, d.head)
	if err != nil {
		return nil, nil, err
	}
	paths = slices.DeleteFunc(paths, func(p string) bool { _, ok := d.tags[p]; return !ok })
	texts, err := blobs(store, d.head, paths)
	return texts, d.tags, err
}

var markerRange = regexp.MustCompile(`(?m)^>>>>>>> records ([0-9a-f]+)\.\.([0-9a-f]+)$`)

// HasMarkers is whether text holds a conflict marker line (<<<<<<< or >>>>>>> and a space).
func HasMarkers(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "<<<<<<< ") || strings.HasPrefix(l, ">>>>>>> ") {
			return true
		}
	}
	return false
}

// MarkerCommits is the records commits that changed rel in the ranges text's markers name (records <base>..<tip>, as
// Follow writes them), oldest first, each as "<short sha> <subject>".
func MarkerCommits(store, rel, text string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, m := range markerRange.FindAllStringSubmatch(text, -1) {
		// a records sync that rebased leaves copies of commits the workspace had: --cherry-pick drops them
		log, err := Git(store, "log", "--reverse", "--cherry-pick", "--right-only", "--format=%h %s", m[1]+"..."+m[2],
			"--", rel)
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(log, "\n") {
			if l != "" && !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out, nil
}

// FollowClean follows each records workspace that has no uncommitted file, under the store lock the caller holds: the
// records sync's follow after a fast-forward, which writes no file a session is editing. A line for each workspace it
// followed that dropped or stranded pages, and for each it could not follow; a failure fails only its own line.
func FollowClean(store string, closed SprintClosed) []string {
	branches, err := DraftBranches(store)
	if err != nil {
		return []string{"listing the records workspaces failed: " + err.Error()}
	}
	var out []string
	for _, w := range branches {
		if w.Dir == "" {
			continue
		}
		dirty, err := w.Dirty()
		if err == nil && len(dirty) > 0 {
			continue
		}
		var f Followed
		if err == nil {
			f, err = Follow(store, w, func() (SprintClosed, error) { return closed, nil })
		}
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("%s not followed: %v", w.Branch(), err))
		case len(f.Stranded) > 0:
			out = append(out, fmt.Sprintf("%s followed; stranded %s", w.Branch(),
				strings.Join(slices.Sorted(maps.Keys(f.Stranded)), ", ")))
		}
	}
	return out
}
