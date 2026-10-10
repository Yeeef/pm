package work

import (
	"fmt"
	"iter"
	"regexp"
	"strconv"
	"strings"
)

// Moving a sprint, as the work-store page's Ids section gives it (Moving a sprint): the id stays, the parent becomes
// the new project, the number is the new project's next, and a move note on the sprint keeps where it was, so the old
// name points at it and the old project never mints that number again.

// MoveAuthor is the author of a move note: pm itself, which is no session id and not the owner.
const MoveAuthor = "pm"

var moveLine = regexp.MustCompile(`^pm sprint move: from (\S+) sprint ([0-9]+) to (\S+) sprint ([0-9]+)$`)

// SprintMove is one move of a sprint, as its move note records it: from a project and its number there, to a project
// and its number there, and why.
type SprintMove struct {
	From       string
	FromNumber int
	To         string
	ToNumber   int
	Reason     string
}

// note is the move note's text: the move on its first line, then the reason.
func (m SprintMove) note() string {
	return fmt.Sprintf("pm sprint move: from %s sprint %d to %s sprint %d\n%s", m.From, m.FromNumber, m.To, m.ToNumber,
		m.Reason)
}

// SprintMoves is the sprint's moves, in the order its comments hold them: each comment of kind note by MoveAuthor
// whose first line is a move.
func SprintMoves(it *Item) []SprintMove {
	var out []SprintMove
	for _, c := range it.Comments {
		if c.Kind != Note || c.Author != MoveAuthor {
			continue
		}
		first, reason, _ := strings.Cut(c.Text, "\n")
		m := moveLine.FindStringSubmatch(first)
		if m == nil {
			continue
		}
		from, _ := strconv.Atoi(m[2])
		to, _ := strconv.Atoi(m[4])
		out = append(out, SprintMove{From: m[1], FromNumber: from, To: m[3], ToNumber: to, Reason: reason})
	}
	return out
}

// LastSprintNumber is the highest sprint number the project has used: its sprints' numbers and the numbers its move
// notes say were moved away from it. A sprint opened or moved into the project takes the next.
func LastSprintNumber(items iter.Seq[*Item], project string) int {
	n := 0
	for it := range items {
		if it.Type != Sprint {
			continue
		}
		if it.Parent == project {
			n = max(n, it.Number)
		}
		for _, m := range SprintMoves(it) {
			if m.From == project {
				n = max(n, m.FromNumber)
			}
		}
	}
	return n
}

// all is every item of the index, as pointers.
func (x *Index) all() iter.Seq[*Item] {
	return func(yield func(*Item) bool) {
		for i := range x.items {
			if !yield(&x.items[i]) {
				return
			}
		}
	}
}

// moveSprintLocal moves the sprint to the project in one write on this store copy: the parent, the next number there,
// the title's "Sprint <n>: " and the move note. A sprint under the project already is returned as it is, nothing
// written, so a rerun after a move whose push outcome was unknown is safe.
func (d *Dolt) moveSprintLocal(id, to, reason string) (Item, error) {
	var moved Item
	err := d.write(fmt.Sprintf("pm: move sprint %s to %s", id, to), func(x *Index) ([]Item, error) {
		it, err := one(x, id)
		if err != nil {
			return nil, err
		}
		if it.Type != Sprint {
			return nil, itemError(id, "is a %s; only a sprint moves to a project", it.Type)
		}
		p := x.Item(to)
		switch {
		case p == nil:
			return nil, fmt.Errorf("work store: no item %s to move a sprint to", to)
		case p.Type != Project:
			return nil, itemError(to, "is a %s; a sprint moves to a project", p.Type)
		case p.Status == Closed:
			return nil, itemError(to, "is closed; a sprint moves to an open project")
		case it.Parent == to:
			moved = it
			return nil, nil
		case it.Status == Closed:
			return nil, itemError(id, "is closed; only an open sprint moves")
		case strings.TrimSpace(reason) == "":
			return nil, itemError(id, "a move needs a reason")
		}
		m := SprintMove{From: it.Parent, FromNumber: it.Number, To: to, ToNumber: LastSprintNumber(x.all(), to) + 1,
			Reason: reason}
		cid, err := newUUID()
		if err != nil {
			return nil, err
		}
		now := d.now()
		if sprintTitle.MatchString(it.Title) {
			it.Title = sprintTitle.ReplaceAllLiteralString(it.Title, fmt.Sprintf("Sprint %d: ", m.ToNumber))
		}
		it.Parent, it.Number, it.UpdatedAt = to, m.ToNumber, now
		it.Comments = append(it.Comments, Comment{ID: cid, Kind: Note, Author: MoveAuthor, Text: m.note(), CreatedAt: now})
		moved = it
		return []Item{it}, nil
	})
	if err != nil {
		return Item{}, err
	}
	return moved, nil
}

// MoveSprint moves an open sprint to an open project and returns it as it is after (Store.MoveSprint). It mints a
// number in the project, so in a store with a remote it runs through the compare-and-swap, as a child create does
// (CALL pm_move_sprint(?), moveSprintShared).
func (d *Dolt) MoveSprint(id, to, reason string) (Item, error) {
	if _, ok, err := d.remoteURL(); err != nil {
		return Item{}, err
	} else if ok {
		return d.callMoveSprint(id, to, reason)
	}
	return d.moveSprintLocal(id, to, reason)
}

// sprintMoveSpec is pm_move_sprint's argument.
type sprintMoveSpec struct {
	ID, To, Reason string
}

// moveSprintShared is MoveSprint on a store with a remote, run by the pm service: the compare-and-swap of a create
// (shared), whose write is the move.
func (d *Dolt) moveSprintShared(s sprintMoveSpec) (Item, error) {
	return d.shared(casWrite{
		verb:  "move sprint " + s.ID,
		mints: "a sprint number",
		write: func() (Item, error) { return d.moveSprintLocal(s.ID, s.To, s.Reason) },
		unknown: func(Item) string {
			return "Nothing was merged here: run the same command again, which moves the sprint only if this push " +
				"did not land"
		},
		landed: func(Item) string {
			return "the move is on the remote; the next sync (pm sync) brings it, and the same command run again then " +
				"writes the records step alone"
		},
	})
}
