package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Thread ids from a real Codex 0.154 run: a root thread and the subagent it spawned, both started 2026-10-11 UTC.
const (
	rootThread  = "01a12beb-06b9-7f40-8ecb-6468bfec7783"
	childThread = "01a12beb-1c93-7450-8bb3-46ac7a13452f"
)

// rollout writes a Codex rollout of thread under $CODEX_HOME/sessions/<day>/ whose first line is its session_meta
// (meta, with the thread's id) and the rest lines; day "" is the thread's start date in local time, as Codex names it.
func rollout(t *testing.T, thread, day string, meta map[string]any, lines ...map[string]any) string {
	t.Helper()
	if day == "" {
		start, ok := uuid7Start(thread)
		if !ok {
			t.Fatalf("%s is no UUIDv7", thread)
		}
		day = start.Local().Format("2006/01/02")
	}
	dir := filepath.Join(os.Getenv("CODEX_HOME"), "sessions", filepath.FromSlash(day))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"id": thread, "cwd": "/somewhere", "originator": "codex_exec"}
	for k, v := range meta {
		payload[k] = v
	}
	var text []byte
	for _, l := range append([]map[string]any{{"timestamp": "2026-10-11T17:03:11.001Z", "type": "session_meta",
		"payload": payload}}, lines...) {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		text = append(append(text, b...), '\n')
	}
	p := filepath.Join(dir, "rollout-2026-10-11T19-03-05-"+thread+".jsonl")
	if err := os.WriteFile(p, text, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUUID7StartIsTheMillisecondsItsFirst48BitsHold(t *testing.T) {
	// the rollout of this thread is rollout-2026-10-11T18-59-35-<id>.jsonl, written at UTC+2
	got, ok := uuid7Start("01a12be7-d22e-72f2-8332-1b979a2b8111")
	if want := time.Date(2026, 10, 11, 16, 59, 35, 342e6, time.UTC); !ok || !got.Equal(want) {
		t.Fatalf("got %v %v, want %v", got, ok, want)
	}
	for _, id := range []string{"th1", "01a12be7-d22e-42f2-8332-1b979a2b8111", "01a12be7-d22e-72f2-8332-1b979a2b811",
		"0za12be7-d22e-72f2-8332-1b979a2b8111"} {
		if _, ok := uuid7Start(id); ok {
			t.Errorf("%s: read as a UUIDv7", id)
		}
	}
}

func TestCodexRootMapsASubagentThreadToItsRootThread(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	rollout(t, rootThread, "", map[string]any{"session_id": rootThread})
	rollout(t, childThread, "", map[string]any{"session_id": rootThread, "parent_thread_id": rootThread,
		"forked_from_id": rootThread, "thread_source": "subagent"})
	// a subagent's subagent whose session_meta names only its parent, in the date directory of UTC+14
	grand := "01a12bec-5489-7250-8c17-2fbbaabb7d9d"
	start, _ := uuid7Start(grand)
	rollout(t, grand, start.Add(14*time.Hour).UTC().Format("2006/01/02"), map[string]any{"parent_thread_id": childThread})
	for thread, want := range map[string]string{rootThread: rootThread, childThread: rootThread, grand: rootThread,
		"01a12bed-14dd-7020-97d5-c7a5f615de95": "01a12bed-14dd-7020-97d5-c7a5f615de95"} { // no rollout: its own root
		if got := codexRoot(thread); got != want {
			t.Errorf("%s: got %s, want %s", thread, got, want)
		}
	}
}

func TestCurrentSessionOfACodexSubagentIsItsRootThread(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", childThread)
	rollout(t, rootThread, "", map[string]any{"session_id": rootThread})
	rollout(t, childThread, "", map[string]any{"session_id": rootThread, "parent_thread_id": rootThread})
	if got := currentSession(); got != rootThread {
		t.Fatalf("got %s, want the root thread %s", got, rootThread)
	}
}

func TestCodexRolloutsReadsOnlyTheDatesAThreadCanHaveStarted(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	p := rollout(t, rootThread, "", nil)
	if !live(rootThread) {
		t.Fatal("a Codex thread whose rollout was just written is not live")
	}
	if got := codexRollouts(rootThread); len(got) != 1 || got[0] != p {
		t.Fatalf("got %v", got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	rollout(t, rootThread, "2026/10/20", nil) // a date the thread cannot have started on
	if got := codexRollouts(rootThread); len(got) != 0 {
		t.Fatalf("read a date directory the thread did not start on: %v", got)
	}
}
