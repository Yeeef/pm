package cli

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The commands that carry the owner's needs and replies: decisions recorded or asked for, actions and PR reviews
// raised and closed, and replies read. Python source: cmd_decision_add, cmd_decision_need, cmd_decision_close,
// cmd_action_need, cmd_action_done, cmd_reply_read and their helpers in cli.py. Each refuses before any write. A site
// reply only adds a reply comment; a need stays open until a session records it with one of these commands, which close
// it with its resolution (the work-store page, Commands).

const (
	levelRule = "project if a later sprint must follow it; sprint if it is about this sprint's own work; " +
		"skip choices cheap to reverse"
	needShape = "a decision need takes its parts as flags, one line each: one --question, one or more --fact, two or " +
		"more --option LABEL TEXT, one --cost LABEL TEXT for each option, and one --default LABEL REASON naming " +
		"the option taken if the owner does not answer. Put each value in single quotes, so code spans stay, e.g.:\n" +
		"  pm decision need --title 'Where the site URL lives' --parent ID \\\n" +
		"    --question 'Where does pm keep the public site URL?' \\\n" +
		"    --fact 'Today, each clone keeps the URL in its git config.' \\\n" +
		"    --fact 'You asked why the URL is not in `.pm/config.toml`.' \\\n" +
		"    --option a 'In `.pm/config.toml`. A pm command still writes it.' --cost a 'the repo has one URL.' \\\n" +
		"    --option b 'In each clone, as today.' --cost b 'you must give the URL to each new clone.' \\\n" +
		"    --default a 'All clones then give the same link.'"
	sentenceLimit = 25 // ASD-STE100: the most words in a descriptive sentence
	actionShape   = "an action's description says what the owner should do and why, e.g.:\n" +
		"  Restart the site on port 8767: the new proxy expects it there."
	reviewForm = `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`
)

var (
	// A request names a PR by its GitHub link or "PR #<n>"; it asks for a review or merge when it also says review,
	// merge or approve (Python's PR_NAMED and REVIEW_ASKED; their \b is checked by wordBefore).
	prLink      = regexp.MustCompile(`(?i)https://github\.com/[\p{L}\p{N}_.-]+/[\p{L}\p{N}_.-]+/pull/\p{Nd}+`)
	prNumbered  = regexp.MustCompile(`(?i)PR` + records.PyS + `*#\p{Nd}+`)
	reviewAsked = regexp.MustCompile(`(?i)(review|merg|approv)`)
	optionLabel = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	urlRE       = regexp.MustCompile(`^https?://[^\t\n\v\f\r \x1c-\x1f\x85\p{Z}]+$`)
	pullNumber  = regexp.MustCompile(`/pull/(\p{Nd}+)`)
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

// ---------------------------------------------------------------- sentences and words

const tick = '\x60' // the backtick that opens and closes a code span

// codeSpanAt is the end of the code span Python's pattern for one (a backtick run, at least one character, the same
// run) matches at rune i of t, or -1: the longest backtick run first, then the nearest closing run, with no line break
// inside.
func codeSpanAt(t []rune, i int) int {
	k := 0
	for i+k < len(t) && t[i+k] == tick {
		k++
	}
	for g := k; g >= 1; g-- {
		for j := i + g + 1; j+g <= len(t); j++ {
			if t[j-1] == '\n' {
				break
			}
			if string(t[j:j+g]) == strings.Repeat(string(tick), g) {
				return j + g
			}
		}
	}
	return -1
}

// sentences splits text at . ! or ? followed by white space; a code span is one unit, so a period in it ends no
// sentence. Python source: sentences.
func sentences(text string) []string {
	t := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(t); {
		if t[i] == tick {
			if end := codeSpanAt(t, i); end >= 0 {
				i = end
				continue
			}
		}
		if (t[i] == '.' || t[i] == '!' || t[i] == '?') && i+1 < len(t) && config.IsPySpace(t[i+1]) {
			out = append(out, strip(string(t[start:i+1])))
			start = i + 1
		}
		i++
	}
	if rest := strip(string(t[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

// words is the runs between white space that hold a letter or digit; a code span is one word. Python source: words.
func words(sentence string) []string {
	t := []rune(sentence)
	var out []string
	for i := 0; i < len(t); {
		if config.IsPySpace(t[i]) {
			i++
			continue
		}
		start := i
		for i < len(t) {
			if t[i] == tick {
				if end := codeSpanAt(t, i); end >= 0 {
					i = end
					continue
				}
				i++
				continue
			}
			if config.IsPySpace(t[i]) {
				break
			}
			i++
		}
		w := t[start:i]
		if slices.Contains(w, tick) || slices.ContainsFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			out = append(out, string(w))
		}
	}
	return out
}

// plain escapes a leading block marker (heading, quote, list item), so a fact stays one plain list item.
func plain(text string) string {
	t := []rune(text)
	after := func(i int) bool { return i >= len(t) || config.IsPySpace(t[i]) }
	if len(t) > 0 && (t[0] == '#' || t[0] == '>' || ((t[0] == '-' || t[0] == '+' || t[0] == '*') && after(1))) {
		text = `\` + text
		t = []rune(text)
	}
	n := 0
	for n < len(t) && n < 10 && unicode.IsDigit(t[n]) {
		n++
	}
	if n >= 1 && n <= 9 && n < len(t) && (t[n] == '.' || t[n] == ')') && after(n+1) {
		text = string(t[:n]) + `\` + string(t[n:])
	}
	return text
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
	for _, v := range p.values[dest] {
		a, b, _ := strings.Cut(v, pairSep)
		out = append(out, [2]string{a, b})
	}
	return out
}

// needText is every part a decision need's flags give, as given, for checks that run before its layout's.
func needText(p *Parsed) string {
	parts := append(append([]string{}, p.values["question"]...), p.values["fact"]...)
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
		if len(p.values[f.dest]) != 1 {
			return "", refuse("give exactly one %s; %s", f.flag, needShape)
		}
	}
	question, err := needPart("--question", p.values["question"][0])
	if err != nil {
		return "", err
	}
	var facts []string
	for _, f := range p.values["fact"] {
		v, err := needPart("--fact", f)
		if err != nil {
			return "", err
		}
		facts = append(facts, v)
	}
	var labels []string
	options, costs := map[string]string{}, map[string]string{}
	for _, o := range pairs(p, "option") {
		label := o[0]
		if !optionLabel.MatchString(label) {
			return "", refuse("the option label %s is not letters and digits only, e.g. a, b or keep", pyRepr(label))
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
	for _, c := range pairs(p, "cost") {
		label := c[0]
		if _, ok := options[label]; !ok {
			return "", refuse("--cost %s names no option; the labels are %s", label, strings.Join(labels, ", "))
		}
		if _, ok := costs[label]; ok {
			return "", refuse("option %s has two --cost flags; give each option one cost", label)
		}
		v, err := needPart("--cost "+label, c[1])
		if err != nil {
			return "", err
		}
		costs[label] = v
	}
	for _, label := range labels {
		if _, ok := costs[label]; !ok {
			return "", refuse("option %s has no cost; add --cost %s '<what it costs>'", label, label)
		}
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
		for _, s := range sentences(pt.text) {
			if found := words(s); len(found) > sentenceLimit {
				return "", refuse("the sentence \"%s …\" in %s has %d words; the limit is %d (ASD-STE100); split it",
					strings.Join(found[:6], " "), pt.name, len(found), sentenceLimit)
			}
		}
	}
	var b strings.Builder
	b.WriteString("**Question:** " + question + "\n\n**Facts:**\n\n")
	for _, f := range facts {
		b.WriteString("- " + plain(f) + "\n")
	}
	b.WriteString("\n**Options:**\n\n")
	for _, opt := range labels {
		text := options[opt]
		first := sentences(text)[0]
		more := strip(text[len(first):])
		b.WriteString("- **(" + opt + ") " + first + "** ")
		if more != "" {
			b.WriteString(more + " ")
		}
		b.WriteString("*Cost:* " + costs[opt] + "\n")
	}
	b.WriteString("\n**Default:** (" + label + "). " + rest)
	return b.String(), nil
}

// deliveryHint is how the raising agent hears the answer: the pm service pushes it into the session's inbox when there
// is one.
func deliveryHint(id string) string {
	if strip(os.Getenv(inboxEnv)) != "" {
		return fmt.Sprintf("the pm service pushes the owner's reply into this session; if the session has ended by then, "+
			"pm show flags it for the next one, which reads it with pm reply read %s", id)
	}
	return fmt.Sprintf("this session has no inbox ($%s unset), so no reply is pushed to it; pm show flags a reply, "+
		"and pm reply read %s prints it", inboxEnv, id)
}

// raisedBy is the session that raises a need and its inbox, or nil outside a known session. Never the inbox's token:
// the work store syncs to the remote.
func raisedBy() (*work.RaisedBy, error) {
	sid := currentSession()
	if sid == "" {
		return nil, nil
	}
	rb := &work.RaisedBy{Session: sid}
	if inbox := strip(os.Getenv(inboxEnv)); inbox != "" {
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

// dedupe is list with each value once, in first-seen order.
func dedupe(list []string) []string {
	var out []string
	for _, v := range list {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// raiseReview raises a PR review: a need of kind review carrying what the owner reads first, the PR, its sprints, the
// design pages behind it and the focus, so the site links each to its page.
func raiseReview(e *env, p *Parsed) (string, error) {
	pr := strip(p.Get("pr"))
	if !urlRE.MatchString(pr) {
		return "", refuse("--pr %s is not a URL; give the pull request's link", pyRepr(pr))
	}
	focus := strip(p.Get("focus"))
	if focus == "" {
		return "", refuse("--focus is empty; say what to look at first: the risky changes and the open choices")
	}
	extra := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprints := dedupe(p.values["sprint"])
	// A sprint closes only once its PR merges, so every sprint a PR under review delivers is open, and its delivery
	// report is written, for the owner to read next to the diff. The review goes under the first sprint and blocks its
	// close.
	for _, sid := range sprints {
		rec, err := r.sprint(sid)
		if err != nil {
			return "", err
		}
		if it := r.item(sid); it != nil && it.Status == work.Closed {
			return "", refuse("sprint %s is closed, but a sprint closes only after its PR merges; a PR under review "+
				"delivers only open sprints", sid)
		}
		line, err := records.Outcome(rec)
		if err != nil {
			return "", err
		}
		var unwritten []string
		for _, part := range []struct{ name, text string }{{"Outcome", line},
			{`Against "Done when"`, records.ReportPart(rec, `Against "Done when"`)}} {
			if part.text == "" || part.text == records.NotClosed {
				unwritten = append(unwritten, "'"+part.name+"'")
			}
		}
		if unwritten != nil {
			verb := "is"
			if len(unwritten) > 1 {
				verb = "are"
			}
			return "", refuse("%s: the committed Delivery report %s %s still '%s'; write the Outcome (done, partial or "+
				"voided, plus one sentence, then optional bullets) and each 'Done when' item with its evidence, and "+
				"commit them with pm commit, so the owner reads the report while reviewing the PR", r.rel(rec.Path),
				strings.Join(unwritten, " and "), verb, records.NotClosed)
		}
	}
	parent := sprints[0]
	designs := dedupe(p.values["design"])
	var known []string
	for _, rec := range r.recs {
		if rec.Type() == "design" && !slices.Contains(known, rec.Name()) {
			known = append(known, rec.Name())
		}
	}
	sort.Strings(known)
	for _, slug := range designs {
		if !slices.Contains(known, slug) {
			list := strings.Join(known, ", ")
			if list == "" {
				list = "none"
			}
			return "", refuse("--design %s has no design page; known: %s", slug, list)
		}
	}
	title := strip(p.Get("title"))
	if title == "" {
		title = "Review " + pr
		if m := pullNumber.FindStringSubmatch(pr); m != nil {
			title = "Review PR #" + m[1]
		}
	}
	desc := fmt.Sprintf("Review %s, which delivers %s.\n\nFocus: %s", pr, strings.Join(sprints, ", "), focus)
	if designs != nil {
		desc += "\n\nDesign pages: " + strings.Join(designs, ", ")
	}
	if extra != "" {
		desc += "\n\n" + extra
	}
	rb, err := raisedBy()
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Need, Parent: parent, Title: title, Description: desc,
		Need: &work.NeedInfo{Kind: work.Review, RaisedBy: rb,
			Review: &work.ReviewInfo{PR: pr, Sprints: sprints, Designs: designs, Focus: focus}}})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("raised review %s under %s; %s; the pm service pushes the PR's merge to main the same way; once "+
		"the PR is on main: pm action done %s --reason \"merged as <sha>\"", made.ID, parent, deliveryHint(made.ID),
		made.ID), nil
}

// ---------------------------------------------------------------- answering needs

// humanIssue is the need id, refused unless it waits for want (decision or action).
func (r *repo) humanIssue(id, want string) (*work.Item, error) {
	need := r.item(id)
	if need == nil {
		return nil, refuse("%s is not in the work store", id)
	}
	if need.Type != work.Need {
		return nil, refuse("%s is not a need; record your own decision with pm decision add "+
			"without --need", id)
	}
	if k := site.Kind(need); k != want {
		if k == "action" {
			return nil, refuse("%s is an action; close it with pm action done %s", id, id)
		}
		return nil, refuse("%s is a decision need; answer it with pm decision add --need %s, or pm decision close %s if "+
			"the answer sets no rule", id, id, id)
	}
	return need, nil
}

// refuseUnread refuses to close a need that holds a site reply not delivered yet: pm show flags only open needs, so
// the reply would reach no session.
func refuseUnread(need *work.Item) error {
	if service.ReplyWaiting(need) {
		return refuse("%s holds a site reply not delivered yet; read it with pm reply read %s, then run this again if it "+
			"still stands", need.ID, need.ID)
	}
	return nil
}

// answered is a need as an answer leaves it: closed with resolution and reason, the answer its last comment.
func answered(need *work.Item, resolution work.Resolution, reason string) work.Item {
	c := closedCopy(need, reason)
	c.Resolution = resolution
	return c
}

// decisionTarget is the open project or sprint record a decision of --level goes into, named by its flag.
func (r *repo) decisionTarget(p *Parsed) (*records.Record, error) {
	level, ok := given(p, "level")
	if !ok {
		return nil, refuse("--level is required (project|sprint): %s", levelRule)
	}
	flag, other := "--sprint", "--project"
	if level == "project" {
		flag, other = "--project", "--sprint"
	}
	if _, ok := given(p, other[2:]); ok {
		return nil, refuse("%s does not match --level %s; name the target with %s", other, level, flag)
	}
	target, ok := given(p, flag[2:])
	if !ok {
		return nil, refuse("--level %s needs %s; nothing is inferred", level, flag)
	}
	var rec *records.Record
	var err error
	if level == "project" {
		rec, err = r.project(target)
	} else {
		rec, err = r.sprint(target)
	}
	if err != nil {
		return nil, err
	}
	bead := rec.Bead()
	if it := r.item(bead); it == nil {
		return nil, refuse("%s %s has bead %s, which is not in the work store", level, target, bead)
	} else if it.Status == work.Closed {
		return nil, refuse("%s %s (%s) is closed; record the decision in an open %s", level, target, bead, level)
	}
	return rec, nil
}

// decisionLine is one part of a decision given by its flag: one non-empty line that opens no block.
func decisionLine(flag, value string) (string, error) {
	value = strip(value)
	switch {
	case value == "":
		return "", refuse("%s is empty; give it one line", flag)
	case strings.Contains(value, "\n"):
		return "", refuse("%s has more than one line; give it one line", flag)
	case strings.HasPrefix(value, ":::"):
		return "", refuse("%s starts with ':::', and blocks cannot nest", flag)
	}
	return value, nil
}

// decisionBlockUntil is a ::: decision block of today, from source, with --until when given.
func decisionBlockUntil(source, body string, p *Parsed) (string, error) {
	until, ok := given(p, "until")
	if !ok {
		return decisionBlock(source, body), nil
	}
	if strip(until) == "" || strings.ContainsAny(until, "\"\n") {
		return "", refuse("--until must be one non-empty line without double quotes")
	}
	return fmt.Sprintf("::: decision {source=%s date=%s until=\"%s\"}\n%s\n:::", source, store.Today(), strip(until),
		body), nil
}

func cmdDecisionAdd(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	rec, err := r.decisionTarget(p)
	if err != nil {
		return "", err
	}
	decision, err := decisionLine("--decision", p.Get("decision"))
	if err != nil {
		return "", err
	}
	reason, err := decisionLine("--reason", p.Get("reason"))
	if err != nil {
		return "", err
	}
	body := decision + "\n" + reason
	source := "agent"
	var need *work.Item
	needID, withNeed := given(p, "need")
	if withNeed && needID != "" {
		if need, err = r.humanIssue(needID, "decision"); err != nil {
			return "", err
		}
		if need.Status == work.Closed && need.Resolution == work.Dismissed {
			return "", refuse("need %s was dismissed, so it has no answer to record", needID)
		}
		source = "owner"
		if !site.Cites(body, needID) {
			body += fmt.Sprintf("\nAnswers `%s`.", needID)
		}
	} else if p.Get("confirmed") != "" {
		source = "owner"
	}
	block, err := decisionBlockUntil(source, body, p)
	if err != nil {
		return "", err
	}
	updated, err := records.InsertEntry(rec.Text, "Decisions", block)
	if err != nil {
		return "", err
	}
	done := fmt.Sprintf("added a source=%s %s decision to %s", source, p.Get("level"), r.rel(rec.Path))
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if need != nil && need.Resolution == work.NoDecision { // a rule after all: it is answered, as its decision says
		cited := *need
		cited.Resolution = work.Answered
		if err := r.checkPlanned(w, r.with(cited)); err != nil {
			return "", err
		}
		ws, err := e.work()
		if err != nil {
			return "", err
		}
		if err := ws.SetResolution(needID, work.Answered); err != nil {
			return "", err
		}
		out, err := r.apply(w, fmt.Sprintf("marked need %s answered and %s", needID, done), "", "pm: ")
		if err != nil {
			return "", fmt.Errorf("%w; the work-store step stands: %s is marked answered; record the decision again "+
				"with pm decision add --need %s and the same flags", err, needID, needID)
		}
		return out, nil
	}
	if need == nil || need.Status == work.Closed {
		if err := r.checkPlanned(w, nil); err != nil {
			return "", err
		}
		return r.apply(w, done, "", "pm: ")
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	if err := r.checkPlanned(w, r.with(answered(need, work.Answered, "Responded"))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Answer(needID, body); err != nil {
		return "", err
	}
	out, err := r.apply(w, fmt.Sprintf("closed need %s and %s", needID, done), "", "pm: ")
	if err != nil { // the work store has no reopen: say what stands and how to finish
		return "", fmt.Errorf("%w; the work-store step stands: %s is closed as answered with this decision; record the "+
			"decision again with pm decision add --need %s and the same flags", err, needID, needID)
	}
	return out, nil
}

// noDecision is the note a need gets when its answer sets no rule.
const noDecisionNote = "No decision record: "

func cmdDecisionClose(e *env, p *Parsed) (string, error) {
	answer := p.Get("text")
	why := strip(p.Get("reason"))
	if why == "" {
		return "", refuse("--reason is empty; say why the answer sets no rule")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("need_id")
	need, err := r.humanIssue(id, "decision")
	if err != nil {
		return "", err
	}
	if need.Resolution == work.NoDecision {
		return "", refuse("need %s is already marked no-decision", id)
	}
	if need.Status == work.Closed && need.Resolution == work.Dismissed {
		return "", refuse("need %s was dismissed, so it has no answer to mark", id)
	}
	if need.Status != work.Closed && answer == "" {
		return "", refuse("the answer is empty; pass the owner's answer, as they gave it, with %s", textForms)
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	note := noDecisionNote + why
	reason := need.CloseReason
	if reason == "" {
		reason = "Responded"
	}
	planned := answered(need, work.NoDecision, reason)
	if need.Status == work.Closed {
		planned = *need
		planned.Resolution = work.NoDecision
	}
	if err := r.checkPlanned(nil, r.with(planned)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	retry := fmt.Sprintf("run the same command again to label it: pm decision close %s --reason \"%s\"", id, why)
	var done, failed string
	if need.Status == work.Closed {
		if !slices.ContainsFunc(need.Comments, func(c work.Comment) bool { return strings.Contains(c.Text, note) }) {
			if _, err := ws.Comment(id, work.Note, noteAuthor(), note); err != nil {
				return "", err
			}
		}
		done = fmt.Sprintf("marked need %s, already answered, as setting no rule: %s", id, why)
		failed = fmt.Sprintf("the reason is on %s as a comment; %s (it does not add the note twice)", id, retry)
	} else {
		if err := ws.Answer(id, answer+"\n\n"+note); err != nil {
			return "", err
		}
		done = fmt.Sprintf("closed need %s with the owner's answer and no decision record: %s", id, why)
		failed = fmt.Sprintf("need %s is closed with the answer but not labelled no-decision, so the render check "+
			"flags it; %s", id, retry)
	}
	if err := ws.SetResolution(id, work.NoDecision); err != nil {
		return "", fmt.Errorf("%w; %s", err, failed)
	}
	return fmt.Sprintf("%s; if it sets a rule after all, record it with pm decision add --need %s, which marks the "+
		"need answered", done, id), nil
}

// noteAuthor is who a note pm writes is by: this session, else the owner at a shell.
func noteAuthor() string {
	if s := currentSession(); s != "" {
		return s
	}
	return "owner"
}

func cmdActionDone(e *env, p *Parsed) (string, error) {
	reason := strip(p.Get("reason"))
	if reason == "" {
		return "", refuse("--reason is empty; say what showed you the owner did it")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("need_id")
	need, err := r.humanIssue(id, "action")
	if err != nil {
		return "", err
	}
	if need.Status == work.Closed {
		return "", refuse("action %s is already closed", id)
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	if err := r.checkPlanned(nil, r.with(closedCopy(need, reason))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(id, reason, work.Done, closer(need)); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed action %s: %s", id, reason), nil
}

// ---------------------------------------------------------------- pm reply read

func cmdReplyRead(e *env, p *Parsed) (string, error) {
	if _, err := store.CodeRoot(e.here, e.records); err != nil {
		return "", err
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	var needs []work.Item
	if ids := dedupe(p.values["ids"]); ids != nil {
		all, err := ws.Items()
		if err != nil {
			return "", err
		}
		byID := map[string]work.Item{}
		for _, it := range all {
			byID[it.ID] = it
		}
		for _, id := range ids {
			it, ok := byID[id]
			if !ok || it.Type != work.Need {
				return "", refuse("%s is not a request to the owner (a need in the work store)", id)
			}
			needs = append(needs, it)
		}
	} else {
		sid := strip(os.Getenv(sessionEnv))
		if sid == "" {
			return "", refuse("name the requests to read: pm reply read <id>…; without %s pm cannot tell which "+
				"requests this session raised", sessionEnv)
		}
		mine, err := ws.Needs(sid)
		if err != nil {
			return "", err
		}
		for _, it := range mine {
			if it.Status != work.Closed { // open ones only
				needs = append(needs, it)
			}
		}
		sort.SliceStable(needs, func(i, j int) bool { return needs[i].ID < needs[j].ID })
	}
	texts := service.Texts{Main: store.MainOf(e.records), Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	var out, names []string
	for i := range needs {
		names = append(names, needs[i].ID)
		text, mark := texts.Undelivered(&needs[i])
		if text == "" {
			continue
		}
		out = append(out, text)
		if err := ws.UpdateNeed(needs[i].ID, *mark); err != nil {
			return "", err
		}
	}
	if out == nil {
		what := strings.Join(names, ", ")
		if what == "" {
			what = "any open request of this session"
		}
		return "nothing undelivered on " + what, nil
	}
	return strings.Join(out, "\n"), nil
}
