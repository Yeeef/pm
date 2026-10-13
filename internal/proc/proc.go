// Package proc runs a command as Python pm's subprocess.run(capture_output=True, text=True, timeout=…) does, and
// reports a failure to start or a timeout as the Python exception would: its type name and its message, with values
// quoted as Go quotes them. pm prints both in hook lines and refusals ("pm show did not run at session start
// (FileNotFoundError: [Errno 2] …)").
package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Error is a command that did not run to an exit status: Python's OSError subclasses and subprocess.TimeoutExpired.
type Error struct {
	Type string // the Python exception's class name, e.g. FileNotFoundError
	Msg  string // its str()
}

func (e *Error) Error() string { return e.Msg }

// Result is a command that ran: its output and exit status.
type Result struct {
	Stdout, Stderr string
	Code           int
}

// Options are subprocess.run's keyword arguments pm uses. Timeout 0 is no timeout; TimeoutText is how Python prints
// the timeout it was given (20, or 19.5 for a float), for TimeoutExpired's message.
type Options struct {
	Cwd         *string  // nil: this process's directory
	Env         []string // nil: this process's environment
	Stdin       string
	Timeout     time.Duration
	TimeoutText string
	Group       bool // run it in a process group of its own, and at the timeout kill the whole group, not only argv[0]
}

// Run runs argv to its exit. A command that could not start, or that ran past its timeout, is an *Error.
func Run(argv []string, o Options) (Result, error) {
	if o.Cwd != nil { // Python's child changes directory before it looks for the program
		if err := dirError(*o.Cwd); err != nil {
			return Result{}, err
		}
	}
	path := argv[0]
	if !strings.Contains(path, "/") {
		found, err := exec.LookPath(path)
		if err != nil {
			return Result{}, &Error{"FileNotFoundError", "[Errno 2] No such file or directory: " + strconv.Quote(path)}
		}
		path = found
	}
	ctx := context.Background()
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Env = o.Env
	if o.Cwd != nil {
		cmd.Dir = *o.Cwd
	}
	cmd.Stdin = strings.NewReader(o.Stdin)
	cmd.WaitDelay = time.Second // a grandchild holding the pipes must not outlive the timeout by long
	// a launcher script (claude's, codex's node wrapper) leaves its real process running when only it dies
	if o.Group {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Start()
	if err == nil {
		if o.Group {
			groups.Store(cmd.Process.Pid, true)
			defer groups.Delete(cmd.Process.Pid)
		}
		err = cmd.Wait()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return Result{}, &Error{"TimeoutExpired",
			fmt.Sprintf("Command %q timed out after %s seconds", argv, o.TimeoutText)}
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Result{text(out), text(errb), 0}, nil
	case errors.As(err, &exit):
		code := exit.ExitCode()
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			code = -int(ws.Signal()) // Python's returncode for a signalled child
		}
		return Result{text(out), text(errb), code}, nil
	case errors.Is(err, fs.ErrPermission):
		return Result{}, &Error{"PermissionError", "[Errno 13] Permission denied: " + strconv.Quote(argv[0])}
	default:
		return Result{}, &Error{"OSError", err.Error()}
	}
}

// groups is the process groups of the Group commands running now, by leader pid.
var groups sync.Map

// KillGroups kills the process group of every Group command running now: for a process that is itself being stopped,
// whose children run in groups of their own and would outlive it.
func KillGroups() {
	groups.Range(func(pid, _ any) bool {
		syscall.Kill(-pid.(int), syscall.SIGKILL) //nolint:errcheck,forcetypeassert // a group already gone is fine; only Run stores, an int
		return true
	})
}

// text is captured output as text=True gives it: universal newlines, \r\n and \r read as \n.
func text(b bytes.Buffer) string {
	return strings.ReplaceAll(strings.ReplaceAll(b.String(), "\r\n", "\n"), "\r", "\n")
}

// dirError is the error Python's child raises when it cannot change into dir, or nil.
func dirError(dir string) error {
	st, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return &Error{"FileNotFoundError", "[Errno 2] No such file or directory: " + strconv.Quote(dir)}
	case errors.Is(err, fs.ErrPermission):
		return &Error{"PermissionError", "[Errno 13] Permission denied: " + strconv.Quote(dir)}
	case err != nil:
		return &Error{"OSError", err.Error()}
	case !st.IsDir():
		return &Error{"NotADirectoryError", "[Errno 20] Not a directory: " + strconv.Quote(dir)}
	}
	return nil
}
