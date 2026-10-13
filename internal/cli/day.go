package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// pm day summarize: yesterday's and today's summary, each asked of the model when the day's activity changed since its
// stored summary, on the runtime summary_runtime in .pm/config.toml names (claude when unset); never on the other.

const summaryTimeout = 180 * time.Second // how long the model call may take

// cmdDaySummarize is pm day summarize. It reads both days' activity in one read of the work store; the records lock is
// taken only for each write, once the model answered, so a slow model blocks no other session.
func cmdDaySummarize(e *env, p *Parsed) (string, error) {
	dryRun := p.Get("dry_run") == "true"
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	runtime := ask.Runtime(cfg.Summary())
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	now := time.Now()
	yesterday, today := now.AddDate(0, 0, -1).Format(time.DateOnly), now.Format(time.DateOnly)
	activity := map[string]string{}
	for _, day := range []string{yesterday, today} {
		if activity[day], err = r.dayActivity(day); err != nil {
			return "", err
		}
	}
	first, err := e.summarizeOne(r, runtime, yesterday, activity[yesterday], dryRun)
	if err != nil {
		return "", err
	}
	second, err := e.summarizeOne(r, runtime, today, activity[today], dryRun)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(first, "skipped") || strings.HasSuffix(first, "nothing to summarize") {
		return second, nil
	}
	return first + "\n" + second, nil
}

// summarizeOne is day's summary: skipped when nothing happened that day or when the activity is unchanged since the
// stored summary (its digest); otherwise asked of the model on runtime and committed to days/<day>.summary.json on the records
// branch (with dryRun, printed and not written).
func (e *env) summarizeOne(r *repo, runtime ask.Runtime, day, activity string, dryRun bool) (string, error) {
	if activity == "" {
		return "no activity on " + day + "; nothing to summarize", nil
	}
	sum := sha256.Sum256([]byte(activity))
	digest := hex.EncodeToString(sum[:])[:16]
	path := records.SummaryPath(r.records, day)
	if fsutil.Exists(path) {
		old, err := records.ReadSummary(path)
		if err != nil {
			return "", err
		}
		if old.Digest == digest {
			return "activity unchanged since the summary of " + old.GeneratedAt + "; skipped", nil
		}
	}
	text, err := ask.Ask(runtime, ask.Call{Prompt: strings.NewReplacer("{day}", day, "{activity}", activity).Replace(pm.DaySummary),
		Timeout: summaryTimeout})
	var ae *ask.Error
	switch {
	case errors.As(err, &ae) && ae.Missing:
		return "", refuse("%s; pm day summarize asks %s, as summary_runtime in %s says", ae.Error(), runtime.Command(), config.Rel)
	case errors.As(err, &ae):
		return "", refuse("%s; no summary written", ae.Error())
	case err != nil:
		return "", err
	}
	generated := time.Now().UTC().Format("2006-01-02T15:04:05") + "+00:00"
	body := "{\n" // json.dumps(data, indent=1, ensure_ascii=False)
	for i, kv := range [][2]string{{"date", day}, {"generated_at", generated}, {"digest", digest},
		{"model", runtime.Model()}, {"text", text}} {
		if i > 0 {
			body += ",\n"
		}
		body += " " + pyjson.String(kv[0], false) + ": " + pyjson.String(kv[1], false)
	}
	body += "\n}\n"
	if dryRun {
		return "dry run, nothing written; would write " + r.rel(path) + ":\n" + body, nil
	}
	unlock, err := store.Lock(r.records)
	if err != nil {
		return "", err
	}
	defer unlock()
	dirty, err := store.Uncommitted(r.records, []string{path})
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 {
		return "", refuse("%s has uncommitted changes; revert them and run this again", r.rel(path))
	}
	return r.apply([]store.Write{{Path: path, Text: body}}, "summarized "+day+" in "+r.rel(path), "pm: ")
}

// dayActivity is plain text of what happened on the local day: per project, the sprints finished (with their
// Outcome) or opened, the tasks finished (with their close reason), started or opened, and the owner requests raised
// or closed that day; the docs dated that day; and the records committed that day, not counting the generated
// summaries. The summary's input and, hashed, its digest. It holds only events dated that day, never current state,
// so a day's digest changes only when that day's events do. Empty when nothing happened that day.
func (r *repo) dayActivity(day string) (string, error) {
	on := func(t time.Time) bool { return site.LocalDay(t) == day }
	var out []string
	for _, p := range r.recs {
		if p.Type() != "project" {
			continue
		}
		var lines []string
		for _, sp := range r.dayChildren(p.Bead(), func(it *work.Item) bool { return it.Type == work.Sprint }) {
			rows, err := r.sprintActivity(sp, on)
			if err != nil {
				return "", err
			}
			if rows != nil {
				lines = append(append(lines, "  sprint "+sp.Title), rows...)
			}
		}
		if lines != nil {
			out = append(append(out, "project "+p.Title()), lines...)
		}
	}
	for _, rec := range r.recs {
		if rec.Type() == "doc" && rec.Meta["date"].Str == day {
			out = append(out, "doc "+rec.Title()+" ("+rec.Rel+")")
		}
	}
	log, err := store.Git(r.records, "log", "--since="+day+"T00:00", "--until="+day+"T23:59:59", "--format=%x00%s",
		"--name-only")
	if err != nil {
		return "", err
	}
	for _, chunk := range strings.Split(log, "\x00")[1:] {
		var lines []string
		for _, l := range strings.Split(chunk, "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			continue
		}
		var names []string
		for _, n := range lines[1:] {
			if !strings.HasSuffix(n, ".summary.json") {
				names = append(names, n)
			}
		}
		if names != nil {
			out = append(out, "records commit: "+lines[0]+" ["+strings.Join(names, ", ")+"]")
		}
	}
	return strings.Join(out, "\n"), nil
}

func (r *repo) dayChildren(parent string, keep func(*work.Item) bool) []*work.Item {
	var out []*work.Item
	for i := range r.items {
		if it := &r.items[i]; it.Parent == parent && keep(it) {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return work.CompareIDs(out[i].ID, out[j].ID) < 0 })
	return out
}

func (r *repo) sprintActivity(sp *work.Item, on func(time.Time) bool) ([]string, error) {
	var rows []string
	if on(sp.ClosedAt) {
		row := "    sprint finished"
		for _, rec := range r.recs {
			if rec.Type() == "sprint" && rec.Bead() == sp.ID {
				o, err := records.Outcome(rec)
				if err != nil {
					return nil, err
				}
				if o != "" {
					row += ": " + o
				}
				break
			}
		}
		rows = append(rows, row)
	}
	if on(sp.CreatedAt) {
		rows = append(rows, "    sprint opened")
	}
	// a dismissed need is a [TEST] or replaced one; a dismissed task is a dropped one, which moved the day
	for _, t := range r.dayChildren(sp.ID, func(it *work.Item) bool {
		return it.Resolution != work.Dismissed || it.Type == work.Task
	}) {
		request := t.Type == work.Need
		noun := "task"
		if request {
			noun = "request to the owner (" + site.Kind(t) + ")"
		}
		if on(t.ClosedAt) {
			verb := "closed"
			if t.Resolution == work.Dismissed {
				verb = "dropped"
			}
			row := "    " + noun + " " + verb + ": " + t.Title
			if t.CloseReason != "" {
				row += " (" + t.CloseReason + ")"
			}
			rows = append(rows, row)
		} else if on(t.StartedAt) {
			rows = append(rows, "    "+noun+" started: "+t.Title)
		}
		if on(t.CreatedAt) {
			if request {
				rows = append(rows, "    "+noun+" raised: "+t.Title)
			} else {
				rows = append(rows, "    task opened: "+t.Title)
			}
		}
	}
	return rows, nil
}
