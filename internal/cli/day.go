package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/cli/dayactivity"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
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
		if activity[day], err = dayactivity.Of(r.records, r.recs, r.items, day); err != nil {
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
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(struct {
		Date        string `json:"date"`
		GeneratedAt string `json:"generated_at"`
		Digest      string `json:"digest"`
		Model       string `json:"model"`
		Text        string `json:"text"`
	}{day, generated, digest, runtime.Model(), text}); err != nil {
		return "", err
	}
	body := b.String()
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
