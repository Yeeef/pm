package site

import (
	"os"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/work"
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

// A moved sprint's old page path serves its page now (the work-store page, Moving a sprint): demo.1's move note says
// it was sprint 4 of project old, so sprints/old-4.html is sprints/demo-1.html; a path no move names is no page.
func TestAMovedSprintsOldPageIsItsPage(t *testing.T) {
	dir := constructsStore(t, nil)
	recs, err := records.Read(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	items := constructsItems(t)
	s1 := items.Get("demo.1")
	s1.Comments = append(s1.Comments, work.Comment{ID: "m", Kind: work.Note, Author: work.MoveAuthor,
		Text: "pm sprint move: from old sprint 4 to demo sprint 1\nwhy\nbecause", CreatedAt: s1.CreatedAt})
	served, err := Serve(recs, items, "demo", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now, found, err := served.Page("sprints/demo-1.html")
	if err != nil || !found {
		t.Fatal(found, err)
	}
	if old, found, err := served.Page("sprints/old-4.html"); err != nil || !found || old != now {
		t.Errorf("sprints/old-4.html: found %v, err %v, the same page %v", found, err, old == now)
	}
	if _, found, _ := served.Page("sprints/old-5.html"); found {
		t.Error("sprints/old-5.html, which no move names, is a page")
	}
}
