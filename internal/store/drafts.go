package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/records"
)

// Draft is a change to the published records that the site shows as a version of each page it changes.
type Draft struct {
	Name  string // the version's URL segment, /drafts/<Name>/…: unique among a clone's drafts
	Label string // what the draft is, as the version bar names it
	State string // how far it is: committed (a draft commit), uncommitted edits, or conflict (markers in a file)
	// Worktree is a workspace's code worktree and Branch that worktree's branch; "" for the store's edits.
	Worktree, Branch string
	// Files is each record file the draft changes, by its path in the store (design/x.md): its text, or nil when the
	// draft removes it.
	Files map[string]*string
	// Err is why the draft could not be read; its version fails with it, and only its version.
	Err error
}

// DraftSource lists a clone's drafts, which the site serves as versions beside the published records. A draft that
// cannot be read is listed with its Err, so it never fails the published site.
type DraftSource interface {
	Drafts() []Draft
}

// EditsDraft is the name of the shared store's uncommitted edits as a draft.
const EditsDraft = "store"

// Edits is the shared store's uncommitted record edits as one draft named EditsDraft, or no draft when it has none:
// each record file that git status lists (modified, staged, removed or untracked) with its text in the working tree.
// One git status, with no optional lock (Uncommitted says why). A file the status lists may hold its published text
// (a mode change, an edit staged then undone): the site drops those.
type Edits struct{ Store string }

func (e Edits) Drafts() []Draft {
	files, err := e.files()
	if err == nil && len(files) == 0 {
		return nil
	}
	return []Draft{{Name: EditsDraft, Label: "uncommitted edits in the shared store", State: "uncommitted edits",
		Files: files, Err: err}}
}

func (e Edits) files() (map[string]*string, error) {
	out, errOut, err := run(e.Store, "--no-optional-locks", "status", "--porcelain", "-z", "--no-renames",
		"--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status failed in %s: %s", e.Store, records.Strip(errOut))
	}
	files := map[string]*string{}
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("git status --porcelain -z in %s printed %q, not a status and a path", e.Store, entry)
		}
		rel := entry[3:]
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(e.Store, filepath.FromSlash(rel)))
		switch {
		case errors.Is(err, fs.ErrNotExist): // removed, or gone since git status listed it: the next look sees which
			files[rel] = nil
		case err != nil:
			return nil, fmt.Errorf("reading %s: %w", rel, err)
		default:
			text := string(b)
			files[rel] = &text
		}
	}
	return files, nil
}

// workspacesDraft names the draft that says why the workspaces could not be listed.
const workspacesDraft = "workspaces"

// Clone is a clone's drafts: the store's uncommitted edits (Edits), then each workspace's, named by its code
// worktree's admin name.
type Clone struct{ Store string }

func (c Clone) Drafts() []Draft {
	out := Edits(c).Drafts()
	trees, err := Git(c.Store, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return append(out, Draft{Name: workspacesDraft, Label: "the records workspaces", Err: err})
	}
	branches := map[string]string{} // each worktree's branch, by its path
	type entry struct{ path, branch string }
	var workspaces []entry
	for _, block := range strings.Split(trees, "\x00\x00") {
		var e entry
		prunable := false
		for _, field := range strings.Split(block, "\x00") {
			key, value, _ := strings.Cut(field, " ")
			switch key {
			case "worktree":
				e.path = value
			case "branch":
				e.branch = strings.TrimPrefix(value, "refs/heads/")
			case "prunable":
				prunable = true
			}
		}
		branches[e.path] = e.branch
		if strings.HasPrefix(e.branch, DraftsPrefix) && !prunable {
			workspaces = append(workspaces, e)
		}
	}
	for _, e := range workspaces {
		w := Workspace{Code: filepath.Dir(e.path), Dir: e.path, Name: strings.TrimPrefix(e.branch, DraftsPrefix)}
		d := Draft{Name: w.Name, Worktree: w.Code, Branch: branches[w.Code]}
		d.Files, d.State, d.Err = c.workspaceFiles(w)
		out = append(out, d)
	}
	return out
}

// workspaceFiles is the record files a workspace changes against its base, its draft commit's then its uncommitted
// ones over them, and its state.
func (c Clone) workspaceFiles(w Workspace) (map[string]*string, string, error) {
	d, err := readDraft(c.Store, w)
	if err != nil {
		return nil, "", err
	}
	files := map[string]*string{}
	var states []string
	if d.draft {
		if files, _, err = DraftPages(c.Store, w); err != nil {
			return nil, "", err
		}
		if len(files) > 0 {
			states = append(states, "committed")
		}
	}
	dirty, err := w.Dirty()
	if err != nil {
		return nil, "", err
	}
	conflict, edited := false, false
	for _, rel := range dirty {
		conflict = conflict || w.binaryConflict(rel)
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		text, err := w.read(rel)
		if err != nil {
			return nil, "", err
		}
		files[rel], edited = text, true
		conflict = conflict || (text != nil && HasMarkers(*text))
	}
	if edited {
		states = append(states, "uncommitted edits")
	}
	if conflict {
		states = append(states, "conflict")
	}
	return files, strings.Join(states, ", "), nil
}

// WorkspaceStamp is what a look at the workspaces reads without git, from the admin dirs under the clone's common git
// dir: each workspace's directory and its branch's ref file.
func WorkspaceStamp(common string) (dirs, refs []string) {
	admins, err := os.ReadDir(filepath.Join(common, "worktrees"))
	if err != nil { // none: no worktree but the main checkout; another failure, the next look reads it again
		return nil, nil
	}
	for _, a := range admins {
		admin := filepath.Join(common, "worktrees", a.Name())
		head, err := os.ReadFile(filepath.Join(admin, "HEAD"))
		ref, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: ")
		if err != nil || !ok || !strings.HasPrefix(ref, "refs/heads/"+DraftsPrefix) {
			continue
		}
		gitdir, err := os.ReadFile(filepath.Join(admin, "gitdir"))
		if err != nil {
			continue
		}
		dirs = append(dirs, filepath.Dir(strings.TrimSpace(string(gitdir))))
		refs = append(refs, filepath.Join(common, filepath.FromSlash(ref)))
	}
	return dirs, refs
}

// Files is each record file the workspace changes against its base, by its path in the store (design/x.md): its draft
// commit's pages, then its uncommitted record edits over them; its text, or nil when it removes the file. The site's
// version of the workspace lays the same files over the published records.
func (w Workspace) Files(store string) (map[string]*string, error) {
	files, _, err := Clone{Store: store}.workspaceFiles(w)
	return files, err
}

// Working is the records as the worktree containing cwd edits them, for pm check: in a worktree with a records
// workspace, the published records with the workspace's drafts over them, and the design pages it drafts (design/x.md);
// elsewhere the store's records as they are.
func Working(cwd, store string) ([]*records.Record, map[string]bool, error) {
	w, err := WorkspaceOf(cwd)
	if err != nil || w == nil {
		recs, err := records.Read(store, nil)
		return recs, nil, err
	}
	files, err := w.Files(store)
	if err != nil {
		return nil, nil, err
	}
	changes, drafts := map[string]*string{}, map[string]bool{}
	for rel, text := range files {
		changes[filepath.Join(store, filepath.FromSlash(rel))] = text
		drafts[rel] = path.Dir(rel) == "design"
	}
	recs, err := Committed(store, changes)
	return recs, drafts, err
}
