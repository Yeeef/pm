// Package needform is a decision need's form: its parts as pm decision need's and pm need edit's flags give them, the
// rules each part keeps, and the description's one layout. It reports every rule broken, so the command refuses once
// with all of them rather than at the first, which cost a retry per rule.
package needform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/cli/parse"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
)

const (
	// Shape is the form, with an example: what a refusal about the form's layout ends with.
	Shape = "a decision need takes its parts as flags, one line each: one --question, one or more --fact, two or " +
		"more --option LABEL TEXT, one --cost LABEL TEXT for each option, and one --default LABEL REASON naming " +
		"the option taken if the owner does not answer. Put each value in single quotes, so code spans stay, e.g.:\n" +
		"  pm decision need --title 'Where the site URL lives' --parent ID \\\n" +
		"    --question 'Where does pm keep the public site URL?' \\\n" +
		"    --fact 'Today, each clone keeps the URL in its git config.' \\\n" +
		"    --fact 'You asked why the URL is not in `.pm/config.toml`.' \\\n" +
		"    --option a 'In `.pm/config.toml`. A pm command still writes it.' --cost a 'the repo has one URL.' \\\n" +
		"    --option b 'In each clone, as today.' --cost b 'you must give the URL to each new clone.' \\\n" +
		"    --default a 'All clones then give the same link.'"
	SentenceLimit = 25 // ASD-STE100: the most words in a descriptive sentence
)

var optionLabel = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// Violations collects every rule a need's form breaks, the command's own rules beside the form's.
type Violations struct {
	List  []string
	Shape bool // a violation asks for the form's layout (Shape), which the refusal then gives once
}

// Add records a violation in the rule's own wording.
func (v *Violations) Add(format string, a ...any) { v.List = append(v.List, fmt.Sprintf(format, a...)) }

// AddShape records a violation that ends with the form's layout.
func (v *Violations) AddShape(format string, a ...any) {
	v.Add(format, a...)
	v.Shape = true
}

// Message is the one refusal, "" with no violation: a single violation as its rule words it, several as a list, the
// layout given once at the end.
func (v *Violations) Message() string {
	tail := ""
	if v.Shape {
		tail = Shape
	}
	switch len(v.List) {
	case 0:
		return ""
	case 1:
		if tail != "" {
			return v.List[0] + "; " + tail
		}
		return v.List[0]
	}
	msg := fmt.Sprintf("this breaks %d rules; fix them all, then run it again:\n- %s", len(v.List),
		strings.Join(v.List, "\n- "))
	if tail != "" {
		msg += "\n" + tail
	}
	return msg
}

// needPart is one part of a decision need, given by its flag: one non-empty line; a broken one goes to v.
func needPart(v *Violations, part, value string) string {
	value = config.PyStrip(value)
	if value == "" {
		v.AddShape("%s is empty", part)
	} else if len(records.SplitLines(value)) > 1 {
		v.Add("%s has more than one line; give each part one line", part)
	}
	return value
}

// pairs is a two-value flag's occurrences, each (label, text).
func pairs(values map[string][]string, dest string) [][2]string {
	var out [][2]string
	for _, v := range values[dest] {
		a, b, _ := strings.Cut(v, parse.PairSep)
		out = append(out, [2]string{a, b})
	}
	return out
}

// Text is every part a decision need's flags give, as given, for checks that run before its layout's.
func Text(values map[string][]string) string {
	parts := append(append([]string{}, values["question"]...), values["fact"]...)
	for _, dest := range []string{"option", "cost", "default"} {
		for _, pr := range pairs(values, dest) {
			parts = append(parts, pr[0], pr[1])
		}
	}
	return strings.Join(parts, "\n")
}

// Markdown is a decision need's description in its one layout, from the parts its flags give (Shape); each
// rule a part breaks goes to v, and the layout is empty while v holds any.
func Markdown(values map[string][]string, v *Violations) string {
	before := len(v.List)
	for _, f := range []struct{ flag, dest string }{{"--question", "question"}, {"--default", "default"}} {
		if len(values[f.dest]) != 1 {
			v.AddShape("give exactly one %s", f.flag)
		}
	}
	type part struct{ name, text string }
	var parts []part
	question := ""
	if len(values["question"]) == 1 {
		question = needPart(v, "--question", values["question"][0])
		parts = append(parts, part{"the question", question})
	}
	var facts []string
	for i, f := range values["fact"] {
		facts = append(facts, needPart(v, "--fact", f))
		parts = append(parts, part{fmt.Sprintf("fact %d", i+1), facts[i]})
	}
	var labels []string
	options := map[string]string{}
	for _, o := range pairs(values, "option") {
		label := o[0]
		if !optionLabel.MatchString(label) {
			v.Add("the option label %s is not letters and digits only, e.g. a, b or keep", strconv.Quote(label))
			continue
		}
		if _, ok := options[label]; ok {
			v.Add("two options have the label %s; give each option its own label", label)
			continue
		}
		options[label] = needPart(v, "--option "+label, o[1])
		labels = append(labels, label)
	}
	costs := needCosts(values, v, options, labels)
	if len(labels) < 2 {
		v.AddShape("give at least two --option flags; a decision needs a choice")
	}
	for _, opt := range labels {
		parts = append(parts, part{"option " + opt, options[opt]}, part{"the cost of option " + opt, costs[opt]})
	}
	label, rest := "", ""
	if defs := pairs(values, "default"); len(defs) == 1 {
		label = defs[0][0]
		if _, ok := options[label]; !ok {
			v.Add("--default %s names no option; the labels are %s", label, strings.Join(labels, ", "))
		}
		rest = needPart(v, "--default REASON", defs[0][1])
		parts = append(parts, part{"the default", rest})
	}
	for _, pt := range parts {
		for _, s := range records.Sentences(pt.text) {
			if found := records.Words(s); len(found) > SentenceLimit {
				v.Add("the sentence \"%s …\" in %s has %d words; the limit is %d (ASD-STE100); split it",
					strings.Join(found[:6], " "), pt.name, len(found), SentenceLimit)
			}
		}
	}
	if len(v.List) > before {
		return ""
	}
	return decisionNeedLayout(question, facts, labels, options, costs, label, rest)
}

// needCosts is each option's cost; each rule a --cost flag breaks, and each option left without one, goes to v.
func needCosts(values map[string][]string, v *Violations, options map[string]string, labels []string) map[string]string {
	costs := map[string]string{}
	for _, c := range pairs(values, "cost") {
		label := c[0]
		if _, ok := options[label]; !ok {
			v.Add("--cost %s names no option; the labels are %s", label, strings.Join(labels, ", "))
			continue
		}
		if _, ok := costs[label]; ok {
			v.Add("option %s has two --cost flags; give each option one cost", label)
			continue
		}
		costs[label] = needPart(v, "--cost "+label, c[1])
	}
	for _, label := range labels {
		if _, ok := costs[label]; !ok {
			v.Add("option %s has no cost; add --cost %s '<what it costs>'", label, label)
		}
	}
	return costs
}

func decisionNeedLayout(question string, facts, labels []string, options, costs map[string]string, label, rest string) string {
	var b strings.Builder
	b.WriteString("**Question:** " + question + "\n\n**Facts:**\n\n")
	for _, f := range facts {
		b.WriteString("- " + records.PlainItem(f) + "\n")
	}
	b.WriteString("\n**Options:**\n\n")
	for _, opt := range labels {
		text := options[opt]
		first := records.Sentences(text)[0]
		more := config.PyStrip(text[len(first):])
		b.WriteString("- **(" + opt + ") " + first + "** ")
		if more != "" {
			b.WriteString(more + " ")
		}
		b.WriteString("*Cost:* " + costs[opt] + "\n")
	}
	b.WriteString("\n**Default:** (" + label + "). " + rest)
	return b.String()
}
