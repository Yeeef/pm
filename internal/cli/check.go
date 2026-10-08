package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Yeeef/yeeef-agents/pm/internal/records"
	"github.com/Yeeef/yeeef-agents/pm/internal/site"
	"github.com/Yeeef/yeeef-agents/pm/internal/store"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// OpenWork opens the clone's work store; root is the main checkout. Go pm has no work store until port sprint P3
// lands it, so until then a command that reads items refuses; tests set a fake.
var OpenWork = func(root string) (work.Store, error) {
	return nil, errors.New("Go pm has no work store yet (port sprint P3); Python pm runs this command until the cut-over")
}

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
	if err != nil {
		return err
	}
	if err := ws.Shutdown(); err != nil {
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
