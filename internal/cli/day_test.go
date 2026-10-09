package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/work"
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

	items := []work.Item{
		{ID: "repo-demo", Type: work.Project, Title: "Demo", CreatedAt: before},
		{ID: "repo-demo.2", Type: work.Sprint, Parent: "repo-demo", Title: "Sprint 2: Second", CreatedAt: before,
			Status: work.Closed, ClosedAt: at},
		{ID: "repo-demo.10", Type: work.Sprint, Parent: "repo-demo", Title: "Sprint 10: Tenth", CreatedAt: at},
		{ID: "repo-demo.2.10", Type: work.Task, Parent: "repo-demo.2", Title: "Late task", CreatedAt: at},
		{ID: "repo-demo.2.9", Type: work.Task, Parent: "repo-demo.2", Title: "Done task", CreatedAt: before,
			StartedAt: at, Status: work.Closed, ClosedAt: at, CloseReason: "commit abc"},
		{ID: "repo-demo.2.8", Type: work.Task, Parent: "repo-demo.2", Title: "Started task", CreatedAt: before,
			StartedAt: at},
		{ID: "repo-demo.2.7", Type: work.Need, Parent: "repo-demo.2", Title: "Run it", CreatedAt: at,
			Need: &work.NeedInfo{Kind: work.Action}},
		{ID: "repo-demo.2.6", Type: work.Need, Parent: "repo-demo.2", Title: "Dismissed", CreatedAt: at,
			Status: work.Closed, ClosedAt: at, Resolution: work.Dismissed, Need: &work.NeedInfo{Kind: work.Decision}},
		{ID: "repo-demo.2.5", Type: work.Task, Parent: "repo-demo.2", Title: "Old task", CreatedAt: before},
	}
	r := &repo{records: dir, items: items, x: records.NewItems(items), recs: recs}
	got, err := r.dayActivity(day)
	if err != nil {
		t.Fatal(err)
	}
	want := "project Demo\n" +
		"  sprint Sprint 2: Second\n" +
		"    sprint finished: Done: shipped.\n" +
		"    request to the owner (action) raised: Run it\n" +
		"    task started: Started task\n" +
		"    task closed: Done task (commit abc)\n" +
		"    task opened: Late task\n" +
		"  sprint Sprint 10: Tenth\n" +
		"    sprint opened\n" +
		"doc A note (docs/" + day + "-note)\n" +
		"records commit: records [docs/" + day + "-note.md, projects/demo.md, sprints/demo-2.md]"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got, err := r.dayActivity(at.AddDate(0, 0, -1).Format(time.DateOnly)); err != nil || got != "" {
		t.Errorf("a day without events: %q, %v", got, err)
	}
}
