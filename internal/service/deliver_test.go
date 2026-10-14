package service

import (
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/work"
)

// A reply to a session's question is relayed as an answer to go on with: its message names a question and no pm
// decision or pm action command, since a question records nothing.
func TestAReplyToAQuestionTellsTheSessionToGoOnWithIt(t *testing.T) {
	n := work.Item{ID: "p-1.2.3", Type: work.Need, Title: "Session s1 asks: Ship it?", Status: work.Open,
		Need: &work.NeedInfo{Kind: work.Question, RaisedBy: &work.RaisedBy{Session: "s1"},
			Ask: &work.AskInfo{Call: "toolu_1", Questions: []work.AskQuestion{{Text: "Ship it?"}}}},
		Comments: []work.Comment{{ID: "c1", Kind: work.Reply, Author: "owner", Text: "Yes.", CreatedAt: at}}}
	text, mark := (Texts{}).Undelivered(&n)
	if !strings.HasPrefix(text, "pm: owner reply to question p-1.2.3 (Session s1 asks: Ship it?), relayed from the site:\n") ||
		!strings.Contains(text, "Yes.") || !strings.Contains(text, "go on with it") ||
		strings.Contains(text, "pm decision") || strings.Contains(text, "pm action") || mark == nil || *mark.Delivered != 1 {
		t.Fatalf("%q %+v", text, mark)
	}
	for kind, want := range map[work.NeedKind]string{work.Decision: "decision", work.Action: "action",
		work.Review: "action", work.Question: "question"} {
		if got := kindWord(&work.Item{Need: &work.NeedInfo{Kind: kind}}); got != want {
			t.Errorf("%s: %s", kind, got)
		}
	}
}
