package service

import (
	"errors"
	"strings"

	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
)

// A question need is a session's ask-tool call (work.NewQuestion), which the owner may answer in the session or on
// the site. The site's answer reaches the session one way per runtime: a Claude Code session's through pm hook ask,
// its PermissionRequest hook, which waits beside the question's dialog, so the service never pushes it; a Codex
// session's through codex queue, once the service closed the question for it. A chat answer is closed by pm hook ask
// (Claude Code's PostToolUse, Codex's UserPromptSubmit), never from a transcript or rollout. Of two answers, the first
// to close the need wins: a close of a closed need fails with work.ErrClosed.

// Close reasons of a question need.
const (
	AnsweredOnSite = "answered on the site"
	SessionEnded   = "the session that asked has ended"
)

// QuestionStore is what closing a question writes.
type QuestionStore interface {
	Close(id, reason string, resolution work.Resolution, session string) error
	Answer(id, text, session string) error
	Comment(id string, kind work.CommentKind, author, text string) (work.Comment, error)
	UpdateNeed(id string, u work.NeedUpdate) error
	Get(ids ...string) ([]work.Item, error)
}

// IsQuestion is whether the item is a question need.
func IsQuestion(n *work.Item) bool {
	return n.Need != nil && n.Need.Kind == work.Question && n.Need.Ask != nil
}

// SiteAnswer is the text of a need's site replies not delivered yet, as the owner wrote them, oldest first.
func SiteAnswer(n *work.Item) string {
	replies := answerReplies(n)
	var said []string
	for _, c := range replies[min(n.Need.Delivered, len(replies)):] {
		said = append(said, strings.TrimSpace(ReplyBody(c.Text)))
	}
	return strings.Join(said, "\n\n")
}

// answerReplies is a need's site replies (siteReplies), or, for a question closed as answered on the site, every
// reply: a closed question takes no reply, and the store stamps to the second, so a reply in the close's second is no
// earlier than the close, which siteReplies would leave out.
func answerReplies(n *work.Item) []work.Comment {
	if !IsQuestion(n) || n.Status != work.Closed || n.CloseReason != AnsweredOnSite {
		return siteReplies(n)
	}
	var out []work.Comment
	for _, c := range n.Comments {
		if c.Kind == work.Reply {
			out = append(out, c)
		}
	}
	return out
}

// AnswerInChat closes an open question with the answer given in the session's own chat, by session. Site replies it
// held, read again once it closed, lost the race: they count as delivered, with a note that says so. A need closed
// already fails with work.ErrClosed.
func AnswerInChat(st QuestionStore, n *work.Item, answer, session string) error {
	if err := st.Answer(n.ID, answer, session); err != nil {
		return err
	}
	got, err := st.Get(n.ID)
	if err != nil {
		return err
	}
	// the replies but the one Answer wrote: the store stamps to the second, so a reply in the close's second is no
	// earlier than the close, and siteReplies would leave it out
	total := -1
	for _, c := range got[0].Comments {
		if c.Kind == work.Reply {
			total++
		}
	}
	if total <= got[0].Need.Delivered {
		return nil
	}
	_, err = st.Comment(n.ID, work.Note, "pm", "answered in chat first; the site's reply did not reach the session")
	return errors.Join(err, st.UpdateNeed(n.ID, work.NeedUpdate{Delivered: &total}))
}

// Relayed is whether a message in a session is one the pm service handed it, not the owner's own: a delivery's text
// (each starts "pm: ") or the site answer of a question of needs, the session's, that the service queued: it closes a
// question as answered on the site before it queues the answer.
func Relayed(needs []work.Item, text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "pm: ") {
		return true
	}
	for i := range needs {
		n := &needs[i]
		if !IsQuestion(n) || n.Status != work.Closed || n.CloseReason != AnsweredOnSite {
			continue
		}
		var bodies []string
		for _, c := range answerReplies(n) {
			bodies = append(bodies, strings.TrimSpace(ReplyBody(c.Text)))
		}
		for from := range bodies {
			if text == bodies[from] || text == strings.Join(bodies[from:], "\n\n") {
				return true
			}
		}
	}
	return false
}

// CodexQuestion is the CODEX_HOME of a question a Codex session asked; false for any other need.
func CodexQuestion(n *work.Item) (string, bool) {
	if !IsQuestion(n) || n.Need.RaisedBy == nil {
		return "", false
	}
	return session.CodexHomeOf(n.Need.RaisedBy.Inbox)
}

// Unqueued is whether n is a Codex question the service closed as answered on the site whose answer codex queue has
// not taken: the close went through and the queue failed, so the sweep queues it again.
func Unqueued(n *work.Item) bool {
	_, codex := CodexQuestion(n)
	return codex && n.Status == work.Closed && n.CloseReason == AnsweredOnSite && len(answerReplies(n)) > n.Need.Delivered
}

// deliverQuestion is pushUndelivered for a question with a site answer to deliver: none for a Claude Code session's,
// whose hook takes the answer; a Codex session's question is closed as answered on the site, then the site's answer,
// alone, is queued into the session and marked delivered. The close comes first, so it decides which answer wins: a
// question closed already (the owner answered in the chat, pm hook ask closed it at UserPromptSubmit) queues nothing.
// A session the answer cannot reach now keeps its question open; a queue that fails once the question closed leaves
// the answer undelivered (Unqueued), for the sweep.
func (s *server) deliverQuestion(n *work.Item) (string, error) {
	home, codex := CodexQuestion(n)
	if !codex {
		return "no push: a Claude Code session's question takes its answer through pm hook ask", nil
	}
	if why := pushBlocked(n); why != "" {
		return why, nil
	}
	text, total := SiteAnswer(n), len(answerReplies(n))
	if n.Status != work.Closed {
		err := s.withStore("close "+n.ID, func(st work.Store) error {
			return st.Close(n.ID, AnsweredOnSite, work.Answered, "")
		})
		if errors.Is(err, work.ErrClosed) {
			return "closed already: answered in the session first", nil
		}
		if err != nil {
			return "", err
		}
	}
	if why := queueCodex(home, n.Need.RaisedBy.Session, text); why != "" {
		return why, nil
	}
	err := s.withStore("delivered "+n.ID, func(st work.Store) error {
		return st.UpdateNeed(n.ID, work.NeedUpdate{Delivered: &total})
	})
	return "delivered", err
}

// settleQuestion closes an open question the service sees settled away from the site: a Claude Code session's
// messaging socket is gone, so its hook is gone too (a backstop: the hook closes the question when the session stops
// it). A Codex question is never dismissed for idleness: an idle TUI waiting on the owner looks the same as an ended
// session, and pm hook ask closes a chat answer. A question raised on another machine, or by a Claude Code session with
// no socket, is left to its hook. What it did, "" for nothing.
func (s *server) settleQuestion(n *work.Item) (string, error) {
	rb := n.Need.RaisedBy
	if rb == nil || rb.Session == "" || rb.Inbox == "" {
		return "", nil
	}
	if host, _ := Hostname(); rb.Host != "" && rb.Host != host { //nolint:errcheck // no name: no host matches
		return "", nil
	}
	if _, codex := session.CodexHomeOf(rb.Inbox); codex {
		return "", nil
	}
	if _, live := inboxKey(rb); live {
		return "", nil
	}
	err := s.withStore("close "+n.ID, func(st work.Store) error { return st.Close(n.ID, SessionEnded, work.Dismissed, "") })
	if errors.Is(err, work.ErrClosed) {
		return "", nil
	}
	return "dismissed: " + SessionEnded, err
}
