package cli

import (
	"strings"
	"testing"
)

// TestSessionInboxIsTheRuntimesOwn: a Claude Code session's inbox is its messaging socket; a Codex session's is its
// CODEX_HOME, which the pm service queues the reply under, never the Claude Code socket it inherits.
func TestSessionInboxIsTheRuntimesOwn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
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
	if got := sessionInbox(); got != "codex-queue:"+home {
		t.Errorf("a Codex session's inbox: %q", got)
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
