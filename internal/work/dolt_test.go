package work

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The store on embedded Dolt, from the work-store page's Storage section and the Store interface: every write one
// transaction and one Dolt commit that lands whole or not at all, every read checked, the claim a compare-and-set,
// the schema version, the gate.

// newStore is a new store in a temp dir with a clock that ticks a second per read, shut down at the test's end.
func newStore(t *testing.T) (*Dolt, Options) {
	t.Helper()
	dir := t.TempDir()
	clock := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	o := Options{Dir: filepath.Join(dir, "store", "work"), RunDir: filepath.Join(dir, "run"), Prefix: "demo",
		Now: func() time.Time { clock = clock.Add(time.Second); return clock }}
	d, err := CreateStore(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Shutdown() })
	return d, o
}

// must is v, panicking (failing the test) on err.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }

// commits is the number of Dolt commits in the store.
func commits(t *testing.T, d *Dolt) int {
	t.Helper()
	var n int
	if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_log").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seed is a project with one sprint, a task in it and a review need on the task.
func seed(t *testing.T, d *Dolt) (p, s, task, need Item) {
	t.Helper()
	p = must(d.Create(New{Type: Project, Title: "P"}))
	s = must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 1: S"}))
	task = must(d.Create(New{Type: Task, Parent: s.ID, Title: "T"}))
	need = must(d.Create(New{Type: Need, Parent: task.ID, Title: "N?", Need: &NeedInfo{Kind: Review,
		RaisedBy: &RaisedBy{Session: "s1", Inbox: "/tmp/s1.sock"}, Review: &ReviewInfo{PR: "https://x/pull/1",
			Sprints: []string{s.ID}, Focus: "f"}}}))
	return
}

func get(t *testing.T, d *Dolt, id string) Item {
	t.Helper()
	return must(d.Get(id))[0]
}

func TestCreateWritesThroughToALaterOpen(t *testing.T) {
	d, o := newStore(t)
	p := must(d.Create(New{Type: Project, Title: "P"}))
	s := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 1: S"}))
	task := must(d.Create(New{Type: Task, Parent: s.ID, Title: "T", Description: "d", Labels: []string{"b", "a", "b"}}))
	if task.ID != s.ID+".1" || s.ID != p.ID+".1" || s.Number != 1 {
		t.Fatalf("ids %s %s %s number %d", p.ID, s.ID, task.ID, s.Number)
	}
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	d2 := must(OpenStore(o))
	defer d2.Shutdown()
	if items := must(d2.Items()); len(items) != 3 {
		t.Fatalf("%d items", len(items))
	}
	got := get(t, d2, task.ID)
	if strings.Join(got.Labels, ",") != "a,b" || got.Description != "d" || !got.CreatedAt.Equal(task.CreatedAt) ||
		got.CreatedAt.Location() != time.UTC {
		t.Fatalf("%+v", got)
	}
	if n := commits(t, d2); n != 5 { // Dolt's init commit, the schema, and one commit per create
		t.Fatalf("%d commits", n)
	}
	var dirty int
	if err := d2.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_status").Scan(&dirty); err != nil || dirty != 0 {
		t.Fatalf("%d uncommitted tables (%v)", dirty, err)
	}
}

func TestAWriteThatBreaksAnInvariantLandsNothing(t *testing.T) {
	d, _ := newStore(t)
	_, _, task, _ := seed(t, d)
	before, n := must(d.Items()), commits(t, d)
	for i, err := range []error{
		d.Edit(task.ID, ptr(""), nil),                          // no title
		d.Move(task.ID, "demo-none"),                           // no such parent
		d.SetResolution(task.ID, Done),                         // open
		d.DepAdd(task.ID, "demo-none"),                         // no such blocker
		d.DepAdd(task.ID, task.ID),                             // itself
		d.DepRemove(task.ID, "demo-none"),                      // no such link
		d.Close(task.ID, "r", "finished", "s1"),                // no such resolution
		d.UpdateNeed(task.ID, NeedUpdate{Delivered: new(int)}), // a task
		d.Answer(task.ID, "yes"),                               // a task
		func() error { _, err := d.Create(New{Type: Task, Title: "orphan"}); return err }(),
		func() error { _, err := d.Create(New{Type: Sprint, Parent: task.ID, Title: "S"}); return err }(),
		func() error { _, err := d.Create(New{Type: Need, Parent: task.ID, Title: "N"}); return err }(),
		func() error { _, err := d.Comment(task.ID, "answer", "s1", "x"); return err }(),
	} {
		if err == nil {
			t.Errorf("write %d breaks an invariant and passed", i)
		}
	}
	after := must(d.Items())
	if commits(t, d) != n || len(after) != len(before) {
		t.Fatalf("a failed write landed: %d commits, was %d", commits(t, d), n)
	}
	for i := range before {
		if string(must(json.Marshal(before[i]))) != string(must(json.Marshal(after[i]))) {
			t.Errorf("%s changed", before[i].ID)
		}
	}
}

func TestEachWriteIsOneDoltCommit(t *testing.T) {
	d, _ := newStore(t)
	_, s, task, need := seed(t, d)
	n := commits(t, d)
	steps := []func() error{
		func() error { return d.Edit(task.ID, ptr("T2"), ptr("body")) },
		func() error { _, err := d.Comment(task.ID, Note, "s1", "a note"); return err },
		func() error { return d.DepAdd(task.ID, s.ID) },
		func() error { return d.DepRemove(task.ID, s.ID) },
		func() error { return d.UpdateNeed(need.ID, NeedUpdate{Delivered: ptr(2), ReviewMerged: ptr("abc")}) },
		func() error { return d.Answer(need.ID, "yes") },
	}
	for i, f := range steps {
		if err := f(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if got := commits(t, d); got != n+len(steps) {
		t.Fatalf("%d commits after %d writes from %d", got, len(steps), n)
	}
	got := get(t, d, task.ID)
	if got.Title != "T2" || got.Description != "body" || len(got.Comments) != 1 || got.Comments[0].Kind != Note ||
		got.Comments[0].Author != "s1" || len(got.BlockedBy) != 0 {
		t.Errorf("%+v", got)
	}
	a := get(t, d, need.ID)
	if a.Status != Closed || a.Resolution != Answered || a.CloseReason != "Responded" || len(a.Comments) != 1 ||
		a.Comments[0].Kind != Reply || a.Comments[0].Author != "owner" || a.Comments[0].Text != "yes" ||
		a.Need.Delivered != 2 || a.Need.Review.Merged != "abc" || a.Need.Review.Sprints[0] != s.ID ||
		a.Need.RaisedBy.Inbox != "/tmp/s1.sock" {
		t.Errorf("%+v %+v", a, a.Need)
	}
	if needs := must(d.Needs("s1")); len(needs) != 1 || needs[0].ID != need.ID || len(must(d.Needs("s2"))) != 0 {
		t.Error("Needs")
	}
	if _, err := d.Get(task.ID, "demo-none"); err == nil {
		t.Error("Get of a missing id passed")
	}
}

func TestClaimIsACompareAndSet(t *testing.T) {
	d, _ := newStore(t)
	_, _, task, _ := seed(t, d)
	alive := map[string]bool{"s1": true, "s2": true}
	live := func(s string) bool { return alive[s] }
	if err := d.Claim(task.ID, Holder{Session: "s1", Host: "mac"}, live); err != nil {
		t.Fatal(err)
	}
	first := get(t, d, task.ID)
	if first.Holder.Session != "s1" || first.Holder.Host != "mac" || first.StartedAt.IsZero() ||
		!first.StartedAt.Equal(first.Holder.ClaimedAt) {
		t.Fatalf("%+v", first)
	}
	if err := d.Claim(task.ID, Holder{Session: "s2"}, live); err == nil ||
		!strings.Contains(err.Error(), "live session s1") {
		t.Fatalf("a live holder was taken over: %v", err)
	}
	if err := d.Claim(task.ID, Holder{Session: "s1"}, live); err != nil { // the holder claims again
		t.Fatal(err)
	}
	alive["s1"] = false
	if err := d.Claim(task.ID, Holder{Session: "s2"}, live); err != nil { // s1 is not live: s2 takes it
		t.Fatal(err)
	}
	got := get(t, d, task.ID)
	if got.Holder.Session != "s2" || !got.StartedAt.Equal(first.StartedAt) {
		t.Fatalf("%+v; started_at must stay the first claim's", got)
	}
	if err := d.Release(task.ID, "s1"); err == nil {
		t.Fatal("s1 released s2's claim")
	}
	if err := d.Release(task.ID, "s2"); err != nil || get(t, d, task.ID).Holder != nil {
		t.Fatal(err)
	}
	if err := d.Claim(task.ID, Holder{Session: "s2"}, live); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(task.ID, "done it", Done, "s2"); err != nil {
		t.Fatal(err)
	}
	got = get(t, d, task.ID)
	if got.Holder != nil || got.Status != Closed || got.ClosedBy != "s2" || got.ClosedAt.IsZero() ||
		got.CloseReason != "done it" || got.Resolution != Done {
		t.Fatalf("close: %+v", got)
	}
	if d.Close(task.ID, "again", Done, "s2") == nil || d.Claim(task.ID, Holder{Session: "s2"}, live) == nil {
		t.Fatal("a closed item closed again or was claimed")
	}
	if err := d.SetResolution(task.ID, NoDecision); err != nil || get(t, d, task.ID).Resolution != NoDecision {
		t.Fatal(err)
	}
}

func TestCreateMintsIdsAndSprintNumbers(t *testing.T) {
	d, _ := newStore(t)
	p, s1, task, _ := seed(t, d)
	if !strings.HasPrefix(p.ID, "demo-") || len(p.ID) != len("demo-")+4 || CheckID(p.ID) != nil {
		t.Errorf("root id %s", p.ID)
	}
	s2 := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 2: S"}))
	other := must(d.Create(New{Type: Task, Parent: s1.ID, Title: "U"}))
	if s2.ID != p.ID+".2" || s2.Number != 2 || other.ID != s1.ID+".2" {
		t.Errorf("%s %d %s", s2.ID, s2.Number, other.ID)
	}
	// A moved task keeps its id, and its number is not minted again under its old parent.
	if err := d.Move(other.ID, s2.ID); err != nil {
		t.Fatal(err)
	}
	if got := get(t, d, other.ID); got.Parent != s2.ID || got.ID != s1.ID+".2" {
		t.Errorf("%+v", got)
	}
	if next := must(d.Create(New{Type: Task, Parent: s1.ID, Title: "V"})); next.ID != s1.ID+".3" {
		t.Errorf("next child %s", next.ID)
	}
	if next := must(d.Create(New{Type: Task, Parent: s2.ID, Title: "W"})); next.ID != s2.ID+".1" {
		t.Errorf("first child of s2 %s", next.ID)
	}
	if err := d.Move(s2.ID, task.ID); err == nil {
		t.Error("a sprint moved")
	}
	// A second project's sprints number from 1.
	q := must(d.Create(New{Type: Project, Title: "Q"}))
	if s := must(d.Create(New{Type: Sprint, Parent: q.ID, Title: "Sprint 1: Q"})); s.Number != 1 {
		t.Errorf("number %d", s.Number)
	}
}

func TestOpenRefusesANewerSchemaAndAMissingStore(t *testing.T) {
	d, o := newStore(t)
	if _, err := d.conn.ExecContext(ctx, "UPDATE schema_version SET version = ?", SchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', 'a newer pm')"); err != nil {
		t.Fatal(err)
	}
	d.Shutdown()
	if _, err := OpenStore(o); err == nil || !strings.Contains(err.Error(), "run pm upgrade") {
		t.Fatalf("a newer schema opened: %v", err)
	}
	if _, err := CreateStore(o); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("created a store over one: %v", err)
	}
	o.Dir = filepath.Join(t.TempDir(), "none")
	if _, err := OpenStore(o); err == nil || !strings.Contains(err.Error(), "pm init creates") {
		t.Fatalf("no store: %v", err)
	}
}

func TestReadsFailHardOnARowNoWriteMakes(t *testing.T) {
	d, _ := newStore(t)
	_, _, task, _ := seed(t, d)
	if err := d.Claim(task.ID, Holder{Session: "s1"}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	// A closed item with a holder, written past pm (a hand edit, a merge).
	if _, err := d.conn.ExecContext(ctx, "UPDATE items SET status = 'closed', resolution = 'done', "+
		"closed_at = '2026-10-08 12:00:00' WHERE id = ?", task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Items(); err == nil || !strings.Contains(err.Error(), task.ID+" is closed but held by s1") {
		t.Fatalf("read: %v", err)
	}
	if err := d.Edit(task.ID, ptr("x"), nil); err == nil {
		t.Fatal("wrote over a store that fails its invariants")
	}
	// The schema refuses what is no enum value.
	if _, err := d.conn.ExecContext(ctx, "UPDATE items SET status = 'blocked' WHERE id = ?", task.ID); err == nil {
		t.Fatal("the schema took status blocked")
	}
}

func TestImportGoesIntoAnEmptyStoreOnly(t *testing.T) {
	d, _ := newStore(t)
	items := tree()
	n := commits(t, d)
	if err := d.Import(items, "import"); err != nil {
		t.Fatal(err)
	}
	if commits(t, d) != n+1 || len(must(d.Items())) != len(items) {
		t.Fatal("the import is not one commit of every item")
	}
	if err := d.Import([]Item{item("d-new", Project, "")}, "again"); err == nil ||
		!strings.Contains(err.Error(), "empty store only") {
		t.Fatalf("imported into a store with items: %v", err)
	}
	d2, _ := newStore(t)
	bad := tree()
	bad[1].Parent = "d-none"
	if err := d2.Import(bad, "import"); err == nil || len(must(d2.Items())) != 0 {
		t.Fatal("an import that fails Check landed")
	}
}

func TestTheGateSerialisesOpensAndLogsEachWait(t *testing.T) {
	d, o := newStore(t)
	o.GateTimeout = 300 * time.Millisecond
	start := time.Now()
	if _, err := OpenStore(o); err == nil || !strings.Contains(err.Error(), filepath.Join(o.RunDir, GateFile)) {
		t.Fatalf("opened while another open held the gate: %v", err)
	}
	if waited := time.Since(start); waited < o.GateTimeout {
		t.Fatalf("gave up after %s", waited)
	}
	// A waiter gets the store as soon as the holder shuts down.
	o.GateTimeout = 10 * time.Second
	go func() { time.Sleep(200 * time.Millisecond); d.Shutdown() }()
	d2, err := OpenStore(o)
	if err != nil {
		t.Fatal(err)
	}
	d2.Shutdown()
	log := string(must(os.ReadFile(filepath.Join(o.RunDir, GateLogFile))))
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], fmt.Sprintf("pid=%d", os.Getpid())) {
		t.Fatalf("gate log, one line per open:\n%s", log)
	}
	var ms float64
	if _, err := fmt.Sscanf(lines[1][strings.Index(lines[1], "wait_ms="):], "wait_ms=%f", &ms); err != nil || ms < 150 {
		t.Fatalf("the second open waited %v ms (%v)", ms, err)
	}
}
