package ask

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fake puts an executable script named name on a PATH of its own, with sh and the coreutils the scripts use.
func fake(t *testing.T, name, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestCodexRunsNoHooksNoRulesNoUserConfigFromATempDir: the judge runs inside a Stop hook, so a child Codex that ran
// hooks would run the judge again. The call turns hooks, rules and the user's config off, saves no session, runs from
// an empty directory outside any repo, gives its instructions as a file, and reads the answer from the last message,
// not from Codex's progress on stdout.
func TestCodexRunsNoHooksNoRulesNoUserConfigFromATempDir(t *testing.T) {
	log := t.TempDir()
	fake(t, "codex", `pwd > "`+log+`/cwd"; ls -A > "`+log+`/ls"; printf '%s\n' "$@" > "`+log+`/args"
cat > "`+log+`/stdin"
prev=; for a in "$@"; do
  case "$prev" in --output-last-message) out=$a;; -c) case "$a" in model_instructions_file=*) f=${a#model_instructions_file=}; f=${f#\"}; cp "${f%\"}" "`+log+`/instructions";; esac;; esac
  prev=$a
done
echo "codex banner and progress"; printf ' {"items": []} \n' > "$out"`)
	got, err := Ask(Codex, Call{System: "You judge.", Prompt: "REPLY: hi", Timeout: 10 * time.Second})
	if err != nil || got != `{"items": []}` {
		t.Fatalf("got %q, %v", got, err)
	}
	args := strings.Split(strings.TrimSpace(read(t, filepath.Join(log, "args"))), "\n")
	for _, want := range [][]string{{"exec"}, {"--disable", "hooks"}, {"--ignore-user-config"}, {"--ignore-rules"},
		{"--ephemeral"}, {"--skip-git-repo-check"}, {"--sandbox", "read-only"}, {"--model", "gpt-5.6-luna"},
		{"-c", `model_reasoning_effort="low"`}} {
		if !containsRun(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if args[len(args)-1] != "-" || read(t, filepath.Join(log, "stdin")) != "REPLY: hi" {
		t.Errorf("the prompt is not on stdin: args end %q, stdin %q", args[len(args)-1], read(t, filepath.Join(log, "stdin")))
	}
	if read(t, filepath.Join(log, "instructions")) != "You judge." {
		t.Errorf("instructions file holds %q", read(t, filepath.Join(log, "instructions")))
	}
	cwd := strings.TrimSpace(read(t, filepath.Join(log, "cwd")))
	if !strings.HasPrefix(filepath.Base(cwd), "pm-ask-") || strings.TrimSpace(read(t, filepath.Join(log, "ls"))) != "instructions.md" {
		t.Errorf("ran in %s holding %q, not an empty temp dir of its own", cwd, read(t, filepath.Join(log, "ls")))
	}
	if _, err := os.Stat(cwd); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temp dir %s outlived the call", cwd)
	}
}

func containsRun(args, run []string) bool {
	for i := range args {
		if i+len(run) <= len(args) && slices.Equal(args[i:i+len(run)], run) {
			return true
		}
	}
	return false
}

func TestClaudeRunsNoSettingsNoToolsNoThinking(t *testing.T) {
	log := t.TempDir()
	fake(t, "claude", `printf '%s\n' "$@" > "`+log+`/args"; echo "$MAX_THINKING_TOKENS" > "`+log+`/env"; cat >/dev/null; echo ' answer '`)
	got, err := Ask(Claude, Call{System: "You judge.", Prompt: "p", Timeout: 10 * time.Second})
	if err != nil || got != "answer" {
		t.Fatalf("got %q, %v", got, err)
	}
	args := strings.Split(strings.TrimSuffix(read(t, filepath.Join(log, "args")), "\n"), "\n")
	for _, want := range [][]string{{"-p"}, {"--setting-sources", ""}, {"--tools", ""}, {"--strict-mcp-config"},
		{"--no-session-persistence"}, {"--model", "claude-haiku-5-5"}, {"--system-prompt", "You judge."}} {
		if !containsRun(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if strings.TrimSpace(read(t, filepath.Join(log, "env"))) != "0" {
		t.Error("thinking is not off")
	}
}

// TestATimeoutKillsTheWholeCall: codex and claude are launcher scripts whose real process is a child; at the timeout
// the whole process group dies, not only the launcher.
func TestATimeoutKillsTheWholeCall(t *testing.T) {
	log := t.TempDir()
	fake(t, "codex", `sleep 30 & echo $! > "`+log+`/child"; wait`)
	start := time.Now()
	_, err := Ask(Codex, Call{Prompt: "p", Timeout: time.Second})
	var ae *Error
	if !errors.As(err, &ae) || err.Error() != "codex exec timed out after 1s" {
		t.Fatalf("got %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %s", took)
	}
	pid := 0
	for _, c := range strings.TrimSpace(read(t, filepath.Join(log, "child"))) {
		pid = pid*10 + int(c-'0')
	}
	tick := time.NewTicker(50 * time.Millisecond) // the kill is sent; its delivery takes a moment
	defer tick.Stop()
	deadline := time.After(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("the call's child %d outlived the timeout", pid)
		}
	}
}

func TestAFailureNamesTheRuntimeAndItsWords(t *testing.T) {
	fake(t, "codex", `echo "banner"; echo "ERROR: You've hit your usage limit." >&2; exit 1`)
	if _, err := Ask(Codex, Call{Prompt: "p", Timeout: 5 * time.Second}); err == nil ||
		err.Error() != "codex exec failed (exit 1): ERROR: You've hit your usage limit." {
		t.Errorf("got %v", err)
	}
	fake(t, "codex", `exit 0`) // no last message: no answer
	if _, err := Ask(Codex, Call{Prompt: "p", Timeout: 5 * time.Second}); err == nil ||
		err.Error() != "codex exec failed (exit 0): no output" {
		t.Errorf("got %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	for _, r := range []Runtime{Claude, Codex} {
		if _, err := Ask(r, Call{Prompt: "p", Timeout: time.Second}); err == nil ||
			err.Error() != string(r)+" is not installed or not on PATH" {
			t.Errorf("%s: got %v", r, err)
		}
	}
}
