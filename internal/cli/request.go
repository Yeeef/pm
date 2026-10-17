package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/needform"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
	"regexp"
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

// deliveryHint is how the raising agent hears the answer: the pm service pushes it into the session's inbox when there
// is one (a Claude Code socket, or codex queue into a Codex session); else, and once the session has ended, pm show
// flags it and pm reply read prints it.
func deliveryHint(id string) string {
	inbox := sessionInbox()
	if _, codex := session.CodexHomeOf(inbox); codex {
		return fmt.Sprintf("the pm service queues the owner's reply into this Codex session (codex queue), which "+
			"starts a turn with it when idle; if the session has ended by then, pm show flags it for the next one, "+
			"which reads it with pm reply read %s", id)
	}
	if inbox != "" {
		return fmt.Sprintf("the pm service pushes the owner's reply into this session; if the session has ended by "+
			"then, pm show flags it for the next one, which reads it with pm reply read %s", id)
	}
	return fmt.Sprintf("this session gets no push of the owner's reply (no inbox pm can push to): pm show flags it "+
		"once it is posted, and pm reply read %s prints it", id)
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

// raiseNeed raises a decision need or an action: a need item of that kind under the sprint --sprint names or the task
// --task names (the caller checked that exactly one is given).
func raiseNeed(e *env, p *Parsed, kind work.NeedKind) (string, error) {
	var v needform.Violations
	title := strip(p.Get("title"))
	if title == "" {
		v.Add("--title is empty")
	}
	desc := ""
	text := title + "\n"
	if kind == work.Action {
		desc = p.Get("text")
		text += desc
	} else {
		text += needform.Text(p.Values)
	}
	if asksReview(text) {
		v.Add("this asks the owner to review or merge a PR; raise it with the review form, so its card links "+
			"the PR, the sprints and design pages and its wait wakes on the merge: %s. If it "+
			"only mentions the PR, say what you ask without review, merge or approve", reviewForm)
	}
	if kind == work.Action && desc == "" {
		v.Add("the description is empty; pass it with %s: %s", textForms, actionShape)
	}
	if kind == work.Decision {
		desc = needform.Markdown(p.Values, &v)
	}
	r, err := e.load(false)
	if err != nil {
		if verr := refuseAll(&v); verr != nil { // the form's own rules need no store: refuse them first
			return "", verr
		}
		return "", err
	}
	flag, want := "--sprint", work.Sprint
	parentID, ok := given(p, "sprint")
	if !ok {
		flag, want = "--task", work.Task
		parentID = p.Get("task")
	}
	parent := r.item(parentID)
	switch {
	case parent == nil:
		v.Add("%s %s is not in the work store", flag, parentID)
	case parent.Type != want:
		v.Add("%s %s is a %s, not a %s; name a sprint with --sprint ID, a task with --task ID", flag, parentID,
			parent.Type, want)
	case parent.Status == work.Closed:
		v.Add("%s %s is closed; raise it under an open sprint or task", flag, parentID)
	default:
		inside := false
		for _, a := range r.x.Ancestors(parentID) {
			for _, rec := range r.recs {
				inside = inside || rec.Type() == "project" && rec.Bead() == a
			}
		}
		if !inside {
			v.Add("%s %s is not inside a project with a record, so no page would show it", flag, parentID)
		}
	}
	if err := refuseAll(&v); err != nil {
		return "", err
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
		return fmt.Sprintf("raised action %s under %s; %s; once you see the owner has done it: pm need close %s "+
			"--reason \"<what you saw>\"", id, parentID, deliveryHint(id), id), nil
	}
	return fmt.Sprintf("raised decision need %s under %s; %s; once the owner answers: pm decision add --need %s "+
		"--sprint ID|--project NAME --decision '<the answer>' --reason '<why>' if it sets a rule, else pm need close %s --reason "+
		"\"<why it sets no rule>\" --text-file - <<'EOF' (the answer, then EOF)", id, parentID, deliveryHint(id), id, id), nil
}

func cmdDecisionNeed(e *env, p *Parsed) (string, error) { return raiseNeed(e, p, work.Decision) }

func cmdActionNeed(e *env, p *Parsed) (string, error) {
	if _, ok := given(p, "pr"); !ok {
		for _, f := range []struct{ flag, dest string }{{"--focus", "focus"}, {"--design", "design"}} {
			if _, ok := given(p, f.dest); ok {
				return "", refuse("%s is only for a PR review; give --pr URL with it, or drop it", f.flag)
			}
		}
		_, task := given(p, "task")
		switch n := len(p.ValuesOf("sprint")); {
		case n > 1:
			return "", refuse("--sprint is given %d times; an action belongs to one sprint or task", n)
		case n == 1 && task:
			return "", refuse("--sprint and --task both given; an action belongs to one sprint or task")
		case n == 0 && !task:
			return "", refuse("--sprint ID or --task ID is required (or --pr for a PR review)")
		}
		if _, ok := given(p, "title"); !ok {
			return "", refuse("--title is required (or --pr for a PR review)")
		}
		return raiseNeed(e, p, work.Action)
	}
	if _, ok := given(p, "task"); ok {
		return "", refuse("--task is not allowed with --pr; a PR review goes under the first sprint it delivers")
	}
	for _, f := range []struct{ flag, dest string }{{"--sprint", "sprint"}, {"--focus", "focus"}} {
		if _, ok := given(p, f.dest); !ok {
			return "", refuse("%s is required with --pr", f.flag)
		}
	}
	return raiseReview(e, p)
}
