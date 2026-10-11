package session

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Yeeef/pm/internal/install"
)

// Codex keeps each thread's transcript, its rollout, at $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<thread>.jsonl,
// in the directory of the thread's start date in local time. The rollout's first line is the thread's session_meta:
// its id, and for a subagent's thread, its root thread's id as session_id and its parent's as parent_thread_id. A
// subagent is a thread of its own, with its own CODEX_THREAD_ID; Codex 0.154 also sets CODEX_SESSION_ID, the root
// thread's id, which pm prefers.

// Rollouts is the paths of thread's rollouts. A Codex thread id is a UUIDv7, whose first 48 bits are its start in
// Unix milliseconds, so only the date directories that start can be in, in any time zone, are read; for another id,
// every date directory is.
func Rollouts(thread string) []string {
	if !sessionID.MatchString(thread) {
		return nil
	}
	sessions := filepath.Join(install.CodexHome(), "sessions")
	name := "rollout-*-" + thread + ".jsonl"
	start, ok := uuid7Start(thread)
	if !ok {
		return glob(filepath.Join(sessions, "*", "*", "*", name))
	}
	var paths []string
	seen := map[string]bool{}
	for _, zone := range []time.Duration{-12 * time.Hour, 0, 14 * time.Hour} { // UTC-12 to UTC+14 span these dates
		day := start.Add(zone).UTC().Format("2006/01/02")
		if !seen[day] {
			seen[day] = true
			paths = append(paths, glob(filepath.Join(sessions, filepath.FromSlash(day), name))...)
		}
	}
	return paths
}

// uuid7Start is the start time a UUIDv7 holds, and whether id is one.
func uuid7Start(id string) (time.Time, bool) {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[14] != '7' || id[18] != '-' || id[23] != '-' {
		return time.Time{}, false
	}
	b, err := hex.DecodeString(id[0:8] + id[9:13])
	if err != nil {
		return time.Time{}, false
	}
	var ms int64
	for _, x := range b {
		ms = ms<<8 | int64(x)
	}
	return time.UnixMilli(ms), true
}

// CodexMeta is a rollout's session_meta payload, as far as pm reads it.
type CodexMeta struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	ParentThreadID string `json:"parent_thread_id"`
	Cwd            string `json:"cwd"`
}

// Root is the thread's root thread: session_id, else the parent thread, else the thread itself.
func (m CodexMeta) Root() string {
	for _, id := range []string{m.SessionID, m.ParentThreadID} {
		if sessionID.MatchString(id) {
			return id
		}
	}
	return m.ID
}

// glob is the paths matching pattern. Its only error is a malformed pattern, which only a directory name holding glob
// syntax ($CODEX_HOME, Claude Code's config dir) makes: no transcript is found there, as none can be matched.
func glob(pattern string) []string {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return paths
}

// readCodexMeta is the session_meta on the rollout's first line, and whether that line is one.
func readCodexMeta(path string) (CodexMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return CodexMeta{}, false
	}
	line, err := bufio.NewReader(f).ReadBytes('\n')
	if cerr := f.Close(); cerr != nil || len(line) == 0 && err != nil {
		return CodexMeta{}, false
	}
	return ParseCodexMeta(line)
}

// ParseCodexMeta is the session_meta a rollout line holds, and whether it holds one.
func ParseCodexMeta(line []byte) (CodexMeta, bool) {
	var entry struct {
		Type    string    `json:"type"`
		Payload CodexMeta `json:"payload"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "session_meta" || entry.Payload.ID == "" {
		return CodexMeta{}, false
	}
	return entry.Payload, true
}

// CodexRoot is the root thread of a Codex thread, read from the session_meta of its rollout, the session_meta of its
// parent's rollout, and so on: a subagent's thread maps to its root's, so it shares its root session's id, as a Claude
// Code subagent shares its session's. A thread whose rollout pm cannot read is its own root.
func CodexRoot(thread string) string {
	for range 8 { // a subagent's own subagents nest a few deep, never more
		paths := Rollouts(thread)
		if len(paths) == 0 {
			return thread
		}
		meta, ok := readCodexMeta(paths[0])
		if !ok || meta.ID != thread {
			return thread
		}
		next := meta.Root()
		if next == thread {
			return thread
		}
		thread = next
	}
	return thread
}
