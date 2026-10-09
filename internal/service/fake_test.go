package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/work"
)

// fakeWork is the work store's data, shared by every connection to it (a fakeStore), and the host's operations: its
// change mark, sync and gc.
type fakeWork struct {
	mu      gosync.Mutex
	items   map[string]*work.Item
	open    int // connections not yet shut down
	opens   int
	reads   int // Items calls
	syncs   int
	gcs     int
	fp      int // the change mark: moves on every write
	failGet error
	// syncWarnings are what each sync warns of: the claims its merge overrode
	syncWarnings []string
}

func newFakeWork(items ...work.Item) *fakeWork {
	w := &fakeWork{items: map[string]*work.Item{}}
	for i := range items {
		it := items[i]
		w.items[it.ID] = &it
	}
	return w
}

func (w *fakeWork) Open() (work.Store, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.open++
	w.opens++
	return &fakeStore{w: w}, nil
}

func (w *fakeWork) Mark() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return fmt.Sprint(w.fp), nil
}

func (w *fakeWork) Sync(ctx context.Context) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncs++
	return append([]string{"up to date"}, w.syncWarnings...), nil
}

func (w *fakeWork) GC(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gcs++
	return nil
}

func (w *fakeWork) item(id string) work.Item {
	w.mu.Lock()
	defer w.mu.Unlock()
	return *w.items[id]
}

func (w *fakeWork) set(it work.Item) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.items[it.ID] = &it
	w.fp++
}

type fakeStore struct {
	w      *fakeWork
	closed bool
}

func (s *fakeStore) lock() func() {
	s.w.mu.Lock()
	if s.closed {
		panic("fake work store: used after Shutdown")
	}
	return s.w.mu.Unlock
}

func (s *fakeStore) Items() ([]work.Item, error) {
	defer s.lock()()
	s.w.reads++
	var out []work.Item
	for _, it := range s.w.items {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *fakeStore) Get(ids ...string) ([]work.Item, error) {
	defer s.lock()()
	if s.w.failGet != nil {
		return nil, s.w.failGet
	}
	var out []work.Item
	for _, id := range ids {
		it, ok := s.w.items[id]
		if !ok {
			return nil, fmt.Errorf("no item %s", id)
		}
		out = append(out, *it)
	}
	return out, nil
}

func (s *fakeStore) Comment(id string, kind work.CommentKind, author, text string) (work.Comment, error) {
	defer s.lock()()
	it := s.w.items[id]
	c := work.Comment{ID: fmt.Sprintf("c%d", len(it.Comments)+1), Kind: kind, Author: author, Text: text,
		CreatedAt: time.Date(2026, 10, 8, 12, 0, len(it.Comments), 0, time.UTC)}
	it.Comments = append(it.Comments, c)
	s.w.fp++
	return c, nil
}

func (s *fakeStore) UpdateNeed(id string, u work.NeedUpdate) error {
	defer s.lock()()
	n := s.w.items[id].Need
	if u.Delivered != nil {
		n.Delivered = *u.Delivered
	}
	if u.ReviewMerged != nil {
		n.Review.Merged = *u.ReviewMerged
	}
	if u.ReviewMergeReported != nil {
		n.Review.MergeReported = *u.ReviewMergeReported
	}
	s.w.fp++
	return nil
}

func (s *fakeStore) Shutdown() error {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.w.open--
	}
	return nil
}

var errUnused = errors.New("not used by the service")

func (s *fakeStore) Needs(string) ([]work.Item, error)                   { return nil, errUnused }
func (s *fakeStore) Create(work.New) (work.Item, error)                  { return work.Item{}, errUnused }
func (s *fakeStore) Edit(string, *string, *string) error                 { return errUnused }
func (s *fakeStore) Close(string, string, work.Resolution, string) error { return errUnused }
func (s *fakeStore) SetResolution(string, work.Resolution) error         { return errUnused }
func (s *fakeStore) Move(string, string) error                           { return errUnused }
func (s *fakeStore) Claim(string, work.Holder, func(string) bool) error  { return errUnused }
func (s *fakeStore) Release(string, string) error                        { return errUnused }
func (s *fakeStore) DepAdd(string, string) error                         { return errUnused }
func (s *fakeStore) DepRemove(string, string) error                      { return errUnused }
func (s *fakeStore) Answer(string, string) error                         { return errUnused }

// fakeSite renders one page per item (items/<id>.html) and an index with a nav, a reply slot per open need and the
// status slot; FillReplies and FillStatus write what they were given, so a test reads it back.
type fakeSite struct {
	mu      gosync.Mutex
	stamp   int
	loadErr error
	loads   int
}

func (f *fakeSite) Stamp() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprint(f.stamp), nil
}

func (f *fakeSite) Load(items []work.Item) (Pages, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return fakePages(items), nil
}

func (f *fakeSite) FillReplies(page, token string, replies map[string]Reply) string {
	return slot.ReplaceAllStringFunc(page, func(m string) string {
		id := slot.FindStringSubmatch(m)[1]
		r := replies[id]
		return fmt.Sprintf("[form %s token=%s state=%s delivery=%s]", id, token, r.State, r.Delivery)
	})
}

func (f *fakeSite) FillStatus(page string, asOf time.Time, digest string, now time.Time) string {
	return strings.Replace(page, StatusSlot, "[status "+digest+"]", 1)
}

var slot = regexp.MustCompile(`<!--pm-reply (\S+) (decision|action)-->`)

type fakePages []work.Item

func (p fakePages) Page(path string) (string, bool, error) {
	if path == "index.html" {
		var b strings.Builder
		b.WriteString("<nav>home</nav>" + StatusSlot)
		for _, it := range p {
			if it.Type == work.Need && it.Status == work.Open {
				fmt.Fprintf(&b, "<!--pm-reply %s %s-->", it.ID, kindWord(&it))
			}
		}
		return b.String(), true, nil
	}
	if path == "broken.html" {
		return "", false, errors.New("the page broke")
	}
	for _, it := range p {
		if path == "items/"+it.ID+".html" {
			return "<h1>" + it.Title + "</h1>" + StatusSlot, true, nil
		}
	}
	return "", false, nil
}

// syncBuffer is an io.Writer tests read while the service writes.
type syncBuffer struct {
	mu gosync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestMain runs the test binary as a pm service with the fake site and store when the fake supervisor starts it
// (PM_TEST_SERVICE names its main checkout), so the lifecycle tests install, probe and restart a real process.
func TestMain(m *testing.M) {
	if main := os.Getenv("PM_TEST_SERVICE"); main != "" {
		os.Exit(helperService(main))
	}
	os.Exit(m.Run())
}

func helperService(main string) int {
	w := newFakeWork()
	var port int
	fmt.Sscan(os.Getenv("PORT"), &port)
	pin := filepath.Join(main, ".pm/config.toml")
	err := Run(Deps{Main: main, Records: store(main), Remote: "origin", MainBranch: "main", Port: port, Pin: pin,
		Spool: filepath.Join(main, ".git", SpoolName), WorkDir: main, Open: w.Open, Mark: w.Mark, Sync: w.Sync, GC: w.GC,
		Site: &fakeSite{}, Summarize: func() (bool, string) { return true, "nothing to summarize" },
		Style: []byte("body{}"), Out: os.Stdout, Log: os.Stderr})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

var _ io.Writer = (*syncBuffer)(nil)
