package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Use is a live session's latest use of a worktree.
type Use struct {
	Session string
	At      time.Time
}

// Uses finds, for each of the worktrees under the main checkout's under/, the latest live session that used it: an entry of a
// Claude Code transcript or a Codex rollout written within LiveWindow (pm's notion of a live session) whose cwd is in
// the worktree, or whose tool call names its path (a subagent runs in its parent's directory and reaches its worktree
// by path). The path's form relative to the main checkout counts when the entry's cwd is in the main checkout. The
// output of a tool call does not count, so listing worktrees, or pm clean's own output, uses none of them. A Codex
// rollout's cwd is its session_meta's and then each turn_context's, and its session the root thread's (CodexMeta).
func Uses(main string, worktrees []string, under string) (map[string]Use, error) {
	u := &uses{main: main, now: time.Now(), found: map[string]Use{}}
	for _, wt := range worktrees {
		if rel, err := filepath.Rel(main, wt); err == nil && strings.HasPrefix(rel, under+"/") {
			u.trees = append(u.trees, tree{wt, rel})
		}
	}
	if len(u.trees) == 0 {
		return u.found, nil
	}
	err := recentLines(filepath.Join(claudeConfigDir(), "projects"), u.now, func(name string) bool {
		return strings.HasSuffix(name, ".jsonl")
	}, func(_ bool, line []byte) { u.claudeLine(line) })
	if err != nil {
		return u.found, err
	}
	var c codexScan
	err = recentLines(filepath.Join(CodexHome(), "sessions"), u.now, func(name string) bool {
		return strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl")
	}, func(first bool, line []byte) { c.line(u, first, line) })
	return u.found, err
}

// tree is a worktree Uses looks for: its path, and the path relative to the main checkout.
type tree struct{ path, rel string }

// uses is what Uses has found so far.
type uses struct {
	main  string
	now   time.Time
	trees []tree
	found map[string]Use
}

// note is one entry: its session, its cwd, its time and its tool calls' inputs, what the session did, never what it
// was shown.
func (u *uses) note(session, cwd, stamp string, calls []byte) {
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || u.now.Sub(at) >= LiveWindow {
		return
	}
	inMain := cwd == u.main || strings.HasPrefix(cwd, u.main+"/")
	for _, t := range u.trees {
		used := cwd == t.path || strings.HasPrefix(cwd, t.path+"/") || namesPath(calls, t.path) ||
			(inMain && namesPath(calls, t.rel))
		if used && at.After(u.found[t.path].At) {
			u.found[t.path] = Use{session, at}
		}
	}
}

// claudeLine is one line of a Claude Code transcript.
func (u *uses) claudeLine(line []byte) {
	var entry struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Cwd       string `json:"cwd"`
		SessionID string `json:"sessionId"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return // not an entry: Claude Code writes one JSON object per line
	}
	var calls []byte
	if entry.Type == "assistant" {
		var content []struct {
			Type  string          `json:"type"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(entry.Message.Content, &content) == nil {
			for _, c := range content {
				if c.Type == "tool_use" {
					calls = append(append(calls, c.Input...), '\n')
				}
			}
		}
	}
	u.note(entry.SessionID, entry.Cwd, entry.Timestamp, calls)
}

// codexScan is where a scan of a Codex rollout is: its root thread, from its first line's session_meta, and its latest
// cwd.
type codexScan struct{ session, cwd string }

// line is one line of a Codex rollout; first starts a rollout.
func (c *codexScan) line(u *uses, first bool, line []byte) {
	if first {
		c.session, c.cwd = "", ""
		if meta, ok := ParseCodexMeta(line); ok {
			c.session = meta.Root()
		}
	}
	var entry struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Payload   struct {
			Type      string          `json:"type"`
			Cwd       string          `json:"cwd"`
			Arguments json.RawMessage `json:"arguments"`
			Input     json.RawMessage `json:"input"`
			Action    json.RawMessage `json:"action"`
		} `json:"payload"`
	}
	if c.session == "" || json.Unmarshal(line, &entry) != nil {
		return // a rollout opens with its session_meta, and Codex writes one JSON object per line
	}
	var calls []byte
	switch entry.Type + "/" + entry.Payload.Type {
	case "session_meta/", "turn_context/":
		c.cwd = entry.Payload.Cwd
	case "response_item/function_call":
		calls = jsonText(entry.Payload.Arguments)
	case "response_item/custom_tool_call":
		calls = jsonText(entry.Payload.Input)
	case "response_item/local_shell_call":
		calls = entry.Payload.Action
	}
	u.note(c.session, c.cwd, entry.Timestamp, calls)
}

// recentLines calls line with each line of each file under dir that name picks and that changed within LiveWindow,
// first for a file's first line. A dir that does not exist holds no file.
func recentLines(dir string, now time.Time, name func(string) bool, line func(first bool, b []byte)) error {
	return filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll // no transcripts at all
			}
			return err
		}
		if e.IsDir() || !name(e.Name()) {
			return nil
		}
		st, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil // removed since it was listed
		} else if err != nil {
			return err
		}
		if now.Sub(st.ModTime()) >= LiveWindow {
			return nil
		}
		f, err := os.Open(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for first := true; sc.Scan(); first = false {
			line(first, sc.Bytes())
		}
		if err := errors.Join(sc.Err(), f.Close()); err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		return nil
	})
}

// jsonText is a JSON string's text, or any other JSON value as it is.
func jsonText(raw json.RawMessage) []byte {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []byte(s)
	}
	return raw
}

// namesPath is whether text names path itself or a path under it: an occurrence not followed by a name character, so
// .claude/worktrees/a names neither .claude/worktrees/ab nor .claude/worktrees/a.b.
func namesPath(text []byte, path string) bool {
	for rest := text; ; {
		i := bytes.Index(rest, []byte(path))
		if i < 0 {
			return false
		}
		rest = rest[i+len(path):]
		if len(rest) == 0 || !isNameByte(rest[0]) {
			return true
		}
	}
}

func isNameByte(b byte) bool {
	return b == '.' || b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
		b >= 0x80
}
