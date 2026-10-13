package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// slowMarks is a store each of whose marks takes a while, on a clock of its own.
type slowMarks struct {
	*worktest.Store
	clock  time.Time
	mark   time.Duration
	marked []string
}

func (s *slowMarks) UpdateNeed(id string, u work.NeedUpdate) error {
	s.clock = s.clock.Add(s.mark)
	s.marked = append(s.marked, id)
	return nil
}

// TestInboxMarksWithinOneBudget: marking happens after the hook printed, so it must end well inside the hook's
// timeout; what is not marked by the budget's end stays undelivered and comes again, and the error says how many. Each
// write may wait for the write lock only as long as the budget has left.
func TestInboxMarksWithinOneBudget(t *testing.T) {
	fake := &slowMarks{Store: worktest.New(nil), clock: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		mark: 150 * time.Millisecond}
	var marks []Mark
	for _, id := range []string{"demo.1", "demo.2", "demo.3", "demo.4"} {
		marks = append(marks, Mark{id, work.NeedUpdate{}})
	}
	var waits []time.Duration
	err := MarkWithin(fake, marks, 400*time.Millisecond, func() time.Time { return fake.clock },
		func(left time.Duration) { waits = append(waits, left) })
	if strings.Join(fake.marked, " ") != "demo.1 demo.2 demo.3" || err == nil ||
		err.Error() != "1 of 4 left undelivered past 400ms" {
		t.Errorf("marked %v, %v", fake.marked, err)
	}
	if len(waits) != 3 || waits[0] != 400*time.Millisecond || waits[2] != 100*time.Millisecond {
		t.Errorf("lock waits %v", waits)
	}
}
