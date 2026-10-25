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

// cmdCheck is pm check: every record renders with the work store's items, writing nothing; it lists the tracked source
// files no page covers (design.Report), and fails on a design page's covers pattern that matches no file where the
// page holds it: a published page's on the remote's main, a draft page's (in a worktree's records workspace, whose
// drafts it checks over the published records) in the worktree's tracked files. It names each project record whose
// title header the work store does not hold (records.TitleDrift), with the pm project edit that fixes it; like the
// uncovered files, that is a report, not a failure, so pm commit, which runs site.Check alone, never refuses on it.
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
	recs, drafts, err := store.Working(here, recordsDir)
	if err != nil {
		return err
	}
	summaries, err := records.ReadSummaries(recordsDir)
	if err != nil {
		return err
	}
	x := records.NewItems(all)
	n, err := site.Check(recs, x, filepath.Base(root), summaries)
	if err != nil {
		return err
	}
	drift := ""
	if d := records.TitleDrift(recs, x); d != nil {
		drift = fmt.Sprintf("; %d project records whose title the work store does not hold; run each fix:\n  %s", len(d),
			strings.Join(d, "\n  "))
	}
	cfg, err := config.Read(here) // its pin is checked before any command runs
	if err != nil {
		return err
	}
	tracked, err := design.Tracked(store.MainOf(recordsDir), root)
	if err != nil {
		return err
	}
	report, err := design.Report(recs, tracked, cfg.DesignSources, config.Rel)
	if err != nil {
		return err
	}
	waiting, err := design.Hold(recs, drafts, recordsDir, root, cfg.Remote, cfg.MainBranch)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "checked the records in %s: all %d pages render%s%s%s\n", recordsDir, n, drift, report,
		waiting)
	return err
}
