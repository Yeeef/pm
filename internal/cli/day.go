package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// pm day summarize: yesterday's and today's summary, each asked of the model when the day's activity changed since its
// stored summary. Python source: summarize_day, summarize_one, day_activity_text and ask_model in cli.py.

const (
	summaryModel   = "claude-haiku-5-5"
	summaryTimeout = 180 // seconds the model call may take
)

// cmdDaySummarize is pm day summarize. It reads both days' activity in one read of the work store; the records lock is
// taken only for each write, once the model answered, so a slow model blocks no other session.
func cmdDaySummarize(e *env, p *Parsed) (string, error) {
	dryRun := p.Get("dry_run") == "true"
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
	first, err := e.summarizeOne(r, yesterday, activity[yesterday], dryRun)
	if err != nil {
		return "", err
	}
	second, err := e.summarizeOne(r, today, activity[today], dryRun)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(first, "skipped") || strings.HasSuffix(first, "nothing to summarize") {
		return second, nil
	}
	return first + "\n" + second, nil
}

// summarizeOne is day's summary: skipped when nothing happened that day or when the activity is unchanged since the
// stored summary (its digest); otherwise asked of the model and committed to days/<day>.summary.json on the records
// branch (with dryRun, printed and not written).
func (e *env) summarizeOne(r *repo, day, activity string, dryRun bool) (string, error) {
	if activity == "" {
		return "no activity on " + day + "; nothing to summarize", nil
	}
	sum := sha256.Sum256([]byte(activity))
	digest := hex.EncodeToString(sum[:])[:16]
	path := records.SummaryPath(r.records, day)
	if exists(path) {
		old, err := records.ReadSummary(path)
		if err != nil {
			return "", err
		}
		if old.Digest == digest {
			return "activity unchanged since the summary of " + old.GeneratedAt + "; skipped", nil
		}
	}
	text, err := askModel(strings.NewReplacer("{day}", day, "{activity}", activity).Replace(pm.DaySummary))
	if err != nil {
		return "", err
	}
	generated := time.Now().UTC().Format("2006-01-02T15:04:05") + "+00:00"
	body := "{\n" // json.dumps(data, indent=1, ensure_ascii=False)
	for i, kv := range [][2]string{{"date", day}, {"generated_at", generated}, {"digest", digest},
		{"model", summaryModel}, {"text", text}} {
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
	return r.apply([]store.Write{{Path: path, Text: body}}, "summarized "+day+" in "+r.rel(path), "", "pm: ")
}

// askModel is the model's answer to prompt from the claude CLI, non-interactive, without tools, hooks or a saved
// session. It fails hard when claude is missing, fails or answers nothing; there is no fallback text.
func askModel(prompt string) (string, error) {
	cwd, err := os.MkdirTemp("", "pm-summary-") // no project settings, hooks or CLAUDE.md apply
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(cwd)
	res, err := proc.Run([]string{"claude", "-p", "--model", summaryModel, "--tools", "", "--setting-sources", "",
		"--no-session-persistence"}, proc.Options{Cwd: &cwd, Stdin: prompt, Timeout: summaryTimeout * time.Second,
		TimeoutText: fmt.Sprint(summaryTimeout)})
	var pe *proc.Error
	switch {
	case errors.As(err, &pe) && pe.Type == "FileNotFoundError":
		return "", refuse("claude is not installed or not on PATH; pm day summarize needs the claude CLI")
	case errors.As(err, &pe) && pe.Type == "TimeoutExpired":
		return "", refuse("claude -p timed out after %ds; no summary written", summaryTimeout)
	case err != nil:
		return "", err
	}
	text := config.PyStrip(res.Stdout)
	if res.Code != 0 || text == "" {
		said := res.Stderr
		if said == "" {
			said = res.Stdout
		}
		said = firstRunes(strings.Join(strings.FieldsFunc(said, config.IsPySpace), " "), 500)
		if said == "" {
			said = "no output"
		}
		return "", refuse("claude -p failed (exit %d): %s; no summary written", res.Code, said)
	}
	return text, nil
}

// dayActivity is plain text of what happened on the local day: per project, the sprints finished (with their
// Outcome) or opened, the tasks finished (with their close reason), started or opened, and the owner requests raised
// or closed that day; the docs dated that day; and the records committed that day, not counting the generated
// summaries. The summary's input and, hashed, its digest. It holds only events dated that day, never current state,
// so a day's digest changes only when that day's events do. Empty when nothing happened that day.
func (r *repo) dayActivity(day string) (string, error) {
	on := func(t time.Time) bool { return site.LocalDay(t) == day }
	children := func(parent string, keep func(*work.Item) bool) []*work.Item {
		var out []*work.Item
		for i := range r.items {
			if it := &r.items[i]; it.Parent == parent && keep(it) {
				out = append(out, it)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return work.CompareIDs(out[i].ID, out[j].ID) < 0 })
		return out
	}
	var out []string
	for _, p := range r.recs {
		if p.Type() != "project" {
			continue
		}
		var lines []string
		for _, sp := range children(p.Bead(), func(it *work.Item) bool { return it.Type == work.Sprint }) {
			var rows []string
			if on(sp.ClosedAt) {
				row := "    sprint finished"
				for _, rec := range r.recs {
					if rec.Type() == "sprint" && rec.Bead() == sp.ID {
						o, err := records.Outcome(rec)
						if err != nil {
							return "", err
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
			for _, t := range children(sp.ID, func(it *work.Item) bool { return it.Resolution != work.Dismissed }) {
				request := t.Type == work.Need
				noun := "task"
				if request {
					noun = "request to the owner (" + site.Kind(t) + ")"
				}
				if on(t.ClosedAt) {
					row := "    " + noun + " closed: " + t.Title
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
