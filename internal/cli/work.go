package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/hooks"
	"github.com/Yeeef/pm/internal/work"
)

// goOnly runs the commands Go pm has and Python pm does not: pm version, pm export [--store DIR], pm init --import-bd FILE,
// pm init --import FILE, and the work-store commands in store_commands.go. They stay out of the command tree in
// commands.go, which pm --help and pm prime's noun list read; pm init --import-bd and --import do the import only. ok is
// false for any other argv.
func goOnly(argv []string, stdin io.Reader, stdout io.Writer) (ok bool, err error) {
	if name, args, ok := storeCommandOf(argv); ok {
		return true, runStoreCommand(name, args, openStore, stdin, stdout)
	}
	switch {
	case len(argv) == 1 && argv[0] == "version": // this build's version, outside any repo too; dev when untagged
		_, err := fmt.Fprintln(stdout, buildinfo.Version)
		return true, err
	case len(argv) == 1 && argv[0] == "export":
		return true, cmdExport(stdout)
	case len(argv) == 3 && argv[0] == "export" && argv[1] == "--store":
		return true, exportStore(argv[2], stdout)
	case len(argv) == 3 && argv[0] == "init" && argv[1] == "--import-bd":
		return true, cmdImportBD(argv[2], stdout)
	case len(argv) == 2 && argv[0] == "init" && strings.HasPrefix(argv[1], "--import-bd="):
		return true, cmdImportBD(strings.TrimPrefix(argv[1], "--import-bd="), stdout)
	case len(argv) == 3 && argv[0] == "init" && argv[1] == "--import":
		return true, cmdImport(argv[2], stdout)
	case len(argv) == 2 && argv[0] == "init" && strings.HasPrefix(argv[1], "--import="):
		return true, cmdImport(strings.TrimPrefix(argv[1], "--import="), stdout)
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

// openStore connects to this clone's work store through its pm service, after the config check.
func openStore() (*work.Dolt, error) {
	main, err := mainCheckout()
	if err != nil {
		return nil, err
	}
	return work.Dial(main)
}

// cmdExport is pm export: every item of the work store, one JSON object per line, ordered by id.
func cmdExport(stdout io.Writer) error {
	main, err := mainCheckout()
	if err != nil {
		return err
	}
	dir, _ := work.Locations(main)
	return exportStore(dir, stdout)
}

// exportStore is pm export --store DIR: every item of the work store at DIR (<main checkout>/.pm/store/work), read
// without the repo's config, so the tests read the store as it is after a command that broke the config, through the
// service of the clone DIR belongs to (DIR/../../run/work.sock).
func exportStore(dir string, stdout io.Writer) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	main := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	d, err := work.DialSock(work.Sock(main), main, work.Options{Prefix: filepath.Base(main)})
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

// cmdImportBD is pm init --import-bd FILE: the bd export in FILE into this clone's work store (importItems). It
// refuses an export it cannot map.
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
	return importItems(main, items, "bd export", path, stdout)
}

// cmdImport is pm init --import FILE: the pm export in FILE (what pm export printed in another clone) into this
// clone's work store, ids kept (importItems). It moves a project's items into a new repo's store; it refuses a line
// that is not an item and items that fail the store's checks.
func cmdImport(path string, stdout io.Writer) error {
	main, err := mainCheckout()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	items, err := work.FromExport(f)
	f.Close()
	if err != nil {
		return err
	}
	return importItems(main, items, "pm export", path, stdout)
}

// importItems writes items, read from the export of this kind at path, into this clone's work store as one
// transaction and one Dolt commit, through the pm service; a clone with no store yet gets an empty one first. It
// refuses a store that holds any item.
func importItems(main string, items []work.Item, kind, path string, stdout io.Writer) error {
	d, err := work.DialSetup(main)
	if err != nil {
		return err
	}
	err = func() error {
		has, err := d.HasStore()
		if err != nil {
			return err
		}
		if !has {
			return d.CreateStore()
		}
		return d.UseStore()
	}()
	if err == nil {
		err = d.Import(items, fmt.Sprintf("pm: import %d items from %s %s", len(items), kind, filepath.Base(path)))
	}
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return err
	}
	comments := 0
	for _, it := range items {
		comments += len(it.Comments)
	}
	dir, _ := work.Locations(main)
	fmt.Fprintf(stdout, "imported %d items with %d comments from %s into %s\n", len(items), comments, path, dir)
	return nil
}
