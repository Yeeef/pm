package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
// same name is reused when it holds no draft commit. "" when the workspace is there already; it changes nothing when
// it fails.
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
		if d, err := readDraft(store, w); err != nil {
			return "", err
		} else if d.draft {
			return "", errorf("%s holds the draft commit %s of a removed worktree named %s; publish or drop it before "+
				"this worktree takes the name", w.Branch(), d.head, name)
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
	if _, err := Git(store, "worktree", "add", "--quiet", "-B", w.Branch(), w.Dir, "refs/heads/"+Branch); err != nil {
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
	if d.draft {
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
