package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/served"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The pm service wired to the clone: pm service run with the work store, the records and the site, and pm service
// install of a unit that runs the installed pm.

// serviceRun is pm service run: the work store held open and served on its socket (work.NewHost; the only call
// of it), then service.Run on this clone's config, records store, that store and the site.
func serviceRun(cfg config.Config, here, main, records string, stdout, stderr io.Writer) error {
	port, err := service.PortFor(main, int(cfg.Port))
	if err != nil {
		return err
	}
	root, err := store.CodeRoot(here, records)
	if err != nil {
		return err
	}
	common, err := git(records, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	pages, err := served.New(records, filepath.Base(root), cfg.Remote+"/"+cfg.MainBranch)
	if err != nil {
		return err
	}
	h, err := work.NewHost(work.HostOptions{Main: main, Version: buildinfo.Version, Ops: work.Ops{
		Sync: func(c context.Context, d *work.Dolt) ([]string, error) {
			return install.SyncStore(c, d, main, cfg.Remote)
		},
		Setup: func(c context.Context, d *work.Dolt) ([]string, error) {
			return install.SetupStore(c, d, main, cfg.Remote)
		},
	}})
	if err != nil {
		return err
	}
	defer h.Close()
	dir, _ := work.Locations(main)
	return service.Run(service.Deps{Main: main, Records: records, Remote: cfg.Remote, MainBranch: cfg.MainBranch,
		SiteURL: cfg.SiteURL, Listen: cfg.ListenAddr(), Access: cfg.Access, AccessTeam: cfg.AccessTeam, AccessAud: cfg.AccessAud, Port: port, Pin: cfg.Path, Spool: filepath.Join(common, service.SpoolName), WorkDir: dir,
		Open: func() (work.Store, error) {
			return work.DialSock(h.Sock(), main, work.Options{Prefix: filepath.Base(main)})
		},
		Mark: h.Mark, Sync: h.Sync, GC: h.GC,
		Site: pages, Summarize: func() (bool, string) { return summarizeDay(main) },
		Style: []byte(site.Style), Out: stdout, Log: stderr})
}

// serviceInstall is pm service install: the unit runs the pm installed at config.BinPath(), which pm init puts there;
// refused when there is none.
func serviceInstall(cfg config.Config, main string) (string, error) {
	exe, err := service.Exe()
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(exe); err != nil || !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
		return "", refuse("no pm is installed at %s, which the pm service runs; run pm init, which installs it there", exe)
	}
	port, err := service.PortFor(main, int(cfg.Port))
	if err != nil {
		return "", err
	}
	said, err := service.Install(main, port, exe, false)
	if err != nil || said != "" {
		return said, err
	}
	_, line := service.Health(main)
	return "already installed and current\n" + line, nil
}

// summarizeDay is the sync's summary step: pm day summarize from the main checkout, ok and what it did, or why not.
func summarizeDay(main string) (bool, string) {
	records, err := store.Find(main)
	if err != nil {
		return false, err.Error()
	}
	e := &env{here: main, records: records, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}
	said, err := cmdDaySummarize(e, &Parsed{Values: map[string][]string{}})
	if err = errors.Join(err, e.release()); err != nil {
		return false, err.Error()
	}
	return true, said
}
