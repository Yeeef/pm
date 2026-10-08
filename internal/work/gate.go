package work

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The gate: an exclusive kernel flock on <main checkout>/.pm/run/work.lock that a pm process takes before it opens the
// store and keeps until it closes it, for reads too, because the engine's own lock is exclusive for readers as well.
// Processes queue on it in the kernel. A waiter that times out fails hard, naming the lock file. Every open appends
// its wait to <main checkout>/.pm/run/work-gate.log: the evidence for moving to a store the service holds (pm-go
// page, Store sharing between the CLI and the service).

const (
	GateFile    = "work.lock"
	GateLogFile = "work-gate.log"
	// GateTimeout bounds the wait: 8 contending writers waited 524 ms at most in the spike.
	GateTimeout = 30 * time.Second
)

type gate struct{ f *os.File }

// takeGate takes the gate in runDir, waiting at most timeout, logs the wait, and returns it with the wait.
func takeGate(runDir string, timeout time.Duration) (*gate, time.Duration, error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, 0, fmt.Errorf("work store gate: %w", err)
	}
	path := filepath.Join(runDir, GateFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, 0, fmt.Errorf("work store gate: %w", err)
	}
	start := time.Now()
	type result struct{ err error }
	got := make(chan result, 1)
	var mu sync.Mutex
	abandoned := false
	go func() {
		err := flock(f)
		mu.Lock()
		defer mu.Unlock()
		if abandoned { // the waiter timed out and returned: let go of what it no longer wants
			if err == nil {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			}
			_ = f.Close()
			return
		}
		got <- result{err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-got:
		wait := time.Since(start)
		if r.err != nil {
			_ = f.Close()
			return nil, wait, fmt.Errorf("work store gate %s: %w", path, r.err)
		}
		g := &gate{f}
		if err := logWait(runDir, wait); err != nil {
			g.release()
			return nil, wait, err
		}
		return g, wait, nil
	case <-timer.C:
		mu.Lock()
		defer mu.Unlock()
		select {
		case r := <-got: // taken just as the timer fired
			if r.err == nil {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			}
			_ = f.Close()
		default:
			abandoned = true
		}
		return nil, time.Since(start), fmt.Errorf("work store gate %s: another pm process held it for over %s; "+
			"see which with lsof %s", path, timeout, path)
	}
}

// flock takes the exclusive lock, retrying when a signal interrupts the wait.
func flock(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			return err
		}
	}
}

// logWait appends one line for this open: when, which process, how long it waited, and its command line.
func logWait(runDir string, wait time.Duration) error {
	line := fmt.Sprintf("%s pid=%d wait_ms=%.1f argv=%q\n", time.Now().UTC().Format(time.RFC3339), os.Getpid(),
		float64(wait.Microseconds())/1000, strings.Join(os.Args, " "))
	f, err := os.OpenFile(filepath.Join(runDir, GateLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("work store gate log: %w", err)
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("work store gate log: %w", err)
	}
	return f.Close()
}

// release lets the gate go; closing the file drops the flock.
func (g *gate) release() error {
	if g == nil || g.f == nil {
		return nil
	}
	err := g.f.Close()
	g.f = nil
	return err
}
