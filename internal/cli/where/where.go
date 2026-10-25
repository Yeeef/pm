// Package where is pm where's report: every location an agent or the owner needs, each with its state (All), and the
// site's URL (SiteURL), which pm show and pm record link name too.
package where

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
)

const (
	whereSetup  = "pm init"
	whereBranch = "records"
)

// whereGit is git's stripped stdout in dir, or "" when it fails.
func whereGit(dir string, args ...string) string {
	res, err := proc.Run(append([]string{"git"}, args...), proc.Options{Cwd: &dir})
	if err != nil || res.Code != 0 {
		return ""
	}
	return config.PyStrip(res.Stdout)
}

// All is every location an agent or the owner needs, each with its state; it works before setup, to show what
// is missing. The work store replaces Beads, so its line takes the place of Python's beads line.
func All(here string) (string, error) {
	storeDir, err := store.PathOf(here)
	if err != nil {
		return "", err
	}
	main := store.MainOf(storeDir)
	cfg, err := config.Load(here)
	if err != nil {
		return "", err
	}

	out := []string{fmt.Sprintf("pm        %s  %s", buildinfo.Version, launch.How())}
	if !fsutil.IsDir(storeDir) {
		out = append(out, fmt.Sprintf("store     %s  missing; run %s", storeDir, whereSetup))
	} else {
		branch := whereGit(storeDir, "rev-parse", "--abbrev-ref", "HEAD")
		line := fmt.Sprintf("store     %s  branch %s", storeDir, branch)
		if branch != whereBranch {
			line += fmt.Sprintf(" (must be %s)", whereBranch)
		}
		upstream := cfg.Remote + "/" + whereBranch
		counts := strings.Fields(whereGit(storeDir, "rev-list", "--left-right", "--count", "HEAD..."+upstream))
		if len(counts) > 0 {
			line += fmt.Sprintf(", %s ahead, %s behind %s (as of the last fetch)", counts[0], counts[1], upstream)
		} else {
			line += ", no " + upstream
		}
		out = append(out, line)
	}
	out = append(out, "clone     "+store.DescribeClone(main, cfg.Remote))

	top := whereGit(here, "rev-parse", "--show-toplevel")
	if top == "" {
		top = "."
	}
	if fsutil.Resolved(top) == fsutil.Resolved(storeDir) {
		top = main
	} else if store.IsWorkspace(top) {
		top = filepath.Dir(top)
	}
	link := filepath.Join(top, "records")
	ws, wsErr := store.WorkspaceAt(top)
	var state string
	switch {
	case wsErr != nil:
		state = wsErr.Error()
	case ws != nil:
		if state, err = ws.Describe(storeDir); err != nil {
			state = "records workspace unreadable: " + err.Error()
		}
	case fsutil.IsLink(link) && fsutil.Resolved(link) == fsutil.Resolved(storeDir) && fsutil.Resolved(top) != fsutil.Resolved(main):
		state = "records link to the shared store; " + whereSetup + " makes it this worktree's records workspace"
	case fsutil.IsLink(link) && fsutil.Resolved(link) == fsutil.Resolved(storeDir):
		state = "records link set up"
	case !fsutil.Exists(link) && !fsutil.IsLink(link) && fsutil.Resolved(top) != fsutil.Resolved(main):
		state = "no records workspace; run " + whereSetup
	case !fsutil.Exists(link) && !fsutil.IsLink(link):
		state = "no records link; run " + whereSetup
	case !fsutil.IsLink(link) && whereGit(top, "ls-files", "--", "records") != "":
		state = "records/ is this branch's tracked copy, not the link; merge the main branch, which tracks none, then run " +
			whereSetup
	default:
		state = "records is not a link to the store; move it away and run " + whereSetup
	}
	out = append(out, fmt.Sprintf("checkout  %s  branch %s, %s", top, whereGit(top, "rev-parse", "--abbrev-ref", "HEAD"), state))
	out = append(out, store.Orphans(storeDir)...)

	workDir, _ := work.Locations(main)
	workState, err := install.WorkState(main, cfg.Remote)
	if err != nil {
		return "", err
	}
	out = append(out, fmt.Sprintf("work      %s  %s", workDir, workState))

	home := codexHome()
	switch {
	case !fsutil.IsDir(home):
		out = append(out, fmt.Sprintf("codex     %s  missing, so Codex is not used here", home))
	case !fsutil.IsDir(storeDir):
		out = append(out, fmt.Sprintf("codex     %s  writable_roots not checked without the store; run %s",
			filepath.Join(home, "config.toml"), whereSetup))
	default:
		path := filepath.Join(home, "config.toml")
		state, err := install.CodexState(path, main)
		if err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("codex     %s  %s", path, state))
	}

	_, health := service.Health(main)
	out = append(out, health)
	described, err := pmsync.Describe(main)
	if err != nil {
		return "", err
	}
	out = append(out, described...)
	url, err := SiteURL(here, main)
	if err != nil {
		return "", err
	}
	out = append(out, fmt.Sprintf("site      %s (served by the pm service)", url))
	return strings.Join(out, "\n"), nil
}

// SiteURL is Python's site_url(): the repo's site_url, else http://localhost:<port>.
func SiteURL(here, main string) (string, error) {
	cfg, err := config.Load(here)
	if err != nil {
		return "", err
	}
	if cfg.SiteURL != "" {
		return cfg.SiteURL, nil
	}
	port, err := service.PortFor(main, int(cfg.Port))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://localhost:%d", port), nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func codexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(homeDir(), ".codex")
}
