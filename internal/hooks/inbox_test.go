package hooks

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
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

func (f *fakePending) read(dir, session, prompt string) ([]string, func() error, error) {
	if f.readErr != nil {
		return nil, nil, f.readErr
	}
	return f.texts, func() error { f.delivered, f.printed = true, f.out.String(); return f.markErr }, nil
}

func TestInboxHandsOverTheSessionsUndeliveredRepliesBeforeItMarksThem(t *testing.T) {
	reply := "pm: owner reply to decision x.1 (Rename X?), relayed from the site:\n  [2026-10-03T12:00:00Z] Yes."
	want := `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"The owner replied to ` +
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

// replyAfterRead is an askStore as a work.Store that the owner replies on the site to need right after the first read
// of the session's needs. Another method of work.Store panics: the inbox calls none.
type replyAfterRead struct {
	work.Store
	s     *askStore
	need  string
	reads int
}

func (r *replyAfterRead) Needs(session string) ([]work.Item, error) {
	needs, err := r.s.Needs(session)
	if r.reads++; r.reads == 1 {
		r.s.reply(r.need, "Blue")
	}
	return needs, err
}

func (r *replyAfterRead) Get(ids ...string) ([]work.Item, error) { return r.s.Get(ids...) }
func (r *replyAfterRead) UpdateNeed(id string, u work.NeedUpdate) error {
	return r.s.UpdateNeed(id, u)
}
func (r *replyAfterRead) Close(id, reason string, res work.Resolution, session string) error {
	return r.s.Close(id, reason, res, session)
}
func (r *replyAfterRead) Answer(id, text, session string) error { return r.s.Answer(id, text, session) }
func (r *replyAfterRead) Comment(id string, kind work.CommentKind, author, text string) (work.Comment, error) {
	return r.s.Comment(id, kind, author, text)
}
func (r *replyAfterRead) Shutdown() error { return r.s.Shutdown() }

// TestInboxMarksAndClosesWhatItRead: a site reply that lands on a Codex question after the inbox read the session's
// needs is not printed, so it is neither marked delivered nor closes the question as answered on the site: the prompt
// typed in the chat answers it, and the reply is recorded as the one that did not reach the session.
func TestInboxMarksAndClosesWhatItRead(t *testing.T) {
	q := work.Item{ID: "q-1", Type: work.Need, Status: work.Open, Need: &work.NeedInfo{Kind: work.Question,
		Ask: &work.AskInfo{Call: "call_1", Questions: colorAsk("call_1").Questions},
		RaisedBy: &work.RaisedBy{Session: "s", Inbox: session.CodexInbox("/codex-home")}}}
	s := newAskStore(q)
	texts, deliver, err := sessionReplies(&replyAfterRead{s: s, need: q.ID}, service.Texts{}, "s", "red, please")
	if err != nil {
		t.Fatal(err)
	}
	if err := deliver(); err != nil {
		t.Fatal(err)
	}
	got := s.item(q.ID)
	var said []string
	for _, c := range got.Comments {
		said = append(said, string(c.Kind)+": "+c.Text)
	}
	if len(texts) != 0 || got.Status != work.Closed || got.CloseReason == service.AnsweredOnSite ||
		got.Need.Delivered != 1 || len(said) != 3 || !strings.HasSuffix(said[1], "red, please") {
		t.Errorf("printed %q; closed %q (%s), delivered %d, comments %q", texts, got.CloseReason, got.Status,
			got.Need.Delivered, said)
	}
}
