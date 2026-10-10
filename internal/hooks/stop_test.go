package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTouchedCountsASubagentsPathOnceItsCallReturns(t *testing.T) {
	start := `{"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "toolu_a", "name": "Task", "input": {"prompt": "edit sprints/x.md"}}]}}`
	launched := `{"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "toolu_a", "content": "launched"}]}, "toolUseResult": {"isAsync": true, "status": "async_launched"}}`
	for name, c := range map[string]struct {
		lines []string
		want  bool
	}{
		"running":           {[]string{start}, false},
		"launched":          {[]string{start, launched}, false},
		"returned in place": {[]string{start, `{"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "toolu_a", "content": "done"}]}, "toolUseResult": {"status": "completed"}}`}, true},
		"notified":          {[]string{start, launched, `{"type": "user", "message": {"content": [{"type": "text", "text": "<task-notification><tool-use-id>toolu_a</tool-use-id><status>failed</status></task-notification>"}]}}`}, true},
		"queued":            {[]string{start, launched, `{"type": "queue-operation", "operation": "enqueue", "content": "<task-notification>\n<tool-use-id>toolu_a</tool-use-id>\n</task-notification>"}`}, true},
		"another call":      {[]string{start, launched, `{"type": "user", "message": {"content": "<task-notification><tool-use-id>toolu_b</tool-use-id></task-notification>"}}`}, false},
	} {
		path := filepath.Join(t.TempDir(), "t.jsonl")
		if err := os.WriteFile(path, []byte(strings.Join(c.lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Touched(path, []string{"sprints/x.md"})
		if err != nil {
			t.Fatal(err)
		}
		if (len(got) == 1) != c.want {
			t.Errorf("%s: touched %v, want %v", name, got, c.want)
		}
	}
}
