package work

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// The host's operation slot (the pm-go page, What runs in the service): one sync, create or setup at a time, each
// bounded, the wait for the slot included; and the garbage collection outside it.

// slotStore is a host with a store pushed to a bare remote, so a child create goes through pm_create, and ops.
func slotStore(t *testing.T, ops Ops) (*served, Item) {
	t.Helper()
	s := host(t, shortMain(t), Options{Prefix: "demo"}, ops)
	d, err := dial(s.h.sock, dialConfig{prefix: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown()
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
	p := must(d.Create(New{Type: Project, Title: "P"}))
	if err := d.AddRemote(bareRemote(t)); err != nil {
		t.Fatal(err)
	}
	if err := d.Push(); err != nil {
		t.Fatal(err)
	}
	return s, p
}

// A collection that held the slot waited for every command in CALL pm_sync() or pm_create(), each waiting for the
// slot: a deadlock until the collection's timeout. Outside the slot, each collection racing such calls in a loop
// finishes at once and succeeds.
func TestGCRacingSyncsAndCreatesFinishesPromptly(t *testing.T) {
	s, p := slotStore(t, Ops{Sync: func(c context.Context, d *Dolt) ([]string, error) {
		_, _, err := d.Remote()
		return []string{"ok"}, err
	}})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	loop := func(call func(i int) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				if err := call(i); err != nil {
					mu.Lock()
					errs = append(errs, err.Error())
					mu.Unlock()
				}
			}
		}()
	}
	syncer, creator := s.dial(t), s.dial(t)
	loop(func(int) error { _, err := syncer.CallSync(); return err })
	loop(func(i int) error {
		_, err := creator.Create(New{Type: Task, Parent: p.ID, Title: fmt.Sprint("C", i)})
		return err
	})
	var slowest time.Duration
	for range 10 {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		err := s.h.GC(c)
		cancel()
		slowest = max(slowest, time.Since(start))
		if err != nil {
			t.Errorf("gc: %v", err)
		}
	}
	close(stop)
	wg.Wait()
	for _, e := range errs {
		t.Errorf("a call racing the collections: %s", e)
	}
	if slowest > 5*time.Second {
		t.Errorf("the slowest of 10 collections took %s", slowest)
	}
	t.Logf("10 collections racing pm_sync and pm_create calls; the slowest took %s", slowest.Round(time.Millisecond))
}

// An operation that outlasts its bound is stopped and frees the slot; a caller waiting behind it gives up at its own
// bound, naming what holds the slot, and writes nothing.
func TestAnOperationOutlastingItsBoundFreesTheSlotAndAWaiterGivesUp(t *testing.T) {
	syncT, createT := SyncTimeout, CreateTimeout
	SyncTimeout, CreateTimeout = 2*time.Second, 300*time.Millisecond
	t.Cleanup(func() { SyncTimeout, CreateTimeout = syncT, createT })
	hang := make(chan struct{}, 1)
	s, p := slotStore(t, Ops{Sync: func(c context.Context, d *Dolt) ([]string, error) {
		select {
		case <-hang: // a sync told to hang waits for its bound
			<-c.Done()
			return nil, c.Err()
		default:
			return []string{"ok"}, nil
		}
	}})
	hang <- struct{}{}
	synced := make(chan error, 1)
	start := time.Now()
	go func() { _, err := s.dial(t).CallSync(); synced <- err }()
	for s.h.holding() != "sync" {
		time.Sleep(time.Millisecond)
	}
	d := s.dial(t)
	before := must(d.Items())
	_, err := d.Create(New{Type: Task, Parent: p.ID, Title: "behind the sync"})
	if err == nil || !strings.Contains(err.Error(), "the create of a task under "+p.ID+" waited 300ms for the pm "+
		"service's sync, running for ") || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("a create behind a held slot: %v", err)
	}
	if len(must(d.Items())) != len(before) {
		t.Fatal("the create that gave up wrote")
	}
	err = <-synced
	if took := time.Since(start); err == nil || !strings.Contains(err.Error(), "the sync did not finish within 2s and "+
		"was stopped") || took > 4*time.Second {
		t.Fatalf("a hung sync after %s: %v", took, err)
	}
	CreateTimeout = createT
	if _, err := d.Create(New{Type: Task, Parent: p.ID, Title: "after"}); err != nil {
		t.Fatalf("the slot stayed taken: %v", err)
	}
}

// A fetch that hangs (a remote whose transport never answers) ends at the sync's bound: the git Dolt runs is killed
// under the operation's context, and the slot is free for the next caller.
func TestAHungFetchEndsAtTheSyncsBound(t *testing.T) {
	syncT := SyncTimeout
	SyncTimeout = time.Second
	t.Cleanup(func() { SyncTimeout = syncT })
	// ssh to the remote never answers: it reads until git, killed at the bound, closes its end (true takes git's
	// arguments), so nothing outlives the test
	t.Setenv("GIT_SSH_COMMAND", "cat > /dev/null; true")
	s := host(t, shortMain(t), Options{Prefix: "demo"}, Ops{Sync: func(c context.Context, d *Dolt) ([]string, error) {
		r, err := d.SyncContext(c)
		return []string{fmt.Sprint(r)}, err
	}})
	d, err := dial(s.h.sock, dialConfig{prefix: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown()
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
	if err := d.AddRemote("git@example.invalid:never.git"); err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second finds the slot free
		start := time.Now()
		_, err := d.CallSync()
		if took := time.Since(start); err == nil || !strings.Contains(err.Error(), "the sync did not finish within 1s") ||
			took > 5*time.Second {
			t.Fatalf("a sync on a hung remote after %s: %v", took, err)
		}
	}
}
