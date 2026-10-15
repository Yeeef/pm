package store

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The published records are the records branch's tip, whatever the working tree holds; the store's uncommitted record
// edits (modified, staged, removed, untracked, a staged rename) are one draft, and a store without any has none.
func TestPublishedIsTheBranchAndEditsAreTheUncommittedRecords(t *testing.T) {
	_, store := clone(t)
	write(t, store, "docs/kept.md", "kept")
	write(t, store, "docs/gone.md", "gone")
	write(t, store, "docs/moved.md", "moved")
	write(t, store, "days/2026-10-01.summary.json", "{}")
	sh(t, store, "git add -A", "git commit -qm more")
	if drafts := (Edits{store}).Drafts(); drafts != nil {
		t.Fatalf("a clean store: %v; want no draft", drafts)
	}

	write(t, store, "projects/demo.md", project+"edited\n")
	write(t, store, "docs/new.md", "new")
	write(t, store, "docs/notes.txt", "not a record")
	sh(t, store, "rm docs/gone.md", "git mv docs/moved.md docs/renamed.md")
	write(t, store, "days/2026-10-02.summary.json", "{}")

	texts, sums, err := Published(store)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(store)
	if err != nil {
		t.Fatal(err)
	}
	at := func(rel string) string { return filepath.Join(root, rel) }
	if want := map[string]string{at("projects/demo.md"): project, at("docs/kept.md"): "kept", at("docs/gone.md"): "gone",
		at("docs/moved.md"): "moved"}; !reflect.DeepEqual(texts, want) {
		t.Errorf("Published texts = %v, want %v", texts, want)
	}
	if want := map[string]string{at("days/2026-10-01.summary.json"): "{}"}; !reflect.DeepEqual(sums, want) {
		t.Errorf("Published summaries = %v, want %v", sums, want)
	}

	drafts := (Edits{store}).Drafts()
	if len(drafts) != 1 || drafts[0].Err != nil {
		t.Fatalf("Drafts = %v; want one, read", drafts)
	}
	d := drafts[0]
	got := map[string]any{}
	for rel, text := range d.Files {
		if text == nil {
			got[rel] = nil
		} else {
			got[rel] = *text
		}
	}
	want := map[string]any{"projects/demo.md": project + "edited\n", "docs/new.md": "new", "docs/gone.md": nil,
		"docs/moved.md": nil, "docs/renamed.md": "moved"}
	if d.Name != EditsDraft || d.Label != "uncommitted edits in the shared store" || d.State != "uncommitted edits" ||
		!reflect.DeepEqual(got, want) {
		t.Errorf("the draft is %s %q %q %v, want %v", d.Name, d.Label, d.State, got, want)
	}
}

// A draft that cannot be read is still listed, with its error, which fails only its version: a record path git lists
// that is no file (a link to a directory), or a store git status fails in.
func TestADraftThatCannotBeReadCarriesItsError(t *testing.T) {
	_, store := clone(t)
	sh(t, store, "mkdir -p docs/dir", "ln -s dir docs/link.md")
	drafts := (Edits{store}).Drafts()
	if len(drafts) != 1 || drafts[0].Name != EditsDraft || drafts[0].Err == nil ||
		!strings.Contains(drafts[0].Err.Error(), "reading docs/link.md") {
		t.Errorf("a link to a directory: %+v", drafts)
	}
	drafts = (Edits{t.TempDir()}).Drafts()
	if len(drafts) != 1 || drafts[0].Err == nil || !strings.Contains(drafts[0].Err.Error(), "git status failed") {
		t.Errorf("no git repository: %+v", drafts)
	}
}
