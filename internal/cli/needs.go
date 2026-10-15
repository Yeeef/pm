package cli

import (
	"fmt"
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
			"ask, and close this one with pm need dismiss %s --reason \"…\"", id, id)
	}
	parts := needText(p)
	_, textGiven := given(p, "text")
	var desc string
	var v violations
	switch need.Need.Kind {
	case work.Question:
		return "", refuse("%s is a session's question, whose card pm builds from the ask tool's call; ask again with the "+
			"ask tool and dismiss this one with pm need dismiss %s --reason \"…\"", id, id)
	case work.Review:
		return "", refuse("%s is a PR review, whose card pm builds from its PR, sprints and focus; raise the review "+
			"again with %s and dismiss this one with pm need dismiss %s --reason \"…\"", id, reviewForm, id)
	case work.Action:
		if parts != "" {
			return "", refuse("%s is an action; its body is the description, given with %s, not a decision need's "+
				"parts", id, textForms)
		}
		if desc = p.Get("text"); desc == "" {
			v.add("the description is empty; pass it with %s: %s", textForms, actionShape)
		}
	case work.Decision:
		if textGiven {
			return "", refuse("%s is a decision need; its body is its parts, given as flags, not --text: %s", id,
				needShape)
		}
		desc = needMarkdown(p, &v)
	}
	if asksReview(need.Title + "\n" + p.Get("text") + parts) {
		v.add("this asks the owner to review or merge a PR; raise it with the review form, so its card links "+
			"the PR, the sprints and design pages and its wait wakes on the merge: %s. If it only mentions the PR, say "+
			"what you ask without review, merge or approve", reviewForm)
	}
	if err := v.err(); err != nil {
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

// humanIssue is the need id, refused unless it waits for want (decision or action); a question never does.
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
		if k == "question" {
			return nil, refuse("%s is a session's question, whose answer goes to the session that asked and records no "+
				"decision; close it with pm need dismiss %s --reason \"…\"", id, id)
		}
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
