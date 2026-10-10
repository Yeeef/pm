package work

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// Moving a sprint, from the work-store page's Ids section (Moving a sprint): the id stays, the number is the new
// project's next, a move note keeps the old place, and the old project never mints the old number again.

func TestMoveSprintKeepsTheIdRenumbersAndNotes(t *testing.T) {
	d, _ := newStore(t)
	p, s1, task, need := seed(t, d)
	must(0, d.Claim(task.ID, Holder{Session: "sa"}, func(string) bool { return true }))
	s2 := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 2: S2"}))
	q := must(d.Create(New{Type: Project, Title: "Q"}))
	must(d.Create(New{Type: Sprint, Parent: q.ID, Title: "Sprint 1: Q1"}))
	before := commits(t, d)

	moved := must(d.MoveSprint(s2.ID, q.ID, "It belongs to Q.\nQ owns the parser."))
	if moved.ID != s2.ID || moved.Parent != q.ID || moved.Number != 2 || moved.Title != "Sprint 2: S2" {
		t.Errorf("moved %+v", moved)
	}
	if commits(t, d) != before+1 {
		t.Errorf("%d commits, want one more than %d", commits(t, d), before)
	}
	want := []SprintMove{{From: p.ID, FromNumber: 2, To: q.ID, ToNumber: 2, Reason: "It belongs to Q.\nQ owns the parser."}}
	if got := SprintMoves(&moved); !slices.Equal(got, want) {
		t.Errorf("moves %+v", got)
	}
	if got := get(t, d, s2.ID); got.Parent != q.ID || got.Number != 2 || len(got.Comments) != 1 ||
		got.Comments[0].Author != MoveAuthor || got.Comments[0].Kind != Note {
		t.Errorf("stored %+v", got)
	}

	// The old project never mints the moved number again; the new one counts the moved sprint.
	if s := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 3: S3"})); s.Number != 3 {
		t.Errorf("next sprint of P is %d, want 3: 2 was moved away", s.Number)
	}
	if s := must(d.Create(New{Type: Sprint, Parent: q.ID, Title: "Sprint 3: Q3"})); s.Number != 3 {
		t.Errorf("next sprint of Q is %d, want 3", s.Number)
	}

	// Moving s1, with its task and need, back and forth: the children are not written, the holder stays.
	at := get(t, d, task.ID)
	moved = must(d.MoveSprint(s1.ID, q.ID, "r\nr"))
	if moved.Number != 4 || moved.Title != "Sprint 4: S" {
		t.Errorf("moved %+v", moved)
	}
	moved = must(d.MoveSprint(s1.ID, p.ID, "back\nback"))
	if moved.Number != 4 || len(SprintMoves(&moved)) != 2 {
		t.Errorf("moved back %+v", moved)
	}
	if got := get(t, d, task.ID); got.Parent != s1.ID || got.Holder == nil || got.Holder.Session != "sa" ||
		!got.UpdatedAt.Equal(at.UpdatedAt) {
		t.Errorf("task %+v", got)
	}
	if got := get(t, d, need.ID); got.Parent != task.ID || !slices.Equal(got.Need.Review.Sprints, []string{s1.ID}) {
		t.Errorf("need %+v", got)
	}
	items := must(d.Items())
	if n := LastSprintNumber(slices.Values(ptrs(items)), q.ID); n != 4 {
		t.Errorf("Q's last number %d, want 4: s1 left it as 4", n)
	}

	// A move to where the sprint is writes nothing and returns it.
	before = commits(t, d)
	if again := must(d.MoveSprint(s1.ID, p.ID, "x\ny")); again.Number != 4 || commits(t, d) != before {
		t.Errorf("a repeated move wrote: %+v", again)
	}
}

func TestMoveSprintRefuses(t *testing.T) {
	d, _ := newStore(t)
	p, s, task, _ := seed(t, d)
	q := must(d.Create(New{Type: Project, Title: "Q"}))
	closedP := must(d.Create(New{Type: Project, Title: "C"}))
	must(0, d.Close(closedP.ID, "done", Done, ""))
	done := must(d.Create(New{Type: Sprint, Parent: p.ID, Title: "Sprint 2: done"}))
	must(0, d.Close(done.ID, "done", Done, ""))
	before := commits(t, d)
	for _, c := range []struct{ id, to, reason, want string }{
		{task.ID, q.ID, "r\nr", "only a sprint moves"},
		{s.ID, task.ID, "r\nr", "a sprint moves to a project"},
		{s.ID, "demo-none", "r\nr", "no item demo-none"},
		{s.ID, closedP.ID, "r\nr", "a sprint moves to an open project"},
		{done.ID, q.ID, "r\nr", "only an open sprint moves"},
		{s.ID, q.ID, " ", "a move needs a reason"},
	} {
		if _, err := d.MoveSprint(c.id, c.to, c.reason); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s to %s: %v, want %q", c.id, c.to, err, c.want)
		}
	}
	if commits(t, d) != before {
		t.Error("a refused move wrote")
	}
}

// A move note is read only from pm's own comments: a session's note in the same words is no move.
func TestSprintMovesReadsOnlyPmsNotes(t *testing.T) {
	text := "pm sprint move: from demo-a sprint 3 to demo-b sprint 1\nwhy"
	it := Item{Type: Sprint, Comments: []Comment{
		{Kind: Note, Author: "session-1", Text: text},
		{Kind: Reply, Author: MoveAuthor, Text: text},
		{Kind: Note, Author: MoveAuthor, Text: "pm sprint move: elsewhere"},
		{Kind: Note, Author: MoveAuthor, Text: text, CreatedAt: time.Now()},
	}}
	if got := SprintMoves(&it); len(got) != 1 || got[0] != (SprintMove{"demo-a", 3, "demo-b", 1, "why"}) {
		t.Fatalf("%+v", got)
	}
}

func ptrs(items []Item) []*Item {
	out := make([]*Item, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}
