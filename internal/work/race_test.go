package work

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The sync and the compare-and-swap racing the clone's own sessions, which write while the service merges: every
// write lands exactly once or fails by losing WriteAttempts times, and a pull either lands or leaves the store as
// it was. And two clones that each migrate a v2 store to v3 on their own still merge.

// B pulls (a fast-forward when B's writers have not written yet, else a 3-way merge) while 3 of B's sessions write in
// a tight loop, 25 writes each. (Writers that never stop can starve a pull: it then fails after WriteAttempts merges,
// leaving the store as it was, and the next sync lands it.)
func TestPullRacingLocalWriters(t *testing.T) {
	x := newPair(t)
	var mu sync.Mutex
	var writeErrs []string
	starved := 0 // pulls that lost WriteAttempts times to the writers: fail-safe, counted
	var seq atomic.Int64
	landed := map[string]bool{}
	const rounds = 3 // one of each: no wait, 5 ms and 10 ms before the pull
	for round := range rounds {
		x.a.do(func(d *Dolt) { // A writes and pushes, so B is behind
			for i := range 3 {
				must(d.Comment(x.t2.ID, Note, "a", fmt.Sprintf("a-%d-%d", round, i)))
			}
		})
		x.a.sync()
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for range 3 {
			c := x.b.s.dial(t)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 25 { // bounded, so the pull races them but is not starved for the length of the test
					select {
					case <-stop:
						return
					default:
					}
					text := fmt.Sprintf("b-%d", seq.Add(1))
					_, err := c.Comment(x.t1.ID, Note, "b", text)
					mu.Lock()
					if err != nil {
						writeErrs = append(writeErrs, err.Error())
					} else {
						landed[text] = true
					}
					mu.Unlock()
				}
			}()
		}
		time.Sleep(time.Duration(round%3) * 5 * time.Millisecond) // a fast-forward, or a merge with B's writes
		x.b.fresh()
		x.b.do(func(d *Dolt) {
			if _, err := d.Sync(); err != nil {
				if !strings.Contains(err.Error(), "the store stays as it was") {
					t.Errorf("a pull racing local writers: %v", err)
				}
				starved++
			}
		})
		x.b.synced = time.Now()
		close(stop)
		wg.Wait()
		x.b.sync() // B's writes, and what a starved pull left, with no writer racing
		x.a.sync()
	}
	for _, e := range writeErrs {
		t.Errorf("a write: %s", e)
	}
	seen := map[string]int{}
	for _, it := range x.b.items() {
		for _, c := range it.Comments {
			seen[c.Text]++
		}
	}
	for text := range landed {
		if seen[text] != 1 {
			t.Errorf("%s landed %d times", text, seen[text])
		}
	}
	for round := range rounds {
		for i := range 3 {
			if n := seen[fmt.Sprintf("a-%d-%d", round, i)]; n != 1 {
				t.Errorf("a-%d-%d is on B %d times", round, i, n)
			}
		}
	}
	t.Logf("%d of B's writes landed once each; %d of %d pulls lost to the writers every time", len(landed), starved,
		rounds)
}

// B's child creates through its service (pm_create: pull, mint on pm-cas, push, merge pm-cas into main) while one
// of B's sessions writes now and then: every create and every write lands.
func TestCreateRacingALocalWriter(t *testing.T) {
	x := newPair(t)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var writeErrs []string
	c := x.b.s.dial(t)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := c.Comment(x.t1.ID, Note, "b", fmt.Sprint("w", i)); err != nil {
				mu.Lock()
				writeErrs = append(writeErrs, err.Error())
				mu.Unlock()
			}
			time.Sleep(time.Duration(i%7) * time.Millisecond)
		}
	}()
	d := x.b.s.dial(t)
	const creates = 15
	for i := range creates {
		if _, err := d.Create(New{Type: Task, Parent: x.s.ID, Title: fmt.Sprint("C", i)}); err != nil {
			t.Errorf("create %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
	for _, e := range writeErrs {
		t.Errorf("a write: %s", e)
	}
	made := map[string]int{}
	for _, it := range x.b.items() {
		if it.Parent == x.s.ID && strings.HasPrefix(it.Title, "C") {
			made[it.Title]++
		}
	}
	if len(made) != creates {
		t.Errorf("%d of %d creates made their item: %v", len(made), creates, made)
	}
}

// Two clones of a v2 store each migrate it to v3 on their own (each service at its start: the same table and row
// added on both sides), write, and sync both ways; then a write on each still lands.
func TestTwoClonesMigratingOnTheirOwnMerge(t *testing.T) {
	bare := bareRemote(t)
	a := newClone(t, t0)
	da := a.setup()
	defer da.Shutdown()
	must(da.conn.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"))
	must(da.conn.ExecContext(ctx, "USE `"+dbName+"`"))
	v2 := append(append([]string{}, migrations[0]...), "UPDATE schema_version SET version = 2")
	for _, stmts := range [][]string{schema, v2} {
		if err := da.inTx("pm: a v2 store", false, func(tx *sql.Tx) error { return execAll(tx, stmts) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := da.AddRemote(bare); err != nil {
		t.Fatal(err)
	}
	if err := da.Push(); err != nil {
		t.Fatal(err)
	}
	b := newClone(t, a.clock)
	db := b.setup()
	defer db.Shutdown()
	if err := db.Clone(bare); err != nil { // B's setup migrates its clone to v3
		t.Fatal(err)
	}
	if err := da.migrate(); err != nil { // and A's service, at its next start, its own
		t.Fatal(err)
	}
	pa := must(da.Create(New{Type: Project, Title: "PA"}))
	pb := must(db.Create(New{Type: Project, Title: "PB"}))
	for _, d := range []*Dolt{da, db, da} {
		time.Sleep(dedup)
		if _, err := d.Sync(); err != nil {
			t.Fatal(err)
		}
	}
	if na, nb := len(must(da.Items())), len(must(db.Items())); na != 2 || nb != 2 {
		t.Fatalf("A holds %d items, B %d", na, nb)
	}
	must(da.Comment(pb.ID, Note, "a", "x"))
	must(db.Comment(pa.ID, Note, "b", "y"))
}
