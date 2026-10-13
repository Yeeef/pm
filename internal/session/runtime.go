// Package session is which agent session runs pm and which sessions are live: the session's id from the environment
// (Claude Code's, or Codex's root thread's) and the runtime that runs pm, read from its ancestry; a session's
// transcripts (Claude Code's under its config dir, Codex's rollouts under $CODEX_HOME) and what they say it did.
//
// The files: runtime.go the session from the environment (CLAUDE_CODE_SESSION_ID, Codex's CODEX_SESSION_ID, else
// CODEX_THREAD_ID mapped to its root thread through its rollout) and the innermost runtime from the process ancestry
// when both are set; codex.go Codex's rollouts and whether pm's session-start hooks ran in a Codex session
// (CodexHooksRan); codexhooks.go the warning a pm command gives once per session when they did not; live.go a session's
// liveness; uses.go the worktrees live sessions used, for pm clean.
package session

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
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return 0, "", false
		}
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
			return "", "", fmt.Errorf("both a Claude Code session ($%s %s) and a Codex session (%s) are set, and "+
				"no Claude Code or Codex process runs this pm, so pm cannot tell which session this is: unset the one "+
				"that is not this session's ($%s, or $%s and $%s)", ClaudeEnv, claude, codex, ClaudeEnv,
				CodexRootEnv, CodexThreadEnv)
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

// The environment variables that name a session.
const (
	ClaudeEnv      = "CLAUDE_CODE_SESSION_ID" // Claude Code exports it to every command a session runs
	CodexThreadEnv = "CODEX_THREAD_ID"        // Codex exports it to every command a thread runs
	CodexRootEnv   = "CODEX_SESSION_ID"       // Codex 0.154 exports its root session's id, a subagent's too
)

var innermostOnce = sync.OnceValue(func() string { return innermostRuntime(os.Getppid(), readProc, claudePID()) })

// Innermost is the runtime of the nearest Claude Code or Codex ancestor of this process ("claude", "codex" or ""),
// read once.
func Innermost() string { return innermostOnce() }

// Current is this command's agent session and its runtime (sessionOf), from the environment, with innermost as the
// runtime the ancestry names.
func Current(innermost func() string) (session, runtime string, err error) {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	return sessionOf(env(ClaudeEnv), env(CodexRootEnv), env(CodexThreadEnv), innermost, CodexRoot)
}

// claudePID is $CLAUDE_PID, Claude Code's own pid, which it exports to its commands; 0 when unset.
func claudePID() int {
	pid, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CLAUDE_PID")))
	if err != nil {
		return 0
	}
	return pid
}
