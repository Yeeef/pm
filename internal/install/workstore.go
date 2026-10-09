package install

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/work"
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

// beadsRef is where bd keeps its Dolt database on the git remote.
const beadsRef = "refs/dolt/data"

// RemoteHasStore is whether the git remote holds a work store under work.RemoteRef; an unreachable remote is an Error.
// git ls-remote runs within c's deadline, when it has one: in the pm service, the operation's bound.
func RemoteHasStore(c context.Context, dir, remote string) (bool, error) {
	return remoteHasRef(c, dir, remote, work.RemoteRef)
}

func remoteHasRef(c context.Context, dir, remote, ref string) (bool, error) {
	o := proc.Options{Cwd: &dir}
	if deadline, ok := c.Deadline(); ok {
		if o.Timeout = time.Until(deadline); o.Timeout <= 0 {
			return false, fmt.Errorf("git ls-remote %s %s: %w", remote, ref, context.DeadlineExceeded)
		}
		o.TimeoutText = fmt.Sprintf("%.1f", o.Timeout.Seconds())
	}
	res, err := proc.Run([]string{"git", "ls-remote", "--exit-code", remote, ref}, o)
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
	return false, refuse("git ls-remote %s %s failed: %s", remote, ref, trimNewline(why))
}

// SetupWork is pm init's work-store step: it asks the clone's pm service, which must be up, to attach the store to
// the repo's remote (CALL pm_setup(); SetupStore runs it). What it did, one line each.
func SetupWork(main string) ([]string, error) {
	d, err := work.DialSetup(main)
	if err != nil {
		return nil, err
	}
	out, err := d.CallSetup()
	return out, errors.Join(err, d.Shutdown())
}

// SetupStore is what the pm service runs for pm_setup(), on a connection of its own with no database selected: it
// attaches the clone's work store to the repo's remote, under work.RemoteRef. A missing store is cloned from the
// remote when it holds one, else created and pushed there; a store without a remote gets the repo's, and one the
// remote lacks is pushed. A clone whose repo has no such git remote keeps its store as it is: there is nothing to
// attach it to (pm where says so). It refuses to start an empty store beside the Beads data a remote holds (the import
// comes first), and to leave a store made here beside another the remote holds, which share no history. What it did,
// one line each.
func SetupStore(c context.Context, d *work.Dolt, main, remote string) ([]string, error) {
	dir, _ := work.Locations(main)
	url := RemoteURL(main, remote)
	var out []string
	has, err := d.HasStore()
	if err != nil {
		return nil, err
	}
	if !has {
		if url == "" {
			return nil, refuse("no work store at %s, and this repo has no remote %s to clone it from or push it to; "+
				"add it (git remote add %s URL) and run pm init again", dir, remote, remote)
		}
		held, err := RemoteHasStore(c, main, remote)
		if err != nil {
			return nil, err
		}
		if held {
			if err := d.Clone(url); err != nil {
				return nil, err
			}
			return []string{fmt.Sprintf("cloned the work store from %s's %s into %s", remote, work.RemoteRef, dir)}, nil
		}
		if beads, err := remoteHasRef(c, main, remote, beadsRef); err != nil {
			return nil, err
		} else if beads {
			return nil, refuse("%s holds Beads data (%s) but no work store (%s), and an empty store here would fork the "+
				"project's work from it: import it first (bd export > FILE, then pm init --import-bd FILE), then run pm "+
				"init again", remote, beadsRef, work.RemoteRef)
		}
		if err := d.CreateStore(); err != nil {
			return nil, err
		}
		if err := d.AddRemote(url); err != nil {
			return nil, err
		}
		if err := d.Push(); err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("created the work store at %s and pushed it to %s's %s", dir, remote,
			work.RemoteRef)}, nil
	}
	if url == "" {
		return nil, nil
	}
	if err := d.UseStore(); err != nil {
		return nil, err
	}
	_, ok, err := d.Remote()
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := d.AddRemote(url); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("pointed the work store %s at %s's %s", dir, remote, work.RemoteRef))
	}
	if _, _, tracked, err := d.Tracking(); err != nil || tracked {
		return out, err // it has fetched or pushed the remote's store: attached
	}
	held, err := RemoteHasStore(c, main, remote)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, refuse("%s", unrelated(dir, remote))
	}
	if err := d.Push(); err != nil {
		return nil, err
	}
	return append(out, fmt.Sprintf("pushed the work store to %s's %s", remote, work.RemoteRef)), nil
}

// WorkDrift is how the clone's work store differs from what pm init makes, one line each; a service that does not
// answer, or a clone with no store, is one such line.
func WorkDrift(main, remote string) ([]string, error) {
	dir, _ := work.Locations(main)
	d, err := work.Dial(main)
	if err != nil {
		return []string{err.Error()}, nil
	}
	if RemoteURL(main, remote) == "" {
		return nil, d.Shutdown()
	}
	_, ok, err := d.Remote()
	tracked := false
	if err == nil && ok {
		_, _, tracked, err = d.Tracking()
	}
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return nil, err
	}
	if !ok {
		return []string{fmt.Sprintf("%s has no remote, so it syncs with no other clone; run pm init", dir)}, nil
	}
	if !tracked {
		held, err := RemoteHasStore(context.Background(), main, remote)
		if err != nil {
			return nil, err
		}
		if held {
			return []string{unrelated(dir, remote)}, nil
		}
		return []string{fmt.Sprintf("%s is not on %s's %s yet; run pm init", dir, remote, work.RemoteRef)}, nil
	}
	return nil, nil
}

// unrelated is what to say of a store made in this clone while the remote holds another.
func unrelated(dir, remote string) string {
	return fmt.Sprintf("the work store %s was made in this clone, and %s holds another at %s: the two share no history, "+
		"so no sync can merge them; keep the remote's: move this one away (pm export --store %s > FILE keeps its items) "+
		"and run pm init, which clones it", dir, remote, work.RemoteRef, dir)
}

// WorkUnsynced is why removing the clone's work store would lose items: it has no remote, or commits the remote
// lacks; "" when the remote holds all of it, or there is no store. It reads the store through the pm service, so it
// fails when the service does not answer.
func WorkUnsynced(main, remote string) (string, error) {
	dir, _ := work.Locations(main)
	d, err := work.Dial(main)
	if errors.Is(err, work.ErrNoStore) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_, ok, err := d.Remote()
	ahead, tracked := 0, false
	if err == nil && ok {
		ahead, _, tracked, err = d.Tracking()
	}
	if err = errors.Join(err, d.Shutdown()); err != nil {
		return "", err
	}
	switch {
	case !ok:
		return fmt.Sprintf("the work store %s has no remote, so its items are on this clone only", dir), nil
	case !tracked:
		return fmt.Sprintf("the work store %s was never pushed to %s's %s", dir, remote, work.RemoteRef), nil
	case ahead > 0:
		return fmt.Sprintf("the work store %s holds %d commit(s) %s's %s lacks", dir, ahead, remote, work.RemoteRef), nil
	}
	return "", nil
}

// WorkState is pm where's work line state: the store's remote and how it stands against the remote's store as of the
// last fetch or push, or why it cannot be read (no store, a service that does not answer).
func WorkState(main, remote string) (string, error) {
	d, err := work.Dial(main)
	if err != nil {
		return err.Error(), nil
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
