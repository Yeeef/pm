package session

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Yeeef/pm/internal/config"
)

// modesDir holds, under the main checkout's .pm/run, one file per session in interactive mode, named by the session's
// id; a session with no file is in autopilot, the default. pm interactive writes one and pm autopilot removes it, run
// by the session's agent when the owner asks for the mode; nothing enforces who asked; the session's end removes it
// (pm hook session-end). It is local to the clone, shared by every worktree of it, and never synced.
const modesDir = "sessions"

// modePath is the mode file of session id in the main checkout main; an error for an id that is not one path
// component of letters, digits, _ and -.
func modePath(main, id string) (string, error) {
	if !sessionID.MatchString(id) {
		return "", fmt.Errorf("the session id %q is not letters, digits, _ and - only", id)
	}
	return filepath.Join(main, config.Run, modesDir, id), nil
}

// Interactive is whether the owner set session id to interactive mode in the clone whose main checkout is main. An id
// that cannot name a mode file is in autopilot.
func Interactive(main, id string) (bool, error) {
	path, err := modePath(main, id)
	if err != nil {
		return false, nil //nolint:nilerr // no mode file can exist for it, so it is in autopilot
	}
	switch _, err := os.Stat(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// SetInteractive writes session id's mode file when on, else removes it; either is a no-op when already so.
func SetInteractive(main, id string, on bool) error {
	path, err := modePath(main, id)
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("interactive\n"), 0o644)
}

// ModeWindow is how long a session's transcripts must have been untouched for pm clean to remove its mode file: a day,
// not LiveWindow, so an interactive session the owner stepped away from keeps its mode. A session that ends removes its
// own (pm hook session-end); pm clean removes what a session that crashed left.
const ModeWindow = 24 * time.Hour

// Mode is one session's mode file: its path, the session's id, and whether the session was active within ModeWindow.
type Mode struct {
	Path, Session string
	Active        bool
}

// Modes is every mode file in the clone whose main checkout is main, in id order, each with whether its session was
// active within ModeWindow; none when the directory does not exist.
func Modes(main string) ([]Mode, error) {
	dir := filepath.Join(main, config.Run, modesDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	modes := make([]Mode, 0, len(entries))
	for _, e := range entries {
		modes = append(modes, Mode{Path: filepath.Join(dir, e.Name()), Session: e.Name(), Active: activeWithin(e.Name(), ModeWindow)})
	}
	return modes, nil
}

// SetMode is pm interactive (on) and pm autopilot: session id's mode file written or removed, and the line saying the
// session's mode now.
func SetMode(main, id string, on bool) (string, error) {
	was, err := Interactive(main, id)
	if err == nil {
		err = SetInteractive(main, id, on)
	}
	switch {
	case err != nil:
		return "", err
	case on:
		return fmt.Sprintf("session %s is in interactive mode: the owner-request check lets asks in chat through "+
			"until pm autopilot", id), nil
	case was:
		return fmt.Sprintf("session %s is in autopilot: the owner-request check blocks an ask left only in chat "+
			"again", id), nil
	}
	return fmt.Sprintf("session %s is already in autopilot, the default; nothing changed", id), nil
}

// CleanModes is pm clean's part for the mode files: each listed on w with keep or remove, as pm clean lists a
// worktree; removed when its session has been idle for ModeWindow (a session that crashed left it), else kept; with
// apply, the removals done. It returns how many it was to remove and how many of those failed.
func CleanModes(main string, apply bool, w io.Writer) (removing, failed int, err error) {
	modes, err := Modes(main)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range modes {
		word, reason := "keep", "the session was active within a day"
		if !m.Active {
			word, reason = "remove", "the session has been idle for a day"
			removing++
		}
		line := fmt.Sprintf("%-6s  %s  (interactive mode of session %s): %s", word, m.Path, m.Session, reason)
		if !m.Active && apply {
			if err := os.Remove(m.Path); err != nil {
				line += "; NOT removed: " + err.Error()
				failed++
			}
		}
		fmt.Fprintln(w, line)
	}
	return removing, failed, nil
}
