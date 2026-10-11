package work

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Sync through a git remote, from the work-store page's Storage (Remote, Sync, Merge, Cycle check) and Ids (the
// child-id compare-and-swap): two clones of one store on a local bare repo, each served by its own host and reached
// per step as a pm command reaches it. The merge-table rows for list fields (comments, labels, blocked_by) and the rules Dolt's clean merge needs
// (close beats claim, the cycle check, add/add) are tested here; the rows on items fields in merge_test.go.

// gitRun runs git in dir and fails the test on an error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// bareRemote is a bare repo with one commit on main, as a repo's remote has: Dolt reaches a git remote only once it
// has a branch.
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", bare)
	gitRun(t, dir, "clone", "-q", bare, "w")
	gitRun(t, filepath.Join(dir, "w"), "commit", "-q", "--allow-empty", "-m", "first")
	gitRun(t, filepath.Join(dir, "w"), "push", "-q", "origin", "main")
	return bare
}

// clone is one clone's store with its own clock, ticking a second per read, served by its own host.
type clone struct {
	t      *testing.T
	o      Options
	mu     sync.Mutex
	clock  time.Time
	s      *served
	synced time.Time // when the clone last read the remote
}

// dedup is Dolt's read dedup of a git remote: a fetch within it of the service's last read of the remote reads
// nothing new. A pm command's own process opened the store anew, so it never met it; a service that holds the store
// does, and a test that wants a sync to see the other clone's latest push waits it out (fresh).
const dedup = 1100 * time.Millisecond

// fresh waits until the clone's next fetch reads the remote.
func (c *clone) fresh() {
	if wait := dedup - time.Since(c.synced); wait > 0 {
		time.Sleep(wait) //nolint:forbidigo // Dolt's read dedup of a git remote is a wall-clock window; no state shows it passed
	}
}

func newClone(t *testing.T, clock time.Time) *clone {
	t.Helper()
	c := &clone{t: t, clock: clock}
	c.o = Options{Prefix: "demo", Now: func() time.Time { // the clone's sessions read it at once
		c.mu.Lock()
		defer c.mu.Unlock()
		c.clock = c.clock.Add(time.Second)
		return c.clock
	}}
	c.s = host(t, shortMain(t), c.o, Ops{})
	return c
}

// setup is a connection to the clone's host with no database selected, as pm init's setup has.
func (c *clone) setup() *Dolt {
	c.t.Helper()
	d, err := dial(c.s.h.sock, dialConfig{prefix: c.o.Prefix, now: c.o.Now})
	if err != nil {
		c.t.Fatal(err)
	}
	return d
}

// do connects to the clone's service, runs fn and disconnects, as one pm command does.
func (c *clone) do(fn func(d *Dolt)) {
	c.t.Helper()
	d, err := dial(c.s.h.sock, dialConfig{prefix: c.o.Prefix, now: c.o.Now, withDB: true})
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() {
		if err := d.Shutdown(); err != nil {
			c.t.Fatal(err)
		}
	}()
	fn(d)
}

func (c *clone) sync() SyncResult {
	c.t.Helper()
	c.fresh()
	var r SyncResult
	c.do(func(d *Dolt) { r = must(d.Sync()) })
	c.synced = time.Now()
	return r
}

func (c *clone) items() []Item {
	c.t.Helper()
	var items []Item
	c.do(func(d *Dolt) { items = must(d.Items()) })
	return items
}

// pair is two clones of a store on a bare remote, seeded by A with a project, a sprint, two tasks and a decision need,
// and pushed; B cloned it after, and its clock starts where A's stopped.
type pair struct {
	a, b               *clone
	bare               string
	p, s, t1, t2, need Item
}

func newPair(t *testing.T) *pair {
	t.Helper()
	x := &pair{bare: bareRemote(t), a: newClone(t, t0)}
	d := x.a.setup()
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
	if err := d.AddRemote(x.bare); err != nil {
		t.Fatal(err)
	}
	x.p = must(d.Create(New{Type: Project, Title: "P"}))
	x.s = must(d.Create(New{Type: Sprint, Parent: x.p.ID, Title: "S"}))
	x.t1 = must(d.Create(New{Type: Task, Parent: x.s.ID, Title: "T1", Labels: []string{"keep", "drop"}}))
	x.t2 = must(d.Create(New{Type: Task, Parent: x.s.ID, Title: "T2"}))
	x.need = must(d.Create(New{Type: Need, Parent: x.t1.ID, Title: "N?", Need: &NeedInfo{Kind: Decision,
		RaisedBy: &RaisedBy{Session: "s1"}}}))
	must(d.Sync())
	x.a.synced = time.Now()
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	x.b = newClone(t, x.a.clock)
	db := x.b.setup()
	if err := db.Clone(x.bare); err != nil {
		t.Fatal(err)
	}
	x.b.synced = time.Now()
	if err := db.Shutdown(); err != nil {
		t.Fatal(err)
	}
	return x
}

// converge syncs A, B, then A, and checks both clones hold the same items; it returns them.
func (x *pair) converge(t *testing.T) []Item {
	t.Helper()
	x.a.sync()
	x.b.sync()
	x.a.sync()
	a, b := x.a.items(), x.b.items()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("the clones differ after sync:\nA %+v\nB %+v", a, b)
	}
	return a
}

func find(items []Item, id string) Item {
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	return Item{}
}

// on makes the pair's clones fail on t, a subtest's, until that subtest ends: a clone made for the parent test would
// otherwise fail the parent from inside the subtest.
func (x *pair) on(t *testing.T) {
	t.Helper()
	ta, tb := x.a.t, x.b.t
	x.a.t, x.b.t = t, t
	t.Cleanup(func() { x.a.t, x.b.t = ta, tb })
}

// reseed gives a case on a shared pair items of its own, its clones failing on the case's t (on): A makes a project, a
// sprint, two tasks and a decision need as newPair does and pushes them, B pulls them, and B's clock starts where A's
// stopped, as after newPair.
func (x *pair) reseed(t *testing.T) *pair {
	t.Helper()
	x.on(t)
	y := &pair{a: x.a, b: x.b, bare: x.bare}
	x.a.do(func(d *Dolt) {
		y.p = must(d.createLocal(New{Type: Project, Title: "P"}))
		y.s = must(d.createLocal(New{Type: Sprint, Parent: y.p.ID, Title: "S"}))
		y.t1 = must(d.createLocal(New{Type: Task, Parent: y.s.ID, Title: "T1", Labels: []string{"keep", "drop"}}))
		y.t2 = must(d.createLocal(New{Type: Task, Parent: y.s.ID, Title: "T2"}))
		y.need = must(d.createLocal(New{Type: Need, Parent: y.t1.ID, Title: "N?", Need: &NeedInfo{Kind: Decision,
			RaisedBy: &RaisedBy{Session: "s1"}}}))
	})
	x.a.do(func(d *Dolt) { // A holds the remote's head: each case ends converged, or with B reset to it
		if err := d.pushNow(branch); err != nil {
			t.Fatal(err)
		}
	})
	x.a.synced = time.Now() // the push read the remote: A's next fetch waits out the read dedup
	x.b.sync()
	x.b.clock = x.a.clock
	return y
}

// resetB moves B back to the remote's head after a case left it holding a merge that fails, so the next case starts
// from clones that sync.
func (x *pair) resetB(t *testing.T) {
	t.Helper()
	x.b.fresh()
	x.b.do(func(d *Dolt) {
		if _, err := d.fetch(); err != nil {
			t.Fatal(err)
		}
		if _, err := d.conn.ExecContext(ctx, "CALL DOLT_RESET('--hard', ?)", remoteHead); err != nil {
			t.Fatal(err)
		}
	})
	x.b.synced = time.Now()
}

// The 2-clone test of the sprint's Done when: concurrent edits on both clones merge per the table, and child mints on
// both clones get distinct ids. Each clone mints first (a create pulls and pushes at once), then edits offline; A
// syncs, and B's sync merges A's edits into its own. The other merge rules that need two clones run after it on the
// same pair, each case on items of its own (reseed).
func TestSyncTwoClonesConcurrentEditsMergePerTheTable(t *testing.T) {
	x := newPair(t)
	var ca, cb Item
	x.a.do(func(d *Dolt) {
		ca = must(d.Create(New{Type: Task, Parent: x.s.ID, Title: "child from A"}))
		title := "T1 from A"
		must(0, d.Edit(x.t1.ID, &title, nil))                                               // title: only A
		must(0, d.Claim(x.t2.ID, Holder{Session: "sa"}, func(string) bool { return true })) // claim at A's clock
		must(d.Comment(x.t1.ID, Note, "sa", "from A"))
		must(0, d.DepAdd(x.t2.ID, x.t1.ID))
	})
	x.b.clock = x.b.clock.Add(time.Minute) // B's writes are later
	x.b.do(func(d *Dolt) {
		cb = must(d.Create(New{Type: Task, Parent: x.s.ID, Title: "child from B"}))
		desc := "from B"
		must(0, d.Edit(x.t1.ID, nil, &desc))                                                 // description: only B
		must(0, d.Claim(x.t2.ID, Holder{Session: "sb"}, func(string) bool { return false })) // a later claim
		must(d.Comment(x.t1.ID, Note, "sb", "from B"))
		must(0, d.UpdateNeed(x.need.ID, NeedUpdate{Delivered: ptr(2)}))
		must(0, d.Close(x.need.ID, "answered", Answered, "sb"))
	})
	if ca.ID == cb.ID {
		t.Fatalf("both clones minted %s", ca.ID)
	}
	x.a.sync()
	r := x.b.sync()
	items := x.converge(t)
	t1, t2, need := find(items, x.t1.ID), find(items, x.t2.ID), find(items, x.need.ID)
	if t1.Title != "T1 from A" || t1.Description != "from B" {
		t.Errorf("t1 fields: %q %q", t1.Title, t1.Description)
	}
	var texts []string
	for _, c := range t1.Comments {
		texts = append(texts, c.Text)
	}
	if slices.Sort(texts); !slices.Equal(texts, []string{"from A", "from B"}) {
		t.Errorf("t1 comments: %v", texts)
	}
	if t2.Holder == nil || t2.Holder.Session != "sb" || !slices.Equal(t2.BlockedBy, []string{x.t1.ID}) {
		t.Errorf("t2: holder %+v blocked by %v", t2.Holder, t2.BlockedBy)
	}
	if r.Resolved != 2 || len(r.Overrides) != 1 || r.Overrides[0].ID != x.t2.ID || r.Overrides[0].Lost.Session != "sa" {
		t.Errorf("B's pull: %d rows resolved, overrides %+v", r.Resolved, r.Overrides)
	}
	if need.Status != Closed || need.Resolution != Answered || need.Need.Delivered != 2 {
		t.Errorf("need: %+v %+v", need, need.Need)
	}
	if find(items, ca.ID).Title != "child from A" || find(items, cb.ID).Title != "child from B" {
		t.Errorf("children %s %s", ca.ID, cb.ID)
	}

	t.Run("MergeCommentsUnionByID", func(t *testing.T) {
		x := x.reseed(t)
		var a1, a2, b1 Comment
		x.a.do(func(d *Dolt) {
			a1 = must(d.Comment(x.t1.ID, Note, "sa", "a1"))
			a2 = must(d.Comment(x.t1.ID, Note, "sa", "a2"))
		})
		x.b.do(func(d *Dolt) { b1 = must(d.Comment(x.t1.ID, Reply, "owner", "b1")) }) // the same position as a1
		items := x.converge(t)
		var ids []string
		for _, c := range find(items, x.t1.ID).Comments {
			ids = append(ids, c.ID)
		}
		want := []string{a1.ID, a2.ID, b1.ID}
		if slices.Sort(ids); !slices.Equal(ids, slices.Sorted(slices.Values(want))) {
			t.Fatalf("comments %v, want %v", ids, want)
		}
		x.b.do(func(d *Dolt) { must(d.Comment(x.t1.ID, Note, "sb", "after")) }) // a write after the merge appends
		if cs := find(x.converge(t), x.t1.ID).Comments; len(cs) != 4 || cs[3].Text != "after" {
			t.Fatalf("%+v", cs)
		}
	})

	t.Run("MergeLabelsAndBlockersPerEntry", func(t *testing.T) {
		x := x.reseed(t)
		x.a.do(func(d *Dolt) {
			setLabels(t, d, x.t1.ID, "keep", "a-added") // A removes drop, adds a-added
			must(0, d.DepAdd(x.t2.ID, x.t1.ID))
		})
		x.b.do(func(d *Dolt) {
			setLabels(t, d, x.t1.ID, "keep", "drop", "b-added") // B leaves drop as in the base, adds b-added
			must(0, d.DepAdd(x.t2.ID, x.need.ID))
		})
		items := x.converge(t)
		if l := find(items, x.t1.ID).Labels; !slices.Equal(l, []string{"a-added", "b-added", "keep"}) {
			t.Errorf("labels %v", l)
		}
		if b := find(items, x.t2.ID).BlockedBy; !slices.Equal(b, []string{x.t1.ID, x.need.ID}) {
			t.Errorf("blocked_by %v", b)
		}
		// One side removes a blocker the other left as in the base: it goes.
		x.a.do(func(d *Dolt) { must(0, d.DepRemove(x.t2.ID, x.t1.ID)) })
		x.b.do(func(d *Dolt) { must(d.Comment(x.t2.ID, Note, "sb", "untouched blockers")) })
		if b := find(x.converge(t), x.t2.ID).BlockedBy; !slices.Equal(b, []string{x.need.ID}) {
			t.Errorf("blocked_by after a removal %v", b)
		}
	})

	// Close beats claim where Dolt merged cleanly: A closes the task while B claims it from no holder, both in the same
	// second, so no cell conflicts; the merged item is closed with no holder.
	t.Run("MergeCloseBeatsClaimOnACleanMerge", func(t *testing.T) {
		x := x.reseed(t)
		x.b.clock = x.a.clock
		x.a.do(func(d *Dolt) { must(0, d.Close(x.t2.ID, "done", Done, "sa")) })
		x.b.do(func(d *Dolt) { must(0, d.Claim(x.t2.ID, Holder{Session: "sb"}, func(string) bool { return false })) })
		var r SyncResult
		x.a.sync()
		x.b.fresh()
		x.b.do(func(d *Dolt) { r = must(d.Sync()) })
		if r.Resolved != 0 {
			t.Fatalf("the merge had %d conflicted rows; the test wants a clean one", r.Resolved)
		}
		t2 := find(x.converge(t), x.t2.ID)
		if t2.Status != Closed || t2.Holder != nil || t2.StartedAt.IsZero() {
			t.Fatalf("%+v", t2)
		}
	})

	t.Run("CreateFailsAfterThreeRejectedPushes", func(t *testing.T) {
		x := x.reseed(t)
		n := 0
		da := x.openA(t)
		x.b.do(func(d *Dolt) {
			head, before := must(d.head()), must(d.Items())
			d.pushFn = func(push func() error) error {
				n++
				must(da.Create(New{Type: Task, Parent: x.s.ID, Title: "A"}))
				return push()
			}
			_, err := d.createShared(New{Type: Task, Parent: x.s.ID, Title: "B"})
			if err == nil || !strings.Contains(err.Error(), "no push landed in 3 attempts, the last: the remote moved") {
				t.Fatal(err)
			}
			// Each attempt pulled what A pushed, so the store moved; but B's own item is nowhere.
			if must(d.head()) == head || len(must(d.Items())) != len(before)+2 {
				t.Fatalf("items %d, before %d", len(must(d.Items())), len(before))
			}
		})
		if err := da.Shutdown(); err != nil {
			t.Fatal(err)
		}
		if n != 3 {
			t.Fatalf("%d pushes", n)
		}
		for _, it := range x.converge(t) {
			if it.Title == "B" {
				t.Fatalf("B's item landed: %s", it.ID)
			}
		}
	})

	// A sprint moved on one clone while the other writes under it (the work-store page, Moving a sprint, Another clone):
	// B claims a task, edits the sprint and comments on it offline, then moves the sprint to Q through the
	// compare-and-swap; before B's push, A opens a sprint in Q, adds a task under the sprint, and comments on and renames
	// the sprint itself, all through A's service.
	// B's push is rejected, B pulls A's writes and moves again: the sprint takes Q's next number, A's task lands in the
	// moved sprint, every offline write survives, and the old project does not mint the moved number again.
	t.Run("MoveSprintAcrossClonesKeepsTheOtherClonesWrites", func(t *testing.T) {
		x := x.reseed(t)
		da := x.openA(t)
		q := must(da.Create(New{Type: Project, Title: "Q"}))
		x.a.sync()
		x.b.sync()
		var qa, ta, moved Item
		x.b.do(func(d *Dolt) {
			must(0, d.Claim(x.t1.ID, Holder{Session: "sb"}, func(string) bool { return false }))
			desc := "from B"
			must(0, d.Edit(x.s.ID, nil, &desc))
			must(d.Comment(x.s.ID, Note, "sb", "from B"))
			raced := false
			d.pushFn = func(push func() error) error {
				if !raced {
					raced = true
					qa = must(da.Create(New{Type: Sprint, Parent: q.ID, Title: "Sprint 1: QA"}))
					ta = must(da.Create(New{Type: Task, Parent: x.s.ID, Title: "from A"}))
					must(da.Comment(x.s.ID, Note, "sa", "from A")) // A writes the sprint's own row too
					title := "S from A"
					must(0, da.Edit(x.s.ID, &title, nil))
				}
				return push()
			}
			moved = must(d.moveSprintShared(sprintMoveSpec{ID: x.s.ID, To: q.ID, Reason: "r\nr"}))
		})
		x.b.synced = time.Now() // the move read the remote: B's next fetch waits out the read dedup
		if qa.Number != 1 || moved.Number != 2 || moved.Parent != q.ID {
			t.Fatalf("A's sprint #%d, the moved sprint #%d under %s", qa.Number, moved.Number, moved.Parent)
		}
		items := x.converge(t)
		s, t1, task := find(items, x.s.ID), find(items, x.t1.ID), find(items, ta.ID)
		if s.Parent != q.ID || s.Number != 2 || s.Description != "from B" || s.Title != "S from A" || len(s.Comments) != 3 ||
			!slices.Equal(SprintMoves(&s), []SprintMove{{x.p.ID, 1, q.ID, 2, "r\nr"}}) {
			t.Errorf("sprint %+v", s)
		}
		if t1.Holder == nil || t1.Holder.Session != "sb" || task.Parent != x.s.ID {
			t.Errorf("t1 holder %+v, A's task under %s", t1.Holder, task.Parent)
		}
		if next := must(da.Create(New{Type: Sprint, Parent: x.p.ID, Title: "Sprint 2: P"})); next.Number != 2 {
			t.Errorf("P's next sprint #%d, want 2: 1 was moved away", next.Number)
		}
		if err := da.Shutdown(); err != nil {
			t.Fatal(err)
		}
		x.converge(t)
	})
}

// setLabels replaces an item's labels by SQL, as one commit: pm sets labels only at create, so the test edits rows.
func setLabels(t *testing.T, d *Dolt, id string, labels ...string) {
	t.Helper()
	if err := d.inTx("test: labels", true, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM labels WHERE item_id = ?", id); err != nil {
			return err
		}
		for _, l := range labels {
			if _, err := tx.ExecContext(ctx, "INSERT INTO labels VALUES (?, ?)", id, l); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// failedSync syncs c, wants the sync to fail with want, and checks the store kept its commit and items.
func failedSync(t *testing.T, c *clone, want string) {
	t.Helper()
	c.fresh()
	c.do(func(d *Dolt) {
		head, before := must(d.head()), must(d.Items())
		_, err := d.Sync()
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "the store stays as it was") {
			t.Fatalf("want a failure with %q, got %v", want, err)
		}
		if h := must(d.head()); h != head || !reflect.DeepEqual(must(d.Items()), before) {
			t.Fatalf("the failed sync moved the store from %s to %s", head, h)
		}
		var dirty int
		if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_status").Scan(&dirty); err != nil || dirty != 0 {
			t.Fatalf("%d uncommitted tables after the failed sync (%v)", dirty, err)
		}
	})
	c.synced = time.Now()
}

// rawPush commits stmts on A by SQL, past pm's checks (a state an older or broken pm could push), and pushes.
func (x *pair) rawPush(t *testing.T, stmts ...string) {
	t.Helper()
	x.a.do(func(d *Dolt) {
		if err := d.inTx("test: raw", false, func(tx *sql.Tx) error { return execAll(tx, stmts) }); err != nil {
			t.Fatal(err)
		}
		if err := d.pushNow(branch); err != nil {
			t.Fatal(err)
		}
	})
}

// openA connects to clone A for the length of the test step, so its writes can land between B's mint and B's push.
func (x *pair) openA(t *testing.T) *Dolt {
	t.Helper()
	return x.a.s.dial(t)
}

// A push that times out: the remote at the new commit means it landed; elsewhere, retry; a failing fetch drops pm-cas
// and fails with the outcome unknown, main untouched. The other cases of the compare-and-swap, the remote and the
// merges that fail hard run on the same pair: first the two that count the pair's items, then each on items of its own
// (reseed); the cases that leave B a merge that fails reset B after them, and the one that pushes a broken remote head
// runs last. The case with a store of its own runs before the pair starts: Dolt panics in a host still running when
// another host of the process closes ("attempt to reference a closed spoolingTableReaderAt").
func TestCreatePushTimeout(t *testing.T) {
	t.Run("DoltRemoteURL", func(t *testing.T) {
		for in, want := range map[string]string{
			"/srv/x.git":                       "git+file:///srv/x.git",
			"file:///srv/x.git":                "git+file:///srv/x.git",
			"https://github.com/o/r.git":       "git+https://github.com/o/r.git",
			"ssh://git@github.com/o/r.git":     "git+ssh://git@github.com/o/r.git",
			"git@github.com:o/r.git":           "git+ssh://git@github.com/o/r.git",
			"git+ssh://git@github.com/o/r.git": "git+ssh://git@github.com/o/r.git",
		} {
			if got, err := DoltRemoteURL(in); err != nil || got != want {
				t.Errorf("%s: %s %v", in, got, err)
			}
		}
		if _, err := DoltRemoteURL("r.git"); err == nil {
			t.Error("a relative path passed")
		}
	})

	t.Run("SyncWithoutARemoteFails", func(t *testing.T) {
		d, _ := newStore(t)
		if _, err := d.Sync(); err == nil || !strings.Contains(err.Error(), "no remote") {
			t.Fatal(err)
		}
		bare := bareRemote(t)
		if err := d.AddRemote(bare); err != nil {
			t.Fatal(err)
		}
		if err := d.AddRemote(bare); err == nil {
			t.Fatal("a second remote was added")
		}
	})

	x := newPair(t)

	t.Run("SyncPushesUnderPmsOwnRefOnly", func(t *testing.T) {
		x.on(t)
		refs := gitRun(t, x.bare, "for-each-ref", "--format=%(refname)")
		if !strings.Contains(refs, RemoteRef+"\n") || strings.Contains(refs, "refs/dolt/data") ||
			strings.Contains(refs, "__dolt_remote_info__") {
			t.Fatalf("remote refs:\n%s", refs)
		}
		if got := x.b.items(); !reflect.DeepEqual(got, x.a.items()) || len(got) != 5 {
			t.Fatalf("the clone has %d items", len(got))
		}
	})

	// Concurrent child mints: B pulls and mints on pm-cas, and before its push A mints under the same parent and pushes;
	// B's push is rejected, B drops pm-cas, pulls A's child and mints the next number. The same for a sprint's number.
	// B runs the compare-and-swap in process (createShared, what its service runs for pm_create), so the test can hold
	// its push; A's creates go through A's service (CALL pm_create).
	t.Run("CreateConcurrentChildMintsGetDistinctIDs", func(t *testing.T) {
		x.on(t)
		var ia, ib, sa, sb Item
		da := x.openA(t)
		race := func(mintA func()) func(push func() error) error {
			raced := false
			return func(push func() error) error {
				if !raced {
					raced = true
					mintA()
				}
				return push()
			}
		}
		x.b.do(func(d *Dolt) {
			d.pushFn = race(func() { ia = must(da.Create(New{Type: Task, Parent: x.s.ID, Title: "A"})) })
			ib = must(d.createShared(New{Type: Task, Parent: x.s.ID, Title: "B"}))
			d.pushFn = race(func() { sa = must(da.Create(New{Type: Sprint, Parent: x.p.ID, Title: "SA"})) })
			sb = must(d.createShared(New{Type: Sprint, Parent: x.p.ID, Title: "SB"}))
		})
		if err := da.Shutdown(); err != nil {
			t.Fatal(err)
		}
		if ia.ID == ib.ID || ib.ID != x.s.ID+".4" || ia.ID != x.s.ID+".3" {
			t.Fatalf("task ids A %s B %s", ia.ID, ib.ID)
		}
		if sa.ID == sb.ID || sa.Number == sb.Number || sb.Number != 3 {
			t.Fatalf("sprints A %s #%d B %s #%d", sa.ID, sa.Number, sb.ID, sb.Number)
		}
		items := x.converge(t)
		if len(items) != 9 {
			t.Fatalf("%d items", len(items))
		}
	})

	t.Run("CreatePushTimeout", func(t *testing.T) {
		x := x.reseed(t)
		timeout := func(landed bool) func(push func() error) error {
			return func(push func() error) error {
				if landed {
					if err := push(); err != nil {
						return err
					}
				}
				return errPushTimeout
			}
		}
		x.b.do(func(d *Dolt) {
			d.pushFn = timeout(true)
			must(d.createShared(New{Type: Task, Parent: x.s.ID, Title: "landed"}))
			d.pushFn = nil
			if r := must(d.Sync()); r.Pushed != 0 {
				t.Fatalf("pushed %d after a landed create", r.Pushed)
			}
			calls, head := 0, must(d.head())
			d.pushFn = func(push func() error) error { // a push that times out and has not landed when checked
				calls++
				return errPushTimeout
			}
			_, err := d.createShared(New{Type: Task, Parent: x.s.ID, Title: "lost"})
			want := "the push may still land as " + x.s.ID + ".4. Nothing was merged here: run pm sync, then check with " +
				"pm show " + x.s.ID + ".4 before you create it again"
			if err == nil || !strings.Contains(err.Error(), "the remote's history does not hold it yet: the outcome is "+
				"unknown") || !strings.Contains(err.Error(), want) || calls != 1 || must(d.head()) != head {
				t.Fatalf("%d pushes: %v", calls, err)
			}
			d.pushFn = nil
		})
		x.b.do(func(d *Dolt) {
			head := must(d.head())
			d.pushFn = func(push func() error) error {
				// The fetch that checks the outcome fails: the store loses its remote. (Taking the remote away would not
				// do: a real timeout comes after PushTimeout, but here the fetch follows the pull within Dolt's 1 s read
				// dedup, and a deduped fetch reads no remote.)
				if _, err := d.conn.ExecContext(ctx, "CALL DOLT_REMOTE('remove', ?)", remote); err != nil {
					t.Fatal(err)
				}
				return errPushTimeout
			}
			_, err := d.createShared(New{Type: Task, Parent: x.s.ID, Title: "unknown"})
			if err == nil || !strings.Contains(err.Error(), "the outcome is unknown") || must(d.head()) != head {
				t.Fatalf("%v", err)
			}
			d.pushFn = nil
			if err := d.AddRemote(x.bare); err != nil {
				t.Fatal(err)
			}
		})
		titles := map[string]bool{}
		for _, it := range x.converge(t) {
			titles[it.Title] = true
		}
		if !titles["landed"] || titles["lost"] || titles["unknown"] {
			t.Fatalf("%v", titles)
		}
	})

	// A push the client dropped at its timeout lands on the remote only after the fetch that checked it (the server
	// finishes the dropped statement): the create fails, the outcome unknown, and mints nothing again, so the item is on
	// the remote once, and the next sync brings it here.
	t.Run("CreateWhosePushLandsAfterItsCheckMakesTheItemOnce", func(t *testing.T) {
		x := x.reseed(t)
		x.b.do(func(d *Dolt) {
			calls := 0
			d.pushFn = func(push func() error) error {
				calls++
				// the push the client dropped: C1, held to land later
				if _, err := d.conn.ExecContext(ctx, "CALL DOLT_BRANCH('held', 'pm-cas')"); err != nil {
					t.Fatal(err)
				}
				return errPushTimeout
			}
			_, err := d.createShared(New{Type: Task, Parent: x.s.ID, Title: "once"})
			if err == nil || !strings.Contains(err.Error(), "the outcome is unknown") || calls != 1 {
				t.Fatalf("%d pushes: %v", calls, err)
			}
			// the dropped push lands now, after the check
			if err := d.remoteCall(ctx, "CALL DOLT_PUSH(?, ?)", remote, "held:main"); err != nil {
				t.Fatal(err)
			}
		})
		var ids []string
		for _, it := range x.converge(t) {
			if it.Title == "once" {
				ids = append(ids, it.ID)
			}
		}
		if len(ids) != 1 {
			t.Fatalf("the item is there %d times: %v", len(ids), ids)
		}
	})

	// Every fetch the host's Dolt runs into the remote's cache repo, under the store's .dolt, finishes the auto
	// maintenance it starts before it returns. git 2.47 on detaches that maintenance by default, and a detached one
	// drops objects/maintenance.lock while a clone walks .dolt, where Dolt's walk panics on the file gone ("clone:
	// invalid connection"). git's trace2 events name each maintenance a fetch starts, with --detach when it detaches.
	t.Run("TheHostsFetchLeavesNoMaintenanceRunning", func(t *testing.T) {
		trace := filepath.Join(t.TempDir(), "git-trace2")
		t.Setenv("GIT_TRACE2_EVENT", trace)
		x := x.reseed(t) // A pushes, B pulls (a fetch into B's cache repo)
		x.b.do(func(d *Dolt) { must(d.Create(New{Type: Task, Parent: x.s.ID, Title: "T3"})) })
		b, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		argv, hierarchy := map[string][]string{}, map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var e struct {
				Event, SID, Hierarchy string
				Argv                  []string
			}
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatalf("%v: %s", err, line)
			}
			switch e.Event {
			case "start":
				argv[e.SID] = e.Argv
			case "cmd_name":
				hierarchy[e.SID] = e.Hierarchy
			}
		}
		started, detached := 0, 0
		for sid, h := range hierarchy {
			if h != "fetch/maintenance" {
				continue
			}
			started++
			if slices.Contains(argv[sid], "--detach") {
				detached++
			}
		}
		if started == 0 {
			t.Fatal("no fetch started its auto maintenance; git's trace2 logged none")
		}
		if detached > 0 {
			t.Fatalf("%d of %d auto maintenances the host's fetches started were detached", detached, started)
		}
		t.Logf("%d auto maintenances started by a fetch, none detached", started)
		x.a.sync() // B's create pushed: A takes it, so the next case's reseed pushes from the remote's head
	})

	// The same new id on both sides: two clones that mint a child without the compare-and-swap (as two stores with no
	// remote would) fail the merge hard.
	t.Run("MergeAddAddFailsHard", func(t *testing.T) {
		x := x.reseed(t)
		var ia, ib Item
		x.a.do(func(d *Dolt) { ia = must(d.createLocal(New{Type: Task, Parent: x.s.ID, Title: "A's"})) })
		x.b.do(func(d *Dolt) { ib = must(d.createLocal(New{Type: Task, Parent: x.s.ID, Title: "B's"})) })
		if ia.ID != ib.ID {
			t.Fatalf("ids %s %s", ia.ID, ib.ID)
		}
		x.a.sync()
		failedSync(t, x.b, "both sides added item "+ia.ID)
		x.resetB(t)
	})

	t.Run("MergeCycleAcrossClonesFailsHard", func(t *testing.T) {
		x := x.reseed(t)
		x.a.do(func(d *Dolt) { must(0, d.DepAdd(x.t1.ID, x.t2.ID)) })
		x.b.do(func(d *Dolt) { must(0, d.DepAdd(x.t2.ID, x.t1.ID)) })
		x.a.sync()
		failedSync(t, x.b, "blocked_by cycle")
		x.resetB(t)
	})

	t.Run("MergeAConflictNoRuleSettlesFailsHard", func(t *testing.T) {
		x := x.reseed(t)
		x.b.clock = x.a.clock // both edit in the same second
		ta, tb := "A", "B"
		x.a.do(func(d *Dolt) { must(0, d.Edit(x.t1.ID, &ta, nil)) })
		x.b.do(func(d *Dolt) { must(0, d.Edit(x.t1.ID, &tb, nil)) })
		x.a.sync()
		failedSync(t, x.b, x.t1.ID+" field title")
		x.resetB(t)
	})

	// A pull that would fast-forward (B has nothing of its own) onto a remote state that fails pm's checks: the remote's
	// head is checked before the merge, so main never moves and nothing resets it. It leaves the remote broken, so it
	// runs last.
	t.Run("PullOntoARemoteHeadThatFailsItsCheckMovesNothing", func(t *testing.T) {
		x := x.reseed(t)
		x.rawPush(t, "INSERT INTO blocked_by VALUES ('"+x.t1.ID+"', '"+x.t2.ID+"'), ('"+x.t2.ID+"', '"+x.t1.ID+"')")
		failedSync(t, x.b, "blocked_by cycle")
		x.rawPush(t, "DELETE FROM blocked_by", fmt.Sprintf("UPDATE schema_version SET version = %d", SchemaVersion+1))
		failedSync(t, x.b, fmt.Sprintf("its schema is version %d, newer than this pm's", SchemaVersion+1))
	})
}

// With the remote unreachable, a child create refuses at its pull, in milliseconds, and creates nothing. A's service
// starts anew first, as one does after its last read of the remote is past Dolt's read dedup (fresh): within it the
// pull's fetch reads nothing, and the push's own fetch, which Dolt's git remote retries 32 times over about 9 s, fails
// with an outcome unknown, the case TestCreatePushTimeout and its neighbours hold.
func TestCreateRefusesWithTheRemoteUnreachable(t *testing.T) {
	x := newPair(t)
	if err := os.Rename(x.bare, x.bare+".gone"); err != nil {
		t.Fatal(err)
	}
	if err := x.a.s.h.Close(); err != nil {
		t.Fatal(err)
	}
	x.a.s = host(t, x.a.s.main, x.a.o, Ops{})
	x.a.do(func(d *Dolt) {
		head := must(d.head())
		start := time.Now()
		_, err := d.Create(New{Type: Task, Parent: x.s.ID, Title: "offline"})
		if err == nil || !strings.Contains(err.Error(), "a child id is minted only against the remote") ||
			must(d.head()) != head {
			t.Fatal(err)
		}
		if took := time.Since(start); took > 3*time.Second { // milliseconds; Dolt's push retries take about 9 s
			t.Fatalf("the refusal took %s", took)
		}
		if _, err := d.Create(New{Type: Project, Title: "a root id needs no remote"}); err != nil {
			t.Fatal(err)
		}
	})
}
