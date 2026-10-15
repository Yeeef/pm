package setup

import (
	"errors"
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Uninstall is pm uninstall: pm's pieces out of this worktree and pm's setup out of the clone and the machine; the
// records branch and the remote's work store (refs/pm/work) stay, and so does pm in the
// bin dir, which other clones run. Refused before anything changes when the store holds uncommitted records or a piece
// cannot be removed without touching what is not pm's.
func Uninstall(here string, stdout io.Writer) error {
	u, err := planUninstall(here)
	if err != nil {
		return err
	}
	// Keep session start from restarting the service between the unsynced check and removal.
	unlock, err := service.LockInstall(u.main)
	if err != nil {
		return err
	}
	defer unlock()
	if err := u.prepare(); err != nil {
		return err
	}
	out, err := u.removeClone()
	if err != nil {
		return err
	}
	changed, err := u.removePieces()
	if err != nil {
		return err
	}
	return u.report(stdout, out, changed)
}

type uninstallPlan struct {
	c                  config.Config
	records, main, top string
	removals           []install.Removal
	trees, roots       []string
	workspaces         []store.Workspace // each code worktree's records workspace, removed with its branch
	codexPath          string
	codexNew           *string
	sparse             map[string]bool
}

func planUninstall(here string) (uninstallPlan, error) {
	c, err := config.Load(here)
	if err != nil {
		return uninstallPlan{}, err
	}
	records, err := store.PathOf(here)
	if err != nil {
		return uninstallPlan{}, err
	}
	main := store.MainOf(records)
	top, err := codeTop(here, records, "uninstall")
	if err != nil {
		return uninstallPlan{}, err
	}
	removals, err := install.Removals(top, install.SettingsOf(c))
	if err != nil {
		return uninstallPlan{}, err
	}
	all, err := install.Worktrees(main)
	if err != nil {
		return uninstallPlan{}, err
	}
	var trees []string
	var workspaces []store.Workspace
	for _, t := range all {
		if fsutil.Resolved(t) == fsutil.Resolved(records) || store.IsWorkspace(t) {
			continue
		}
		trees = append(trees, t)
		w, err := store.WorkspaceAt(t)
		if err != nil {
			return uninstallPlan{}, err
		}
		if w == nil {
			continue
		}
		if dirty, err := w.Dirty(); err != nil {
			return uninstallPlan{}, err
		} else if len(dirty) > 0 {
			return uninstallPlan{}, fmt.Errorf("the records workspace %s holds uncommitted records (%s); commit them "+
				"with pm commit there or revert them, then run pm uninstall again", w.Dir, strings.Join(dirty, ", "))
		}
		if pages, _, err := store.DraftPages(records, *w); err != nil {
			return uninstallPlan{}, err
		} else if len(pages) > 0 {
			return uninstallPlan{}, fmt.Errorf("the records workspace %s holds a draft commit of design pages not "+
				"published on the records branch; publish them, then run pm uninstall again", w.Dir)
		}
		workspaces = append(workspaces, *w)
	}
	var roots []string
	if fsutil.IsDir(records) {
		dirty, err := install.Git(records, "status", "--porcelain")
		if err != nil {
			return uninstallPlan{}, err
		}
		if dirty != "" {
			return uninstallPlan{}, fmt.Errorf("the store %s holds uncommitted records; commit them with pm commit or revert them, then run "+
				"pm uninstall again:\n%s", records, dirty)
		}
		if roots, err = install.CodexRoots(main); err != nil {
			return uninstallPlan{}, err
		}
	}
	return uninstallPlan{c: c, records: records, main: main, top: top, removals: removals, trees: trees, roots: roots,
		workspaces: workspaces}, nil
}

func (u *uninstallPlan) prepare() error {
	records := u.records
	main := u.main
	trees := u.trees
	roots := u.roots
	c := u.c
	// the work store goes with the clone's .pm/store; the remote's refs/pm/work keeps the project's items, so a store
	// holding what the remote lacks is refused, as uncommitted records are
	if why, err := uninstallUnsynced(main, c.Remote); err != nil {
		return err
	} else if why != "" {
		return fmt.Errorf("%s, and pm uninstall would delete it with the clone's .pm/store: push it with pm sync, or keep "+
			"its items with pm export > FILE and move it away, then run pm uninstall again", why)
	}
	u.codexPath = filepath.Join(install.CodexHome(), "config.toml")
	if len(roots) > 0 && isFile(u.codexPath) {
		n, err := install.RemoveCodexRoots(u.codexPath, roots)
		if err != nil {
			return err
		}
		u.codexNew = &n
	}
	u.sparse = map[string]bool{}
	for _, t := range trees { // refuse an unreadable settings file, and read the sparse checkout, before any change
		if _, err := install.ClaudeListsStore(t, records); err != nil {
			return err
		}
		patterns, err := install.Sparse(t)
		if err != nil {
			return err
		}
		u.sparse[t] = strings.Join(patterns, "\x00") == strings.Join(install.SparsePatterns, "\x00")
	}
	return nil
}

func (u uninstallPlan) removeClone() ([]string, error) {
	records := u.records
	main := u.main
	trees := u.trees
	codexPath := u.codexPath
	codexNew := u.codexNew
	sparse := u.sparse
	var out []string
	said, err := service.Uninstall(main)
	if err != nil {
		return nil, err
	}
	if said != "" {
		out = append(out, said)
	}
	for _, t := range trees {
		link := filepath.Join(t, "records")
		if fsutil.IsLink(link) && fsutil.Resolved(link) == fsutil.Resolved(records) {
			if err := os.Remove(link); err != nil {
				return nil, err
			}
			out = append(out, "removed the link "+link)
		}
		if sparse[t] {
			if err := install.Unsparse(t); err != nil {
				return nil, err
			}
			out = append(out, "turned off the sparse checkout of "+t)
		}
		said, err := install.RemoveClaude(t, records)
		if err != nil {
			return nil, err
		}
		if said != "" {
			out = append(out, said)
		}
	}
	for _, w := range u.workspaces {
		if _, err := install.Git(main, "worktree", "remove", w.Dir); err != nil {
			return nil, err
		}
		if _, err := install.Git(main, "branch", "-D", "--quiet", w.Branch()); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("removed the records workspace %s and its branch %s", w.Dir, w.Branch()))
	}
	if fsutil.IsDir(records) {
		if _, err := install.Git(main, "worktree", "remove", records); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("removed the store checkout %s; the %s branch stays", records, store.Branch))
	}
	if said, err := install.RemoveExclude(main); err != nil {
		return nil, err
	} else if said != "" {
		out = append(out, said)
	}
	if said, err := install.RetireHooksPath(main); err != nil {
		return nil, err
	} else if said != "" {
		out = append(out, said)
	}
	if codexNew != nil {
		if cur, err := os.ReadFile(codexPath); err == nil && string(cur) != *codexNew {
			if err := store.WriteAtomic(codexPath, *codexNew); err != nil {
				return nil, err
			}
			out = append(out, "removed this clone's writable_roots from "+codexPath)
		}
	}
	return out, nil
}

func (u uninstallPlan) removePieces() ([]string, error) {
	main := u.main
	top := u.top
	removals := u.removals
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
				return nil, err
			}
		} else if err := os.WriteFile(r.Path, []byte(*r.New), 0o644); err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(top, r.Path)
		if err != nil {
			return nil, err
		}
		note(filepath.ToSlash(rel))
	}
	bases := []string{fsutil.Resolved(top)}
	if fsutil.Resolved(main) != bases[0] {
		bases = append(bases, fsutil.Resolved(main))
	}
	for _, base := range bases {
		pmdir := filepath.Join(base, ".pm")
		if !fsutil.Exists(pmdir) {
			continue
		}
		if base == fsutil.Resolved(top) { // the work store goes with the clone's .pm/store
			if err := os.RemoveAll(pmdir); err != nil {
				return nil, err
			}
			note(".pm")
		} else { // another branch's checkout: only the clone's own state, never its tracked files
			for _, d := range []string{"store", "run"} {
				os.RemoveAll(filepath.Join(pmdir, d)) //nolint:errcheck // Other worktrees' clone-state removal remains best-effort.
			}
		}
	}
	return changed, nil
}

func (u uninstallPlan) report(stdout io.Writer, out, changed []string) error {
	top := u.top
	for _, rel := range changed {
		if fsutil.Exists(filepath.Join(top, rel)) {
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
	_, err := fmt.Fprintln(stdout, strings.Join(out, "\n"))
	return err
}

// uninstallUnsynced is install.WorkUnsynced, which reads the store through the clone's pm service. When that service
// does not answer (stopped, say), the check runs through a pm service run of pm uninstall's own, since only pm service
// run may host the store (the pm-go page, Store access: Startup). A clone with no work store has nothing to check.
func uninstallUnsynced(main, remote string) (string, error) {
	why, err := install.WorkUnsynced(main, remote)
	if !errors.Is(err, work.ErrNoService) {
		return why, err
	}
	if dir, _ := work.Locations(main); !fsutil.IsDir(dir) {
		return "", nil
	}
	stop, err := serveForCheck(main)
	if err != nil {
		return "", err
	}
	why, err = install.WorkUnsynced(main, remote)
	return why, errors.Join(err, stop())
}

// serveForCheck runs this pm as pm service run in the main checkout, on PORT=0 so it takes no site port, and waits
// up to service.RestartWait for the work store's socket; stop ends it (SIGTERM, as a supervisor stops the service)
// and waits for it to exit. Fails, with the run's output, when it exits or does not answer in time.
func serveForCheck(main string) (stop func() error, err error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	out, err := os.CreateTemp("", "pm-uninstall-service-*.log")
	if err != nil {
		return nil, err
	}
	defer os.Remove(out.Name()) //nolint:errcheck // Scratch-log removal remains best-effort; the open file stays readable.
	cmd := exec.Command(exe, "service", "run")
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = main, append(os.Environ(), "PORT=0"), out, out
	// in pm uninstall's process group, so a Ctrl-C of pm uninstall stops it too
	if err := cmd.Start(); err != nil {
		out.Close() //nolint:errcheck // Scratch-log cleanup must not replace the process-start error.
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	said := func() string {
		data, _ := os.ReadFile(out.Name()) //nolint:errcheck // Diagnostic output is best-effort; the process result remains the cause.
		return strings.TrimSpace(string(data))
	}
	end := func() error {
		defer out.Close()                   //nolint:errcheck // Scratch-log cleanup must not replace the child-stop result.
		cmd.Process.Signal(syscall.SIGTERM) //nolint:errcheck // The wait below determines whether the child actually stopped.
		select {
		case <-exited:
			return nil
		case <-time.After(service.RestartWait):
			cmd.Process.Kill() //nolint:errcheck // The wait determines the result even if the child exited before the signal.
			<-exited
			return fmt.Errorf("the pm service run that pm uninstall started for its check did not exit %d s after "+
				"SIGTERM, and was killed", int(service.RestartWait.Seconds()))
		}
	}
	sock := work.Sock(main)
	deadline := time.Now().Add(service.RestartWait)
	for !service.SockAnswers(sock) {
		select {
		case err := <-exited:
			out.Close() //nolint:errcheck // Scratch-log cleanup must not replace the child-exit error.
			return nil, fmt.Errorf("the clone's pm service does not answer, so pm uninstall ran pm service run for its "+
				"unsynced-work check, and it exited (%v) before it answered on %s:\n%s", err, sock, said())
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			text := said()
			end() //nolint:errcheck // Startup timeout remains the result; stopping the child is cleanup.
			return nil, fmt.Errorf("the clone's pm service does not answer, so pm uninstall ran pm service run for its "+
				"unsynced-work check, and it did not answer on %s within %d s:\n%s", sock,
				int(service.RestartWait.Seconds()), text)
		}
	}
	return end, nil
}
