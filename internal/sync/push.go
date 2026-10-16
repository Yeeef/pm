// Package sync is the push the pm service runs every Interval: the work store's sync, today's summary, the records
// branch and the draft refs (PushDrafts), so no session pushes any. Each run records the outcome of each step's last
// attempt in a state file in the clone's runtime directory (.pm/run/, never committed) and appends a line per step to
// a log beside it; pm where, pm service status and the served site read the state to flag a failed or overdue push.
//
// pm push (internal/cli/push.go) runs the same steps once.
package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/config"
	pmstore "github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

const (
	Interval = 600 * time.Second // between the service's runs
	Overdue  = 3 * Interval      // no successful push for this long is flagged
	Branch   = "records"
)

// Timeout is how long each step's command (git fetch, rebase, push) may take; a variable so tests can shorten it.
var Timeout = 120 * time.Second

// Steps are the run's steps in order: the work store's sync, the day summary, the records push, the draft refs' push
// and fetch. Stores are the steps that push a store; the summary is a step.
var (
	Steps  = []string{"work", "summary", "records", "drafts"}
	stores = map[string]bool{"work": true, "records": true, "drafts": true}
)

// Step is one step of a run: its name and what it does, returning ok and one line saying what it did or why it failed.
type Step struct {
	Name string
	Run  func() (bool, string)
}

// Files are the state file, the log and the lock file, in the clone's runtime directory.
func Files(main string) (state, log, lock string) {
	run := filepath.Join(main, config.Run)
	return filepath.Join(run, "push.json"), filepath.Join(run, "push.log"), filepath.Join(run, "push.lock")
}

// Now is the time a step's outcome is stamped with: UTC, whole seconds. A variable so tests can move it.
var Now = func() time.Time { return time.Now().UTC().Truncate(time.Second) }

// iso is Python's datetime.isoformat() of an aware UTC time: 2026-10-08T21:32:48+00:00.
func iso(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05+00:00") }

// State is the push state file: a step's outcome by its name, or only its first while it has not run (StartClocks).
type State map[string]any

// ReadState is the state file's contents; empty when there is none.
func ReadState(main string) (State, error) {
	path, _, _ := Files(main)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is not JSON: %v", path, err)
	}
	if s == nil {
		s = State{}
	}
	return s, nil
}

// WriteState replaces the state file whole.
func WriteState(main string, s State) error {
	path, _, _ := Files(main)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	tmp := strings.TrimSuffix(path, ".json") + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// StartClocks gives each step that has no outcome yet a first of now, the time its overdue clock starts from. The
// service runs it at each start, its install's included, so a step that a later pm adds is counted from the start of
// the first service that runs it, not from the clone's install; a step that already has one keeps it. A push under way
// records every step itself, so StartClocks leaves the state to it.
func StartClocks(main string) error {
	unlock, ok, err := lock(main)
	if err != nil || !ok {
		return err
	}
	defer unlock()
	s, err := ReadState(main)
	if err != nil {
		return err
	}
	changed := false
	for _, name := range Steps {
		if _, ok := s[name]; !ok {
			s[name] = map[string]string{"first": iso(Now())}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return WriteState(main, s)
}

// lock takes the push lock without waiting: ok is false while another run holds it.
func lock(main string) (unlock func(), ok bool, err error) {
	_, _, path := Files(main)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if cerr := syscall.Close(fd); cerr != nil {
			return nil, false, cerr
		}
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { syscall.Close(fd) }, true, nil //nolint:errcheck // the close releases the lock; a failed close leaves nothing to undo
}

type stepState struct {
	At      string  `json:"at"`
	OK      bool    `json:"ok"`
	Message string  `json:"message"`
	LastOK  *string `json:"last_ok"`
	First   string  `json:"first"`
}

func (s State) step(name string) (stepState, bool) {
	raw, ok := s[name]
	if !ok {
		return stepState{}, false
	}
	data, _ := json.Marshal(raw)
	var st stepState
	if json.Unmarshal(data, &st) != nil {
		return stepState{}, false
	}
	return st, true
}

// Run runs cmd in dir with Timeout: ok and its output (or why it failed), on one line. The command runs in its own
// process group and a timeout kills the whole group: git starts ssh or git-remote-https children that would outlive a
// kill of the direct child.
func Run(dir string, cmd ...string) (bool, string) {
	path, err := exec.LookPath(cmd[0])
	if err != nil {
		return false, cmd[0] + " is not installed"
	}
	c := exec.Command(path, cmd[1:]...)
	c.Args[0] = cmd[0]
	c.Dir = dir
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return false, fmt.Sprintf("%s did not run: %v", strings.Join(cmd, " "), err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err = <-done:
	case <-time.After(Timeout):
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		<-done
		return false, fmt.Sprintf("%s timed out after %ds", strings.Join(cmd, " "), int(Timeout.Seconds()))
	}
	said := strings.Join(strings.Fields(out.String()), " ")
	if err != nil {
		return false, fmt.Sprintf("%s failed: %s", strings.Join(cmd, " "), said)
	}
	return true, said
}

// flock takes an exclusive lock on path (a file or the store's directory), waiting for it; the returned func
// releases it.
func flock(path string, flags int) (func(), error) {
	fd, err := syscall.Open(path, flags, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return func() { syscall.Close(fd) }, nil
}

// PushRecords brings the store and <remote>/records together, after a fetch: under the store lock, which every pm
// write takes, it reads how far each is ahead and checks HEAD is on the records branch, then fast-forwards a store only
// behind (fastForward) or rebases one ahead and behind onto the remote; then it pushes a store ahead, outside the lock.
// A rebase that stops is aborted, and an abort that fails or leaves HEAD off the records branch is reported as needing
// repair by hand. closed reads which sprints are closed, for the follow after a fast-forward.
func PushRecords(store, remote string, closed func() (pmstore.SprintClosed, error)) (bool, string) {
	if ok, said := Run(store, "git", "fetch", "--quiet", remote, Branch); !ok {
		return false, said
	}
	ahead, behind, said := counts(store, remote)
	if said != "" {
		return false, said
	}
	if ahead == 0 && behind == 0 {
		return true, "up to date with " + remote + "/" + Branch
	}
	var isClosed pmstore.SprintClosed
	if behind > 0 && ahead == 0 {
		var err error
		if isClosed, err = closed(); err != nil { // before the store lock: the work store is never read under it
			return false, "reading the work store's sprints failed: " + err.Error()
		}
	}
	unlock, err := flock(store, syscall.O_RDONLY)
	if err != nil {
		return false, fmt.Sprintf("could not lock %s: %v", store, err)
	}
	ok, said := func() (bool, string) {
		defer unlock()
		if on, branch := Run(store, "git", "symbolic-ref", "--short", "HEAD"); !on || branch != Branch {
			return false, fmt.Sprintf("the store %s is not on the %s branch (HEAD: %s); not synced: git -C %s switch %s",
				store, Branch, branch, store, Branch)
		}
		if ahead, behind, said = counts(store, remote); said != "" { // again, under the lock: a write may have moved it
			return false, said
		}
		switch {
		case behind == 0:
			return true, ""
		case ahead == 0 && isClosed == nil: // a write made it only behind since the first read: next run
			return true, fmt.Sprintf("%d behind %s/%s, fast-forwarded on the next run", behind, remote, Branch)
		case ahead == 0:
			return fastForward(store, remote, behind, isClosed)
		}
		return rebase(store, remote)
	}()
	if !ok || ahead == 0 {
		return ok, said
	}
	if ok, said := Run(store, "git", "push", "--quiet", remote, "HEAD:"+Branch); !ok {
		return false, said
	}
	pushed := fmt.Sprintf("pushed %d commit(s)", ahead)
	if behind > 0 {
		pushed += fmt.Sprintf(" after rebasing onto %d new on %s/%s", behind, remote, Branch)
	}
	return true, pushed
}

// counts is how many commits the store's HEAD is ahead of and behind <remote>/records, or why it cannot say.
func counts(store, remote string) (ahead, behind int, why string) {
	ok, out := Run(store, "git", "rev-list", "--left-right", "--count", "HEAD..."+remote+"/"+Branch)
	if !ok {
		return 0, 0, out
	}
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, "git rev-list printed " + out
	}
	ahead, aerr := strconv.Atoi(f[0])
	behind, berr := strconv.Atoi(f[1])
	if aerr != nil || berr != nil {
		return 0, 0, "git rev-list printed " + out
	}
	return ahead, behind, ""
}

// rebase rebases the store, ahead and behind, onto <remote>/records, under the store lock the caller holds; skipped
// while tracked records have uncommitted edits.
func rebase(store, remote string) (bool, string) {
	if clean, _ := Run(store, "git", "diff", "--quiet", "HEAD"); !clean {
		return false, remote + "/" + Branch + " moved and the store has uncommitted changes to tracked records; " +
			"not rebased, retried on the next run"
	}
	ok, said := Run(store, "git", "rebase", "--quiet", remote+"/"+Branch)
	if ok {
		return true, ""
	}
	aborted, why := Run(store, "git", "rebase", "--abort")
	on, branch := Run(store, "git", "symbolic-ref", "--short", "HEAD")
	if !aborted || !on || branch != Branch {
		if why == "" {
			why = "ok"
		}
		if !on || branch == "" {
			branch = "detached"
		}
		return false, fmt.Sprintf("rebase onto %s/%s stopped and its abort did not restore the store, which "+
			"needs manual repair: git -C %s status, then git -C %s rebase --abort or git -C %s switch %s "+
			"(abort: %s; HEAD: %s); the rebase: %s", remote, Branch, store, store, store, Branch, why, branch, said)
	}
	return false, fmt.Sprintf("rebase onto %s/%s stopped, aborted and the store left as it was: %s", remote,
		Branch, said)
}

// DialClosed reads which sprints are closed through a connection to the clone's pm service, as pm push does.
func DialClosed(main string) func() (pmstore.SprintClosed, error) {
	return func() (pmstore.SprintClosed, error) {
		d, err := work.Dial(main)
		if err != nil {
			return nil, err
		}
		items, err := d.Items()
		return pmstore.ClosedSprints(items), errors.Join(err, d.Shutdown())
	}
}

// fastForward brings a store only behind <remote>/records up to it, under the store lock the caller holds: git merge
// --ff-only, never a merge commit and never an autostash (merge.autoStash would stash into the stash every worktree of
// the clone shares). It refuses, flagged and retried on the next run, while tracked records have uncommitted edits, as
// the rebase does, and while a path the remote changes exists on disk untracked or ignored, which the fast-forward
// would overwrite. Then each records workspace with no uncommitted file follows (store.FollowClean), so a draft drops
// the pages another clone's close published.
func fastForward(store, remote string, behind int, closed pmstore.SprintClosed) (bool, string) {
	refused := fmt.Sprintf("%d behind %s/%s and not fast-forwarded, so nothing was overwritten", behind, remote, Branch)
	if clean, _ := Run(store, "git", "diff", "--quiet", "HEAD"); !clean {
		return false, refused + ": the store has uncommitted changes to tracked records; commit or revert them, and " +
			"the next run fast-forwards"
	}
	ok, changed := Run(store, "git", "diff", "--name-only", "HEAD", remote+"/"+Branch)
	if !ok {
		return false, changed
	}
	var onDisk []string
	for _, p := range strings.Fields(changed) {
		_, err := os.Lstat(filepath.Join(store, filepath.FromSlash(p)))
		if tracked, _ := Run(store, "git", "cat-file", "-e", "HEAD:"+p); err == nil && !tracked {
			onDisk = append(onDisk, p)
		}
	}
	if onDisk != nil {
		return false, refused + ": the remote adds " + strings.Join(onDisk, ", ") + ", which the store holds untracked " +
			"or ignored; move them away, and the next run fast-forwards"
	}
	if ok, said := Run(store, "git", "-c", "merge.autoStash=false", "merge", "--ff-only", "--no-autostash", "--quiet",
		remote+"/"+Branch); !ok {
		return false, refused + ": " + said
	}
	said := fmt.Sprintf("fast-forwarded %d commit(s) from %s/%s", behind, remote, Branch)
	if lines := pmstore.FollowClean(store, closed); len(lines) > 0 {
		said += "; " + strings.Join(lines, "; ")
	}
	return true, said
}

// PushDrafts pushes this clone's draft refs (store.PushDrafts), then fetches the other clones' (store.FetchDrafts):
// the record-drafts page, Draft refs. A lease that failed fails the step, which flags it; nothing is forced.
func PushDrafts(records, remote string) (bool, string) {
	id, err := pmstore.CloneID(records)
	if err != nil {
		return false, err.Error()
	}
	said, err := pmstore.PushDrafts(records, remote, id)
	if err != nil {
		said = err.Error()
	}
	if ferr := pmstore.FetchDrafts(records, remote, id); ferr != nil {
		return false, said + "; fetching the other clones' drafts failed: " + ferr.Error()
	}
	if err != nil {
		return false, said
	}
	return true, said + "; fetched the other clones'"
}

// Push is one run, under a lock no second run waits for: each step in order, its outcome recorded; a failed step does
// not stop the next. The code is non-zero when a step of Steps failed.
func Push(main string, steps []Step) (int, string, error) {
	_, log, _ := Files(main)
	unlock, ok, err := lock(main)
	if err != nil {
		return 1, "", err
	}
	if !ok {
		return 0, "another pm push holds the lock; skipped", nil
	}
	defer unlock()
	state, err := ReadState(main)
	if err != nil {
		return 1, "", err
	}
	var lines []string
	for _, s := range steps {
		at := iso(Now())
		ok, said := runStep(s)
		prev, _ := state.step(s.Name)
		first := prev.First
		if first == "" {
			first = at
		}
		lastOK := prev.LastOK
		if ok {
			lastOK = &at
		}
		state[s.Name] = stepState{At: at, OK: ok, Message: said, LastOK: lastOK, First: first}
		word := "error"
		if ok {
			word = "ok"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s: %s", at, s.Name, word, said))
	}
	if err := WriteState(main, state); err != nil {
		return 1, "", err
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 1, "", err
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		return 1, "", err
	}
	code := 0
	for _, n := range Steps {
		if st, ok := state.step(n); !ok || !st.OK {
			code = 1
		}
	}
	return code, strings.Join(lines, "\n"), nil
}

// runStep runs one step; a panic is that step's recorded failure, as an exception is in Python's push, and does not
// stop the next step.
func runStep(s Step) (ok bool, said string) {
	defer func() {
		if r := recover(); r != nil {
			ok, said = false, fmt.Sprintf("panic: %v", r)
		}
	}()
	return s.Run()
}

// unpushed is the records commits not on <remote>/records as of the last fetch; -1 without it.
func unpushed(store, remote string) int {
	out, err := exec.Command("git", "-C", store, "rev-list", "--count", remote+"/"+Branch+"..HEAD").Output()
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return n
}

// Flags is a line per step whose last run failed or whose last success is older than Overdue, counted from the
// step's first (StartClocks, or its first run) when it never succeeded. store "" skips the count of unpushed records commits.
func Flags(main, store, remote string) ([]string, error) {
	state, err := ReadState(main)
	if err != nil {
		return nil, err
	}
	_, log, _ := Files(main)
	var out []string
	for _, name := range Steps {
		kind := "step"
		if stores[name] {
			kind = "push"
		}
		s, ok := state.step(name)
		if !ok { // no service that runs it has started yet: its clock has not started
			continue
		}
		var line string
		if s.At != "" && !s.OK {
			line = fmt.Sprintf("%s %s failed at %s: %s", name, kind, s.At, s.Message)
		} else {
			since := s.First
			if s.LastOK != nil {
				since = *s.LastOK
			}
			t, err := time.Parse(time.RFC3339, since)
			if err != nil {
				return nil, fmt.Errorf("%s: %s of %s is not a time: %q", log, name, kind, since)
			}
			age := Now().Sub(t)
			if age <= Overdue {
				continue
			}
			what := "no successful " + kind + " since"
			if s.LastOK != nil {
				what = "last successful " + kind
			}
			line = fmt.Sprintf("%s %s overdue: %s %s, %d min ago (the pm service pushes every %d min)", name, kind,
				what, since, int(age.Minutes()), int(Interval.Minutes()))
		}
		if name == "records" && store != "" {
			if n := unpushed(store, remote); n > 0 {
				line += fmt.Sprintf("; %d records commit(s) not on %s/%s", n, remote, Branch)
			}
		}
		out = append(out, line+"; log "+log)
	}
	return out, nil
}

// Banner is the site's warning over the home and project pages; empty when every push is current.
func Banner(main, store, remote string) (string, error) {
	lines, err := Flags(main, store, remote)
	if err != nil || len(lines) == 0 {
		return "", err
	}
	var items strings.Builder
	for _, l := range lines {
		items.WriteString("<li>" + html.EscapeString(l) + "</li>")
	}
	return `<div class="note draft push"><p><strong>Push</strong> — needs attention:</p><ul>` + items.String() +
		`</ul></div>`, nil
}

// Describe is pm where's lines: the last attempt of each step.
func Describe(main string) ([]string, error) {
	state, err := ReadState(main)
	if err != nil {
		return nil, err
	}
	_, log, _ := Files(main)
	var out []string
	for _, name := range Steps {
		if s, ok := state.step(name); ok && s.At != "" { // a step that has not run has only its first
			word := "error"
			if s.OK {
				word = "ok"
			}
			out = append(out, fmt.Sprintf("push      %s %s at %s: %s", name, word, s.At, s.Message))
		}
	}
	if len(out) == 0 {
		return []string{"push      " + log + "  no push recorded yet"}, nil
	}
	return append(out, "push      log "+log), nil
}
