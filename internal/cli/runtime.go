package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	out, err := exec.Command("ps", "-o", "ppid=,command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", false
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, "", false
	}
	return ppid, f[1], true
}

// sessionOf is the session that runs this command, from the environment's Claude Code session id (claude) and Codex's
// root session id (codexSession) and thread id (codexThread): the runtime's that is set, or, when both runtimes' are,
// the innermost runtime's (Claude Code's when no ancestor is either). Codex 0.154 sets CODEX_SESSION_ID, its root
// session's id in a subagent too; an earlier Codex sets only the thread's, which root maps to its root thread, so a
// subagent shares its session's id.
func sessionOf(claude, codexSession, codexThread string, innermost func() string, root func(string) string) string {
	if claude != "" && (codexSession != "" || codexThread != "") && innermost() == "codex" {
		claude = ""
	}
	switch {
	case claude != "":
		return claude
	case codexSession != "":
		return codexSession
	case codexThread != "":
		return root(codexThread)
	}
	return ""
}
