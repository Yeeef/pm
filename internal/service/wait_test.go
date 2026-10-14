package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/work"
)

// askedInClaude is an open question need a Claude Code session with no messaging socket raised (left to its hook),
// with no reply yet.
func askedInClaude() work.Item {
	n := question("s1", "unset")
	n.Need.RaisedBy.Inbox, n.Comments = "", nil
	return n
}

// waiting is WaitNeed for askedInClaude's need on main's service, on a goroutine: its error once it returns.
func waiting(ctx context.Context, main string) <-chan error {
	done := make(chan error, 1)
	go func() { done <- WaitNeed(ctx, main, "p-1.2.9") }()
	return done
}

// stillWaiting fails the test when the wait has returned within d.
func stillWaiting(t *testing.T, done <-chan error, d time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("the wait returned before its need moved: %v", err)
	case <-time.After(d):
	}
}

// returned is the wait's error once it returns, failing the test after 5 s.
func returned(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the wait did not return within 5 s")
	}
	return nil
}

// TestAWaitEndsAsSoonAsTheSiteReplyIsStored: with the service's own look an hour apart, the site reply's write wakes
// the wait, well under a second after the POST.
func TestAWaitEndsAsSoonAsTheSiteReplyIsStored(t *testing.T) {
	w := newFakeWork(askedInClaude())
	s := serve(t, w, "", func(*Deps) { ServeCheck = time.Hour }) // quick's cleanup restores it
	tok := s.token()
	done := waiting(context.Background(), s.main)
	stillWaiting(t, done, 300*time.Millisecond)
	start := time.Now()
	form := url.Values{"token": {tok}, "id": {"p-1.2.9"}, "rid": {"r-1"}, item.AnswerField(0): {"Blue"}}
	if code, _, body := s.post(form, ""); code != 303 {
		t.Fatalf("POST: %d %s", code, body)
	}
	if err := returned(t, done); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	t.Logf("site reply to wait answered: %v", took)
	if took > 500*time.Millisecond {
		t.Errorf("the wait answered %v after the POST", took)
	}
	if n := w.item("p-1.2.9"); !ReplyWaiting(&n) {
		t.Errorf("answered before the reply was stored: %+v", n.Comments)
	}
}

// TestAWaitEndsWhenItsNeedIsClosedElsewhere: a close any command makes reaches the wait at the service's next look.
func TestAWaitEndsWhenItsNeedIsClosedElsewhere(t *testing.T) {
	w := newFakeWork(askedInClaude())
	s := serve(t, w, "")
	done := waiting(context.Background(), s.main)
	stillWaiting(t, done, 200*time.Millisecond)
	n := w.item("p-1.2.9")
	n.Status, n.CloseReason = work.Closed, "answered in the session"
	w.set(n)
	if err := returned(t, done); err != nil {
		t.Fatal(err)
	}
}

// TestACallerThatHangsUpEndsItsWait: the caller's cancel ends the call at once, and the service drops the wait.
func TestACallerThatHangsUpEndsItsWait(t *testing.T) {
	w := newFakeWork(askedInClaude())
	s := serve(t, w, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := waiting(ctx, s.main)
	stillWaiting(t, done, 200*time.Millisecond)
	cancel()
	if err := returned(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	eventually(t, "the service drops the wait", func() bool { return strings.Contains(s.log.String(), "wait p-1.2.9 left") })
}

// TestAWaitFailsWhenTheServiceStopsAndAnswersFromTheNextOne: the stop ends the call with an error, so the caller calls
// again; the next service on the same clone answers it.
func TestAWaitFailsWhenTheServiceStopsAndAnswersFromTheNextOne(t *testing.T) {
	main := shortTemp(t)
	fakeBin(t, map[string]string{"gh": fakeGH})
	s := serve(t, newFakeWork(askedInClaude()), main)
	done := waiting(context.Background(), main)
	stillWaiting(t, done, 200*time.Millisecond)
	if err := s.stop(); err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("stop: %v", err)
	}
	if err := returned(t, done); err == nil {
		t.Fatal("the wait ended without an error when the service stopped")
	}
	if err := WaitNeed(context.Background(), main, "p-1.2.9"); err == nil {
		t.Fatal("a wait with no service answered")
	}
	closed := askedInClaude()
	closed.Status = work.Closed
	serve(t, newFakeWork(closed), main)
	if err := returned(t, waiting(context.Background(), main)); err != nil {
		t.Fatal(err)
	}
}
