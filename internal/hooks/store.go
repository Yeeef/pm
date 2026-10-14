package hooks

import (
	"errors"

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
