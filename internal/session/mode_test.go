package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestASessionsModeIsItsFileUnderRunAndOnlyASafeIDHasOne(t *testing.T) {
	main := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	interactive := func(id string) bool {
		t.Helper()
		on, err := Interactive(main, id)
		if err != nil {
			t.Fatal(err)
		}
		return on
	}
	if interactive("s-1") {
		t.Fatal("a session with no mode file is interactive; autopilot is the default")
	}
	for _, on := range []bool{true, true, false, false} { // either way twice: setting the mode it has is a no-op
		if err := SetInteractive(main, "s-1", on); err != nil {
			t.Fatal(err)
		}
		if interactive("s-1") != on {
			t.Fatalf("set %v, read %v", on, !on)
		}
	}
	if _, err := os.Stat(filepath.Join(main, ".pm/run/sessions")); err != nil {
		t.Fatalf("the mode files' directory: %v", err)
	}
	for _, id := range []string{"", "..", "../s", "a/b", "s.1", "s 1"} {
		if err := SetInteractive(main, id, true); err == nil {
			t.Errorf("the session id %q set a mode file", id)
		}
		if interactive(id) {
			t.Errorf("the session id %q is interactive", id)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(main, ".pm/run")); err != nil || len(entries) != 1 {
		t.Errorf("a refused id wrote under .pm/run: %v %v", entries, err)
	}

	// Modes: each file with whether its session was active within ModeWindow: a Claude Code transcript written two
	// hours ago (idle past LiveWindow) keeps its mode, one written 25 hours ago does not
	if modes, err := Modes(t.TempDir()); err != nil || modes != nil {
		t.Errorf("no mode directory: %v %v", modes, err)
	}
	for id, age := range map[string]time.Duration{"away": 2 * time.Hour, "gone": 25 * time.Hour} {
		if err := SetInteractive(main, id, true); err != nil {
			t.Fatal(err)
		}
		transcript := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "-repo", id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(transcript), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(transcript, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(transcript, at, at); err != nil {
			t.Fatal(err)
		}
	}
	modes, err := Modes(main)
	dir := filepath.Join(main, ".pm/run/sessions")
	want := []Mode{{filepath.Join(dir, "away"), "away", true}, {filepath.Join(dir, "gone"), "gone", false}}
	if err != nil || !reflect.DeepEqual(modes, want) {
		t.Errorf("Modes: %+v %v", modes, err)
	}
	if Live("away") {
		t.Error("a session idle for two hours is live")
	}
}
