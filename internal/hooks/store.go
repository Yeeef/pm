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

// PendingReplies is pm hook inbox's read: the replies (and reviews' merges) to the session's open needs that have not
// reached it, each as the pm service pushes it, from the work store of the clone containing dir; deliver marks them
// delivered, as the push does once its write went through, and lets go of the store.
func PendingReplies(dir, session string) ([]string, func() error, error) {
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
	if err != nil {
		return nil, nil, errors.Join(err, ws.Shutdown())
	}
	deliver := func() error {
		return errors.Join(service.MarkWithin(ws, marks, inboxMarkBudget, time.Now, func(left time.Duration) {
			work.WriteLockWait = left //nolint:reassign // this process runs the one hook: its writes wait at most the budget left
		}), ws.Shutdown())
	}
	return texts, deliver, nil
}
