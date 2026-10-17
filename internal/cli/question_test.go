package cli

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// A session's question is no decision or action: pm need edit and pm decision add --need refuse it, naming pm need
// close, which refuses an answer for it and closes it as dismissed; pm show lists it with its kind, and a question asked
// outside any task at the top level.

// questionItems is a project, a sprint, a task, a question under the task and one under nothing.
func questionItems() []work.Item {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	ask := &work.AskInfo{Call: "toolu_1", Questions: []work.AskQuestion{{Text: "Ship it?",
		Options: []work.AskOption{{Label: "Yes"}, {Label: "No"}}}}}
	need := func(id, parent string) work.Item {
		return work.Item{ID: id, Type: work.Need, Parent: parent, Title: "Session s1 asks: Ship it?", Status: work.Open,
			CreatedAt: at, UpdatedAt: at, Need: &work.NeedInfo{Kind: work.Question, RaisedBy: &work.RaisedBy{Session: "s1"},
				Ask: ask}}
	}
	return []work.Item{
		{ID: "demo-pxy", Type: work.Project, Title: "P", Status: work.Open, CreatedAt: at, UpdatedAt: at},
		{ID: "demo-pxy.1", Type: work.Sprint, Parent: "demo-pxy", Title: "S", Number: 1, Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		{ID: "demo-pxy.1.1", Type: work.Task, Parent: "demo-pxy.1", Title: "T", Status: work.Open, CreatedAt: at,
			UpdatedAt: at},
		need("demo-pxy.1.1.1", "demo-pxy.1.1"),
		need("demo-ask1", ""),
	}
}

func TestAQuestionIsClosedOnlyAsDismissed(t *testing.T) {
	items := questionItems()
	if err := work.Check(items); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "records"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "records"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	e := &env{here: dir, records: dir, ws: worktest.New(items)}
	args := func(kv ...string) *Parsed {
		p := &Parsed{Values: map[string][]string{}}
		for i := 0; i < len(kv); i += 2 {
			p.Values[kv[i]] = []string{kv[i+1]}
		}
		return p
	}
	r := &repo{items: items, x: records.NewItems(items)}
	for _, id := range []string{"demo-pxy.1.1.1", "demo-ask1"} {
		_, _, _, addErr := r.decisionSource(args("need", id), "d\nr")
		_, editErr := cmdNeedEdit(e, args("need_id", id, "text", "a new body"))
		for name, err := range map[string]error{"decision add --need": addErr, "need edit": editErr} {
			if err == nil || !strings.Contains(err.Error(), "session's question") ||
				!strings.Contains(err.Error(), "pm need close "+id) {
				t.Errorf("pm %s %s: %v", name, id, err)
			}
		}
		if _, err := cmdNeedClose(e, args("need_id", id, "reason", "no rule", "text", "Yes")); err == nil ||
			!strings.Contains(err.Error(), "is of kind question; only a decision need closes with the owner's answer") {
			t.Errorf("pm need close --text %s: %v", id, err)
		}
		// pm need close without an answer dismisses it: it reaches the store's close, which this store refuses
		if _, err := cmdNeedClose(e, args("need_id", id, "reason", "moot")); err == nil ||
			!strings.Contains(err.Error(), "read-only") {
			t.Errorf("pm need close %s: %v", id, err)
		}
	}
}

func TestShowListsQuestionsAndThoseAskedOutsideAnyTask(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "me")
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CODEX_SESSION_ID", "")
	q := showNeed{sprint: "demo-pxy.1", task: "demo-pxy.1.1", id: "demo-pxy.1.1.1", title: "Session s1 asks: Ship it?",
		kind: "question", session: "s1"}
	d := showNeed{sprint: "demo-pxy.1", id: "demo-pxy.1.2", title: "Which?", kind: "decision"}
	p := &showProject{name: "p", bead: "demo-pxy", needs: []showNeed{q, d},
		sprints: []*showSprint{{id: "demo-pxy.1", name: "sprint 1"}}}
	if out := showProjectText(p, nil); !strings.Contains(out, "questions await you (1):\n  .1.1.1  Session s1 asks: "+
		"Ship it?  (sprint 1, task .1.1)") || !strings.Contains(out, "decisions await you (1):") {
		t.Errorf("project level:\n%s", out)
	}
	p.needs = []showNeed{d}
	if out := showProjectText(p, nil); !strings.Contains(out, "questions await you (0):") {
		t.Errorf("a project with no question:\n%s", out)
	}
	p.needs = []showNeed{q, d}
	asks := []string{"    question demo-ask1  Session s2 asks: Why?  (session s2)"}
	out := showText(&showData{site: "http://x", projects: []*showProject{p}, asks: asks})
	for _, want := range []string{"sessions ask you, outside any task:\n    question demo-ask1  Session s2 asks: Why?  " +
		"(session s2)\n", "    question .1.1.1  Session s1 asks: Ship it?"} {
		if !strings.Contains(out, want) {
			t.Errorf("top level lacks %q:\n%s", want, out)
		}
	}
}
