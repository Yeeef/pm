package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A Codex session's ask-tool question (request_user_input_async) returns at once; the owner answers it by typing into
// the session, which the rollout records as a user message after the call. ChatAfterCall finds that answer, so the
// pm service does not also queue the site's answer into the session.

// ChatAfterCall is the first message the user typed into the session after the function call whose call_id is call,
// read from the rollouts at paths in order: a UserMessage item (Codex 0.154), or a user_message event (earlier Codex),
// leaving out each message relayed says is not the user's own (one the pm service queued into the session); false when
// the call is not there or nothing was typed after it.
func ChatAfterCall(paths []string, call string, relayed func(string) bool) (string, bool) {
	seen := false
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		r := bufio.NewReader(f)
		for {
			line, err := r.ReadBytes('\n')
			if !seen && strings.Contains(string(line), call) {
				seen = callLine(line, call)
			} else if seen {
				if text, ok := typed(line); ok && !relayed(text) {
					_ = f.Close() //nolint:errcheck // read only
					return text, true
				}
			}
			if err != nil {
				break
			}
		}
		_ = f.Close() //nolint:errcheck // read only
	}
	return "", false
}

// callLine is whether the rollout line is the function call call.
func callLine(line []byte, call string) bool {
	var e struct {
		Type    string `json:"type"`
		Payload struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		} `json:"payload"`
	}
	return json.Unmarshal(line, &e) == nil && e.Type == "response_item" && e.Payload.Type == "function_call" &&
		e.Payload.CallID == call
}

// typed is the text of a rollout line that records a message the user typed.
func typed(line []byte) (string, bool) {
	s := string(line)
	if !strings.Contains(s, "UserMessage") && !strings.Contains(s, "user_message") {
		return "", false
	}
	var e struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Item    struct {
				Type    string `json:"type"`
				Content []struct{ Type, Text string }
			} `json:"item"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &e) != nil || e.Type != "event_msg" {
		return "", false
	}
	switch {
	case e.Payload.Type == "user_message":
		return e.Payload.Message, true
	case e.Payload.Type == "item_completed" && e.Payload.Item.Type == "UserMessage":
		var parts []string
		for _, c := range e.Payload.Item.Content {
			if c.Type == "text" {
				parts = append(parts, c.Text)
			}
		}
		return strings.Join(parts, "\n"), true
	}
	return "", false
}

var rolloutPath = regexp.MustCompile(`/sessions/\d{4}/\d\d/\d\d/rollout-[^/]+\.jsonl$`)

// CodexHomeOfRollout is the CODEX_HOME a rollout's path lies under (<home>/sessions/YYYY/MM/DD/rollout-….jsonl), and
// whether it lies under one.
func CodexHomeOfRollout(path string) (string, bool) {
	if !rolloutPath.MatchString(filepath.ToSlash(path)) {
		return "", false
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path))))), true
}
