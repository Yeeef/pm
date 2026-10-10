package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/launch"
)

// launchedByGo is whether a Go launcher launched this pm for its own version: the launcher then owns the bin dir. A
// pm the Python bridge launched (PM_LAUNCHER 0.1.x) installs itself in place of the pm uv tool.
func launchedByGo() bool { return launch.IsLaunched() && launch.IsGo(launch.LauncherVersion()) }

// self is the running binary, its links followed.
func self() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// InstallBinary puts this pm in the bin dir, config.BinPath(), which hooks, agents and the service unit run: copied
// there atomically when nothing is there or another binary or link is (a link, such as the retired pm uv tool's, is
// replaced, never followed). A pm a Go launcher launched leaves the bin dir to the launcher. What it did, "" when the
// bin-dir pm is this one.
func InstallBinary() (string, error) {
	if launchedByGo() {
		return "", nil
	}
	bin, err := config.BinPath()
	if err != nil {
		return "", err
	}
	me, err := self()
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(bin); err == nil {
		if mine, err := os.Stat(me); err == nil && os.SameFile(st, mine) {
			return "", nil
		}
	}
	if err := copyAtomic(me, bin); err != nil {
		return "", err
	}
	return fmt.Sprintf("installed pm %s at %s", buildinfo.Version, bin), nil
}

// copyAtomic copies the file src to dst, executable, through a temporary file beside dst, so a reader never sees
// part of it.
func copyAtomic(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pm.*.tmp")
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	ok = true
	return nil
}
