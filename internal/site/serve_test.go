package site

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/item"
)

// testdata/serve/filled.html is the page below with its reply slots and status filled, frozen, in
// America/New_York: a reply saving, one sent and not delivered, one failed whose text
// and id go back in the box, escaped, and one sent with no known delivery; then the status 13 s behind.
func TestFillRepliesAndStatus(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("no time zone data:", err)
	}
	saved := time.Local
	time.Local = loc
	defer func() { time.Local = saved }()
	page := "<a><!--pm-status--><!--pm-reply x-1 decision--><!--pm-reply x-2 action--><!--pm-reply x&amp;3 action-->" +
		"<!--pm-reply x-4 decision--></a>"
	replies := map[string]Reply{"x-1": {State: "saving", Text: "Hi *there*"},
		"x-2": {State: "sent", Text: "Ok.", RID: "r2", Delivery: "not running"},
		"x&3": {State: "failed", Text: `a "b" 'c' <d>`, RID: "r-3", Error: "it's <bad>"},
		"x-4": {State: "sent", Text: "x"}}
	asOf := time.Unix(1760000000, 123456000)
	got := FillStatus(FillReplies(page, "tok'en", replies), asOf, "abcd", asOf.Add(12600*time.Millisecond))
	want, err := os.ReadFile("testdata/serve/filled.html")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("differs from testdata/serve/filled.html:\n%s\n%s", got, want)
	}
}

// TestAReplyToASessionWithNoInboxWaitsForIt: a session that stores no inbox gets no push, so a reply to its need is
// not "not running": the card after the reply and the need's thread say it waits for that session, at its next prompt
// or through pm reply wait; a need with an inbox, a Claude Code socket or a Codex route, keeps the not-delivered text.
// With codex missing from the service's PATH, the card says so and how to fix it.
func TestAReplyToASessionWithNoInboxWaitsForIt(t *testing.T) {
	card := FillReplies("<!--pm-reply x-1 decision--><!--pm-reply x-2 decision-->", "tok", map[string]Reply{
		"x-1": {State: "sent", Text: "Ok.", RID: "r1", Delivery: "no push"},
		"x-2": {State: "sent", Text: "Ok.", RID: "r2", Delivery: "no codex"}})
	if !strings.Contains(card, "waiting for the session that asked, which has no inbox pm can push to: it sees "+
		"the reply at its next prompt, or now if it waits with pm reply wait") || strings.Contains(card, "not running") ||
		!strings.Contains(card, "codex is not on the pm service's PATH") {
		t.Errorf("card: %s", card)
	}
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	for inbox, want := range map[string]string{"": NotPushed, "/run/claude.sock": NotDelivered,
		"codex-queue:/home/u/.codex": NotDelivered} {
		it := &Item{ID: "x-1", Type: item.Need, Status: item.Open, Need: &item.NeedInfo{Kind: item.Decision,
			RaisedBy: &item.RaisedBy{Session: "cx", Inbox: inbox}},
			Comments: []item.Comment{{ID: "c1", Kind: item.Reply, Author: "owner", Text: "Ok.", CreatedAt: at}}}
		if got := thread(it); !strings.Contains(got, `<span class="replied">`+want+"</span>") {
			t.Errorf("inbox %q: %s", inbox, got)
		}
	}
}
