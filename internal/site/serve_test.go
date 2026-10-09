package site

import (
	"os"
	"testing"
	"time"
)

// testdata/serve/filled.html is Python pm's fill_replies then fill_status of the same page, replies and times
// (site.py), made once with TZ=America/New_York: a reply saving, one sent and not delivered, one failed whose text
// and id go back in the box, escaped, and one sent with no known delivery; then the status 13 s behind.
func TestFillRepliesAndStatusEqualPythons(t *testing.T) {
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
		t.Fatalf("differs from Python pm's:\n%s\n%s", got, want)
	}
}
