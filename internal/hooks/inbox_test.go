package hooks

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestInboxHandsOverTheSessionsUndeliveredReplies(t *testing.T) {
	reply := "pm: owner reply to decision x.1 (Rename X?), relayed from the site:\n  [2026-10-03T12:00:00Z] Yes."
	lead := "The owner replied to requests this session raised; pm marked the replies delivered:\\n\\n"
	quoted := strings.ReplaceAll(reply, "\n", `\n`)
	for in, want := range map[string]string{
		`{"hook_event_name": "UserPromptSubmit", "session_id": "s", "prompt": "go on"}`: `{"hookSpecificOutput": ` +
			`{"hookEventName": "UserPromptSubmit", "additionalContext": "` + lead + quoted + `"}}` + "\n",
		`{"hook_event_name": "Stop", "session_id": "s", "stop_hook_active": false}`: `{"decision": "block", ` +
			`"reason": "` + lead + quoted + `"}` + "\n",
		`{"hook_event_name": "Stop", "session_id": "s", "stop_hook_active": true}`: "", // never loops
		`{"hook_event_name": "Stop"}`: "", // no session
	} {
		read := func(dir, session string) ([]string, error) {
			if session != "s" || dir != "/here" {
				t.Errorf("read %s %s", dir, session)
			}
			return []string{reply}, nil
		}
		var out, errb bytes.Buffer
		if err := HookInbox("/here", read, strings.NewReader(in), &out, &errb); err != nil || errb.Len() != 0 {
			t.Fatalf("%s: %v %q", in, err, errb.String())
		}
		if out.String() != want {
			t.Errorf("%s:\ngot  %s\nwant %s", in, out.String(), want)
		}
	}
}

func TestInboxIsSilentWithNothingPendingAndFailsOpen(t *testing.T) {
	for name, read := range map[string]Pending{
		"nothing": func(string, string) ([]string, error) { return nil, nil },
		"error":   func(string, string) ([]string, error) { return nil, errors.New("work store:\n down") },
	} {
		var out, errb bytes.Buffer
		err := HookInbox("/here", read, strings.NewReader(`{"hook_event_name": "Stop", "session_id": "s"}`), &out, &errb)
		wantErr := ""
		if name == "error" {
			wantErr = "pm hook inbox: reading this session's replies failed (work store: down); no reply handed over\n"
		}
		if err != nil || out.Len() != 0 || errb.String() != wantErr {
			t.Errorf("%s: %v %q %q", name, err, out.String(), errb.String())
		}
	}
}
