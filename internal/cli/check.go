package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// OpenWork connects to the clone's work store under the main checkout, through the pm service that holds it; Shutdown
// disconnects. A root id it mints takes the repo name as its prefix: the main checkout's directory name. Tests set a
// fake (worktest).
var OpenWork = func(main string) (work.Store, error) { return work.Dial(main) }

// cmdCheck is pm check: every record renders with the work store's items, writing nothing.
func cmdCheck(here string, stdout io.Writer) error {
	recordsDir, err := store.Find(here)
	if err != nil {
		return err
	}
	root, err := store.CodeRoot(here, recordsDir)
	if err != nil {
		return err
	}
	ws, err := OpenWork(store.MainOf(recordsDir))
	if err != nil {
		return err
	}
	all, err := ws.Items()
	if err = errors.Join(err, ws.Shutdown()); err != nil { // disconnect before the render, which needs no item more
		return err
	}
	recs, err := records.Read(recordsDir, nil)
	if err != nil {
		return err
	}
	summaries, err := records.ReadSummaries(recordsDir)
	if err != nil {
		return err
	}
	n, err := site.Check(recs, records.NewItems(all), filepath.Base(root), summaries)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "checked the records in %s: all %d pages render\n", recordsDir, n)
	return err
}
