package service

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	pmsync "github.com/Yeeef/pm/internal/sync"
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

func (m *machine) install(port int) string {
	m.t.Helper()
	exe, _ := Exe()
	said, err := Install(m.main, port, exe)
	if err != nil {
		m.t.Fatal(err)
	}
	return said
}

func TestSystemdUnitRunsThisPmRestartsItAndQuotesPaths(t *testing.T) {
	m := newMachine(t, Systemd)
	said := m.install(8123)
	if !strings.HasPrefix(said, "installed the pm service: systemd user service local.pm.main.") ||
		!strings.Contains(said, ", serving http://localhost:8123 and pushing every 10 min; log "+LogPath(m.main)) {
		t.Fatal(said)
	}
	if len(m.waited) != 1 || m.waited[0] != 8123 {
		t.Fatalf("install waits for the site to answer: %v", m.waited)
	}
	unit := UnitFile(m.main, Systemd)
	text, _ := os.ReadFile(unit)
	exe, _ := Exe()
	for _, want := range []string{"WorkingDirectory=" + m.main + "\n",
		`Environment="PATH=` + strings.ReplaceAll(os.Getenv("PATH"), "%", "%%") + `" "PORT=8123"` + "\n",
		`ExecStart="` + exe + `" "service" "run"` + "\n", "Restart=always\n", "WantedBy=default.target\n",
		"StandardOutput=append:" + m.main + "/.pm/run/service.log\n"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("unit lacks %q:\n%s", want, text)
		}
	}
	if c := m.calls(); c[len(c)-1] != "systemctl --user enable --now "+filepath.Base(unit) {
		t.Fatalf("calls %v", c)
	}
	if st, _ := pmsync.ReadState(m.main); st["installed_at"] == nil {
		t.Fatal("a push that never succeeds is flagged overdue from the install")
	}
	if p, _ := UnitPort(m.main, Systemd); p != 8123 {
		t.Fatal(p)
	}
	n := len(m.calls())
	if said := m.install(8123); said != "" {
		t.Fatalf("installed, current and active: %q", said)
	}
	for _, c := range m.calls()[n:] {
		if !strings.Contains(c, "show-environment") && !strings.Contains(c, "is-active") &&
			!strings.Contains(c, "is-enabled") {
			t.Fatalf("a current install ran %q", c)
		}
	}
	if said := m.install(8124); !strings.HasPrefix(said, "updated the pm service") {
		t.Fatal(said)
	}
	var c []string
	for _, call := range m.calls() {
		if !strings.Contains(call, "is-enabled") {
			c = append(c, call)
		}
	}
	if strings.Join(c[len(c)-2:], "|") != "systemctl --user daemon-reload|systemctl --user restart "+filepath.Base(unit) {
		t.Fatalf("calls %v", c)
	}
}

func TestLaunchdAgentKeepsTheServiceAlive(t *testing.T) {
	m := newMachine(t, Launchd)
	if said := m.install(8123); !strings.HasPrefix(said, "installed the pm service: launchd agent") {
		t.Fatal(said)
	}
	plist := UnitFile(m.main, Launchd)
	exe, _ := Exe()
	want, _ := UnitBytes(m.main, Launchd, os.Getenv("PATH"), 8123, Command(exe))
	if have, _ := os.ReadFile(plist); string(have) != string(want) {
		t.Fatalf("plist:\n%s", have)
	}
	if st, err := os.Stat(filepath.Join(m.main, ".pm/run")); err != nil || !st.IsDir() {
		t.Fatal("launchd opens the log but does not make its directory")
	}
	if said := m.install(8123); said != "" {
		t.Fatal(said)
	}
	if said := m.install(9000); !strings.HasPrefix(said, "updated") {
		t.Fatal(said)
	}
	var acts []string
	for _, c := range m.calls() {
		if !strings.HasPrefix(c, "launchctl print") {
			acts = append(acts, c)
		}
	}
	gui := "gui/" + strconv.Itoa(os.Getuid())
	if strings.Join(acts[len(acts)-2:], "|") != "launchctl bootout "+gui+"/"+Label(m.main)+"|launchctl bootstrap "+gui+" "+plist {
		t.Fatalf("a changed plist is loaded again, once launchd let go: %v", acts)
	}
}

func TestInstallRestartsACurrentUnitThatAnswersOnAnotherBuild(t *testing.T) {
	for _, other := range []string{"0.0.1", ""} {
		m := newMachine(t, Systemd)
		m.install(8123)
		m.build = other
		if said := m.install(8123); !strings.HasPrefix(said, "updated the pm service") {
			t.Fatal(said)
		}
		c := m.calls()
		if c[len(c)-1] != "systemctl --user restart "+Label(m.main)+".service" || len(m.waited) != 2 {
			t.Fatalf("calls %v waited %v", c, m.waited)
		}
	}
}

func TestInstallHoldsTheClonesInstallLock(t *testing.T) {
	m := newMachine(t, Systemd)
	var held []bool
	waitUp = func(main string, port int, done, hint string) (string, error) {
		fd, _ := syscall.Open(filepath.Join(main, ".pm/run/install.lock"), syscall.O_RDWR, 0)
		defer syscall.Close(fd)
		held = append(held, syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil)
		return "", nil
	}
	m.install(8123)
	if len(held) != 1 || !held[0] {
		t.Fatalf("the lock was not held while install waited: %v", held)
	}
	fd, _ := syscall.Open(filepath.Join(m.main, ".pm/run/install.lock"), syscall.O_RDWR, 0)
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal("released once install returns:", err)
	}
}

func TestAUnitRunningAnotherPmIsStaleUntilPmServiceInstall(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install(8123)
	unit := UnitFile(m.main, Systemd)
	exe, _ := Exe()
	old := "/data/pm/pins/0.1.0/tools/pm/bin/python"
	text, _ := os.ReadFile(unit)
	os.WriteFile(unit, []byte(strings.Replace(string(text), `ExecStart="`+exe+`"`, `ExecStart="`+old+`" "-m" "pm.cli"`, 1)), 0o644)
	ok, line := Health(m.main)
	if ok || !strings.HasSuffix(line, "stale: its unit runs "+old+" -m pm.cli service run, not this pm ("+exe+
		" service run); run pm service install") {
		t.Fatal(line)
	}
	if said := m.install(8123); !strings.HasPrefix(said, "updated the pm service") {
		t.Fatal(said)
	}
	if ok, line := Health(m.main); !ok || !strings.HasSuffix(line, "running; the site answers on :8123") {
		t.Fatal(line)
	}
}

func TestRefusalsWithoutASupervisorOrTheTools(t *testing.T) {
	m := newMachine(t, Systemd)
	t.Setenv("FAKE_NO_SYSTEMD", "1")
	exe, _ := Exe()
	if _, err := Install(m.main, 8123, exe); err == nil || !strings.HasPrefix(err.Error(), "no service manager here") {
		t.Fatal(err)
	}
	if ok, line := Health(m.main); ok || !strings.HasPrefix(line, "service   no service manager here") {
		t.Fatal(line)
	}
	os.Unsetenv("FAKE_NO_SYSTEMD")
	t.Setenv("PATH", filepath.Dir(m.log)+"/tools%x")
	if _, err := Install(m.main, 8123, exe); err == nil || err.Error() != "the pm service needs git on PATH; git not found in "+os.Getenv("PATH") {
		t.Fatal(err)
	}
	if _, err := Restart(m.main); err == nil || !strings.Contains(err.Error(), "the pm service is not installed (") {
		t.Fatal(err)
	}
	if _, err := Logs(m.main, 5); err == nil || !strings.Contains(err.Error(), "no service log at ") {
		t.Fatal(err)
	}
}

func TestLogsIsTheLogsLastLines(t *testing.T) {
	main := t.TempDir()
	os.MkdirAll(RunDir(main), 0o755)
	os.WriteFile(LogPath(main), []byte("a\nb\nc\n"), 0o644)
	for n, want := range map[int]string{2: "b\nc", 3: "a\nb\nc", 9: "a\nb\nc", 0: "a\nb\nc", -1: "b\nc"} {
		if got, _ := Logs(main, n); got != want {
			t.Errorf("Logs(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestUninstallStopsAndRemovesTheUnit(t *testing.T) {
	m := newMachine(t, Systemd)
	if said, err := Uninstall(m.main); said != "" || err != nil {
		t.Fatal(said, err)
	}
	m.install(8123)
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
}

func TestDriftNamesWhatDiffersFromWhatPmInitInstalls(t *testing.T) {
	m := newMachine(t, Systemd)
	if d := Drift(m.main, 8123); len(d) != 1 || !strings.HasPrefix(d[0], "not installed (") {
		t.Fatal(d)
	}
	m.install(8123)
	if d := Drift(m.main, 8123); len(d) != 0 {
		t.Fatal(d)
	}
	m.build = "0.0.1"
	d := Drift(m.main, 9000)
	if len(d) != 2 || !strings.HasSuffix(d[0], "serves on :8123, not :9000; run pm service install") ||
		!strings.Contains(d[1], "is stale: it runs pm 0.0.1, not pm "+Version()) {
		t.Fatal(d)
	}
}

// A service on the main checkout's pin, seen from a checkout that pins another pm (a branch from before the pin move):
// the service is not stale, so neither pm where's line nor pm doctor's service drift says to restart it; the line
// names this checkout's pin and its fix, and pm doctor's work store line does (work.CheckVersion).
func TestAServiceOnMainsPinIsNotStaleFromACheckoutOffIt(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install(8123)
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
	tmp := t.TempDir()
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
	port := FreePort()
	exe, _ := Exe()
	said, err := Install(main, port, exe)
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
	if said, err := Install(main, port, exe); said != "" || err != nil {
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
	data, err := os.ReadFile(state)
	if err != nil {
		return
	}
	for _, m := range regexp.MustCompile(`"[^"]+": (\d+)`).FindAllStringSubmatch(string(data), -1) {
		pid, _ := strconv.Atoi(m[1])
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			for i := 0; i < 100 && syscall.Kill(pid, 0) == nil; i++ {
				time.Sleep(20 * time.Millisecond)
			}
		}
	}
}

// pm service stop disables the unit and stops it; the stop holds at session start (StartIfDown) and shows in health
// and drift, and pm service restart and pm service install each enable and start it again.
func TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall(t *testing.T) {
	for _, kind := range []string{Systemd, Launchd} {
		t.Run(kind, func(t *testing.T) {
			m := newMachine(t, kind)
			if _, err := Stop(m.main); err == nil || !strings.Contains(err.Error(), "is not installed") {
				t.Fatal("stop of no unit:", err)
			}
			m.install(8123)
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
			if !Disabled(m.main, kind) || Loaded(m.main, kind) {
				t.Fatal("the unit is not disabled and stopped")
			}
			if ok, line := Health(m.main); ok || !strings.HasSuffix(line, Stopped) {
				t.Fatal(line)
			}
			if d := Drift(m.main, 8123); len(d) != 1 || !strings.HasSuffix(d[0], Stopped) {
				t.Fatal(d)
			}
			n = len(m.calls())
			if said, stopped, err := StartIfDown(m.main); said != "" || !stopped || err != nil {
				t.Fatal(said, stopped, err)
			}
			for _, c := range m.calls()[n:] {
				if strings.Contains(c, " enable") || strings.Contains(c, "restart") || strings.Contains(c, "bootstrap") {
					t.Fatalf("session start started a stopped service: %q", c)
				}
			}
			saved := waitUp
			waitUp = func(string, int, string, string) (string, error) { return "up", nil }
			defer func() { waitUp = saved }()
			if _, err := Restart(m.main); err != nil || Disabled(m.main, kind) || !Loaded(m.main, kind) {
				t.Fatal("restart ends the stop:", err)
			}
			if _, err := Stop(m.main); err != nil {
				t.Fatal(err)
			}
			exe, _ := Exe()
			if _, err := Install(m.main, 8123, exe); err != nil || Disabled(m.main, kind) || !Loaded(m.main, kind) {
				t.Fatal("install (pm init) ends the stop:", err)
			}
		})
	}
}

// A stop that leaves something answering on the socket fails, naming it.
func TestStopFailsWhileTheSocketStillAnswers(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install(8123)
	probe = func(int) *Served { return nil }
	saved := RestartWait
	RestartWait = 300 * time.Millisecond
	defer func() { RestartWait = saved }()
	sock := filepath.Join(m.main, ".pm/run/work.sock")
	os.MkdirAll(filepath.Dir(sock), 0o755)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("socket path too long here:", err)
	}
	defer ln.Close()
	if _, err := Stop(m.main); err == nil || !strings.Contains(err.Error(), "the work store's socket "+sock+" still answer") {
		t.Fatal(err)
	}
}

// A systemctl that cannot reach the user instance exits non-zero for is-enabled too: that is no stop, so session
// start still starts the service and health says down, not stopped.
func TestASupervisorThatDoesNotAnswerIsNoStop(t *testing.T) {
	m := newMachine(t, Systemd)
	m.install(8123)
	fakeBin(t, map[string]string{"systemctl": `[ "$2" = is-enabled ] && { echo "Failed to connect to bus" >&2; exit 1; }; exit 0`})
	if Disabled(m.main, Systemd) {
		t.Fatal("a failed is-enabled reads as a stop")
	}
}
