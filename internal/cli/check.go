package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// OpenWork connects to the clone's work store under the main checkout, through the pm service that holds it; Shutdown
// disconnects. A root id it mints takes the repo name as its prefix: the main checkout's directory name. Tests set a
// fake (worktest).
var OpenWork = func(main string) (work.Store, error) { return work.Dial(main) }

// cmdCheck is pm check: every record renders with the work store's items, and every design page's covers match a
// tracked file, writing nothing; it lists the tracked source files no page covers.
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
	if err := errors.Join(err, ws.Shutdown()); err != nil { // disconnect before the render, which needs no item more
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
	cfg, err := config.Read(here) // its pin is checked before any command runs
	if err != nil {
		return err
	}
	tracked, err := design.Tracked(store.MainOf(recordsDir), root)
	if err != nil {
		return err
	}
	uncovered, err := design.Check(recs, tracked, cfg.DesignSources)
	if err != nil {
		return err
	}
	out := fmt.Sprintf("checked the records in %s: all %d pages render", recordsDir, n)
	switch {
	case uncovered != nil:
		out += fmt.Sprintf("; %d tracked source files (design_sources in %s) no design page covers; add each to the "+
			"covers: of the page about its area:\n  %s", len(uncovered), config.Rel, strings.Join(uncovered, "\n  "))
	case cfg.DesignSources != nil:
		out += "; a design page covers every tracked source file"
	}
	_, err = fmt.Fprintln(stdout, out)
	return err
}
