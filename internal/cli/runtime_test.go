package cli

import (
	"os"
	"strings"
	"testing"
)

func TestSessionOfIsTheSetOneOrTheInnermostRuntimesWhenBothAre(t *testing.T) {
	rootOf := func(thread string) string {
		if thread == "child" {
			return "root"
		}
		return thread
	}
	for _, c := range []struct{ claude, session, thread, innermost, want, runtime string }{
		{"", "", "", "", "", ""},
		{"cl", "", "", "unasked", "cl", "claude"},
		{"", "root", "child", "unasked", "root", "codex"}, // Codex 0.154 names the root session itself
		{"", "", "child", "unasked", "root", "codex"},     // an earlier Codex: a subagent's thread is its root's session
		{"", "", "th", "unasked", "th", "codex"},
		{"cl", "root", "child", "codex", "root", "codex"}, // Codex started from a Claude Code session: Codex runs pm
		{"cl", "", "child", "codex", "root", "codex"},
		{"cl", "root", "child", "claude", "cl", "claude"}, // Claude Code started from Codex
	} {
		innermost := func() string {
			if c.innermost == "unasked" {
				t.Errorf("%+v: the ancestry was read with one runtime's session set", c)
			}
			return c.innermost
		}
		got, runtime, err := sessionOf(c.claude, c.session, c.thread, innermost, rootOf)
		if got != c.want || runtime != c.runtime || err != nil {
			t.Errorf("%+v: got %q %q %v", c, got, runtime, err)
		}
	}
	// both set and neither runtime among the ancestors: either could be this command's session
	_, _, err := sessionOf("cl", "root", "child", func() string { return "" }, rootOf)
	want := "both a Claude Code session ($CLAUDE_CODE_SESSION_ID cl) and a Codex session (root) are set, and no " +
		"Claude Code or Codex process runs this pm, so pm cannot tell which session this is: unset the one that is " +
		"not this session's ($CLAUDE_CODE_SESSION_ID, or $CODEX_SESSION_ID and $CODEX_THREAD_ID)"
	if err == nil || err.Error() != want {
		t.Errorf("ambiguous: %v", err)
	}
}

func TestSessionInboxIsClaudeCodesOnly(t *testing.T) {
	t.Setenv("CLAUDE_CODE_MESSAGING_SOCKET", "/run/claude.sock")
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "cl")
	t.Setenv("CODEX_SESSION_ID", "")
	if got := sessionInbox(); got != "/run/claude.sock" {
		t.Errorf("a Claude Code session: %q", got)
	}
	// a Codex run started from a Claude Code session inherits its socket, not its session id
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_SESSION_ID", "cx")
	if got := sessionInbox(); got != "" {
		t.Errorf("a Codex session got Claude Code's inbox %q", got)
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

// TestAnAmbiguousSessionRefusesEveryCommand: both runtimes' sessions set and neither runtime among the ancestors, a
// command refuses before anything runs, the self-parsing ones too (comment add would credit the note to the owner);
// a hook, help and version still run.
func TestAnAmbiguousSessionRefusesEveryCommand(t *testing.T) {
	saved := innermost
	innermost = func() string { return "" }
	t.Cleanup(func() { innermost = saved })
	t.Chdir(t.TempDir()) // no repo: a command that got past the refusal fails on that, never on a real store
	t.Setenv("CLAUDE_CODE_SESSION_ID", "cl")
	t.Setenv("CODEX_SESSION_ID", "cx")
	for _, argv := range [][]string{{"comment", "add", "x-1", "--text=a note"}, {"show", "x-1"},
		{"task", "add", "--parent", "x-1", "--title", "T"}, {"need", "dismiss", "x-1", "--reason", "r"}, {"show"}} {
		var out, errb strings.Builder
		if code := Execute(argv, strings.NewReader(""), &out, &errb); code != 1 ||
			!strings.Contains(errb.String(), "both a Claude Code session ($CLAUDE_CODE_SESSION_ID cl) and a Codex "+
				"session (cx) are set") {
			t.Errorf("%q: exit %d, stderr %q", argv, code, errb.String())
		}
	}
	var out, errb strings.Builder
	if code := Execute([]string{"version"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Errorf("version: exit %d %q", code, errb.String())
	}
}
