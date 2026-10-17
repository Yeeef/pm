// Package setup runs pm's setup commands on a clone: pm init (init.go), pm doctor (doctor.go), pm upgrade
// (upgrade.go), pm uninstall (uninstall.go) and the retired git hooks' commands (hooks.go), on install's pieces.
package setup

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// pm init, pm doctor, pm upgrade, pm uninstall and the retired git hooks. The pieces and
// the clone's setup are internal/install's.

// latest is how to install the newest Go pm release, whose launcher launches any pin: its install.sh (install.sh).
const latest = `curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh | sh`

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// codeTop is the code worktree a repo-level command acts on; refused inside the store or a records workspace.
func codeTop(here, records, what string) (string, error) {
	top, err := install.Git(here, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if fsutil.Resolved(top) == fsutil.Resolved(records) {
		return "", fmt.Errorf("%s is the records store; run pm %s from a code worktree", top, what)
	}
	if store.IsWorkspace(top) {
		return "", fmt.Errorf("%s is a records workspace; run pm %s from its code worktree %s", top, what, filepath.Dir(top))
	}
	return top, nil
}

// checkSiteURL refuses a --site-url that is not an http(s) base URL.
func checkSiteURL(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" || p.Path != "" ||
		p.RawQuery != "" || p.Fragment != "" {
		return fmt.Errorf("--site-url %q is not an http(s) base URL like https://pm.example.com", u)
	}
	return nil
}

func siteURLArg(v string) string { return strings.TrimRight(config.PyStrip(v), "/") }

// keyOrder is the order install.ConfigText writes the config's keys in, which pm doctor compares the file with: read
// from a config that sets every key.
var keyOrder = func() []string {
	var keys []string
	all := install.ConfigText(install.Settings{Remote: "r", MainBranch: "m", Port: 1, SiteURL: "s", Listen: "l", Access: "a",
		AccessTeam: "t", AccessAud: "u", SummaryRuntime: "c", DesignSources: []string{"d"}})
	for _, line := range strings.Split(strings.TrimSpace(all), "\n") {
		keys = append(keys, strings.TrimSpace(strings.SplitN(line, "=", 2)[0]))
	}
	return keys
}()

// setKey sets key to the TOML string v in the config file at path, in place of its line, else before the first key
// ConfigText writes after it, else appended; "" removes it.
func setKey(path, key, v string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text, line := string(b), ""
	if v != "" {
		line = key + ` = "` + v + "\"\n"
	}
	if loc := regexp.MustCompile(`(?m)^` + key + `\s*=.*\n?`).FindStringIndex(text); loc != nil {
		text = text[:loc[0]] + line + text[loc[1]:]
		return os.WriteFile(path, []byte(text), 0o644)
	}
	for _, later := range keyOrder[slices.Index(keyOrder, key)+1:] {
		if loc := regexp.MustCompile(`(?m)^` + later + `\s*=`).FindStringIndex(text); loc != nil && v != "" {
			return os.WriteFile(path, []byte(text[:loc[0]]+line+text[loc[0]:]), 0o644)
		}
	}
	if v != "" {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += line
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// setupSiteURL stores the repo's public site URL as site_url in .pm/config.toml ("" removes it), for the owner to
// commit; "" when nothing changed.
func setupSiteURL(v string, c config.Config, main string) (string, error) {
	u := siteURLArg(v)
	if u != "" {
		if err := checkSiteURL(u); err != nil {
			return "", err
		}
	}
	old := c.SiteURL
	if u == old {
		return "", nil
	}
	if err := setKey(c.Path, "site_url", u); err != nil {
		return "", err
	}
	if u != "" {
		was := ""
		if old != "" {
			was = " (was " + old + ")"
		}
		return fmt.Sprintf("site URL set to %s%s in %s; commit it", u, was, c.Path), nil
	}
	port, err := service.PortFor(main, int(c.Port))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("site URL cleared (was %s) in %s; commit it; links use http://localhost:%d", old, c.Path, port), nil
}

// reach is pm init's --listen and --access, each "" when not given; "" for --listen clears it (the site binds
// loopback again).
type reach struct {
	listen, access           string
	listenGiven, accessGiven bool
}

func reachArgs(p Arguments) reach {
	return reach{listen: config.PyStrip(p.Get("listen")), access: p.Get("access"),
		listenGiven: len(p.ValuesOf("listen")) > 0, accessGiven: len(p.ValuesOf("access")) > 0}
}

// check refuses a --listen that is not an IP address.
func (r reach) check() error {
	if r.listen != "" && net.ParseIP(r.listen) == nil {
		return fmt.Errorf("--listen %q is not an IP address like %s (this machine only) or 0.0.0.0 (every interface)",
			r.listen, config.Loopback)
	}
	return nil
}

// needsAccess refuses settings that expose the site with no access, which the pm service would not start on.
func needsAccess(c config.Config) error {
	if slices.Contains(c.AccessMissing(), "access") {
		return fmt.Errorf("the site would be reachable beyond this machine (listen = %q, site_url = %q) with no access "+
			"set, which the pm service does not start on: give --access cloudflare (Cloudflare Access verifies each "+
			"request) or --access none (the network is the gate); pm init wrote nothing", c.ListenAddr(), c.SiteURL)
	}
	return nil
}

// planConfig is whether a later pm init migrates c to access = "cloudflare"; refused when the config it would write
// exposes the site with no access after a flag changed it.
func planConfig(c config.Config, siteArg string, siteGiven bool, r reach) (bool, error) {
	want, migrate := c.Migrated()
	migrate = migrate && !r.accessGiven
	if siteGiven {
		want.SiteURL = siteURLArg(siteArg)
	}
	if r.listenGiven {
		want.Listen = r.listen
	}
	if r.accessGiven {
		want.Access = r.access
	}
	if siteGiven || r.listenGiven || r.accessGiven {
		return migrate, needsAccess(want)
	}
	return migrate, nil
}

// setupConfig is a later pm init's writes to .pm/config.toml, each said: the migration that gives a config with the
// Cloudflare Access keys and no access access = "cloudflare" (what it meant before access existed), then
// --site-url, --listen and --access as given. Refused, writing nothing, when the result would expose the site with no
// access.
func setupConfig(c config.Config, main, siteArg string, siteGiven bool, r reach) ([]string, error) {
	migrate, err := planConfig(c, siteArg, siteGiven, r)
	if err != nil {
		return nil, err
	}
	var said []string
	if migrate {
		if err := setKey(c.Path, "access", "cloudflare"); err != nil {
			return nil, err
		}
		said = append(said, fmt.Sprintf(`access set to "cloudflare" in %s, as its access_team means; commit it`, c.Path))
	}
	if siteGiven {
		line, err := setupSiteURL(siteArg, c, main)
		if err != nil {
			return nil, err
		}
		if line != "" {
			said = append(said, line)
		}
	}
	for _, kv := range []struct {
		key, old, v string
		given       bool
	}{{"listen", c.Listen, r.listen, r.listenGiven}, {"access", c.Access, r.access, r.accessGiven}} {
		if !kv.given || kv.v == kv.old {
			continue
		}
		if err := setKey(c.Path, kv.key, kv.v); err != nil {
			return nil, err
		}
		if kv.v == "" {
			said = append(said, fmt.Sprintf("%s cleared (was %s) in %s; commit it", kv.key, kv.old, c.Path))
		} else {
			said = append(said, fmt.Sprintf("%s set to %s in %s; commit it; pm service restart applies it", kv.key, kv.v, c.Path))
		}
	}
	return said, nil
}

// initSettings is a new repo's settings: the remote origin, which must exist, and its default branch (origin/HEAD),
// else the branch checked out; port is the site port.
func initSettings(top, siteArg string, given bool, r reach, port int) (install.Settings, error) {
	remote := "origin"
	if install.RemoteURL(top, remote) == "" {
		return install.Settings{}, fmt.Errorf("this repo has no remote %s; pm keeps the records branch and the work store "+
			"there, so add it (git remote add %s URL) and run pm init again", remote, remote)
	}
	res, err := proc.Run([]string{"git", "symbolic-ref", "--quiet", "--short", "refs/remotes/" + remote + "/HEAD"},
		proc.Options{Cwd: &top})
	if err != nil {
		return install.Settings{}, err
	}
	branch := strings.TrimPrefix(config.PyStrip(res.Stdout), remote+"/")
	if branch == "" {
		if branch, err = install.Git(top, "symbolic-ref", "--short", "HEAD"); err != nil {
			return install.Settings{}, err
		}
	}
	u := ""
	if given {
		u = siteURLArg(siteArg)
	}
	if u != "" {
		if err := checkSiteURL(u); err != nil {
			return install.Settings{}, err
		}
	}
	if err := needsAccess(config.Config{SiteURL: u, Listen: r.listen, Access: r.access}); err != nil {
		return install.Settings{}, err
	}
	return install.Settings{Remote: remote, MainBranch: branch, Port: port, SiteURL: u, Listen: r.listen, Access: r.access}, nil
}

// worktreeChanges is each path git status shows in top (untracked files one by one), in its order, with its status and,
// for a file, a hash of its bytes, so a file changed again under the same status still differs.
func worktreeChanges(top string) ([]string, map[string]string, error) {
	res, err := proc.Run([]string{"git", "status", "--porcelain=v1", "-z", "--untracked-files=all"},
		proc.Options{Cwd: &top})
	if err != nil {
		return nil, nil, err
	}
	if res.Code != 0 {
		return nil, nil, fmt.Errorf("git status failed in %s: %s", top, config.PyStrip(res.Stderr))
	}
	var order []string
	out := map[string]string{}
	entries := strings.Split(res.Stdout, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		code, rel := e[:2], e[3:]
		if strings.ContainsAny(code, "RC") {
			i++ // a rename's or copy's source path
		}
		digest := ""
		p := filepath.Join(top, rel)
		if st, err := os.Lstat(p); err == nil && st.Mode().IsRegular() {
			if b, err := os.ReadFile(p); err == nil {
				sum := sha1.Sum(b)
				digest = hex.EncodeToString(sum[:])
			}
		}
		if _, seen := out[rel]; !seen {
			order = append(order, rel)
		}
		out[rel] = code + " " + digest
	}
	return order, out, nil
}

// commitHint is the commit to make of what pm init changed in top since before; "" when nothing changed.
func commitHint(top string, before map[string]string, written []string) (string, error) {
	order, after, err := worktreeChanges(top)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	var changed []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			changed = append(changed, p)
		}
	}
	for _, p := range written {
		add(p)
	}
	for _, p := range order {
		if b, ok := before[p]; !ok || b != after[p] {
			add(p)
		}
	}
	if len(changed) == 0 {
		return "", nil
	}
	branch, err := install.Git(top, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pm commits nothing on %s; commit what pm init changed there: git add -- %s && git commit -m "+
		"\"Install pm %s\"", branch, strings.Join(changed, " "), buildinfo.Version), nil
}

// Init is pm init: the repo's half only when the worktree has no .pm/config.toml yet (a first install; pm doctor
// reports and pm upgrade rewrites a changed piece after that), then the clone's and this worktree's half every time,
// pm on the machine, and the pm service last. Session start runs it, so every worktree an agent works in is ready. The
// repo's pieces are written, never committed: the output names the commit to make, also when a later step fails.
func Init(p Arguments, here string, stdout io.Writer) error {
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(records)
	top, err := codeTop(here, records, "init")
	if err != nil {
		return err
	}
	sessionStart := p.Get("session_start") == "true"
	siteArg, siteGiven := p.Get("site_url"), len(p.ValuesOf("site_url")) > 0
	r := reachArgs(p)
	if err := r.check(); err != nil {
		return err
	}
	fresh := !isFile(filepath.Join(top, config.Rel))
	if err := checkInitWorktree(here, top, main, fresh); err != nil {
		return err
	}
	s, err := settingsForInit(here, top, main, siteArg, siteGiven, r, fresh)
	if err != nil {
		return err
	}
	if err := install.RefuseLegacy(top, main, fresh); err != nil {
		return err
	}
	if !(sessionStart && service.Installed(main)) { // session start leaves an installed service alone
		port := s.Port
		if !fresh {
			if port, err = service.PortFor(main, s.Port); err != nil {
				return err
			}
		}
		listen := s.Listen
		if r.listenGiven {
			listen = r.listen
		}
		if listen == "" {
			listen = config.Loopback
		}
		if err := service.CheckPort(main, listen, port); err != nil { // refused before anything is written
			return err
		}
	}
	_, before, err := worktreeChanges(top)
	if err != nil {
		return err
	}
	if fresh {
		if _, err := install.Plan(top, s); err != nil { // read-only: a refusal comes before any write
			return err
		}
	}
	var out, written []string
	done, err := initSteps(here, top, main, s, fresh, sessionStart, siteArg, siteGiven, r, &out, &written)
	if err != nil {
		hint, herr := commitHint(top, before, written)
		if herr != nil || hint == "" {
			return errors.Join(err, herr)
		}
		return fmt.Errorf("%s", strings.Join(append(append([]string{err.Error()}, out...), hint,
			"then fix the error above and run pm init again"), "\n"))
	}
	hint, err := commitHint(top, before, written)
	if err != nil {
		return err
	}
	var lines []string
	for _, l := range []string{done, hint} {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if c, err := config.Load(here); err == nil && !sessionStart { // a typed pm init says what the config leaves unserved
		if why := c.Unaddressed(); why != "" {
			lines = append(lines, "warning: "+why)
		}
	}
	if !sessionStart { // session start runs no Codex: it would cost seconds at every start
		skips, _, err := install.CodexTrust(main, install.CodexTrustTimeout)
		if err != nil {
			return err
		}
		lines = append(lines, skips...)
	}
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

// initSteps is pm init's writes, in order; out and written say how far it got when one fails. The repo's half (pm's
// pieces, the records branch) runs only when fresh.
func initSteps(here, top, main string, s install.Settings, fresh, sessionStart bool, siteArg string, siteGiven bool,
	r reach, out, written *[]string) (string, error) {
	said, err := install.InstallBinary() // the service's unit and the hooks run the bin-dir pm, not this one
	if err != nil {
		return "", err
	}
	if said != "" {
		*out = append(*out, said)
	}
	if err := initRepo(here, top, main, s, fresh, siteArg, siteGiven, r, out, written); err != nil {
		return "", err
	}
	done, err := install.SetupClone(here, s.Remote)
	if err != nil {
		return "", err
	}
	*out = append(*out, done)
	return initService(here, main, sessionStart, out)
}

func checkInitWorktree(here, top, main string, fresh bool) error {
	if fsutil.Resolved(top) != fsutil.Resolved(main) {
		if fresh { // pm's files come from the main branch; a linked worktree without them is on a branch from before
			return fmt.Errorf("this worktree's branch has no %s: it was cut before pm was installed in this repo, and pm "+
				"init installs the repo's files only in the main checkout; merge the main branch into this one, or run "+
				"pm init in the main checkout %s; pm init wrote nothing", config.Rel, main)
		}
		// the service runs in the main checkout, under its pin, with the one pm the hooks run too; this worktree's
		// setup does not depend on either, so it runs first and only pm's install and the service are refused
		var why string
		pin := ""
		if mc, err := config.Read(main); err != nil {
			why = fmt.Sprintf("the pm service runs in the main checkout %s, under its branch's pin, and that branch has "+
				"none (%s); run pm init there, or check out a branch there that pins pm %s", main, err, buildinfo.Version)
		} else {
			pin = mc.Version
			why = fmt.Sprintf("the main checkout %s pins pm %s, and the pm service and the installed pm follow it, but "+
				"this checkout pins pm %s: %s", main, pin, buildinfo.Version, work.PinFix(buildinfo.Version, mc))
		}
		if pin != buildinfo.Version {
			c, err := config.Load(here)
			if err != nil {
				return err
			}
			done, err := install.SetupClone(here, c.Remote)
			if err != nil {
				return err
			}
			return fmt.Errorf("%s. pm init set up this worktree and left the installed pm and the service alone:\n%s", why, done)
		}
	}
	return nil
}

func settingsForInit(here, top, main, siteArg string, siteGiven bool, r reach, fresh bool) (install.Settings, error) {
	var s install.Settings
	if fresh { // the site port: $PORT, else the clone's unit's, else the first free one no pm unit here names
		listen := r.listen
		if listen == "" {
			listen = config.Loopback
		}
		port, err := service.PortFor(main, service.FreePort(listen))
		if err != nil {
			return install.Settings{}, err
		}
		if s, err = initSettings(top, siteArg, siteGiven, r, port); err != nil {
			return install.Settings{}, err
		}
	} else {
		c, err := config.Load(here)
		if err != nil {
			return install.Settings{}, err
		}
		s = install.SettingsOf(c)
		if siteGiven && config.PyStrip(siteArg) != "" { // "/" is refused here, as no base URL
			if err := checkSiteURL(siteURLArg(siteArg)); err != nil { // before anything is written
				return install.Settings{}, err
			}
		}
		if _, err := planConfig(c, siteArg, siteGiven, r); err != nil { // before anything is written
			return install.Settings{}, err
		}
	}
	return s, nil
}

func initRepo(here, top, main string, s install.Settings, fresh bool, siteArg string, siteGiven bool, r reach, out, written *[]string) error {
	if fresh {
		planned, err := install.Plan(top, s)
		if err != nil {
			return err
		}
		res, err := proc.Run([]string{"git", "rev-parse", "--verify", "--quiet", "refs/heads/" + store.Branch},
			proc.Options{Cwd: &top})
		if err != nil {
			return err
		}
		if res.Code != 0 {
			has, err := install.RemoteHasBranch(top, s.Remote, store.Branch)
			if err != nil {
				return err
			}
			if !has {
				said, err := install.CreateRecordsBranch(top, s.Remote, store.Branch)
				if err != nil {
					return err
				}
				*out = append(*out, said)
			}
		}
		w, err := install.Write(planned)
		*written = append(*written, w...)
		if err != nil {
			return err
		}
		*out = append(*out, install.Said(planned)...)
	} else {
		c, err := config.Load(here)
		if err != nil {
			return err
		}
		said, err := setupConfig(c, main, siteArg, siteGiven, r)
		if err != nil {
			return err
		}
		*out = append(*out, said...)
	}
	return nil
}

func initService(here, main string, sessionStart bool, out *[]string) (string, error) {
	// the work store is the service's: the service comes up first, then attaches the store (CALL pm_setup())
	attach := func() error {
		said, err := install.SetupWork(main)
		*out = append(*out, said...)
		return err
	}
	if sessionStart && service.Installed(main) {
		// A service that does not answer is started, once for parallel session starts (the clone's install lock); a
		// running one, a stale one included, is left as it is and reported, since a restart from a session start
		// would restart it under every other session too; a stopped one (pm service stop) is left stopped.
		said, stopped, err := service.StartIfDown(main)
		if err != nil {
			return "", err
		}
		if stopped { // pm service stop's stop holds until a typed pm init or pm service restart
			*out = append(*out, "left the pm service stopped: it was "+service.Stopped)
			return strings.Join(*out, "\n"), nil
		}
		if said != "" {
			*out = append(*out, said)
		}
		if ok, line := service.Health(main); !ok {
			if _, after, found := strings.Cut(line, ")  "); found {
				line = after
			}
			*out = append(*out, "left the installed pm service as it is (session start never restarts a running one): "+
				line)
			return strings.Join(*out, "\n"), nil
		}
		if err := attach(); err != nil {
			return "", err
		}
		return strings.Join(*out, "\n"), nil
	}
	c, err := config.Load(here)
	if err != nil {
		return "", err
	}
	port, err := service.PortFor(main, int(c.Port))
	if err != nil {
		return "", err
	}
	exe, err := service.Exe()
	if err != nil {
		return "", err
	}
	said, err := service.Install(main, port, exe, true) // a PATH of its own never restarts a running service
	if err != nil {
		return "", err
	}
	if said != "" {
		*out = append(*out, said)
	}
	if err := attach(); err != nil {
		return "", err
	}
	return strings.Join(*out, "\n"), nil
}
