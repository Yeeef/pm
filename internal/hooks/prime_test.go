package hooks

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var nouns = []string{"show", "prime", "task", "hook", "push", "commit"}

func TestChunksFitTheCapAndAddUpToTheHead(t *testing.T) {
	chunks, err := Chunks(nouns)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != len(Starts) {
		t.Fatalf("%d chunks for %d starts", len(chunks), len(Starts))
	}
	var bodies []string
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > Cap {
			t.Errorf("chunk %d has %d characters, over the %d cap", i+1, n, Cap)
		}
		title, body, _ := strings.Cut(c, "\n\n")
		if !regexp.MustCompile(`^# pm rules \(\d of \d\): `).MatchString(title) {
			t.Errorf("chunk %d title %q", i+1, title)
		}
		bodies = append(bodies, body)
	}
	if strings.Join(bodies, "\n\n") != Head(nouns) {
		t.Error("the chunks without their titles are not the rules and the command list")
	}
}

func TestCommandsLeaveOutTheMachinery(t *testing.T) {
	want := "# Commands\n\n`pm` nouns: `show`, `task`, `commit`."
	if got := Commands(nouns); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// fakeSelf points Self at a shell script standing in for pm.
func fakeSelf(t *testing.T, script string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := Self
	Self = func() (string, error) { return path, nil }
	t.Cleanup(func() { Self = old })
}

func TestContextCutsAtALineToTheCapacity(t *testing.T) {
	fakeSelf(t, `i=0; while [ $i -lt 100 ]; do echo "line $i of pm show"; i=$((i+1)); done`)
	now := time.Date(2026, 10, 8, 4, 5, 0, 0, time.UTC)
	text, err := Context(nil, "", 600, 5*time.Second, "5", now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "Project state from `pm show` at session start, 2026-10-08 04:05 UTC: ") {
		t.Errorf("header: %q", text[:80])
	}
	kept, ok := strings.CutSuffix(text, cut)
	if !ok || utf8.RuneCountInString(text) > 600 {
		t.Fatalf("not cut to 600 characters with the cut line: %d characters", utf8.RuneCountInString(text))
	}
	// the kept part ends at a whole line: the last newline before 600 minus the cut line's length
	if !strings.HasSuffix(kept, " of pm show") {
		t.Errorf("cut inside a line: %q", kept[len(kept)-20:])
	}
	if utf8.RuneCountInString(kept)+len("line 99 of pm show")+1 <= 600-utf8.RuneCountInString(cut) {
		t.Errorf("cut a whole line more than needed: kept %d characters", utf8.RuneCountInString(kept))
	}
}

func TestFailuresNameOneLine(t *testing.T) {
	fakeSelf(t, `echo "warning: first" >&2; echo "error: the reason" >&2; echo "last" >&2; exit 3`)
	in, _ := Init(nil)
	if want := "pm init failed at session start (error: the reason); run `pm init` by hand.\n\n"; in != want {
		t.Errorf("init: got %q, want %q", in, want)
	}
	wh, _ := Where(nil)
	if want := "pm where failed at session start (last); run `pm where` by hand.\n\n"; wh != want {
		t.Errorf("where: got %q, want %q", wh, want)
	}
	fakeSelf(t, `exit 4`)
	show, _ := Context(nil, "", Cap, 5*time.Second, "5", time.Now())
	if want := "pm show failed at session start (exit 4); run `pm show` by hand."; show != want {
		t.Errorf("show: got %q, want %q", show, want)
	}
}

func TestATimeoutIsNamedAsPythonNamesIt(t *testing.T) {
	fakeSelf(t, `sleep 5`)
	exe, _ := Self()
	show, _ := Context(nil, "", Cap, 100*time.Millisecond, "0.1", time.Now())
	want := "pm show did not run at session start (TimeoutExpired: Command '['" + exe +
		"', 'show', '--refresh-inbox']' timed out after 0.1 seconds); run `pm show` by hand."
	if show != want {
		t.Errorf("got %q, want %q", show, want)
	}
}
