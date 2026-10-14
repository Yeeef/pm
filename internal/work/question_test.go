package work

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The question need: a session's ask-tool call, raised through Create with NewQuestion under a task or under nothing,
// its ask written once and read back whole, and closed once: of two closes the first wins, the second fails with
// ErrClosed. And the schema's version 4, which a v3 store migrates to.

func TestAQuestionIsRaisedClosedOnceAndKeepsItsAsk(t *testing.T) {
	d, _ := newStore(t)
	_, _, task, _ := seed(t, d)
	by := RaisedBy{Session: "0123456789abcdef", Inbox: "/run/s.sock", Host: "mac"}
	q := must(d.Create(NewQuestion(task.ID, by, *askOf())))
	if q.Parent != task.ID || !strings.HasPrefix(q.ID, task.ID+".") || q.Title != "Session 01234567 asks: Ship it today?" {
		t.Fatalf("raised %s under %s: %q", q.ID, q.Parent, q.Title)
	}
	for _, want := range []string{"**Ship**: Ship it today?\n- Yes: merge now\n- No: wait for CI",
		"**Checks**: Which checks? (choose one or more)\n- lint\n- go: the Go tests"} {
		if !strings.Contains(q.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, q.Description)
		}
	}
	got := get(t, d, q.ID)
	if !reflect.DeepEqual(got.Need.Ask, askOf()) || *got.Need.RaisedBy != by || got.Need.Kind != Question {
		t.Fatalf("read back %+v %+v", got.Need, got.Need.Ask)
	}

	// A session that holds no task asks under nothing: a root id, as a project's.
	lone := must(d.Create(NewQuestion("", RaisedBy{Session: "s2"}, AskInfo{Call: "call_9",
		Questions: []AskQuestion{{Text: "Free text?"}}})))
	if lone.Parent != "" || strings.Contains(lone.ID, ".") {
		t.Fatalf("a question with no parent got %s under %q", lone.ID, lone.Parent)
	}
	if g := get(t, d, lone.ID); len(g.Need.Ask.Questions) != 1 || len(g.Need.Ask.Questions[0].Options) != 0 {
		t.Fatalf("free-text question read back as %+v", g.Need.Ask)
	}
	if _, err := d.Create(New{Type: Need, Title: "N", Need: &NeedInfo{Kind: Decision}}); err == nil ||
		!strings.Contains(err.Error(), "needs a parent") {
		t.Fatalf("a decision need with no parent: %v", err)
	}

	// pm export carries the ask, and its import reads it back as it was.
	var out bytes.Buffer
	if err := Export(&out, must(d.Items())); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"ask":{"call":"toolu_01","questions":[{"text":"Ship it today?","header":"Ship",`+
		`"multi":false,"options":[{"label":"Yes","detail":"merge now"}`) {
		t.Fatalf("export: %s", out.String())
	}
	if back := find(must(FromExport(bytes.NewReader(out.Bytes()))), q.ID); !reflect.DeepEqual(back.Need.Ask, askOf()) {
		t.Fatalf("import: %+v", back.Need)
	}

	// The ask is written once.
	err := d.update("change the ask of", q.ID, func(it *Item, _ *Index) error {
		it.Need.Ask.Questions[0].Options[0].Detail = "merge tomorrow"
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "written once") {
		t.Fatalf("an ask changed: %v", err)
	}

	// First close wins; a later close, as an answer or a dismissal, fails with ErrClosed and writes nothing.
	must(0, d.Close(q.ID, "answered on the site", Answered, by.Session))
	n := commits(t, d)
	for name, err := range map[string]error{
		"answer":  d.Answer(q.ID, "Yes", by.Session),
		"dismiss": d.Close(q.ID, "the session ended", Dismissed, ""),
	} {
		if !errors.Is(err, ErrClosed) || !strings.Contains(err.Error(), q.ID+" is closed already") {
			t.Errorf("%s after the close: %v", name, err)
		}
	}
	if g := get(t, d, q.ID); commits(t, d) != n || g.Resolution != Answered || g.CloseReason != "answered on the site" {
		t.Fatalf("after the refused closes: %d commits (was %d), %s %q", commits(t, d), n, g.Resolution, g.CloseReason)
	}
	must(0, d.Answer(lone.ID, "the answer in chat", "s2"))
	if err := d.Close(lone.ID, "x", Dismissed, ""); !errors.Is(err, ErrClosed) {
		t.Fatalf("a dismissal after the answer: %v", err)
	}
	if g := get(t, d, lone.ID); g.Resolution != Answered || g.Comments[0].Text != "the answer in chat" {
		t.Fatalf("%+v", g)
	}
}

// A v3 store migrates to 4 keeping its items: need_kind takes question, behind a CHECK still in force, and a question
// can then be raised.
func TestMigratingAV3StoreKeepsItsItemsAndTakesQuestions(t *testing.T) {
	c := newClone(t, t0)
	d := c.setup()
	defer d.Shutdown() //nolint:errcheck // cleanup at the test's end; the test checks its own writes
	must(d.conn.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"))
	must(d.conn.ExecContext(ctx, "USE `"+dbName+"`"))
	v3 := append(append(append([]string{}, migrations[0]...), migrations[1]...), "UPDATE schema_version SET version = 3")
	rows := []string{
		`INSERT INTO items (id, type, title, description, status, created_at, updated_at) VALUES ('demo-abc', 'project',
			'P', '', 'open', '2026-10-08 12:00:00', '2026-10-08 12:00:00')`,
		`INSERT INTO items (id, type, parent, title, description, status, created_at, updated_at, need_kind,
			raised_session, delivered) VALUES ('demo-abc.1', 'need', 'demo-abc', 'N?', 'd', 'open', '2026-10-08 12:00:01',
			'2026-10-08 12:00:01', 'decision', 's1', 0)`,
	}
	for _, stmts := range [][]string{schema, v3, rows} {
		if err := d.inTx("pm: a v3 store", false, func(tx *sql.Tx) error { return execAll(tx, stmts) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.migrate(); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := d.conn.QueryRowContext(ctx, "SELECT version FROM schema_version").Scan(&v); err != nil || v != 4 {
		t.Fatalf("version %d (%v)", v, err)
	}
	if need := get(t, d, "demo-abc.1"); need.Need == nil || need.Need.Kind != Decision || need.Need.Ask != nil ||
		need.Need.RaisedBy.Session != "s1" || need.Parent != "demo-abc" {
		t.Fatalf("the need after the migration: %+v", need)
	}
	q := must(d.Create(NewQuestion("demo-abc.1", RaisedBy{Session: "s1"}, *askOf())))
	if q.ID != "demo-abc.1.1" || !reflect.DeepEqual(get(t, d, q.ID).Need.Ask, askOf()) {
		t.Fatalf("question %s", q.ID)
	}
	var create, table string
	if err := d.conn.QueryRowContext(ctx, "SHOW CREATE TABLE items").Scan(&table, &create); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(create, "CHECK ((`need_kind`"); n != 1 || !strings.Contains(create, "`items_need_kind` CHECK") {
		t.Fatalf("%d CHECKs on need_kind:\n%s", n, create)
	}
	_, err := d.conn.ExecContext(ctx, fmt.Sprintf(`INSERT INTO items (id, type, parent, title, description, status,
		created_at, updated_at, need_kind, delivered) VALUES ('demo-abc.2', 'need', 'demo-abc', 'x', '', 'open', %s, %s,
		'riddle', 0)`, "'2026-10-08 12:00:02'", "'2026-10-08 12:00:02'"))
	if err == nil || !strings.Contains(err.Error(), "items_need_kind") {
		t.Fatalf("need_kind riddle: %v", err)
	}
}
