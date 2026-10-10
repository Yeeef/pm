package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The commands that write records and work items: findings, feedback, new docs, design pages and postmortems,
// projects and sprints opened and closed, tasks added, closed, claimed and moved. Each refuses before any write; a
// write that touches both stores writes the work store first, then the records.

const (
	frameShape = "--text is the sprint's frame, e.g.:\n" +
		"  ## Goal\n  Ship the parser, because the site needs tables.\n" +
		"  ## Scope\n  **In:** the parser and its tests.\n  **Out:** the site's styling.\n" +
		"  ## Done when\n  - make test passes with a table record."
	decisionShape = "a decision body is the decision on its first line, then its reason on the next line, e.g.:\n" +
		"  Records live on their own branch, in one store every worktree shares.\n" +
		"  A record kept on a code branch is invisible to the other branches until a merge."
	worktreesDir = ".claude/worktrees" // where pm task claim tells agents to make theirs
)

var (
	frame       = []string{"Goal", "Scope", "Done when"}
	slugRE      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	feedbackRE  = regexp.MustCompile(`(?m)^### \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC, session ` + "`")
	mergedAs    = regexp.MustCompile(`^merged as ([0-9a-f]{7,40})\b`)
	sectionHead = regexp.MustCompile(`(?m)^## (.+?)` + records.PyS + `*$`) // Python's \s
	scopeRE     = regexp.MustCompile(`^(?s)\*\*In:\*\*(.*?)\*\*Out:\*\*(.*)`)
)

// feedbackEntry matches the heading of each entry in a feedback doc.
var feedbackEntry = feedbackRE

func isEpic(it *work.Item) bool { return it.Type == work.Project || it.Type == work.Sprint }

func strip(s string) string { return config.PyStrip(s) }

// given is the dest's value and whether the command line gave it (argparse's None when not).
func given(p *Parsed, dest string) (string, bool) {
	v := p.values[dest]
	if len(v) == 0 {
		return "", false
	}
	return v[len(v)-1], true
}

// planned is a new item as a check sees it before the work store mints it.
func planned(id string, t work.Type, parent, title string) work.Item {
	now := time.Now().UTC().Truncate(time.Second)
	return work.Item{ID: id, Type: t, Parent: parent, Title: title, Status: work.Open, CreatedAt: now, UpdatedAt: now}
}

// closedCopy is an item as a close leaves it, for the check before the close.
func closedCopy(it *work.Item, reason string) work.Item {
	c := *it
	c.Status, c.Resolution, c.CloseReason, c.Holder = work.Closed, work.Done, reason, nil
	c.ClosedAt = time.Now().UTC().Truncate(time.Second)
	return c
}

// closer is the session a close records: the holder's, else this command's. Python pm's Beads keep the claiming
// session, which the work-store import reads as the closer.
func closer(it *work.Item) string {
	if it.Holder != nil {
		return it.Holder.Session
	}
	return currentSession()
}

// undoCreate closes an item a command made before its records step failed, so the failure leaves no stray item; the
// work store has no delete.
func undoCreate(ws work.Store, id string, err error) error {
	if cerr := ws.Close(id, "the records step of the command that made it failed", work.Dismissed, ""); cerr != nil {
		return fmt.Errorf("%w; closing %s as dismissed failed too: %v", err, id, cerr)
	}
	return fmt.Errorf("%w; the work-store step is undone: %s closed as dismissed", err, id)
}

// ---------------------------------------------------------------- finding, feedback, doc, design, postmortem

func cmdFindingAdd(e *env, p *Parsed) (string, error) {
	text := p.Get("text") // --text, or --text-file's body (runAgent)
	if words, body := p.values["words"], p.values["text"]; len(words) > 0 {
		if body != nil {
			return "", refuse("give the finding as one argument or with %s, not both", textForms)
		}
		text = strip(strings.Join(words, " "))
		if strings.HasPrefix(text, "--") {
			return "", refuse("the finding text starts with --, an option pm finding add does not have: %s; give "+
				"the text as one quoted argument, with --text=\"…\", or with --text-file - <<'EOF'", pyRepr(text))
		}
	}
	if text == "" {
		return "", refuse("the finding text is empty")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprintID := p.Get("sprint")
	rec, err := r.sprint(sprintID)
	if err != nil {
		return "", err
	}
	if it := r.item(sprintID); it != nil && it.Status == work.Closed {
		return "", refuse("sprint %s is closed; add the finding to an open sprint", sprintID)
	}
	entry := wrapFill(text, 78, "- ", "  ")
	updated, err := records.InsertEntry(rec.Text, "Findings", entry)
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added a finding to "+r.rel(rec.Path), "", "pm: ")
}

// feedbackDoc is the repo's pm feedback doc, or nil before its first entry.
func feedbackDoc(recs []*records.Record) *records.Record {
	for _, r := range recs {
		if records.IsFeedbackDoc(r) {
			return r
		}
	}
	return nil
}

func noSession() error {
	return refuse("no agent session: neither %s nor %s is set; name one with --session", sessionEnv, codexSessionEnv)
}

func cmdFeedbackAdd(e *env, p *Parsed) (string, error) {
	text := p.Get("text")
	if text == "" {
		return "", refuse("the feedback text is empty; pass it with %s", textForms)
	}
	sid := strip(p.Get("session"))
	if sid == "" {
		sid = currentSession()
	}
	if sid == "" {
		return "", noSession()
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var about []string
	if name, ok := given(p, "project"); ok {
		prec, err := r.project(name)
		if err != nil {
			return "", err
		}
		about = append(about, fmt.Sprintf("project `%s`", prec.Name()))
	}
	for _, f := range []struct{ flag, dest, word string }{{"--sprint", "sprint", "sprint"}, {"--task", "task", "task"}} {
		if id, ok := given(p, f.dest); ok {
			if r.item(id) == nil {
				return "", refuse("%s %s is not in the work store", f.flag, id)
			}
			about = append(about, fmt.Sprintf("%s `%s`", f.word, id))
		}
	}
	for _, rec := range r.recs { // a per-project doc from before the repo's one doc: merged by hand first
		if err := records.CheckFeedbackDoc(rec); err != nil {
			return "", refuse("%s", err)
		}
	}
	entry := fmt.Sprintf("### %s UTC, session `%s`\n\n", time.Now().UTC().Format("2006-01-02 15:04"), sid)
	if about != nil {
		entry += "About " + strings.Join(about, ", ") + ".\n\n"
	}
	entry += text + "\n"
	path := filepath.Join(r.records, records.FeedbackDoc+".md")
	var old string
	if doc := feedbackDoc(r.recs); doc != nil {
		old = doc.Text
	} else if exists(path) {
		return "", refuse("%s exists but is no pm feedback doc (type: doc); move it, then run this again", r.rel(path))
	} else {
		old = fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n---\n\n"+
			"Where pm got in the way, one entry per `pm feedback add`, newest last; each names the project it is about, "+
			"when it is about one.\n", records.FeedbackTitle, store.Today())
	}
	w := []store.Write{{Path: path, Text: strings.TrimRight(old, "\n") + "\n\n" + entry}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added feedback to "+r.rel(path), "", "pm: ")
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// newRecord is the checks every new-record command makes first: the slug's form (badSlug the refusal, with the slug
// for its %s) and a non-empty title.
func newRecord(badSlug, slug, title string) error {
	if !slugRE.MatchString(slug) {
		return refuse(badSlug, pyRepr(slug))
	}
	if strip(title) == "" {
		return refuse("--title is empty")
	}
	return nil
}

func (r *repo) refuseExisting(path string) error {
	if exists(path) {
		return refuse("%s already exists; edit it by hand or choose another slug", r.rel(path))
	}
	return nil
}

func cmdDocNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("doc slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	body := p.Get("text")
	if body == "" {
		return "", refuse("the doc body is empty; pass it with %s", textForms)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var target string
	if bead, ok := given(p, "bead"); ok {
		if r.item(bead) == nil {
			return "", refuse("--bead %s is not in the work store", bead)
		}
		inside := false
		chain := append([]string{bead}, r.x.Ancestors(bead)...)
		for _, rec := range r.recs {
			if rec.Type() == "project" && contains(chain, rec.Bead()) {
				inside = true
			}
		}
		if !inside {
			return "", refuse("--bead %s is not inside a project with a record", bead)
		}
		target = "bead: " + bead
	} else {
		prec, err := r.project(p.Get("project"))
		if err != nil {
			return "", err
		}
		target = "project: " + prec.Name()
	}
	day := store.Today()
	path := filepath.Join(r.records, "docs", day+"-"+slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	text := fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n%s\n---\n\n%s\n", records.YAMLStr(strip(title)), day, target, body)
	w := []store.Write{{Path: path, Text: text}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "created "+r.rel(path), "", "pm: ")
}

func cmdDesignNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("design slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	prec, err := r.project(p.Get("project"))
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.records, "design", slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: records.DesignText(records.YAMLStr(strip(title)), prec.Name())}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, fmt.Sprintf("created %s; fill in its sections by hand in the store, then pm commit -m \"…\" %s",
		r.rel(path), r.rel(path)), "", "pm: ")
}

func cmdPostmortemNew(e *env, p *Parsed) (string, error) {
	slug, title := p.Get("slug"), p.Get("title")
	if err := newRecord("postmortem slug %s must be lowercase words joined by '-'", slug, title); err != nil {
		return "", err
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var target string
	if sprintID, ok := given(p, "sprint"); ok {
		rec, err := r.sprint(sprintID)
		if err != nil {
			return "", err
		}
		target = "sprint: " + rec.Bead()
	} else {
		prec, err := r.project(p.Get("project"))
		if err != nil {
			return "", err
		}
		target = "project: " + prec.Name()
	}
	day := store.Today()
	path := filepath.Join(r.records, "postmortems", day+"-"+slug+".md")
	if err := r.refuseExisting(path); err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: records.PostmortemText(records.YAMLStr(strip(title)), day, target)}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, fmt.Sprintf("created %s; fill in its sections by hand in the store, then pm commit -m \"…\" %s",
		r.rel(path), r.rel(path)), "", "pm: ")
}

// ---------------------------------------------------------------- projects and sprints

func cmdProjectOpen(e *env, p *Parsed) (string, error) {
	name := p.Get("name")
	if !slugRE.MatchString(name) {
		return "", refuse("project name %s must be lowercase words joined by '-'", pyRepr(name))
	}
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	goal := p.Get("text")
	if goal == "" {
		return "", refuse("Goal is empty; pass the project's goal with %s", textForms)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.records, "projects", name+".md")
	taken := exists(path)
	for _, rec := range r.recs {
		taken = taken || rec.Type() == "project" && rec.Name() == name
	}
	if taken {
		return "", refuse("project %s already exists (%s)", pyRepr(name), r.rel(path))
	}
	const plannedID = "pm-planned-project"
	text := func(id string) string { return records.ProjectText(records.YAMLStr(title), id, goal) }
	if err := r.checkPlanned([]store.Write{{Path: path, Text: text(plannedID)}},
		r.with(planned(plannedID, work.Project, "", title))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Project, Title: title})
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: text(made.ID)}}
	if err := r.checkPlanned(w, r.with(made)); err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	out, err := r.apply(w, fmt.Sprintf("opened project %s: epic %s, record %s", name, made.ID, r.rel(path)), "", "pm: ")
	if err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	return out, nil
}

// parseSections splits a frame like "## Goal\n…\n## Scope\n…" into section bodies.
func parseSections(text string, allowed []string) (map[string]string, error) {
	locs := sectionHead.FindAllStringSubmatchIndex(text, -1)
	head := text
	if locs != nil {
		head = text[:locs[0][0]]
	}
	if strip(head) != "" {
		return nil, refuse("text before the first section heading; start --text with '## %s'; %s", allowed[0], frameShape)
	}
	out := map[string]string{}
	for i, l := range locs {
		name := text[l[2]:l[3]]
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		if !contains(allowed, name) {
			return nil, refuse("unknown section '## %s' in --text; allowed: %s; %s", name, strings.Join(allowed, ", "),
				frameShape)
		}
		out[name] = strip(text[l[1]:end])
	}
	return out, nil
}

var trailingNumber = regexp.MustCompile(`^[0-9]+$`)

func cmdSprintOpen(e *env, p *Parsed) (string, error) {
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	fr, err := parseSections(p.Get("text"), frame)
	if err != nil {
		return "", err
	}
	for _, name := range frame {
		if fr[name] == "" {
			return "", refuse("'## %s' is missing or empty in --text; %s", name, frameShape)
		}
	}
	if m := scopeRE.FindStringSubmatch(fr["Scope"]); m == nil || strip(m[1]) == "" || strip(m[2]) == "" {
		return "", refuse("Scope needs a non-empty **In:** list followed by a non-empty **Out:** list; %s", frameShape)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	project := p.Get("project")
	prec, err := r.project(project)
	if err != nil {
		return "", err
	}
	epicID := prec.Bead()
	epic := r.item(epicID)
	if epic == nil {
		return "", refuse("project %s has bead %s, which is not in the work store", project, epicID)
	}
	if epic.Status == work.Closed {
		return "", refuse("project %s (%s) is closed; open a sprint in an open project", project, epicID)
	}
	// The numbers moved away from the project count on both sides: pm sprint move renamed their records, and the
	// project never mints them again (the work-store page, Moving a sprint).
	kids, top, suffix, movedAway := 0, work.LastSprintNumber(slices.Values(r.x.All()), epicID), 0, 0
	for _, it := range r.x.All() {
		for _, m := range work.SprintMoves(it) {
			if it.Type == work.Sprint && m.From == epicID {
				movedAway = max(movedAway, m.FromNumber)
			}
		}
	}
	for _, k := range r.x.Children(epicID) {
		if isEpic(k) {
			kids++
		}
		if last := k.ID[strings.LastIndex(k.ID, ".")+1:]; strings.Contains(k.ID, ".") && trailingNumber.MatchString(last) {
			n, _ := strconv.Atoi(last)
			suffix = max(suffix, n)
		}
	}
	n := max(kids, movedAway)
	name := regexp.MustCompile(`^` + regexp.QuoteMeta(project) + `-([0-9]+)$`)
	for _, rec := range r.recs {
		if m := name.FindStringSubmatch(rec.Name()); rec.Type() == "sprint" && m != nil {
			v, _ := strconv.Atoi(m[1])
			n = max(n, v)
		}
	}
	n++
	if top+1 != n {
		return "", refuse("the next sprint of %s is %d by its records and %d by the work store's sprint numbers; "+
			"fix the records or the work store first", project, n, top+1)
	}
	path := filepath.Join(r.records, "sprints", fmt.Sprintf("%s-%d.md", project, n))
	if exists(path) {
		return "", refuse("%s already exists", r.rel(path))
	}
	full := fmt.Sprintf("Sprint %d: %s", n, title)
	plannedID := fmt.Sprintf("%s.%d", epicID, suffix+1)
	plan := planned(plannedID, work.Sprint, epicID, full)
	plan.Number = n
	text := func(id string) string { return records.SprintText(records.YAMLStr(title), id, fr) }
	if err := r.checkPlanned([]store.Write{{Path: path, Text: text(plannedID)}}, r.with(plan)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Sprint, Parent: epicID, Title: full})
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: path, Text: text(made.ID)}}
	if err := r.checkPlanned(w, r.with(made)); err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	out, err := r.apply(w, fmt.Sprintf("opened sprint %d of %s: epic %s, record %s", n, project, made.ID, r.rel(path)),
		"", "pm: ")
	if err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	return out, nil
}

// requireCommitted is the records commit a close names; the closed record itself must be committed.
func (r *repo) requireCommitted(path string) (string, error) {
	dirty, err := store.Uncommitted(r.records, []string{path})
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 {
		return "", refuse("%s has uncommitted changes; commit it first with pm commit -m \"…\" <path> so the close "+
			"names the exact state it delivered", strings.Join(dirty, ", "))
	}
	return store.Git(r.records, "rev-parse", "--short", "HEAD")
}

// mergeStamp is a sprint record's text with stamp as a paragraph after the Outcome's verdict paragraph.
func mergeStamp(text, stamp string) string {
	rng, _ := records.SectionRange(text, "Delivery report")
	lines := strings.Split(text, "\n")
	n := rng.Start
	for n < rng.End && lines[n] != "### Outcome" {
		n++
	}
	n++
	for n < rng.End && (strip(lines[n]) == "" || strings.HasPrefix(lines[n], ">")) {
		n++
	}
	for n < rng.End && strip(lines[n]) != "" && !strings.HasPrefix(lines[n], "#") {
		n++
	}
	add := []string{"", stamp}
	if n < rng.End && strings.HasPrefix(lines[n], "#") {
		add = append(add, "")
	}
	out := append(append(append([]string{}, lines[:n]...), add...), lines[n:]...)
	return strings.Join(out, "\n")
}

// sprintReviews is each PR review naming the sprint, open or closed, by id.
func (r *repo) sprintReviews(sprintID string) []*work.Item {
	var out []*work.Item
	for i := range r.items {
		it := &r.items[i]
		if it.Need != nil && it.Need.Review != nil && contains(it.Need.Review.Sprints, sprintID) {
			out = append(out, it)
		}
	}
	return out
}

// statusText is an item's status as Python pm names it from Beads: in_progress for an open item with a holder.
func statusText(it *work.Item) string {
	if it.Status == work.Open && it.Holder != nil {
		return "in_progress"
	}
	return string(it.Status)
}

func cmdSprintClose(e *env, p *Parsed) (string, error) {
	merged, agentMerge := given(p, "merged")
	pr, prGiven := given(p, "pr")
	if prGiven && !agentMerge {
		return "", refuse("--pr names the PR whose merge --merged gives; give --merged SHA with it")
	}
	var mergedSHA string
	if agentMerge {
		if pr = strip(pr); prGiven && !urlRE.MatchString(pr) {
			return "", refuse("--pr %s is not a URL; give the pull request's link", pyRepr(pr))
		}
		var err error
		if mergedSHA, err = onMain(e, strip(merged)); err != nil { // before the records lock: it fetches
			return "", err
		}
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("sprint_id")
	rec, err := r.sprint(id)
	if err != nil {
		return "", err
	}
	epic := r.item(id)
	if epic == nil {
		return "", refuse("sprint %s is not in the work store", id)
	}
	if epic.Status == work.Closed {
		return "", refuse("sprint %s is already closed", id)
	}
	head, err := r.requireCommitted(rec.Path)
	if err != nil {
		return "", err
	}
	line, err := records.Outcome(rec)
	if err != nil {
		return "", err
	}
	if line == "" {
		return "", refuse("%s: the committed Delivery report Outcome is still '%s'; write it (done, partial or voided, "+
			"plus one sentence, then optional bullets) and commit", r.rel(rec.Path), records.NotClosed)
	}
	if part := records.ReportPart(rec, `Against "Done when"`); part == "" || part == records.NotClosed {
		return "", refuse("%s: the committed Delivery report 'Against \"Done when\"' is still '%s'; write each item with "+
			"its evidence and commit", r.rel(rec.Path), records.NotClosed)
	}
	var open []*work.Item
	for _, k := range r.x.Children(id) {
		if k.Status != work.Closed {
			open = append(open, k)
		}
	}
	reviews := r.sprintReviews(id)
	for _, rv := range reviews {
		if rv.Status != work.Closed && !containsItem(open, rv) {
			open = append(open, rv)
		}
	}
	if open != nil {
		listing := make([]string, len(open))
		for i, k := range open {
			listing[i] = fmt.Sprintf("%s (%s)", k.ID, statusText(k))
		}
		return "", refuse("open tasks in the sprint: %s; close each with pm task close or move it with pm task move (a PR "+
			"review closes with pm action done <id> --reason \"merged as <sha>\" once the PR is on main)",
			strings.Join(listing, ", "))
	}
	type merge struct{ sha, pr string }
	var merges []merge
	for _, rv := range reviews {
		if rv.Resolution == work.Dismissed { // a replaced PR's review, or a [TEST] one: it delivers nothing
			continue
		}
		if agentMerge {
			return "", refuse("sprint %s holds PR review %s (%s), whose close stamps the merge; close it with pm action "+
				"done %s --reason \"merged as <sha>\" if it is not closed yet, then run pm sprint close %s without --merged",
				id, rv.ID, rv.Need.Review.PR, rv.ID, id)
		}
		m := mergedAs.FindStringSubmatch(rv.CloseReason)
		if m == nil {
			why := "no reason"
			if rv.CloseReason != "" {
				why = rv.CloseReason
			}
			return "", refuse("PR review %s (%s) is closed as %s, not 'merged as <sha>'; a sprint closes only once each PR "+
				"delivering it is on main, so the close stamps its merge commit into the Outcome", rv.ID,
				rv.Need.Review.PR, pyRepr(why))
		}
		pr := rv.Need.Review.PR
		if strings.Contains(pr, "/pull/") {
			pr = site.PRLabel(pr)
		}
		merges = append(merges, merge{m[1][:7], pr})
	}
	if agentMerge {
		label := ""
		if prGiven {
			label = pr
			if strings.Contains(pr, "/pull/") {
				label = site.PRLabel(pr)
			}
		}
		merges = append(merges, merge{mergedSHA[:7], label})
	}
	b, err := os.ReadFile(rec.Path)
	if err != nil {
		return "", err
	}
	text := string(b)
	stamps, shas := make([]string, len(merges)), make([]string, len(merges))
	for i, m := range merges {
		stamps[i] = fmt.Sprintf("Merged as %s (%s).", m.sha, m.pr)
		if m.pr == "" {
			stamps[i] = fmt.Sprintf("Merged as %s.", m.sha)
		}
		shas[i] = m.sha
	}
	stamp := strings.Join(stamps, " ")
	var w []store.Write
	if merges != nil && !strings.Contains(text, stamp) {
		w = []store.Write{{Path: rec.Path, Text: mergeStamp(text, stamp)}}
	}
	proj := records.ProjectOf(rec, r.recs, r.x)
	number := rec.Name()
	projName := ""
	if proj != nil {
		projName = proj.Name()
		if m := regexp.MustCompile(`^` + regexp.QuoteMeta(projName) + `-([0-9]+)$`).FindStringSubmatch(rec.Name()); m != nil {
			number = m[1]
		}
	}
	message := fmt.Sprintf("[SPRINT] %s sprint %s: closed, merged as %s", projName, number, strings.Join(shas, ", "))
	if err := r.checkPlanned(w, r.with(closedCopy(epic, fmt.Sprintf("%s (records commit %s)", line, head)))); err != nil {
		return "", err
	}
	if w != nil {
		if _, err := r.apply(w, message, "", ""); err != nil {
			return "", err
		}
		if head, err = store.Git(r.records, "rev-parse", "--short", "HEAD"); err != nil {
			return "", err
		}
	}
	reason := fmt.Sprintf("%s (records commit %s)", line, head)
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(id, reason, work.Done, closer(epic)); err != nil {
		if w != nil {
			return "", fmt.Errorf("%w; the stamp is committed as %s, so run pm sprint close %s again", err, head, id)
		}
		return "", err
	}
	out := fmt.Sprintf("closed %s: %s", id, reason)
	if w != nil {
		out += "; stamped the Outcome: " + stamp
	}
	return out, nil
}

// shaRE is a commit's hex sha, as --merged takes it.
var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// onMain is the full sha of the commit sha names, refused unless it is on the remote's main branch, fetched first.
func onMain(e *env, sha string) (string, error) {
	if !shaRE.MatchString(sha) {
		return "", refuse("--merged %s is not a commit sha: 7 to 40 hex digits", pyRepr(sha))
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	root, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	main := cfg.Remote + "/" + cfg.MainBranch
	if _, err := store.Git(root, "fetch", "--quiet", cfg.Remote, fmt.Sprintf("+refs/heads/%s:refs/remotes/%s",
		cfg.MainBranch, main)); err != nil {
		return "", refuse("--merged %s: pm cannot see %s, so it cannot check the commit is on it: %v", sha, main, err)
	}
	full, err := store.Git(root, "rev-parse", "--verify", "--quiet", sha+"^{commit}")
	if err != nil {
		return "", refuse("--merged %s is no commit of this repo, %s included", sha, main)
	}
	if _, err := store.Git(root, "merge-base", "--is-ancestor", full, "refs/remotes/"+main); err != nil {
		return "", refuse("--merged %s is not on %s; give the merge commit once the PR is on main", sha, main)
	}
	return full, nil
}

func containsItem(list []*work.Item, it *work.Item) bool {
	for _, x := range list {
		if x.ID == it.ID {
			return true
		}
	}
	return false
}

func cmdProjectClose(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	name := p.Get("name")
	prec, err := r.project(name)
	if err != nil {
		return "", err
	}
	epicID := prec.Bead()
	epic := r.item(epicID)
	if epic == nil {
		return "", refuse("project %s has bead %s, which is not in the work store", name, epicID)
	}
	if epic.Status == work.Closed {
		return "", refuse("project %s (%s) is already closed", name, epicID)
	}
	head, err := r.requireCommitted(prec.Path)
	if err != nil {
		return "", err
	}
	text := records.SectionText(prec.Body, "Outcome")
	if text == "" || text == records.NotClosed {
		return "", refuse("%s: the committed ## Outcome is still '%s'; write it and commit", r.rel(prec.Path),
			records.NotClosed)
	}
	var open []string
	for _, k := range r.x.Children(epicID) {
		if k.Status != work.Closed {
			open = append(open, fmt.Sprintf("%s (%s)", k.ID, k.Title))
		}
	}
	if open != nil {
		return "", refuse("open sprints in the project: %s; close them with pm sprint close first", strings.Join(open, ", "))
	}
	reason := fmt.Sprintf("%s (commit %s)", firstSentence(text, 200), head)
	if err := r.checkPlanned(nil, r.with(closedCopy(epic, reason))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(epicID, reason, work.Done, closer(epic)); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed %s: %s", epicID, reason), nil
}

// ---------------------------------------------------------------- tasks

// openSprint is the record of an open sprint item.
func (r *repo) openSprint(id string) (*records.Record, error) {
	rec, err := r.sprint(id)
	if err != nil {
		return nil, err
	}
	it := r.item(id)
	if it == nil || !isEpic(it) {
		return nil, refuse("sprint %s is not a sprint in the work store", id)
	}
	if it.Status == work.Closed {
		return nil, refuse("sprint %s is closed; use an open sprint", id)
	}
	return rec, nil
}

// openTask is an open item that is no project or sprint.
func (r *repo) openTask(id string) (*work.Item, error) {
	it := r.item(id)
	if it == nil {
		return nil, refuse("%s is not in the work store", id)
	}
	if isEpic(it) {
		return nil, refuse("%s is an epic; close a sprint with pm sprint close, a project with pm project close", id)
	}
	if it.Status == work.Closed {
		return nil, refuse("task %s is already closed", id)
	}
	return it, nil
}

func cmdTaskAdd(e *env, p *Parsed) (string, error) {
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	desc := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprintID := p.Get("sprint")
	if _, err := r.openSprint(sprintID); err != nil {
		return "", err
	}
	if err := r.checkPlanned(nil, r.with(planned("pm-planned-task", work.Task, sprintID, title))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Task, Parent: sprintID, Title: title, Description: desc})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("created task %s in sprint %s; claim it with pm task claim %s", made.ID, sprintID, made.ID), nil
}

func cmdTaskClose(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("task_id")
	task, err := r.openTask(id)
	if err != nil {
		return "", err
	}
	if task.Type == work.Need {
		return "", refuse("%s is a need; answer a decision with pm decision add --need %s (or pm "+
			"decision close if the answer sets no rule), close an action with pm action done %s", id, id, id)
	}
	if err := heldElsewhere(id, task, currentSession(),
		" to close it, or to release it with pm task release "+id); err != nil {
		return "", err
	}
	reason := strip(p.Get("reason"))
	ref, _ := given(p, "commit")
	resolution := work.Done
	if p.Get("dropped") != "" {
		if ref != "" {
			return "", refuse("--dropped closes %s as not done, so no commit holds its work; drop --commit", id)
		}
		if reason == "" {
			return "", refuse("--dropped needs --reason: why %s is dropped and not done", id)
		}
		resolution = work.Dismissed
	} else {
		if reason == "" {
			reason = "Done"
		}
		commit := ""
		if ref != "" {
			if commit, err = closeCommit(r.root, ref); err != nil {
				return "", err
			}
		} else {
			// HEAD names the work only if it was committed after the task started.
			started := task.StartedAt
			if started.IsZero() {
				started = task.CreatedAt
			}
			if out, err := store.Git(r.root, "log", "-1", "--format=%h %cI"); err == nil {
				head, when, _ := strings.Cut(out, " ")
				if t, err := time.Parse(time.RFC3339, when); err == nil && head != "" && !t.Before(started) {
					commit = "commit " + head
				}
			}
			if commit == "" {
				fmt.Fprintf(e.stderr, "warning: no commit since %s started, so the reason names none; name one with "+
					"--commit if the work is committed\n", id)
			}
			// HEAD stands in for the work, so a dirty tree may hold work it lacks; a named commit is the work.
			status, err := store.Git(r.root, "status", "--porcelain")
			if err != nil {
				return "", err
			}
			if status != "" {
				fmt.Fprintf(e.stderr, "warning: the working tree has uncommitted changes; the commit may not contain "+
					"the work of %s\n", id)
			}
		}
		if commit != "" {
			reason += " (" + commit + ")"
		}
	}
	closed := closedCopy(task, reason)
	closed.Resolution = resolution
	if err := r.checkPlanned(nil, r.with(closed)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(id, reason, resolution, closer(task)); err != nil {
		return "", err
	}
	if resolution == work.Dismissed {
		return fmt.Sprintf("dropped %s: %s", id, reason), nil
	}
	return fmt.Sprintf("closed %s: %s", id, reason), nil
}

var (
	otherRepoCommit = regexp.MustCompile(`^([\w.-]+/[\w.-]+)@([0-9a-fA-F]{4,40})$`)
	pullURL         = regexp.MustCompile(`^https://github\.com/([\w.-]+/[\w.-]+)/pull/[0-9]+$`)
)

// ghTimeout bounds one gh call of pm task close.
const ghTimeout = 60 * time.Second

// closeCommit is how a close names the work --commit ref gives: "commit <short>" for a commit of this repo,
// "commit OWNER/REPO@<short>" for OWNER/REPO@SHA and "PR <url>" (with its merge commit once merged) for a PR URL, the
// last two resolved on GitHub with gh. A ref that does not resolve is refused.
func closeCommit(root, ref string) (string, error) {
	if out, err := store.Git(root, "rev-parse", "--verify", "--quiet", "--short", ref+"^{commit}"); err == nil {
		return "commit " + out, nil // this repo's first: a branch may be named like OWNER/REPO@SHA
	}
	if m := otherRepoCommit.FindStringSubmatch(ref); m != nil {
		var c struct {
			SHA string `json:"sha"`
		}
		if err := ghJSON(root, &c, "api", "repos/"+m[1]+"/commits/"+m[2]); err != nil {
			return "", refuse("--commit %s is not a commit of %s on GitHub: %v", ref, m[1], err)
		}
		if len(c.SHA) < 7 {
			return "", refuse("--commit %s is not a commit of %s on GitHub: gh api gave no sha", ref, m[1])
		}
		return "commit " + m[1] + "@" + c.SHA[:7], nil
	}
	if m := pullURL.FindStringSubmatch(strings.TrimSuffix(ref, "/")); m != nil {
		ref = strings.TrimSuffix(ref, "/")
		var pr struct {
			State       string `json:"state"`
			MergeCommit *struct {
				OID string `json:"oid"`
			} `json:"mergeCommit"`
		}
		if err := ghJSON(root, &pr, "pr", "view", ref, "--json", "state,mergeCommit"); err != nil {
			return "", refuse("--commit %s is not a pull request on GitHub: %v", ref, err)
		}
		switch {
		case pr.State == "MERGED" && pr.MergeCommit != nil && len(pr.MergeCommit.OID) >= 7:
			return "PR " + ref + ", merged as " + m[1] + "@" + pr.MergeCommit.OID[:7], nil
		case pr.State == "CLOSED":
			return "", refuse("--commit %s was closed without merging, so it holds no work; name the commit or PR "+
				"that does, or close the task with --dropped", ref)
		}
		return "PR " + ref, nil
	}
	return "", refuse("--commit %s is not a commit of this repo, an OWNER/REPO@SHA on GitHub or a PR URL", ref)
}

// ghJSON runs gh in dir and decodes the JSON it prints into v; a gh that fails, runs past ghTimeout or prints no
// JSON is an error naming why.
func ghJSON(dir string, v any, args ...string) error {
	cmd := "gh " + strings.Join(args, " ")
	res, err := proc.Run(append([]string{"gh"}, args...), proc.Options{Cwd: &dir, Timeout: ghTimeout,
		TimeoutText: strconv.Itoa(int(ghTimeout.Seconds()))})
	switch {
	case err != nil:
		return fmt.Errorf("%s did not run: %v", cmd, err)
	case res.Code != 0:
		why := strings.TrimSpace(res.Stderr)
		if why == "" {
			why = fmt.Sprintf("exit %d", res.Code)
		}
		return fmt.Errorf("%s failed: %s", cmd, why)
	}
	if err := json.Unmarshal([]byte(res.Stdout), v); err != nil {
		return fmt.Errorf("%s printed no JSON: %v", cmd, err)
	}
	return nil
}

func cmdTaskClaim(e *env, p *Parsed) (string, error) {
	id := p.Get("task_id")
	sid := strip(p.Get("session"))
	if sid == "" {
		sid = currentSession()
	}
	if sid == "" {
		return "", noSession()
	}
	root, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	dirs, err := store.Git(root, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gitDir, common, _ := strings.Cut(dirs, "\n")
	if resolvedPath(gitDir) == resolvedPath(common) { // the main checkout; a linked worktree has its own git dir
		cfg, err := config.Load(e.here)
		if err != nil {
			return "", err
		}
		return "", refuse("not claiming %s here: %s is the main checkout; agents change code only in a worktree of their "+
			"own. Make one and work there: `git -C %s fetch %s %s && git -C %s worktree add -b <branch> %s/<name> %s/%s`, "+
			"then `cd %s/%s/<name>` (in Claude Code, the EnterWorktree tool does the same); pm's post-checkout hook and "+
			"session start link its records/ (else run `pm init` there). A subagent works in its parent's worktree.",
			id, root, root, cfg.Remote, cfg.MainBranch, root, worktreesDir, cfg.Remote, cfg.MainBranch, root, worktreesDir)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	task, err := r.openTask(id)
	if err != nil {
		return "", err
	}
	if err := heldElsewhere(id, task, sid, ""); err != nil {
		return "", err
	}
	h := holderOf(task)
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	// Python pm records no host; the host comes with liveness across hosts (work-store page, Open questions).
	if err := ws.Claim(id, work.Holder{Session: sid}, live); err != nil {
		return "", err
	}
	claimed, err := ws.Get(id) // the claim time the store stamped
	if err != nil {
		return "", err
	}
	now := claimed[0].Holder.ClaimedAt.UTC().Format("2006-01-02T15:04:05Z")
	was := ""
	if h != nil && h.Session != sid {
		was = "; took it over from idle session " + h.Session
	}
	return fmt.Sprintf("claimed %s for session %s at %s%s", id, sid, now, was), nil
}

// heldElsewhere refuses a write to task id while a live session other than sid holds it; then says what to ask that
// session for.
func heldElsewhere(id string, task *work.Item, sid, then string) error {
	h := holderOf(task)
	if h != nil && h.Session != sid && h.Live {
		return refuse("%s is held by live session %s (claimed %s ago; its transcript was written in the last %d "+
			"minutes); leave it, or ask that session%s", id, h.Session, age(h.ClaimedAt), liveWindow/60, then)
	}
	return nil
}

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
		r.rel(rec.Path)), "", "pm: ")
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
		return r.apply(w, "finished moving "+moveText(r, id, mv)+" (the work store held it already)", "", "pm: ")
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
	out, err := r.apply(w, "moved "+moveText(r, id, mv), "", "pm: ")
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

// moveWrites is a move's records step, one commit: the sprint record renamed to <project>-<number>, its text
// unchanged, and the move as a source=agent decision in both projects' records.
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
	if exists(path) {
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
			"number in front of it", pyRepr(title))
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
		return "", refuse("sprint %s is titled %s already", id, pyRepr(title))
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
	out, err := r.apply(w, fmt.Sprintf("renamed sprint %s to %s and added a sprint decision to %s", id, pyRepr(full),
		r.rel(rec.Path)), "", "pm: ")
	if err != nil {
		if uerr := ws.Edit(id, &sp.Title, nil); uerr != nil {
			return "", fmt.Errorf("%w; putting back the title %s failed too: %v", err, pyRepr(sp.Title), uerr)
		}
		return "", fmt.Errorf("%w; the work-store step is undone: %s is titled %s again", err, id, pyRepr(sp.Title))
	}
	return out, nil
}

// ---------------------------------------------------------------- pm commit

func cmdCommit(e *env, p *Parsed) (string, error) {
	message := strip(p.Get("message"))
	if message == "" {
		return "", refuse("-m is empty; say what the hand edit changed")
	}
	dirty, err := gitPorcelain(e.records)
	if err != nil {
		return "", err
	}
	if dirty == "" {
		return "", refuse("nothing to commit in %s", e.records)
	}
	lines := strings.Split(dirty, "\n")
	names := p.values["paths"]
	if len(names) == 0 {
		listing := make([]string, len(lines))
		for i, l := range lines {
			listing[i] = "  " + l[:3] + "records/" + l[3:]
		}
		return "", refuse("name the records you edited: pm commit -m \"…\" <path>…; other sessions' edits may be in the "+
			"store too. Uncommitted now:\n%s", strings.Join(listing, "\n"))
	}
	changed := map[string]bool{}
	for _, l := range lines {
		parts := strings.Split(l[3:], " -> ")
		changed[strings.Trim(parts[len(parts)-1], `"`)] = true
	}
	root := resolvedPath(e.records)
	var paths []string
	for _, name := range names {
		g := name
		top, under, _ := strings.Cut(name, "/")
		if !filepath.IsAbs(g) && top == "records" {
			g = filepath.Join(e.records, under) // records/... from any worktree, its link may not exist yet
		}
		if !filepath.IsAbs(g) {
			g = filepath.Join(e.here, g)
		}
		path := resolvedPath(g)
		inside, err := filepath.Rel(root, path)
		outside := err != nil || inside == ".." || strings.HasPrefix(inside, "../")
		// A path relative to the store, such as sprints/x.md, from anywhere outside it.
		rel := filepath.ToSlash(filepath.Clean(name))
		if outside && !filepath.IsAbs(name) && rel != ".." && !strings.HasPrefix(rel, "../") &&
			(changed[rel] || exists(filepath.Join(e.records, rel))) {
			path, inside, outside = resolvedPath(filepath.Join(e.records, rel)), rel, false
		}
		if outside {
			return "", refuse("%s is not in the records store %s", name, e.records)
		}
		inside = filepath.ToSlash(inside)
		if inside == "." {
			return "", refuse("%s is the records store itself; name the records you edited: pm commit -m \"…\" "+
				"records/<…>.md…", name)
		}
		if !changed[inside] {
			return "", refuse("%s has no uncommitted change", name)
		}
		paths = append(paths, path)
	}
	codeRoot, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	changes := map[string]*string{}
	for _, path := range paths {
		if b, err := os.ReadFile(path); err == nil {
			text := string(b)
			changes[path] = &text
		} else if errors.Is(err, os.ErrNotExist) {
			if err := records.LinkToNothing(root, path); err != nil {
				return "", err
			}
			changes[path] = nil
		} else {
			return "", err
		}
		if strings.HasSuffix(filepath.Base(path), ".summary.json") && exists(path) {
			if _, err := records.ReadSummary(path); err != nil {
				return "", err
			}
		}
	}
	recs, err := store.Committed(e.records, changes)
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	items, err := ws.Items()
	if err != nil {
		return "", err
	}
	if _, err := site.Check(recs, records.NewItems(items), filepath.Base(codeRoot), nil); err != nil {
		return "", err
	}
	head, err := store.Commit(e.records, message, paths)
	if err != nil {
		return "", err
	}
	rels := make([]string, len(paths))
	for i, path := range paths {
		rel, _ := filepath.Rel(root, path)
		rels[i] = "records/" + filepath.ToSlash(rel)
	}
	return fmt.Sprintf("committed %s as %s: %s", strings.Join(rels, ", "), head, message), nil
}

// gitPorcelain is git status --porcelain --untracked-files=all in dir, each line's status columns kept.
func gitPorcelain(dir string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status failed in %s: %s", dir, strip(errOut.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
