package service

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
)

// fakeCodex is a codex on PATH that says it is version, and records each queue call's argv and CODEX_HOME in log;
// it answers for thread answered.
func fakeCodex(t *testing.T, version, answered string) (log string) {
	t.Helper()
	log = filepath.Join(t.TempDir(), "codex.log")
	fakeBin(t, map[string]string{"codex": `[ "$1" = --version ] && { echo "codex-cli ` + version + `"; exit 0; }
printf '%s\n' "CODEX_HOME=$CODEX_HOME" "$@" >> "` + log + `"
echo "Queued message 01a1-q for thread ` + answered + `."`})
	return log
}

// codexHome is a short CODEX_HOME (a socket under it must fit the kernel's limit).
func codexHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "pmcx") //nolint:usetesting // short: the control socket's path must fit the kernel's limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(home); err != nil {
			t.Error(err)
		}
	})
	return home
}

// codexNeed is a need a Codex session raised with its CODEX_HOME home.
func codexNeed(home string) work.Item {
	n := need("p-1.2.1", work.Decision, "")
	host, _ := Hostname() //nolint:errcheck // the test machine's name, as raisedBy stores it
	n.Need.RaisedBy = &work.RaisedBy{Session: rootThread, Inbox: session.CodexInbox(home), Host: host}
	return n
}

const rootThread = "01a12beb-06b9-7f40-8ecb-6468bfec7783"

// TestAReplyIsQueuedIntoALiveCodexSession: a Codex session's need stores its CODEX_HOME; while the session is live,
// the push runs codex queue under that home for its root thread with the push's own text, and through a local
// app-server daemon (--remote) when one answers on the home's control socket; an ended session is left undelivered,
// as a dead Claude Code socket is; a codex missing from PATH, older than 0.149.0, or answering for another thread is
// an undelivered outcome that says why.
func TestAReplyIsQueuedIntoALiveCodexSession(t *testing.T) {
	saved := SessionLive
	t.Cleanup(func() { SessionLive = saved })
	live := true
	SessionLive = func(s string) bool { return s == rootThread && live }
	home := codexHome(t)
	n := codexNeed(home)
	text := "pm: owner reply to decision p-1.2.1 (Pick a port), relayed from the site:\n  [x] yes"

	log := fakeCodex(t, "0.154.0", rootThread)
	if got := PushInbox(&n, text); got != "" {
		t.Fatalf("live: %q", got)
	}
	want := "CODEX_HOME=" + home + "\nqueue\n--thread\n" + rootThread + "\n--message\n" + text + "\n"
	if b, err := os.ReadFile(log); err != nil || string(b) != want {
		t.Fatalf("codex was run as %q (%v), want %q", b, err, want)
	}

	sock := filepath.Join(home, "app-server-control", "app-server-control.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	log = fakeCodex(t, "0.154.0", rootThread)
	if got := PushInbox(&n, text); got != "" {
		t.Fatalf("with a daemon: %q", got)
	}
	if b, err := os.ReadFile(log); err != nil || !strings.HasSuffix(string(b), "--remote\nunix://"+sock+"\n") {
		t.Fatalf("with a daemon, codex was run as %q (%v)", b, err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	live = false
	log = fakeCodex(t, "0.154.0", rootThread)
	if got := PushInbox(&n, text); got != "not running: the session that asked has ended" {
		t.Errorf("ended: %q", got)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("codex was run for a session that has ended")
	}
	live = true

	for name, c := range map[string]struct{ version, answered, want string }{
		"another thread": {"0.154.0", "01a12bef-027d-7c33-8fb3-8fd52363eaf7",
			"failed: codex queue said: Queued message 01a1-q for thread 01a12bef-027d-7c33-8fb3-8fd52363eaf7."},
		"too old": {"0.148.0", rootThread, "no codex: codex 0.148.0 is older than 0.149.0, the first with codex " +
			"queue, so the reply cannot be queued into the Codex session; install codex 0.149.0 or later and run pm " +
			"service install from a shell where it is on PATH"},
	} {
		fakeCodex(t, c.version, c.answered)
		if got := PushInbox(&n, text); got != c.want {
			t.Errorf("%s: %q", name, got)
		}
	}
	fakeBin(t, nil)
	if got := PushInbox(&n, text); !strings.HasPrefix(got, "no codex: codex is not on the pm service's PATH") {
		t.Errorf("no codex: %q", got)
	}
}

// TestTheServiceQueuesAReplyIntoACodexSessionAndMarksItDelivered: a reply any command stores to a live Codex
// session's need is queued into that session with the text a Claude Code push sends, and marked delivered once codex
// said it queued it.
func TestTheServiceQueuesAReplyIntoACodexSessionAndMarksItDelivered(t *testing.T) {
	saved := SessionLive
	t.Cleanup(func() { SessionLive = saved })
	SessionLive = func(s string) bool { return s == rootThread }
	home := codexHome(t)
	w := newFakeWork(codexNeed(home))
	main := t.TempDir()
	log := fakeCodex(t, "0.154.0", rootThread)
	s := serveEvery(t, w, main, time.Hour)
	eventually(t, "the first snapshot", func() bool { return strings.Contains(s.log.String(), "refresh total=") })
	n := w.item("p-1.2.1")
	n.Comments = []work.Comment{{ID: "c1", Kind: work.Reply, Author: "owner", Text: "Yes.", CreatedAt: at}}
	text, _ := (Texts{}).Undelivered(&n) // what a Claude Code push of this need would send
	w.set(n)
	eventually(t, "marked delivered", func() bool { return w.item("p-1.2.1").Need.Delivered == 1 })
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "--message\n"+text+"\n") {
		t.Fatalf("codex was run as %q, want the message %q", b, text)
	}
}
