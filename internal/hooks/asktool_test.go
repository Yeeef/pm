package hooks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func transcript(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestToolAsksReadsTheLastTurnsAskToolCalls: the questions of the ask tool calls in the stopping turn, in each
// runtime's transcript as it records them (Codex 0.154's function_call of request_user_input_async, measured live, and
// of request_user_input; Claude Code's AskUserQuestion tool_use), and none from an earlier turn.
func TestToolAsksReadsTheLastTurnsAskToolCalls(t *testing.T) {
	codexAsync := `{"type":"response_item","payload":{"type":"function_call","name":"request_user_input_async",` +
		`"arguments":"{\"questions\":[{\"title\":\"Which color should the doc use?\",\"options\":[\"Red\",\"Blue\"]}]}","call_id":"c1"}}`
	codexPlan := `{"type":"response_item","payload":{"type":"function_call","name":"request_user_input",` +
		`"arguments":"{\"questions\":[{\"id\":\"q\",\"header\":\"Cache\",\"question\":\"Disk or memory?\",\"options\":[]}],\"isBlocking\":true}"}}`
	codexShell := `{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"echo request_user_input\"}"}}`
	turn := `{"type":"turn_context","payload":{"cwd":"/r"}}`
	claudeAsk := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion",` +
		`"input":{"questions":[{"question":"Red or blue?","header":"Color","options":[{"label":"Red"},{"label":"Blue"}],"multiSelect":false}]}}]}}`
	claudeResult := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"Blue"}]}}`
	claudePrompt := `{"type":"user","message":{"role":"user","content":"Go on."}}`
	claudeMeta := `{"type":"user","isMeta":true,"message":{"role":"user","content":"<command-name>/x</command-name>"}}`
	for _, c := range []struct {
		name  string
		lines []string
		want  []string
	}{
		{"codex async, this turn", []string{turn, codexShell, codexAsync}, []string{"Which color should the doc use?"}},
		{"codex plan mode", []string{turn, codexPlan}, []string{"Disk or memory?"}},
		{"codex, an earlier turn's", []string{turn, codexAsync, turn}, nil},
		{"claude, answered within the turn", []string{claudePrompt, claudeAsk, claudeResult, claudeMeta}, []string{"Red or blue?"}},
		{"claude, an earlier turn's", []string{claudePrompt, claudeAsk, claudeResult, claudePrompt}, nil},
		{"no ask tool", []string{claudePrompt, codexShell}, nil},
	} {
		if got := ToolAsks(transcript(t, c.lines...)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := ToolAsks(filepath.Join(t.TempDir(), "missing.jsonl")); got != nil {
		t.Errorf("a missing transcript: got %q", got)
	}
}
