package launch

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Yeeef/pm"
)

// testNotes is a build's notes (testdata/notes): four releases, and the entry files of the next one, of which only b.md
// and c.md give steps of their own. tests/test_changelog.py holds release/changelog.py's assemble of those entries to
// testdata/assembled-0.5.0.txt, the guide this test wants for 0.5.0.
var testNotes = os.DirFS("testdata/notes")

func testChangelog(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/notes/CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGuidesPrintsEachCrossedReleasesGuideOldestFirst(t *testing.T) {
	g030 := "pm 0.3.0:\n1. Install 0.3.0.\n2. Run pm upgrade --to 0.3.0."
	g031 := "pm 0.3.1:\n1. Install 0.3.1.\n   It wraps.\n2. Run pm upgrade --to 0.3.1."
	g040 := "pm 0.4.0:\n1. Install 0.4.0.\n2. Run pm upgrade --to 0.4.0.\n3. Restart the service; then merge every " +
		"worktree's branch (steps 1 and 2 first)."
	// 0.5.0 has no section yet: its entry files give its guide, the two steps every release opens with, then b's
	// and c's own, renumbered after them, in file name order
	assembled, err := os.ReadFile("testdata/assembled-0.5.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	g050 := "pm 0.5.0:\n" + strings.TrimSuffix(string(assembled), "\n")
	for _, c := range []struct {
		from, to string
		want     []string // each crossed release's guide, in order; none when the move crosses none
	}{
		{"0.3.0", "0.5.0", []string{g031, g040, g050}},
		{"0.2.0", "0.4.0", []string{g030, g031, g040}},
		{"0.3.1", "0.4.0-rc.1", []string{g040}}, // X's section when there is one
		{"0.4.0-rc.1", "0.4.0", []string{g040}},
		{"0.5.0-rc.1", "0.5.0-rc.2", []string{strings.ReplaceAll(g050, "0.5.0", "0.5.0-rc.2")}},
		{"0.5.0-rc.2", "0.5.0-rc.2", nil},
		{"0.1.5", "0.3.0", []string{"pm 0.2.0:\n1. Install 0.2.0.\n2. Run pm upgrade --to 0.2.0.", g030}},
		{"not a version", "0.2.0", []string{"pm 0.2.0:\n1. Install 0.2.0.\n2. Run pm upgrade --to 0.2.0."}},
		{"0.4.0", "0.4.0", nil}, {"0.4.0", "0.3.0", nil}, {"0.3.0", "dev", nil},
	} {
		got, err := Guides(testNotes, c.from, c.to)
		if err != nil {
			t.Fatalf("Guides(%q, %q): %v", c.from, c.to, err)
		}
		if c.want == nil {
			if got != "" {
				t.Errorf("Guides(%q, %q) = %q, want none", c.from, c.to, got)
			}
			continue
		}
		head, body, _ := strings.Cut(got, "\n\n")
		if !strings.HasPrefix(head, "pm "+c.from+" to "+c.to+" crosses ") {
			t.Errorf("Guides(%q, %q) opens %q", c.from, c.to, head)
		}
		if want := strings.Join(c.want, "\n\n") + "\n\n"; body != want {
			t.Errorf("Guides(%q, %q) =\n%s\nwant\n%s", c.from, c.to, body, want)
		}
	}
}

func TestGuidesFailWhenACrossedReleaseHasNoGuide(t *testing.T) {
	notes := fstest.MapFS{"CHANGELOG.md": {Data: []byte(strings.Replace(testChangelog(t),
		"### Upgrade guide\n\n1. Install 0.3.1.\n   It wraps.\n2. Run pm upgrade --to 0.3.1.\n\n", "", 1))}}
	_, err := Guides(notes, "0.3.0", "0.4.0")
	if err == nil || !strings.Contains(err.Error(), "give release 0.3.1 no upgrade guide") {
		t.Fatalf("err = %v, want release 0.3.1 named", err)
	}
	if _, err := Guides(fstest.MapFS{}, "0.3.0", "0.4.0"); err == nil {
		t.Fatal("no CHANGELOG.md gave no error")
	}
}

// The notes this pm embeds give every release a guide: a move from before the first one to past the last crosses
// them all.
func TestEmbeddedNotesGiveEveryReleaseAGuide(t *testing.T) {
	got, err := Guides(pm.Notes, "0.1.0", "999.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"0.2.0", "0.5.0", "0.8.0", "999.0.0"} {
		if !strings.Contains(got, "\npm "+v+":\n1. ") {
			t.Errorf("no guide for %s in\n%s", v, got)
		}
	}
}

// With no entry files, changelog.d/ holds only its README, which is no entry: X's guide is the two steps every
// release opens with.
func TestGuidesWithNoEntryFilesGiveTheTwoSteps(t *testing.T) {
	readme, err := os.ReadFile("testdata/notes/changelog.d/README.md")
	if err != nil {
		t.Fatal(err)
	}
	notes := fstest.MapFS{"CHANGELOG.md": {Data: []byte(testChangelog(t))}, "changelog.d/README.md": {Data: readme}}
	got, err := Guides(notes, "0.4.0", "0.5.0")
	if err != nil {
		t.Fatal(err)
	}
	want := "pm 0.5.0:\n1. On each machine, install the release:\n" +
		"   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.5.0/install.sh | sh`.\n" +
		"2. In the repo, run `pm upgrade --to 0.5.0`, then commit what it changes and merge it, as an ordinary PR.\n\n"
	if _, body, _ := strings.Cut(got, "\n\n"); body != want {
		t.Fatalf("Guides =\n%s\nwant\n%s", body, want)
	}
}
