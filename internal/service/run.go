package service

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	gosync "sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/pyjson"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
)

var (
	ServeCheck  = time.Second // between the service's looks for a change in the records or the work store
	ServeBehind = 10          // seconds a page may be behind before it says so (the site states it)
)

// StatusSlot is the slot on every page that Site.FillStatus fills with the data's age and the reload script.
const StatusSlot = "<!--pm-status-->"

// Site is the site renderer as the service uses it; internal/site implements it.
type Site interface {
	// Stamp is a cheap fingerprint of what the pages read besides the work store: the record files, the records
	// store's HEAD and today's date. Equal stamps mean Load would read the same records.
	Stamp() (string, error)
	// Load reads the records as of now and checks them against items; its error is served instead of every page.
	Load(items []work.Item) (Pages, error)
	// FillReplies replaces each awaiting card's reply slot with its form, carrying token for the POST, and shows the
	// replies still on their way to the work store, by need id.
	FillReplies(page, token string, replies map[string]Reply) string
	// FillStatus fills the page's StatusSlot: the time its data is current as of, its digest (which the page's
	// reload script compares with /version) and now.
	FillStatus(page string, asOf time.Time, digest string, now time.Time) string
}

// Pages renders the pages of one Load.
type Pages interface {
	// Page is the page at path (index.html, sprints/x.html); found false when the site has no such page.
	Page(path string) (html string, found bool, err error)
}

// Reply is a site reply on its way to the work store, as its card shows it: State saving, sent or failed; Delivery
// for a sent one (delivered, nothing to deliver, not running, failed); Error for a failed one.
type Reply struct{ State, Text, RID, Error, Delivery string }

// Deps is what the service runs on. Every field is required.
type Deps struct {
	Main, Records      string // the main checkout, and the records store (named in StoreHeader)
	Remote, MainBranch string // the config's
	SiteURL            string // the config's site_url, "" when unset
	Port               int    // 0: a free one
	Pin                string // the config file, read again at every look: a pin moved off Version() stops the service
	Spool              string // the reply spool, in the clone's git dir
	WorkDir            string // the work store's directory, whose size the gc log line gives
	// Open connects to the work store the service holds, through its own socket, for one poll or write.
	Open func() (work.Store, error)
	// Mark is the work store's change mark, main's HEAD commit (HASHOF('main')): it moves on every write; "" when
	// the clone has no store yet.
	Mark func() (string, error)
	// Sync fetches the remote's work data, merges it and pushes what the remote lacks (the work-store page's Sync),
	// under the host's operation mutex: one line saying what it did, then a warning line for each claim the merge
	// overrode, which the service logs.
	Sync func(ctx context.Context) ([]string, error)
	// GC collects the store's garbage (CALL DOLT_GC()), online: it deletes no item and squashes no commit.
	GC        func(ctx context.Context) error
	Site      Site
	Summarize func() (bool, string) // the day summary, the sync's second step
	Style     []byte
	Out, Log  io.Writer // "Serving …" goes to Out; every log line to Log
}

// snapshot is what the service serves: the records and work items read at asOf or later, so current as of then; err
// when they do not render. cache holds each page rendered from them.
type snapshot struct {
	asOf       time.Time
	stamp, fp  string
	items      map[string]*work.Item
	order      []*work.Item
	pages      Pages
	err        string
	readFailed bool // err is a failure to stamp, open or read, not a render error
	mu         *gosync.Mutex
	cache      map[string]rendered
}

type rendered struct {
	code int
	text string
	none bool // no such page
}

type job struct {
	entry *Entry
	tries int
	merge string // the merge commit, for a merge of need id
	id    string
	sweep bool
}

type sweepState struct {
	got string
	key string // the inbox found not running: path, inode and mtime; "" to look again
}

type server struct {
	d                     Deps
	texts                 Texts
	token                 string
	snap                  atomic.Pointer[snapshot]
	mu                    gosync.Mutex
	replies               map[string]Reply
	merges                map[string]bool
	swept                 map[string]sweepState
	queue                 chan job
	wake                  chan struct{}
	stopped               chan string
	done                  chan struct{} // closed when Run returns: every loop ends
	serveCheck, mergePoll time.Duration // ServeCheck and MergePoll as Run started
	logMu                 gosync.Mutex
	gh                    bool
}

func (s *server) logf(format string, a ...any) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	fmt.Fprintf(s.d.Log, format+"\n", a...)
}

// withStore connects to the work store, runs fn and disconnects; a slow one is logged with how long it took.
func (s *server) withStore(what string, fn func(work.Store) error) error {
	t := time.Now()
	st, err := s.d.Open()
	if err != nil {
		return fmt.Errorf("reaching the work store for %s: %w", what, err)
	}
	err = fn(st)
	if cerr := st.Shutdown(); err == nil {
		err = cerr
	}
	if took := time.Since(t); took > time.Second {
		s.logf("store %s took=%dms", what, took.Milliseconds())
	}
	return err
}

// sleep waits d; false when Run returned meanwhile, so the loop ends.
func (s *server) sleep(d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-s.done:
		return false
	}
}

func (s *server) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run is pm service run: serve the site on 127.0.0.1:Port and sync every pmsync.Interval, the first that long after
// start. A request never waits on a store: it renders from the last snapshot, which the refresher keeps current. Each
// look (every ServeCheck, sooner after a request or a reply) stamps the records and reads the work store's change
// mark, and only when one moved reads the items; a state that fails to render is read again
// under the records store's shared lock, and then served as the error, never as an old page. A reply is checked
// against the snapshot, spooled before the POST is answered, and stored by the writer, which retries a failed write
// with backoff and then pushes the reply into the raising session's inbox. The merge watch, at start and every
// MergePoll, hands reviews' merges to main to the writer, then has it sweep the open needs for anything their running
// session has not received. Every look reads the pin again: once it pins another version, Run returns that error.
func Run(d Deps) error {
	if d.Open == nil || d.Mark == nil || d.Sync == nil || d.GC == nil || d.Site == nil || d.Summarize == nil || d.Out == nil || d.Log == nil ||
		d.Main == "" || d.Records == "" || d.Spool == "" || d.Pin == "" || d.WorkDir == "" {
		return errors.New("pm service run: a dependency is missing")
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return err
	}
	s := &server{d: d, texts: Texts{d.Main, d.Remote, d.MainBranch}, token: base64.RawURLEncoding.EncodeToString(tok),
		replies: map[string]Reply{}, merges: map[string]bool{}, swept: map[string]sweepState{},
		queue: make(chan job, 1024), wake: make(chan struct{}, 1), stopped: make(chan string, 1),
		done: make(chan struct{}), serveCheck: ServeCheck, mergePoll: MergePoll}
	defer close(s.done)
	_, err := exec.LookPath("gh")
	s.gh = err == nil
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", d.Port))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 30 * time.Second}
	s.snap.Store(s.refresh(nil)) // the first load has data to serve
	pending, err := Pending(d.Spool)
	if err != nil {
		ln.Close()
		return err
	}
	go s.writer()               // before the pending replies are queued, so any number of them fits
	for _, e := range pending { // replies a crash or kill left pending
		s.mu.Lock()
		s.replies[e.ID] = Reply{State: "saving", Text: e.Text, RID: e.RID}
		s.mu.Unlock()
		s.put(job{entry: &e, id: e.ID})
	}
	if !s.gh {
		s.logf("note: gh is not installed, so the pm service does not watch reviews' PRs for their merge")
	}
	go s.refresher(srv)
	go s.ticker()
	go s.syncer()
	go s.collector()
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(d.Out, "Serving http://localhost:%d; each page states the age of its data, at most %d s behind unless "+
		"it says so; the work store reread when it changes; pushing every %d min\n", port, ServeBehind,
		int(pmsync.Interval.Minutes()))
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	select {
	case why := <-s.stopped:
		return &Error{why}
	default:
		return nil // replies still pending stay in the spool for the next start
	}
}

// put queues a job for the writer; it never blocks a request (the queue holds 1,024, and a full one drops a sweep).
func (s *server) put(j job) {
	if j.sweep {
		select {
		case s.queue <- j:
		default:
		}
		return
	}
	s.queue <- j
}

// pinMoved is why the service must stop, its config no longer pinning the version it runs; "" while it does.
func pinMoved(path string) string {
	var c struct {
		Version any `toml:"version"`
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return fmt.Sprintf("the pm service cannot read its pin in %s (%v); stopping", path, err)
	}
	pin := fmt.Sprint(c.Version)
	if c.Version == nil {
		pin = "None"
	}
	if pin != Version() {
		return fmt.Sprintf("%s now pins pm %s, but this service runs pm %s; stopping, so the supervisor starts it "+
			"again and the launcher runs pm %s", path, pin, Version(), pin)
	}
	return ""
}

func (s *server) refresher(srv *http.Server) {
	for {
		select {
		case <-s.wake:
		case <-time.After(s.serveCheck):
		case <-s.done:
			return
		}
		if moved := pinMoved(s.d.Pin); moved != "" { // the supervisor starts it again, on the pinned version
			s.stopped <- moved
			s.logf("error: %s", moved)
			_ = srv.Shutdown(context.Background())
			return
		}
		s.snap.Store(s.refresh(s.snap.Load()))
	}
}

// refresh is old with a newer asOf when neither the records nor the work store moved since it was read, else a new
// read. The read takes no lock, so a writer never delays it; between a pm write's steps the records and the items
// may not render together (a need closed before its decision is written), so a read that fails is repeated under
// the records store's shared lock, which a pm write holds exclusively across its steps: an error shown is a real one.
// A failure to stamp, reach or read the store is not a render error: it is served, and the next look tries again
// even when nothing moved.
func (s *server) refresh(old *snapshot) *snapshot {
	now := time.Now()
	stamp, serr := s.d.Site.Stamp()
	fp, ferr := s.d.Mark()
	if serr == nil && ferr == nil && old != nil && !old.readFailed && stamp == old.stamp && fp == old.fp {
		next := *old
		next.asOf = now
		return &next
	}
	start := time.Now()
	snap := &snapshot{asOf: now, stamp: stamp, fp: fp, mu: &gosync.Mutex{}, cache: map[string]rendered{}}
	fail := func(err error) *snapshot {
		snap.err, snap.readFailed = "error: "+err.Error(), true
		s.logf("refresh total=%dms failed: %v", time.Since(start).Milliseconds(), err)
		return snap
	}
	if err := errors.Join(serr, ferr); err != nil {
		return fail(err)
	}
	var items []work.Item
	var err error
	if old != nil && old.err == "" && fp == old.fp { // only the records moved
		for _, it := range old.order {
			items = append(items, *it)
		}
	} else if err = s.withStore("snapshot", func(st work.Store) error {
		items, err = st.Items()
		return err
	}); err != nil {
		return fail(err)
	}
	pages, err := s.d.Site.Load(items)
	var lock time.Duration
	if err != nil {
		err = s.withStore("snapshot under the records lock", func(st work.Store) error {
			fd, err := syscall.Open(s.d.Records, syscall.O_RDONLY, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)
			t := time.Now()
			if err := syscall.Flock(fd, syscall.LOCK_SH); err != nil {
				return err
			}
			lock = time.Since(t)
			if items, err = st.Items(); err != nil {
				return err
			}
			pages, err = s.d.Site.Load(items)
			if err != nil {
				snap.err = "error: " + err.Error() // a render error: served until the records or the store move
			}
			return nil
		})
		if err != nil {
			return fail(err)
		}
	}
	snap.items = map[string]*work.Item{}
	for i := range items {
		snap.items[items[i].ID] = &items[i]
		snap.order = append(snap.order, &items[i])
	}
	if snap.err == "" {
		snap.pages = pages
	}
	s.logf("refresh total=%dms lock=%dms", time.Since(start).Milliseconds(), lock.Milliseconds())
	return snap
}

func errorPage(title, heading, body string) string {
	return `<!doctype html><meta charset="utf-8"><title>` + title + `</title><link rel="stylesheet" href="/style.css">` +
		"<main>" + StatusSlot + "<h1>" + heading + "</h1>" + body + "</main>"
}

// page is the status and HTML of the page at path from the current snapshot, its reply slots filled and its status
// slot left for FillStatus, and that snapshot.
func (s *server) page(path string) (int, string, *snapshot) {
	snap := s.snap.Load()
	if snap.err != "" {
		return 500, errorPage("Render failed", "The site did not render", "<pre>"+html.EscapeString(snap.err)+
			"</pre><p>Fix the cause; this page offers a reload once it renders.</p>"), snap
	}
	snap.mu.Lock()
	r, ok := snap.cache[path]
	if !ok {
		text, found, err := snap.pages.Page(path)
		switch {
		case err != nil:
			r = rendered{code: 500, text: errorPage("Render failed", "The page did not render", "<pre>"+
				html.EscapeString("error: "+err.Error())+"</pre><p>Fix the cause; this page offers a reload once it "+
				"renders.</p>")}
		case !found:
			r = rendered{none: true}
		default:
			r = rendered{code: 200, text: text}
		}
		snap.cache[path] = r
	}
	snap.mu.Unlock()
	if r.none { // a page that does not exist yet offers a reload once a newer snapshot has it
		return 404, errorPage("No such page", "No page "+html.EscapeString(path),
			"<p>This page offers a reload once the site has it.</p>"), snap
	}
	text := r.text
	// the push banner is read on every request: the sync changes it, not the records
	if path == "index.html" || strings.HasPrefix(path, "projects/") {
		banner, err := pmsync.Banner(s.d.Main, s.d.Records, s.d.Remote)
		if err != nil {
			banner = `<div class="note draft push"><p>` + html.EscapeString(err.Error()) + `</p></div>`
		}
		text = strings.Replace(text, "</nav>", "</nav>"+banner, 1)
	}
	// a sent reply stays on its card until the snapshot's items hold its comment
	shown := map[string]Reply{}
	s.mu.Lock()
	for id, r := range s.replies {
		if !(r.State == "sent" && replyIn(snap, id, r.RID)) {
			shown[id] = r
		}
	}
	s.mu.Unlock()
	return r.code, s.d.Site.FillReplies(text, s.token, shown), snap
}

// replyIn is whether the snapshot shows reply rid on its card: a comment on the need ends in its mark, or the need
// is gone or closed, so its card shows no thread to wait for.
func replyIn(snap *snapshot, id, rid string) bool {
	n := snap.items[id]
	if n == nil || n.Status == work.Closed {
		return true
	}
	for _, c := range n.Comments {
		if strings.Contains(c.Text, replyMark(rid)) {
			return true
		}
	}
	return false
}

func digest(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:8*2]
}

// pathOf is the page a URL path names: index.html for a directory.
func pathOf(p string) string {
	p = strings.TrimLeft(p, "/")
	if p == "" || strings.HasSuffix(p, "/") {
		p += "index.html"
	}
	return p
}

type logged struct {
	http.ResponseWriter
	code int
}

func (l *logged) WriteHeader(code int) {
	l.code = code
	l.ResponseWriter.WriteHeader(code)
}

// ServeHTTP answers GET and HEAD for the pages, /style.css and /version, and POST for /reply; it logs one line each.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lw := &logged{ResponseWriter: w}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.get(lw, r)
	case http.MethodPost:
		s.post(lw, r)
	default:
		s.reply(lw, http.StatusNotImplemented, "text/plain; charset=utf-8", []byte("unsupported method\n"))
	}
	s.logf("%s %s %d total=%dms", r.Method, r.URL.RequestURI(), lw.code, time.Since(start).Milliseconds())
}

func (s *server) reply(w http.ResponseWriter, code int, ctype string, body []byte) {
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Length", fmt.Sprint(len(body)))
	h.Set("Cache-Control", "no-store")
	h.Set(StoreHeader, resolve(s.d.Records)) // lets a probe tell this service apart
	h.Set(VersionHeader, Version())
	w.WriteHeader(code)
	_, _ = w.Write(body) // net/http drops the body of a HEAD
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	s.poke() // a reload asks for a look at once; it does not wait for it
	switch r.URL.Path {
	case "/style.css":
		s.reply(w, 200, "text/css; charset=utf-8", s.d.Style)
		return
	case "/version":
		target := r.URL.Query().Get("page")
		if _, ok := r.URL.Query()["page"]; !ok {
			target = "/"
		}
		u, err := url.Parse(target)
		if err != nil {
			u = &url.URL{Path: target}
		}
		_, text, snap := s.page(pathOf(u.Path))
		body := fmt.Sprintf(`{"asof": %s, "page": %s}`, pyjson.FloatRepr(float64(snap.asOf.UnixMicro())/1e6),
			pyjson.String(digest(text), true))
		s.reply(w, 200, "application/json", []byte(body))
		return
	}
	code, text, snap := s.page(pathOf(r.URL.Path))
	text = s.d.Site.FillStatus(text, snap.asOf, digest(text), time.Now())
	s.reply(w, code, "text/html; charset=utf-8", []byte(text))
}

var replyID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// MaxReply is the largest reply body the service takes, form encoding included.
const MaxReply = 1 << 20

// post takes a reply from a card's form: checked, spooled, and back to the card, which shows it saving.
func (s *server) post(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/reply" {
		s.reply(w, 404, "text/plain; charset=utf-8", []byte("only /reply takes a POST\n"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxReply+1))
	if err != nil {
		s.reply(w, 400, "text/plain; charset=utf-8", []byte("refused: the body could not be read\n"))
		return
	}
	if len(raw) > MaxReply { // refused whole, never stored cut short
		s.reply(w, 413, "text/plain; charset=utf-8", []byte(fmt.Sprintf("refused: the reply is over %d bytes\n", MaxReply)))
		return
	}
	form, _ := url.ParseQuery(strings.ToValidUTF8(string(raw), "�"))
	code, said := s.take(form, r.Host)
	if code != 303 {
		s.reply(w, code, "text/plain; charset=utf-8", []byte(said+"\n"))
		return
	}
	back := "/"
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host && r.Referer() != "" {
		back = ref.Path
		if back == "" {
			back = "/"
		}
	}
	w.Header().Set("Location", back+"#need-"+said)
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(303)
}

// replyHosts are the Host names a reply may come to: the service binds 127.0.0.1, so any other is a rebinding
// domain, except the configured public URL's host.
func (s *server) replyHosts() []string {
	hosts := []string{"127.0.0.1", "localhost"}
	if u, err := url.Parse(s.d.SiteURL); err == nil && u.Hostname() != "" {
		hosts = append(hosts, strings.ToLower(u.Hostname()))
	}
	return hosts
}

func (s *server) siteURL() string {
	if s.d.SiteURL != "" {
		return s.d.SiteURL
	}
	return fmt.Sprintf("http://localhost:%d", s.d.Port)
}

// check checks the owner's reply against the snapshot's items, so the check reads nothing: the status, then on
// success (303) the need's id and the reply's text, else why it was refused and "".
func (s *server) check(form url.Values, host string, items map[string]*work.Item) (int, string, string) {
	name := host
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[:i]
	}
	allowed := false
	for _, h := range s.replyHosts() {
		allowed = allowed || name == h
	}
	if !allowed {
		shown := "None"
		if host != "" {
			shown = proc.Repr(host)
		}
		return 403, fmt.Sprintf("refused: Host %s is neither this machine nor the configured site_url; open the site "+
			"at %s", shown, s.siteURL()), ""
	}
	if subtle.ConstantTimeCompare([]byte(form.Get("token")), []byte(s.token)) != 1 {
		return 403, "refused: the reply carries no valid token; reload the page (the server may have restarted) and " +
			"send it again", ""
	}
	id := form.Get("id")
	text := config.PyStrip(strings.ReplaceAll(form.Get("text"), "\r\n", "\n"))
	n := items[id]
	if n == nil || n.Type != work.Need {
		return 400, fmt.Sprintf("refused: %s is not a request waiting on the owner", proc.Repr(id)), ""
	}
	if n.Status == work.Closed {
		why := n.CloseReason
		if why == "" {
			why = "no reason"
		}
		return 400, fmt.Sprintf("refused: %s is already closed (%s)", id, why), ""
	}
	if text == "" {
		return 400, "refused: the reply is empty", ""
	}
	return 303, id, text
}

func (s *server) take(form url.Values, host string) (int, string) {
	snap := s.snap.Load()
	if snap.err != "" {
		return 500, "error: the reply was not taken, since the site does not render: " + snap.err
	}
	code, id, text := s.check(form, host, snap.items)
	if code != 303 {
		return code, id
	}
	rid := form.Get("rid")
	if rid == "" { // a page without the form's script sends none
		rid = newUUID()
	}
	if !replyID.MatchString(rid) {
		return 400, fmt.Sprintf("refused: %s is not a reply id", proc.Repr(rid))
	}
	e := Entry{RID: rid, ID: id, Text: text, At: float64(time.Now().UnixMicro()) / 1e6}
	added, err := SpoolAdd(s.d.Spool, e)
	if err != nil {
		return 500, "error: the reply was not taken: " + err.Error()
	}
	if added { // a resubmit of a reply id the spool holds is already on its way
		s.mu.Lock()
		s.replies[id] = Reply{State: "saving", Text: text}
		s.mu.Unlock()
		s.put(job{entry: &e, id: id})
	}
	return 303, id
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Backoff is the wait before a failed reply write is tried again: 1, 2, 4 … up to 60 seconds.
var Backoff = func(tries int) time.Duration { return min(60*time.Second, time.Second<<min(tries, 6)) }

// writer makes the service's writes one at a time, so a reply's POST never waits for the store: replies, merges and
// the sweep. A failed reply stays in the spool and is tried again after a backoff.
func (s *server) writer() {
	for {
		var j job
		select {
		case j = <-s.queue:
		case <-s.done:
			return
		}
		switch {
		case j.sweep:
			s.sweep()
		case j.merge != "": // a merge the watch saw: stored, then pushed like a reply
			start := time.Now()
			err := s.withStore("merge "+j.id, func(st work.Store) error {
				return st.UpdateNeed(j.id, work.NeedUpdate{ReviewMerged: &j.merge})
			})
			var got string
			if err == nil {
				got, err = s.pushUndelivered(j.id)
			}
			if err != nil {
				s.logf("merge %s %s failed: %v", j.id, j.merge, err)
				s.mu.Lock()
				delete(s.merges, j.id) // the next look tries it again
				s.mu.Unlock()
			} else {
				s.logf("merge %s %s %s total=%dms", j.id, j.merge, got, time.Since(start).Milliseconds())
			}
		default:
			s.writeReply(j)
		}
		s.poke()
	}
}

func (s *server) writeReply(j job) {
	start, e := time.Now(), *j.entry
	if err := s.deliverReply(e); err != nil {
		s.mu.Lock()
		s.replies[e.ID] = Reply{State: "failed", Text: e.Text, RID: e.RID, Error: err.Error()}
		s.mu.Unlock()
		s.logf("reply %s failed total=%dms: %v", e.ID, time.Since(start).Milliseconds(), err)
		time.AfterFunc(Backoff(j.tries), func() { s.put(job{entry: &e, id: e.ID, tries: j.tries + 1}) })
		return
	}
	delivery, err := s.pushUndelivered(e.ID)
	if err != nil { // the reply is stored; it stays undelivered, and the sweep tries it again
		delivery = "failed: " + err.Error()
	}
	s.mu.Lock()
	s.replies[e.ID] = Reply{State: "sent", Text: e.Text, RID: e.RID, Delivery: strings.SplitN(delivery, ":", 2)[0]}
	s.mu.Unlock()
	s.logf("reply %s sent total=%dms", e.ID, time.Since(start).Milliseconds())
	s.logf("push %s: %s", e.ID, delivery)
}

// sweep, on the writer: push again what an open need's session has not received (a push that failed, or a crash
// between storing a reply and pushing it). A session that is gone costs a stat per look: a need whose push found it
// not running is skipped until its inbox path appears or changes. It logs only a change.
func (s *server) sweep() {
	snap := s.snap.Load()
	for _, n := range snap.order {
		if n.Status == work.Closed || n.Need == nil || n.Need.RaisedBy == nil || n.Need.RaisedBy.Inbox == "" {
			continue
		}
		if !ReplyWaiting(n) && MergeWaiting(n) == "" {
			continue
		}
		path := n.Need.RaisedBy.Inbox
		st, err := os.Stat(path)
		if err != nil {
			continue // no socket: the session is not running
		}
		var ino uint64
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			ino = uint64(sys.Ino)
		}
		key := fmt.Sprintf("%s\x00%d\x00%d", path, ino, st.ModTime().UnixNano())
		s.mu.Lock()
		last := s.swept[n.ID]
		s.mu.Unlock()
		if last.key == key {
			continue
		}
		got, err := s.pushUndelivered(n.ID)
		if err != nil {
			got = "failed: " + err.Error()
		}
		if got != last.got {
			s.logf("push %s (sweep): %s", n.ID, got)
		}
		next := sweepState{got: got}
		if strings.HasPrefix(got, "not running") {
			next.key = key
		}
		s.mu.Lock()
		s.swept[n.ID] = next
		s.mu.Unlock()
	}
}

// ticker, at start and every MergePoll: look at each open review's PR without a seen merge (with gh), and hand one
// merged to main to the writer; then queue a sweep. A failing look is logged, and the next tick looks again.
func (s *server) ticker() {
	for {
		snap := s.snap.Load()
		for _, n := range snap.order {
			if !s.gh {
				break
			}
			pr := reviewPR(n)
			s.mu.Lock()
			queued := s.merges[n.ID]
			s.mu.Unlock()
			if pr == "" || n.Need.Review.Merged != "" || queued {
				continue
			}
			if sha := s.mergedOnMain(pr); sha != "" {
				s.mu.Lock()
				s.merges[n.ID] = true
				s.mu.Unlock()
				s.put(job{id: n.ID, merge: sha})
			}
		}
		s.put(job{sweep: true})
		if !s.sleep(s.mergePoll) {
			return
		}
	}
}

// SyncTimeout bounds the work store's sync.
var SyncTimeout = 120 * time.Second

// SyncSteps are one sync run's steps: the work store's sync, the day summary, the records push.
func (s *server) SyncSteps() []pmsync.Step {
	return []pmsync.Step{
		{Name: "work", Run: func() (bool, string) {
			ctx, cancel := context.WithTimeout(context.Background(), SyncTimeout)
			defer cancel()
			lines, err := s.d.Sync(ctx)
			if err != nil {
				return false, err.Error()
			}
			for _, w := range lines[1:] { // a claim a merge overrode: the session that lost it learns it from here
				s.logf("%s", w)
			}
			return true, lines[0]
		}},
		{Name: "summary", Run: s.d.Summarize},
		{Name: "records", Run: func() (bool, string) { return pmsync.PushRecords(s.d.Records, s.d.Remote) }},
	}
}

// syncer runs the sync every pmsync.Interval, logging each step's line; a run that fails is logged, and the next
// one tries again.
func (s *server) syncer() {
	for {
		if !s.sleep(pmsync.Interval) {
			return
		}
		_, out, err := pmsync.Push(s.d.Main, s.SyncSteps())
		if err != nil {
			out = "sync failed: " + err.Error()
		}
		s.logf("%s", out)
	}
}
