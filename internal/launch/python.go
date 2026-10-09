package launch

// A Python pin (< 0.2.0), launch.py's path ported. A tag makes uv fetch on every run (6 s measured), a commit runs
// from uv's cache (0.2 s). So the first launch of a pin on a machine resolves its tag to a commit (git ls-remote), runs
// it once to fetch and build it, and only then keeps the commit in pins/<pin>/<CommitFile>; a failure or timeout there
// fails hard naming the tag and the command. Deleting the commit file makes the next launch resolve the tag again.
//
// A pin older than 0.1.2 predates the launcher: its pm init reinstalls the pm uv tool at its own version. It runs with
// UV_TOOL_DIR and UV_TOOL_BIN_DIR in its pin's directory, that bin dir first on PATH, so its tool and the service unit
// it writes stay there. It gets no markers: it never reads them and its children would inherit them; Scrub drops
// those dirs from a child's environment before it picks the pin.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

const (
	// CommitFile, in pins/<pin>/, keeps the commit of release tag pm-v<pin> in Repo (Yeeef/pm). Launchers from before
	// pm moved out of yeeef-agents keep that repo's commit of the same tag in pins/<pin>/commit, which Yeeef/pm does
	// not have, so this one is named apart and both launchers can run on one machine.
	CommitFile     = "commit-Yeeef-pm"
	resolveTimeout = 10 * time.Second  // git ls-remote resolving a release tag
	buildTimeout   = 300 * time.Second // uv fetching and building a release the first time
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func requirement(commit string) string {
	return "git+" + config.Repo + "@" + commit
}

// kept is the commit kept for version; "" when there is none or the file holds no commit sha.
func kept(version string) string {
	b, err := os.ReadFile(filepath.Join(PinDir(version), CommitFile))
	if err != nil {
		return ""
	}
	sha := config.PyStrip(string(b))
	if !shaRe.MatchString(sha) {
		return ""
	}
	return sha
}

// pythonEnv is the launched pm's environment: the markers, or for a pin older than the launcher its own uv tool dirs.
func pythonEnv(version string) []string {
	if !old(version) {
		return environ(map[string]string{Launched: version, Launcher: buildinfo.Version})
	}
	d := PinDir(version)
	return environ(map[string]string{"UV_TOOL_DIR": filepath.Join(d, "tools"),
		"UV_TOOL_BIN_DIR": filepath.Join(d, "bin"),
		"PATH":            filepath.Join(d, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")})
}

func pythonHead(version string) string {
	return fmt.Sprintf("this repo pins pm %s, which pm %s runs through uv", version, buildinfo.Version)
}

func uvMissing(version string) error {
	return fmt.Errorf("%s, but uv is not installed (https://docs.astral.sh/uv/)", pythonHead(version))
}

// lastLine is (s.strip().splitlines() or [dflt])[-1].
func lastLine(s, dflt string) string {
	lines := strings.Split(strings.ReplaceAll(config.PyStrip(s), "\r\n", "\n"), "\n")
	if l := lines[len(lines)-1]; l != "" || len(lines) > 1 {
		return l
	}
	return dflt
}

// commit is the commit release tag pm-v<version> names, fetched and built once; kept after the first launch.
func commit(version string, env []string) (string, error) {
	if sha := kept(version); sha != "" {
		return sha, nil
	}
	tag := "pm-v" + version
	fix := fmt.Sprintf("check the network and that tag %s exists, then run pm again; or run it yourself with uv tool "+
		"run --from \"git+%s@%s\" pm …, or move the pin with pm upgrade", tag, config.Repo, tag)
	head := pythonHead(version)
	res, err := proc.Run([]string{"git", "ls-remote", config.Repo, "refs/tags/" + tag, "refs/tags/" + tag + "^{}"},
		proc.Options{Env: environ(map[string]string{"GIT_TERMINAL_PROMPT": "0"}), Timeout: resolveTimeout})
	var perr *proc.Error
	if errors.As(err, &perr) && perr.Type == "FileNotFoundError" {
		return "", fmt.Errorf("%s, but git is not installed to find release tag %s", head, tag)
	} else if errors.As(err, &perr) && perr.Type == "TimeoutExpired" {
		return "", fmt.Errorf("%s, but git ls-remote %s did not answer within %d s for release tag %s; %s", head,
			config.Repo, int(resolveTimeout.Seconds()), tag, fix)
	} else if err != nil {
		return "", err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if sha, ref, ok := strings.Cut(line, "\t"); ok {
			refs[ref] = sha
		}
	}
	sha := refs["refs/tags/"+tag+"^{}"]
	if sha == "" {
		sha = refs["refs/tags/"+tag]
	}
	if res.Code != 0 || sha == "" {
		return "", fmt.Errorf("%s, but release tag %s was not found at %s (%s); %s", head, tag, config.Repo,
			lastLine(res.Stderr, "no such tag"), fix)
	}
	uv, ok := lookPath("uv", env)
	if !ok {
		return "", uvMissing(version)
	}
	res, err = proc.Run([]string{uv, "tool", "run", "--from", requirement(sha), "pm", "--help"},
		proc.Options{Env: env, Timeout: buildTimeout})
	if errors.As(err, &perr) && perr.Type == "TimeoutExpired" {
		return "", fmt.Errorf("%s, but uv did not fetch and build %s (%s) within %d s; %s", head, tag, sha,
			int(buildTimeout.Seconds()), fix)
	} else if err != nil {
		return "", err
	}
	if res.Code != 0 {
		why := config.PyStrip(lastLine(res.Stderr, fmt.Sprintf("exit %d", res.Code)))
		return "", fmt.Errorf("%s, but uv could not fetch and build %s (%s): %s; %s", head, tag, sha, why, fix)
	}
	path := filepath.Join(PinDir(version), CommitFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	// another launch may read it at once: it sees no file or the whole sha
	tmp := fmt.Sprintf("%s.%d", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(sha+"\n"), 0o644); err != nil {
		return "", err
	}
	return sha, os.Rename(tmp, path)
}

// execPython replaces this process with uv tool run of the pin's commit.
func execPython(version string, argv []string) error {
	env := pythonEnv(version)
	sha, err := commit(version, env)
	if err != nil {
		return err
	}
	uv, ok := lookPath("uv", env)
	if !ok {
		return uvMissing(version)
	}
	return syscall.Exec(uv, append([]string{"uv", "--quiet", "tool", "run", "--from", requirement(sha), "pm"}, argv...),
		env)
}
