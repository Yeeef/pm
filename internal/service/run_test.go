package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/work"
)

// served is a running Run on a free port with the fake site and store, its main checkout in a temp dir.
type served struct {
	t         *testing.T
	w         *fakeWork
	site      *fakeSite
	main, pin string
	spool     string
	out, log  *syncBuffer
	base      string
	done      chan error
	stopped   bool
}

// fakeBin puts sh scripts on PATH ahead of the system's, so git is real and gh, launchctl and systemctl are fakes.
func fakeBin(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+filepath.Dir(git)+string(os.PathListSeparator)+"/usr/bin:/bin")
	return dir
}

// fakeGH answers gh pr view with $FAKE_GH_JSON, or fails with $FAKE_GH_FAIL.
const fakeGH = `[ -n "$FAKE_GH_FAIL" ] && { echo "$FAKE_GH_FAIL" >&2; exit 1; }
if [ -n "$FAKE_GH_JSON" ]; then echo "$FAKE_GH_JSON"; else echo '{"state":"OPEN"}'; fi`

func quick(t *testing.T) {
	t.Helper()
	saved := []time.Duration{ServeCheck, MergePoll}
	ServeCheck, MergePoll = 20*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { ServeCheck, MergePoll = saved[0], saved[1] })
}

func serve(t *testing.T, w *fakeWork, main string) *served {
	t.Helper()
	quick(t)
	if main == "" {
		main = t.TempDir()
		fakeBin(t, map[string]string{"gh": fakeGH})
	}
	for _, d := range []string{".pm/store/records", ".git"} {
		if err := os.MkdirAll(filepath.Join(main, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := &served{t: t, w: w, site: &fakeSite{}, main: main, pin: filepath.Join(main, ".pm/config.toml"),
		spool: filepath.Join(main, ".git", SpoolName), out: &syncBuffer{}, log: &syncBuffer{}, done: make(chan error, 1)}
	s.setPin(Version())
	go func() {
		err := Run(Deps{Main: main, Records: store(main), Remote: "origin", MainBranch: "main", Port: 0, Pin: s.pin,
			Spool: s.spool, WorkDir: main, Open: w.Open, Mark: w.Mark, Sync: w.Sync, GC: w.GC, Site: s.site,
			Summarize: func() (bool, string) { return true, "summarized" }, Style: []byte("body{}"), Out: s.out,
			Log: s.log})
		w.mu.Lock()
		w.ran = true
		w.mu.Unlock()
		s.done <- err
	}()
	re := regexp.MustCompile(`Serving http://localhost:(\d+);`)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if m := re.FindStringSubmatch(s.out.String()); m != nil {
			s.base = "http://127.0.0.1:" + m[1]
			break
		}
		select {
		case err := <-s.done:
			t.Fatalf("Run returned before serving: %v; log:\n%s", err, s.log.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("not serving after 5 s; log:\n%s", s.log.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() {
		if !s.stopped {
			s.stop()
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.open != 0 {
			t.Errorf("the service left %d connections to the work store open", w.open)
		}
		if w.late != 0 {
			t.Errorf("%d calls reached the work store after Run returned", w.late)
		}
	})
	return s
}

func (s *served) setPin(v string) {
	tmp := s.pin + ".new" // replaced whole, as git pull does
	if err := os.WriteFile(tmp, []byte("version = \""+v+"\"\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Rename(tmp, s.pin); err != nil {
		s.t.Fatal(err)
	}
}

// stop moves the pin, which stops the service; Run's error.
func (s *served) stop() error {
	s.stopped = true
	s.setPin("9.9.9")
	select {
	case err := <-s.done:
		return err
	case <-time.After(5 * time.Second):
		s.t.Fatal("the service did not stop 5 s after its pin moved")
	}
	return nil
}

func (s *served) get(path string) (*http.Response, string) {
	s.t.Helper()
	resp, err := http.Get(s.base + path)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

var tokenRe = regexp.MustCompile(`token=(\S+) `)

func (s *served) token() string {
	_, body := s.get("/")
	m := tokenRe.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no form on the index: %s", body)
	}
	return m[1]
}

// post sends a reply as the card's form does; the status, Location and body.
func (s *served) post(form url.Values, host string) (int, string, string) {
	s.t.Helper()
	req, _ := http.NewRequest("POST", s.base+"/reply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.base+"/sprints/x.html")
	if host != "" {
		req.Host = host
	} else {
		req.Host = strings.TrimPrefix(s.base, "http://")
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(body)
}

// eventually waits up to 5 s for cond.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("not within 5 s: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// inbox is a session's inbox socket: the lines written to it, one per connection.
func inbox(t *testing.T) (string, chan string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pm-inbox") // a socket path must be short
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	lines := make(chan string, 10)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			l, _ := bufio.NewReader(c).ReadString('\n')
			c.Close()
			lines <- l
		}
	}()
	return path, lines
}

func receive(t *testing.T, lines chan string) string {
	t.Helper()
	select {
	case l := <-lines:
		var msg struct {
			Type    string `json:"type"`
			Message struct {
				Role, Content string
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(l), &msg); err != nil || msg.Type != "user" || msg.Message.Role != "user" {
			t.Fatalf("not a user message line: %q (%v)", l, err)
		}
		return msg.Message.Content
	case <-time.After(5 * time.Second):
		t.Fatal("nothing reached the inbox within 5 s")
	}
	return ""
}

var at = time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)

func need(id string, kind work.NeedKind, inbox string) work.Item {
	host, _ := Hostname()
	n := work.Item{ID: id, Type: work.Need, Parent: "p-1.2", Title: "Pick a port", Status: work.Open, CreatedAt: at,
		UpdatedAt: at, Need: &work.NeedInfo{Kind: kind}}
	if inbox != "" {
		n.Need.RaisedBy = &work.RaisedBy{Session: "s1", Inbox: inbox, Host: host}
	}
	if kind == work.Review {
		n.Need.Review = &work.ReviewInfo{PR: "https://github.com/o/r/pull/7", Sprints: []string{"p-1.2"}, Focus: "all"}
	}
	return n
}

func TestServesPagesStyleAndVersionWithTheStoreAndVersionHeaders(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "")
	resp, body := s.get("/")
	if resp.StatusCode != 200 || resp.Header.Get(StoreHeader) != store(s.main) ||
		resp.Header.Get(VersionHeader) != Version() || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /: %d %v", resp.StatusCode, resp.Header)
	}
	if !strings.HasPrefix(body, "<nav>home</nav>[status ") || !strings.Contains(body, "[form p-1.2.1 token=") {
		t.Fatalf("index: %s", body)
	}
	if resp, body := s.get("/items/p-1.2.1.html"); resp.StatusCode != 200 || !strings.HasPrefix(body, "<h1>Pick a port</h1>[status ") {
		t.Fatalf("item page: %d %s", resp.StatusCode, body)
	}
	if resp, body := s.get("/nope.html"); resp.StatusCode != 404 || !strings.Contains(body, "No page nope.html") ||
		!strings.Contains(body, "[status ") {
		t.Fatalf("missing page: %d %s", resp.StatusCode, body)
	}
	if resp, body := s.get("/broken.html"); resp.StatusCode != 500 || !strings.Contains(body, "error: the page broke") {
		t.Fatalf("broken page: %d %s", resp.StatusCode, body)
	}
	if resp, body := s.get("/style.css"); body != "body{}" || resp.Header.Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("style: %s %v", body, resp.Header)
	}
	_, index := s.get("/")
	_, version := s.get("/version?page=/")
	var v struct {
		AsOf float64 `json:"asof"`
		Page string  `json:"page"`
	}
	if err := json.Unmarshal([]byte(version), &v); err != nil || v.AsOf == 0 || "[status "+v.Page+"]" != regexp.MustCompile(`\[status \w+\]`).FindString(index) {
		t.Fatalf("version %s against index %s (%v)", version, index, err)
	}
	head, _ := http.Head(s.base + "/")
	if head.StatusCode != 200 || head.Header.Get(StoreHeader) == "" {
		t.Fatalf("HEAD: %d", head.StatusCode)
	}
	if code, _, body := s.postTo("/other"); code != 404 || body != "only /reply takes a POST\n" {
		t.Fatalf("POST /other: %d %q", code, body)
	}
	if !regexp.MustCompile(`(?m)^GET / 200 total=\d+ms$`).MatchString(s.log.String()) {
		t.Fatalf("no request line in the log:\n%s", s.log.String())
	}
}

// raw sends path as the request line holds it, so no client cleans a .. out first; it returns the status and the
// Content-Type.
func (s *served) raw(path string) (int, string) {
	s.t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(s.base, "http://"))
	if err != nil {
		s.t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET " + path + " HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n")); err != nil {
		s.t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Content-Type")
}

func TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt(t *testing.T) {
	s := serve(t, newFakeWork(), "")
	recs := store(s.main)
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"/>`
	files := map[string]string{"docs/x.svg": svg, "docs/fig.png": "\x89PNG\r\n", "docs/a.jpg": "jpg", "docs/b.webp": "webp",
		"docs/notes.txt": "text", ".git/x.svg": svg, ".hidden.svg": svg}
	for rel, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(recs, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(recs, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(filepath.Dir(recs), "secret.svg") // beside the store, so ../secret.svg would reach it
	if err := os.WriteFile(secret, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"docs/out.svg": "../../secret.svg", "docs/abs.svg": secret, "docs/in.svg": "x.svg",
		"docs/leak.svg": "../.hidden.svg", "docs/git.svg": "../.git/x.svg"} {
		if err := os.Symlink(target, filepath.Join(recs, link)); err != nil {
			t.Fatal(err)
		}
	}
	resp, body := s.get("/docs/x.svg")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" || body != svg ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("GET /docs/x.svg: %d %v %q", resp.StatusCode, resp.Header, body)
	}
	for path, want := range map[string]string{"/docs/fig.png": "image/png", "/docs/a.jpg": "image/jpeg",
		"/docs/b.webp": "image/webp", "/docs/in.svg": "image/svg+xml"} {
		if code, ctype := s.raw(path); code != 200 || ctype != want {
			t.Errorf("GET %s: %d %s, want 200 %s", path, code, ctype, want)
		}
	}
	for _, path := range []string{"/docs/missing.svg", "/../secret.svg", "/docs/../../secret.svg",
		"/docs/%2e%2e/%2e%2e/secret.svg", "/%2e%2e/secret.svg", "//" + strings.TrimPrefix(secret, "/"), "/docs/out.svg",
		"/docs/abs.svg", "/.git/x.svg", "/docs/./x.svg", "/docs", "/docs/leak.svg", "/docs/git.svg"} {
		if code, _ := s.raw(path); code != 404 {
			t.Errorf("GET %s: %d, want 404", path, code)
		}
	}
	if code, ctype := s.raw("/docs/notes.txt"); code != 404 || !strings.HasPrefix(ctype, "text/html") {
		t.Errorf("GET /docs/notes.txt: %d %s, want the 404 page: only image files are served", code, ctype)
	}
}

func (s *served) postTo(path string) (int, string, string) {
	resp, err := http.Post(s.base+path, "text/plain", strings.NewReader("x"))
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, "", string(body)
}

func TestAReplyIsSpooledStoredOnceAndPushedIntoTheSessionsInbox(t *testing.T) {
	path, lines := inbox(t)
	w := newFakeWork(need("p-1.2.1", work.Decision, path))
	s := serve(t, w, "")
	form := url.Values{"token": {s.token()}, "id": {"p-1.2.1"}, "rid": {"r-1"}, "text": {"  Use 8123.\r\nWhy: free.  "}}
	code, loc, body := s.post(form, "")
	if code != 303 || loc != "/sprints/x.html#need-p-1.2.1" {
		t.Fatalf("POST: %d %q %q", code, loc, body)
	}
	got := receive(t, lines)
	want := "pm: owner reply to decision p-1.2.1 (Pick a port), relayed from the site:\n  [2026-10-08T12:00:00Z] Use 8123.\n" +
		"  Why: free.\nnext: record the answer: pm decision add --need p-1.2.1 --level … --decision '<the answer>' " +
		"--reason '<why>' if it sets a rule, else pm decision close p-1.2.1 --reason \"<why it sets no rule>\" " +
		"--text-file - <<'EOF' (the answer, then EOF)"
	if got != want {
		t.Fatalf("inbox got\n%s\nwant\n%s", got, want)
	}
	eventually(t, "delivered marked", func() bool { return w.item("p-1.2.1").Need.Delivered == 1 })
	n := w.item("p-1.2.1")
	if len(n.Comments) != 1 || n.Comments[0].Kind != work.Reply || n.Comments[0].Author != ReplyAuthor ||
		n.Comments[0].Text != "Use 8123.\nWhy: free.\n\n<!-- pm-reply r-1 -->" || n.Status != work.Open {
		t.Fatalf("stored: %+v", n)
	}
	eventually(t, "the spool emptied", func() bool { data, _ := os.ReadFile(s.spool); return len(data) == 0 })
	eventually(t, "the card shows it sent and delivered", func() bool {
		_, body := s.get("/")
		return strings.Contains(body, "state=sent delivery=delivered") || strings.Contains(body, "state= ")
	})
	if code, _, _ := s.post(form, ""); code != 303 { // a resubmit of the same reply id
		t.Fatalf("resubmit: %d", code)
	}
	time.Sleep(100 * time.Millisecond)
	if n := w.item("p-1.2.1"); len(n.Comments) != 1 {
		t.Fatalf("a resubmit stored the reply twice: %+v", n.Comments)
	}
}

func TestAReplyIsRefusedWithItsReason(t *testing.T) {
	closed := need("p-1.2.2", work.Action, "")
	closed.Status, closed.CloseReason = work.Closed, "done by hand"
	task := work.Item{ID: "p-1.2.3", Type: work.Task, Title: "t", Status: work.Open}
	w := newFakeWork(need("p-1.2.1", work.Decision, ""), closed, task)
	s := serve(t, w, "")
	tok := s.token()
	ok := url.Values{"token": {tok}, "id": {"p-1.2.1"}, "text": {"yes"}}
	with := func(k, v string) url.Values {
		f := url.Values{}
		for key, vals := range ok {
			f[key] = vals
		}
		f.Set(k, v)
		return f
	}
	cases := []struct {
		form url.Values
		host string
		code int
		said string
	}{
		{ok, "evil.example:80", 403, "refused: Host 'evil.example:80' is neither this machine nor the configured site_url; open the site at http://localhost:0"},
		{with("token", "x"), "", 403, "refused: the reply carries no valid token; reload the page (the server may have restarted) and send it again"},
		{with("id", "p-9"), "", 400, "refused: 'p-9' is not a request waiting on the owner"},
		{with("id", "p-1.2.3"), "", 400, "refused: 'p-1.2.3' is not a request waiting on the owner"},
		{with("id", "p-1.2.2"), "", 400, "refused: p-1.2.2 is already closed (done by hand)"},
		{with("text", " \r\n "), "", 400, "refused: the reply is empty"},
		{with("rid", "a b"), "", 400, "refused: 'a b' is not a reply id"},
	}
	for _, c := range cases {
		code, _, body := s.post(c.form, c.host)
		if code != c.code || body != c.said+"\n" {
			t.Errorf("got %d %q, want %d %q", code, body, c.code, c.said)
		}
	}
	if code, _, _ := s.post(ok, "localhost:1"); code != 303 { // no rid: one is made
		t.Fatalf("a reply without rid: %d", code)
	}
	eventually(t, "stored", func() bool { return len(w.item("p-1.2.1").Comments) == 1 })
}

func TestARenderErrorIsServedAsTheErrorAndTakesNoReply(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "")
	tok := s.token()
	s.site.mu.Lock()
	s.site.loadErr, s.site.stamp = errors.New("sprints/x.md: no Goal"), 1
	s.site.mu.Unlock()
	eventually(t, "the error served", func() bool {
		resp, body := s.get("/")
		return resp.StatusCode == 500 && strings.Contains(body, "The site did not render") &&
			strings.Contains(body, "error: sprints/x.md: no Goal")
	})
	code, _, body := s.post(url.Values{"token": {tok}, "id": {"p-1.2.1"}, "text": {"yes"}}, "")
	if code != 500 || body != "error: the reply was not taken, since the site does not render: error: sprints/x.md: no Goal\n" {
		t.Fatalf("POST: %d %q", code, body)
	}
	s.site.mu.Lock()
	loads := s.site.loads
	s.site.mu.Unlock()
	if loads < 3 { // the first good load, then a failed read and its repeat under the records lock
		t.Fatalf("a failed read is not repeated under the lock: %d loads", loads)
	}
}

// A read that found the records moving (a Partial) is repeated under the records lock; one that still finds them
// moving there (no pm write moves them under it) is served as read, never as the error page.
func TestAPartialReadIsRepeatedUnderTheLockAndServedThere(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "")
	if resp, body := s.get("/"); resp.StatusCode != 200 || !strings.Contains(body, "[form p-1.2.1") {
		t.Fatalf("the first read: %d %q", resp.StatusCode, body)
	}
	s.site.mu.Lock()
	start := s.site.loads
	s.site.loadErr = &Partial{Pages: fakePages{}, Gone: []string{"sprints/x.md"}} // read without the need's page
	s.site.stamp = 1
	s.site.mu.Unlock()
	eventually(t, "the partial read served", func() bool {
		resp, body := s.get("/")
		return resp.StatusCode == 200 && strings.Contains(body, "<nav>home</nav>") && !strings.Contains(body, "[form")
	})
	s.site.mu.Lock()
	loads := s.site.loads - start
	s.site.mu.Unlock()
	if loads < 2 {
		t.Fatalf("a partial read is not repeated under the lock: %d loads", loads)
	}
}

func TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsRefused(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	failing := true
	var mu gosync.Mutex
	open := func() (work.Store, error) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return nil, errors.New("the pm service does not answer")
		}
		return w.Open()
	}
	main := t.TempDir()
	fakeBin(t, map[string]string{"gh": fakeGH})
	quick(t)
	os.MkdirAll(filepath.Join(main, ".pm/store/records"), 0o755)
	s := &served{t: t, w: w, site: &fakeSite{}, main: main, pin: filepath.Join(main, ".pm/config.toml"),
		spool: filepath.Join(main, SpoolName), out: &syncBuffer{}, log: &syncBuffer{}, done: make(chan error, 1)}
	s.setPin(Version())
	go func() {
		s.done <- Run(Deps{Main: main, Records: store(main), Remote: "origin", MainBranch: "main", Pin: s.pin,
			Spool: s.spool, WorkDir: main, Open: open, Mark: w.Mark, Sync: w.Sync, GC: w.GC, Site: s.site,
			Summarize: func() (bool, string) { return true, "" }, Style: []byte("x"), Out: s.out, Log: s.log})
	}()
	eventually(t, "serving", func() bool { return strings.Contains(s.out.String(), "Serving http://localhost:") })
	s.base = "http://127.0.0.1:" + regexp.MustCompile(`localhost:(\d+);`).FindStringSubmatch(s.out.String())[1]
	t.Cleanup(func() { s.stop() })
	if resp, body := s.get("/"); resp.StatusCode != 500 || !strings.Contains(body, "the pm service does not answer") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	mu.Lock()
	failing = false
	mu.Unlock()
	eventually(t, "served once the store opens, with nothing moved", func() bool { resp, _ := s.get("/"); return resp.StatusCode == 200 })
	big := url.Values{"token": {s.token()}, "id": {"p-1.2.1"}, "text": {strings.Repeat("x", MaxReply)}}
	if code, _, body := s.post(big, ""); code != 413 || !strings.HasPrefix(body, "refused: the reply is over ") {
		t.Fatalf("%d %q", code, body)
	}
}

func TestTheStoreIsOpenedOnlyWhenItMovedAndNeverHeld(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "")
	time.Sleep(200 * time.Millisecond) // about 10 looks
	w.mu.Lock()
	reads := w.reads
	w.mu.Unlock()
	if reads != 1 {
		t.Fatalf("%d reads of the items while nothing moved; want 1", reads)
	}
	n := w.item("p-1.2.1")
	n.Title = "Pick a port, again"
	w.set(n)
	eventually(t, "the page follows the store", func() bool {
		_, body := s.get("/items/p-1.2.1.html")
		return strings.Contains(body, "again")
	})
}

func TestAPendingSpoolIsStoredAtStartOnce(t *testing.T) {
	main := t.TempDir()
	fakeBin(t, map[string]string{"gh": fakeGH})
	os.MkdirAll(filepath.Join(main, ".git"), 0o755)
	spool := filepath.Join(main, ".git", SpoolName)
	n := need("p-1.2.1", work.Decision, "")
	// a crash after the comment was written but before the spool's done line: the comment is not doubled
	n.Comments = []work.Comment{{ID: "c1", Kind: work.Reply, Author: ReplyAuthor, Text: "a\n\n<!-- pm-reply r-1 -->", CreatedAt: at}}
	w := newFakeWork(n)
	for _, e := range []Entry{{RID: "r-1", ID: "p-1.2.1", Text: "a", At: 1}, {RID: "r-2", ID: "p-1.2.1", Text: "b", At: 2}} {
		if _, err := SpoolAdd(spool, e); err != nil {
			t.Fatal(err)
		}
	}
	serve(t, w, main)
	eventually(t, "both stored", func() bool { return len(w.item("p-1.2.1").Comments) == 2 })
	eventually(t, "the spool emptied", func() bool { data, _ := os.ReadFile(spool); return len(data) == 0 })
	if c := w.item("p-1.2.1").Comments; c[1].Text != "b\n\n<!-- pm-reply r-2 -->" {
		t.Fatalf("comments: %+v", c)
	}
}

func TestTheSweepDeliversWhatAFailedPushLeftAndSkipsAGoneSession(t *testing.T) {
	path, lines := inbox(t)
	waiting := need("p-1.2.1", work.Action, path)
	waiting.Comments = []work.Comment{{ID: "c1", Kind: work.Note, Author: "s1", Text: "a note", CreatedAt: at},
		{ID: "c2", Kind: work.Reply, Author: ReplyAuthor, Text: "done: see the log\n\n<!-- pm-reply r -->", CreatedAt: at}}
	gone := need("p-1.2.2", work.Decision, filepath.Join(t.TempDir(), "gone.sock"))
	gone.Comments = waiting.Comments[1:]
	w := newFakeWork(waiting, gone)
	s := serve(t, w, "")
	got := receive(t, lines)
	if !strings.HasPrefix(got, "pm: owner reply to action p-1.2.1 (Pick a port), relayed from the site:\n  [2026-10-08T09:00:00Z] done: see the log\nnext: check the evidence, then pm action done p-1.2.1") {
		t.Fatalf("sweep pushed %q", got)
	}
	eventually(t, "marked delivered", func() bool { return w.item("p-1.2.1").Need.Delivered == 1 })
	if w.item("p-1.2.2").Need.Delivered != 0 || strings.Contains(s.log.String(), "p-1.2.2") {
		t.Fatalf("a need whose inbox is gone was pushed: %s", s.log.String())
	}
}

func TestAMergeToMainIsStoredAndPushedOnce(t *testing.T) {
	main := t.TempDir()
	fakeBin(t, map[string]string{"gh": fakeGH})
	origin := filepath.Join(t.TempDir(), "origin.git")
	git := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(".", "init", "-q", "--bare", "-b", "main", origin)
	git(".", "clone", "-q", origin, main)
	git(main, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "merge")
	git(main, "push", "-q", "origin", "HEAD:main")
	sha := git(main, "rev-parse", "HEAD")
	t.Setenv("FAKE_GH_JSON", `{"state":"MERGED","mergeCommit":{"oid":"`+sha+`"}}`)
	path, lines := inbox(t)
	w := newFakeWork(need("p-1.2.1", work.Review, path))
	serve(t, w, main)
	got := receive(t, lines)
	want := "pm: PR #7 of review p-1.2.1 (Pick a port) merged to main as " + sha + "\nnext: pm action done p-1.2.1 " +
		"--reason \"merged as " + sha + "\", then update the main checkout: git -C " + main + " pull --ff-only origin main"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	eventually(t, "reported", func() bool { r := w.item("p-1.2.1").Need.Review; return r.MergeReported == sha && r.Merged == sha })
	select {
	case l := <-lines:
		t.Fatalf("pushed again: %s", l)
	case <-time.After(300 * time.Millisecond): // three more polls
	}
}

func TestAMergeNotOnMainYetIsNotReported(t *testing.T) {
	fakeBin(t, map[string]string{"gh": `echo '{"state":"MERGED","mergeCommit":{"oid":"0123456789abcdef0123456789abcdef01234567"}}'`})
	main := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", main).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	w := newFakeWork(need("p-1.2.1", work.Review, ""))
	s := serve(t, w, main)
	eventually(t, "the failed fetch logged", func() bool {
		return strings.Contains(s.log.String(), "warning: git fetch origin main failed, still waiting:")
	})
	if w.item("p-1.2.1").Need.Review.Merged != "" {
		t.Fatal("a merge not seen on main was stored")
	}
}

func TestTheServiceStopsOnceThePinMoves(t *testing.T) {
	s := serve(t, newFakeWork(), "")
	err := s.stop()
	if err == nil || !strings.Contains(err.Error(), "now pins pm 9.9.9, but this service runs pm "+Version()+"; stopping") {
		t.Fatalf("Run returned %v", err)
	}
	if !strings.Contains(s.log.String(), "error: "+err.Error()) {
		t.Fatalf("not logged: %s", s.log.String())
	}
}

// Run returns only once every goroutine it started ended: here the writer, held in Open when the pin moves. Returning
// before would let it reach the store after the caller closed the store's host.
func TestRunReturnsOnlyOnceTheGoroutinesItStartedEnded(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "")
	tok := s.token()
	h := w.holdNextOpen()
	if code, _, body := s.post(url.Values{"token": {tok}, "id": {"p-1.2.1"}, "text": {"yes"}}, ""); code != 303 {
		t.Fatalf("the reply: %d %s", code, body)
	}
	select {
	case <-h.entered: // the writer, storing the reply
	case <-time.After(5 * time.Second):
		t.Fatal("the writer did not open the store within 5 s")
	}
	s.stopped = true
	s.setPin("9.9.9")
	eventually(t, "the service sees its pin move", func() bool { return strings.Contains(s.log.String(), "now pins pm 9.9.9") })
	select {
	case <-s.done:
		close(h.release)
		t.Fatal("Run returned while the writer it started was still opening the work store")
	case <-time.After(300 * time.Millisecond): // Run, once the server stopped, waits for the writer
	}
	close(h.release)
	select {
	case err := <-s.done:
		if err == nil || !strings.Contains(err.Error(), "now pins pm 9.9.9") {
			t.Fatalf("Run returned %v, want the pin's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5 s of the writer going on")
	}
}

func TestSyncStepsSyncTheStoreSummarizeAndPushTheRecords(t *testing.T) {
	w, log := newFakeWork(), &syncBuffer{}
	w.syncWarnings = []string{"warning: demo-1.1: the claim by s1 (t1) was overridden by the later claim of s2 (t2)"}
	s := &server{d: Deps{Main: t.TempDir(), Records: filepath.Join(t.TempDir(), "none"), Remote: "origin",
		Open: w.Open, Sync: w.Sync, Summarize: func() (bool, string) { return true, "summarized" }, Log: log}}
	var names, said []string
	for _, step := range s.SyncSteps() {
		ok, line := step.Run()
		names, said = append(names, step.Name), append(said, line)
		if step.Name != "records" && !ok {
			t.Fatalf("%s failed: %s", step.Name, line)
		}
	}
	if strings.Join(names, ",") != "work,summary,records" || said[0] != "up to date" || w.syncs != 1 || w.open != 0 {
		t.Fatalf("steps %v said %v; syncs %d, open %v", names, said, w.syncs, w.open)
	}
	if !strings.Contains(log.String(), "\n"+w.syncWarnings[0]+"\n") && !strings.HasPrefix(log.String(), w.syncWarnings[0]+"\n") {
		t.Fatalf("the overridden claim is not in the service log:\n%s", log.String())
	}
}

func TestGCRunsWhenDueAndRecordsTheSizes(t *testing.T) {
	main := t.TempDir()
	if err := os.WriteFile(filepath.Join(main, "chunk"), make([]byte, 2_500_000), 0o644); err != nil {
		t.Fatal(err)
	}
	w, log := newFakeWork(), &syncBuffer{}
	s := &server{d: Deps{Main: main, WorkDir: main, Open: w.Open, GC: w.GC, Log: log}}
	if due, err := gcDue(main, time.Now()); !due || err != nil {
		t.Fatalf("no collection recorded: due %v (%v)", due, err)
	}
	if line := GCLine(main); line != "gc        no collection recorded yet (the pm service collects every 24 h)" {
		t.Fatal(line)
	}
	st := s.collect()
	if !st.OK || w.gcs != 1 || w.open != 0 || st.Before != 2_500_000 || st.After != 2_500_000 {
		t.Fatalf("collected %+v; gcs %d, open %v", st, w.gcs, w.open)
	}
	if line := GCLine(main); !regexp.MustCompile(`^gc        ok at \S+Z: 2\.5 MB -> 2\.5 MB in \d+ ms$`).MatchString(line) {
		t.Fatal(line)
	}
	if !strings.Contains(log.String(), "gc ok: 2.5 MB -> 2.5 MB in ") {
		t.Fatal(log.String())
	}
	if due, _ := gcDue(main, time.Now()); due {
		t.Fatal("due again right after a collection")
	}
	if due, _ := gcDue(main, time.Now().Add(GCInterval)); !due {
		t.Fatal("not due GCInterval after the last collection")
	}
}
