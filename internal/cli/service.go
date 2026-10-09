package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
	"github.com/Yeeef/yeeef-agents/pm/internal/service"
)

// exitCode is a command that printed its output and exits with code, as pm service status does.
type exitCode struct{ code int }

func (e *exitCode) Error() string { return "exit " + strconv.Itoa(e.code) }

// git runs git in dir and returns its stripped stdout; a failure is store.py's RecordError.
func git(dir string, args ...string) (string, error) {
	res, err := proc.Run(append([]string{"git"}, args...), proc.Options{Cwd: &dir})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		why := res.Stderr
		if why == "" {
			why = res.Stdout
		}
		return "", &refusal{fmt.Sprintf("git %s failed in %s: %s", strings.Join(args, " "), dir, config.PyStrip(why))}
	}
	return config.PyStrip(res.Stdout), nil
}

// findStore is the clone's main checkout and its records store, checked: a worktree of this clone on the records
// branch (store.py's find_store). Never a fallback.
func findStore(cwd string) (main, records string, err error) {
	common, err := git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	if filepath.Base(common) != ".git" {
		return "", "", &refusal{fmt.Sprintf("the clone's git dir %s is not a .git directory inside a main checkout", common)}
	}
	main = filepath.Dir(common)
	records = filepath.Join(main, config.Store)
	if !isDir(records) {
		return "", "", &refusal{fmt.Sprintf("no records store at %s; set it up with pm init", records)}
	}
	top, err := git(records, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	branch, err := git(records, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	if resolvePath(top) != resolvePath(records) || branch != "records" {
		return "", "", &refusal{fmt.Sprintf("%s is not a worktree on branch records; move it away and run pm init", records)}
	}
	return main, records, nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// resolvePath is Python's Path.resolve(): symlinks followed where the path exists.
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// runService runs pm service install, status, restart, logs and run (serve.go wires the service to the clone).
func runService(sub string, p *Parsed, here string, stdout, stderr io.Writer) error {
	cfg, err := config.Load(here)
	if err != nil {
		return err
	}
	main, records, err := findStore(here)
	if err != nil {
		return err
	}
	switch sub {
	case "run":
		return serviceRun(cfg, here, main, records, stdout, stderr)
	case "install", "restart":
		var said string
		if sub == "install" {
			said, err = serviceInstall(cfg, main)
		} else {
			said, err = service.Restart(main)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, said)
		return nil
	}
	if sub == "status" {
		code, said, err := service.Status(main, cfg.Remote)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, said)
		if code != 0 {
			return &exitCode{code}
		}
		return nil
	}
	n := 50
	if v := p.Get("lines"); v != "" {
		n, _ = strconv.Atoi(v) // parse checked it is an int
	}
	out, err := service.Logs(main, n)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, out)
	return nil
}
