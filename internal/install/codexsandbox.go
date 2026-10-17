package install

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// On Linux, Codex runs each sandboxed command, apply_patch's included, under bubblewrap: the system's bwrap when one is
// on PATH, else the one Codex bundles, with --unshare-user and --unshare-net. Where bwrap cannot make those namespaces
// (Ubuntu 24.04's AppArmor gives an unprivileged user namespace no rights unless a profile allows it), every sandboxed
// command fails, in workspace-write mode and read-only alike. `codex sandbox -- true` runs Codex's own launch path on
// true: about 50 ms, no model and no login. The read-only mode is pinned so a user config that turns the sandbox off
// does not hide the failure.

// CodexSandboxTimeout bounds the codex sandbox run.
const CodexSandboxTimeout = 10 * time.Second

// apparmorUserns is the sysctl that, at 1, keeps unprivileged user namespaces from programs no AppArmor profile allows.
var apparmorUserns = "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"

// CodexSandbox is whether Codex's sandbox starts on this Linux host: a problem line when bwrap fails in it, or a note
// when the check cannot tell; both "" when it starts, off Linux, or where Codex is not installed or never ran.
func CodexSandbox(timeout time.Duration) (problem, note string) {
	if runtime.GOOS != "linux" {
		return "", ""
	}
	codex, err := exec.LookPath("codex")
	if err != nil || !isDir(CodexHome()) { // CodexTrust notes both
		return "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, codex, "sandbox", "-c", `sandbox_mode="read-only"`, "--", "true")
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if err == nil {
		return "", ""
	}
	said := strings.Join(strings.Fields(out.String()), " ")
	if ctx.Err() != nil || !strings.Contains(said, "bwrap:") {
		return "", fmt.Sprintf("codex: cannot tell whether Codex's sandbox starts here (codex sandbox -- true: %v; %s)",
			err, said)
	}
	problem = fmt.Sprintf("codex: Codex's sandbox cannot start here (codex sandbox -- true: %s), so every command Codex "+
		"runs sandboxed, apply_patch's edits included, fails; ", said)
	bwrap, err := exec.LookPath("bwrap")
	if v, rerr := os.ReadFile(apparmorUserns); rerr != nil || strings.TrimSpace(string(v)) != "1" {
		return problem + "bwrap cannot make the user and network namespaces it needs; allow unprivileged user " +
			"namespaces on this host", ""
	}
	problem += "AppArmor keeps unprivileged user namespaces from every program no profile allows " +
		"(kernel.apparmor_restrict_unprivileged_userns is 1): "
	if err == nil { // the system bwrap, which Codex runs in preference to its own
		problem += fmt.Sprintf("add /etc/apparmor.d/bwrap holding 'abi <abi/4.0>, include <tunables/global> profile "+
			"bwrap %s flags=(unconfined) { userns, }' and load it with sudo apparmor_parser -r /etc/apparmor.d/bwrap, "+
			"or ", bwrap)
	}
	return problem + "lift the restriction for every program with sudo sysctl -w " +
		"kernel.apparmor_restrict_unprivileged_userns=0", ""
}
