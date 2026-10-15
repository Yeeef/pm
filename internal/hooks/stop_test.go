package hooks

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/store"
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

// A page a follow stranded (its sprint closed without it) is named in the Stop hook's reason with what to do.
func TestTheStopHookSaysWhatToDoWithAStrandedPage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "none"))
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for p, text := range map[string]string{"design/p.md": "x\n", ".git/pm-stranded": "design/p.md\tpm-1\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reason := workspaceStopReason(store.Workspace{Dir: dir}, io.Discard)
	if !strings.Contains(reason, "- "+store.StrandedLine("design/p.md", "pm-1")) {
		t.Errorf("the reason does not name the stranded page and what to do:\n%s", reason)
	}
}
