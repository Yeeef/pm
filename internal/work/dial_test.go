package work

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDialNamesASocketTheSystemRefuses: a connect the system denies (here EACCES, a socket in a directory this process
// may not search; in Codex's network-off sandbox, EPERM from seccomp) is no service down: in Codex the refusal says
// how to run pm outside the sandbox, elsewhere that a sandbox blocks the socket.
func TestDialNamesASocketTheSystemRefuses(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root searches any directory")
	}
	dir := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	sock := filepath.Join(dir, "work.sock")
	for env, want := range map[string]string{
		"CODEX_SESSION_ID": "Codex's sandbox blocks it, and runs pm outside the sandbox only when the whole command is " +
			"plain pm commands; a heredoc body (--text-file - <<'EOF') keeps it inside, so give the body with " +
			"--text-file PATH (a file under the worktree) or --text=\"…\", or rerun the command with escalation",
		"CODEX_THREAD_ID": "Codex's sandbox blocks it",
		"":                "a sandbox around this command blocks the socket; run pm outside it",
	} {
		t.Setenv("CODEX_SESSION_ID", "")
		t.Setenv("CODEX_THREAD_ID", "")
		if env != "" {
			t.Setenv(env, "th")
		}
		_, err := DialSock(sock, t.TempDir(), Options{})
		if !errors.Is(err, ErrSandboxed) || errors.Is(err, ErrNoService) || !strings.Contains(err.Error(), want) ||
			!strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%q: %v", env, err)
		}
	}
	// a socket nothing listens on is still the service down
	if _, err := DialSock(filepath.Join(t.TempDir(), "work.sock"), t.TempDir(), Options{}); !errors.Is(err, ErrNoService) {
		t.Errorf("no listener: %v", err)
	}
}
