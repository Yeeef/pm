package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/designindex"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

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
		if !slices.Contains(allowed, name) {
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
	//nolint:gocritic // weakCond: FindStringSubmatch gives nil or a slice holding every group
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
	n, suffix, err := r.nextSprintNumber(project, epicID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.records, "sprints", fmt.Sprintf("%s-%d.md", project, n))
	if fsutil.Exists(path) {
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
		"pm: ")
	if err != nil {
		return "", undoCreate(ws, made.ID, err)
	}
	return out + designindex.Block(func() (string, error) { return designindex.Of(r.recs, e.siteURL, prec) }), nil
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

// sprintReviews is each PR review naming the sprint, open or closed, by id.
func (r *repo) sprintReviews(sprintID string) []*work.Item {
	var out []*work.Item
	for i := range r.items {
		it := &r.items[i]
		if it.Need != nil && it.Need.Review != nil && slices.Contains(it.Need.Review.Sprints, sprintID) {
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
			return "", refuse("--pr %s is not a URL; give the pull request's link", strconv.Quote(pr))
		}
		var err error
		if mergedSHA, err = onMain(e, "--merged", strip(merged)); err != nil { // before the records lock: it fetches
			return "", err
		}
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	if err := store.FetchOtherDrafts(e.records, cfg.Remote); err != nil { // before the records lock: it fetches
		return "", err
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
	line, err := r.sprintOutcome(rec)
	if err != nil {
		return "", err
	}
	reviews, err := r.closedSprintReviews(id)
	if err != nil {
		return "", err
	}
	discard, err := discards(p)
	if err != nil {
		return "", err
	}
	waived, err := r.waive(rec, discard) // the gate reads --discard's waivers
	if err != nil {
		return "", err
	}
	merges, err := r.sprintMerges(id, reviews, agentMerge, mergedSHA, pr, prGiven)
	if err != nil {
		return "", err
	}
	closing, err := design.PlanClose(e.here, r.records, r.root, rec, discard)
	if err != nil {
		return "", err
	}
	w, stamp, message := r.sprintStamp(rec, waived, merges, closing)
	if err := r.checkPlanned(w, r.with(closedCopy(epic, fmt.Sprintf("%s (records commit %s)", line, head)))); err != nil {
		return "", err
	}
	if w != nil {
		if _, err := r.apply(w, message, ""); err != nil {
			return "", err
		}
		if head, err = store.Git(r.records, "rev-parse", "--short", "HEAD"); err != nil {
			return "", err
		}
	}
	if err := closeDone(closing, r.records); err != nil {
		return "", fmt.Errorf("%w; the close's records commit is %s, so run pm sprint close %s again", err, head, id)
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
	if stamp != "" {
		out += "; stamped the Outcome: " + stamp
	}
	if said := closing.Said(discard); said != "" {
		out += "; " + said
	}
	return out, nil
}

// closeDone finishes the close's drafts (design.Closing.Done); a variable so a test can fail it.
var closeDone = (*design.Closing).Done

func (r *repo) sprintOutcome(rec *records.Record) (string, error) {
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
	return line, nil
}

func (r *repo) closedSprintReviews(id string) ([]*work.Item, error) {
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
		return nil, refuse("open tasks in the sprint: %s; close each with pm task close or move it with pm task move (a PR "+
			"review closes with pm need close <id> --reason \"merged as <sha>\" once the PR is on main)",
			strings.Join(listing, ", "))
	}
	return reviews, nil
}

type sprintMerge struct{ sha, pr string }

func (r *repo) sprintMerges(id string, reviews []*work.Item, agentMerge bool, mergedSHA, pr string, prGiven bool) ([]sprintMerge, error) {
	var merges []sprintMerge
	for _, rv := range reviews {
		if rv.Resolution == work.Dismissed { // a replaced PR's review, or a [TEST] one: it delivers nothing
			continue
		}
		if _, err := r.designGate(rv.Need.Review.PR, rv.Need.Review.Sprints, design.PRFiles(r.root, rv.Need.Review.PR)); err != nil {
			return nil, err
		}
		if agentMerge {
			return nil, refuse("sprint %s holds PR review %s (%s), whose close stamps the merge; close it with pm action "+
				"done %s --reason \"merged as <sha>\" if it is not closed yet, then run pm sprint close %s without --merged",
				id, rv.ID, rv.Need.Review.PR, rv.ID, id)
		}
		m := mergedAs.FindStringSubmatch(rv.CloseReason)
		if m == nil {
			why := "no reason"
			if rv.CloseReason != "" {
				why = rv.CloseReason
			}
			return nil, refuse("PR review %s (%s) is closed as %s, not 'merged as <sha>'; a sprint closes only once each PR "+
				"delivering it is on main, so the close stamps its merge commit into the Outcome; once its work is on main, "+
				"restamp it: pm need close %s --reason \"merged as <sha>\"", rv.ID, rv.Need.Review.PR, strconv.Quote(why), rv.ID)
		}
		pr := rv.Need.Review.PR
		if strings.Contains(pr, "/pull/") {
			pr = site.PRLabel(pr)
		}
		merges = append(merges, sprintMerge{m[1][:7], pr})
	}
	if agentMerge {
		named, files := "commit "+mergedSHA[:7], design.MergeFiles(r.root, mergedSHA)
		if prGiven {
			named, files = pr, design.PRFiles(r.root, pr)
		}
		if _, err := r.designGate(named, []string{id}, files); err != nil {
			return nil, err
		}
		label := ""
		if prGiven {
			label = pr
			if strings.Contains(pr, "/pull/") {
				label = site.PRLabel(pr)
			}
		}
		merges = append(merges, sprintMerge{mergedSHA[:7], label})
	}
	return merges, nil
}

// discards is --discard's pages, each with its --reason, in one line.
func discards(p *Parsed) (map[string]string, error) {
	slugs, reasons := p.Values["discard"], p.Values["reason"]
	if len(slugs) != len(reasons) {
		return nil, refuse("give one --reason with each --discard: %d --discard, %d --reason", len(slugs), len(reasons))
	}
	out := map[string]string{}
	for i, slug := range slugs {
		reason, err := decisionLine("--reason", reasons[i])
		if err != nil {
			return nil, err
		}
		out[slug] = reason
	}
	return out, nil
}

// waive is the sprint record's text with --discard's waivers (design.Waive); it reloads the records with that text,
// for the gate.
func (r *repo) waive(rec *records.Record, discard map[string]string) (string, error) {
	text, err := design.Waive(rec, discard, func(body string) string { return decisionBlock("agent", body) })
	if err != nil || text == rec.Text {
		return text, err
	}
	r.recs, err = store.Committed(r.records, map[string]*string{rec.Path: &text})
	return text, err
}

func (r *repo) sprintStamp(rec *records.Record, text string, merges []sprintMerge,
	closing *design.Closing) ([]store.Write, string, string) {
	stamps, shas := make([]string, len(merges)), make([]string, len(merges))
	for i, m := range merges {
		stamps[i] = fmt.Sprintf("Merged as %s (%s).", m.sha, m.pr)
		if m.pr == "" {
			stamps[i] = fmt.Sprintf("Merged as %s.", m.sha)
		}
		shas[i] = m.sha
	}
	stamp := strings.Join(stamps, " ")
	if merges != nil && !strings.Contains(text, stamp) {
		text = records.MergeStamp(text, stamp)
	} else {
		stamp = "" // stamped by a close run before
	}
	var w []store.Write
	if text != rec.Text {
		w = []store.Write{{Path: rec.Path, Text: text}}
	}
	w = append(w, closing.Writes(r.records)...)
	proj := records.ProjectOf(rec, r.recs, r.x)
	number := rec.Name()
	projName := ""
	if proj != nil {
		projName = proj.Name()
		if m := regexp.MustCompile(`^` + regexp.QuoteMeta(projName) + `-([0-9]+)$`).FindStringSubmatch(rec.Name()); m != nil {
			number = m[1]
		}
	}
	message := fmt.Sprintf("[SPRINT] %s sprint %s: closed", projName, number)
	if len(shas) > 0 {
		message += ", merged as " + strings.Join(shas, ", ")
	}
	if trailer := closing.Trailer(rec.Bead()); trailer != "" {
		message += "\n\n" + trailer
	}
	return w, stamp, message
}

// shaRE is a commit's hex sha, as --merged takes it.
var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// onMain is the full sha of the commit sha names, refused unless it is on the remote's main branch, fetched first;
// given names the sha in a refusal (--merged, merged as).
func onMain(e *env, given, sha string) (string, error) {
	if !shaRE.MatchString(sha) {
		return "", refuse("%s %s is not a commit sha: 7 to 40 hex digits", given, strconv.Quote(sha))
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
	if _, err := store.FetchMain(root, cfg.Remote, cfg.MainBranch); err != nil {
		return "", refuse("%s %s: pm cannot see %s, so it cannot check the commit is on it: %v", given, sha, main, err)
	}
	full, err := store.Git(root, "rev-parse", "--verify", "--quiet", sha+"^{commit}")
	if err != nil {
		return "", refuse("%s %s is no commit of this repo, %s included", given, sha, main)
	}
	if _, err := store.Git(root, "merge-base", "--is-ancestor", full, "refs/remotes/"+main); err != nil {
		return "", refuse("%s %s is not on %s; give the merge commit once the PR is on main", given, sha, main)
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

func (r *repo) nextSprintNumber(project, epicID string) (int, int, error) {
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
			n, err := strconv.Atoi(last)
			if err != nil {
				return 0, 0, fmt.Errorf("item %s: %w", k.ID, err)
			}
			suffix = max(suffix, n)
		}
	}
	n := max(kids, movedAway)
	name := regexp.MustCompile(`^` + regexp.QuoteMeta(project) + `-([0-9]+)$`)
	for _, rec := range r.recs {
		if m := name.FindStringSubmatch(rec.Name()); rec.Type() == "sprint" && m != nil {
			v, err := strconv.Atoi(m[1])
			if err != nil {
				return 0, 0, fmt.Errorf("record %s: %w", rec.Name(), err)
			}
			n = max(n, v)
		}
	}
	n++
	if top+1 != n {
		return 0, 0, refuse("the next sprint of %s is %d by its records and %d by the work store's sprint numbers; "+
			"fix the records or the work store first", project, n, top+1)
	}
	return n, suffix, nil
}
