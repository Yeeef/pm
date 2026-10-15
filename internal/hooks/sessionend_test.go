package hooks

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/session"
)

func TestSessionEndRemovesTheModeFileAndFailsOpen(t *testing.T) {
	dir := gitDir(t)
	end := func(in string) string {
		t.Helper()
		var errb bytes.Buffer
		if err := HookSessionEnd(dir, strings.NewReader(in), &errb); err != nil {
			t.Fatal(err)
		}
		return errb.String()
	}
	for _, id := range []string{"s", "other"} {
		if err := session.SetInteractive(dir, id, true); err != nil {
			t.Fatal(err)
		}
	}
	if errs := end(`{"hook_event_name": "SessionEnd", "session_id": "s", "reason": "other"}`); errs != "" {
		t.Errorf("stderr %q", errs)
	}
	for id, want := range map[string]bool{"s": false, "other": true} {
		if on, err := session.Interactive(dir, id); err != nil || on != want {
			t.Errorf("%s: interactive %v %v, want %v", id, on, err, want)
		}
	}
	if errs := end(`{"hook_event_name": "SessionEnd", "session_id": "s"}`); errs != "" { // already gone: a no-op
		t.Errorf("stderr %q", errs)
	}
	for in, said := range map[string]string{
		`[1]`: "the hook input names no ended session",
		`{"hook_event_name": "Stop", "session_id": "s"}`:          "the hook input names no ended session",
		`{"hook_event_name": "SessionEnd", "session_id": "../s"}`: "removing the session's mode file failed",
	} {
		if errs := end(in); !strings.Contains(errs, "pm hook session-end: "+said) {
			t.Errorf("%s: stderr %q", in, errs)
		}
	}
}

func TestTheStateOpensWithTheRequestRuleOfTheSessionsMode(t *testing.T) {
	dir := gitDir(t)
	if err := session.SetInteractive(dir, "s", true); err != nil {
		t.Fatal(err)
	}
	for id, on := range map[string]bool{"s": true, "other": false, "": false, "../s": false} {
		if got := requestRuleFor(&dir, id); got != RequestRule(on) {
			t.Errorf("%q: got the rule of interactive %v", id, !on)
		}
	}
	nowhere := t.TempDir() // not a repo: the strict rule
	if got := requestRuleFor(&nowhere, "s"); got != RequestRule(false) {
		t.Error("outside a repo: not autopilot's rule")
	}
	if RequestRule(true) == RequestRule(false) || !strings.Contains(RequestRule(false), "Never leave a request only in chat") {
		t.Error("the two rules")
	}
}

func TestTheRequestRuleHookGivesTheModesRuleAndAutopilotsOnAnyFailure(t *testing.T) {
	dir := gitDir(t)
	if err := session.SetInteractive(dir, "s", true); err != nil {
		t.Fatal(err)
	}
	for in, on := range map[string]bool{
		`{"hook_event_name": "SessionStart", "session_id": "s", "cwd": "` + dir + `"}`:     true,
		`{"hook_event_name": "SessionStart", "session_id": "other", "cwd": "` + dir + `"}`: false,
		`{"hook_event_name": "SessionStart", "session_id": "s", "cwd": 7}`:                 false, // no repo found
		`{"hook_event_name": "SessionStart", "session_id": 7, "cwd": "` + dir + `"}`:       false,
		`[1]`: false,
	} {
		var out bytes.Buffer
		if err := CmdPrime(Part{RequestRule: true}, true, nouns, strings.NewReader(in), &out); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		var want bytes.Buffer
		if err := writeJSON(&want, contextFor("SessionStart", RequestRule(on))); err != nil {
			t.Fatal(err)
		}
		if out.String() != want.String() {
			t.Errorf("%s: got %s", in, out.String())
		}
	}
}
