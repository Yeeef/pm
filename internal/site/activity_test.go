package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
)

const dayProject = "---\ntype: project\ntitle: Demo\nbead: repo-demo\n---\n\n## Goal\n\nA demo.\n"

const daySprint = "---\ntype: sprint\ntitle: Second\nbead: repo-demo.2\n---\n\n## Goal\n\nShip it.\n\n" +
	"## Delivery report\n\n### Outcome\n\nDone: shipped.\n\n### Against \"Done when\"\n\n- It works: met.\n"

func TestDayActivityHoldsOnlyThatDaysEvents(t *testing.T) {
	now := time.Now()
	at := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local)
	day, before := at.Format(time.DateOnly), at.AddDate(0, 0, -3)
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		// the commits land on day even when the test runs across midnight
		stamp := at.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "records")
	write("projects/demo.md", dayProject)
	write("sprints/demo-2.md", daySprint)
	write("docs/"+day+"-note.md", "---\ntype: doc\ntitle: A note\ndate: "+day+"\nproject: demo\n---\n\nText.\n")
	run("add", "-A")
	run("commit", "-qm", "records")
	write("days/"+day+".summary.json", "{}\n") // a commit of summaries alone is no activity
	run("add", "-A")
	run("commit", "-qm", "summarized")
	recs, err := records.Read(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	all := []item.Item{
		{ID: "repo-demo", Type: item.Project, Title: "Demo", CreatedAt: before},
		{ID: "repo-demo.2", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 2: Second", CreatedAt: before,
			Status: item.Closed, ClosedAt: at},
		{ID: "repo-demo.10", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 10: Tenth", CreatedAt: at},
		{ID: "repo-demo.2.10", Type: item.Task, Parent: "repo-demo.2", Title: "Late task", CreatedAt: at},
		{ID: "repo-demo.2.9", Type: item.Task, Parent: "repo-demo.2", Title: "Done task", CreatedAt: before,
			StartedAt: at, Status: item.Closed, ClosedAt: at, CloseReason: "commit abc"},
		{ID: "repo-demo.2.8", Type: item.Task, Parent: "repo-demo.2", Title: "Started task", CreatedAt: before,
			StartedAt: at},
		{ID: "repo-demo.2.7", Type: item.Need, Parent: "repo-demo.2", Title: "Run it", CreatedAt: at,
			Need: &item.NeedInfo{Kind: item.Action}},
		{ID: "repo-demo.2.6", Type: item.Need, Parent: "repo-demo.2", Title: "Dismissed", CreatedAt: at,
			Status: item.Closed, ClosedAt: at, Resolution: item.Dismissed, Need: &item.NeedInfo{Kind: item.Decision}},
		{ID: "repo-demo.2.5", Type: item.Task, Parent: "repo-demo.2", Title: "Old task", CreatedAt: before},
		{ID: "repo-demo.2.4", Type: item.Task, Parent: "repo-demo.2", Title: "Dropped task", CreatedAt: before,
			Status: item.Closed, ClosedAt: at, Resolution: item.Dismissed, CloseReason: "a library does it"},
		// proposals: one made today, one started today by its first task, one dropped today
		{ID: "repo-demo.11", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 11: Proposed", CreatedAt: at,
			Labels: []string{item.ProposalLabel}},
		{ID: "repo-demo.12", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 12: Started", CreatedAt: before,
			Labels: []string{item.ProposalLabel}},
		{ID: "repo-demo.12.1", Type: item.Task, Parent: "repo-demo.12", Title: "First", CreatedAt: at},
		{ID: "repo-demo.13", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 13: Dropped", CreatedAt: before,
			Labels: []string{item.ProposalLabel}, Status: item.Closed, ClosedAt: at, Resolution: item.Dismissed,
			CloseReason: "duplicate of sprint 11"},
		// started today by a task moved in from another sprint, made before the proposal: dated by its move note
		{ID: "repo-demo.14", Type: item.Sprint, Parent: "repo-demo", Title: "Sprint 14: Moved in", CreatedAt: before,
			Labels: []string{item.ProposalLabel}},
		{ID: "repo-demo.14.1", Type: item.Task, Parent: "repo-demo.14", Title: "Moved", CreatedAt: before.AddDate(0, 0, -1),
			Comments: []item.Comment{{Kind: item.Note, Author: item.MoveAuthor, CreatedAt: at,
				Text: item.TaskMoveText("repo-demo.2", "repo-demo.14")}}},
	}
	items := records.NewItems(all)
	got, err := DayActivity(day, dir, recs, items)
	if err != nil {
		t.Fatal(err)
	}
	want := "project Demo\n" +
		"  sprint Sprint 2: Second\n" +
		"    sprint finished: Done: shipped.\n" +
		"    task dropped: Dropped task (a library does it)\n" +
		"    request to the owner (action) raised: Run it\n" +
		"    task started: Started task\n" +
		"    task closed: Done task (commit abc)\n" +
		"    task opened: Late task\n" +
		"  sprint Sprint 10: Tenth\n" +
		"    sprint opened\n" +
		"  sprint Sprint 11: Proposed\n" +
		"    sprint proposed\n" +
		"  sprint Sprint 12: Started\n" +
		"    sprint started\n" +
		"    task opened: First\n" +
		"  sprint Sprint 13: Dropped\n" +
		"    proposal dropped (duplicate of sprint 11)\n" +
		"  sprint Sprint 14: Moved in\n" +
		"    sprint started\n" +
		"doc A note (docs/" + day + "-note)\n" +
		"records commit: records [docs/" + day + "-note.md, projects/demo.md, sprints/demo-2.md]"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got, err := DayActivity(at.AddDate(0, 0, -1).Format(time.DateOnly), dir, recs, items); err != nil || got != "" {
		t.Errorf("a day without events: %q, %v", got, err)
	}
}
