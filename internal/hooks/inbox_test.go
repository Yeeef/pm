package hooks

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakePending answers texts, and records whether deliver ran and what it saw printed by then.
type fakePending struct {
	texts     []string
	readErr   error
	markErr   error
	delivered bool
	printed   string
	out       *bytes.Buffer
}

func (f *fakePending) read(dir, session string) ([]string, func() error, error) {
	if f.readErr != nil {
		return nil, nil, f.readErr
	}
	return f.texts, func() error { f.delivered, f.printed = true, f.out.String(); return f.markErr }, nil
}

func TestInboxHandsOverTheSessionsUndeliveredRepliesBeforeItMarksThem(t *testing.T) {
	reply := "pm: owner reply to decision x.1 (Rename X?), relayed from the site:\n  [2026-10-03T12:00:00Z] Yes."
	want := `{"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": "The owner replied to ` +
		`requests this session raised:\n\n` + strings.ReplaceAll(reply, "\n", `\n`) + `"}}` + "\n"
	var out, errb bytes.Buffer
	f := &fakePending{texts: []string{reply}, out: &out}
	in := `{"hook_event_name": "UserPromptSubmit", "session_id": "s", "cwd": "/here", "prompt": "go on"}`
	if err := HookInbox("/x", f.read, strings.NewReader(in), &out, &errb); err != nil || errb.Len() != 0 {
		t.Fatalf("%v %q", err, errb.String())
	}
	if out.String() != want || !f.delivered || f.printed != want {
		t.Errorf("got %s, delivered %v after printing %q", out.String(), f.delivered, f.printed)
	}
}

func TestInboxIsNotOnStopAndFailsOpen(t *testing.T) {
	for name, c := range map[string]struct {
		in              string
		f               *fakePending
		stderr          string
		wantOut, marked bool
	}{
		"a stop":     {`{"hook_event_name": "Stop", "session_id": "s"}`, &fakePending{texts: []string{"r"}}, "", false, false},
		"no session": {`{"hook_event_name": "UserPromptSubmit"}`, &fakePending{texts: []string{"r"}}, "", false, false},
		"nothing":    {`{"hook_event_name": "UserPromptSubmit", "session_id": "s"}`, &fakePending{}, "", false, true},
		"read fails": {`{"hook_event_name": "UserPromptSubmit", "session_id": "s"}`,
			&fakePending{readErr: errors.New("work store:\n down")},
			"pm hook inbox: reading this session's replies failed (work store: down); no reply handed over\n", false, false},
		"mark fails": {`{"hook_event_name": "UserPromptSubmit", "session_id": "s"}`,
			&fakePending{texts: []string{"r"}, markErr: errors.New("write lock held")},
			"pm hook inbox: marking the replies delivered failed (write lock held); they may be handed over again\n", true, true},
	} {
		var out, errb bytes.Buffer
		c.f.out = &out
		if err := HookInbox("/here", c.f.read, strings.NewReader(c.in), &out, &errb); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if (out.Len() > 0) != c.wantOut || errb.String() != c.stderr || c.f.delivered != c.marked {
			t.Errorf("%s: out %q, stderr %q, marked %v", name, out.String(), errb.String(), c.f.delivered)
		}
	}
}
