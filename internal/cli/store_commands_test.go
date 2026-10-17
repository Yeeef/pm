package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// The work-store commands of the work-store page's Commands table, run as pm runs them (parse, connect to the service
// that holds the store, run, disconnect) on a store a test host serves. Every refusal leaves the store as it was; every happy path writes what it says.

type fixture struct {
	t                                       *testing.T
	main                                    string // the clone the host serves
	p, s1, s2, t1, t2, blocked, held, stale work.Item
	loose, closed, need                     work.Item
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// newFixture is a store with a project, sprints 1 and 2, and tasks in each state the ready rules distinguish; the
// session "me" runs the commands, "alive" is a live session and "gone" a session that is not.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("CODEX_HOME", filepath.Join(dir, "codex"))
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CODEX_SESSION_ID", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "me")
	if err := os.MkdirAll(filepath.Join(cfg, "projects", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	for sid, age := range map[string]time.Duration{"alive": time.Minute, "gone": 2 * session.LiveWindow} {
		p := filepath.Join(cfg, "projects", "x", sid+".jsonl")
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, time.Now().Add(-age), time.Now().Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	main, err := os.MkdirTemp("", "pm") //nolint:usetesting // short: the socket must fit the kernel's limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(main) })
	f := &fixture{t: t, main: main}
	worktest.ServeAt(t, main, work.Ops{Sync: func(c context.Context, d *work.Dolt) ([]string, error) {
		return syncStore(c, d, main, "origin")
	}})
	d := must(f.open())
	defer d.Shutdown()
	add := func(typ work.Type, parent, title string) work.Item {
		return must(d.Create(work.New{Type: typ, Parent: parent, Title: title}))
	}
	f.p = add(work.Project, "", "P")
	f.s1 = add(work.Sprint, f.p.ID, "S1")
	f.s2 = add(work.Sprint, f.p.ID, "S2")
	f.t2 = add(work.Task, f.s2.ID, "in sprint 2")
	f.t1 = add(work.Task, f.s1.ID, "in sprint 1")
	f.blocked = add(work.Task, f.s1.ID, "blocked")
	f.held = add(work.Task, f.s1.ID, "held by a live session")
	f.stale = add(work.Task, f.s1.ID, "held by a gone session")
	f.loose = add(work.Task, f.p.ID, "under the project")
	f.closed = add(work.Task, f.s1.ID, "closed")
	f.need = must(d.Create(work.New{Type: work.Need, Parent: f.t1.ID, Title: "N?",
		Need: &work.NeedInfo{Kind: work.Decision, RaisedBy: &work.RaisedBy{Session: "alive"}}}))
	yes := func(string) bool { return true }
	if err := d.DepAdd(f.blocked.ID, f.t2.ID); err != nil {
		t.Fatal(err)
	}
	for id, sid := range map[string]string{f.held.ID: "alive", f.stale.ID: "gone", f.t2.ID: "me"} {
		if err := d.Claim(id, work.Holder{Session: sid}, yes); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Release(f.t2.ID, "me"); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(f.closed.ID, "done", work.Done, "me"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) open() (*work.Dolt, error) {
	return work.DialSock(work.Sock(f.main), f.main, work.Options{Prefix: "demo"})
}

// run runs pm <argv> with stdin, as the tree routes it to its store command or a form, and gives its stdout and error.
func (f *fixture) run(stdin string, argv ...string) (string, error) {
	f.t.Helper()
	name, args, ok := storeFormOf(argv)
	for n := min(2, len(argv)); !ok && n > 0; n-- {
		if _, ok = storeCommands[strings.Join(argv[:n], " ")]; ok {
			name, args = strings.Join(argv[:n], " "), argv[n:]
		}
	}
	if !ok {
		f.t.Fatalf("%v is no store command", argv)
	}
	var out bytes.Buffer
	err := runStoreCommand(name, args, f.open, strings.NewReader(stdin), &out)
	return out.String(), err
}

func (f *fixture) items() []work.Item {
	f.t.Helper()
	d := must(f.open())
	defer d.Shutdown()
	return must(d.Items())
}

func (f *fixture) get(id string) work.Item {
	f.t.Helper()
	for _, it := range f.items() {
		if it.ID == id {
			return it
		}
	}
	f.t.Fatalf("no item %s", id)
	return work.Item{}
}

// refuses runs argv, wants an error containing want, and checks the store did not change.
func (f *fixture) refuses(want string, argv ...string) {
	f.t.Helper()
	before := f.items()
	out, err := f.run("", argv...)
	if err == nil || !strings.Contains(err.Error(), want) {
		f.t.Fatalf("pm %v: want a refusal with %q, got %v (stdout %q)", argv, want, err, out)
	}
	if !reflect.DeepEqual(f.items(), before) {
		f.t.Fatalf("pm %v refused but changed the store", argv)
	}
}

func TestTaskReadyListsReadyTasksInOrder(t *testing.T) {
	f := newFixture(t)
	out := must(f.run("", "task", "ready"))
	want := f.t1.ID + "  in sprint 1\n" +
		f.stale.ID + "  held by a gone session  (stale holder gone, claimed "
	if !strings.HasPrefix(out, want) {
		t.Fatalf("got\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || lines[2] != f.t2.ID+"  in sprint 2" || lines[3] != f.loose.ID+"  under the project" {
		t.Fatalf("got\n%s", out)
	}
	out = must(f.run("", "task", "ready", "--sprint", f.s2.ID, "--json"))
	var got []work.Item
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 1 || got[0].ID != f.t2.ID {
		t.Fatalf("%s %v", out, err)
	}
	if out := must(f.run("", "task", "ready", "--sprint", f.s1.ID, "--json")); !strings.HasPrefix(out, "[{") {
		t.Fatal(out)
	}
	f.refuses("names no sprint", "task", "ready", "--sprint", f.t1.ID)
	f.refuses("takes 0 argument", "task", "ready", f.s1.ID)
}

func TestTaskEdit(t *testing.T) {
	f := newFixture(t)
	if out := must(f.run("-- a body that starts with a dash\n", "task", "edit", f.t1.ID, "--title", "New",
		"--text-file", "-")); out != "edited "+f.t1.ID+"\n" {
		t.Fatal(out)
	}
	if it := f.get(f.t1.ID); it.Title != "New" || it.Description != "-- a body that starts with a dash" {
		t.Fatalf("%+v", it)
	}
	must(f.run("", "task", "edit", f.t1.ID, "--text=only the description"))
	if it := f.get(f.t1.ID); it.Title != "New" || it.Description != "only the description" {
		t.Fatalf("%+v", it)
	}
	f.refuses("nothing to edit", "task", "edit", f.t1.ID)
	f.refuses("is a sprint, not a task", "task", "edit", f.s1.ID, "--title", "x")
	f.refuses("no item demo-zzzz", "task", "edit", "demo-zzzz", "--title", "x")
	f.refuses("--title is empty", "task", "edit", f.t1.ID, "--title", " ")
	f.refuses("not both", "task", "edit", f.t1.ID, "--text", "a", "--text-file", "-")
	f.refuses("--text-file - is empty", "task", "edit", f.t1.ID, "--text-file", "-")
}

func TestTaskRelease(t *testing.T) {
	f := newFixture(t)
	d := must(f.open())
	if err := d.Claim(f.t1.ID, work.Holder{Session: "me"}, session.Live); err != nil {
		t.Fatal(err)
	}
	d.Shutdown()
	if out := must(f.run("", "task", "release", f.t1.ID)); out != "released "+f.t1.ID+"\n" {
		t.Fatal(out)
	}
	if it := f.get(f.t1.ID); it.Holder != nil {
		t.Fatalf("%+v", it.Holder)
	}
	f.refuses("is not held by me", "task", "release", f.held.ID)
	f.refuses("is not held by me", "task", "release", f.t1.ID)
	f.refuses("is a need, not a task", "task", "release", f.need.ID)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	f.refuses("no session", "task", "release", f.held.ID)
	t.Run("Live", func(t *testing.T) { // on the fixture's sessions: "alive" live, "gone" not
		if !session.Live("alive") || session.Live("gone") || session.Live("nobody") || session.Live("../x") {
			t.Fatal("liveness")
		}
		codex := filepath.Join(os.Getenv("CODEX_HOME"), "sessions", "2026", "10", "08")
		if err := os.MkdirAll(codex, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(codex, "rollout-2026-10-08T12-00-00-th1.jsonl"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if !session.Live("th1") {
			t.Fatal("a Codex thread's rollout is not live")
		}
	})
}

func TestDepAddAndRm(t *testing.T) {
	f := newFixture(t)
	if out := must(f.run("", "dep", "add", f.t1.ID, "--on", f.loose.ID)); out != f.t1.ID+" now waits on "+f.loose.ID+"\n" {
		t.Fatal(out)
	}
	if it := f.get(f.t1.ID); !reflect.DeepEqual(it.BlockedBy, []string{f.loose.ID}) {
		t.Fatalf("%v", it.BlockedBy)
	}
	f.refuses("blocked_by cycle", "dep", "add", f.loose.ID, "--on", f.t1.ID)
	f.refuses("blocked_by cycle", "dep", "add", f.s1.ID, "--on", f.t1.ID) // a task cannot block its own sprint
	f.refuses("no item demo-zzzz", "dep", "add", f.t1.ID, "--on", "demo-zzzz")
	f.refuses("blocked by "+f.loose.ID+" already", "dep", "add", f.t1.ID, "--on", f.loose.ID)
	f.refuses("--on BLOCKER is required", "dep", "add", f.t1.ID)
	if out := must(f.run("", "dep", "rm", f.t1.ID, "--on", f.loose.ID)); out != f.t1.ID+" no longer waits on "+f.loose.ID+"\n" {
		t.Fatal(out)
	}
	if it := f.get(f.t1.ID); len(it.BlockedBy) != 0 {
		t.Fatalf("%v", it.BlockedBy)
	}
	f.refuses("is not blocked by", "dep", "rm", f.t1.ID, "--on", f.loose.ID)
}

func TestSyncCommand(t *testing.T) {
	f := newFixture(t)
	f.refuses("no remote", "sync")
	dir := t.TempDir()
	bare := filepath.Join(dir, "origin.git")
	for _, args := range [][]string{{"init", "-q", "--bare", "-b", "main", bare}, {"clone", "-q", bare, "w"},
		{"-C", "w", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"},
		{"-C", "w", "push", "-q", "origin", "main"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	d := must(f.open())
	if err := d.AddRemote(bare); err != nil {
		t.Fatal(err)
	}
	d.Shutdown()
	out := must(f.run("", "sync"))
	if !strings.HasPrefix(out, "synced: pulled 0 commits, resolved 0 items both sides changed, pushed ") {
		t.Fatal(out)
	}
	if out := must(f.run("", "sync")); out != "synced: pulled 0 commits, resolved 0 items both sides changed, pushed 0 commits\n" {
		t.Fatal(out)
	}
}

func TestShowIDPrintsTheItemItsHolderBlockersChildrenNeedsAndComments(t *testing.T) {
	f := newFixture(t)
	if _, err := f.run("", "comment", "add", f.t1.ID, "--text", "a note\non two lines"); err != nil {
		t.Fatal(err)
	}
	out := must(f.run("", "show", f.t1.ID))
	for _, want := range []string{
		f.t1.ID + "  task  open  in sprint 1\nparent: " + f.s1.ID + "  sprint  open  S1\n",
		"\nneeds:\n  " + f.need.ID + "  need  open  N?\n",
		"\ncomments:\n", " me (note)\n    a note\n    on two lines\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pm show %s lacks %q:\n%s", f.t1.ID, want, out)
		}
	}
	held := must(f.run("", "show", f.held.ID))
	if !strings.Contains(held, "holder: session alive (live), claimed ") {
		t.Errorf("held:\n%s", held)
	}
	stale := must(f.run("", "show", f.stale.ID))
	if !strings.Contains(stale, "holder: session gone (not live), claimed ") {
		t.Errorf("stale:\n%s", stale)
	}
	blocked := must(f.run("", "show", f.blocked.ID))
	if !strings.Contains(blocked, "blocked by: "+f.t2.ID+"  task  open  in sprint 2\n") {
		t.Errorf("blocked:\n%s", blocked)
	}
	sprint := must(f.run("", "show", f.s1.ID))
	if !strings.Contains(sprint, "\nchildren:\n") || !strings.Contains(sprint, "  "+f.closed.ID+"  task  closed (done)  closed\n") {
		t.Errorf("sprint:\n%s", sprint)
	}
	var got work.Item
	if err := json.Unmarshal([]byte(must(f.run("", "show", f.t2.ID, "--json"))), &got); err != nil || got.ID != f.t2.ID {
		t.Fatalf("--json: %v %+v", err, got)
	}
	f.refuses("no item demo-nope in the work store", "show", "demo-nope")
	if _, _, ok := storeFormOf([]string{"show", "--sprint", f.s1.ID}); ok {
		t.Error("pm show --sprint is the command tree's")
	}
}

func TestTaskAddParentAddsASubTaskUnderAnOpenTask(t *testing.T) {
	f := newFixture(t)
	out := must(f.run("the body\n", "task", "add", "--task", f.t1.ID, "--title", "part one", "--text-file", "-"))
	id := f.t1.ID + ".2" // .1 is the need under it
	if out != "created task "+id+" under task "+f.t1.ID+"; claim it with pm task claim "+id+"\n" {
		t.Fatalf("out = %q", out)
	}
	if it := f.get(id); it.Type != work.Task || it.Parent != f.t1.ID || it.Title != "part one" || it.Description != "the body" {
		t.Fatalf("made %+v", it)
	}
	f.refuses("is a sprint, not a task", "task", "add", "--task", f.s1.ID, "--title", "x")
	f.refuses("task "+f.closed.ID+" is closed", "task", "add", "--task", f.closed.ID, "--title", "x")
	f.refuses("--title is required", "task", "add", "--task", f.t1.ID)
	if _, _, ok := storeFormOf([]string{"task", "add", "--sprint", f.s1.ID, "--title", "x"}); ok {
		t.Error("pm task add --sprint is the command tree's")
	}
}
