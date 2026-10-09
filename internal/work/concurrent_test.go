package work

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Concurrent writers (the work-store page, Data model): many sessions write through one service at once, each on its
// own connection. Every write sets the write stamp, so any two concurrent writes conflict and the one that loses runs
// again from a fresh read: 8 writers x 20 writes land 160 of 160, with no lock to wait on, and an invariant that spans
// rows holds.

func TestEightWritersTwentyWritesEachAllLand(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	s := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "S"}))
	var shared []Item // three items every writer comments on: the same rows, so the writes conflict
	for i := range 3 {
		shared = append(shared, must(d.Create(New{Type: Task, Parent: s.ID, Title: fmt.Sprint("T", i)})))
	}
	const writers, writes = 8, 20
	before := Attempts.Retries.Load()
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
	took, retries := time.Since(start), Attempts.Retries.Load()-before
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
	t.Logf("%d x %d writes: %d landed in %s, %d retries, at most %d attempts for one write", writers, writes, len(seen),
		took.Round(time.Millisecond), retries, Attempts.Max.Load())
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

// A write that loses WriteAttempts times fails hard, naming the write, and writes nothing.
func TestAWriteThatAlwaysLosesFailsHard(t *testing.T) {
	d, o := serve(t, Options{Prefix: "demo"})
	p := must(d.Create(New{Type: Project, Title: "P"}))
	other := o.dial(t)
	sleep := backoff
	backoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { backoff = sleep })
	title := "never"
	err := d.update("edit", p.ID, func(it *Item, _ *Index) error {
		// another session's write lands while this one runs, every attempt
		if _, err := other.Comment(p.ID, Note, "s", "meanwhile"); err != nil {
			return err
		}
		it.Title = title
		return nil
	})
	want := fmt.Sprintf("work store: edit %s lost to concurrent writes %d times; nothing was written; run the command "+
		"again", p.ID, WriteAttempts)
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
	if got := must(d.Get(p.ID))[0]; got.Title != "P" || len(got.Comments) != WriteAttempts {
		t.Fatalf("title %q, %d comments", got.Title, len(got.Comments))
	}
}
