package site

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
)

// testdata/serve/filled.html is the page below with its reply slots and status filled, frozen, in
// America/New_York: a reply saving, one sent and not delivered, one failed whose text
// and id go back in the box, escaped, and one sent with no known delivery; a question's answer form, and one whose
// answer failed, its text above the form; then the status 13 s behind. -update rewrites it.
func TestGoldenFillRepliesAndStatus(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("no time zone data:", err)
	}
	saved := time.Local
	time.Local = loc
	defer func() { time.Local = saved }()
	page := "<a><!--pm-status--><!--pm-reply x-1 decision--><!--pm-reply x-2 action--><!--pm-reply x&amp;3 action-->" +
		"<!--pm-reply x-4 decision--><!--pm-reply x-5 question--><!--pm-reply x-6 question--></a>"
	replies := map[string]Reply{"x-1": {State: "saving", Text: "Hi *there*"},
		"x-2": {State: "sent", Text: "Ok.", RID: "r2", Delivery: "not running"},
		"x&3": {State: "failed", Text: `a "b" 'c' <d>`, RID: "r-3", Error: "it's <bad>"},
		"x-4": {State: "sent", Text: "x"},
		"x-6": {State: "failed", Text: "Ship it?: No\nWhich checks?: lint, <go>", RID: "r-6", Error: "store down"}}
	asOf := time.Unix(1760000000, 123456000)
	got := FillStatus(FillReplies(page, "tok'en", replies), asOf, "abcd", asOf.Add(12600*time.Millisecond))
	if *update {
		if err := os.WriteFile("testdata/serve/filled.html", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/serve/filled.html")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("differs from testdata/serve/filled.html:\n%s\n%s", got, want)
	}
}

// TestAReplyToASessionWithNoInboxWaitsForIt: a session that stores no inbox gets no push, so a reply to its need is
// not "not running": the card after the reply and the need's thread say it waits for that session, which pm show flags
// it to and pm reply read prints it for; a need with an inbox, a Claude Code socket or a Codex route, keeps the not-delivered text.
// With codex missing from the service's PATH, the card says so and how to fix it.
func TestAReplyToASessionWithNoInboxWaitsForIt(t *testing.T) {
	card := FillReplies("<!--pm-reply x-1 decision--><!--pm-reply x-2 decision-->", "tok", map[string]Reply{
		"x-1": {State: "sent", Text: "Ok.", RID: "r1", Delivery: "no push"},
		"x-2": {State: "sent", Text: "Ok.", RID: "r2", Delivery: "no codex"}})
	if !strings.Contains(card, "waiting for the session that asked, which has no inbox pm can push to: pm show "+
		"flags the reply for it, and pm reply read prints it") || strings.Contains(card, "not running") ||
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

// A question's slot becomes the form its card's inputs post: named reply-<id>, as their form attribute names it, with
// the token, the id and a reply id its script makes from the answer, and no text box of its own.
func TestAQuestionsSlotBecomesTheFormItsInputsPost(t *testing.T) {
	got := FillReplies("<!--pm-reply x-1 question-->", "tok", nil)
	for _, want := range []string{`<form class="reply" id="reply-x-1" method="post" action="/reply"`,
		`<input type="hidden" name="token" value="tok">`, `<input type="hidden" name="id" value="x-1">`,
		`<input type="hidden" name="rid" value="">`, `filter(([k]) => /^q\d/.test(k))`, "Send answer</button></form>"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in %s", want, got)
		}
	}
	if strings.Contains(got, "<textarea") {
		t.Errorf("a question's form has a text box: %s", got)
	}
}

// The service serves the docs index that every page's navigation links.
func TestTheServiceServesTheDocsIndex(t *testing.T) {
	recs := linkRecs("docs/2026-10-15-note-cfr")
	s, err := Serve(recs, records.NewItems(nil), "demo", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, found, err := s.Page(DocsIndex)
	if err != nil || !found {
		t.Fatalf("docs index: found %v, err %v", found, err)
	}
	if !strings.Contains(p, `<a href="../docs/2026-10-15-note-cfr.html">T</a><span class="k">2026-10-15</span>`) {
		t.Errorf("the docs index does not list the doc:\n%s", p)
	}
	if !strings.Contains(p, `<a class="docs" href="../docs/index.html">Docs</a>`) {
		t.Errorf("the docs index's navigation does not link it:\n%s", p)
	}
}
