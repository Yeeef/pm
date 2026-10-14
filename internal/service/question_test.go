package service

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
)

// question is an open question need raised by session thread, with its call call_1 and the inbox inbox.
func question(thread, inbox string) work.Item {
	n := need("p-1.2.9", work.Question, inbox)
	n.Need.RaisedBy.Session = thread
	n.Need.Ask = &work.AskInfo{Call: "call_1", Questions: []work.AskQuestion{{Text: "Red or blue?",
		Options: []work.AskOption{{Label: "Red"}, {Label: "Blue"}}}}}
	n.Comments = []work.Comment{{ID: "c1", Kind: work.Reply, Author: "owner", Text: "Red or blue?: Blue\n\n" +
		"<!-- pm-reply 0b6c7c1e-2d3f -->", CreatedAt: at}}
	return n
}

// rollout writes rootThread's rollout under home: the ask call, then each message the user typed.
func rollout(t *testing.T, home string, typed ...string) {
	t.Helper()
	dir := filepath.Join(home, "sessions", "2026", "10", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{`{"type":"response_item","payload":{"type":"function_call","name":"request_user_input_async",` +
		`"arguments":"{}","call_id":"call_1"}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"call_1"}}}`}
	for _, m := range typed {
		lines = append(lines, `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage",`+
			`"content":[{"type":"text","text":"`+m+`"}]}}}`)
	}
	p := filepath.Join(dir, "rollout-2026-10-12T10-00-00-"+rootThread+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func bareServer(w *fakeWork) *server { return &server{d: Deps{Open: w.Open, Log: io.Discard}} }

// TestACodexQuestionGetsTheSitesAnswerQueuedUnlessTheChatAnsweredFirst: the site's answer alone is queued into the
// live session, then the question is closed as answered on the site and marked delivered; when the rollout shows the
// owner typed an answer after the question, that answer closes it, nothing is queued, and the site's reply counts as
// delivered with a note.
func TestACodexQuestionGetsTheSitesAnswerQueuedUnlessTheChatAnsweredFirst(t *testing.T) {
	saved := CodexLive
	t.Cleanup(func() { CodexLive = saved })
	CodexLive = func(string, string) bool { return true }
	home := codexHome(t)
	rollout(t, home)
	w := newFakeWork(question(rootThread, session.CodexInbox(home)))
	log := fakeCodex(t, "0.154.0", rootThread)
	if got, err := bareServer(w).pushUndelivered("p-1.2.9"); got != "delivered" || err != nil {
		t.Fatalf("queued: %q %v", got, err)
	}
	b, err := os.ReadFile(log)
	n := w.item("p-1.2.9")
	if err != nil || !strings.HasSuffix(string(b), "--message\nRed or blue?: Blue\n") || n.Status != work.Closed ||
		n.Resolution != work.Answered || n.CloseReason != AnsweredOnSite || n.Need.Delivered != 1 {
		t.Fatalf("queued %q (%v); need %+v", b, err, n)
	}

	rollout(t, home, "Red, thanks")
	w = newFakeWork(question(rootThread, session.CodexInbox(home)))
	log = fakeCodex(t, "0.154.0", rootThread)
	if got, err := bareServer(w).pushUndelivered("p-1.2.9"); got != "answered in chat first" || err != nil {
		t.Fatalf("chat first: %q %v", got, err)
	}
	n = w.item("p-1.2.9")
	if _, err := os.Stat(log); err == nil {
		t.Error("codex was run for a question answered in the chat")
	}
	if n.Status != work.Closed || n.Resolution != work.Answered || n.Need.Delivered != 1 || len(n.Comments) != 3 ||
		n.Comments[1].Text != "Red, thanks" || n.Comments[2].Kind != work.Note {
		t.Errorf("chat first: %+v", n)
	}
}

// TestAClaudeQuestionIsNeverPushedAndEndsWithItsSession: its hook takes the answer, so the push leaves its inbox alone;
// the sweep dismisses it once its messaging socket is gone, and leaves one with no socket to its hook. A Codex
// question is never dismissed for idleness: with its rollout idle and nothing answered, it stays open.
func TestAClaudeQuestionIsNeverPushedAndEndsWithItsSession(t *testing.T) {
	sock := filepath.Join(codexHome(t), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	w := newFakeWork(question("s1", sock))
	s := bareServer(w)
	if got, err := s.pushUndelivered("p-1.2.9"); !strings.HasPrefix(got, "no push: ") || err != nil {
		t.Fatalf("push: %q %v", got, err)
	}
	q := w.item("p-1.2.9")
	if did, err := s.settleQuestion(&q); did != "" || err != nil || w.item("p-1.2.9").Status != work.Open {
		t.Fatalf("live: %q %v", did, err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if did, err := s.settleQuestion(&q); did != "dismissed: "+SessionEnded || err != nil ||
		w.item("p-1.2.9").Resolution != work.Dismissed {
		t.Errorf("ended: %q %v", did, err)
	}
	noSocket := question("s1", sock)
	noSocket.Need.RaisedBy = &work.RaisedBy{Session: "s1"}
	w = newFakeWork(noSocket)
	if did, err := bareServer(w).settleQuestion(&noSocket); did != "" || err != nil {
		t.Errorf("no socket: %q %v", did, err)
	}

	saved := CodexLive
	t.Cleanup(func() { CodexLive = saved })
	CodexLive = func(string, string) bool { return false } // idle past the live window
	home := codexHome(t)
	rollout(t, home)
	codex := question(rootThread, session.CodexInbox(home))
	codex.Comments = nil
	w = newFakeWork(codex)
	if did, err := bareServer(w).settleQuestion(&codex); did != "" || err != nil || w.item("p-1.2.9").Status != work.Open {
		t.Errorf("an idle Codex question with nothing answered: %q %v", did, err)
	}
	rollout(t, home, "Red, thanks")
	if did, err := bareServer(w).settleQuestion(&codex); did != "answered in chat" || err != nil ||
		w.item("p-1.2.9").Resolution != work.Answered {
		t.Errorf("an idle Codex question answered in its chat: %q %v", did, err)
	}
}

// TestWhatTheServiceQueuedIsNoChatAnswer: codex queue's message shows in the rollout as a user message, so a site
// answer the service queued for one need, or a delivery's text, is not taken for the owner's answer to another
// question; what the owner typed is.
func TestWhatTheServiceQueuedIsNoChatAnswer(t *testing.T) {
	home := codexHome(t)
	answered := question(rootThread, session.CodexInbox(home))
	answered.ID = "p-1.2.8"
	open := question(rootThread, session.CodexInbox(home))
	open.Comments = nil
	needs := []work.Item{answered, open}
	for text, relayed := range map[string]bool{"Red or blue?: Blue": true, "pm: owner reply to decision x": true,
		"Blue, thanks": false} {
		if got := Relayed(needs, text); got != relayed {
			t.Errorf("%q: relayed %v", text, got)
		}
	}
	rollout(t, home, "Red or blue?: Blue", "Blue, thanks")
	if got, ok := ChatAnswer(&open, needs); !ok || got != "Blue, thanks" {
		t.Errorf("the answer typed after a queued one: %q %v", got, ok)
	}
}

// TestTheSweepClosesACodexQuestionAnsweredInTheChat: with no site answer, the sweep leaves an open Codex question
// alone until its rollout shows what the owner typed after the call, then closes it with that answer, queueing
// nothing.
func TestTheSweepClosesACodexQuestionAnsweredInTheChat(t *testing.T) {
	home := codexHome(t)
	n := question(rootThread, session.CodexInbox(home))
	n.Comments = nil
	w := newFakeWork(n)
	s := bareServer(w)
	s.swept = map[string]sweepState{}
	s.snap.Store(&snapshot{order: []*work.Item{&n}})
	log := fakeCodex(t, "0.154.0", rootThread)
	rollout(t, home)
	s.sweep()
	if got := w.item(n.ID); got.Status != work.Open {
		t.Fatalf("nothing typed yet: %+v", got)
	}
	rollout(t, home, "Blue, thanks")
	s.sweep()
	got := w.item(n.ID)
	if got.Status != work.Closed || got.Resolution != work.Answered || len(got.Comments) != 1 ||
		got.Comments[0].Text != "Blue, thanks" {
		t.Errorf("answered in the chat: %+v", got)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("codex was run for a question answered in the chat")
	}
}
