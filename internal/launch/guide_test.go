package launch

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Yeeef/pm"
)

const testChangelog = `# Changelog

Preamble, with a numbered list that is no release's:

1. Not a step.

## [0.4.0] - 2026-10-04

Summary of 0.4.0.

### Upgrade guide

1. Install 0.4.0.
2. Run pm upgrade --to 0.4.0.
3. Restart the service; then merge every worktree's branch (steps 1 and 2 first).

### Added

- Something.

## [0.3.1] - 2026-10-03

### Upgrade guide

1. Install 0.3.1.
   It wraps.
2. Run pm upgrade --to 0.3.1.

### Fixed

- A fix.

## [0.3.0] - 2026-10-02

### Upgrade guide

1. Install 0.3.0.
2. Run pm upgrade --to 0.3.0.

## [0.2.0] - 2026-10-01

### Upgrade guide

1. Install 0.2.0.
2. Run pm upgrade --to 0.2.0.

[0.4.0]: https://github.com/Yeeef/pm/compare/pm-v0.3.1...pm-v0.4.0
[0.3.1]: https://github.com/Yeeef/pm/compare/pm-v0.3.0...pm-v0.3.1
`

// testNotes is a build's notes: four releases, and the entry files of the next one, which only b.md and c.md give
// steps of their own.
var testNotes = fstest.MapFS{
	"CHANGELOG.md":     {Data: []byte(testChangelog)},
	"changelog.d/c.md": {Data: []byte("Summary of c.\n\n### Upgrade guide\n\n1. Re-trust Codex.\n   It wraps.\n2. Restart.\n\n### Fixed\n\n- c.\n")},
	"changelog.d/a.md": {Data: []byte("Summary of a.\n\n### Added\n\n- a.\n")},
	"changelog.d/b.md": {Data: []byte("### Upgrade guide\n\n1. Set a key.\n\n### Changed\n\n- b.\n")},
}

func TestGuidesPrintsEachCrossedReleasesGuideOldestFirst(t *testing.T) {
	g030 := "pm 0.3.0:\n1. Install 0.3.0.\n2. Run pm upgrade --to 0.3.0."
	g031 := "pm 0.3.1:\n1. Install 0.3.1.\n   It wraps.\n2. Run pm upgrade --to 0.3.1."
	g040 := "pm 0.4.0:\n1. Install 0.4.0.\n2. Run pm upgrade --to 0.4.0.\n3. Restart the service; then merge every " +
		"worktree's branch (steps 1 and 2 first)."
	// 0.5.0 has no section yet: its entry files give its guide, the two steps every release opens with, then b's
	// and c's own, renumbered after them, in file name order
	g050 := "pm 0.5.0:\n1. On each machine, install the release:\n" +
		"   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.5.0/install.sh | sh`.\n" +
		"2. In the repo, run `pm upgrade --to 0.5.0`, then commit what it changes and merge it, as an ordinary PR.\n" +
		"3. Set a key.\n4. Re-trust Codex.\n   It wraps.\n5. Restart."
	for _, c := range []struct {
		from, to string
		want     []string // each crossed release's guide, in order; none when the move crosses none
	}{
		{"0.3.0", "0.5.0", []string{g031, g040, g050}},
		{"0.2.0", "0.4.0", []string{g030, g031, g040}},
		{"0.3.1", "0.4.0-rc.1", []string{g040}}, // X's section when there is one
		{"0.4.0-rc.1", "0.4.0", []string{g040}},
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
	notes := fstest.MapFS{"CHANGELOG.md": {Data: []byte(strings.Replace(testChangelog,
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
