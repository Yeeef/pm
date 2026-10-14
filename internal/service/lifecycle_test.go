package service

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The supervisor's tools as recorders: each call is a line in $FAKE_LOG, what is loaded or active a line in
// $FAKE_UP, and what is disabled (launchd) or enabled (systemd) a line in $FAKE_UP.en. $FAKE_NO_SYSTEMD makes
// systemctl answer as on a machine without a user instance.
const (
	recLaunchctl = `echo "launchctl $*" >> "$FAKE_LOG"
eval "last=\${$#}"
name=$(basename "$last" .plist)
drop() { grep -vxF "$1" "$2" > "$2.t" 2>/dev/null; mv "$2.t" "$2"; }
case "$1" in
  print) grep -qxF "$name" "$FAKE_UP" 2>/dev/null && exit 0; exit 113;;
  print-disabled) echo "disabled services = {"; while read -r l; do printf '\t"%s" => disabled\n' "$l"; done < "$FAKE_UP.en" 2>/dev/null; echo "}";;
  disable) echo "$name" >> "$FAKE_UP.en";;
  enable) drop "$name" "$FAKE_UP.en";;
  bootstrap) grep -qxF "$name" "$FAKE_UP.en" 2>/dev/null && exit 5; echo "$name" >> "$FAKE_UP";;
  bootout) drop "$name" "$FAKE_UP";;
esac
exit 0`
	recSystemctl = `echo "systemctl $*" >> "$FAKE_LOG"
[ -n "$FAKE_NO_SYSTEMD" ] && exit 1
drop() { grep -vxF "$1" "$2" > "$2.t" 2>/dev/null; mv "$2.t" "$2"; }
eval "u=\${$#}"
case "$2" in
  is-active) grep -qxF "$3" "$FAKE_UP" 2>/dev/null && exit 0; exit 3;;
  is-enabled) grep -qxF "$3" "$FAKE_UP.en" 2>/dev/null && { echo enabled; exit 0; }; echo disabled; exit 1;;
  enable) echo "$u" >> "$FAKE_UP.en"; [ "$3" = --now ] && echo "$u" >> "$FAKE_UP";;
  disable) drop "$u" "$FAKE_UP.en"; [ "$3" = --now ] && drop "$u" "$FAKE_UP";;
  restart) grep -qxF "$u" "$FAKE_UP" 2>/dev/null || echo "$u" >> "$FAKE_UP";;
esac
exit 0`
)

type machine struct {
	t      *testing.T
	main   string
	log    string
	waited []int
	build  string // the build the stand-in site answers with
}

// newMachine is a main checkout, HOME in a temp dir, the supervisor's tools replaced by recorders on a PATH that
// holds a '%', and a site that comes up at once for this store (the end-to-end test runs both for real).
func newMachine(t *testing.T, kind string) *machine {
	t.Helper()
	tmp := t.TempDir()
	m := &machine{t: t, main: filepath.Join(tmp, "main"), log: filepath.Join(tmp, "calls"), build: Version()}
	os.MkdirAll(m.main, 0o755)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "home/.config"))
	t.Setenv("FAKE_LOG", m.log)
	t.Setenv("FAKE_UP", filepath.Join(tmp, "up"))
	bin := fakeBin(t, map[string]string{"launchctl": recLaunchctl, "systemctl": recSystemctl})
	pct := filepath.Join(tmp, "tools%x")
	os.Rename(bin, pct)
	t.Setenv("PATH", strings.Replace(os.Getenv("PATH"), bin, pct, 1))
	saved := []any{PlatformKind, probe, waitUp}
	PlatformKind = func() string { return kind }
	probe = func(int) *Served { return &Served{store(m.main), m.build} }
	waitUp = func(main string, port int, done, hint string) (string, error) {
		m.waited = append(m.waited, port)
		return "", nil
	}
	t.Cleanup(func() {
		PlatformKind = saved[0].(func() string)
		probe = saved[1].(func(int) *Served)
		waitUp = saved[2].(func(string, int, string, string) (string, error))
	})
	return m
}

func (m *machine) calls() []string {
	data, _ := os.ReadFile(m.log)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// install installs the unit for the site port 8123.
func (m *machine) install() string {
	m.t.Helper()
	exe, _ := Exe()
	said, err := Install(m.main, 8123, exe, false)
	if err != nil {
		m.t.Fatal(err)
	}
	return said
}

// TestPmInitKeepsTheUnitsPATH: an agent runtime runs pm init with a PATH of its own (Codex puts its directory first).
// pm init (keepPath) leaves a running service whose unit differs from its env only in PATH unwritten and unrestarted;
// a real difference (the port) still rewrites and restarts it, keeping the unit's PATH; pm service install takes the
// PATH it runs with.
func TestPmInitKeepsTheUnitsPATH(t *testing.T) {
	for _, kind := range []string{Systemd, Launchd} {
		m := newMachine(t, kind)
		m.install()
		unit := UnitFile(m.main, kind)
		before, err := os.ReadFile(unit)
		if err != nil {
			t.Fatal(err)
		}
		was := os.Getenv("PATH")
		calls := len(m.calls())
		t.Setenv("PATH", "/opt/codex/vendor/x86_64-unknown-linux-musl/codex-path"+string(os.PathListSeparator)+was)
		exe, err := Exe()
		if err != nil {
			t.Fatal(err)
		}
		if said, err := Install(m.main, 8123, exe, true); said != "" || err != nil {
			t.Fatalf("%s: a PATH-only difference: %q %v", kind, said, err)
		}
		restarted := slices.ContainsFunc(m.calls()[calls:], func(c string) bool {
			return regexp.MustCompile(`restart|daemon-reload|enable --now|bootstrap|bootout`).MatchString(c)
		})
		if after, err := os.ReadFile(unit); err != nil || string(after) != string(before) || restarted {
			t.Fatalf("%s: the unit was rewritten or the service restarted: %v", kind, m.calls()[calls:])
		}
		if said, err := Install(m.main, 9000, exe, true); err != nil || !strings.HasPrefix(said, "updated the pm service") {
			t.Fatalf("%s: a new port: %q %v", kind, said, err)
		}
		if path, ok := UnitPath(m.main, kind); !ok || path != was {
			t.Fatalf("%s: pm init took its own PATH %q", kind, path)
		}
		if said, err := Install(m.main, 9000, exe, false); err != nil || !strings.HasPrefix(said, "updated the pm service") {
			t.Fatalf("%s: pm service install: %q", kind, said)
		}
		if path, _ := UnitPath(m.main, kind); path != os.Getenv("PATH") {
			t.Fatalf("%s: pm service install kept %q", kind, path)
		}
	}
}

func TestUninstallStopsAndRemovesTheUnit(t *testing.T) {
	m := newMachine(t, Systemd)
	if said, err := Uninstall(m.main); said != "" || err != nil {
		t.Fatal(said, err)
	}
	m.install()
	said, err := Uninstall(m.main)
	if err != nil || said != "removed the pm service: systemd "+Label(m.main)+" ("+UnitFile(m.main, Systemd)+")" {
		t.Fatal(said, err)
	}
	c := m.calls()
	if strings.Join(c[len(c)-2:], "|") != "systemctl --user disable --now "+Label(m.main)+".service|systemctl --user daemon-reload" {
		t.Fatal(c)
	}
	if Installed(m.main) {
		t.Fatal("the unit is still there")
	}
	t.Run("InstallHoldsTheClonesInstallLock", func(t *testing.T) {
		m := newMachine(t, Systemd)
		var held []bool
		waitUp = func(main string, port int, done, hint string) (string, error) {
			fd, _ := syscall.Open(filepath.Join(main, ".pm/run/install.lock"), syscall.O_RDWR, 0)
			defer syscall.Close(fd)
			held = append(held, syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil)
			return "", nil
		}
		m.install()
		if len(held) != 1 || !held[0] {
			t.Fatalf("the lock was not held while install waited: %v", held)
		}
		fd, _ := syscall.Open(filepath.Join(m.main, ".pm/run/install.lock"), syscall.O_RDWR, 0)
		defer syscall.Close(fd)
		if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal("released once install returns:", err)
		}
	})
}

// A service on the main checkout's pin, seen from a checkout that pins another pm (a branch from before the pin move):
// the service is not stale, so neither pm where's line nor pm doctor's service drift says to restart it; the line
// names this checkout's pin and its fix, and pm doctor's work store line does (work.CheckVersion).
func TestAServiceOnMainsPinIsNotStaleFromACheckoutOffIt(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install()
	if out, err := exec.Command("git", "init", "-q", m.main).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	os.MkdirAll(filepath.Join(m.main, ".pm"), 0o755)
	os.WriteFile(filepath.Join(m.main, ".pm/config.toml"), []byte("version = \"9.9.9\"\nremote = \"origin\"\n"+
		"main_branch = \"main\"\nport = 8123\n"), 0o644)
	m.build = "9.9.9"
	ok, line := Health(m.main)
	if ok || !strings.Contains(line, "  running pm 9.9.9, the main checkout's pin, and the site answers on :8123; pm "+
		"refuses this checkout's work-store commands: this checkout pins pm "+Version()+", but the clone's pm service runs pm 9.9.9, which "+
		filepath.Join(m.main, ".pm/config.toml")+" pins: ") {
		t.Fatal(line)
	}
	if d := Drift(m.main, 8123); len(d) != 0 {
		t.Fatal(d)
	}
	m.build = "0.0.1" // a service on neither pin is stale
	if ok, line := Health(m.main); ok || !strings.Contains(line, "stale: it runs pm 0.0.1, not pm "+Version()) {
		t.Fatal(line)
	}
}

// The end to end: under the fake supervisor (tests/fake_sched.py), which starts the unit's command as launchd or
// systemd would, the unit runs this test binary as a pm service with the fake site and store (TestMain).
func TestServiceInstallStatusRestartAndLogsEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the fake supervisor needs python3")
	}
	sched, _ := filepath.Abs("../../tests/fake_sched.py")
	tmp := shortTemp(t)
	main := filepath.Join(tmp, "main")
	os.MkdirAll(filepath.Join(main, ".pm/store/records"), 0o755)
	os.MkdirAll(filepath.Join(main, ".git"), 0o755)
	os.WriteFile(filepath.Join(main, ".pm/config.toml"), []byte(`version = "`+Version()+`"`+"\n"), 0o644)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "home/.config"))
	t.Setenv("FAKE_SCHED_LOG", filepath.Join(tmp, "sched.log"))
	t.Setenv("FAKE_SCHED_STATE", filepath.Join(tmp, "sched.json"))
	t.Setenv("PM_TEST_SERVICE", main) // TestMain runs the started binary as the service
	savedExe := Exe
	Exe = os.Executable // the unit runs this test binary, not an installed pm
	defer func() { Exe = savedExe }()
	wrap := func(tool string) string { return "FAKE_TOOL=" + tool + ` exec python3 "` + sched + `" "$@"` }
	fakeBin(t, map[string]string{"launchctl": wrap("launchctl"), "systemctl": wrap("systemctl")})
	t.Cleanup(func() { stopServices(t, filepath.Join(tmp, "sched.json")) })
	saved := RestartWait
	RestartWait = 10 * time.Second
	defer func() { RestartWait = saved }()

	code, out, err := Status(main, "origin")
	if err != nil || code != 1 || !strings.Contains(out, "not installed; run pm service install") {
		t.Fatal(code, out, err)
	}
	// The site port is one the kernel hands out, not FreePort's: FreePort takes the first free port from 8000 that no
	// unit in this HOME names, and each copy of this test has a HOME of its own, so two copies running at once (make
	// test-go runs this package twice, plain and under -race, and other worktrees run it too) took the same port, and
	// the copy whose service bound second found the other's store on it. The root package's
	// TestNoTestTakesFreePortsSitePort fails any test that calls FreePort.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if err := l.Close(); err != nil || !ok {
		t.Fatal(l.Addr(), err)
	}
	port := addr.Port
	exe, _ := Exe()
	said, err := Install(main, port, exe, false)
	if err != nil || !strings.HasPrefix(said, "installed the pm service: ") ||
		!strings.Contains(said, "serving http://localhost:"+strconv.Itoa(port)) {
		t.Fatal(said, err)
	}
	code, out, err = Status(main, "origin")
	lines := strings.Split(out, "\n")
	if err != nil || code != 0 || !strings.HasSuffix(lines[0], "running; the site answers on :"+strconv.Itoa(port)) ||
		!strings.Contains(out, "log       "+LogPath(main)+" (pm service logs)") ||
		!strings.Contains(out, "gc        no collection recorded yet") {
		t.Fatal(code, out, err)
	}
	said, err = Restart(main)
	if err != nil || !strings.HasSuffix(said, "running; the site answers on :"+strconv.Itoa(port)) {
		t.Fatal(said, err)
	}
	logs, _ := Logs(main, 50)
	if !regexp.MustCompile(`(?m)^Serving http://localhost:` + strconv.Itoa(port) + `; .*pushing every 10 min$`).MatchString(logs) {
		t.Fatal(logs)
	}
	if said, err := Install(main, port, exe, false); said != "" || err != nil {
		t.Fatal("installed and current:", said, err)
	}
	stopServices(t, filepath.Join(tmp, "sched.json")) // the process dies; nothing restarts it here
	code, out, _ = Status(main, "origin")
	if code != 1 || !strings.Contains(out, "down: nothing answers on :"+strconv.Itoa(port)+"; run pm service restart") {
		t.Fatal(code, out)
	}
}

// stopServices stops every process the fake supervisor started.
func stopServices(t *testing.T, state string) {
	t.Helper()
	data, err := os.ReadFile(state)
	if err != nil {
		return
	}
	for _, m := range regexp.MustCompile(`"[^"]+": (\d+)`).FindAllStringSubmatch(string(data), -1) {
		pid, _ := strconv.Atoi(m[1])
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			eventually(t, fmt.Sprintf("process %d exits", pid), func() bool { return syscall.Kill(pid, 0) != nil })
		}
	}
}

// pm service stop disables the unit and stops it; the stop holds at session start (StartIfDown) and shows in health
// and drift, and pm service restart and pm service install each enable and start it again.
func TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall(t *testing.T) {
	for _, kind := range []string{Systemd, Launchd} {
		t.Run(kind, func(t *testing.T) { checkStopOnSupervisor(t, kind) })
	}

	// A stop that leaves something answering on the socket fails, naming it.
	t.Run("StopFailsWhileTheSocketStillAnswers", checkStopFailsWhileTheSocketStillAnswers)
}

// A systemctl that cannot reach the user instance exits non-zero for is-enabled too: that is no stop, so session
// start still starts the service and health says down, not stopped.
func TestASupervisorThatDoesNotAnswerIsNoStop(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install()
	fakeBin(t, map[string]string{"systemctl": `[ "$2" = is-enabled ] && { echo "Failed to connect to bus" >&2; exit 1; }; exit 0`})
	if Disabled(m.main, Systemd) {
		t.Fatal("a failed is-enabled reads as a stop")
	}
}

func checkStopOnSupervisor(t *testing.T, kind string) {
	t.Helper()
	m := newMachine(t, kind)
	if _, err := Stop(m.main); err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Fatal("stop of no unit:", err)
	}
	m.install()
	up := true // the stand-in site answers while the supervisor holds the unit
	probe = func(int) *Served {
		if up && Loaded(m.main, kind) {
			return &Served{store(m.main), m.build}
		}
		return nil
	}
	n := len(m.calls())
	said, err := Stop(m.main)
	if err != nil || !strings.HasPrefix(said, "stopped the pm service: "+kind+" "+Label(m.main)) ||
		!strings.Contains(said, "pm service restart or pm init starts it again") {
		t.Fatal(said, err)
	}
	var acts []string
	for _, c := range m.calls()[n:] {
		if !strings.Contains(c, " print") && !strings.Contains(c, "is-") && !strings.Contains(c, "show-env") {
			acts = append(acts, c)
		}
	}
	want := "systemctl --user disable --now " + Label(m.main) + ".service"
	if kind == Launchd {
		gui := "gui/" + strconv.Itoa(os.Getuid()) + "/" + Label(m.main)
		want = "launchctl disable " + gui + "|launchctl bootout " + gui
	}
	if strings.Join(acts, "|") != want {
		t.Fatalf("stop ran %v", acts)
	}
	checkStoppedService(t, m, kind)
	saved := waitUp
	waitUp = func(string, int, string, string) (string, error) { return "up", nil }
	defer func() { waitUp = saved }()
	if _, err := Restart(m.main); err != nil || Disabled(m.main, kind) || !Loaded(m.main, kind) {
		t.Fatal("restart ends the stop:", err)
	}
	if _, err := Stop(m.main); err != nil {
		t.Fatal(err)
	}
	exe, err := Exe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install(m.main, 8123, exe, true); err != nil || Disabled(m.main, kind) || !Loaded(m.main, kind) {
		t.Fatal("install (pm init) ends the stop:", err)
	}
}

func checkStopFailsWhileTheSocketStillAnswers(t *testing.T) {
	t.Helper()
	m := newMachine(t, Systemd)
	m.install()
	probe = func(int) *Served { return nil }
	saved := RestartWait
	RestartWait = 300 * time.Millisecond
	defer func() { RestartWait = saved }()
	sock := filepath.Join(m.main, ".pm/run/work.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("socket path too long here:", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := Stop(m.main); err == nil || !strings.Contains(err.Error(), "the work store's socket "+sock+" still answer") {
		t.Fatal(err)
	}
}

func checkStoppedService(t *testing.T, m *machine, kind string) {
	t.Helper()
	if !Disabled(m.main, kind) || Loaded(m.main, kind) {
		t.Fatal("the unit is not disabled and stopped")
	}
	if ok, line := Health(m.main); ok || !strings.HasSuffix(line, Stopped) {
		t.Fatal(line)
	}
	if d := Drift(m.main, 8123); len(d) != 1 || !strings.HasSuffix(d[0], Stopped) {
		t.Fatal(d)
	}
	n := len(m.calls())
	if said, stopped, err := StartIfDown(m.main); said != "" || !stopped || err != nil {
		t.Fatal(said, stopped, err)
	}
	for _, c := range m.calls()[n:] {
		if strings.Contains(c, " enable") || strings.Contains(c, "restart") || strings.Contains(c, "bootstrap") {
			t.Fatalf("session start started a stopped service: %q", c)
		}
	}
}
