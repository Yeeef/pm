// Package service is the pm service: one supervised background process per clone, `pm service run` in the main
// checkout, that serves the site from the records and the work store, delivers the owner's site replies and reviewed
// PRs' merges into the sessions that raised them, and syncs the stores. This file holds its unit under the machine's
// supervisor: a launchd agent with KeepAlive on macOS, a systemd user service on Linux, byte for byte what Python pm
// writes for the same inputs. Python source: service.py.
package service

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
)

// Launchd and Systemd are the supervisors the service runs under.
const (
	Launchd = "launchd"
	Systemd = "systemd"
)

// Error is a refusal or failure of a service command, printed as "error: <message>".
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func refuse(format string, a ...any) error { return &Error{fmt.Sprintf(format, a...)} }

// resolve is Python's Path.resolve(): absolute, symlinks followed where the path exists.
func resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return filepath.Clean(abs)
}

// Label names one clone's service: its directory name and a hash of its resolved path.
func Label(main string) string {
	sum := sha1.Sum([]byte(resolve(main)))
	return fmt.Sprintf("local.pm.%s.%s", filepath.Base(main), hex.EncodeToString(sum[:])[:8])
}

// RunDir is the clone's runtime-state directory, <main checkout>/.pm/run; writers create it.
func RunDir(main string) string { return filepath.Join(main, config.Run) }

// LogPath is the service's log, which the supervisor appends its stdout and stderr to.
func LogPath(main string) string { return filepath.Join(RunDir(main), "service.log") }

// PlatformKind is the supervisor this platform's unit is for, without asking it; a variable so tests can play the
// other platform.
var PlatformKind = func() string {
	if runtime.GOOS == "darwin" {
		return Launchd
	}
	return Systemd
}

// home is Python's Path.home(): $HOME.
func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// UnitDir is where the supervisor of kind reads user units.
func UnitDir(kind string) string {
	if kind == Launchd {
		return filepath.Join(home(), "Library/LaunchAgents")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home(), ".config")
	}
	return filepath.Join(base, "systemd/user")
}

// UnitFile is the clone's unit file for the supervisor of kind.
func UnitFile(main, kind string) string {
	if kind == Launchd {
		return filepath.Join(UnitDir(kind), Label(main)+".plist")
	}
	return filepath.Join(UnitDir(kind), Label(main)+".service")
}

// Command is what the supervisor runs: pm at exe, the binary in the bin dir, whose path stays across versions.
func Command(exe string) []string { return []string{exe, "service", "run"} }

// UnitBytes is the unit for kind that runs argv in main with PATH path and the site on port.
func UnitBytes(main, kind, path string, port int, argv []string) ([]byte, error) {
	if kind == Launchd {
		return launchdPlist(main, path, port, argv)
	}
	return []byte(systemdUnit(main, path, port, argv)), nil
}

// launchdPlist is plistlib.dumps of the job Python's launchd_job builds: XML, keys sorted, tab indents.
func launchdPlist(main, path string, port int, argv []string) ([]byte, error) {
	log := LogPath(main)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n" +
		`<plist version="1.0">` + "\n<dict>\n")
	var bad error
	str := func(indent, s string) {
		if strings.ContainsFunc(s, plistControl) && bad == nil {
			bad = refuse("a launchd plist cannot hold %q: it has a control character", s)
		}
		b.WriteString(indent + "<string>" + plistEscape(s) + "</string>\n")
	}
	key := func(indent, k string) { b.WriteString(indent + "<key>" + k + "</key>\n") }
	// the keys in sorted order, as plistlib writes them
	key("\t", "EnvironmentVariables")
	b.WriteString("\t<dict>\n")
	key("\t\t", "PATH")
	str("\t\t", path)
	key("\t\t", "PORT")
	str("\t\t", strconv.Itoa(port))
	b.WriteString("\t</dict>\n")
	key("\t", "KeepAlive")
	b.WriteString("\t<true/>\n")
	key("\t", "Label")
	str("\t", Label(main))
	key("\t", "ProgramArguments")
	if len(argv) == 0 {
		b.WriteString("\t<array/>\n")
	} else {
		b.WriteString("\t<array>\n")
		for _, a := range argv {
			str("\t\t", a)
		}
		b.WriteString("\t</array>\n")
	}
	key("\t", "RunAtLoad")
	b.WriteString("\t<true/>\n")
	key("\t", "StandardErrorPath")
	str("\t", log)
	key("\t", "StandardOutPath")
	str("\t", log)
	key("\t", "WorkingDirectory")
	str("\t", main)
	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String()), bad
}

// plistControl is a character plistlib refuses in a string.
func plistControl(r rune) bool { return r < 0x20 && r != '\t' && r != '\n' && r != '\r' }

// plistEscape is plistlib's _escape: line endings to \n, then &, < and >.
func plistEscape(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// unitQuote is a systemd unit value, double-quoted, with % (which starts a specifier) escaped.
func unitQuote(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(v) + `"`
}

func systemdUnit(main, path string, port int, argv []string) string {
	pct := func(s string) string { return strings.ReplaceAll(s, "%", "%%") }
	log := pct(LogPath(main))
	exec := make([]string, len(argv))
	for i, a := range argv {
		exec[i] = unitQuote(a)
	}
	// WorkingDirectory= takes the rest of the line as the path: quotes would be part of it, and systemd then refuses
	// the unit as not absolute.
	return "[Unit]\nDescription=pm service for " + main + "\n\n[Service]\nType=simple\n" +
		"WorkingDirectory=" + pct(main) + "\n" +
		"Environment=" + unitQuote("PATH="+path) + " " + unitQuote("PORT="+strconv.Itoa(port)) + "\n" +
		"ExecStart=" + strings.Join(exec, " ") + "\nRestart=always\nRestartSec=5\n" +
		"StandardOutput=append:" + log + "\nStandardError=append:" + log + "\n\n[Install]\nWantedBy=default.target\n"
}

var (
	plistPort   = regexp.MustCompile(`<key>PORT</key>\s*<string>(\d+)</string>`)
	unitPort    = regexp.MustCompile(`"PORT=(\d+)"`)
	anyPort     = regexp.MustCompile(`PORT(?:</key>\s*<string>|=)(\d+)`)
	execStart   = regexp.MustCompile(`(?m)^ExecStart=(.*)$`)
	quotedWord  = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	plistArgs   = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>(.*?)</array>`)
	plistString = regexp.MustCompile(`(?s)<string>(.*?)</string>`)
)

// UnitPort is the port the installed unit serves on, as install wrote it.
func UnitPort(main, kind string) (int, error) {
	path := UnitFile(main, kind)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, refuse("%s sets no PORT (%v); run pm service install again", path, err)
	}
	re := unitPort
	if kind == Launchd {
		re = plistPort
	}
	found := re.FindSubmatch(data)
	if found == nil {
		return 0, refuse("%s sets no PORT; run pm service install again", path)
	}
	return strconv.Atoi(string(found[1]))
}

// UnitCommand is the command the installed unit runs, as install wrote it; nil when it names none.
func UnitCommand(main, kind string) []string {
	data, err := os.ReadFile(UnitFile(main, kind))
	if err != nil {
		return nil
	}
	if kind == Launchd {
		found := plistArgs.FindSubmatch(data)
		if found == nil {
			return nil
		}
		var out []string
		for _, m := range plistString.FindAllSubmatch(found[1], -1) {
			s := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(string(m[1]))
			out = append(out, s)
		}
		return out
	}
	found := execStart.FindSubmatch(data)
	if found == nil {
		return nil
	}
	var out []string
	for _, w := range quotedWord.FindAllString(string(found[1]), -1) {
		out = append(out, strings.ReplaceAll(unquoteC(w[1:len(w)-1]), "%%", "%"))
	}
	return out
}

// unquoteC undoes unitQuote's backslash escapes.
func unquoteC(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Installed is whether the clone's unit file exists for this platform's supervisor.
func Installed(main string) bool {
	_, err := os.Stat(UnitFile(main, PlatformKind()))
	return err == nil
}

// PortFor is the clone's site port: $PORT, else the installed unit's (so installing again keeps a clone's own port
// and every link and probe uses the port the service serves on), else def, the config's.
func PortFor(main string, def int) (int, error) {
	if p := os.Getenv("PORT"); p != "" {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0, fmt.Errorf("invalid literal for int() with base 10: %q", p)
		}
		return n, nil
	}
	if Installed(main) {
		return UnitPort(main, PlatformKind())
	}
	return def, nil
}

// UnitPorts is the ports every pm service unit on this machine serves on, this clone's too; a unit that names none
// is skipped, and so is a directory (a systemd drop-in, local.pm.<name>.service.d).
func UnitPorts() map[int]bool {
	out := map[int]bool{}
	matches, _ := filepath.Glob(filepath.Join(UnitDir(PlatformKind()), "local.pm.*"))
	sort.Strings(matches)
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if found := anyPort.FindSubmatch(data); found != nil {
			if n, err := strconv.Atoi(string(found[1])); err == nil {
				out[n] = true
			}
		}
	}
	return out
}

// FirstPort is where a new repo's site port starts: the first free one from here.
var FirstPort = 8000

// FreePort is the first port from FirstPort up that is free and no pm service unit on this machine names.
func FreePort() int {
	taken := UnitPorts()
	port := FirstPort
	for taken[port] || !PortFree(port) {
		port++
	}
	return port
}
