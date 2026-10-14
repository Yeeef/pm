package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LiveWindow is how recently a session's transcript must have changed for it to be live.
const LiveWindow = 30 * time.Minute

// sessionID is the form of a session or thread id pm looks a transcript up by.
var sessionID = regexp.MustCompile(`^[\w-]+$`)

// CodexHome is $CODEX_HOME, else ~/.codex: where Codex keeps its config and its rollouts.
func CodexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(home(), ".codex")
}

// codexInbox is how a need's inbox names the Codex route: the pm service queues the reply into the session with
// codex queue under that CODEX_HOME. A Claude Code inbox is its messaging socket's path, which never starts so.
const codexInbox = "codex-queue:"

// CodexInbox is the inbox of a Codex session whose CODEX_HOME is home.
func CodexInbox(home string) string { return codexInbox + home }

// CodexLive is whether one of thread's rollouts under the CODEX_HOME home changed within LiveWindow: the pm service
// asks it of a Codex session by the home its need stores, not by its own.
func CodexLive(home, thread string) bool {
	for _, p := range RolloutsIn(home, thread) {
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < LiveWindow {
			return true
		}
	}
	return false
}

// Inbox is the inbox the pm service pushes the replies of a session of runtime into: Claude Code's messaging socket
// for a Claude Code session (a Codex run started from one inherits that socket, and a push there would land in the
// Claude Code session); for a Codex session, its CODEX_HOME, under which the service queues the reply (codex queue);
// "" for none.
func Inbox(runtime string) string {
	switch runtime {
	case "codex":
		if home, err := filepath.Abs(CodexHome()); err == nil {
			return CodexInbox(home)
		}
	case "claude":
		return strings.TrimSpace(os.Getenv("CLAUDE_CODE_MESSAGING_SOCKET"))
	}
	return ""
}

// CodexHomeOf is the CODEX_HOME a Codex inbox names, and whether inbox is one.
func CodexHomeOf(inbox string) (string, bool) { return strings.CutPrefix(inbox, codexInbox) }

// home is $HOME, else the user's home directory, else "" (no transcript is found under it).
func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// claudeConfigDir is Claude Code's config dir, which holds its transcripts under projects/.
func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude" // no home: no transcript is found
	}
	return filepath.Join(home, ".claude")
}

// Live is whether one of the session's transcripts changed within LiveWindow: Claude Code's
// <config>/projects/<project dir>/<id>.jsonl (any project dir, since each worktree has its own), or Codex's rollout of
// the thread (Rollouts).
func Live(session string) bool { return activeWithin(session, LiveWindow) }

// activeWithin is whether one of the session's transcripts (Live) changed within window.
func activeWithin(session string, window time.Duration) bool {
	if !sessionID.MatchString(session) {
		return false
	}
	paths := append(glob(filepath.Join(claudeConfigDir(), "projects", "*", session+".jsonl")), Rollouts(session)...)
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < window {
			return true
		}
	}
	return false
}
