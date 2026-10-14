package item

import (
	"fmt"
	"slices"
	"strings"
)

// A question need's ask, as pm reads and writes it: the title and description pm gives the need, the rules an ask
// holds, and the equality that keeps it written once. Package work raises, stores and checks it with these.

// QuestionTitleCut is the most characters of the first question a question need's title carries.
const QuestionTitleCut = 120

// QuestionTitle is a question need's title: "Session <the first 8 characters of the session id> asks: <the first
// question>", the question on one line and cut at QuestionTitleCut characters.
func QuestionTitle(session string, ask AskInfo) string {
	session = ShortSession(session)
	first := ""
	if len(ask.Questions) > 0 {
		first = oneLine(ask.Questions[0].Text)
	}
	if r := []rune(first); len(r) > QuestionTitleCut {
		first = string(r[:QuestionTitleCut-1]) + "…"
	}
	return "Session " + session + " asks: " + first
}

// ShortSession is the first 8 characters of a session id, as a question names its session to the owner.
func ShortSession(session string) string {
	if r := []rune(session); len(r) > 8 {
		return string(r[:8])
	}
	return session
}

// QuestionText is a question need's description, in Markdown: each question with its header and its options as a
// list, so the owner reads the ask whole wherever the need shows.
func QuestionText(ask AskInfo) string {
	var desc []string
	for _, q := range ask.Questions {
		text := strings.TrimSpace(q.Text)
		if q.Header != "" {
			text = "**" + q.Header + "**: " + text
		}
		if q.Multi {
			text += " (choose one or more)"
		}
		lines := []string{text}
		for _, o := range q.Options {
			opt := "- " + o.Label
			if o.Detail != "" {
				opt += ": " + o.Detail
			}
			lines = append(lines, opt)
		}
		desc = append(desc, strings.Join(lines, "\n"))
	}
	return strings.Join(desc, "\n\n")
}

// AskFault is what breaks the ask rules in a need's fields, "" when nothing does: an ask exactly on a question, which a
// session raised, whose call has an id, with one or more questions, each with a text and options with a label each,
// no label twice.
func AskFault(n *NeedInfo) string {
	switch a := n.Ask; {
	case (n.Kind == Question) != (a != nil) && a == nil:
		return "is a question need without its ask"
	case (n.Kind == Question) != (a != nil):
		return fmt.Sprintf("is a %s need with an ask", n.Kind)
	case a == nil:
		return ""
	case n.RaisedBy == nil:
		return "is a question no session raised"
	case a.Call == "":
		return "is a question without its call key"
	case len(a.Questions) == 0:
		return "is a question that asks nothing"
	}
	for i, q := range n.Ask.Questions {
		if q.Text == "" {
			return fmt.Sprintf("has question %d without a text", i)
		}
		for j, o := range q.Options {
			if o.Label == "" || slices.ContainsFunc(q.Options[:j], func(p AskOption) bool { return p.Label == o.Label }) {
				return fmt.Sprintf("has question %d with an empty or repeated option %q", i, o.Label)
			}
		}
	}
	return ""
}

// SameAsk is whether two asks hold the same call, questions and options.
func SameAsk(a, b *AskInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Call == b.Call && slices.EqualFunc(a.Questions, b.Questions, func(x, y AskQuestion) bool {
		return x.Text == y.Text && x.Header == y.Header && x.Multi == y.Multi && slices.Equal(x.Options, y.Options)
	})
}
