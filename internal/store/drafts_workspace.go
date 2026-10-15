package store

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Workspaces: each code worktree but the main checkout edits records in its own workspace, a worktree of the branch
// drafts/<name> at <worktree>/records, where <name> is the code worktree's admin name (.git/worktrees/<name>), unique
// in the clone. The branch is a records commit, its base, plus at most one draft commit (drafts_follow.go). The main
// checkout keeps its records/ link to the store.

// DraftsPrefix is the branch prefix of the workspaces: drafts/<name>.
const DraftsPrefix = "drafts/"

// Workspace is a code worktree's records workspace.
type Workspace struct {
	Code string // the code worktree
	Dir  string // <Code>/records, the workspace's working tree
	Name string // the code worktree's admin name
}

// Branch is the workspace's branch, drafts/<Name>.
func (w Workspace) Branch() string { return DraftsPrefix + w.Name }

// Ref is the workspace's branch as a full ref.
func (w Workspace) Ref() string { return "refs/heads/" + w.Branch() }

// BaseRef is where pm records the workspace's base, the records commit its branch is on: a ref, so the base stays
// known and reachable after a records sync rebased it off the branch and its reflog entry expired.
func (w Workspace) BaseRef() string { return "refs/pm/workspaces/" + w.Name + "/base" }

// DeleteBranch deletes the workspace's branch and its recorded base, run in dir; for a workspace already removed.
func (w Workspace) DeleteBranch(dir string) error {
	_, err := Git(dir, "branch", "-D", "--quiet", w.Branch())
	if err == nil {
		_, err = Git(dir, "update-ref", "-d", w.BaseRef())
	}
	return err
}

// adminName is the admin name of the linked worktree code (its git dir is <common>/worktrees/<name>), or "" for the
// main checkout, whose git dir is the common dir.
func adminName(code string) (string, error) {
	out, err := Git(code, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return "", err
	}
	dirs := strings.Split(out, "\n")
	if len(dirs) != 2 {
		return "", fmt.Errorf("git rev-parse --git-dir --git-common-dir printed %q in %s", out, code)
	}
	if resolved(dirs[0]) == resolved(dirs[1]) {
		return "", nil
	}
	if resolved(filepath.Dir(dirs[0])) != resolved(filepath.Join(dirs[1], "worktrees")) {
		return "", errorf("%s's git dir %s is not under %s/worktrees", code, dirs[0], dirs[1])
	}
	return filepath.Base(dirs[0]), nil
}

// headOf is the top of the worktree containing dir and the branch its HEAD names, as a ref ("" when detached).
func headOf(dir string) (top, ref string, err error) {
	if top, err = Git(dir, "rev-parse", "--show-toplevel"); err != nil {
		return "", "", err
	}
	out, _, err := run(dir, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return top, "", nil //nolint:nilerr // exit 1: a detached HEAD names no branch
	}
	return top, strings.TrimSpace(out), nil
}

// IsWorkspace is whether top, a worktree's top, is a records workspace: its HEAD is on a drafts/ branch.
func IsWorkspace(top string) bool {
	_, ref, err := headOf(top)
	return err == nil && strings.HasPrefix(ref, "refs/heads/"+DraftsPrefix)
}

// WorkspaceAt is the workspace of the code worktree code, nil when it has none: the main checkout, or a worktree whose
// records/ is the link to the store, nothing, or no worktree's top (a branch's tracked copy). A records/ that is a
// worktree on any branch but this worktree's drafts/<name> is refused.
func WorkspaceAt(code string) (*Workspace, error) {
	name, err := adminName(code)
	if err != nil || name == "" {
		return nil, err
	}
	w := &Workspace{Code: code, Dir: filepath.Join(code, "records"), Name: name}
	st, err := os.Lstat(w.Dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && st.Mode()&os.ModeSymlink != 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	st, err = os.Lstat(filepath.Join(w.Dir, ".git"))
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !st.Mode().IsRegular()) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	top, ref, err := headOf(w.Dir)
	if err != nil {
		return nil, err
	}
	if resolved(top) != resolved(w.Dir) || ref != w.Ref() {
		return nil, errorf("%s is a git worktree on %s, not this worktree's records workspace on %s; move it away and "+
			"run %s", w.Dir, strings.TrimPrefix(ref, "refs/heads/"), w.Branch(), Setup)
	}
	return w, nil
}

// WorkspaceOf is the workspace of the code worktree containing cwd (from inside a workspace, its own), nil when it has
// none.
func WorkspaceOf(cwd string) (*Workspace, error) {
	top, ref, err := headOf(cwd)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(ref, "refs/heads/"+DraftsPrefix) {
		top = filepath.Dir(top)
	}
	return WorkspaceAt(top)
}

// AddWorkspace makes the code worktree's workspace in place of its records/ link, or of no records/: the branch
// drafts/<name> set to the records tip and checked out at <code>/records. A branch left by a removed worktree of the
// same name is reused when its draft commit holds no unpublished page. "" when the workspace is there already; it
// changes nothing when it fails.
func AddWorkspace(store, code string) (string, error) {
	if w, err := WorkspaceAt(code); err != nil || w != nil {
		return "", err
	}
	name, err := adminName(code)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("%s is the main checkout, which keeps its records/ link to the store", code)
	}
	if name == EditsDraft || name == workspacesDraft { // its draft's URL would be the store's, or the error's
		return "", errorf("a worktree named %s gets no records workspace: the site names a draft %s already; rename "+
			"the worktree (git worktree move) and run %s there", name, name, Setup)
	}
	if err := CheckGit(); err != nil {
		return "", err
	}
	w := Workspace{Code: code, Dir: filepath.Join(code, "records"), Name: name}
	link := ""
	if st, err := os.Lstat(w.Dir); err == nil {
		if st.Mode()&os.ModeSymlink == 0 || resolved(w.Dir) != resolved(store) {
			return "", errorf("%s exists and is neither the link to %s nor a records workspace; move it away and run %s "+
				"again", w.Dir, store, Setup)
		}
		if link, err = os.Readlink(w.Dir); err != nil {
			return "", err
		}
	}
	if _, err := Git(store, "rev-parse", "--verify", "--quiet", w.Ref()); err == nil {
		d, err := readDraft(store, w)
		if err != nil {
			return "", err
		}
		// a draft commit whose every page is published (untagged, or named by a close's Published: line) is no draft
		if pending, err := d.pending(store); err != nil {
			return "", err
		} else if pending != nil {
			return "", errorf("%s holds the draft commit %s of a removed worktree named %s, with pages not published "+
				"(%s); publish them with pm sprint close, or drop them with its --discard, before this worktree takes "+
				"the name", w.Branch(), d.head, name, strings.Join(pending, ", "))
		}
	}
	// a worktree removed without git worktree remove still holds its branch in git's list until pruned
	if _, err := Git(store, "worktree", "prune"); err != nil {
		return "", err
	}
	if link != "" {
		if err := os.Remove(w.Dir); err != nil {
			return "", err
		}
	}
	tip, err := Git(store, "rev-parse", "refs/heads/"+Branch)
	if err != nil {
		return "", err
	}
	if _, err := Git(store, "update-ref", w.BaseRef(), tip); err != nil {
		return "", err
	}
	if _, err := Git(store, "worktree", "add", "--quiet", "-B", w.Branch(), w.Dir, tip); err != nil {
		_, undo := Git(store, "update-ref", "-d", w.BaseRef())
		err = errors.Join(err, undo)
		if link != "" {
			err = errors.Join(err, os.Symlink(link, w.Dir))
		}
		return "", err
	}
	return fmt.Sprintf("made %s a records workspace on branch %s", w.Dir, w.Branch()), nil
}

// Dirty is each file of the workspace that differs from its branch's HEAD (modified, staged, removed or untracked),
// by its path in the workspace (sprints/x.md). One git status, with no optional lock (Uncommitted says why).
func (w Workspace) Dirty() ([]string, error) {
	out, errOut, err := run(w.Dir, "--no-optional-locks", "status", "--porcelain", "-z", "--no-renames",
		"--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status failed in %s: %s", w.Dir, strings.TrimSpace(errOut))
	}
	var dirty []string
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("git status --porcelain -z in %s printed %q, not a status and a path", w.Dir, entry)
		}
		dirty = append(dirty, entry[3:])
	}
	return dirty, nil
}

// read is the workspace file rel's text, nil when there is none.
func (w Workspace) read(rel string) (*string, error) {
	b, err := os.ReadFile(filepath.Join(w.Dir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	text := string(b)
	return &text, nil
}

// Describe is the workspace's state as pm where gives it: its branch, its draft commit's pages, its uncommitted files
// (those with conflict markers named) and how far the records branch moved past its base.
func (w Workspace) Describe(store string) (string, error) {
	d, err := readDraft(store, w)
	if err != nil {
		return "", err
	}
	parts := []string{"records workspace on " + w.Branch()}
	if len(d.tags) > 0 { // a page with no tag was published or discarded by its sprint's close
		parts = append(parts, fmt.Sprintf("a draft commit of %d design pages", len(d.tags)))
	}
	dirty, err := w.Dirty()
	if err != nil {
		return "", err
	}
	var marked []string
	for _, rel := range dirty {
		if text, err := w.read(rel); err == nil && text != nil && HasMarkers(*text) {
			marked = append(marked, "records/"+rel)
		}
	}
	edits := fmt.Sprintf("%d uncommitted files", len(dirty))
	if marked != nil {
		edits += " (conflict markers in " + strings.Join(marked, ", ") + ")"
	}
	parts = append(parts, edits)
	stranded, err := w.Stranded()
	if err != nil {
		return "", err
	}
	for _, rel := range slices.Sorted(maps.Keys(stranded)) {
		parts = append(parts, StrandedLine(rel, stranded[rel]))
	}
	behind, err := Git(store, "rev-list", "--count", d.base+"..refs/heads/"+Branch)
	if err != nil {
		return "", err
	}
	if behind == "0" {
		parts = append(parts, "on the records tip")
	} else {
		parts = append(parts, behind+" records commits behind, followed at the next pm write from here")
	}
	return strings.Join(parts, ", "), nil
}

// Named is each named path as a path in the workspace (sprints/x.md): records/<…> from the code worktree, a path in
// the workspace, or one relative to it from anywhere. Each must be one of dirty, the files with an uncommitted change.
func (w Workspace) Named(here string, names, dirty []string) ([]string, error) {
	root := resolved(w.Dir)
	var rels []string
	for _, name := range names {
		g := name
		if top, under, _ := strings.Cut(name, "/"); !filepath.IsAbs(g) && top == "records" {
			g = filepath.Join(w.Dir, under)
		} else if !filepath.IsAbs(g) {
			g = filepath.Join(here, g)
		}
		rel, err := filepath.Rel(root, resolved(g))
		rel = filepath.ToSlash(rel)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			rel = filepath.ToSlash(filepath.Clean(name)) // relative to the workspace, from anywhere
			if filepath.IsAbs(name) || !slices.Contains(dirty, rel) {
				return nil, errorf("%s is not in this worktree's records workspace %s", name, w.Dir)
			}
		}
		if rel == "." {
			return nil, errorf("%s is the records workspace itself; name the records you edited: pm commit -m \"…\" "+
				"records/<…>.md…", name)
		}
		if !slices.Contains(dirty, rel) {
			return nil, errorf("%s has no uncommitted change", name)
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

// minGit is the oldest git a workspace works with: follow runs git merge-tree --write-tree --merge-base (git 2.40).
var minGit = [2]int{2, 40}

// CheckGit refuses a git older than minGit, naming the version found and the one needed.
func CheckGit() error {
	out, err := Git(".", "version")
	if err != nil {
		return err
	}
	return gitNewEnough(out)
}

// gitNewEnough checks git version's output ("git version 2.43.0", "git version 2.39.3 (Apple Git-146)").
func gitNewEnough(version string) error {
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimPrefix(version, "git version "), "%d.%d", &major, &minor); err != nil {
		return errorf("cannot read the git version from %q", version)
	}
	if major > minGit[0] || (major == minGit[0] && minor >= minGit[1]) {
		return nil
	}
	return errorf("git %d.%d is older than %d.%d, which records workspaces need (git merge-tree --merge-base); "+
		"install git %d.%d or newer, then run %s again", major, minor, minGit[0], minGit[1], minGit[0], minGit[1], Setup)
}

// binariesFile, in the workspace's git dir, notes each uncommitted binary file that a follow kept whole while the
// records branch changed it: a line "<path>\t<base>..<tip>", the records range it was kept over.
const binariesFile = "pm-binaries"

func (w Workspace) binariesPath() (string, error) { return w.gitFile(binariesFile) }

// gitFile is the path of name in the workspace's git dir.
func (w Workspace) gitFile(name string) (string, error) {
	dir, err := Git(w.Dir, "rev-parse", "--absolute-git-dir")
	return filepath.Join(dir, name), err
}

// binaries is the noted binary files, each with its records range.
func (w Workspace) binaries() (map[string]string, error) {
	p, err := w.binariesPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if rel, span, ok := strings.Cut(line, "\t"); ok {
			out[rel] = span
		}
	}
	return out, nil
}

func (w Workspace) writeBinaries(m map[string]string) error {
	p, err := w.binariesPath()
	if err != nil {
		return err
	}
	if len(m) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	var b strings.Builder
	for _, rel := range slices.Sorted(maps.Keys(m)) {
		b.WriteString(rel + "\t" + m[rel] + "\n")
	}
	return WriteAtomic(p, b.String())
}

// noteBinaries notes the binary files a follow from base to tip kept whole, widening a file's range when it was noted
// before, and forgets those no longer uncommitted.
func (w Workspace) noteBinaries(base, tip string, kept []string) error {
	m, err := w.binaries()
	if err != nil || (len(m) == 0 && len(kept) == 0) {
		return err
	}
	dirty, err := w.Dirty()
	if err != nil {
		return err
	}
	for rel := range m {
		if !slices.Contains(dirty, rel) {
			delete(m, rel)
		}
	}
	for _, rel := range kept {
		from := base
		if span, ok := m[rel]; ok {
			from, _, _ = strings.Cut(span, "..")
		}
		m[rel] = from + ".." + tip
	}
	return w.writeBinaries(m)
}

// BinaryCommits is the records commits that changed rel, an uncommitted binary file a follow kept whole over them,
// each as "<short sha> <subject>"; none when it was not noted. It forgets the note, so a second pm commit of the file
// replaces the published one.
func (w Workspace) BinaryCommits(store, rel string) ([]string, error) {
	m, err := w.binaries()
	if err != nil {
		return nil, err
	}
	span, ok := m[rel]
	if !ok {
		return nil, nil
	}
	delete(m, rel)
	if err := w.writeBinaries(m); err != nil {
		return nil, err
	}
	return MarkerCommits(store, rel, ">>>>>>> records "+span+"\n")
}

// binaryConflict is whether rel is a noted binary file.
func (w Workspace) binaryConflict(rel string) bool {
	m, err := w.binaries()
	_, ok := m[rel]
	return err == nil && ok
}

// strandedFile, in the workspace's git dir, notes each draft page a follow took out of the draft commit because its
// sprint had closed (Follow): a line "<path>\t<sprint>", kept while the file stays uncommitted.
const strandedFile = "pm-stranded"

// Stranded is each uncommitted page of the workspace that a follow took out of its draft commit as its sprint had
// closed, with that sprint: an edit no close will publish until it is committed under an open sprint.
func (w Workspace) Stranded() (map[string]string, error) {
	p, err := w.gitFile(strandedFile)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	dirty, err := w.Dirty()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if rel, sprint, ok := strings.Cut(line, "\t"); ok && slices.Contains(dirty, rel) {
			out[rel] = sprint
		}
	}
	return out, nil
}

// noteStranded adds the pages of add to the noted ones and forgets those of done, and any no longer uncommitted.
func (w Workspace) noteStranded(add map[string]string, done []string) error {
	m, err := w.Stranded()
	if err != nil {
		return err
	}
	p, err := w.gitFile(strandedFile)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) && len(add) == 0 {
		return nil
	}
	maps.Copy(m, add)
	for _, rel := range done {
		delete(m, rel)
	}
	if len(m) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		// every stranded page was committed again or reverted: the kept draft is no longer needed
		_, _, err := run(w.Dir, "update-ref", "-d", w.StrandedRef())
		return err
	}
	var b strings.Builder
	for _, rel := range slices.Sorted(maps.Keys(m)) {
		b.WriteString(rel + "\t" + m[rel] + "\n")
	}
	return WriteAtomic(p, b.String())
}

// StrandedRef keeps the draft text of the pages a follow stranded, so removing the worktree, which deletes their
// uncommitted files, loses nothing: pm deletes it only once each is committed again or reverted in the workspace.
func (w Workspace) StrandedRef() string { return "refs/pm/workspaces/" + w.Name + "/stranded" }

// keepStranded records the stranded pages' text, as the follow's merged draft commit full holds it, in StrandedRef: a
// commit on the ref's last one, if any, with those pages laid over it and a line "Stranded: <path> <sprint>" each.
func (w Workspace) keepStranded(store, full string, stranded map[string]string) error {
	if len(stranded) == 0 {
		return nil
	}
	pages := slices.Sorted(maps.Keys(stranded))
	tree, parent, msg := full, full, "pm stranded: pages of "+w.Branch()+" whose sprint closed\n\n"
	if old, err := Git(store, "rev-parse", "--verify", "--quiet", w.StrandedRef()); err == nil {
		if tree, err = withFiles(store, old, full, pages); err != nil {
			return err
		}
		parent = old
		prev, err := Git(store, "log", "-1", "--format=%B", old)
		if err != nil {
			return err
		}
		for _, m := range strandedTag.FindAllStringSubmatch(prev, -1) {
			if _, ok := stranded[m[1]]; !ok {
				msg += m[0] + "\n"
			}
		}
	}
	for _, p := range pages {
		msg += "Stranded: " + p + " " + stranded[p] + "\n"
	}
	head, err := gitIn(store, nil, msg, "commit-tree", tree+"^{tree}", "-p", parent, "-F", "-")
	if err != nil {
		return err
	}
	_, err = Git(store, "update-ref", w.StrandedRef(), head)
	return err
}

var strandedTag = regexp.MustCompile(`(?m)^Stranded: (\S+) (\S+)$`)

// StrandedRefs is pm where's and pm doctor's line for each kept stranded draft (StrandedRef): its pages and sprints,
// and how to recover them.
func StrandedRefs(store string) []string {
	list, err := Git(store, "for-each-ref", "--format=%(refname)", "refs/pm/workspaces/")
	if err != nil {
		return []string{"stranded  unreadable: " + err.Error()}
	}
	var out []string
	for _, ref := range strings.Split(list, "\n") {
		if !strings.HasSuffix(ref, "/stranded") {
			continue
		}
		msg, err := Git(store, "log", "-1", "--format=%B", ref)
		if err != nil {
			out = append(out, "stranded  "+ref+"  unreadable: "+err.Error())
			continue
		}
		var pages []string
		for _, m := range strandedTag.FindAllStringSubmatch(msg, -1) {
			pages = append(pages, fmt.Sprintf("records/%s (sprint %s, closed)", m[1], m[2]))
		}
		out = append(out, fmt.Sprintf("stranded  %s  keeps %s: take each with git -C %s show %s:<path> into a "+
			"workspace of yours and pm commit it under an open sprint; then git -C %s update-ref -d %s", ref,
			strings.Join(pages, ", "), store, ref, store, ref))
	}
	return out
}

// StrandedLine is what to do about a stranded page, as pm where, the site and the Stop hook say it.
func StrandedLine(rel, sprint string) string {
	return fmt.Sprintf("records/%s was drafted for sprint %s, which closed without publishing this text: commit it "+
		"under an open sprint that links the page (pm commit --sprint ID records/%s), or revert it", rel, sprint, rel)
}
