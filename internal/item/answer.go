package item

import (
	"fmt"
	"slices"
	"strings"
)

// A question need's answer from the site: the fields of its card, named here so the card and the pm service that reads
// them agree, and the reply text they make, which the hook that relays it to the session parses.

// AnswerField names a question's picked options on its card: a radio, or a checkbox per option when the question is
// multi-select; each posts its option's label. q counts from 0 in the order asked.
func AnswerField(q int) string { return fmt.Sprintf("q%d", q) }

// OtherField names a question's "Other" text on its card: the owner's own answer.
func OtherField(q int) string { return fmt.Sprintf("q%d-other", q) }

// AnswerText is the reply a question card's fields make: one line per question, in the order asked,
// "<question text>: <answer>". A single-select question's answer is the label it picked; a multi-select one's is the
// labels it picked, in the options' order, joined ", ". "Other", when filled, counts as one more choice: it replaces a
// single-select question's pick and follows a multi-select one's picks. The question text and the Other text are put
// on one line. It refuses a question with no answer, a label that is not one of its options, and two picks on a
// single-select question; the error names the question.
func AnswerText(ask AskInfo, fields map[string][]string) (string, error) {
	lines := make([]string, 0, len(ask.Questions))
	for i, q := range ask.Questions {
		text := oneLine(q.Text)
		picked := fields[AnswerField(i)]
		for _, p := range picked {
			if !slices.ContainsFunc(q.Options, func(o AskOption) bool { return o.Label == p }) {
				return "", fmt.Errorf("%q is not an option of question %d, %q", p, i+1, text)
			}
		}
		if !q.Multi && len(picked) > 1 {
			return "", fmt.Errorf("question %d, %q, takes one option, and %d were picked", i+1, text, len(picked))
		}
		var answer []string
		for _, o := range q.Options {
			if slices.Contains(picked, o.Label) {
				answer = append(answer, o.Label)
			}
		}
		if other := oneLine(strings.Join(fields[OtherField(i)], " ")); other != "" {
			if !q.Multi {
				answer = nil
			}
			answer = append(answer, other)
		}
		if len(answer) == 0 {
			return "", fmt.Errorf("question %d, %q, has no answer: pick an option or fill in Other", i+1, text)
		}
		lines = append(lines, text+": "+strings.Join(answer, ", "))
	}
	return strings.Join(lines, "\n"), nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
