package install

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// RemoteURL is the git remote's URL in dir, "" when the repo has no such remote.
func RemoteURL(dir, remote string) string {
	res, err := proc.Run([]string{"git", "remote", "get-url", remote}, proc.Options{Cwd: &dir})
	if err != nil || res.Code != 0 {
		return ""
	}
	return trimNewline(res.Stdout)
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// RemoteHasStore is whether the git remote holds a work store under work.RemoteRef; an unreachable remote is an Error.
func RemoteHasStore(dir, remote string) (bool, error) {
	res, err := proc.Run([]string{"git", "ls-remote", "--exit-code", remote, work.RemoteRef}, proc.Options{Cwd: &dir})
	if err != nil {
		return false, err
	}
	switch res.Code {
	case 0:
		return true, nil
	case 2:
		return false, nil
	}
	why := res.Stderr
	if why == "" {
		why = res.Stdout
	}
	return false, refuse("git ls-remote %s %s failed: %s", remote, work.RemoteRef, trimNewline(why))
}

// quietStdout runs fn with the process's standard output sent to /dev/null: Dolt's clone prints its progress there,
// which is no part of pm's output.
func quietStdout(fn func() error) error {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	saved, err := unix.Dup(1)
	if err != nil {
		return err
	}
	defer unix.Close(saved)
	if err := unix.Dup2(int(null.Fd()), 1); err != nil {
		return err
	}
	ferr := fn()
	if err := unix.Dup2(saved, 1); err != nil {
		return errors.Join(ferr, err)
	}
	return ferr
}

// SetupWork attaches the clone's work store to the repo's remote, under work.RemoteRef: a missing store is cloned from
// the remote when it holds one, else created and pushed there; a store without a remote gets the repo's, and one the
// remote lacks is pushed. A clone whose repo has no such git remote keeps its store as it is: there is nothing to
// attach it to (pm where says so). What it did, one line each.
func SetupWork(main, remote string) ([]string, error) {
	dir, run := work.Locations(main)
	o := work.Options{Dir: dir, RunDir: run}
	url := RemoteURL(main, remote)
	var out []string
	var d *work.Dolt
	if !work.Exists(dir) {
		if url == "" {
			return nil, refuse("no work store at %s, and this repo has no remote %s to clone it from or push it to; "+
				"add it (git remote add %s URL) and run pm init again", dir, remote, remote)
		}
		has, err := RemoteHasStore(main, remote)
		if err != nil {
			return nil, err
		}
		if has {
			err := quietStdout(func() (err error) { d, err = work.CloneStore(o, url); return err })
			if err != nil {
				return nil, err
			}
			out = append(out, fmt.Sprintf("cloned the work store from %s's %s into %s", remote, work.RemoteRef, dir))
			return out, d.Shutdown()
		}
		if d, err = work.CreateStore(o); err != nil {
			return nil, err
		}
		if err := d.AddRemote(url); err != nil {
			return nil, errors.Join(err, d.Shutdown())
		}
		if err := d.Push(); err != nil {
			return nil, errors.Join(err, d.Shutdown())
		}
		out = append(out, fmt.Sprintf("created the work store at %s and pushed it to %s's %s", dir, remote, work.RemoteRef))
		return out, d.Shutdown()
	}
	if url == "" {
		return nil, nil
	}
	d, err := work.OpenStore(o)
	if err != nil {
		return nil, err
	}
	err = func() error {
		_, ok, err := d.Remote()
		if err != nil {
			return err
		}
		if !ok {
			if err := d.AddRemote(url); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("pointed the work store %s at %s's %s", dir, remote, work.RemoteRef))
		}
		if _, _, tracked, err := d.Tracking(); err != nil || tracked {
			return err // it has fetched or pushed the remote's store: attached
		}
		has, err := RemoteHasStore(main, remote)
		if err != nil || has {
			return err // the remote's store is there: the service's sync merges the two
		}
		if err := d.Push(); err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("pushed the work store to %s's %s", remote, work.RemoteRef))
		return nil
	}()
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return nil, err
	}
	return out, nil
}

// WorkDrift is how the clone's work store differs from what pm init makes, one line each.
func WorkDrift(main, remote string) ([]string, error) {
	dir, run := work.Locations(main)
	if !work.Exists(dir) {
		return []string{fmt.Sprintf("%s is missing; run pm init", dir)}, nil
	}
	if RemoteURL(main, remote) == "" {
		return nil, nil
	}
	d, err := work.OpenStore(work.Options{Dir: dir, RunDir: run})
	if err != nil {
		return nil, err
	}
	_, ok, err := d.Remote()
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return nil, err
	}
	if !ok {
		return []string{fmt.Sprintf("%s has no remote, so it syncs with no other clone; run pm init", dir)}, nil
	}
	return nil, nil
}

// WorkState is pm where's work line state: the store's remote and how it stands against the remote's store as of the
// last fetch or push, or what is missing.
func WorkState(main, remote string) (string, error) {
	dir, run := work.Locations(main)
	if !work.Exists(dir) {
		return "missing; run pm init", nil
	}
	d, err := work.OpenStore(work.Options{Dir: dir, RunDir: run})
	if err != nil {
		return "", err
	}
	var state string
	err = func() error {
		_, ok, err := d.Remote()
		if err != nil {
			return err
		}
		if !ok {
			state = "no remote, so it syncs with no other clone; run pm init"
			return nil
		}
		ahead, behind, tracked, err := d.Tracking()
		if err != nil {
			return err
		}
		upstream := remote + " " + work.RemoteRef
		if !tracked {
			state = "not pushed to " + upstream + " yet"
		} else {
			state = fmt.Sprintf("%d ahead, %d behind %s (as of the last sync)", ahead, behind, upstream)
		}
		return nil
	}()
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return "", err
	}
	return state, nil
}
