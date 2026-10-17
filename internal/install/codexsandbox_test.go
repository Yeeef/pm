package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSandbox puts first on PATH a codex whose sandbox subcommand prints said and exits with code, a bwrap beside it,
// and a CODEX_HOME that exists; restrict is the AppArmor sysctl's value ("" for none).
func fakeSandbox(t *testing.T, said string, code int, restrict string) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Codex's bwrap sandbox is Linux's")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = sandbox ] || exit 2\necho '" + said + "' >&2\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("CODEX_HOME", t.TempDir())
	sysctl := filepath.Join(t.TempDir(), "restrict")
	if restrict != "" {
		if err := os.WriteFile(sysctl, []byte(restrict+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := apparmorUserns
	apparmorUserns = sysctl
	t.Cleanup(func() { apparmorUserns = old })
	return filepath.Join(dir, "bwrap")
}

func TestCodexSandboxNamesAppArmorAndItsFix(t *testing.T) {
	bwrap := fakeSandbox(t, "bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted", 1, "1")
	problem, note := CodexSandbox(5 * time.Second)
	if note != "" {
		t.Fatalf("note %q", note)
	}
	for _, want := range []string{
		"codex: Codex's sandbox cannot start here (codex sandbox -- true: bwrap: loopback: Failed RTM_NEWADDR: " +
			"Operation not permitted)",
		"kernel.apparmor_restrict_unprivileged_userns is 1",
		"profile bwrap " + bwrap + " flags=(unconfined) { userns, }",
		"sudo apparmor_parser -r /etc/apparmor.d/bwrap",
		"sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0",
	} {
		if !strings.Contains(problem, want) {
			t.Errorf("problem lacks %q:\n%s", want, problem)
		}
	}
}

func TestCodexSandboxWithoutAppArmorNamesTheNamespaces(t *testing.T) {
	fakeSandbox(t, "bwrap: setting up uid map: Permission denied", 1, "0")
	problem, _ := CodexSandbox(5 * time.Second)
	if !strings.Contains(problem, "bwrap cannot make the user and network namespaces it needs") ||
		strings.Contains(problem, "AppArmor") {
		t.Fatalf("problem %q", problem)
	}
}

func TestCodexSandboxThatStartsSaysNothing(t *testing.T) {
	fakeSandbox(t, "", 0, "1")
	if problem, note := CodexSandbox(5 * time.Second); problem != "" || note != "" {
		t.Fatalf("problem %q, note %q", problem, note)
	}
}

// A failure that is not bwrap's (an older Codex without the subcommand, a panic) is not known to be the sandbox's, so
// it is a note, not a problem.
func TestCodexSandboxOtherFailureIsANote(t *testing.T) {
	fakeSandbox(t, "error: unrecognized subcommand", 2, "1")
	problem, note := CodexSandbox(5 * time.Second)
	if problem != "" || !strings.Contains(note, "cannot tell whether Codex's sandbox starts here") ||
		!strings.Contains(note, "unrecognized subcommand") {
		t.Fatalf("problem %q, note %q", problem, note)
	}
}

func TestCodexSandboxIsNotCheckedWhereCodexNeverRan(t *testing.T) {
	fakeSandbox(t, "bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted", 1, "1")
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "missing"))
	if problem, note := CodexSandbox(5 * time.Second); problem != "" || note != "" {
		t.Fatalf("problem %q, note %q", problem, note)
	}
}
