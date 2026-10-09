package work

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Concurrent writers (the work-store page, Data model): many sessions write through one service at once, each on its
// own connection. Every write holds the store's write lock, in the order the writers asked, so 8 writers x 20 writes
// land 160 of 160 and an invariant that spans rows holds; the write stamp turns a write that raced one outside the
// lock into a hard failure.

func TestEightWritersTwentyWritesEachAllLand(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	s := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "S"}))
	var shared []Item // three items every writer comments on: the same rows, so the writes conflict
	for i := range 3 {
		shared = append(shared, must(d.Create(New{Type: Task, Parent: s.ID, Title: fmt.Sprint("T", i)})))
	}
	const writers, writes = 8, 20
	waits, waited := Waits.Writes.Load(), Waits.Total.Load()
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, writers*writes)
	for w := range writers {
		c := o.dial(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range writes {
				if _, err := c.Comment(shared[(w+i)%3].ID, Note, "s", fmt.Sprintf("w%d-%d", w, i)); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	took := time.Since(start)
	n := Waits.Writes.Load() - waits
	mean := time.Duration((Waits.Total.Load() - waited) / max(n, 1))
	for err := range errs {
		t.Error(err)
	}
	seen := map[string]int{}
	for _, it := range must(d.Items()) { // Items checks every invariant
		for _, c := range it.Comments {
			seen[c.Text]++
		}
	}
	for w := range writers {
		for i := range writes {
			if n := seen[fmt.Sprintf("w%d-%d", w, i)]; n != 1 {
				t.Errorf("w%d-%d landed %d times", w, i, n)
			}
		}
	}
	t.Logf("%d x %d writes: %d landed in %s; a write waited %s for the lock on average, at most %s", writers, writes,
		len(seen), took.Round(time.Millisecond), mean.Round(time.Microsecond),
		time.Duration(Waits.Max.Load()).Round(time.Microsecond))
}

// Two opposite pm dep add at once change different rows, so without the stamp both would land and make a cycle that
// fails every later read. With it, exactly one lands; the other runs again, sees the first, and refuses the cycle.
func TestOppositeDepAddsAtOnceExactlyOneLands(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	s := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "S"}))
	a := must(d.Create(New{Type: Task, Parent: s.ID, Title: "A"}))
	b := must(d.Create(New{Type: Task, Parent: s.ID, Title: "B"}))
	for range 5 {
		ca, cb := o.dial(t), o.dial(t)
		var wg sync.WaitGroup
		var ea, eb error
		wg.Add(2)
		go func() { defer wg.Done(); ea = ca.DepAdd(a.ID, b.ID) }()
		go func() { defer wg.Done(); eb = cb.DepAdd(b.ID, a.ID) }()
		wg.Wait()
		if (ea == nil) == (eb == nil) {
			t.Fatalf("both or neither landed: %v; %v", ea, eb)
		}
		if err := errorsJoin(ea, eb); !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("the loser did not refuse the cycle: %v", err)
		}
		must(d.Items()) // no cycle in the store
		if ea == nil {
			must(0, d.DepRemove(a.ID, b.ID))
		} else {
			must(0, d.DepRemove(b.ID, a.ID))
		}
	}
}

func errorsJoin(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// A write that races one outside the write lock (a SQL client past pm) conflicts on the write stamp and fails hard,
// writing nothing; the change outside pm, never committed, then fails every pm write until it is dropped.
func TestAWriteRacingOneOutsideTheLockFailsHard(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	other := o.dial(t)
	err := d.update("edit", p.ID, func(it *Item, _ *Index) error {
		if _, err := other.conn.ExecContext(ctx, "UPDATE write_stamp SET txn = 'outside' WHERE one = 1"); err != nil {
			return err
		}
		it.Title = "never"
		return nil
	})
	want := "work store: edit " + p.ID + " conflicted with a write that did not take the store's write lock (a SQL " +
		"client past pm?); nothing was written: run the command again"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
	if got := must(d.Get(p.ID))[0]; got.Title != "P" {
		t.Fatalf("title %q", got.Title)
	}
	err = d.Edit(p.ID, ptr("later"), nil)
	if err == nil || !strings.Contains(err.Error(), "found the store's working set differing from its head in "+
		"write_stamp, which no pm write leaves") {
		t.Fatalf("a write over a change outside pm: %v", err)
	}
	if _, err := other.conn.ExecContext(ctx, "CALL DOLT_RESET('--hard')"); err != nil {
		t.Fatal(err)
	}
	must(0, d.Edit(p.ID, ptr("later"), nil))
}

// The write lock is fair: a long write waiting behind a stream of short ones gets it in its turn.
func TestALongWriteIsNotStarvedByShortOnes(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range 3 {
		c := o.dial(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := c.Comment(p.ID, Note, "s", fmt.Sprintf("w%d-%d", w, i)); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	for range 5 { // each holds the lock 50 ms, 10 or more times a short write's
		start := time.Now()
		must(0, d.update("slow edit", p.ID, func(it *Item, _ *Index) error {
			time.Sleep(50 * time.Millisecond)
			it.Title += "+"
			return nil
		}))
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("a long write behind 3 tight-loop writers took %s", took)
		}
	}
	close(stop)
	wg.Wait()
}
