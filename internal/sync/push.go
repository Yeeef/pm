// Package sync is the push the pm service runs every Interval: the work store's sync, today's summary and the records
// branch, so no session pushes either. Each run records the outcome of each step's last attempt in a state file in the
// clone's runtime directory (.pm/run/, never committed) and appends a line per step to a log beside it; pm where, pm
// service status and the served site read the state to flag a failed or overdue push. Python source: push.py.
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

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
)

const (
	Interval = 600 * time.Second // between the service's runs
	Overdue  = 3 * Interval      // no successful push for this long is flagged
	Branch   = "records"
)

// Timeout is how long each step's command (git fetch, rebase, push) may take; a variable so tests can shorten it.
var Timeout = 120 * time.Second

// Steps are the run's steps in order: the work store's sync, the day summary, the records push. Stores are the steps
// that push a store; the summary is a step.
var (
	Steps  = []string{"work", "summary", "records"}
	stores = map[string]bool{"work": true, "records": true}
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

// State is the push state file: a step's outcome by its name, and installed_at, when the service was installed.
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

// MarkInstalled stamps installed_at once, so a push that never succeeds is flagged overdue counted from the install.
func MarkInstalled(main string) error {
	s, err := ReadState(main)
	if err != nil {
		return err
	}
	if _, ok := s["installed_at"]; ok {
		return nil
	}
	s["installed_at"] = iso(Now())
	return WriteState(main, s)
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

// PushRecords pushes the store when it is ahead of <remote>/records, rebasing onto it first when the remote moved.
// The rebase holds the store lock every pm write takes; a rebase that stops is aborted, and an abort that fails or
// leaves HEAD off the records branch is reported as needing repair by hand.
func PushRecords(store, remote string) (bool, string) {
	if ok, said := Run(store, "git", "fetch", "--quiet", remote, Branch); !ok {
		return false, said
	}
	ok, counts := Run(store, "git", "rev-list", "--left-right", "--count", "HEAD..."+remote+"/"+Branch)
	if !ok {
		return false, counts
	}
	f := strings.Fields(counts)
	if len(f) != 2 {
		return false, "git rev-list printed " + counts
	}
	ahead, _ := strconv.Atoi(f[0])
	behind, _ := strconv.Atoi(f[1])
	if ahead == 0 {
		said := "up to date with " + remote + "/" + Branch
		if behind > 0 {
			said += fmt.Sprintf(" (%d behind; not pulled)", behind)
		}
		return true, said
	}
	if behind > 0 {
		unlock, err := flock(store, syscall.O_RDONLY)
		if err != nil {
			return false, fmt.Sprintf("could not lock %s: %v", store, err)
		}
		ok, said := func() (bool, string) {
			defer unlock()
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
		}()
		if !ok {
			return false, said
		}
	}
	if ok, said := Run(store, "git", "push", "--quiet", remote, "HEAD:"+Branch); !ok {
		return false, said
	}
	said := fmt.Sprintf("pushed %d commit(s)", ahead)
	if behind > 0 {
		said += fmt.Sprintf(" after rebasing onto %d new on %s/%s", behind, remote, Branch)
	}
	return true, said
}

// Push is one run, under a lock no second run waits for: each step in order, its outcome recorded; a failed step does
// not stop the next. The code is non-zero when a step of Steps failed.
func Push(main string, steps []Step) (int, string, error) {
	_, log, lock := Files(main)
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return 1, "", err
	}
	fd, err := syscall.Open(lock, syscall.O_RDWR|syscall.O_CREAT, 0o600)
	if err != nil {
		return 1, "", err
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return 0, "another pm push holds the lock; skipped", nil
	}
	state, err := ReadState(main)
	if err != nil {
		return 1, "", err
	}
	var lines []string
	for _, s := range steps {
		at := iso(Now())
		ok, said := s.Run()
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
// service's install when it never succeeded; none before both. store "" skips the count of unpushed records commits.
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
		if !ok {
			installed, ok := state["installed_at"].(string)
			if !ok {
				continue
			}
			s = stepState{OK: true, First: installed}
		}
		var line string
		if !s.OK {
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
		if s, ok := state.step(name); ok {
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
