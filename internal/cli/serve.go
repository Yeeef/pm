package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The pm service wired to the clone: pm service run with the work store, the records and the site, and pm service
// install of a unit that runs the installed pm. Python source: cmd_serve and cmd_service_* in cli.py.

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
	pages, err := newServedSite(records, filepath.Base(root))
	if err != nil {
		return err
	}
	h, err := work.NewHost(work.HostOptions{Main: main, Version: buildinfo.Version, Ops: work.Ops{
		Sync: func(c context.Context, d *work.Dolt) ([]string, error) { return syncStore(c, d, main, cfg.Remote) },
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
		SiteURL: cfg.SiteURL, Port: port, Pin: cfg.Path, Spool: filepath.Join(common, service.SpoolName), WorkDir: dir,
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
	said, err := service.Install(main, port, exe)
	if err != nil || said != "" {
		return said, err
	}
	_, line := service.Health(main)
	return "already installed and current\n" + line, nil
}

// syncStore is the sync the pm service runs (every 600 s, and for pm_sync(): pm sync, pm push), on a connection of
// its own: pull, merge, push, said in one line, then a warning per claim the merge overrode. A store with no remote yet
// (the repo's remote was added after pm init made the store) is first attached to the config's remote, under
// work.RemoteRef, and the line says so; refused when the remote holds a work store already, which a store made apart
// from it shares no history with. The sync stops once ctx is done; a ctx done already starts none, since Dolt's first
// reach of a remote, killed, leaves a remote cache no sync repairs.
func syncStore(ctx context.Context, d *work.Dolt, main, remote string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	attached := ""
	if _, ok, err := d.Remote(); err != nil {
		return nil, err
	} else if !ok {
		url, err := git(main, "remote", "get-url", remote)
		if err != nil {
			return nil, fmt.Errorf("work store: it has no remote to sync with, and %v", err)
		}
		held, err := install.RemoteHasStore(ctx, main, remote) // within the sync's bound
		if err != nil {
			return nil, fmt.Errorf("work store: it has no remote to sync with, and %v", err)
		}
		if held {
			return nil, fmt.Errorf("work store: it has no remote, and %s holds a work store under %s already, "+
				"which this one, made apart from it, shares no history with; move %s away and run pm init, which "+
				"clones it", remote, work.RemoteRef, filepath.Join(main, ".pm/store/work"))
		}
		if err := d.AddRemote(url); err != nil {
			return nil, err
		}
		attached = fmt.Sprintf("attached the work store to %s under %s; ", remote, work.RemoteRef)
	}
	r, err := d.SyncContext(ctx)
	if err != nil {
		return nil, err
	}
	lines := []string{attached + fmt.Sprintf("synced: pulled %d commits, resolved %d items both sides changed, "+
		"pushed %d commits", r.Pulled, r.Resolved, r.Pushed)}
	for _, o := range r.Overrides {
		lines = append(lines, fmt.Sprintf("warning: %s: the claim by %s (%s) was overridden by the later claim "+
			"of %s (%s)", o.ID, o.Lost.Session, o.Lost.ClaimedAt.Format(time.RFC3339), o.Kept.Session,
			o.Kept.ClaimedAt.Format(time.RFC3339)))
	}
	return lines, nil
}

// summarizeDay is the sync's summary step: pm day summarize from the main checkout, ok and what it did, or why not.
func summarizeDay(main string) (bool, string) {
	records, err := store.Find(main)
	if err != nil {
		return false, err.Error()
	}
	e := &env{here: main, records: records, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}
	said, err := cmdDaySummarize(e, &Parsed{values: map[string][]string{}})
	if err = errors.Join(err, e.release()); err != nil {
		return false, err.Error()
	}
	return true, said
}

// servedSite is internal/site as the pm service renders it (service.Site): the records store's files, its HEAD and
// today's date as the stamp; each look's records and items checked and their pages rendered on demand; the reply forms
// and the status line filled on every page served.
type servedSite struct {
	records, name string
	heads         []string // the files whose bytes change whenever the store's HEAD commit does
	mu            gosync.Mutex
	datesStamp    string // the stamp the cached design dates were read at
	dates         site.Dates
}

// newServedSite reads where the records store keeps its HEAD: the worktree's HEAD, the records branch's loose ref and
// packed-refs, read as files so a look runs no git (store.py's head_files).
func newServedSite(records, name string) (*servedSite, error) {
	out, err := git(records, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	dirs := strings.Split(out, "\n")
	if len(dirs) != 2 {
		return nil, fmt.Errorf("git rev-parse --git-dir --git-common-dir printed %q", out)
	}
	if !isDir(filepath.Join(dirs[1], "refs/heads")) {
		return nil, refuse("%s keeps refs in a format other than files (reftable?); the pm service reads refs as files",
			dirs[1])
	}
	return &servedSite{records: records, name: name, heads: []string{filepath.Join(dirs[0], "HEAD"),
		filepath.Join(dirs[1], "refs/heads", store.Branch), filepath.Join(dirs[1], "packed-refs")}}, nil
}

// Stamp is a digest of every record file and day summary (path and bytes), the store's HEAD files and today's date.
func (s *servedSite) Stamp() (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(s.records, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() || !(strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".summary.json")) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, p := range s.heads {
		data, err := os.ReadFile(p)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
	}
	h.Write([]byte(store.Today()))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Load reads the records as they are on disk, uncommitted edits included, and checks them against items. The design
// pages' dates come from git, read again only when the stamp moved.
func (s *servedSite) Load(items []work.Item) (service.Pages, error) {
	stamp, err := s.Stamp() // before the read: a write between the two moves the stamp again, and the next look reads
	if err != nil {
		return nil, err
	}
	recs, err := records.Read(s.records, nil)
	if err != nil {
		return nil, err
	}
	summaries, err := records.ReadSummaries(s.records)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	dates := s.dates
	if s.datesStamp != stamp {
		if dates, err = store.DesignDates(s.records, recs, store.Today()); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.dates, s.datesStamp = dates, stamp
	}
	s.mu.Unlock()
	served, err := site.Serve(recs, records.NewItems(items), s.name, dates, summaries)
	if err != nil {
		return nil, err
	}
	return served, nil
}

func (s *servedSite) FillReplies(page, token string, replies map[string]service.Reply) string {
	shown := make(map[string]site.Reply, len(replies))
	for id, r := range replies {
		shown[id] = site.Reply(r)
	}
	return site.FillReplies(page, token, shown)
}

func (s *servedSite) FillStatus(page string, asOf time.Time, digest string, now time.Time) string {
	return site.FillStatus(page, asOf, digest, now)
}

var _ service.Site = (*servedSite)(nil)
