package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/hooks"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// goOnly runs the commands Go pm has and Python pm does not: pm export, and pm init --import-bd FILE. The argparse
// tree mirrors Python pm's, whose help texts and pm prime noun list the parity tests hold equal, so these stay out of
// it until Python pm has them or the cut-over. In Go pm today, pm init --import-bd does the import only; the rest of
// pm init comes with the install sprint. ok is false for any other argv.
func goOnly(argv []string, stdout io.Writer) (ok bool, err error) {
	switch {
	case len(argv) == 1 && argv[0] == "export":
		return true, cmdExport(stdout)
	case len(argv) == 3 && argv[0] == "init" && argv[1] == "--import-bd":
		return true, cmdImportBD(argv[2], stdout)
	case len(argv) == 2 && argv[0] == "init" && strings.HasPrefix(argv[1], "--import-bd="):
		return true, cmdImportBD(strings.TrimPrefix(argv[1], "--import-bd="), stdout)
	}
	return false, nil
}

// mainCheckout is the main checkout of the clone containing here, after the config check every command passes.
func mainCheckout() (string, error) {
	here, err := cwd()
	if err != nil {
		return "", err
	}
	if _, err := config.Load(here); err != nil {
		return "", err
	}
	records, perr := hooks.StoreOf(&here)
	if perr != nil {
		return "", perr
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(records))), nil // <main>/.pm/store/records
}

// cmdExport is pm export: every item of the work store, one JSON object per line, ordered by id.
func cmdExport(stdout io.Writer) error {
	main, err := mainCheckout()
	if err != nil {
		return err
	}
	dir, run := work.Locations(main)
	d, err := work.OpenStore(work.Options{Dir: dir, RunDir: run})
	if err != nil {
		return err
	}
	items, err := d.Items()
	err = errors.Join(err, d.Shutdown())
	if err != nil {
		return err
	}
	return work.Export(stdout, items)
}

// cmdImportBD is pm init --import-bd FILE: the bd export in FILE into this clone's work store, as one transaction and
// one Dolt commit. It refuses a store that holds any item, and an export it cannot map.
func cmdImportBD(path string, stdout io.Writer) error {
	main, err := mainCheckout()
	if err != nil {
		return err
	}
	recs, err := work.ReadBDRecords(filepath.Join(main, config.Store))
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("bd import: %w", err)
	}
	items, err := work.FromBD(f, recs)
	f.Close()
	if err != nil {
		return err
	}
	dir, run := work.Locations(main)
	o := work.Options{Dir: dir, RunDir: run}
	open := work.CreateStore
	if work.Exists(dir) {
		open = work.OpenStore
	}
	d, err := open(o)
	if err != nil {
		return err
	}
	err = d.Import(items, fmt.Sprintf("pm: import %d items from bd export %s", len(items), filepath.Base(path)))
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return err
	}
	comments := 0
	for _, it := range items {
		comments += len(it.Comments)
	}
	fmt.Fprintf(stdout, "imported %d items with %d comments from %s into %s\n", len(items), comments, path, dir)
	return nil
}
