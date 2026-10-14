package service

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/work"
)

// A question's card posts a field per question; the service stores the reply they make, one "<question>: <answer>"
// line each, and refuses a post that leaves a question unanswered, storing and spooling nothing.
func TestAQuestionsAnswerIsComposedFromItsFieldsAndAnUnansweredOneIsRefused(t *testing.T) {
	q := need("p-1.2.1", work.Question, "")
	q.Need.RaisedBy = &work.RaisedBy{Session: "s1"}
	q.Need.Ask = &work.AskInfo{Call: "toolu_1", Questions: []work.AskQuestion{
		{Text: "Ship it?", Options: []work.AskOption{{Label: "Yes"}, {Label: "No"}}},
		{Text: "Which checks?", Multi: true, Options: []work.AskOption{{Label: "lint"}, {Label: "go"}}}}}
	w := newFakeWork(q)
	s := serve(t, w, "")
	unanswered := url.Values{"token": {s.token()}, "id": {"p-1.2.1"}, "rid": {"r-0"}, "q0": {"Yes"},
		"text": {"a text is not an answer"}}
	if code, _, body := s.post(unanswered, ""); code != 400 ||
		body != "refused: question 2, \"Which checks?\", has no answer: pick an option or fill in Other\n" {
		t.Fatalf("unanswered: %d %q", code, body)
	}
	data, err := os.ReadFile(s.spool)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if len(data) != 0 || len(w.item("p-1.2.1").Comments) != 0 {
		t.Fatalf("a refused answer was kept: spool %q, comments %+v", data, w.item("p-1.2.1").Comments)
	}
	form := url.Values{"token": {s.token()}, "id": {"p-1.2.1"}, "rid": {"r-1"}, "q0": {"No"},
		"q0-other": {"after the release"}, "q1": {"go", "lint"}, "q1-other": {"vet"}}
	if code, loc, body := s.post(form, ""); code != 303 || !strings.HasSuffix(loc, "#need-p-1.2.1") {
		t.Fatalf("answered: %d %q %q", code, loc, body)
	}
	eventually(t, "stored", func() bool { return len(w.item("p-1.2.1").Comments) == 1 })
	if got := w.item("p-1.2.1").Comments[0].Text; got !=
		"Ship it?: after the release\nWhich checks?: lint, go, vet\n\n<!-- pm-reply r-1 -->" {
		t.Fatalf("stored %q", got)
	}
}
