// Command render-pages prints every page of the site, as the pm service renders it from the records and the work
// store, as one JSON object by path: what the harness's tests of the pages' HTML read (Repo.pages in
// tests/conftest.py), since pm writes no site to disk. Run it in a checkout whose clone's pm service is up; make
// go-build builds it with pm's version, which the service's handshake requires.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	here, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err := store.Find(here)
	if err != nil {
		return err
	}
	root, err := store.CodeRoot(here, dir)
	if err != nil {
		return err
	}
	ws, err := work.Dial(store.MainOf(dir))
	if err != nil {
		return err
	}
	items, err := ws.Items()
	if err = errors.Join(err, ws.Shutdown()); err != nil {
		return err
	}
	recs, err := records.Read(dir, nil)
	if err != nil {
		return err
	}
	summaries, err := records.ReadSummaries(dir)
	if err != nil {
		return err
	}
	dates, err := store.DesignDates(dir, recs, store.Today())
	if err != nil {
		return err
	}
	pages, err := site.RenderPages(recs, records.NewItems(items), filepath.Base(root), dates, summaries)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(pages)
}
