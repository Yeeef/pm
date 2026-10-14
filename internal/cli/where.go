package cli

import (
	"errors"
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
	"slices"
)

// cmdWhere is pm where: with records, the store's path alone, for scripts; without, every location with its state.
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

	top := whereGit(here, "rev-parse", "--show-toplevel")
	if top == "" {
		top = "."
	}
	if fsutil.Resolved(top) == fsutil.Resolved(storeDir) {
		top = main
	}
	link := filepath.Join(top, "records")
	var state string
	switch {
	case fsutil.IsLink(link) && fsutil.Resolved(link) == fsutil.Resolved(storeDir):
		state = "records link set up"
	case !fsutil.Exists(link) && !fsutil.IsLink(link):
		state = "no records link; run " + whereSetup
	case !fsutil.IsLink(link) && whereGit(top, "ls-files", "--", "records") != "":
		state = "records/ is this branch's tracked copy, not the link; merge the main branch, which tracks none, then run " +
			whereSetup
	default:
		state = "records is not a link to the store; move it away and run " + whereSetup
	}
	out = append(out, fmt.Sprintf("checkout  %s  branch %s, %s", top, whereGit(top, "rev-parse", "--abbrev-ref", "HEAD"), state))

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
			if !slices.Contains(have, r) {
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

// whereCodexRoots is the writable roots the clone needs in Codex's sandbox (install.CodexRoots).
func whereCodexRoots(main string) ([]string, error) { return install.CodexRoots(main) }
