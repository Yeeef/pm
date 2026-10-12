package session

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

// rollout writes a Codex rollout of thread under $CODEX_HOME/sessions/<day>/ whose line is its session_meta (meta,
// with the thread's id); day "" is the thread's start date in local time, as Codex names it.
func rollout(t *testing.T, thread, day string, meta map[string]any) string {
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
	text, err := json.Marshal(map[string]any{"timestamp": "2026-10-11T17:03:11.001Z", "type": "session_meta",
		"payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	text = append(text, '\n')
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
		if got := CodexRoot(thread); got != want {
			t.Errorf("%s: got %s, want %s", thread, got, want)
		}
	}
}

func TestCurrentSessionOfACodexSubagentIsItsRootThread(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_SESSION_ID", "") // an earlier Codex sets only the thread's id
	t.Setenv("CODEX_THREAD_ID", childThread)
	rollout(t, rootThread, "", map[string]any{"session_id": rootThread})
	rollout(t, childThread, "", map[string]any{"session_id": rootThread, "parent_thread_id": rootThread})
	if got := current(t); got != rootThread {
		t.Fatalf("got %s, want the root thread %s", got, rootThread)
	}
	t.Setenv("CODEX_SESSION_ID", rootThread) // Codex 0.154 names it, with no rollout read
	t.Setenv("CODEX_HOME", t.TempDir())
	if got := current(t); got != rootThread {
		t.Fatalf("with CODEX_SESSION_ID: got %s, want %s", got, rootThread)
	}
}

func TestCodexRolloutsReadsOnlyTheDatesAThreadCanHaveStarted(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	p := rollout(t, rootThread, "", nil)
	if !Live(rootThread) {
		t.Fatal("a Codex thread whose rollout was just written is not live")
	}
	if got := Rollouts(rootThread); len(got) != 1 || got[0] != p {
		t.Fatalf("got %v", got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	rollout(t, rootThread, "2026/10/20", nil) // a date the thread cannot have started on
	if got := Rollouts(rootThread); len(got) != 0 {
		t.Fatalf("read a date directory the thread did not start on: %v", got)
	}
}

// current is the session the environment names, with no runtime among the ancestors.
func current(t *testing.T) string {
	t.Helper()
	id, _, err := Current(func() string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestCodexLiveLooksUnderTheHomeItIsGiven: the pm service asks whether a Codex session is live under the CODEX_HOME
// its need stores, whatever its own CODEX_HOME is.
func TestCodexLiveLooksUnderTheHomeItIsGiven(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	rollout(t, rootThread, "", nil)
	t.Setenv("CODEX_HOME", t.TempDir()) // the service's own
	if !CodexLive(home, rootThread) || CodexLive(os.Getenv("CODEX_HOME"), rootThread) {
		t.Fatal("liveness was read under another home")
	}
}

// TestCodexHooksRanReadsTheRolloutUpToTheFirstPrompt: the opening of a real Codex 0.154 rollout, with and without
// pm's session-start context, and one whose first prompt is not written yet.
func TestCodexHooksRanReadsTheRolloutUpToTheFirstPrompt(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	msg := func(role string, texts ...string) map[string]any {
		var content []map[string]any
		for _, x := range texts {
			content = append(content, map[string]any{"type": "input_text", "text": x})
		}
		return map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": role,
			"content": content}}
	}
	codexContext := []map[string]any{
		{"type": "event_msg", "payload": map[string]any{"type": "task_started"}},
		msg("developer", "<permissions instructions>\n…"),
		// Codex's user context: plugins, the user's AGENTS.md (which may name pm's rules), the environment
		msg("user", "<recommended_plugins>\n…", "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n# pm rules (quoted)\n", "<environment_context>…"),
		{"type": "turn_context", "payload": map[string]any{"cwd": "/repo"}},
	}
	prompt := msg("user", "Run pm show.")
	write := func(lines []map[string]any) {
		path := rollout(t, rootThread, "", nil)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			b, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(append(b, '\n')); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name       string
		lines      []map[string]any
		ran, known bool
	}{
		{"hooks ran", append(append([]map[string]any{}, codexContext...),
			msg("developer", RulesMark+"1 of 3): the introduction\n…"), msg("developer", "state …"), prompt), true, true},
		{"hooks skipped", append(append([]map[string]any{}, codexContext...), prompt), false, true},
		{"first prompt not written yet", codexContext, false, false},
	} {
		write(c.lines)
		if ran, known := CodexHooksRan(rootThread); ran != c.ran || known != c.known {
			t.Errorf("%s: got ran %v known %v, want %v %v", c.name, ran, known, c.ran, c.known)
		}
	}
	if _, known := CodexHooksRan(childThread); known {
		t.Error("a thread with no rollout: known")
	}
}
