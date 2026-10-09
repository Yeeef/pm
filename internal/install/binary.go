package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
)

// The launcher's markers, read as the process starts: a launched pm takes them out of its environment before its
// children start (the pm-product page, Version pin), so pm init reads the copy taken here.
var (
	launched = os.Getenv("PM_LAUNCHED")
	launcher = os.Getenv("PM_LAUNCHER")
)

// goLauncher is the first launcher version that is Go pm: a launcher from it on installs itself, and the pm it
// launches leaves the bin dir alone.
const goLauncher = "0.2.0"

// VersionKey is a release version's numbers, for ordering; nil for a version that is not dotted numbers.
func VersionKey(v string) []int {
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || p == "" || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// VersionLess is a < b for two VersionKeys.
func VersionLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// launchedByGo is whether a Go launcher launched this pm: it then owns the bin dir.
func launchedByGo() bool {
	k := VersionKey(launcher)
	return launched != "" && launched == buildinfo.Version && k != nil && !VersionLess(k, VersionKey(goLauncher))
}

// self is the running binary, its links followed.
func self() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// uvToolLink is whether the bin-dir pm is the pm uv tool's: a link into `uv tool dir`.
func uvToolLink(bin string) bool {
	if !isLink(bin) {
		return false
	}
	res, err := proc.Run([]string{"uv", "--color", "never", "tool", "dir"}, proc.Options{})
	if err != nil || res.Code != 0 {
		return false
	}
	dir := Resolve(config.PyStrip(res.Stdout))
	target := Resolve(bin)
	return target == dir || strings.HasPrefix(target, dir+string(filepath.Separator))
}

// InstallBinary puts this pm in the bin dir, config.BinPath(), which hooks, agents and the service unit run: copied
// there atomically when nothing is there or another binary is, after uninstalling the pm uv tool whose link it is. A
// pm a Go launcher launched leaves the bin dir to the launcher. What it did, "" when the bin-dir pm is this one.
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
	var said []string
	if uvToolLink(bin) {
		res, err := proc.Run([]string{"uv", "tool", "uninstall", "pm"}, proc.Options{})
		if err != nil {
			return "", err
		}
		if res.Code != 0 {
			return "", refuse("uv tool uninstall pm failed: %s", config.PyStrip(res.Stderr+res.Stdout))
		}
		said = append(said, "uninstalled the pm uv tool (uv tool uninstall pm)")
	}
	if err := copyAtomic(me, bin); err != nil {
		return "", err
	}
	said = append(said, fmt.Sprintf("installed pm %s at %s", buildinfo.Version, bin))
	return strings.Join(said, "; "), nil
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
