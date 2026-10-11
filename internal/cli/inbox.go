package cli

import (
	"errors"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// inboxLockWait bounds pm hook inbox's wait for the store's write lock, well under the hook's 10 s timeout: a hook
// that waits longer fails open, and the replies wait for the next prompt.
const inboxLockWait = 3 * time.Second

// pendingReplies is pm hook inbox's read: the replies (and reviews' merges) to the session's open needs that have not
// reached it, each as the pm service pushes it, from the work store of the clone containing dir; deliver marks them
// delivered, as the push does once its write went through, and lets go of the store.
func pendingReplies(dir, session string) ([]string, func() error, error) {
	work.WriteLockWait = inboxLockWait // this process runs the hook alone
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
	needs, err := ws.Needs(session)
	if err != nil {
		return nil, nil, errors.Join(err, ws.Shutdown())
	}
	t := service.Texts{Main: main, Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	var texts []string
	type mark struct {
		id string
		u  work.NeedUpdate
	}
	var marks []mark
	for i := range needs {
		if needs[i].Status == work.Closed {
			continue
		}
		if text, u := t.Undelivered(&needs[i]); text != "" {
			texts = append(texts, text)
			marks = append(marks, mark{needs[i].ID, *u})
		}
	}
	deliver := func() error {
		var errs []error
		for _, m := range marks {
			errs = append(errs, ws.UpdateNeed(m.id, m.u))
		}
		return errors.Join(append(errs, ws.Shutdown())...)
	}
	return texts, deliver, nil
}
