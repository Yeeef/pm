package cli

import (
	"errors"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// pendingReplies is pm hook inbox's read: the replies (and reviews' merges) to the session's open needs that have not
// reached it, each as the pm service pushes it, marked delivered as the push marks it, from the work store of the
// clone containing dir.
func pendingReplies(dir, session string) (texts []string, err error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	records, err := store.PathOf(dir)
	if err != nil {
		return nil, err
	}
	main := store.MainOf(records)
	ws, err := OpenWork(main)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, ws.Shutdown()) }()
	needs, err := ws.Needs(session)
	if err != nil {
		return nil, err
	}
	t := service.Texts{Main: main, Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	for i := range needs {
		if needs[i].Status == work.Closed {
			continue
		}
		text, mark := t.Undelivered(&needs[i])
		if text == "" {
			continue
		}
		if err := ws.UpdateNeed(needs[i].ID, *mark); err != nil {
			return texts, err
		}
		texts = append(texts, text)
	}
	return texts, nil
}
