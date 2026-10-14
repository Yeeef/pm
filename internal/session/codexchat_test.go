package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChatAfterCallIsTheFirstMessageTypedAfterTheCall: a UserMessage item (Codex 0.154) or a user_message event
// after the call's function_call line, across the thread's rollouts in order; nothing typed before the call, an agent
// message naming the call, or a missing call is no answer. A rollout's path names its CODEX_HOME.
func TestChatAfterCallIsTheFirstMessageTypedAfterTheCall(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, lines ...string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	typedItem := func(text string) string {
		return `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"` + text + `"}]}}}`
	}
	call := `{"type":"response_item","payload":{"type":"function_call","name":"request_user_input_async","call_id":"call_1"}}`
	agent := `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"call_1"}}}`
	first := write("a.jsonl", typedItem("Ask me."), call, agent)
	if got, ok := ChatAfterCall([]string{first}, "call_1"); ok {
		t.Errorf("nothing typed after the call: %q", got)
	}
	second := write("b.jsonl", `{"type":"event_msg","payload":{"type":"user_message","message":"Blue"}}`, typedItem("later"))
	if got, ok := ChatAfterCall([]string{first, second}, "call_1"); !ok || got != "Blue" {
		t.Errorf("typed in the next rollout: %q %v", got, ok)
	}
	if got, ok := ChatAfterCall([]string{write("c.jsonl", call, typedItem("Red"))}, "call_1"); !ok || got != "Red" {
		t.Errorf("typed after the call: %q %v", got, ok)
	}
	if got, ok := ChatAfterCall([]string{second}, "call_1"); ok {
		t.Errorf("no call: %q", got)
	}

	home, ok := CodexHomeOfRollout("/h/.codex/sessions/2026/10/14/rollout-2026-10-14T10-00-00-x.jsonl")
	if !ok || home != "/h/.codex" {
		t.Errorf("home of a rollout: %q %v", home, ok)
	}
	if _, ok := CodexHomeOfRollout("/tmp/t.jsonl"); ok {
		t.Error("a path that is no rollout's names a home")
	}
}
