package session

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// LiveWindow is how recently a session's transcript must have changed for it to be live.
const LiveWindow = 30 * time.Minute

// sessionID is the form of a session or thread id pm looks a transcript up by.
var sessionID = regexp.MustCompile(`^[\w-]+$`)

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
func Live(session string) bool {
	if !sessionID.MatchString(session) {
		return false
	}
	paths := append(glob(filepath.Join(claudeConfigDir(), "projects", "*", session+".jsonl")), Rollouts(session)...)
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < LiveWindow {
			return true
		}
	}
	return false
}
