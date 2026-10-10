package service

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
)

const (
	StoreHeader   = "X-PM-Store"   // the service's replies name the store they render, so a probe can check who answers
	VersionHeader = "X-PM-Version" // and the pm build they run, so a probe sees a service left on an old one
)

var (
	ProbeTimeout = time.Second      // a probe of the site; pm where runs one at every session start
	RestartWait  = 15 * time.Second // how long install and restart wait for the site to answer
	poll         = 200 * time.Millisecond
)

// Tools is what the service runs; install refuses a PATH without them.
var Tools = []string{"git"}

// Version is the pm build the service runs and that a probe expects; buildinfo.Version unless a test sets it.
var Version = func() string { return buildinfo.Version }

// quiet runs a supervisor command and captures its output; ran is false when the program is not on PATH.
func quiet(argv ...string) (code int, out string, ran bool) {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return 0, "", false
	}
	c := exec.Command(path, argv[1:]...)
	c.Args[0] = argv[0]
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err = c.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, "", true
	case errors.As(err, &exit):
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = strings.TrimSpace(stdout.String())
		}
		return exit.ExitCode(), said, true
	}
	return -1, err.Error(), true
}

// checked runs a supervisor command that must succeed.
func checked(argv ...string) error {
	code, said, ran := quiet(argv...)
	if !ran {
		return refuse("%s failed: %s is not on PATH", strings.Join(argv, " "), argv[0])
	}
	if code != 0 {
		return refuse("%s failed: %s", strings.Join(argv, " "), said)
	}
	return nil
}

// Supervisor is launchd on macOS, systemd with a user instance elsewhere; refused on a machine with neither.
func Supervisor() (string, error) {
	if PlatformKind() == Launchd {
		return Launchd, nil
	}
	if code, _, ran := quiet("systemctl", "--user", "show-environment"); ran && code == 0 {
		return Systemd, nil
	}
	return "", refuse("no service manager here: the pm service needs launchd (macOS) or a systemd user instance " +
		"(Linux), and this machine has neither")
}

func gui() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// Loaded is whether the supervisor holds the unit: loaded in launchd, active in systemd.
func Loaded(main, kind string) bool {
	if kind == Launchd {
		code, _, ran := quiet("launchctl", "print", gui()+"/"+Label(main))
		return ran && code == 0
	}
	code, _, ran := quiet("systemctl", "--user", "is-active", Label(main)+".service")
	return ran && code == 0
}

// Disabled is whether the supervisor keeps the unit from starting at login and after a crash: pm service stop's
// record, the one there is. systemd: is-enabled prints disabled; launchd: print-disabled lists the label as disabled.
// A supervisor that does not answer counts as not disabled, so a failure never reads as a stop.
func Disabled(main, kind string) bool {
	if kind == Launchd {
		code, out, ran := quietOut("launchctl", "print-disabled", gui())
		if !ran || code != 0 {
			return false
		}
		for _, line := range strings.Split(out, "\n") {
			name, state, found := strings.Cut(strings.TrimSpace(line), "=>")
			if found && strings.Trim(strings.TrimSpace(name), `"`) == Label(main) {
				state = strings.TrimSpace(state)
				return state == "disabled" || state == "true"
			}
		}
		return false
	}
	// is-enabled exits non-zero for a failure to reach the user instance too: only its "disabled" counts
	_, out, ran := quietOut("systemctl", "--user", "is-enabled", Label(main)+".service")
	return ran && strings.TrimSpace(out) == "disabled"
}

// quietOut runs a supervisor command and returns its stdout.
func quietOut(argv ...string) (code int, out string, ran bool) {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return 0, "", false
	}
	c := exec.Command(path, argv[1:]...)
	c.Args[0] = argv[0]
	var stdout bytes.Buffer
	c.Stdout = &stdout
	err = c.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, stdout.String(), true
	case errors.As(err, &exit):
		return exit.ExitCode(), stdout.String(), true
	}
	return -1, "", true
}

// enable lets the supervisor start a unit pm service stop disabled; nothing for an enabled one.
func enable(main, kind string) error {
	if !Disabled(main, kind) {
		return nil
	}
	if kind == Launchd {
		return checked("launchctl", "enable", gui()+"/"+Label(main))
	}
	return checked("systemctl", "--user", "enable", filepath.Base(UnitFile(main, kind)))
}

// PortFree is whether a server could bind 127.0.0.1:port now, as the service does: with SO_REUSEADDR (Go sets it on
// a listener), so a port whose last connections linger in TIME_WAIT, as after this clone's service stopped, is free.
func PortFree(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// Served is what answers on a port: the store it renders and the pm build it runs, by StoreHeader and VersionHeader
// ("" for one it does not send: what answers is not pm, or a pm too old to say).
type Served struct{ Store, Version string }

// Answering is what serves on port; nil when nothing answers. The probe fetches style.css, which the service answers
// without rendering.
func Answering(port int) *Served {
	client := http.Client{Timeout: ProbeTimeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/style.css", port))
	if err != nil {
		return nil
	}
	resp.Body.Close()
	return &Served{resp.Header.Get(StoreHeader), resp.Header.Get(VersionHeader)}
}

// probe is Answering; the in-process tests stand in for what answers.
var probe = Answering

// store is the clone's records store, resolved: what its service names in StoreHeader.
func store(main string) string { return resolve(filepath.Join(main, config.Store)) }

// CheckPort refuses a site port another server holds. This clone's own pm service on it is fine (pm init run again),
// and so is a port this clone's installed unit serves on where nothing answers: its own service hung, which install
// restarts.
func CheckPort(main string, port int) error {
	if PortFree(port) {
		return nil
	}
	served := probe(port)
	if served != nil && served.Store != "" && resolve(served.Store) == store(main) {
		return nil
	}
	if served == nil && Installed(main) {
		if p, err := UnitPort(main, PlatformKind()); err == nil && p == port {
			return nil
		}
	}
	what := "a process that does not answer HTTP"
	if served != nil && served.Store != "" {
		what = "the pm service of another store (" + served.Store + ")"
	} else if served != nil {
		what = "a server that is not pm"
	}
	return refuse("the site port :%d is held by %s, so the pm service could not serve there; pm init wrote nothing. "+
		"Run PORT=%d pm init (a free port), or stop what holds :%d", port, what, FreePort(), port)
}

// lockInstall holds the clone's install lock, <main checkout>/.pm/run/install.lock, waiting for another holder: two
// session starts or a typed pm init in parallel would otherwise rewrite and restart the one unit at once.
func lockInstall(main string) (func(), error) {
	path := filepath.Join(RunDir(main), "install.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return func() { syscall.Close(fd) }, nil
}

// Install installs this clone's service to run exe, or brings an installed one up to date, starts it and waits until
// the site answers for this clone's store; "" when it is installed, current and held by the supervisor. Refused
// unless the Tools resolve on the PATH it runs with; failed when the site does not come up (another clone's service
// on the port, say). One install per clone runs at a time.
func Install(main string, port int, exe string) (string, error) {
	unlock, err := lockInstall(main)
	if err != nil {
		return "", err
	}
	defer unlock()
	return installLocked(main, port, exe)
}

// waitUp is a variable so the in-process tests can stand in for a site that comes up at once.
var waitUp = WaitUp

func installLocked(main string, port int, exe string) (string, error) {
	kind, err := Supervisor()
	if err != nil {
		return "", err
	}
	path := os.Getenv("PATH")
	var missing []string
	for _, t := range Tools {
		if !onPath(t, path) {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return "", refuse("the pm service needs %s on PATH; %s not found in %s", strings.Join(Tools, ", "),
			strings.Join(missing, ", "), path)
	}
	unit := UnitFile(main, kind)
	want, err := UnitBytes(main, kind, path, port, Command(exe))
	if err != nil {
		return "", err
	}
	have, err := os.ReadFile(unit)
	changed := err != nil || !bytes.Equal(have, want)
	up := Loaded(main, kind)
	if !changed && up && !Disabled(main, kind) {
		if s := probe(port); s != nil && *s == (Served{store(main), Version()}) {
			return "", nil // a unit held but answering on another build or not at all is restarted
		}
	}
	if changed {
		if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(unit, want, 0o644); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(RunDir(main), 0o755); err != nil { // launchd opens the log but does not make its directory
		return "", err
	}
	name := Label(main)
	var said string
	if kind == Launchd {
		if up { // a changed plist takes effect only once launchd loads it again
			if err := bootout(main); err != nil {
				return "", err
			}
		}
		if err := enable(main, kind); err != nil { // a stopped unit: pm init starts it again
			return "", err
		}
		if err := checked("launchctl", "bootstrap", gui(), unit); err != nil {
			return "", err
		}
		said = fmt.Sprintf("launchd agent %s (%s)", name, unit)
	} else {
		if err := checked("systemctl", "--user", "daemon-reload"); err != nil {
			return "", err
		}
		argv := []string{"systemctl", "--user", "restart", filepath.Base(unit)}
		if !up {
			argv = []string{"systemctl", "--user", "enable", "--now", filepath.Base(unit)}
		} else if err := enable(main, kind); err != nil { // running, but stopped by pm service stop: enabled again
			return "", err
		}
		if err := checked(argv...); err != nil {
			return "", err
		}
		said = fmt.Sprintf("systemd user service %s (%s)", name, unit)
	}
	if err := pmsync.MarkInstalled(main); err != nil {
		return "", err
	}
	verb := "installed"
	if up {
		verb = "updated"
	}
	if _, err := waitUp(main, port, verb+" "+name,
		fmt.Sprintf("if another clone's service holds :%d, give this clone its own port with PORT=<n> pm service install", port)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s the pm service: %s, serving http://localhost:%d and pushing every %d min; log %s", verb,
		said, port, int(pmsync.Interval.Minutes()), LogPath(main)), nil
}

func onPath(tool, path string) bool {
	for _, d := range filepath.SplitList(path) {
		if d == "" {
			continue
		}
		if st, err := os.Stat(filepath.Join(d, tool)); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// bootout unloads the launchd job and waits until launchd let go: bootout returns before the job is gone, and a
// bootstrap fails until then.
func bootout(main string) error {
	if err := checked("launchctl", "bootout", gui()+"/"+Label(main)); err != nil {
		return err
	}
	deadline := time.Now().Add(RestartWait)
	for Loaded(main, Launchd) {
		if time.Now().After(deadline) {
			return refuse("launchd still holds %s %d s after bootout; run pm service install again", Label(main),
				int(RestartWait.Seconds()))
		}
		time.Sleep(poll)
	}
	return nil
}

// WaitUp waits up to RestartWait for the site to answer on port for this clone's store; its health line, or failed
// naming what answers instead, hint and the log.
func WaitUp(main string, port int, done, hint string) (string, error) {
	deadline := time.Now().Add(RestartWait)
	for {
		ok, line := Health(main)
		if ok {
			return line, nil
		}
		served := probe(port)
		if time.Now().After(deadline) || (served != nil && served.Store != store(main)) {
			// past the deadline, or another server holds the port, so this clone's service cannot bind it
			why := line
			if _, after, found := strings.Cut(line, "  "); found {
				why = after
			}
			if hint != "" {
				hint += "; "
			}
			return "", refuse("%s, but the site does not answer for this store on :%d (%s); %sread pm service logs",
				done, port, strings.TrimSpace(why), hint)
		}
		time.Sleep(poll)
	}
}

// Exe is the pm the unit runs: the pm installed at config.BinPath(), the launcher that runs the repo's pin. A
// variable so the in-process tests can run the test binary as the service.
var Exe = config.BinPath

// stale is what to say of a service answering for this store on another pm build than this one.
func stale(build string) string {
	if build == "" {
		build = "older than 0.1.0"
	}
	return fmt.Sprintf("it runs pm %s, not pm %s: pm was replaced since it started; run pm service restart", build,
		Version())
}

// Health is whether the service is up, and one pm where line saying so or what to run.
func Health(main string) (bool, string) {
	kind, err := Supervisor()
	if err != nil {
		return false, "service   " + err.Error()
	}
	name, unit := Label(main), UnitFile(main, kind)
	head := fmt.Sprintf("service   %s %s (%s)  ", kind, name, unit)
	if _, err := os.Stat(unit); err != nil {
		return false, head + "not installed; run pm service install"
	}
	port, err := UnitPort(main, kind)
	if err != nil {
		return false, head + "broken: " + err.Error()
	}
	exe, err := Exe()
	if err != nil {
		return false, head + "broken: " + err.Error()
	}
	want, have := Command(exe), UnitCommand(main, kind)
	if strings.Join(have, "\x00") != strings.Join(want, "\x00") {
		runs := strings.Join(have, " ")
		if have == nil {
			runs = "nothing"
		}
		return false, head + fmt.Sprintf("stale: its unit runs %s, not this pm (%s); run pm service install", runs,
			strings.Join(want, " "))
	}
	if !Loaded(main, kind) {
		if Disabled(main, kind) {
			return false, head + Stopped
		}
		return false, head + fmt.Sprintf("down: %s does not hold it; run pm service restart", kind)
	}
	served := probe(port)
	if served == nil {
		return false, head + fmt.Sprintf("down: nothing answers on :%d; run pm service restart, then pm service logs", port)
	}
	if served.Store == "" || resolve(served.Store) != store(main) {
		what := "a server that is not pm"
		if served.Store != "" {
			what = "another store (" + served.Store + ")"
		}
		return false, head + fmt.Sprintf("down: :%d is held by %s; stop it, then run pm service restart", port, what)
	}
	if served.Version != Version() {
		return false, head + "stale: " + stale(served.Version)
	}
	return true, head + fmt.Sprintf("running; the site answers on :%d", port)
}

// Stopped is the health of a service pm service stop stopped.
const Stopped = "stopped by pm service stop: it stays stopped at login and at session start; run pm service restart to " +
	"start it"

// Status is pm service status: the service's health, its pushes and its garbage collection; non-zero when it is
// down or a push needs attention.
func Status(main, remote string) (int, string, error) {
	ok, line := Health(main)
	described, err := pmsync.Describe(main)
	if err != nil {
		return 1, "", err
	}
	flags, err := pmsync.Flags(main, filepath.Join(main, config.Store), remote)
	if err != nil {
		return 1, "", err
	}
	lines := append([]string{line}, described...)
	for _, f := range flags {
		lines = append(lines, "push      needs attention: "+f)
	}
	lines = append(lines, GCLine(main), fmt.Sprintf("log       %s (pm service logs)", LogPath(main)))
	code := 0
	if !ok || len(flags) > 0 {
		code = 1
	}
	return code, strings.Join(lines, "\n"), nil
}

// Restart restarts the installed service (loads it when the supervisor does not hold it), then waits for the site to
// answer for this store; refused when it is not installed, failed when it does not come up.
func Restart(main string) (string, error) {
	kind, err := Supervisor()
	if err != nil {
		return "", err
	}
	unit, name := UnitFile(main, kind), Label(main)
	if _, err := os.Stat(unit); err != nil {
		return "", refuse("the pm service is not installed (%s is missing); run pm service install", unit)
	}
	if err := enable(main, kind); err != nil { // a stopped unit: restart ends the stop
		return "", err
	}
	switch {
	case kind == Launchd && Loaded(main, kind):
		err = checked("launchctl", "kickstart", "-k", gui()+"/"+name)
	case kind == Launchd:
		err = checked("launchctl", "bootstrap", gui(), unit)
	default:
		err = checked("systemctl", "--user", "restart", filepath.Base(unit))
	}
	if err != nil {
		return "", err
	}
	port, err := UnitPort(main, kind)
	if err != nil {
		return "", err
	}
	line, err := waitUp(main, port, "restarted "+name, "")
	if err != nil {
		return "", err
	}
	return "restarted the pm service\n" + line, nil
}

// StartIfDown starts the installed service when it does not answer, under the clone's install lock, so parallel
// session starts start it once: "" when it answers (a stale one too: it is left as it is), else what the restart
// said. A service pm service stop stopped is left stopped: stopped is true, and nothing is started. Refused when the
// service is not installed.
func StartIfDown(main string) (said string, stopped bool, err error) {
	unlock, err := lockInstall(main)
	if err != nil {
		return "", false, err
	}
	defer unlock()
	kind, err := Supervisor()
	if err != nil {
		return "", false, err
	}
	port, err := UnitPort(main, kind)
	if err != nil {
		return "", false, err
	}
	if s := probe(port); s != nil && s.Store != "" && resolve(s.Store) == store(main) {
		return "", false, nil
	}
	if Disabled(main, kind) {
		return "", true, nil
	}
	said, err = Restart(main)
	return said, false, err
}

// Stop disables this clone's service and stops it, under the clone's install lock, so a parallel session start does
// not start it meanwhile; then waits up to RestartWait until neither the work store's socket nor the site answers,
// and fails naming what still answers. The supervisor's disabled state is the record of the stop: it starts the
// service neither at login nor after a crash, and session start leaves it stopped. Refused when it is not installed.
func Stop(main string) (string, error) {
	kind, err := Supervisor()
	if err != nil {
		return "", err
	}
	unit, name := UnitFile(main, kind), Label(main)
	if _, err := os.Stat(unit); err != nil {
		return "", refuse("the pm service is not installed (%s is missing); there is nothing to stop", unit)
	}
	port, err := UnitPort(main, kind)
	if err != nil {
		return "", err
	}
	unlock, err := lockInstall(main)
	if err != nil {
		return "", err
	}
	defer unlock()
	if kind == Launchd {
		if err := checked("launchctl", "disable", gui()+"/"+name); err != nil {
			return "", err
		}
		if Loaded(main, kind) {
			if err := bootout(main); err != nil {
				return "", refuse("%s", strings.Replace(err.Error(), "run pm service install again",
					"run pm service stop again", 1))
			}
		}
	} else if err := checked("systemctl", "--user", "disable", "--now", filepath.Base(unit)); err != nil {
		return "", err
	}
	sock := work.Sock(main)
	deadline := time.Now().Add(RestartWait)
	for {
		var still []string
		if SockAnswers(sock) {
			still = append(still, "the work store's socket "+sock)
		}
		if s := probe(port); s != nil && s.Store != "" && resolve(s.Store) == store(main) {
			still = append(still, fmt.Sprintf("the site on :%d", port))
		}
		if len(still) == 0 {
			break
		}
		if time.Now().After(deadline) {
			return "", refuse("%s disabled and stopped %s, but %s still answer(s) %d s later: a pm service run of this "+
				"clone outside the supervisor (started by hand?) holds the store; stop that process", kind, name,
				strings.Join(still, " and "), int(RestartWait.Seconds()))
		}
		time.Sleep(poll)
	}
	return fmt.Sprintf("stopped the pm service: %s %s (%s); neither the work store's socket %s nor the site on :%d "+
		"answers. It stays stopped at login and at session start, and every pm command that reads or writes work "+
		"items refuses meanwhile; pm service restart or pm init starts it again", kind, name, unit, sock, port), nil
}

// SockAnswers is whether a process accepts connections on the work store's socket sock.
func SockAnswers(sock string) bool {
	c, err := net.DialTimeout("unix", sock, ProbeTimeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Logs is the last n lines of the service log.
func Logs(main string, n int) (string, error) {
	path := LogPath(main)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", refuse("no service log at %s: the service has not run here; run pm service install", path)
	}
	if err != nil {
		return "", err
	}
	text := strings.ToValidUTF8(string(data), "�")
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
	if text == "" {
		lines = nil
	}
	switch { // Python's lines[-n:]: the whole log for 0, all but the first -n for a negative n
	case n > 0 && n < len(lines):
		lines = lines[len(lines)-n:]
	case n < 0:
		lines = lines[min(len(lines), -n):]
	}
	return strings.Join(lines, "\n"), nil
}

// Uninstall stops this clone's service and removes its unit; "" when none is installed.
func Uninstall(main string) (string, error) {
	kind := PlatformKind()
	unit, name := UnitFile(main, kind), Label(main)
	if _, err := os.Stat(unit); err != nil {
		return "", nil
	}
	if Loaded(main, kind) {
		if kind == Launchd {
			if err := bootout(main); err != nil {
				return "", refuse("%s", strings.Replace(err.Error(), "run pm service install again",
					"run pm uninstall again", 1))
			}
		} else if err := checked("systemctl", "--user", "disable", "--now", filepath.Base(unit)); err != nil {
			return "", err
		}
	}
	if err := os.Remove(unit); err != nil {
		return "", err
	}
	if kind == Launchd && Disabled(main, kind) { // pm service stop's record goes with the unit
		if err := checked("launchctl", "enable", gui()+"/"+name); err != nil {
			return "", err
		}
	}
	if kind == Systemd {
		if err := checked("systemctl", "--user", "daemon-reload"); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("removed the pm service: %s %s (%s)", kind, name, unit), nil
}

// Drift is how this clone's service differs from what pm init installs, one line each: installed, held by the
// supervisor, running this pm, on port.
func Drift(main string, port int) []string {
	kind, err := Supervisor()
	if err != nil {
		return []string{err.Error()}
	}
	unit := UnitFile(main, kind)
	if _, err := os.Stat(unit); err != nil {
		return []string{fmt.Sprintf("not installed (%s is missing); run pm init", unit)}
	}
	var out []string
	if !Loaded(main, kind) {
		if Disabled(main, kind) {
			out = append(out, fmt.Sprintf("%s is %s", Label(main), Stopped))
		} else {
			out = append(out, fmt.Sprintf("%s does not hold %s; run pm service restart", kind, Label(main)))
		}
	}
	have, err := UnitPort(main, kind)
	if err != nil {
		out = append(out, err.Error())
	} else if have != port {
		out = append(out, fmt.Sprintf("%s serves on :%d, not :%d; run pm service install", unit, have, port))
	}
	exe, err := Exe()
	if err != nil {
		out = append(out, err.Error())
	} else if want := Command(exe); strings.Join(UnitCommand(main, kind), "\x00") != strings.Join(want, "\x00") {
		out = append(out, fmt.Sprintf("%s does not run this pm (%s); run pm service install", unit,
			strings.Join(want, " ")))
	}
	if err == nil && have != 0 {
		if s := probe(have); s != nil && s.Store != "" && resolve(s.Store) == store(main) && s.Version != Version() {
			out = append(out, fmt.Sprintf("the service on :%d is stale: %s", have, stale(s.Version)))
		}
	}
	return out
}
