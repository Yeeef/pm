package hooks

import (
	"errors"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The hooks' reads and writes of the work store of the clone containing the hook's directory, through the pm service
// that holds it: one connection per use, closed before the hook goes on.

// dial connects to the work store of the clone containing dir.
func dial(dir string) (work.Store, error) {
	records, err := store.PathOf(dir)
	if err != nil {
		return nil, err
	}
	return work.Dial(store.MainOf(records))
}

// OpenNeeds is pm hook owner-request's read: the open needs the session raised, from the work store of the clone
// containing dir, which closes before the judge runs. Python pm listed them with bd (issues labelled human whose
// metadata session is the session, closed ones left out).
func OpenNeeds(dir, session string) ([]Request, error) {
	ws, err := dial(dir)
	if err != nil {
		return nil, err
	}
	needs, err := ws.Needs(session)
	if err := errors.Join(err, ws.Shutdown()); err != nil {
		return nil, err
	}
	var out []Request
	for _, it := range needs {
		if it.Status != work.Closed {
			out = append(out, Request{ID: it.ID, Title: it.Title, Description: it.Description})
		}
	}
	return out, nil
}

// inboxMarkBudget bounds all of pm hook inbox's marking, after it printed, well under the hook's 10 s timeout: what
// it has not marked by then stays undelivered and comes again at the next prompt (twice is acceptable, lost is not).
var inboxMarkBudget = 5 * time.Second

// SessionReplies is pm hook inbox's read: the replies (and reviews' merges) to the session's open needs that have not
// reached it, each as the pm service pushes it, from the work store of the clone containing dir; deliver marks them
// delivered, as the push does once its write went through, then closes the session's open Codex questions (a Claude
// Code question's hook takes its own answer): one whose site answer it handed over as answered on the site, any
// other with prompt, the owner's answer in the chat; then it lets go of the store.
func SessionReplies(dir, session, prompt string) ([]string, func() error, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, nil, err
	}
	records, err := store.PathOf(dir)
	if err != nil {
		return nil, nil, err
	}
	main := store.MainOf(records)
	ws, err := work.Dial(main)
	if err != nil {
		return nil, nil, err
	}
	t := service.Texts{Main: main, Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	texts, marks, err := t.SessionUndelivered(ws, session)
	var needs []work.Item
	if err == nil {
		needs, err = ws.Needs(session)
	}
	if err != nil {
		return nil, nil, errors.Join(err, ws.Shutdown())
	}
	deliver := func() error {
		deadline := time.Now().Add(inboxMarkBudget)
		errs := []error{service.MarkWithin(ws, marks, inboxMarkBudget, time.Now, func(left time.Duration) {
			work.WriteLockWait = left //nolint:reassign // this process runs the one hook: its writes wait at most the budget left
		})}
		work.WriteLockWait = max(time.Until(deadline), 0) //nolint:reassign // as above
		errs = append(errs, closeChatQuestions(ws, needs, prompt, session), ws.Shutdown())
		return errors.Join(errs...)
	}
	return texts, deliver, nil
}

// closeChatQuestions closes the open Codex questions among needs: answered on the site when a site reply waited (the
// inbox has just handed it over), else in the chat with prompt. A question closed meanwhile is left as it is.
func closeChatQuestions(st service.QuestionStore, needs []work.Item, prompt, session string) error {
	var errs []error
	for i := range needs {
		n := &needs[i]
		if _, codex := service.CodexQuestion(n); !codex {
			continue
		}
		var err error
		if service.ReplyWaiting(n) {
			err = service.AnswerOnSite(st, n, session)
		} else if prompt != "" && !service.Relayed(needs, prompt) { // the service's queued answer to another need
			err = service.AnswerInChat(st, n, prompt, session)
		}
		if !errors.Is(err, work.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
