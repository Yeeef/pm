package work

import (
	"database/sql"
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
		time.Sleep(wait)
	}
}

func newClone(t *testing.T, clock time.Time) *clone {
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

func (c *clone) item(id string) Item {
	c.t.Helper()
	for _, it := range c.items() {
		if it.ID == id {
			return it
		}
	}
	c.t.Fatalf("no item %s", id)
	return Item{}
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

func TestSyncPushesUnderPmsOwnRefOnly(t *testing.T) {
	x := newPair(t)
	refs := gitRun(t, x.bare, "for-each-ref", "--format=%(refname)")
	if !strings.Contains(refs, RemoteRef+"\n") || strings.Contains(refs, "refs/dolt/data") ||
		strings.Contains(refs, "__dolt_remote_info__") {
		t.Fatalf("remote refs:\n%s", refs)
	}
	if got := x.b.items(); !reflect.DeepEqual(got, x.a.items()) || len(got) != 5 {
		t.Fatalf("the clone has %d items", len(got))
	}
}

func TestSyncWithoutARemoteFails(t *testing.T) {
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
}

func TestDoltRemoteURL(t *testing.T) {
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
}

// The 2-clone test of the sprint's Done when: concurrent edits on both clones merge per the table, and child mints on
// both clones get distinct ids. Each clone mints first (a create pulls and pushes at once), then edits offline; A
// syncs, and B's sync merges A's edits into its own.
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
}

func TestMergeCommentsUnionByID(t *testing.T) {
	x := newPair(t)
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

func TestMergeLabelsAndBlockersPerEntry(t *testing.T) {
	x := newPair(t)
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
}

// Close beats claim where Dolt merged cleanly: A closes the task while B claims it from no holder, both in the same
// second, so no cell conflicts; the merged item is closed with no holder.
func TestMergeCloseBeatsClaimOnACleanMerge(t *testing.T) {
	x := newPair(t)
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

// The same new id on both sides: two clones that mint a child without the compare-and-swap (as two stores with no
// remote would) fail the merge hard.
func TestMergeAddAddFailsHard(t *testing.T) {
	x := newPair(t)
	var ia, ib Item
	x.a.do(func(d *Dolt) { ia = must(d.createLocal(New{Type: Task, Parent: x.s.ID, Title: "A's"})) })
	x.b.do(func(d *Dolt) { ib = must(d.createLocal(New{Type: Task, Parent: x.s.ID, Title: "B's"})) })
	if ia.ID != ib.ID {
		t.Fatalf("ids %s %s", ia.ID, ib.ID)
	}
	x.a.sync()
	failedSync(t, x.b, "both sides added item "+ia.ID)
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

// A pull that would fast-forward (B has nothing of its own) onto a remote state that fails pm's checks: the remote's
// head is checked before the merge, so main never moves and nothing resets it.
func TestPullOntoARemoteHeadThatFailsItsCheckMovesNothing(t *testing.T) {
	x := newPair(t)
	x.rawPush(t, "INSERT INTO blocked_by VALUES ('"+x.t1.ID+"', '"+x.t2.ID+"'), ('"+x.t2.ID+"', '"+x.t1.ID+"')")
	failedSync(t, x.b, "blocked_by cycle")
	x.rawPush(t, "DELETE FROM blocked_by", fmt.Sprintf("UPDATE schema_version SET version = %d", SchemaVersion+1))
	failedSync(t, x.b, fmt.Sprintf("its schema is version %d, newer than this pm's", SchemaVersion+1))
}

func TestMergeCycleAcrossClonesFailsHard(t *testing.T) {
	x := newPair(t)
	x.a.do(func(d *Dolt) { must(0, d.DepAdd(x.t1.ID, x.t2.ID)) })
	x.b.do(func(d *Dolt) { must(0, d.DepAdd(x.t2.ID, x.t1.ID)) })
	x.a.sync()
	failedSync(t, x.b, "blocked_by cycle")
}

func TestMergeAConflictNoRuleSettlesFailsHard(t *testing.T) {
	x := newPair(t)
	x.b.clock = x.a.clock // both edit in the same second
	ta, tb := "A", "B"
	x.a.do(func(d *Dolt) { must(0, d.Edit(x.t1.ID, &ta, nil)) })
	x.b.do(func(d *Dolt) { must(0, d.Edit(x.t1.ID, &tb, nil)) })
	x.a.sync()
	failedSync(t, x.b, x.t1.ID+" field title")
}

// openA connects to clone A for the length of the test step, so its writes can land between B's mint and B's push.
func (x *pair) openA(t *testing.T) *Dolt {
	t.Helper()
	return x.a.s.dial(t)
}

// Concurrent child mints: B pulls and mints on pm-cas, and before its push A mints under the same parent and pushes;
// B's push is rejected, B drops pm-cas, pulls A's child and mints the next number. The same for a sprint's number.
// B runs the compare-and-swap in process (createShared, what its service runs for pm_create), so the test can hold
// its push; A's creates go through A's service (CALL pm_create).
func TestCreateConcurrentChildMintsGetDistinctIDs(t *testing.T) {
	x := newPair(t)
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
}

func TestCreateFailsAfterThreeRejectedPushes(t *testing.T) {
	x := newPair(t)
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
}

// A push that times out: the remote at the new commit means it landed; elsewhere, retry; a failing fetch drops pm-cas
// and fails with the outcome unknown, main untouched.
func TestCreatePushTimeout(t *testing.T) {
	x := newPair(t)
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
}

// A push the client dropped at its timeout lands on the remote only after the fetch that checked it (the server
// finishes the dropped statement): the create fails, the outcome unknown, and mints nothing again, so the item is on
// the remote once, and the next sync brings it here.
func TestCreateWhosePushLandsAfterItsCheckMakesTheItemOnce(t *testing.T) {
	x := newPair(t)
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
}

func TestCreateRefusesWithTheRemoteUnreachable(t *testing.T) {
	x := newPair(t)
	if err := os.Rename(x.bare, x.bare+".gone"); err != nil {
		t.Fatal(err)
	}
	x.a.do(func(d *Dolt) {
		head := must(d.head())
		_, err := d.Create(New{Type: Task, Parent: x.s.ID, Title: "offline"})
		// The pull's fetch fails, or, within Dolt's 1 s read dedup of the service's last fetch, reads nothing and
		// the push fails with an outcome the failing fetch cannot check: either way nothing is created.
		if err == nil || !strings.Contains(err.Error(), "minted only against the remote") &&
			!strings.Contains(err.Error(), "the outcome is unknown") || must(d.head()) != head {
			t.Fatal(err)
		}
		if _, err := d.Create(New{Type: Project, Title: "a root id needs no remote"}); err != nil {
			t.Fatal(err)
		}
	})
}
