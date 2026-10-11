package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// slowMarks is a store whose needs each hold an undelivered reply and whose every mark takes a while.
type slowMarks struct {
	*worktest.Store
	mark   time.Duration
	marked []string
}

func (s *slowMarks) UpdateNeed(id string, u work.NeedUpdate) error {
	time.Sleep(s.mark)
	s.marked = append(s.marked, id)
	return nil
}

// TestInboxMarksWithinOneBudget: marking happens after the hook printed, so it must end well inside the hook's
// timeout; what is not marked by the budget's end stays undelivered and comes again, and the error says how many.
func TestInboxMarksWithinOneBudget(t *testing.T) {
	root := clone(t)
	defer func(old string) { buildinfo.Version = old }(buildinfo.Version)
	buildinfo.Version = "9.9.9" // the pin clone writes
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var items []work.Item
	for _, id := range []string{"demo.1", "demo.2", "demo.3", "demo.4"} {
		items = append(items, work.Item{ID: id, Type: work.Need, Parent: "demo", Title: "Q?", Status: work.Open,
			CreatedAt: at, UpdatedAt: at, Need: &work.NeedInfo{Kind: work.Decision, RaisedBy: &work.RaisedBy{Session: "s"}},
			Comments: []work.Comment{{ID: "c" + id, Kind: work.Reply, Author: "owner", Text: "yes", CreatedAt: at}}})
	}
	fake := &slowMarks{Store: worktest.New(items), mark: 150 * time.Millisecond}
	defer func(old func(string) (work.Store, error)) { OpenWork = old }(OpenWork)
	OpenWork = func(string) (work.Store, error) { return fake, nil }
	defer func(old time.Duration) { inboxMarkBudget = old }(inboxMarkBudget)
	inboxMarkBudget = 400 * time.Millisecond
	texts, deliver, err := pendingReplies(root, "s")
	if err != nil || len(texts) != 4 {
		t.Fatalf("%d texts, %v", len(texts), err)
	}
	start := time.Now()
	err = deliver()
	if took := time.Since(start); took > inboxMarkBudget+200*time.Millisecond {
		t.Errorf("marking took %s, past its budget %s", took, inboxMarkBudget)
	}
	if len(fake.marked) == 0 || len(fake.marked) == 4 || err == nil ||
		!strings.Contains(err.Error(), "left undelivered past 400ms") {
		t.Errorf("marked %v, %v", fake.marked, err)
	}
}
