package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/needform"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/work"
	"regexp"
)

// The shapes and patterns shared by the commands that carry the owner's needs and replies (each in its noun's file),
// pm need edit and the need checks: decisions recorded or asked for, actions and PR reviews raised and closed, and
// replies read. Each refuses before any write. A site
// reply only adds a reply comment; a need stays open until a session records it with one of these commands, which close
// it with its resolution (the work-store page, Commands).

const (
	actionShape = "an action's description says what the owner should do and why, e.g.:\n" +
		"  Restart the site on port 8767: the new proxy expects it there."
	reviewForm = `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`
)

var (
	// A request names a PR by its GitHub link or "PR #<n>"; it asks for a review or merge when it also says review,
	// merge or approve (Python's PR_NAMED and REVIEW_ASKED; their \b is checked by wordBefore).
	prLink      = regexp.MustCompile(`(?i)https://github\.com/[\p{L}\p{N}_.-]+/[\p{L}\p{N}_.-]+/pull/\p{Nd}+`)
	prNumbered  = regexp.MustCompile(`(?i)PR` + records.PyS + `*#\p{Nd}+`)
	reviewAsked = regexp.MustCompile(`(?i)(review|merg|approv)`)
	urlRE       = regexp.MustCompile(`^https?://[^\t\n\v\f\r \x1c-\x1f\x85\p{Z}]+$`)
	pullNumber  = regexp.MustCompile(`/pull/(\p{Nd}+)`)
)

// refuseAll is the one refusal for every rule v holds, nil when it holds none.
func refuseAll(v *needform.Violations) error {
	if m := v.Message(); m != "" {
		return refuse("%s", m)
	}
	return nil
}

// cmdNeedEdit rewrites an open action's or decision need's body, with raiseNeed's checks, until the owner replies.
func cmdNeedEdit(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("need_id")
	need := r.item(id)
	if need == nil {
		return "", refuse("%s is not in the work store", id)
	}
	if need.Type != work.Need || need.Need == nil {
		return "", refuse("%s is a %s, not a need; a task's description is pm task edit's, a sprint's title pm "+
			"sprint edit's", id, need.Type)
	}
	if need.Status == work.Closed {
		return "", refuse("%s is closed (%s); only an open need's body is edited", id, need.Resolution)
	}
	if work.HasReply(need) {
		return "", refuse("%s holds the owner's reply, which answers the body it has; raise a new need for the changed "+
			"ask, and close this one with pm need close %s --dropped --reason \"…\"", id, id)
	}
	parts := needform.Text(p.Values)
	_, textGiven := given(p, "text")
	var desc string
	var v needform.Violations
	switch need.Need.Kind {
	case work.Question:
		return "", refuse("%s is a session's question, whose card pm builds from the ask tool's call; ask again with the "+
			"ask tool and close this one with pm need close %s --reason \"…\"", id, id)
	case work.Review:
		return "", refuse("%s is a PR review, whose card pm builds from its PR, sprints and focus; raise the review "+
			"again with %s and close this one with pm need close %s --dropped --reason \"…\"", id, reviewForm, id)
	case work.Action:
		if parts != "" {
			return "", refuse("%s is an action; its body is the description, given with %s, not a decision need's "+
				"parts", id, textForms)
		}
		if desc = p.Get("text"); desc == "" {
			v.Add("the description is empty; pass it with %s: %s", textForms, actionShape)
		}
	case work.Decision:
		if textGiven {
			return "", refuse("%s is a decision need; its body is its parts, given as flags, not --text: %s", id,
				needform.Shape)
		}
		desc = needform.Markdown(p.Values, &v)
	}
	if asksReview(need.Title + "\n" + p.Get("text") + parts) {
		v.Add("this asks the owner to review or merge a PR; raise it with the review form, so its card links "+
			"the PR, the sprints and design pages and its wait wakes on the merge: %s. If it only mentions the PR, say "+
			"what you ask without review, merge or approve", reviewForm)
	}
	if err := refuseAll(&v); err != nil {
		return "", err
	}
	if desc == need.Description {
		return "", refuse("%s has this body already", id)
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Edit(id, nil, &desc); err != nil {
		return "", err
	}
	return fmt.Sprintf("edited the body of %s %s", need.Need.Kind, id), nil
}

// ---------------------------------------------------------------- answering needs

// cmdNeedClose closes a need that sets no rule, its resolution picked by its kind: a decision need as no-decision with
// the owner's answer, an action or PR review as done, a question as dismissed; --dropped closes any open need as
// dismissed, with no answer.
func cmdNeedClose(e *env, p *Parsed) (string, error) {
	id, reason := p.Get("need_id"), strip(p.Get("reason"))
	_, answered := given(p, "text")
	dropped := p.Get("dropped") != ""
	if reason == "" {
		return "", refuse("--reason is empty; say why the need closes")
	} else if dropped && answered {
		return "", refuse("--dropped closes a need with no answer; drop %s", textForms)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	need := r.item(id)
	if need == nil || need.Type != work.Need {
		return "", refuse("%s is no need in the work store; a task closes with pm task close", id)
	}
	kind := site.Kind(need)
	if answered && kind != "decision" {
		return "", refuse("%s is of kind %s; only a decision need closes with the owner's answer: drop %s", id, kind,
			textForms)
	}
	if kind == "action" && !dropped {
		return closeAction(e, r, need, reason)
	} else if kind == "decision" && !dropped {
		return closeDecisionNeed(e, r, need, reason, p.Get("text"))
	}
	if need.Status != work.Open { // dismissed: a question, or any need --dropped
		return "", refuse("%s is closed already (%s)", id, need.Resolution)
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(need.ID, reason, work.Dismissed, currentSession()); err != nil {
		return "", err
	}
	return fmt.Sprintf("dismissed %s: %s", need.ID, reason), nil
}

// decisionNeed is the need id, refused unless it is a decision need: an action or a question records no decision.
func (r *repo) decisionNeed(id string) (*work.Item, error) {
	need := r.item(id)
	if need == nil {
		return nil, refuse("%s is not in the work store", id)
	}
	if need.Type != work.Need {
		return nil, refuse("%s is not a need; record your own decision with pm decision add "+
			"without --need", id)
	}
	if k := site.Kind(need); k == "question" {
		return nil, refuse("%s is a session's question, whose answer goes to the session that asked and records no "+
			"decision; close it with pm need close %s --reason \"…\"", id, id)
	} else if k == "action" {
		return nil, refuse("%s is an action; close it with pm need close %s --reason \"…\"", id, id)
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
