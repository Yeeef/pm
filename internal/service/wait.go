package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/work"
)

// The service's waits: a Claude Code session's pm hook ask holds one call on the service's wait socket, which answers
// once the snapshot shows the question's need with a site reply waiting or closed. Each snapshot the refresher stores
// wakes every waiter, so they share its one look at the store (every ServeCheck, at once after the writer stored a
// reply): a wait costs no read of the store, and the hook dials once per answer instead of once a second.

// WaitSockName is the wait socket's file in <main checkout>/.pm/run, beside the work store's socket and no longer
// than its name, so a clone whose work socket fits the kernel's limit fits this one.
const WaitSockName = "wait.sock"

// WaitSock is the wait socket of the clone whose main checkout is main.
func WaitSock(main string) string { return filepath.Join(main, ".pm", "run", WaitSockName) }

// setSnap stores snap as the one served and wakes every waiter to look at it.
func (s *server) setSnap(snap *snapshot) {
	s.snap.Store(snap)
	s.movedMu.Lock()
	close(s.moved)
	s.moved = make(chan struct{})
	s.movedMu.Unlock()
}

// serveWaits listens on the wait socket, mode 0600, removing one a crash left (the clone's one service holds the work
// store, so no other serves it), and serves waits until Run closes the server it returns.
func (s *server) serveWaits() (*http.Server, error) {
	sock := WaitSock(s.d.Main)
	if err := os.Remove(sock); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("the wait socket: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		return nil, fmt.Errorf("the wait socket: %w", err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("the wait socket: %w", err)
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("the wait socket: %w", err), ln.Close())
	}
	srv := &http.Server{Handler: http.HandlerFunc(s.waitNeed), ReadHeaderTimeout: 30 * time.Second}
	s.spawn(func() { _ = srv.Serve(ln) }) //nolint:errcheck // ErrServerClosed, once Run closes srv
	return srv, nil
}

// waitNeed answers GET /need?id=<id> once the snapshot shows need id with a site reply waiting or closed: 200. A need
// not in the snapshot yet (raised since its last look) is waited for. A caller that hangs up ends it, and a stopping
// service answers 503, so the caller calls again.
func (s *server) waitNeed(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id := r.URL.Query().Get("id")
	if r.URL.Path != "/need" || id == "" {
		http.Error(w, "only GET /need?id=<need id> waits", http.StatusNotFound)
		return
	}
	for {
		s.movedMu.Lock()
		moved := s.moved // taken before the look, so a snapshot stored after it wakes this wait
		s.movedMu.Unlock()
		if n := s.snap.Load().items[id]; n != nil && (n.Status == work.Closed || ReplyWaiting(n)) {
			s.logf("wait %s answered total=%dms", id, time.Since(start).Milliseconds())
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "moved\n") //nolint:errcheck // a caller gone has nothing to answer
			return
		}
		select {
		case <-moved:
		case <-r.Context().Done():
			s.logf("wait %s left total=%dms", id, time.Since(start).Milliseconds())
			return
		case <-s.done:
			http.Error(w, "the pm service is stopping", http.StatusServiceUnavailable)
			return
		}
	}
}

// WaitNeed blocks until the pm service of the clone whose main checkout is main shows need id with a site reply
// waiting or closed (nil), ctx ends, or the call fails (the service down, restarting or stopping).
func WaitNeed(ctx context.Context, main, id string) error {
	sock := WaitSock(main)
	c := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	defer c.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://pm/need?id="+url.QueryEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("waiting on the pm service for %s: %w", id, err)
	}
	defer resp.Body.Close()                                //nolint:errcheck // read only
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // the status says what happened
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("waiting on the pm service for %s: %d %s", id, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
