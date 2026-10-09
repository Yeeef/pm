package cli

import (
	"errors"

	"github.com/Yeeef/yeeef-agents/pm/internal/hooks"
	"github.com/Yeeef/yeeef-agents/pm/internal/store"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// openRequests is pm hook owner-request's read: the open needs the session raised, from the work store of the clone
// containing dir, which closes before the judge runs. Python pm lists them with bd (issues labelled human whose
// metadata session is the session, closed ones left out).
func openRequests(dir, session string) ([]hooks.Request, error) {
	records, err := store.PathOf(dir)
	if err != nil {
		return nil, err
	}
	ws, err := OpenWork(store.MainOf(records))
	if err != nil {
		return nil, err
	}
	needs, err := ws.Needs(session)
	if err = errors.Join(err, ws.Shutdown()); err != nil {
		return nil, err
	}
	var out []hooks.Request
	for _, it := range needs {
		if it.Status != work.Closed {
			out = append(out, hooks.Request{ID: it.ID, Title: it.Title, Description: it.Description})
		}
	}
	return out, nil
}
