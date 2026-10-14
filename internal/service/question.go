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
// session's through codex queue, unless the rollout shows the owner answered in the chat first. Of two answers, the
// first to close the need wins: a close of a closed need fails with work.ErrClosed.

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
}

// IsQuestion is whether the item is a question need.
func IsQuestion(n *work.Item) bool {
	return n.Need != nil && n.Need.Kind == work.Question && n.Need.Ask != nil
}

// SiteAnswer is the text of a need's site replies not delivered yet, as the owner wrote them, oldest first.
func SiteAnswer(n *work.Item) string {
	replies := siteReplies(n)
	var said []string
	for _, c := range replies[min(n.Need.Delivered, len(replies)):] {
		said = append(said, strings.TrimSpace(ReplyBody(c.Text)))
	}
	return strings.Join(said, "\n\n")
}

// AnswerOnSite marks an open question's site replies delivered, once they reached the session, then closes it as
// answered on the site, by session (the one the answer reached, "" for the service). A need closed meanwhile fails
// with work.ErrClosed, its replies marked still: they did reach the session.
func AnswerOnSite(st QuestionStore, n *work.Item, session string) error {
	total := len(siteReplies(n))
	if err := st.UpdateNeed(n.ID, work.NeedUpdate{Delivered: &total}); err != nil {
		return err
	}
	return st.Close(n.ID, AnsweredOnSite, work.Answered, session)
}

// AnswerInChat closes an open question with the answer given in the session's own chat, by session. Site replies it
// held lost the race: they count as delivered, with a note that says so. A need closed already fails with
// work.ErrClosed.
func AnswerInChat(st QuestionStore, n *work.Item, answer, session string) error {
	if err := st.Answer(n.ID, answer, session); err != nil {
		return err
	}
	total := len(siteReplies(n))
	if total <= n.Need.Delivered {
		return nil
	}
	_, err := st.Comment(n.ID, work.Note, "pm", "answered in chat first; the site's reply did not reach the session")
	return errors.Join(err, st.UpdateNeed(n.ID, work.NeedUpdate{Delivered: &total}))
}

// CodexQuestion is the CODEX_HOME of an open question a Codex session asked; false for any other need.
func CodexQuestion(n *work.Item) (string, bool) {
	if !IsQuestion(n) || n.Status == work.Closed || n.Need.RaisedBy == nil {
		return "", false
	}
	return session.CodexHomeOf(n.Need.RaisedBy.Inbox)
}

// ChatAnswer is the answer the owner typed into the Codex session after its question, from its rollouts; false when
// none, or the need is no open Codex question.
func ChatAnswer(n *work.Item) (string, bool) {
	home, ok := CodexQuestion(n)
	if !ok {
		return "", false
	}
	return session.ChatAfterCall(session.RolloutsIn(home, n.Need.RaisedBy.Session), n.Need.Ask.Call)
}

// deliverQuestion is pushUndelivered for a question: none for a Claude Code session's, whose hook takes the answer;
// for a Codex session's, the answer typed in its chat closes it, else the site's answer, alone, is queued into the
// session, then marked delivered and the need closed.
func (s *server) deliverQuestion(n *work.Item) (string, error) {
	if _, codex := CodexQuestion(n); !codex {
		return "no push: a Claude Code session's question takes its answer through pm hook ask", nil
	}
	if chat, ok := ChatAnswer(n); ok {
		err := s.withStore("answer "+n.ID, func(st work.Store) error { return AnswerInChat(st, n, chat, "") })
		if errors.Is(err, work.ErrClosed) {
			return "closed already", nil
		}
		return "answered in chat first", err
	}
	if why := PushInbox(n, SiteAnswer(n)); why != "" {
		return why, nil
	}
	err := s.withStore("delivered "+n.ID, func(st work.Store) error { return AnswerOnSite(st, n, "") })
	if err != nil && !errors.Is(err, work.ErrClosed) { // closed meanwhile by the session's own prompt: delivered still
		return "", err
	}
	return "delivered", nil
}

// settleQuestion closes an open question the service sees settled away from the site: a Codex session's owner
// answered in its chat, or the session that asked has ended (a Codex rollout idle past the live window, a Claude Code
// session's messaging socket gone), unless a site answer waits for a Codex session's next prompt. A question raised on another machine, or by a
// Claude Code session with no socket, is left to its hook. What it did, "" for nothing.
func (s *server) settleQuestion(n *work.Item) (string, error) {
	rb := n.Need.RaisedBy
	if rb == nil || rb.Session == "" {
		return "", nil
	}
	if host, _ := Hostname(); rb.Host != "" && rb.Host != host { //nolint:errcheck // no name: no host matches
		return "", nil
	}
	home, codex := session.CodexHomeOf(rb.Inbox)
	var live bool
	switch {
	case codex:
		if chat, ok := ChatAnswer(n); ok {
			return s.closeQuestion(n, "answered in chat", func(st work.Store) error {
				return AnswerInChat(st, n, chat, "")
			})
		}
		live = CodexLive(home, rb.Session)
	case rb.Inbox != "":
		_, live = inboxKey(rb)
	default:
		return "", nil
	}
	if live || codex && ReplyWaiting(n) { // a Codex session gets the answer at its next prompt (pm hook inbox)
		return "", nil
	}
	return s.closeQuestion(n, "dismissed: "+SessionEnded, func(st work.Store) error {
		return st.Close(n.ID, SessionEnded, work.Dismissed, "")
	})
}

// closeQuestion runs the close, saying what it did; a need that is closed already is nothing done.
func (s *server) closeQuestion(n *work.Item, did string, write func(work.Store) error) (string, error) {
	err := s.withStore("close "+n.ID, write)
	if errors.Is(err, work.ErrClosed) {
		return "", nil
	}
	return did, err
}
