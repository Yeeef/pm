// Package served is internal/site as the pm service serves it (service.Site): the records store's published records
// and drafts, read as files under a stamp that moves with them, rendered on demand.
package served

import (
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

	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// Site is internal/site as the pm service renders it (service.Site): the records published on the records
// branch, each draft beside them as a version, the remote's main branch and today's date as the stamp; each look's
// records and items checked and their pages rendered on demand; the reply forms and the status line filled on every
// page served.
type Site struct {
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

// New reads where the records store keeps its HEAD: the worktree's HEAD, the records branch's loose ref, the
// remote's main branch's (main, <remote>/<branch>) and packed-refs, read as files so a look runs no git. Its drafts
// are the store's uncommitted edits and each records workspace's (store.Clone).
func New(records, name, main string) (*Site, error) {
	out, err := store.Git(records, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	dirs := strings.Split(out, "\n")
	if len(dirs) != 2 {
		return nil, fmt.Errorf("git rev-parse --git-dir --git-common-dir printed %q", out)
	}
	if !fsutil.IsDir(filepath.Join(dirs[1], "refs/heads")) {
		return nil, fmt.Errorf("%s keeps refs in a format other than files (reftable?); the pm service reads refs as files",
			dirs[1])
	}
	return &Site{records: records, name: name, main: main, heads: []string{filepath.Join(dirs[0], "HEAD"),
		filepath.Join(dirs[1], "refs/heads", store.Branch), filepath.Join(dirs[1], "refs/remotes", main),
		filepath.Join(dirs[1], "packed-refs")}, common: dirs[1], drafts: store.Clone{Store: records},
		indexLock: filepath.Join(dirs[0], "index.lock")}, nil
}

// Stamp is a digest of every record file and day summary in the working trees of the store and of each records
// workspace (path and bytes), the store's HEAD files, the workspaces' branch refs, the other clones' fetched draft refs
// and today's date: it moves with the published records and with every draft.
func (s *Site) Stamp() (string, error) {
	h := sha256.New()
	workspaces, refs := store.WorkspaceStamp(s.common)
	refs = append(refs, store.FetchedStamp(s.common)...)
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
		_, err = h.Write(data)
		return err
	})
}

// Load reads the published records from the records branch (store.Published) and checks them against items, and lists
// the drafts, which the site builds as versions on request. The design pages' dates and their covered code's changes
// come from git, read again only when the stamp moved. Drafts that moved while it read them (the stamp after the read
// not the stamp before, or git rewriting the worktree) make it a service.Partial.
func (s *Site) Load(items []work.Item) (service.Pages, error) {
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

func (s *Site) FillReplies(page, token string, replies map[string]service.Reply) string {
	shown := make(map[string]site.Reply, len(replies))
	for id, r := range replies {
		shown[id] = site.Reply(r)
	}
	return site.FillReplies(page, token, shown)
}

func (s *Site) FillStatus(page string, asOf time.Time, digest string, now time.Time) string {
	return site.FillStatus(page, asOf, digest, now)
}

var _ service.Site = (*Site)(nil)
