package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// decisionBody checks a decision body: two lines or more, no block opened.
func decisionBody(body string) error {
	if body == "" {
		return refuse("the decision body is empty; pass it with %s: %s", textForms, decisionShape)
	}
	n := 0
	for _, l := range records.SplitLines(body) {
		if strip(l) != "" {
			n++
		}
	}
	if n < 2 {
		return refuse("the decision body is a single line; %s", decisionShape)
	}
	if regexp.MustCompile(`(?m)^:::`).MatchString(body) {
		return refuse("the decision body has a line starting with ':::', and blocks cannot nest; %s", decisionShape)
	}
	return nil
}

// decisionBlock is a ::: decision block of today, from source.
func decisionBlock(source, body string) string { return decisionBlockOn(source, store.Today(), body) }

func decisionBlockOn(source, date, body string) string {
	return fmt.Sprintf("::: decision {source=%s date=%s}\n%s\n:::", source, date, body)
}

func cmdTaskMove(e *env, p *Parsed) (string, error) {
	reason := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id, to := p.Get("task_id"), p.Get("to")
	task, err := r.openTask(id)
	if err != nil {
		return "", err
	}
	// The scope change is a decision in the sprint the task leaves, or, for a task filed directly under a project,
	// in the sprint it joins, which must be one of that project's.
	source := task.Parent
	var rec *records.Record
	fromProject := false
	for _, x := range r.recs {
		if source != "" && x.Bead() == source && (x.Type() == "sprint" || x.Type() == "project") {
			rec, fromProject = x, x.Type() == "project"
		}
	}
	if rec == nil {
		return "", refuse("%s is not in a sprint or a project with a record, so no sprint can record the scope change", id)
	}
	if to == source && fromProject {
		return "", refuse("%s is already directly under project %s; move it into one of that project's open sprints",
			id, rec.Name())
	}
	if to == source {
		return "", refuse("%s is already in sprint %s", id, source)
	}
	dest, err := r.openSprint(to)
	if err != nil {
		return "", err
	}
	if fromProject {
		if sp := r.item(to); sp.Parent != source {
			return "", refuse("sprint %s is not in project %s, which holds %s; move it into one of that project's open "+
				"sprints", to, rec.Name(), id)
		}
	}
	if err := decisionBody(reason); err != nil {
		return "", err
	}
	text := fmt.Sprintf("Moved %s to %s: %s", id, to, reason)
	if fromProject {
		rec = dest
		text = fmt.Sprintf("Moved %s into this sprint from project %s: %s", id, source, reason)
	}
	updated, err := records.InsertEntry(rec.Text, "Decisions", decisionBlock("agent", text))
	if err != nil {
		return "", err
	}
	moved := *task
	moved.Parent = to
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if err := r.checkPlanned(w, r.with(moved)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Move(id, to); err != nil {
		return "", err
	}
	from := source
	if fromProject {
		from = "project " + source
	}
	out, err := r.apply(w, fmt.Sprintf("moved %s from %s to %s and added a sprint decision to %s", id, from, to,
		r.rel(rec.Path)), "pm: ")
	if err != nil {
		if uerr := ws.Move(id, source); uerr != nil {
			return "", fmt.Errorf("%w; moving %s back to %s failed too: %v", err, id, source, uerr)
		}
		return "", fmt.Errorf("%w; the work-store step is undone: %s is back in %s", err, id, source)
	}
	return out, nil
}

// cmdSprintMove moves an open sprint to another open project (the work-store page, Moving a sprint): the work store
// first, in one write through the compare-and-swap (the parent, the project's next number, the move note), then one
// records commit that renames the record and adds the move as a decision to both projects' records. A rerun that
// finds the sprint in the project with its record not yet renamed writes the records step alone, from the move note.
func cmdSprintMove(e *env, p *Parsed) (string, error) {
	reason := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id, toName := p.Get("sprint_id"), p.Get("to")
	sp := r.item(id)
	if sp == nil || sp.Type != work.Sprint {
		return "", refuse("%s is not a sprint in the work store", id)
	}
	rec, err := r.sprint(id)
	if err != nil {
		return "", err
	}
	trec, err := r.project(toName)
	if err != nil {
		return "", err
	}
	to := trec.Bead()
	if pi := r.item(to); pi == nil {
		return "", refuse("project %s has bead %s, which is not in the work store", toName, to)
	} else if pi.Status == work.Closed {
		return "", refuse("project %s (%s) is closed; move the sprint to an open project", toName, to)
	}
	if err := decisionBody(reason); err != nil {
		return "", err
	}
	if sp.Parent == to {
		return r.finishSprintMove(sp, rec, id, toName)
	}
	if sp.Status == work.Closed {
		return "", refuse("sprint %s is closed; only an open sprint moves", id)
	}
	if mv, ok := lastMove(sp); ok { // a move whose records step has not run: finish it first, or the record skips a project
		if here, err := r.projectRecord(mv.To); err == nil && rec.Name() != sprintName(here.Name(), sp.Number) {
			return "", refuse("the move of sprint %s to %s is not finished: its record is still %s; run pm sprint move %s "+
				"--to %s first", id, here.Name(), r.rel(rec.Path), id, here.Name())
		}
	}
	plan := work.SprintMove{From: sp.Parent, FromNumber: sp.Number, To: to,
		ToNumber: work.LastSprintNumber(slices.Values(r.x.All()), to) + 1, Reason: reason}
	planned := *sp
	planned.Parent, planned.Number = to, plan.ToNumber
	planDate := time.Now().UTC().Format("2006-01-02")
	w, err := r.moveWrites(rec, id, plan, planDate)
	if err != nil {
		return "", err
	}
	if err := r.checkPlanned(w, r.with(planned)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	moved, err := ws.MoveSprint(id, to, reason)
	if err != nil {
		return "", err
	}
	// The store minted against the remote, which may hold a sprint this clone has not seen: plan with its number.
	mv, ok := lastMove(&moved)
	if !ok || mv.To != to {
		return "", fmt.Errorf("sprint %s: the work store moved it but holds no move note to %s", id, to)
	}
	held := fmt.Sprintf("the work store holds the move (%s is sprint %d of %s): run the same command again to "+
		"finish the records step", id, mv.ToNumber, toName)
	if date := moveDate(&moved); mv != plan || date != planDate {
		if w, err = r.moveWrites(rec, id, mv, date); err == nil {
			err = r.checkPlanned(w, r.with(moved))
		}
		if err != nil {
			return "", fmt.Errorf("%w; %s", err, held)
		}
	}
	out, err := r.apply(w, "moved "+moveText(r, id, mv), "pm: ")
	if err != nil {
		return "", fmt.Errorf("%w; %s", err, held)
	}
	return out, nil
}

// sprintName is a sprint record's name: <project>-<number>.
func sprintName(project string, n int) string { return fmt.Sprintf("%s-%d", project, n) }

// lastMove is the sprint's last move, which brought it where it is.
func lastMove(sp *work.Item) (work.SprintMove, bool) {
	moves := work.SprintMoves(sp)
	if len(moves) == 0 || moves[len(moves)-1].To != sp.Parent || moves[len(moves)-1].ToNumber != sp.Number {
		return work.SprintMove{}, false
	}
	return moves[len(moves)-1], true
}

// projectRecord is the project record whose bead is id.
func (r *repo) projectRecord(id string) (*records.Record, error) {
	for _, rec := range r.recs {
		if rec.Type() == "project" && rec.Bead() == id {
			return rec, nil
		}
	}
	return nil, refuse("no project record has bead %s", id)
}

// moveText names a move for its records commit and output.
func moveText(r *repo, id string, mv work.SprintMove) string {
	name := func(bead string) string {
		if rec, err := r.projectRecord(bead); err == nil {
			return rec.Name()
		}
		return bead
	}
	return fmt.Sprintf("sprint %s from %s (sprint %d) to %s (sprint %d): record records/sprints/%s.md, a decision in "+
		"both projects", id, name(mv.From), mv.FromNumber, name(mv.To), mv.ToNumber, sprintName(name(mv.To), mv.ToNumber))
}

// moveDate is the UTC date of the sprint's last move note, which the move's decisions carry: the records step is then
// the same on every clone and in every time zone, so a rerun on a clone whose records had not synced another clone's
// finished step writes that step again, and the records sync's rebase onto it drops the copy.
func moveDate(sp *work.Item) string {
	for i := len(sp.Comments) - 1; i >= 0; i-- {
		if c := sp.Comments[i]; c.Kind == work.Note && c.Author == work.MoveAuthor {
			return c.CreatedAt.UTC().Format("2006-01-02")
		}
	}
	return ""
}

// moveWrites is the records step of a move: the record renamed, and the move as a decision dated date in both
// projects' records.
func (r *repo) moveWrites(rec *records.Record, id string, mv work.SprintMove, date string) ([]store.Write, error) {
	from, err := r.projectRecord(mv.From)
	if err != nil {
		return nil, err
	}
	to, err := r.projectRecord(mv.To)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(r.records, "sprints", sprintName(to.Name(), mv.ToNumber)+".md")
	if fsutil.Exists(path) {
		return nil, refuse("%s already exists; sprint %s cannot take its name", r.rel(path), id)
	}
	fromText, err := records.InsertEntry(from.Text, "Decisions", decisionBlockOn("agent", date, fmt.Sprintf(
		"Moved sprint %d (%s) to %s as sprint %d: %s", mv.FromNumber, id, to.Name(), mv.ToNumber, mv.Reason)))
	if err != nil {
		return nil, err
	}
	toText, err := records.InsertEntry(to.Text, "Decisions", decisionBlockOn("agent", date, fmt.Sprintf(
		"Took in sprint %d of %s (%s) as sprint %d: %s", mv.FromNumber, from.Name(), id, mv.ToNumber, mv.Reason)))
	if err != nil {
		return nil, err
	}
	return []store.Write{{Path: path, Text: rec.Text}, {Path: rec.Path, Remove: true},
		{Path: from.Path, Text: fromText}, {Path: to.Path, Text: toText}}, nil
}

// sprintPrefix is the "Sprint <n>: " a sprint's work-store title starts with, which its record's title has not.
var sprintPrefix = regexp.MustCompile(`^Sprint [0-9]+: `)

// cmdSprintEdit renames an open sprint, as pm task move moves a task: the work store first (its title, the
// "Sprint <n>: " prefix kept, which the work-store merge keeps at the sprint's number), then one records commit with the
// record's title header and the rename as a sprint decision; a failed records step puts the old title back.
func cmdSprintEdit(e *env, p *Parsed) (string, error) {
	title, reason := strip(p.Get("title")), p.Get("text")
	if title == "" {
		return "", refuse("--title is empty")
	}
	if sprintPrefix.MatchString(title) {
		return "", refuse("--title %s carries a 'Sprint <n>: ' prefix; give the title alone, and pm keeps the sprint's "+
			"number in front of it", strconv.Quote(title))
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("sprint_id")
	sp := r.item(id)
	if sp == nil || sp.Type != work.Sprint {
		return "", refuse("%s is not a sprint in the work store", id)
	}
	if sp.Status == work.Closed {
		return "", refuse("sprint %s is closed; only an open sprint is renamed", id)
	}
	rec, err := r.sprint(id)
	if err != nil {
		return "", err
	}
	old := rec.Title()
	if title == old {
		return "", refuse("sprint %s is titled %s already", id, strconv.Quote(title))
	}
	if err := decisionBody(reason); err != nil {
		return "", err
	}
	full := title
	if sprintPrefix.MatchString(sp.Title) {
		full = fmt.Sprintf("Sprint %d: %s", sp.Number, title)
	}
	text, err := records.WithTitle(rec.Text, r.rel(rec.Path), title)
	if err != nil {
		return "", err
	}
	if text, err = records.InsertEntry(text, "Decisions", decisionBlock("agent",
		fmt.Sprintf("Renamed the sprint from \"%s\" to \"%s\": %s", old, title, reason))); err != nil {
		return "", err
	}
	renamed := *sp
	renamed.Title = full
	w := []store.Write{{Path: rec.Path, Text: text}}
	if err := r.checkPlanned(w, r.with(renamed)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Edit(id, &full, nil); err != nil {
		return "", err
	}
	out, err := r.apply(w, fmt.Sprintf("renamed sprint %s to %s and added a sprint decision to %s", id, strconv.Quote(full),
		r.rel(rec.Path)), "pm: ")
	if err != nil {
		if uerr := ws.Edit(id, &sp.Title, nil); uerr != nil {
			return "", fmt.Errorf("%w; putting back the title %s failed too: %v", err, strconv.Quote(sp.Title), uerr)
		}
		return "", fmt.Errorf("%w; the work-store step is undone: %s is titled %s again", err, id, strconv.Quote(sp.Title))
	}
	return out, nil
}

func (r *repo) finishSprintMove(sp *work.Item, rec *records.Record, id, toName string) (string, error) {
	if rec.Name() == sprintName(toName, sp.Number) {
		return "", refuse("sprint %s is in project %s already, as sprint %d (%s)", id, toName, sp.Number,
			r.rel(rec.Path))
	}
	mv, ok := lastMove(sp)
	if !ok {
		return "", refuse("sprint %s is in project %s, but its record is %s and no move note says how it got "+
			"there; rename the record to records/sprints/%s.md by hand and commit it with pm commit", id, toName,
			r.rel(rec.Path), sprintName(toName, sp.Number))
	}
	w, err := r.moveWrites(rec, id, mv, moveDate(sp))
	if err != nil {
		return "", err
	}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "finished moving "+moveText(r, id, mv)+" (the work store held it already)", "pm: ")
}
