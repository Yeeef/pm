package hooks

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Yeeef/pm/internal/ask"
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
	text, err := Context(nil, "", 600, 5*time.Second, "5", func() time.Time { return now })
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
	in, err := Init(nil)
	if err != nil {
		t.Fatal(err) // a failed run is its line, never an error
	}
	if want := "pm init failed at session start (error: the reason); run `pm init` by hand.\n\n"; in != want {
		t.Errorf("init: got %q, want %q", in, want)
	}
	wh, err := Where(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "pm where failed at session start (last); run `pm where` by hand.\n\n"; wh != want {
		t.Errorf("where: got %q, want %q", wh, want)
	}
	fakeSelf(t, `exit 4`)
	show, err := Context(nil, "", Cap, 5*time.Second, "5", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if want := "pm show failed at session start (exit 4); run `pm show` by hand."; show != want {
		t.Errorf("show: got %q, want %q", show, want)
	}
	t.Run("AFailedCheckExitsOneWithItsCause", func(t *testing.T) {
		read := func(dir, session string) ([]Request, error) { return nil, errors.New("work store: none at /x") }
		for in, want := range map[string]string{
			`[1]`: "the hook input is not a JSON object",
			`{}`:  "the hook input has no session_id",
			`{"session_id": "s", "last_assistant_message": 7}`: "the hook input's last_assistant_message is a number, " +
				"not a string", // null or missing is no reply, which asks nothing
			`{"session_id": "s", "last_assistant_message": "Ask?"}`: "reading this session's open needs failed: " +
				"work store: none at /x",
		} {
			for _, runtime := range []ask.Runtime{ask.Claude, ask.Codex} { // the same failure on either runtime
				var out, errb bytes.Buffer
				code, err := HookOwnerRequest(gitDir(t), runtime, read, strings.NewReader(in), &out, &errb)
				if code != 1 || err != nil || out.Len() != 0 ||
					errb.String() != "pm hook owner-request: "+want+"; the owner-request check did not run\n" {
					t.Errorf("%s %s: code %d, err %v, stdout %q, stderr %q", runtime, in, code, err, out.String(), errb.String())
				}
			}
		}
	})
	t.Run("ParseJudgedRefusesAnAnswerOutOfShape", func(t *testing.T) {
		for _, out := range []string{
			"", "no JSON here", `{"items": null}`, `{"verdict": []}`, `{"items": [1]}`, `{"items": {}}`,
			`{"items": [{"quote": "Q", "kind": "decision", "match": 7}]}`, `{"items": [{"quote": "Q"}]}`,
			`{"items": [{"quote": 1, "kind": "decision"}]}`, `{"items": []} and {"items": []}`, `{"items": [}`,
		} {
			if items, ok := parseJudged(out); ok {
				t.Errorf("%q: got %v, want out of shape", out, items)
			}
		}
	})
}

// gitDir is a new git repo, the main checkout a hook finds from its directory.
func gitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}
