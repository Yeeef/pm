package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// wordBefore is whether s holds a Python word character just before byte i, where \b before a word fails.
func wordBefore(s string, i int) bool {
	if i == 0 {
		return false
	}
	r := []rune(s[:i])
	c := r[len(r)-1]
	return c == '_' || unicode.IsLetter(c) || unicode.IsNumber(c)
}

// anyAtWordStart is whether re matches s at a word boundary.
func anyAtWordStart(re *regexp.Regexp, s string) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		if !wordBefore(s, m[0]) {
			return true
		}
	}
	return false
}

// asksReview is whether text names a PR and asks for its review or merge.
func asksReview(text string) bool {
	named := prLink.MatchString(text) || anyAtWordStart(prNumbered, text)
	return named && anyAtWordStart(reviewAsked, text)
}

// ---------------------------------------------------------------- decision need and action need

// needPart is one part of a decision need, given by its flag: one non-empty line.
func needPart(part, value string) (string, error) {
	value = strip(value)
	if value == "" {
		return "", refuse("%s is empty; %s", part, needShape)
	}
	if len(records.SplitLines(value)) > 1 {
		return "", refuse("%s has more than one line; give each part one line", part)
	}
	return value, nil
}

// pairs is a two-value flag's occurrences, each (label, text).
func pairs(p *Parsed, dest string) [][2]string {
	var out [][2]string
	for _, v := range p.Values[dest] {
		a, b, _ := strings.Cut(v, pairSep)
		out = append(out, [2]string{a, b})
	}
	return out
}

// needText is every part a decision need's flags give, as given, for checks that run before its layout's.
func needText(p *Parsed) string {
	parts := append(append([]string{}, p.Values["question"]...), p.Values["fact"]...)
	for _, dest := range []string{"option", "cost", "default"} {
		for _, pr := range pairs(p, dest) {
			parts = append(parts, pr[0], pr[1])
		}
	}
	return strings.Join(parts, "\n")
}

// needMarkdown is a decision need's description in its one layout, from the parts its flags give (needShape).
func needMarkdown(p *Parsed) (string, error) {
	for _, f := range []struct{ flag, dest string }{{"--question", "question"}, {"--default", "default"}} {
		if len(p.Values[f.dest]) != 1 {
			return "", refuse("give exactly one %s; %s", f.flag, needShape)
		}
	}
	question, err := needPart("--question", p.Values["question"][0])
	if err != nil {
		return "", err
	}
	var facts []string
	for _, f := range p.Values["fact"] {
		v, err := needPart("--fact", f)
		if err != nil {
			return "", err
		}
		facts = append(facts, v)
	}
	var labels []string
	options := map[string]string{}
	for _, o := range pairs(p, "option") {
		label := o[0]
		if !optionLabel.MatchString(label) {
			return "", refuse("the option label %s is not letters and digits only, e.g. a, b or keep", strconv.Quote(label))
		}
		if _, ok := options[label]; ok {
			return "", refuse("two options have the label %s; give each option its own label", label)
		}
		v, err := needPart("--option "+label, o[1])
		if err != nil {
			return "", err
		}
		options[label] = v
		labels = append(labels, label)
	}
	costs, err := needCosts(p, options, labels)
	if err != nil {
		return "", err
	}
	if len(labels) < 2 {
		return "", refuse("give at least two --option flags; a decision needs a choice; %s", needShape)
	}
	def := pairs(p, "default")[0]
	label := def[0]
	if _, ok := options[label]; !ok {
		return "", refuse("--default %s names no option; the labels are %s", label, strings.Join(labels, ", "))
	}
	rest, err := needPart("--default REASON", def[1])
	if err != nil {
		return "", err
	}
	type part struct{ name, text string }
	parts := []part{{"the question", question}}
	for i, f := range facts {
		parts = append(parts, part{fmt.Sprintf("fact %d", i+1), f})
	}
	for _, opt := range labels {
		parts = append(parts, part{"option " + opt, options[opt]}, part{"the cost of option " + opt, costs[opt]})
	}
	parts = append(parts, part{"the default", rest})
	for _, pt := range parts {
		for _, s := range records.Sentences(pt.text) {
			if found := records.Words(s); len(found) > sentenceLimit {
				return "", refuse("the sentence \"%s …\" in %s has %d words; the limit is %d (ASD-STE100); split it",
					strings.Join(found[:6], " "), pt.name, len(found), sentenceLimit)
			}
		}
	}
	return decisionNeedLayout(question, facts, labels, options, costs, label, rest), nil
}

// deliveryHint is how the raising agent hears the answer: the pm service pushes it into the session's inbox when there
// is one (Claude Code's); else pm hook inbox hands it over at the session's next prompt. Either way pm reply wait
// waits for it within the turn.
func deliveryHint(id string) string {
	inbox := sessionInbox()
	if _, codex := session.CodexHomeOf(inbox); codex {
		return fmt.Sprintf("the pm service queues the owner's reply into this Codex session (codex queue), which "+
			"starts a turn with it when idle; to wait for it within this turn, run pm reply wait %s; if the session has "+
			"ended by then, pm show flags it for the next one, which reads it with pm reply read %s", id, id)
	}
	if inbox != "" {
		return fmt.Sprintf("the pm service pushes the owner's reply into this session; to wait for it within this turn, "+
			"run pm reply wait %s; if the session has ended by then, pm show flags it for the next one, which reads it "+
			"with pm reply read %s", id, id)
	}
	return fmt.Sprintf("this session gets no push of the owner's reply (no inbox pm can push to): wait for it within "+
		"this turn with pm reply wait %s (in the background where the runtime allows), else it reaches the session at "+
		"its next prompt, and pm reply read %s prints it", id, id)
}

// raisedBy is the session that raises a need and its inbox, or nil outside a known session. Never the inbox's token:
// the work store syncs to the remote.
func raisedBy() (*work.RaisedBy, error) {
	sid := currentSession()
	if sid == "" {
		return nil, nil
	}
	rb := &work.RaisedBy{Session: sid}
	if inbox := sessionInbox(); inbox != "" {
		host, err := service.Hostname()
		if err != nil {
			return nil, err
		}
		rb.Inbox, rb.Host = inbox, host
	}
	return rb, nil
}

// raiseNeed raises a decision need or an action: a need item of that kind under --parent.
func raiseNeed(e *env, p *Parsed, kind work.NeedKind) (string, error) {
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	desc := ""
	text := title + "\n"
	if kind == work.Action {
		desc = p.Get("text")
		text += desc
	} else {
		text += needText(p)
	}
	if asksReview(text) {
		return "", refuse("this asks the owner to review or merge a PR; raise it with the review form, so its card links "+
			"the PR, the sprints and design pages and its wait wakes on the merge: %s. If it "+
			"only mentions the PR, say what you ask without review, merge or approve", reviewForm)
	}
	if kind == work.Action && desc == "" {
		return "", refuse("the description is empty; pass it with %s: %s", textForms, actionShape)
	}
	if kind == work.Decision {
		var err error
		if desc, err = needMarkdown(p); err != nil {
			return "", err
		}
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	parentID := p.Get("parent")
	parent := r.item(parentID)
	if parent == nil {
		return "", refuse("--parent %s is not in the work store", parentID)
	}
	if parent.Status == work.Closed {
		return "", refuse("--parent %s is closed; raise it under an open sprint or task", parentID)
	}
	inside := false
	for _, a := range r.x.Ancestors(parentID) {
		for _, rec := range r.recs {
			inside = inside || rec.Type() == "project" && rec.Bead() == a
		}
	}
	if !inside {
		return "", refuse("--parent %s is not inside a project with a record, so no page would show it", parentID)
	}
	rb, err := raisedBy()
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Need, Parent: parentID, Title: title, Description: desc,
		Need: &work.NeedInfo{Kind: kind, RaisedBy: rb}})
	if err != nil {
		return "", err
	}
	id := made.ID
	if kind == work.Action {
		return fmt.Sprintf("raised action %s under %s; %s; once you see the owner has done it: pm action done %s "+
			"--reason \"<what you saw>\"", id, parentID, deliveryHint(id), id), nil
	}
	return fmt.Sprintf("raised decision need %s under %s; %s; once the owner answers: pm decision add --need %s "+
		"--level … --decision '<the answer>' --reason '<why>' if it sets a rule, else pm decision close %s --reason "+
		"\"<why it sets no rule>\" --text-file - <<'EOF' (the answer, then EOF)", id, parentID, deliveryHint(id), id, id), nil
}

func cmdDecisionNeed(e *env, p *Parsed) (string, error) { return raiseNeed(e, p, work.Decision) }

func cmdActionNeed(e *env, p *Parsed) (string, error) {
	if _, ok := given(p, "pr"); !ok {
		for _, f := range []struct{ flag, dest string }{{"--sprint", "sprint"}, {"--focus", "focus"}, {"--design", "design"}} {
			if _, ok := given(p, f.dest); ok {
				return "", refuse("%s is only for a PR review; give --pr URL with it, or drop it", f.flag)
			}
		}
		if _, ok := given(p, "parent"); !ok {
			return "", refuse("--parent is required (or --pr for a PR review)")
		}
		if _, ok := given(p, "title"); !ok {
			return "", refuse("--title is required (or --pr for a PR review)")
		}
		return raiseNeed(e, p, work.Action)
	}
	if _, ok := given(p, "parent"); ok {
		return "", refuse("--parent is not allowed with --pr; a PR review goes under the first sprint it delivers")
	}
	for _, f := range []struct{ flag, dest string }{{"--sprint", "sprint"}, {"--focus", "focus"}} {
		if _, ok := given(p, f.dest); !ok {
			return "", refuse("%s is required with --pr", f.flag)
		}
	}
	return raiseReview(e, p)
}

func needCosts(p *Parsed, options map[string]string, labels []string) (map[string]string, error) {
	costs := map[string]string{}
	for _, c := range pairs(p, "cost") {
		label := c[0]
		if _, ok := options[label]; !ok {
			return nil, refuse("--cost %s names no option; the labels are %s", label, strings.Join(labels, ", "))
		}
		if _, ok := costs[label]; ok {
			return nil, refuse("option %s has two --cost flags; give each option one cost", label)
		}
		v, err := needPart("--cost "+label, c[1])
		if err != nil {
			return nil, err
		}
		costs[label] = v
	}
	for _, label := range labels {
		if _, ok := costs[label]; !ok {
			return nil, refuse("option %s has no cost; add --cost %s '<what it costs>'", label, label)
		}
	}
	return costs, nil
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
		more := strip(text[len(first):])
		b.WriteString("- **(" + opt + ") " + first + "** ")
		if more != "" {
			b.WriteString(more + " ")
		}
		b.WriteString("*Cost:* " + costs[opt] + "\n")
	}
	b.WriteString("\n**Default:** (" + label + "). " + rest)
	return b.String()
}
