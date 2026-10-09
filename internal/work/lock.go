package work

import (
	"errors"
	"sync"
	"time"
)

// The store's write lock (the work-store page, Concurrent writers): every pm write transaction holds it from before
// its first read to after its commit or rollback, so pm's writes run one at a time and a long one (a merge over every
// item) never loses to shorter ones. The host serves it as CALL pm_lock(seconds) and CALL pm_unlock() on the
// command's connection. It is fair: writers get it in the order they asked, so no writer waits behind a stream of
// later ones (go-mysql-server's GET_LOCK polls a compare-and-swap and keeps no order). A holder whose connection
// dropped holds it no more: a waiter that finds the holder's session gone takes it over.

// writeLock is a FIFO lock owned by server session ids.
type writeLock struct {
	mu    sync.Mutex
	owner uint32 // 0: free
	queue []*lockWaiter
	alive func(session uint32) bool // whether the session still has a connection
}

type lockWaiter struct {
	session uint32
	ready   chan struct{} // closed once the lock is handed to this waiter
}

// errLockTimeout is a wait for the lock that reached its bound.
var errLockTimeout = errors.New("timed out")

// lockCheck is how often a waiter looks whether the holder's connection is gone.
const lockCheck = 50 * time.Millisecond

// lock waits up to timeout for the lock, in order of asking, for session; done ends the wait early.
func (l *writeLock) lock(session uint32, timeout time.Duration, done <-chan struct{}) error {
	l.mu.Lock()
	if l.owner == session {
		l.mu.Unlock()
		return errors.New("this connection holds the write lock already")
	}
	if l.owner == 0 && len(l.queue) == 0 {
		l.owner = session
		l.mu.Unlock()
		return nil
	}
	w := &lockWaiter{session: session, ready: make(chan struct{})}
	l.queue = append(l.queue, w)
	l.mu.Unlock()
	timer, check := time.NewTimer(timeout), time.NewTicker(lockCheck)
	defer timer.Stop()
	defer check.Stop()
	for {
		select {
		case <-w.ready:
			return nil
		case <-check.C:
			l.mu.Lock()
			if l.owner != 0 && !l.alive(l.owner) {
				l.handOff()
			}
			l.mu.Unlock()
		case <-timer.C:
			return l.leave(w, errLockTimeout)
		case <-done:
			return l.leave(w, errors.New("the statement ended"))
		}
	}
}

// leave takes w out of the queue after a wait that ended without the lock, unless the lock reached it meanwhile.
func (l *writeLock) leave(w *lockWaiter, why error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == w.session {
		select {
		case <-w.ready:
			return nil // handed over as the wait ended: it holds the lock
		default:
		}
	}
	for i, q := range l.queue {
		if q == w {
			l.queue = append(l.queue[:i], l.queue[i+1:]...)
			break
		}
	}
	return why
}

// unlock releases the lock session holds, handing it to the first waiter.
func (l *writeLock) unlock(session uint32) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner != session {
		return errors.New("this connection holds no write lock")
	}
	l.handOff()
	return nil
}

// handOff gives the lock to the first waiter, or frees it; l.mu is held.
func (l *writeLock) handOff() {
	if len(l.queue) == 0 {
		l.owner = 0
		return
	}
	w := l.queue[0]
	l.queue = l.queue[1:]
	l.owner = w.session
	close(w.ready)
}
