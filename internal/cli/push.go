package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/store"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
)

// cmdPush is pm push: one run of the steps the pm service syncs every 10 minutes (the work store's sync, today's
// summary, the records push), each step's line printed and its outcome kept in the push state; exit code 1 when a
// step failed. It finds the records store itself, so a missing one is the records step's recorded failure.
func cmdPush(cfg config.Config, here string, stdout io.Writer) error {
	recordsPath, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(recordsPath)
	var warnings []string
	steps := []pmsync.Step{
		{Name: "work", Run: func() (bool, string) { // the service syncs: CALL pm_sync()
			d, err := work.Dial(main)
			if err != nil {
				return false, err.Error()
			}
			lines, err := d.CallSync()
			if err = errors.Join(err, d.Shutdown()); err != nil {
				return false, err.Error()
			}
			warnings = lines[1:]
			return true, lines[0]
		}},
		{Name: "summary", Run: func() (bool, string) { return summarizeDay(main) }},
		{Name: "records", Run: func() (bool, string) {
			records, err := store.Find(here)
			if err != nil {
				return false, err.Error()
			}
			return pmsync.PushRecords(records, cfg.Remote)
		}},
	}
	code, out, err := pmsync.Push(main, steps)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, out)
	for _, w := range warnings {
		fmt.Fprintln(stdout, w)
	}
	if code != 0 {
		return &exitCode{code}
	}
	return nil
}
