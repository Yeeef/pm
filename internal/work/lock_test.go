package work

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// The store's write lock (lock.go): FIFO, bounded waits, takeover from a holder whose connection is gone, refusals
// of a second lock on one connection and of an unlock without the lock. First on the lock itself, then through the
// host's pm_lock and pm_unlock.

func newLock(alive func(uint32) bool) *writeLock {
	if alive == nil {
		alive = func(uint32) bool { return true }
	}
	return &writeLock{alive: alive}
}

// queued waits until n waiters are queued.
func queued(t *testing.T, l *writeLock, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		l.mu.Lock()
		got := len(l.queue)
		l.mu.Unlock()
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d waiters queued, want %d", got, n)
		}
		time.Sleep(time.Millisecond)
	}
}

// Waiters get the lock in the order they asked, each holding it in turn: an unfair lock (one that hands it to any
// waiter, or to a fresh asker, as go-mysql-server's GET_LOCK does) fails this.
func TestTheWriteLockIsFIFO(t *testing.T) {
	l := newLock(nil)
	never := make(chan struct{})
	must(0, l.lock(1, 1, "first", time.Minute, never))
	var mu sync.Mutex
	var order []uint32
	var wg sync.WaitGroup
	for s := uint32(2); s <= 7; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			must(0, l.lock(s, int64(s), "w", time.Minute, never))
			mu.Lock()
			order = append(order, s)
			mu.Unlock()
			must(0, l.unlock(s))
		}()
		queued(t, l, int(s-1)) // s asks only once s-1 is queued
	}
	// a fresh asker after them is queued behind them, not served first
	late := make(chan uint32, 1)
	go func() { must(0, l.lock(9, 9, "late", time.Minute, never)); late <- 9; must(0, l.unlock(9)) }()
	queued(t, l, 7)
	must(0, l.unlock(1))
	wg.Wait()
	<-late
	if got := order; len(got) != 6 || got[0] != 2 || got[5] != 7 || !orderedAsc(got) {
		t.Fatalf("waiters got the lock in the order %v", got)
	}
}

func orderedAsc(s []uint32) bool {
	for i := 1; i < len(s); i++ {
		if s[i] < s[i-1] {
			return false
		}
	}
	return true
}

// A wait ends at its bound, naming the holder, and leaves the queue; one ended by its statement leaves it too, so the
// lock never passes to a waiter that went.
func TestAWriteLockWaitEndsAtItsBoundOrItsStatement(t *testing.T) {
	l := newLock(nil)
	never := make(chan struct{})
	must(0, l.lock(1, 4242, "edit demo-1", time.Minute, never))
	start := time.Now()
	err := l.lock(2, 2, "w", 100*time.Millisecond, never)
	if !errors.Is(err, errLockTimeout) || !strings.Contains(err.Error(), "held by connection 1, pid 4242, edit demo-1, for ") ||
		time.Since(start) > 2*time.Second {
		t.Fatalf("a wait at its bound: %v", err)
	}
	done := make(chan struct{})
	ended := make(chan error, 1)
	go func() { ended <- l.lock(3, 3, "w", time.Minute, done) }()
	queued(t, l, 1)
	close(done)
	if err := <-ended; err == nil {
		t.Fatal("a wait whose statement ended took the lock")
	}
	queued(t, l, 0)
	must(0, l.unlock(1))
	if l.owner != 0 {
		t.Fatalf("the lock passed to %d, a waiter that went", l.owner)
	}
}

// A holder whose session is gone (its connection dropped) is taken over by the first waiter within lockCheck.
func TestAWriteLockHolderWhoseConnectionIsGoneIsTakenOver(t *testing.T) {
	var mu sync.Mutex
	gone := map[uint32]bool{}
	l := newLock(func(s uint32) bool { mu.Lock(); defer mu.Unlock(); return !gone[s] })
	never := make(chan struct{})
	must(0, l.lock(1, 1, "w", time.Minute, never))
	got := make(chan error, 1)
	go func() { got <- l.lock(2, 2, "w", time.Minute, never) }()
	queued(t, l, 1)
	mu.Lock()
	gone[1] = true
	mu.Unlock()
	start := time.Now()
	if err := <-got; err != nil || l.owner != 2 {
		t.Fatalf("no takeover: %v, owner %d", err, l.owner)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("the takeover took %s", took)
	}
}

func TestASecondLockOnAConnectionAndAnUnlockWithoutItAreRefused(t *testing.T) {
	l := newLock(nil)
	never := make(chan struct{})
	must(0, l.lock(1, 1, "w", time.Minute, never))
	if err := l.lock(1, 1, "w", time.Minute, never); err == nil ||
		err.Error() != "this connection holds the write lock already" {
		t.Fatalf("a second lock: %v", err)
	}
	if err := l.unlock(2); err == nil || err.Error() != "this connection holds no write lock" {
		t.Fatalf("an unlock without the lock: %v", err)
	}
	must(0, l.unlock(1))
}

// Through the host: a write takes over from a holder whose connection dropped, without pm_unlock; a write behind a
// live holder fails at its bound, a sub-second one included, naming the holder's connection, pid and write.
func TestPmLockThroughTheHost(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	dead := o.dial(t)
	if _, err := dead.conn.ExecContext(ctx, "CALL pm_lock(60000, 7, 'crashed')"); err != nil {
		t.Fatal(err)
	}
	dead.Shutdown()
	start := time.Now()
	must(0, d.Edit(p.ID, ptr("taken over"), nil))
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took over a dead holder in %s", took)
	}
	live := o.dial(t)
	if _, err := live.conn.ExecContext(ctx, "CALL pm_lock(60000, 4242, 'stalled write')"); err != nil {
		t.Fatal(err)
	}
	old := WriteLockWait
	t.Cleanup(func() { WriteLockWait = old })
	WriteLockWait = 300 * time.Millisecond
	start = time.Now()
	err := d.Edit(p.ID, ptr("x"), nil)
	if err == nil || !strings.Contains(err.Error(), "waited 300ms for the store's write lock, which a live pm process "+
		"holds (connection ") || !strings.Contains(err.Error(), ", pid 4242, stalled write, for ") ||
		time.Since(start) < 300*time.Millisecond {
		t.Fatalf("a write behind a live holder after %s: %v", time.Since(start), err)
	}
	if _, err := live.conn.ExecContext(ctx, "CALL pm_unlock()"); err != nil {
		t.Fatal(err)
	}
	if _, err := live.conn.ExecContext(context.Background(), "CALL pm_unlock()"); err == nil {
		t.Fatal("an unlock without the lock passed")
	}
	must(0, d.Edit(p.ID, ptr("x"), nil))
}
