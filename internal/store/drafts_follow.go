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
	"sort"
	"strings"
)

// Follow brings a workspace onto the records tip; the draft commit and pm commit's publish build on it. pm never runs
// git rebase, merge or stash in a workspace: it merges with git merge-tree --write-tree and git merge-file, writes
// commits with git commit-tree, moves the branch with git update-ref, resets the workspace's index with git read-tree
// and writes the files itself. Each runs under the store lock and completes or changes nothing.

// draftSubject starts a draft commit's message; each of its pages has a line "Sprint: <path> <sprint id>".
const draftSubject = "pm draft: "

var sprintTag = regexp.MustCompile(`(?m)^Sprint: (\S+) (\S+)$`)

// draftState is a workspace's branch: its commit, the records commit it is on, and its draft commit's pages' sprints.
type draftState struct {
	head, base string
	draft      bool              // head is a draft commit on base
	tags       map[string]string // each page of the draft commit (design/x.md): its sprint
}

func readDraft(store string, w Workspace) (draftState, error) {
	out, err := Git(store, "log", "-1", "--format=%H%x00%P%x00%B", w.Ref())
	if err != nil {
		return draftState{}, err
	}
	parts := strings.SplitN(out, "\x00", 3)
	if len(parts) != 3 {
		return draftState{}, fmt.Errorf("git log -1 %s printed %q", w.Ref(), out)
	}
	d := draftState{head: parts[0], base: parts[0], tags: map[string]string{}}
	if strings.HasPrefix(parts[2], draftSubject) {
		parents := strings.Fields(parts[1])
		if len(parents) != 1 {
			return draftState{}, errorf("the draft commit %s of %s has %d parents, not its base alone", d.head,
				w.Branch(), len(parents))
		}
		d.draft, d.base = true, parents[0]
		for _, m := range sprintTag.FindAllStringSubmatch(parts[2], -1) {
			d.tags[m[1]] = m[2]
		}
	}
	if !isRecordsCommit(store, d.base) {
		return draftState{}, errorf("%s is at %s, which is neither a records commit nor a draft commit on one: a commit "+
			"made by hand in %s? pm commits there itself; undo it with git -C %s reset --soft HEAD~1, which keeps its "+
			"edits uncommitted, then pm commit them", w.Branch(), d.base, w.Dir, w.Dir)
	}
	return d, nil
}

// isRecordsCommit is whether sha is or was on the records branch: on it now, or in its reflog (a records sync rebases
// the branch, leaving a workspace's base off it).
func isRecordsCommit(store, sha string) bool {
	if _, _, err := run(store, "merge-base", "--is-ancestor", sha, "refs/heads/"+Branch); err == nil {
		return true
	}
	out, _, err := run(store, "reflog", "show", "--format=%H", "refs/heads/"+Branch)
	return err == nil && slices.Contains(strings.Fields(out), sha)
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
}

// Follow brings the workspace onto the records tip, under the store lock the caller holds:
//
//  1. the draft commit, if any, is merged onto the tip with git merge-tree --write-tree from its base;
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
	f := Followed{Base: d.base, Tip: tip}
	if d.base == tip {
		return f, nil
	}
	head, conflicted := tip, []string(nil)
	if d.draft {
		var tree string
		if tree, conflicted, err = mergeTree(store, d.base, tip, d.head); err != nil {
			return Followed{}, err
		}
		if head, err = draftCommit(store, w, tree, tip, d.tags); err != nil {
			return Followed{}, err
		}
	}
	plan, err := followPlan(store, w, d, head, conflicted, &f)
	if err != nil {
		return Followed{}, err
	}
	return f, w.move(store, d.head, head, plan)
}

// followPlan is each workspace file Follow rewrites, with its new text (nil: removed), and f.Conflicts set.
func followPlan(store string, w Workspace, d draftState, head string, conflicted []string, f *Followed) (
	map[string]*string, error) {
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
	plan := map[string]*string{}
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
			plan[p] = text
		}
	}
	return plan, nil
}

// mergedText is ours merged with theirs from base, nil when ours removed the file and the merge keeps nothing; a
// conflict is noted in f. A binary file (a figure) is never merged: the workspace's edit stands whole.
func mergedText(base, ours, theirs *string, label, p string, f *Followed) (*string, error) {
	for _, t := range []*string{base, ours, theirs} {
		if t != nil && strings.Contains(*t, "\x00") {
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
	msg := draftSubject + "design pages of " + w.Branch() + "\n"
	sort.Strings(pages)
	for i, p := range pages {
		if i == 0 {
			msg += "\n"
		}
		sprint := tags[p]
		if sprint == "" {
			return "", errorf("the draft of %s holds %s with no sprint", w.Branch(), p)
		}
		msg += "Sprint: " + p + " " + sprint + "\n"
	}
	return gitIn(store, nil, msg, "commit-tree", tree, "-p", base, "-F", "-")
}

// move writes plan's files into the workspace, then moves its branch from old to head and its index to head's tree;
// whatever fails puts back what it changed.
func (w Workspace) move(store, old, head string, plan map[string]*string) error {
	before := map[string]*string{}
	undo := func(err error) error {
		for p, text := range before {
			err = errors.Join(err, w.put(p, text))
		}
		return err
	}
	for _, p := range slices.Sorted(maps.Keys(plan)) {
		was, err := w.read(p)
		if err != nil {
			return undo(err)
		}
		before[p] = was
		if err := w.put(p, plan[p]); err != nil {
			return undo(err)
		}
	}
	if head == old {
		return nil
	}
	if err := w.setBranch(store, old, head); err != nil {
		return undo(err)
	}
	return nil
}

// setBranch moves the branch from old to head and the workspace's index to head's tree, the working tree untouched;
// it moves the branch back when the index fails.
func (w Workspace) setBranch(store, old, head string) error {
	if _, err := Git(store, "update-ref", "-m", "pm: follow", w.Ref(), head, old); err != nil {
		return err
	}
	if _, err := Git(w.Dir, "read-tree", head); err != nil {
		_, back := Git(store, "update-ref", "-m", "pm: follow undone", w.Ref(), old, head)
		return errors.Join(err, back)
	}
	run(w.Dir, "update-index", "-q", "--refresh") //nolint:errcheck // exits 1 while files differ, which they may
	return nil
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
	return d.head, w.setBranch(store, d.head, head)
}

// ResetDraft moves the workspace's branch back to old, the commit CommitDraft returned, and its index with it.
func ResetDraft(store string, w Workspace, old string) error {
	head, err := Git(store, "rev-parse", w.Ref())
	if err != nil || head == old {
		return err
	}
	return w.setBranch(store, head, old)
}

// DraftPages is the pages the workspace's draft commit holds, each with its text (nil: it removes the page), and their
// sprints.
func DraftPages(store string, w Workspace) (map[string]*string, map[string]string, error) {
	d, err := readDraft(store, w)
	if err != nil || !d.draft {
		return nil, nil, err
	}
	paths, err := changedPaths(store, d.base, d.head)
	if err != nil {
		return nil, nil, err
	}
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
		log, err := Git(store, "log", "--reverse", "--format=%h %s", m[1]+".."+m[2], "--", rel)
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
