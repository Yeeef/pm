package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"slices"
	"strings"
)

// decisionTarget is the open project or sprint record a decision of --level goes into, named by its flag.
func (r *repo) decisionTarget(p *Parsed) (*records.Record, error) {
	level := p.Get("level") // required by the parser
	flag, other := "--sprint", "--project"
	if level == "project" {
		flag, other = "--project", "--sprint"
	}
	if _, ok := given(p, other[2:]); ok {
		return nil, refuse("%s does not match --level %s; name the target with %s", other, level, flag)
	}
	target, ok := given(p, flag[2:])
	if !ok {
		return nil, refuse("--level %s needs %s; nothing is inferred", level, flag)
	}
	var rec *records.Record
	var err error
	if level == "project" {
		rec, err = r.project(target)
	} else {
		rec, err = r.sprint(target)
	}
	if err != nil {
		return nil, err
	}
	bead := rec.Bead()
	if it := r.item(bead); it == nil {
		return nil, refuse("%s %s has bead %s, which is not in the work store", level, target, bead)
	} else if it.Status == work.Closed {
		return nil, refuse("%s %s (%s) is closed; record the decision in an open %s", level, target, bead, level)
	}
	return rec, nil
}

// decisionLine is one part of a decision given by its flag: one non-empty line that opens no block.
func decisionLine(flag, value string) (string, error) {
	value = strip(value)
	switch {
	case value == "":
		return "", refuse("%s is empty; give it one line", flag)
	case strings.Contains(value, "\n"):
		return "", refuse("%s has more than one line; give it one line", flag)
	case strings.HasPrefix(value, ":::"):
		return "", refuse("%s starts with ':::', and blocks cannot nest", flag)
	}
	return value, nil
}

// decisionBlockUntil is a ::: decision block of today, from source, with --until when given.
func decisionBlockUntil(source, body string, p *Parsed) (string, error) {
	until, ok := given(p, "until")
	if !ok {
		return decisionBlock(source, body), nil
	}
	if strip(until) == "" || strings.ContainsAny(until, "\"\n") {
		return "", refuse("--until must be one non-empty line without double quotes")
	}
	return fmt.Sprintf("::: decision {source=%s date=%s until=\"%s\"}\n%s\n:::", source, store.Today(), strip(until),
		body), nil
}

func cmdDecisionAdd(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	rec, err := r.decisionTarget(p)
	if err != nil {
		return "", err
	}
	decision, err := decisionLine("--decision", p.Get("decision"))
	if err != nil {
		return "", err
	}
	reason, err := decisionLine("--reason", p.Get("reason"))
	if err != nil {
		return "", err
	}
	body := decision + "\n" + reason
	body, source, need, err := r.decisionSource(p, body)
	if err != nil {
		return "", err
	}
	needID := p.Get("need")
	block, err := decisionBlockUntil(source, body, p)
	if err != nil {
		return "", err
	}
	updated, err := records.InsertEntry(rec.Text, "Decisions", block)
	if err != nil {
		return "", err
	}
	done := fmt.Sprintf("added a source=%s %s decision to %s", source, p.Get("level"), r.rel(rec.Path))
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if need != nil && need.Resolution == work.NoDecision { // a rule after all: it is answered, as its decision says
		cited := *need
		cited.Resolution = work.Answered
		if err := r.checkPlanned(w, r.with(cited)); err != nil {
			return "", err
		}
		ws, err := e.work()
		if err != nil {
			return "", err
		}
		if err := ws.SetResolution(needID, work.Answered); err != nil {
			return "", err
		}
		out, err := r.apply(w, fmt.Sprintf("marked need %s answered and %s", needID, done), "pm: ")
		if err != nil {
			return "", fmt.Errorf("%w; the work-store step stands: %s is marked answered; record the decision again "+
				"with pm decision add --need %s and the same flags", err, needID, needID)
		}
		return out, nil
	}
	if need == nil || need.Status == work.Closed {
		if err := r.checkPlanned(w, nil); err != nil {
			return "", err
		}
		return r.apply(w, done, "pm: ")
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	if err := r.checkPlanned(w, r.with(answered(need, work.Answered, "Responded"))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Answer(needID, body, currentSession()); err != nil {
		return "", err
	}
	out, err := r.apply(w, fmt.Sprintf("closed need %s and %s", needID, done), "pm: ")
	if err != nil { // the work store has no reopen: say what stands and how to finish
		return "", fmt.Errorf("%w; the work-store step stands: %s is closed as answered with this decision; record the "+
			"decision again with pm decision add --need %s and the same flags", err, needID, needID)
	}
	return out, nil
}

// noDecision is the note a need gets when its answer sets no rule.
const noDecisionNote = "No decision record: "

func cmdDecisionClose(e *env, p *Parsed) (string, error) {
	answer := p.Get("text")
	why := strip(p.Get("reason"))
	if why == "" {
		return "", refuse("--reason is empty; say why the answer sets no rule")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("need_id")
	need, err := r.humanIssue(id, "decision")
	if err != nil {
		return "", err
	}
	if need.Resolution == work.NoDecision {
		return "", refuse("need %s is already marked no-decision", id)
	}
	if need.Status == work.Closed && need.Resolution == work.Dismissed {
		return "", refuse("need %s was dismissed, so it has no answer to mark", id)
	}
	if need.Status != work.Closed && answer == "" {
		return "", refuse("the answer is empty; pass the owner's answer, as they gave it, with %s; a need that became "+
			"moot before the owner answered closes with pm need dismiss %s --reason \"…\"", textForms, id)
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	note := noDecisionNote + why
	reason := need.CloseReason
	if reason == "" {
		reason = "Responded"
	}
	planned := answered(need, work.NoDecision, reason)
	if need.Status == work.Closed {
		planned = *need
		planned.Resolution = work.NoDecision
	}
	if err := r.checkPlanned(nil, r.with(planned)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	retry := fmt.Sprintf("run the same command again to label it: pm decision close %s --reason \"%s\"", id, why)
	var done, failed string
	if need.Status == work.Closed {
		if !slices.ContainsFunc(need.Comments, func(c work.Comment) bool { return strings.Contains(c.Text, note) }) {
			if _, err := ws.Comment(id, work.Note, noteAuthor(), note); err != nil {
				return "", err
			}
		}
		done = fmt.Sprintf("marked need %s, already answered, as setting no rule: %s", id, why)
		failed = fmt.Sprintf("the reason is on %s as a comment; %s (it does not add the note twice)", id, retry)
	} else {
		if err := ws.Answer(id, answer+"\n\n"+note, currentSession()); err != nil {
			return "", err
		}
		done = fmt.Sprintf("closed need %s with the owner's answer and no decision record: %s", id, why)
		failed = fmt.Sprintf("need %s is closed with the answer but not labelled no-decision, so the render check "+
			"flags it; %s", id, retry)
	}
	if err := ws.SetResolution(id, work.NoDecision); err != nil {
		return "", fmt.Errorf("%w; %s", err, failed)
	}
	return fmt.Sprintf("%s; if it sets a rule after all, record it with pm decision add --need %s, which marks the "+
		"need answered", done, id), nil
}

func (r *repo) decisionSource(p *Parsed, body string) (string, string, *work.Item, error) {
	var err error
	source := "agent"
	var need *work.Item
	needID, withNeed := given(p, "need")
	if withNeed && needID != "" {
		if need, err = r.humanIssue(needID, "decision"); err != nil {
			return "", "", nil, err
		}
		if need.Status == work.Closed && need.Resolution == work.Dismissed {
			return "", "", nil, refuse("need %s was dismissed, so it has no answer to record", needID)
		}
		source = "owner"
		if !site.Cites(body, needID) {
			body += fmt.Sprintf("\nAnswers `%s`.", needID)
		}
	} else if p.Get("confirmed") != "" {
		source = "owner"
	}
	return body, source, need, nil
}
