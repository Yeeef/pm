package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Which agent runtime runs this pm, when the environment names a session of both: Codex inherits the environment of
// whatever started it, so a Codex run from a Claude Code session sees that session's CLAUDE_CODE_SESSION_ID beside its
// own CODEX_SESSION_ID and CODEX_THREAD_ID, and a Claude Code run from Codex the other way round. The innermost runtime is the one running
// pm: the nearest ancestor process that is Claude Code or Codex.

// procInfo is a process's parent pid and argv[0], and whether the process could be read.
type procInfo func(pid int) (ppid int, argv0 string, ok bool)

// runtimeOf is "claude" or "codex" when the process is Claude Code (its pid is $CLAUDE_PID, which Claude Code exports
// to its commands, or its executable is claude, or one of the native installer's claude/versions/<version>) or Codex
// (codex, or one of its helpers codex-*), else "".
func runtimeOf(pid int, argv0 string, claudePID int) string {
	base := filepath.Base(argv0)
	switch {
	case pid == claudePID || base == "claude" || strings.Contains(argv0, "/claude/versions/"):
		return "claude"
	case base == "codex" || strings.HasPrefix(base, "codex-"):
		return "codex"
	}
	return ""
}

// innermostRuntime is the runtime of the nearest of pid and its ancestors that is Claude Code or Codex, or "" when none
// is.
func innermostRuntime(pid int, info procInfo, claudePID int) string {
	for range 64 {
		if pid <= 1 {
			return ""
		}
		ppid, argv0, ok := info(pid)
		if !ok {
			return ""
		}
		if r := runtimeOf(pid, argv0, claudePID); r != "" {
			return r
		}
		pid = ppid
	}
	return ""
}

// readProc is a process's parent pid and argv[0]: from /proc on Linux, else from ps (macOS).
func readProc(pid int) (int, string, bool) {
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		// fields after the command name, which is in parentheses and may hold spaces: the parent pid is field 4
		i := bytes.LastIndexByte(stat, ')')
		if i < 0 {
			return 0, "", false
		}
		f := strings.Fields(string(stat[i+1:]))
		if len(f) < 2 {
			return 0, "", false
		}
		ppid, err := strconv.Atoi(f[1])
		if err != nil {
			return 0, "", false
		}
		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		argv0, _, _ := bytes.Cut(cmdline, []byte{0})
		return ppid, string(argv0), true
	} else if _, err := os.Stat("/proc/self/stat"); err == nil {
		return 0, "", false // a /proc without this pid: it is gone
	}
	// comm is the executable's full path on macOS, spaces and all, where command's first word would cut it
	out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", false
	}
	ppidText, path, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(ppidText)
	if err != nil {
		return 0, "", false
	}
	return ppid, strings.TrimSpace(path), true
}

// sessionOf is the session that runs this command and its runtime ("claude" or "codex"), from the environment's
// Claude Code session id (claude) and Codex's root session id (codexSession) and thread id (codexThread): the
// runtime's that is set, or, when both runtimes' are, the innermost runtime's. With both set and neither runtime
// among the ancestors it refuses: either could be this command's, and taking one would hold or answer for the other.
// Codex 0.154 sets CODEX_SESSION_ID, its root session's id in a subagent too; an earlier Codex sets only the thread's,
// which root maps to its root thread, so a subagent shares its session's id.
func sessionOf(claude, codexSession, codexThread string, innermost func() string,
	root func(string) string) (session, runtime string, err error) {
	if claude != "" && (codexSession != "" || codexThread != "") {
		switch innermost() {
		case "codex":
			claude = ""
		case "claude":
		default:
			codex := codexSession
			if codex == "" {
				codex = codexThread
			}
			return "", "", refuse("both a Claude Code session ($%s %s) and a Codex session (%s) are set, and no "+
				"Claude Code or Codex process runs this pm, so pm cannot tell which session this is: unset the one "+
				"that is not this session's ($%s, or $%s and $%s)", sessionEnv, claude, codex, sessionEnv,
				codexRootEnv, codexSessionEnv)
		}
	}
	switch {
	case claude != "":
		return claude, "claude", nil
	case codexSession != "":
		return codexSession, "codex", nil
	case codexThread != "":
		return root(codexThread), "codex", nil
	}
	return "", "", nil
}

// innermost is the runtime of the nearest Claude Code or Codex ancestor of this process, read once; a variable so a
// test can set the ancestry.
var innermost = sync.OnceValue(func() string { return innermostRuntime(os.Getppid(), readProc, claudePID()) })

// agentSession is this command's agent session and its runtime (sessionOf), from the environment.
func agentSession() (session, runtime string, err error) {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	return sessionOf(env(sessionEnv), env(codexRootEnv), env(codexSessionEnv), innermost, codexRoot)
}

// sessionInbox is the inbox the pm service pushes this session's replies into: Claude Code's messaging socket, and
// only when the session is Claude Code's. A Codex run started from a Claude Code session inherits that session's
// socket; a push there would land in the Claude Code session.
func sessionInbox() string {
	if _, runtime, err := agentSession(); err != nil || runtime != "claude" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(inboxEnv))
}
