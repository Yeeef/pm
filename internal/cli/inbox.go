package cli

import (
	"errors"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// inboxMarkBudget bounds all of pm hook inbox's marking, after it printed, well under the hook's 10 s timeout: what
// it has not marked by then stays undelivered and comes again at the next prompt (twice is acceptable, lost is not).
var inboxMarkBudget = 5 * time.Second

// pendingReplies is pm hook inbox's read: the replies (and reviews' merges) to the session's open needs that have not
// reached it, each as the pm service pushes it, from the work store of the clone containing dir; deliver marks them
// delivered, as the push does once its write went through, and lets go of the store.
func pendingReplies(dir, session string) ([]string, func() error, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, nil, err
	}
	records, err := store.PathOf(dir)
	if err != nil {
		return nil, nil, err
	}
	main := store.MainOf(records)
	ws, err := OpenWork(main)
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
