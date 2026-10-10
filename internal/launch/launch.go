// Package launch is the launcher: the pm binary installed on a machine runs each repo's pinned pm version, so repos on
// different pins share one machine (the pm-go page, Distribution).
//
// main calls Launch before anything else. It reads only `version` in the repo's .pm/config.toml (config.Root picks the
// checkout, as every command does) and runs the command in this process when there is no readable pin, when the pin is
// this pm's version, for pm upgrade without --to X (it moves the pin to the running pm), and when this process was
// launched for that pin already ($PM_LAUNCHED): a release that builds another version then fails the config check
// instead of launching again. The markers are for the launched pm alone: Launch takes them out of the environment, so
// its children (git hooks, pm push's claude -p, any pm it runs) reach the launcher afresh.
//
// Otherwise the pin is launched, replacing this process, so stdin, stdout, stderr, the pid and the exit code are the
// launched pm's: the release binary kept at <data>/pm/pins/<pin>/pm, downloaded once from release pm-v<pin> and checked
// against its SHA256SUMS (go.go). A pin below 0.2.0 names a Python pm release, retired, which pm no longer runs: it
// fails hard, naming the fix (move the pin to a release from 0.2.0 on).
//
// Any failure is a hard error that names the release; nothing falls back.
package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
)

const (
	Launched = "PM_LAUNCHED" // the pin this process was launched for
	Launcher = "PM_LAUNCHER" // the version of the pm that launched it
)

var goPin = []int{0, 2, 0} // the first Go release

// marks are Launched and Launcher as this process got them; Scrub takes them out of the environment.
var marks = map[string]string{}

// Launch runs pm <argv> as the repo's pinned version: nil to run it in this process; otherwise it replaces this
// process and returns only the error that kept the pin from running, for main to print as "error: …" and exit 1.
func Launch(argv []string) error {
	Scrub()
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	version := Target(argv, cwd, marks[Launched])
	if version == "" {
		return nil
	}
	if !IsGo(version) {
		return retired(version, len(argv) > 0 && argv[0] == "upgrade")
	}
	return execGo(version, argv)
}

// retired is the refusal of a version that names no Go release: a Python pm release (below 0.2.0), retired, or no
// release at all; the repo's pin, or for upgrade the version --to names.
func retired(version string, upgrade bool) error {
	what := "a Python pm release, retired: pm runs only releases from 0.2.0 on"
	if _, ok := Key(version); !ok {
		what = "which names no release"
	}
	if upgrade {
		return fmt.Errorf("pm upgrade --to %s names %s; give a release from 0.2.0 on (releases: %s/releases)",
			version, strings.TrimPrefix(what, "which names "), config.Repo)
	}
	return fmt.Errorf("this repo pins pm %s, %s; move the pin to a release from 0.2.0 on with pm upgrade --to <X> "+
		"(releases: %s/releases), and commit .pm/config.toml", version, what, config.Repo)
}

// Pin is the version the config of the checkout containing cwd pins; "" when there is no readable one, for the
// config check to report.
func Pin(cwd string) string {
	root, err := config.Root(cwd)
	if err != nil {
		return ""
	}
	data := map[string]any{}
	if _, err := toml.DecodeFile(filepath.Join(root, config.Rel), &data); err != nil {
		return ""
	}
	v, _ := data["version"].(string)
	return v
}

// Target is the version to launch pm <argv> into; "" to run it in this process. launched is $PM_LAUNCHED as this
// process got it.
func Target(argv []string, cwd, launched string) string {
	var want string
	if len(argv) > 0 && argv[0] == "upgrade" {
		for _, a := range argv {
			if strings.HasPrefix(a, "--to=") {
				want = strings.TrimPrefix(a, "--to=")
				break
			}
		}
		for i, a := range argv[:len(argv)-1] {
			if a == "--to" {
				want = argv[i+1]
				break
			}
		}
	} else {
		want = Pin(cwd)
	}
	if want == buildinfo.Version || want == launched {
		return ""
	}
	return want
}

// Key is version as numbers to compare, a pre-release's "-…" suffix left out; ok is false when it is not dotted
// numbers.
func Key(version string) (key []int, ok bool) {
	for _, p := range strings.Split(strings.SplitN(version, "-", 2)[0], ".") {
		n, err := strconv.Atoi(strings.TrimSpace(p)) // Python's int(): surrounding space and a sign allowed
		if err != nil {
			return nil, false
		}
		key = append(key, n)
	}
	return key, true
}

// Less is Python's tuple comparison a < b.
func Less(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// IsGo is whether version is a Go release: its leading dotted-numeric part, before any "-" suffix, is at least
// 0.2.0 (0.2.0, 0.2.0-rc.1, 0.10.3). Anything else names a retired Python release or none.
func IsGo(version string) bool {
	k, ok := Key(version)
	return ok && !Less(k, goPin)
}

// Pins is the pins cache: $XDG_DATA_HOME/pm/pins, or ~/.local/share/pm/pins.
func Pins() string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, _ := os.UserHomeDir()
		data = filepath.Join(home, ".local/share")
	}
	return filepath.Join(data, "pm/pins")
}

// PinDir is where a pin's launch state lives.
func PinDir(version string) string { return filepath.Join(Pins(), version) }

// Scrub takes the markers out of the environment into marks.
func Scrub() {
	marks = map[string]string{}
	for _, k := range []string{Launched, Launcher} {
		if v, ok := os.LookupEnv(k); ok {
			marks[k] = v
			os.Unsetenv(k)
		}
	}
}

// environ is the environment with set's keys replaced or added.
func environ(set map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if _, ok := set[strings.SplitN(kv, "=", 2)[0]]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range set {
		env = append(env, k+"="+v)
	}
	return env
}

// IsLaunched is whether this pm was launched for its own version: the pm that launched it is another version.
func IsLaunched() bool { return marks[Launched] == buildinfo.Version }

// LauncherVersion is the version of the pm that launched this one ($PM_LAUNCHER as Scrub took it); "" when none did.
func LauncherVersion() string { return marks[Launcher] }

// Markers are the launcher's markers as NAME=value, for a child that is this same pm doing part of this command's
// work (the session-start pm init); none when this pm was not launched. Other children never get them.
func Markers() []string {
	if !IsLaunched() {
		return nil
	}
	return []string{Launched + "=" + marks[Launched], Launcher + "=" + marks[Launcher]}
}

// How is how the running pm was chosen, for pm where and pm doctor.
func How() string {
	if !IsLaunched() {
		return "run in process: it is this repo's pin, or the repo pins none yet"
	}
	v, by := buildinfo.Version, marks[Launcher]
	if by == "" {
		by = "unknown"
	}
	return "this repo's pin, " + filepath.Join(PinDir(v), "pm") + " from release pm-v" + v + ", launched by pm " +
		by + "; delete " + PinDir(v) + " to download it again"
}
