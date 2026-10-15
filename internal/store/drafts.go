package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/records"
)

// Draft is a change to the published records that the site shows as a version of each page it changes.
type Draft struct {
	Name  string // the version's URL segment, /drafts/<Name>/…: unique among a clone's drafts
	Label string // what the draft is, as the version bar names it
	State string // how far it is: uncommitted edits
	// Files is each record file the draft changes, by its path in the store (design/x.md): its text, or nil when the
	// draft removes it.
	Files map[string]*string
}

// DraftSource lists a clone's drafts, which the site serves as versions beside the published records.
type DraftSource interface {
	Drafts() ([]Draft, error)
}

// EditsDraft is the name of the shared store's uncommitted edits as a draft.
const EditsDraft = "store"

// Edits is the shared store's uncommitted record edits as one draft named EditsDraft, or no draft when it has none:
// each record file that git status lists (modified, staged, removed or untracked) with its text in the working tree.
// One git status, with no optional lock (Uncommitted says why).
type Edits struct{ Store string }

func (e Edits) Drafts() ([]Draft, error) {
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
			return nil, err
		default:
			text := string(b)
			files[rel] = &text
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	return []Draft{{Name: EditsDraft, Label: "uncommitted edits in the shared store", State: "uncommitted edits",
		Files: files}}, nil
}
