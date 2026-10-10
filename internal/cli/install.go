package cli

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
)

// pm init, pm doctor, pm upgrade, pm uninstall and the git hooks. The pieces and
// the clone's setup are internal/install's.

// latest is how to install the newest Go pm release, whose launcher launches any pin: its install.sh (install.sh).
const latest = `curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh | sh`

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// codeTop is the code worktree a repo-level command acts on; refused inside the store.
func codeTop(here, records, what string) (string, error) {
	top, err := install.Git(here, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if resolvedPath(top) == resolvedPath(records) {
		return "", refuse("%s is the records store; run pm %s from a code worktree", top, what)
	}
	return top, nil
}

func settingsOf(c config.Config) install.Settings {
	return install.Settings{Remote: c.Remote, MainBranch: c.MainBranch, Port: int(c.Port), SiteURL: c.SiteURL}
}

// checkSiteURL refuses a --site-url that is not an http(s) base URL.
func checkSiteURL(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" || p.Path != "" ||
		p.RawQuery != "" || p.Fragment != "" {
		return refuse("--site-url %s is not an http(s) base URL like https://pm.example.com", pyjson.StrRepr(u))
	}
	return nil
}

func siteURLArg(v string) string { return strings.TrimRight(config.PyStrip(v), "/") }

var siteLine = regexp.MustCompile(`(?m)^site_url\s*=.*\n?`)

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
	b, err := os.ReadFile(c.Path)
	if err != nil {
		return "", err
	}
	text, line := string(b), ""
	if u != "" {
		line = `site_url = "` + u + "\"\n"
	}
	if loc := siteLine.FindStringIndex(text); loc != nil {
		text = text[:loc[0]] + line + text[loc[1]:]
	} else if u != "" {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += line
	}
	if err := os.WriteFile(c.Path, []byte(text), 0o644); err != nil {
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

// initSettings is a new repo's settings: the remote origin, which must exist, and its default branch (origin/HEAD),
// else the branch checked out; port is the site port.
func initSettings(top, siteArg string, given bool, port int) (install.Settings, error) {
	remote := "origin"
	if install.RemoteURL(top, remote) == "" {
		return install.Settings{}, refuse("this repo has no remote %s; pm keeps the records branch and the work store "+
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
	return install.Settings{Remote: remote, MainBranch: branch, Port: port, SiteURL: u}, nil
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
		return nil, nil, refuse("git status failed in %s: %s", top, config.PyStrip(res.Stderr))
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

// cmdInit is pm init: the repo's half only when the worktree has no .pm/config.toml yet (a first install; pm doctor
// reports and pm upgrade rewrites a changed piece after that), then the clone's and this worktree's half every time,
// pm on the machine, and the pm service last. Session start runs it, so every worktree an agent works in is ready. The
// repo's pieces are written, never committed: the output names the commit to make, also when a later step fails.
func cmdInit(p *Parsed, here string, stdout io.Writer) error {
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(records)
	top, err := install.Git(here, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if resolvedPath(top) == resolvedPath(records) {
		return refuse("%s is the records store; run pm init from a code worktree", top)
	}
	if err := install.CheckHooksPath(top, main); err != nil {
		return err
	}
	sessionStart := p.Get("session_start") == "true"
	siteArg, siteGiven := p.Get("site_url"), len(p.values["site_url"]) > 0
	fresh := !isFile(filepath.Join(top, config.Rel))
	if resolvedPath(top) != resolvedPath(main) {
		if fresh { // pm's files come from the main branch; a linked worktree without them is on a branch from before
			return refuse("this worktree's branch has no %s: it was cut before pm was installed in this repo, and pm "+
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
			why = fmt.Sprintf("the main checkout %s pins pm %s, and the pm service and the installed pm follow it; run "+
				"pm init with pm %s, or move main's pin with pm upgrade there first", main, pin, pin)
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
			return refuse("%s. pm init set up this worktree and left the installed pm and the service alone:\n%s", why, done)
		}
	}
	var s install.Settings
	if fresh { // the site port: $PORT, else the clone's unit's, else the first free one no pm unit here names
		port, err := service.PortFor(main, service.FreePort())
		if err != nil {
			return err
		}
		if s, err = initSettings(top, siteArg, siteGiven, port); err != nil {
			return err
		}
	} else {
		c, err := config.Load(here)
		if err != nil {
			return err
		}
		s = settingsOf(c)
		if siteGiven && config.PyStrip(siteArg) != "" { // "/" is refused here, as no base URL
			if err := checkSiteURL(siteURLArg(siteArg)); err != nil { // before anything is written
				return err
			}
		}
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
		if err := service.CheckPort(main, port); err != nil { // refused before anything is written
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
	done, err := initSteps(here, top, main, s, fresh, sessionStart, siteArg, siteGiven, &out, &written)
	if err != nil {
		hint, herr := commitHint(top, before, written)
		if herr != nil || hint == "" {
			return errors.Join(err, herr)
		}
		return refuse("%s", strings.Join(append(append([]string{err.Error()}, out...), hint,
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
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

// initSteps is pm init's writes, in order; out and written say how far it got when one fails. The repo's half (pm's
// pieces, the records branch) runs only when fresh.
func initSteps(here, top, main string, s install.Settings, fresh, sessionStart bool, siteArg string, siteGiven bool,
	out, written *[]string) (string, error) {
	said, err := install.InstallBinary() // the service's unit and the hooks run the bin-dir pm, not this one
	if err != nil {
		return "", err
	}
	if said != "" {
		*out = append(*out, said)
	}
	if fresh {
		planned, err := install.Plan(top, s)
		if err != nil {
			return "", err
		}
		res, err := proc.Run([]string{"git", "rev-parse", "--verify", "--quiet", "refs/heads/" + store.Branch},
			proc.Options{Cwd: &top})
		if err != nil {
			return "", err
		}
		if res.Code != 0 {
			has, err := install.RemoteHasBranch(top, s.Remote, store.Branch)
			if err != nil {
				return "", err
			}
			if !has {
				said, err := install.CreateRecordsBranch(top, s.Remote, store.Branch)
				if err != nil {
					return "", err
				}
				*out = append(*out, said)
			}
		}
		w, err := install.Write(planned)
		*written = append(*written, w...)
		if err != nil {
			return "", err
		}
		*out = append(*out, install.Said(planned)...)
	} else if siteGiven {
		c, err := config.Load(here)
		if err != nil {
			return "", err
		}
		said, err := setupSiteURL(siteArg, c, main)
		if err != nil {
			return "", err
		}
		if said != "" {
			*out = append(*out, said)
		}
	}
	done, err := install.SetupClone(here, s.Remote)
	if err != nil {
		return "", err
	}
	*out = append(*out, done)
	// the work store is the service's: the service comes up first, then attaches the store (CALL pm_setup())
	attach := func() error {
		said, err := install.SetupWork(main)
		*out = append(*out, said...)
		return err
	}
	if sessionStart && service.Installed(main) {
		// A service that does not answer is started, once for parallel session starts (the clone's install lock); a
		// running one, a stale one included, is left as it is and reported, since a restart from a session start
		// would restart it under every other session too.
		said, err := service.StartIfDown(main)
		if err != nil {
			return "", err
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
	said, err = service.Install(main, port, exe)
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

// cmdDoctor is pm doctor: every managed piece against what this pm writes, and the clone and worktree against what pm
// init makes; non-zero on any difference.
func cmdDoctor(here string, stdout io.Writer) error {
	c, err := config.Load(here)
	if err != nil {
		return err
	}
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(records)
	top, err := codeTop(here, records, "doctor")
	if err != nil {
		return err
	}
	fix := "pm upgrade --to " + buildinfo.Version // the pin: a bare pm upgrade runs the launcher's version
	repo, err := install.Drift(top, settingsOf(c))
	if err != nil {
		return err
	}
	var diffs []string
	for _, d := range repo {
		diffs = append(diffs, fmt.Sprintf("repo: %s; run %s to rewrite it", d, fix))
	}
	port, err := service.PortFor(main, int(c.Port))
	if err != nil {
		return err
	}
	setup, err := install.DoctorSetup(top, main, records, c.Remote, port)
	if err != nil {
		return err
	}
	diffs = append(diffs, setup...)
	found, err := install.LegacyRepo(top)
	var ie *install.Error
	if errors.As(err, &ie) { // a file pm cannot read: no legacy piece is known, the file needs a hand fix
		diffs = append(diffs, "repo: cannot look for the pre-package harness's pieces: "+ie.Msg)
	} else if err != nil {
		return err
	}
	for _, d := range append(found, install.LegacyClone(main)...) {
		diffs = append(diffs, "legacy: "+d+"; "+install.LegacyFix())
	}
	if len(diffs) > 0 {
		fmt.Fprintln(stdout, strings.Join(diffs, "\n"))
		return &exitCode{1}
	}
	_, err = fmt.Fprintf(stdout, "pm %s (%s): every managed piece and the clone's setup match what pm init makes\n",
		buildinfo.Version, launch.How())
	return err
}

// cmdUpgrade is pm upgrade: move the pin to the running pm, rewrite every managed piece as it writes them, take out
// Beads' pieces and the retired ones and point core.hooksPath at pm's hooks; commits nothing, and names the untracking
// of a records/ copy the branch still tracks. Without --to it never moves a pin down.
func cmdUpgrade(p *Parsed, here string, stdout io.Writer) error {
	to := p.Get("to")
	if to == "" {
		to = buildinfo.Version
	}
	if to != buildinfo.Version {
		return refuse("pm upgrade --to %s must run pm %s, but pm %s is running; run it as the pm on PATH, which "+
			"launches pm %s: install the latest with %s", to, to, buildinfo.Version, to, latest)
	}
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	top, err := codeTop(here, records, "upgrade")
	if err != nil {
		return err
	}
	c, err := config.Read(top)
	if err != nil {
		return err
	}
	have, okHave := launch.Key(c.Version)
	run, okRun := launch.Key(buildinfo.Version)
	if p.Get("to") == "" && okHave && okRun && launch.Less(run, have) {
		return refuse("this repo pins pm %s, newer than the running pm %s, and pm upgrade moves a pin down only when "+
			"--to names the version; run pm upgrade --to %s to rewrite pm's pieces at the pin, or install the latest "+
			"pm with %s, then pm upgrade", c.Version, buildinfo.Version, c.Version, latest)
	}
	main := store.MainOf(records)
	if err := install.RefuseLegacy(top, main, true); err != nil {
		return err
	}
	if err := install.CheckHooksPath(top, main); err != nil {
		return err
	}
	planned, err := install.Rewrite(top, settingsOf(c))
	if err != nil {
		return err
	}
	written, err := install.Write(planned)
	if err != nil {
		return err
	}
	// pm's hook files live in .pm/hooks: a clone Python pm set up points core.hooksPath at Beads' .beads/hooks
	hooksPath, err := install.SetupHooksPath(main)
	if err != nil {
		return err
	}
	// the main branch's records/ copy an earlier pm kept: no branch tracks records/ now
	tracks, err := install.TracksRecords(top)
	if err != nil {
		return err
	}
	if len(written) == 0 && !tracks {
		if hooksPath != "" {
			fmt.Fprintln(stdout, hooksPath)
		}
		_, err := fmt.Fprintf(stdout, "pm %s: every managed piece is current; nothing to commit\n", buildinfo.Version)
		return err
	}
	branch, err := install.Git(top, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	moved := "pin stays " + buildinfo.Version
	if c.Version != buildinfo.Version {
		moved = fmt.Sprintf("moved the pin from %s to %s", c.Version, buildinfo.Version)
	}
	lines := append([]string{moved}, install.Said(planned)...)
	if hooksPath != "" {
		lines = append(lines, hooksPath)
	}
	var steps []string
	if len(written) > 0 {
		steps = append(steps, "git add -- "+strings.Join(written, " "))
	}
	if tracks {
		lines = append(lines, "this branch tracks records/, a copy no branch keeps now; the commit below untracks it")
		steps = append(steps, "git rm -r -q --cached --sparse --ignore-unmatch records")
	}
	lines = append(lines, fmt.Sprintf("pm commits nothing on %s; commit pm's files there: %s && git commit -m \"Upgrade "+
		"pm to %s\"", branch, strings.Join(steps, " && "), buildinfo.Version))
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

// cmdUninstall is pm uninstall: pm's pieces out of this worktree and pm's setup out of the clone and the machine; the
// records branch and the remote's work store (refs/pm/work) stay, and so does pm in the
// bin dir, which other clones run. Refused before anything changes when the store holds uncommitted records or a piece
// cannot be removed without touching what is not pm's.
func cmdUninstall(here string, stdout io.Writer) error {
	c, err := config.Load(here)
	if err != nil {
		return err
	}
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(records)
	top, err := codeTop(here, records, "uninstall")
	if err != nil {
		return err
	}
	removals, err := install.Removals(top, settingsOf(c))
	if err != nil {
		return err
	}
	all, err := install.Worktrees(main)
	if err != nil {
		return err
	}
	var trees []string
	for _, t := range all {
		if resolvedPath(t) != resolvedPath(records) {
			trees = append(trees, t)
		}
	}
	var roots []string
	if isDir(records) {
		dirty, err := install.Git(records, "status", "--porcelain")
		if err != nil {
			return err
		}
		if dirty != "" {
			return refuse("the store %s holds uncommitted records; commit them with pm commit or revert them, then run "+
				"pm uninstall again:\n%s", records, dirty)
		}
		if roots, err = install.CodexRoots(main); err != nil {
			return err
		}
	}
	// the work store goes with the clone's .pm/store; the remote's refs/pm/work keeps the project's items, so a store
	// holding what the remote lacks is refused, as uncommitted records are
	if why, err := install.WorkUnsynced(main, c.Remote); err != nil {
		return err
	} else if why != "" {
		return refuse("%s, and pm uninstall would delete it with the clone's .pm/store: push it with pm sync, or keep "+
			"its items with pm export > FILE and move it away, then run pm uninstall again", why)
	}
	codexPath := filepath.Join(install.CodexHome(), "config.toml")
	var codexNew *string
	if len(roots) > 0 && isFile(codexPath) {
		n, err := install.RemoveCodexRoots(codexPath, roots)
		if err != nil {
			return err
		}
		codexNew = &n
	}
	sparse := map[string]bool{}
	for _, t := range trees { // refuse an unreadable settings file, and read the sparse checkout, before any change
		if _, err := install.ClaudeListsStore(t, records); err != nil {
			return err
		}
		patterns, err := install.Sparse(t)
		if err != nil {
			return err
		}
		sparse[t] = strings.Join(patterns, "\x00") == strings.Join(install.SparsePatterns, "\x00")
	}
	var out []string
	said, err := service.Uninstall(main)
	if err != nil {
		return err
	}
	if said != "" {
		out = append(out, said)
	}
	for _, t := range trees {
		link := filepath.Join(t, "records")
		if whereIsLink(link) && resolvedPath(link) == resolvedPath(records) {
			if err := os.Remove(link); err != nil {
				return err
			}
			out = append(out, "removed the link "+link)
		}
		if sparse[t] {
			if err := install.Unsparse(t); err != nil {
				return err
			}
			out = append(out, "turned off the sparse checkout of "+t)
		}
		said, err := install.RemoveClaude(t, records)
		if err != nil {
			return err
		}
		if said != "" {
			out = append(out, said)
		}
	}
	if isDir(records) {
		if _, err := install.Git(main, "worktree", "remove", records); err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("removed the store checkout %s; the %s branch stays", records, store.Branch))
	}
	if said, err := install.RemoveExclude(main); err != nil {
		return err
	} else if said != "" {
		out = append(out, said)
	}
	if codexNew != nil {
		if cur, err := os.ReadFile(codexPath); err == nil && string(cur) != *codexNew {
			if err := store.WriteAtomic(codexPath, *codexNew); err != nil {
				return err
			}
			out = append(out, "removed this clone's writable_roots from "+codexPath)
		}
	}
	var changed []string
	seen := map[string]bool{}
	note := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			changed = append(changed, rel)
		}
	}
	for _, r := range removals {
		if r.New == nil {
			if err := os.Remove(r.Path); err != nil {
				return err
			}
		} else if err := os.WriteFile(r.Path, []byte(*r.New), 0o644); err != nil {
			return err
		}
		rel, _ := filepath.Rel(top, r.Path)
		note(filepath.ToSlash(rel))
	}
	bases := []string{resolvedPath(top)}
	if resolvedPath(main) != bases[0] {
		bases = append(bases, resolvedPath(main))
	}
	for _, base := range bases {
		pmdir := filepath.Join(base, ".pm")
		if !whereExists(pmdir) {
			continue
		}
		if base == resolvedPath(top) { // the work store goes with the clone's .pm/store
			if err := os.RemoveAll(pmdir); err != nil {
				return err
			}
			note(".pm")
		} else { // another branch's checkout: only the clone's own state, never its tracked files
			for _, d := range []string{"store", "run"} {
				os.RemoveAll(filepath.Join(pmdir, d))
			}
		}
	}
	for _, rel := range changed {
		if whereExists(filepath.Join(top, rel)) {
			out = append(out, "removed pm's part of "+rel)
		} else {
			out = append(out, "removed "+rel)
		}
	}
	if len(changed) > 0 {
		branch, err := install.Git(top, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("pm commits nothing on %s; commit the removal there: git add -A -- %s && git "+
			"commit -m \"Uninstall pm\"", branch, strings.Join(changed, " ")))
	}
	if len(out) == 0 {
		out = []string{"pm is not installed here; nothing to remove"}
	}
	_, err = fmt.Fprintln(stdout, strings.Join(out, "\n"))
	return err
}

// hookGitPostCheckout is pm hook git-post-checkout: in a new worktree (previous HEAD all zeros), pm init's clone and
// worktree half, all but the pm service. The store's own checkout, which that half itself makes, is skipped. A failure
// is printed and exits 1, which git reports without undoing the checkout.
func hookGitPostCheckout(p *Parsed, here string, stdout, stderr io.Writer) error {
	args := p.values["git_args"]
	if len(args) == 0 || strings.Trim(args[0], "0") != "" || args[0] == "" {
		return nil
	}
	err := func() error {
		records, err := store.PathOf(here)
		if err != nil {
			return err
		}
		top, err := install.Git(here, "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		if resolvedPath(top) == resolvedPath(records) {
			return nil
		}
		c, err := config.Load(here)
		if err != nil {
			return err
		}
		done, err := install.SetupClone(here, c.Remote)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, done)
		return err
	}()
	if err != nil {
		fmt.Fprintf(stderr, "pm hook git-post-checkout: %s\n", err)
		return &exitCode{1}
	}
	return nil
}

// hookGitPreCommit is pm hook git-pre-commit, retired with the main branch's records/ copy, whose guard it was: it does
// nothing. An earlier pm's section in the main checkout's .pm/hooks/pre-commit runs it in every worktree of the clone
// until pm upgrade's removal of that section reaches the main checkout, so a worktree already pinned to this pm must
// still commit.
func hookGitPreCommit() error { return nil }
