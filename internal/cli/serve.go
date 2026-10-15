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
	"slices"
	"strings"
	gosync "sync"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/records"
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
	pages, err := newServedSite(records, filepath.Base(root), cfg.Remote+"/"+cfg.MainBranch)
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
		SiteURL: cfg.SiteURL, AccessTeam: cfg.AccessTeam, AccessAud: cfg.AccessAud, Port: port, Pin: cfg.Path, Spool: filepath.Join(common, service.SpoolName), WorkDir: dir,
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
	said, err := cmdDaySummarize(e, &Parsed{Values: map[string][]string{}})
	if err = errors.Join(err, e.release()); err != nil {
		return false, err.Error()
	}
	return true, said
}

// servedSite is internal/site as the pm service renders it (service.Site): the records published on the records
// branch, each draft beside them as a version, the remote's main branch and today's date as the stamp; each look's
// records and items checked and their pages rendered on demand; the reply forms and the status line filled on every
// page served.
type servedSite struct {
	records, name string
	main          string   // <remote>/<main branch>: the remote's main, whose commits to covered files a design page names
	heads         []string // the files whose bytes change whenever the store's HEAD commit or the main branch does
	common        string   // the clone's common git dir, whose worktrees/ names the records workspaces
	drafts        store.DraftSource
	mu            gosync.Mutex
	datesStamp    string // the stamp the cached design dates were read at
	dates         site.Dates
	// the store worktree's index.lock, which git holds while it rewrites the worktree (a checkout, a rebase); a git status
	// that refreshes the index takes it too, which costs a read under the records lock
	indexLock string
}

// newServedSite reads where the records store keeps its HEAD: the worktree's HEAD, the records branch's loose ref, the
// remote's main branch's (main, <remote>/<branch>) and packed-refs, read as files so a look runs no git. Its drafts
// are the store's uncommitted edits and each records workspace's (store.Clone).
func newServedSite(records, name, main string) (*servedSite, error) {
	out, err := git(records, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	dirs := strings.Split(out, "\n")
	if len(dirs) != 2 {
		return nil, fmt.Errorf("git rev-parse --git-dir --git-common-dir printed %q", out)
	}
	if !fsutil.IsDir(filepath.Join(dirs[1], "refs/heads")) {
		return nil, refuse("%s keeps refs in a format other than files (reftable?); the pm service reads refs as files",
			dirs[1])
	}
	return &servedSite{records: records, name: name, main: main, heads: []string{filepath.Join(dirs[0], "HEAD"),
		filepath.Join(dirs[1], "refs/heads", store.Branch), filepath.Join(dirs[1], "refs/remotes", main),
		filepath.Join(dirs[1], "packed-refs")}, common: dirs[1], drafts: store.Clone{Store: records},
		indexLock: filepath.Join(dirs[0], "index.lock")}, nil
}

// Stamp is a digest of every record file and day summary in the working trees of the store and of each records
// workspace (path and bytes), the store's HEAD files, the workspaces' branch refs and today's date: it moves with the
// published records and with every draft.
func (s *servedSite) Stamp() (string, error) {
	h := sha256.New()
	workspaces, refs := store.WorkspaceStamp(s.common)
	for _, root := range append([]string{s.records}, workspaces...) {
		if err := stampTree(h, root, root == s.records); err != nil {
			return "", err
		}
	}
	for _, p := range append(slices.Clone(s.heads), refs...) {
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

// stampTree adds each record file and day summary under root, path and bytes, to h; a workspace that went (its
// worktree removed) adds nothing, while the store must be there.
func stampTree(h io.Writer, root string, required bool) error {
	// A file or directory the walk listed can be gone when it is read: git rewrites a record by unlinking it, so a records
	// sync that rebases opens that window on every file it changes. One that went is not in the stamp; the next look sees
	// the files that replaced it.
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && (p != root || !required) {
			return nil
		}
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
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
		return nil
	})
}

// Load reads the published records from the records branch (store.Published) and checks them against items, and lists
// the drafts, which the site builds as versions on request. The design pages' dates and their covered code's changes
// come from git, read again only when the stamp moved. Drafts that moved while it read them (the stamp after the read
// not the stamp before, or git rewriting the worktree) make it a service.Partial.
func (s *servedSite) Load(items []work.Item) (service.Pages, error) {
	rewriting := fsutil.Exists(s.indexLock)
	stamp, err := s.Stamp() // before the read: a write between the two moves the stamp again, and the next look reads
	if err != nil {
		return nil, err
	}
	texts, sums, err := store.Published(s.records)
	if err != nil {
		return nil, err
	}
	recs, err := records.ParseAll(s.records, texts)
	if err != nil {
		return nil, err
	}
	summaries, err := records.ParseSummaries(sums)
	if err != nil {
		return nil, err
	}
	drafts := s.drafts.Drafts()
	site.LabelDrafts(drafts, items, store.MainOf(s.records))
	s.mu.Lock()
	dates := s.dates
	if s.datesStamp != stamp {
		if dates, err = store.DesignDates(s.records, recs); err == nil {
			err = store.CodeChanges(s.records, s.main, recs, dates)
		}
		if err != nil {
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
	if err := served.AddDrafts(s.records, texts, drafts); err != nil {
		return nil, err
	}
	// A records sync rewrote the worktree mid-read: the stamp moved, or git was rewriting the worktree, which a git that
	// stalled mid-checkout hides from both stamps (each walk misses the same unlinked file). The service reads again
	// under the records lock.
	if after, err := s.Stamp(); err != nil || after != stamp || rewriting || fsutil.Exists(s.indexLock) {
		return nil, &service.Partial{Pages: served}
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
