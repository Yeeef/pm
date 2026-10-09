package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/launch"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
	"github.com/Yeeef/yeeef-agents/pm/internal/service"
	"github.com/Yeeef/yeeef-agents/pm/internal/store"
	pmsync "github.com/Yeeef/yeeef-agents/pm/internal/sync"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// cmdWhere is pm where: with records, the store's path alone, for scripts; without, every location with its state.
// Python source: cmd_where and where_all in cli.py.
func cmdWhere(p *Parsed, here string, stdout io.Writer) error {
	if p.Get("what") == "" {
		out, err := whereAll(here)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, out)
		return err
	}
	records, err := store.Find(here)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, records)
	return err
}

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

// whereIsLink is Path.is_symlink().
func whereIsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

// whereExists is Path.exists(): symlinks followed.
func whereExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// whereAll is every location an agent or the owner needs, each with its state; it works before setup, to show what
// is missing. The work store replaces Beads, so its line takes the place of Python's beads line.
func whereAll(here string) (string, error) {
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
	if !isDir(storeDir) {
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

	top := whereGit(here, "rev-parse", "--show-toplevel")
	if top == "" {
		top = "."
	}
	if resolvedPath(top) == resolvedPath(storeDir) {
		top = main
	}
	link := filepath.Join(top, "records")
	var state string
	switch {
	case whereIsLink(link) && resolvedPath(link) == resolvedPath(storeDir):
		state = "records link set up"
	case !whereExists(link) && !whereIsLink(link):
		state = "no records link; run " + whereSetup
	case !whereIsLink(link) && whereGit(top, "ls-files", "--", "records") != "":
		state = "records/ is main's tracked copy, not the link; run " + whereSetup
	default:
		state = "records is not a link to the store; move it away and run " + whereSetup
	}
	out = append(out, fmt.Sprintf("checkout  %s  branch %s, %s", top, whereGit(top, "rev-parse", "--abbrev-ref", "HEAD"), state))

	workDir, _ := work.Locations(main)
	if work.Exists(workDir) {
		out = append(out, fmt.Sprintf("work      %s  set up", workDir))
	} else {
		out = append(out, fmt.Sprintf("work      %s  missing; run %s", workDir, whereSetup))
	}

	if hp := whereGit(main, "config", "--get", "core.hooksPath"); hp != "" {
		hooks := hp
		if !filepath.IsAbs(hp) {
			hooks = filepath.Join(main, hp)
		}
		hooks = resolvedPath(hooks)
		var have []string
		for _, h := range []string{"post-checkout", "pre-commit"} {
			st := "missing"
			if syscall.Access(filepath.Join(hooks, h), 0x1) == nil {
				st = "installed"
			}
			have = append(have, h+" "+st)
		}
		out = append(out, fmt.Sprintf("hooks     %s  %s", hooks, strings.Join(have, ", ")))
	} else {
		out = append(out, "hooks     core.hooksPath unset; run "+whereSetup)
	}

	home := codexHome()
	switch {
	case !isDir(home):
		out = append(out, fmt.Sprintf("codex     %s  missing, so Codex is not used here", home))
	case !isDir(storeDir):
		out = append(out, fmt.Sprintf("codex     %s  writable_roots not checked without the store; run %s",
			filepath.Join(home, "config.toml"), whereSetup))
	default:
		path := filepath.Join(home, "config.toml")
		state, err := whereCodexState(path, main)
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
	url, err := showSiteURL(here, main)
	if err != nil {
		return "", err
	}
	out = append(out, fmt.Sprintf("site      %s (served by the pm service)", url))
	return strings.Join(out, "\n"), nil
}

// whereCodexState is the codex line's state: the writable roots set, the ones missing, or why the config cannot be
// read. A refusal reads as "unreadable: …"; any other failure (git) is the command's error.
func whereCodexState(path, main string) (string, error) {
	state := func() (string, error) {
		have, err := whereCodexConfig(path)
		if err != nil {
			return "", err
		}
		roots, err := whereCodexRoots(main)
		if err != nil {
			return "", err
		}
		var missing []string
		for _, r := range roots {
			if !contains(have, r) {
				missing = append(missing, r)
			}
		}
		if len(missing) == 0 {
			return "writable_roots set", nil
		}
		return fmt.Sprintf("writable_roots missing %s; run %s", strings.Join(missing, ", "), whereSetup), nil
	}
	s, err := state()
	var r *refusal
	if errors.As(err, &r) {
		return "unreadable: " + r.msg, nil
	}
	return s, err
}

const whereCodexTable = "sandbox_workspace_write"

// whereCodexConfig is codex_config's read side: the writable roots the Codex config already lists; it refuses what it
// cannot read.
func whereCodexConfig(path string) ([]string, error) {
	text, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	data := map[string]any{}
	if _, err := toml.Decode(string(text), &data); err != nil {
		return nil, refuse("cannot parse %s: %s; fix it by hand and run %s again", path, err, whereSetup)
	}
	bad := refuse("%s: %s.writable_roots is not a list of paths; fix it by hand", path, whereCodexTable)
	table, ok := data[whereCodexTable]
	if !ok {
		return nil, nil
	}
	t, ok := table.(map[string]any)
	if !ok {
		return nil, bad
	}
	roots, ok := t["writable_roots"]
	if !ok {
		return nil, nil
	}
	list, ok := roots.([]any)
	if !ok {
		return nil, bad
	}
	have := make([]string, len(list))
	for i, r := range list {
		if have[i], ok = r.(string); !ok {
			return nil, bad
		}
	}
	return have, nil
}

// whereCodexRoots is codex_roots: the clone's git dir, the store, the store's git dir and the Beads dir, then uv's
// cache, which every clone on the machine shares; each resolved.
func whereCodexRoots(main string) ([]string, error) {
	storeDir := filepath.Join(main, config.Store)
	gitDir, err := store.Git(storeDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, p := range []string{filepath.Join(main, ".git"), storeDir, gitDir, filepath.Join(main, ".beads")} {
		roots = append(roots, resolvedPath(p))
	}
	res, err := proc.Run([]string{"uv", "--color", "never", "cache", "dir"}, proc.Options{})
	var pe *proc.Error
	if errors.As(err, &pe) && pe.Type == "FileNotFoundError" {
		return nil, refuse("uv is not installed; pm is a uv tool and needs it")
	}
	if err != nil {
		return nil, err
	}
	if res.Code != 0 || config.PyStrip(res.Stdout) == "" {
		return nil, refuse("uv cache dir failed: %s", config.PyStrip(res.Stderr))
	}
	return append(roots, resolvedPath(config.PyStrip(res.Stdout))), nil
}
