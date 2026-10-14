package item

import (
	"strings"
	"testing"
)

var shipAsk = AskInfo{Call: "toolu_1", Questions: []AskQuestion{
	{Text: "Ship it\ntoday?", Header: "Ship", Options: []AskOption{{Label: "Yes", Detail: "merge now"}, {Label: "No"}}},
	{Text: "Which checks?", Multi: true, Options: []AskOption{{Label: "lint"}, {Label: "go"}, {Label: "py"}}},
	{Text: "Which port?"}, // free text: no options
}}

// A card's fields make one line per question, "<question text>: <answer>": a pick's label; a multi-select question's
// picks in the options' order, joined ", "; Other replacing a single-select pick and following multi-select picks.
func TestAnswerTextComposesOneLinePerQuestion(t *testing.T) {
	for name, c := range map[string]struct {
		fields map[string][]string
		want   string
	}{
		"picks": {map[string][]string{"q0": {"Yes"}, "q1": {"py", "lint"}, "q2-other": {"8080"}},
			"Ship it today?: Yes\nWhich checks?: lint, py\nWhich port?: 8080"},
		"other replaces a single pick": {map[string][]string{"q0": {"No"}, "q0-other": {"  after the\nrelease "},
			"q1": {"go"}, "q2-other": {"80"}},
			"Ship it today?: after the release\nWhich checks?: go\nWhich port?: 80"},
		"other follows multi picks": {map[string][]string{"q0": {"Yes"}, "q1": {"go", "lint"},
			"q1-other": {"vet"}, "q2-other": {"80"}},
			"Ship it today?: Yes\nWhich checks?: lint, go, vet\nWhich port?: 80"},
		"other alone on multi": {map[string][]string{"q0": {"Yes"}, "q1-other": {"none"}, "q2-other": {"80"}},
			"Ship it today?: Yes\nWhich checks?: none\nWhich port?: 80"},
		"blank other is no answer": {map[string][]string{"q0": {"Yes"}, "q0-other": {"   "}, "q1": {"go"},
			"q2-other": {"80"}}, "Ship it today?: Yes\nWhich checks?: go\nWhich port?: 80"},
	} {
		got, err := AnswerText(shipAsk, c.fields)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", name, got, err, c.want)
		}
	}
}

// An unanswered question, a label that is no option, and two picks on a single-select question are refused, naming the
// question.
func TestAnswerTextRefusesWhatAnswersNothing(t *testing.T) {
	for name, c := range map[string]struct {
		fields map[string][]string
		want   string
	}{
		"nothing":       {nil, `question 1, "Ship it today?", has no answer: pick an option or fill in Other`},
		"one left":      {map[string][]string{"q0": {"Yes"}, "q2-other": {"80"}}, `question 2, "Which checks?", has no answer`},
		"free text":     {map[string][]string{"q0": {"Yes"}, "q1": {"go"}}, `question 3, "Which port?", has no answer`},
		"not an option": {map[string][]string{"q0": {"Maybe"}}, `"Maybe" is not an option of question 1, "Ship it today?"`},
		"two picks":     {map[string][]string{"q0": {"Yes", "No"}}, `question 1, "Ship it today?", takes one option, and 2 were picked`},
	} {
		got, err := AnswerText(shipAsk, c.fields)
		if err == nil || !strings.Contains(err.Error(), c.want) || got != "" {
			t.Errorf("%s: got %q, %v; want the error %q", name, got, err, c.want)
		}
	}
}
