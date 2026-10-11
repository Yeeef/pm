package cli

import (
	"os"
	"testing"
)

func TestSessionOfIsTheSetOneOrTheInnermostRuntimesWhenBothAre(t *testing.T) {
	rootOf := func(thread string) string {
		if thread == "child" {
			return "root"
		}
		return thread
	}
	for _, c := range []struct{ claude, codex, innermost, want string }{
		{"", "", "", ""},
		{"cl", "", "unasked", "cl"},
		{"", "child", "unasked", "root"}, // a Codex subagent's thread is its root's session
		{"", "th", "unasked", "th"},
		{"cl", "child", "codex", "root"}, // Codex started from a Claude Code session: Codex runs pm
		{"cl", "child", "claude", "cl"},  // Claude Code started from Codex
		{"cl", "child", "", "cl"},        // no runtime among the ancestors: Claude Code's, as before
	} {
		innermost := func() string {
			if c.innermost == "unasked" {
				t.Errorf("%+v: the ancestry was read with one session set", c)
			}
			return c.innermost
		}
		if got := sessionOf(c.claude, c.codex, innermost, rootOf); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}

func TestInnermostRuntimeIsTheNearestClaudeCodeOrCodexAncestor(t *testing.T) {
	type proc struct {
		ppid  int
		argv0 string
	}
	const claudePID = 33
	tree := map[int]proc{
		100: {90, "pm"},
		90:  {85, "/bin/bash"},
		85:  {80, "/opt/codex/vendor/x86_64-unknown-linux-musl/bin/codex-linux-sandbox"},
		80:  {75, "/opt/codex/vendor/x86_64-unknown-linux-musl/bin/codex"},
		75:  {70, "node"}, // Codex's npm launcher
		70:  {60, "/bin/bash"},
		60:  {50, "/home/u/.local/share/claude/versions/2.1.296"},
		50:  {1, "-bash"},
		// Claude Code known by its pid alone (npm's launcher), and by its name
		40:        {35, "/bin/zsh"},
		35:        {claudePID, "pm"},
		claudePID: {1, "node"},
		30:        {1, "/usr/local/bin/claude"},
		// none
		20: {10, "/bin/sh"},
		10: {1, "tmux"},
		// an ancestor that cannot be read
		5: {4, "/bin/sh"},
	}
	info := func(pid int) (int, string, bool) {
		p, ok := tree[pid]
		return p.ppid, p.argv0, ok
	}
	for start, want := range map[int]string{100: "codex", 85: "codex", 75: "claude", 60: "claude", 40: "claude",
		30: "claude", 20: "", 5: "", 1: ""} {
		if got := innermostRuntime(start, info, claudePID); got != want {
			t.Errorf("from pid %d: got %q, want %q", start, got, want)
		}
	}
	if got := innermostRuntime(40, info, 0); got != "" { // without $CLAUDE_PID, npm's launcher is only node
		t.Errorf("without CLAUDE_PID: got %q", got)
	}
}

func TestReadProcReadsAProcesssParentAndArgv0(t *testing.T) {
	ppid, argv0, ok := readProc(os.Getpid())
	if !ok || ppid != os.Getppid() || argv0 != os.Args[0] {
		t.Fatalf("got %d %q %v, want %d %q", ppid, argv0, ok, os.Getppid(), os.Args[0])
	}
	if _, _, ok := readProc(1 << 30); ok {
		t.Error("a pid that does not exist was read")
	}
}
