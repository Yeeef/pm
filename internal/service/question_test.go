package service

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
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

func bareServer(w *fakeWork) *server { return &server{d: Deps{Open: w.Open, Log: io.Discard}} }

// liveCodex makes every Codex session live for the test.
func liveCodex(t *testing.T, live bool) {
	t.Helper()
	saved := CodexLive
	t.Cleanup(func() { CodexLive = saved })
	CodexLive = func(string, string) bool { return live }
}

// queuedMessages is the --message arguments codex queue got, from the fake codex's log; none when it never ran.
func queuedMessages(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if l == "--message" && i+1 < len(lines) {
			out = append(out, lines[i+1])
		}
	}
	return out
}

// TestTheServiceClosesACodexQuestionBeforeItQueuesTheSitesAnswer: the site's answer closes the question as answered
// on the site, then alone is queued into the live session and marked delivered. A question closed first (the owner
// answered in the chat, which pm hook ask closed at UserPromptSubmit) queues nothing. A session the answer cannot reach
// keeps its question open, undelivered.
func TestTheServiceClosesACodexQuestionBeforeItQueuesTheSitesAnswer(t *testing.T) {
	liveCodex(t, true)
	home := codexHome(t)
	w := newFakeWork(question(rootThread, session.CodexInbox(home)))
	log := fakeCodex(t, "0.154.0", rootThread)
	if got, err := bareServer(w).pushUndelivered("p-1.2.9"); got != "delivered" || err != nil {
		t.Fatalf("queued: %q %v", got, err)
	}
	n := w.item("p-1.2.9")
	if q := queuedMessages(t, log); !slices.Equal(q, []string{"Red or blue?: Blue"}) || n.Status != work.Closed ||
		n.Resolution != work.Answered || n.CloseReason != AnsweredOnSite || n.Need.Delivered != 1 {
		t.Fatalf("queued %q; need %+v", q, n)
	}

	stale := question(rootThread, session.CodexInbox(home)) // read open, then answered in the chat
	w = newFakeWork(stale)
	st, err := w.Open()
	if err == nil {
		err = AnswerInChat(st, &stale, "Red, thanks", rootThread)
	}
	if err != nil {
		t.Fatal(err)
	}
	log = fakeCodex(t, "0.154.0", rootThread)
	if got, err := bareServer(w).deliverQuestion(&stale); got != "closed already: answered in the session first" ||
		err != nil {
		t.Fatalf("chat first: %q %v", got, err)
	}
	if q := queuedMessages(t, log); q != nil {
		t.Errorf("codex queued %q for a question answered in the chat", q)
	}
	if got, err := bareServer(w).pushUndelivered("p-1.2.9"); got != "nothing to deliver" || err != nil {
		t.Errorf("answered in the chat, pushed again: %q %v", got, err)
	}

	liveCodex(t, false)
	w = newFakeWork(question(rootThread, session.CodexInbox(home)))
	if got, err := bareServer(w).pushUndelivered("p-1.2.9"); !strings.HasPrefix(got, "not running") || err != nil ||
		w.item("p-1.2.9").Status != work.Open {
		t.Errorf("a session not live: %q %v, need %+v", got, err, w.item("p-1.2.9"))
	}
}

// TestASiteAnswerWhoseQueueFailedIsQueuedAgainByTheSweep: codex queue failing once the question closed leaves the
// answer undelivered (Unqueued), even with the reply stamped in the close's second; the next sweep queues it and marks
// it delivered, and the one after queues nothing more.
func TestASiteAnswerWhoseQueueFailedIsQueuedAgainByTheSweep(t *testing.T) {
	liveCodex(t, true)
	home := codexHome(t)
	w := newFakeWork(question(rootThread, session.CodexInbox(home)))
	s := bareServer(w)
	s.swept = map[string]sweepState{}
	fakeCodex(t, "0.154.0", "another-thread") // says it queued for another thread: a failure
	if got, err := s.pushUndelivered("p-1.2.9"); !strings.HasPrefix(got, "failed: ") || err != nil {
		t.Fatalf("failed queue: %q %v", got, err)
	}
	n := w.item("p-1.2.9")
	n.ClosedAt = n.Comments[0].CreatedAt // the reply in the close's second
	w = newFakeWork(n)
	s = bareServer(w)
	s.swept = map[string]sweepState{}
	if n.CloseReason != AnsweredOnSite || n.Need.Delivered != 0 || !Unqueued(&n) {
		t.Fatalf("closed, not delivered: %+v", n)
	}
	log := fakeCodex(t, "0.154.0", rootThread)
	s.snap.Store(&snapshot{order: []*work.Item{&n}})
	s.sweep()
	got := w.item("p-1.2.9")
	if q := queuedMessages(t, log); !slices.Equal(q, []string{"Red or blue?: Blue"}) || got.Need.Delivered != 1 ||
		Unqueued(&got) {
		t.Fatalf("the sweep queued %q; need %+v", q, got)
	}
	s.snap.Store(&snapshot{order: []*work.Item{&got}})
	s.sweep()
	if q := queuedMessages(t, log); len(q) != 1 {
		t.Errorf("queued again once delivered: %q", q)
	}
}

// TestAClaudeQuestionIsNeverPushedAndEndsWithItsSession: its hook takes the answer, so the push leaves its inbox alone;
// the sweep dismisses it once its messaging socket is gone, and leaves one with no socket to its hook. A Codex
// question is never dismissed for idleness: with its session idle and nothing answered, it stays open.
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

	liveCodex(t, false) // idle past the live window
	codex := question(rootThread, session.CodexInbox(codexHome(t)))
	codex.Comments = nil
	w = newFakeWork(codex)
	if did, err := bareServer(w).settleQuestion(&codex); did != "" || err != nil || w.item("p-1.2.9").Status != work.Open {
		t.Errorf("an idle Codex question with nothing answered: %q %v", did, err)
	}
}

// TestWhatTheServiceQueuedIsNoChatAnswer: codex queue's message may come back as a prompt, so the site answer of a
// question the service closed as answered on the site (its reply stamped in the close's second included), or a
// delivery's text, is not taken for the owner's answer to another question; what the owner typed is, even when it
// repeats an earlier chat answer.
func TestWhatTheServiceQueuedIsNoChatAnswer(t *testing.T) {
	home := codexHome(t)
	onSite := question(rootThread, session.CodexInbox(home))
	onSite.ID, onSite.Status, onSite.CloseReason, onSite.ClosedAt = "p-1.2.8", work.Closed, AnsweredOnSite, at
	inChat := question(rootThread, session.CodexInbox(home))
	inChat.ID, inChat.Status, inChat.CloseReason, inChat.ClosedAt = "p-1.2.7", work.Closed, "Responded", at
	inChat.Comments = []work.Comment{{Kind: work.Reply, Author: "owner", Text: "Red", CreatedAt: at}}
	open := question(rootThread, session.CodexInbox(home))
	open.Comments = nil
	needs := []work.Item{onSite, inChat, open}
	for text, relayed := range map[string]bool{"Red or blue?: Blue": true, "pm: owner reply to decision x": true,
		"Blue, thanks": false, "Red": false} {
		if got := Relayed(needs, text); got != relayed {
			t.Errorf("%q: relayed %v", text, got)
		}
	}
}
