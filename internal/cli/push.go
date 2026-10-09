package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	pmsync "github.com/Yeeef/pm/internal/sync"
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
		{Name: "work", Run: func() (bool, string) {
			st, err := openServiceStore(main, cfg.Remote)
			if err != nil {
				return false, err.Error()
			}
			ctx, cancel := context.WithTimeout(context.Background(), service.SyncTimeout)
			defer cancel()
			said, w, err := st.Sync(ctx)
			warnings = w
			if err = errors.Join(err, st.Shutdown()); err != nil {
				return false, err.Error()
			}
			return true, said
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
