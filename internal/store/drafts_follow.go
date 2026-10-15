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
)

// Follow brings a workspace onto the records tip; the draft commit and pm commit's publish build on it. pm never runs
// git rebase, merge or stash in a workspace: it merges with git merge-tree --write-tree and git merge-file, writes
// commits with git commit-tree, moves the branch with git update-ref, resets the workspace's index with git read-tree
// and writes the files itself. Each runs under the store lock and completes or changes nothing.

// draftSubject starts a draft commit's message; each of its unpublished pages has a line "Sprint: <path> <sprint id>".
// A page the draft commit changes with no such line was published or discarded by its sprint's close
// (drafts_publish.go), which leaves the tree as it was: the next follow drops it.
const draftSubject = "pm draft: "

var sprintTag = regexp.MustCompile(`(?m)^Sprint: (\S+) (\S+)$`)

// draftState is a workspace's branch: its commit, the records commit it is on, and its draft commit's pages' sprints.
type draftState struct {
	head, base string
	draft      bool              // head is a draft commit on base
	tags       map[string]string // each unpublished page of the draft commit (design/x.md): its sprint
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
	d := draftState{head: parts[0], base: base, tags: map[string]string{}}
	switch {
	case d.head == base:
	case strings.HasPrefix(parts[2], draftSubject) && parts[1] == base:
		d.draft = true
		for _, m := range sprintTag.FindAllStringSubmatch(parts[2], -1) {
			d.tags[m[1]] = m[2]
		}
	default:
		return draftState{}, errorf("%s is at %s, a commit pm did not make on its base %s: a commit made by hand in %s? "+
			"pm commits there itself; undo it with git -C %s reset --soft %s, which keeps its edits uncommitted, then pm "+
			"commit them", w.Branch(), d.head, base, w.Dir, w.Dir, base)
	}
	return d, nil
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
	Binaries  []string // uncommitted binary files the records branch changed under: kept whole, noted for pm commit
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
//  4. the branch moves to the tip plus the new draft commit; a file not uncommitted is refreshed from the new tree.
//
// The markers name the records range merged in (records <base>..<tip>), which pm commit reads to name the commits. A
// binary file is not merged: an uncommitted one stays as the workspace has it. It writes the files, then moves the
// branch and the index; a failure puts back what it changed.
func Follow(store string, w Workspace) (Followed, error) {
	tip, err := Git(store, "rev-parse", "refs/heads/"+Branch)
	if err != nil {
		return Followed{}, err
	}
	d, err := readDraft(store, w)
	if err != nil {
		return Followed{}, err
	}
	closed, err := d.closed(store)
	if err != nil {
		return Followed{}, err
	}
	f := Followed{Base: d.base, Tip: tip}
	if d.base == tip && len(closed) == 0 {
		return f, w.syncIndex() // a follow stopped between the branch and the index leaves the index behind
	}
	head, conflicted := tip, []string(nil)
	if d.draft {
		draft, err := withoutPages(store, d, closed)
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
		if head, err = draftCommit(store, w, tree, tip, tags); err != nil {
			return Followed{}, err
		}
	}
	plan, err := followPlan(store, w, d, head, conflicted, &f)
	if err != nil {
		return Followed{}, err
	}
	if err := w.move(store, d.head, head, tip, plan); err != nil {
		return Followed{}, err
	}
	return f, w.noteBinaries(d.base, tip, f.Binaries)
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

// draftCommit is the draft commit of tree on base, its pages tagged as tags give them, or base when tree adds nothing.
func draftCommit(store string, w Workspace, tree, base string, tags map[string]string) (string, error) {
	pages, err := changedPaths(store, base, tree)
	if err != nil || len(pages) == 0 {
		return base, err
	}
	for _, p := range pages {
		if tags[p] == "" {
			return "", errorf("the draft of %s holds %s with no sprint: the records branch moved a drafted page in a "+
				"way pm cannot follow. Keep the draft as uncommitted edits with git -C %s reset --soft %s, then pm "+
				"commit them", w.Branch(), p, w.Dir, w.BaseRef())
		}
	}
	return gitIn(store, nil, draftMessage(w, pages, tags), "commit-tree", tree, "-p", base, "-F", "-")
}

// draftMessage is a draft commit's message: the subject, then a "Sprint:" line for each of pages that tags names.
func draftMessage(w Workspace, pages []string, tags map[string]string) string {
	msg := draftSubject + "design pages of " + w.Branch() + "\n"
	sep := "\n"
	for _, p := range slices.Sorted(slices.Values(pages)) {
		if sprint := tags[p]; sprint != "" {
			msg += sep + "Sprint: " + p + " " + sprint + "\n"
			sep = ""
		}
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
	for _, p := range slices.Sorted(maps.Keys(pages)) {
		text, err := w.read(p)
		if err != nil {
			return "", err
		}
		d.tags[p] = pages[p]
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
	head, err := draftCommit(store, w, tree, d.base, d.tags)
	if err != nil {
		return "", err
	}
	return d.head, w.setBranch(store, d.head, head, d.base)
}

// ResetDraft moves the workspace's branch back to old, the commit CommitDraft returned, and its index with it.
func ResetDraft(store string, w Workspace, old string) error {
	d, err := readDraft(store, w)
	if err != nil || d.head == old {
		return err
	}
	return w.setBranch(store, d.head, old, d.base)
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
