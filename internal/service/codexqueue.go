package service

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/launch"
)

// CodexQueueTimeout bounds one codex queue call; a variable so tests can shorten it.
var CodexQueueTimeout = 10 * time.Second

// CodexQueueSince is the first codex with codex queue, and with queued messages that wake an idle session reliably
// (codex-cli 0.149.0's release notes, 2026-08-20).
const CodexQueueSince = "0.149.0"

// codexVersions is each codex binary's version as `codex --version` said it, read once per service run.
var codexVersions sync.Map

// CodexVersion is the version codex (an executable's path) says it is, as `codex-cli X` gives it; "" when it says
// none.
func CodexVersion(codex string) string {
	if v, ok := codexVersions.Load(codex); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, codex, "--version").Output()
	v := ""
	if err == nil {
		if f := strings.Fields(string(out)); len(f) > 0 {
			v = f[len(f)-1]
		}
	}
	codexVersions.Store(codex, v)
	return v
}

// CodexUsable is the codex on this process's PATH, or why it cannot queue a reply (CodexOn).
func CodexUsable() (codex, why string) { return CodexOn(os.Getenv("PATH")) }

// CodexOn is the codex on path, or why it cannot queue a reply: missing, or older than CodexQueueSince.
func CodexOn(path string) (codex, why string) {
	for _, d := range filepath.SplitList(path) {
		if p := filepath.Join(d, "codex"); d != "" && onPath("codex", d) {
			codex = p
			break
		}
	}
	if codex == "" {
		return "", "codex is not on the pm service's PATH"
	}
	if v := CodexVersion(codex); !launch.Newer(v, CodexQueueSince) && v != CodexQueueSince {
		if v == "" {
			v = "of no known version"
		}
		return "", "codex " + v + " is older than " + CodexQueueSince + ", the first with codex queue"
	}
	return codex, ""
}

// CodexDrift is pm doctor's line on the pm service's codex, for a repo whose main checkout has Codex hooks: the codex
// on the service unit's PATH must queue replies into Codex sessions. "" when it can, or when no unit is installed.
func CodexDrift(main string) string {
	if _, err := os.Stat(filepath.Join(main, ".codex", "hooks.json")); err != nil {
		return ""
	}
	path, ok := UnitPath(main, PlatformKind())
	if !ok {
		return ""
	}
	if _, why := CodexOn(path); why != "" {
		return fmt.Sprintf("%s, so owner replies cannot be queued into Codex sessions (they wait for their next "+
			"prompt); install codex %s or later and run pm service install from a shell where it is on PATH", why,
			CodexQueueSince)
	}
	return ""
}

// queueCodex queues text into the Codex session thread, the root thread a Codex session's pm commands run as, with
// codex queue under home, the CODEX_HOME the session raised its need with: codex 0.154 writes it to that home's thread
// queue (thread/queue/add), and the session's app-server, a TUI's embedded one included, starts a turn with it once
// the thread is idle (within its 10 s poll), or when the thread is next loaded. When a local app-server daemon
// answers on home's control socket, the call goes to it (--remote): codex refuses an embedded call while one runs.
// The thread is the session's UUID, never a name (a name resolves to the most recent session that has it). "" once
// codex said it queued the message for that thread; else "no codex: …" (missing, or older than CodexQueueSince) or
// "failed: …".
func queueCodex(home, thread, text string) string {
	codex, why := CodexUsable()
	if why != "" {
		return noCodex(why)
	}
	args := []string{"queue", "--thread", thread, "--message", text}
	sock := filepath.Join(home, "app-server-control", "app-server-control.sock")
	if conn, err := net.DialTimeout("unix", sock, time.Second); err == nil {
		if err := conn.Close(); err != nil {
			return "failed: " + err.Error()
		}
		args = append(args, "--remote", "unix://"+sock)
	}
	ctx, cancel := context.WithTimeout(context.Background(), CodexQueueTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, codex, args...)
	cmd.Dir = home // no project's .codex/ applies to the call
	cmd.Env = append(withoutEnv(os.Environ(), "CODEX_HOME"), "CODEX_HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	switch {
	case ctx.Err() != nil:
		return "failed: codex queue gave no answer within " + CodexQueueTimeout.String()
	case err != nil:
		return "failed: codex queue: " + firstLine(stderr.String(), err.Error())
	}
	// "Queued message <id> for thread <uuid>.": the thread must be the one asked for
	said := firstLine(stdout.String(), "nothing")
	if m := queued.FindStringSubmatch(said); len(m) < 2 || m[1] != thread {
		return "failed: codex queue said: " + said
	}
	return ""
}

// noCodex is PushInbox's reason when codex cannot queue a reply: why, as CodexUsable says it.
func noCodex(why string) string {
	return "no codex: " + why + ", so the reply cannot be queued into the Codex session; install codex " +
		CodexQueueSince + " or later and run pm service install from a shell where it is on PATH"
}

var queued = regexp.MustCompile(`^Queued message \S+ for thread (\S+?)\.?$`)

// firstLine is s's first non-blank line, or otherwise.
func firstLine(s, otherwise string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return otherwise
}

// withoutEnv is env without key's entries.
func withoutEnv(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}
