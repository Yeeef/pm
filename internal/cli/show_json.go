package cli

import (
	"encoding/json"
	"strings"

	"github.com/Yeeef/pm/internal/records"
)

// pm show --json: the top level as JSON, each object's keys in its struct's field order.

type showJSON struct {
	Site         string            `json:"site"`
	Projects     []showProjectJSON `json:"projects"`
	Push         []string          `json:"push"`
	Today        showTodayJSON     `json:"today"`
	Feedback     *showFeedbackJSON `json:"feedback"`
	RepoFeedback *showFeedbackJSON `json:"repo_feedback"`
}

type showProjectJSON struct {
	Name      string             `json:"name"`
	Bead      string             `json:"bead"`
	Title     string             `json:"title"`
	URL       string             `json:"url"`
	Goal      string             `json:"goal"`
	Sprints   []showSprintJSON   `json:"sprints"`
	Needs     []showNeedJSON     `json:"needs"`
	Decisions []showDecisionJSON `json:"decisions"`
}

type showSprintJSON struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Title         string         `json:"title"`
	State         string         `json:"state"`
	Record        *string        `json:"record"`
	URL           *string        `json:"url"`
	Done          int            `json:"done"`
	Total         int            `json:"total"`
	Goal          string         `json:"goal"`
	DoneWhenItems int            `json:"done_when_items"`
	DoneWhen      string         `json:"done_when"`
	Tasks         []showTaskJSON `json:"tasks"`
}

type showTaskJSON struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	State     string          `json:"state"`
	Status    string          `json:"status"`
	Human     bool            `json:"human"`
	Kind      string          `json:"kind"`
	BlockedBy []string        `json:"blocked_by"`
	Holder    *showHolderJSON `json:"holder"`
}

type showHolderJSON struct {
	Session   string  `json:"session"`
	Assignee  *string `json:"assignee"` // always null: the work store's holder is a session
	ClaimedAt string  `json:"claimed_at"`
	Live      bool    `json:"live"`
}

type showNeedJSON struct {
	Sprint  *string `json:"sprint"`
	Task    *string `json:"task"`
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Kind    string  `json:"kind"`
	Session *string `json:"session"`
	Replied bool    `json:"replied"`
}

type showDecisionJSON struct {
	Date   string `json:"date"`
	Source string `json:"source"`
	Level  string `json:"level"`
	Record string `json:"record"`
	Text   string `json:"text"`
}

type showTodayJSON struct {
	Date        string  `json:"date"`
	Page        string  `json:"page"`
	Summary     *string `json:"summary"`
	GeneratedAt *string `json:"generated_at"`
}

type showFeedbackJSON struct {
	Entries int    `json:"entries"`
	URL     string `json:"url"`
}

// showOpt is s, or null when it is "".
func showOpt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *holder) json() *showHolderJSON {
	if h == nil {
		return nil
	}
	return &showHolderJSON{Session: h.Session, ClaimedAt: h.ClaimedAt.UTC().Format("2006-01-02T15:04:05Z"), Live: h.Live}
}

func (d *showData) json() showJSON {
	out := showJSON{Site: d.site, Projects: []showProjectJSON{}, Push: append([]string{}, d.push...),
		Today: showTodayJSON{Date: d.today, Page: "days/" + d.today + ".html"}}
	for _, p := range d.projects {
		pj := showProjectJSON{Name: p.name, Bead: p.bead, Title: p.title, URL: p.url, Goal: p.goal,
			Sprints: []showSprintJSON{}, Needs: []showNeedJSON{}, Decisions: []showDecisionJSON{}}
		for _, s := range p.sprints {
			sj := showSprintJSON{ID: s.id, Name: s.name, Title: s.title, State: s.state, Record: s.record, URL: s.url,
				Done: s.done, Total: s.total, Goal: s.goal, DoneWhenItems: s.doneWhenItems, DoneWhen: s.doneWhen,
				Tasks: []showTaskJSON{}}
			for _, t := range s.tasks {
				sj.Tasks = append(sj.Tasks, showTaskJSON{ID: t.id, Title: t.title, State: t.state, Status: t.status,
					Human: t.human, Kind: t.kind, BlockedBy: append([]string{}, t.blockedBy...), Holder: t.holder.json()})
			}
			pj.Sprints = append(pj.Sprints, sj)
		}
		for _, n := range p.needs {
			pj.Needs = append(pj.Needs, showNeedJSON{Sprint: showOpt(n.sprint), Task: showOpt(n.task), ID: n.id,
				Title: n.title, Kind: n.kind, Session: showOpt(n.session), Replied: n.replied})
		}
		for _, x := range p.decisions {
			pj.Decisions = append(pj.Decisions, showDecisionJSON{x.date, x.source, x.level, x.record, x.text})
		}
		out.Projects = append(out.Projects, pj)
	}
	if d.summary != nil {
		summary := firstSentence(records.SummaryLine(d.summary.Text), 160)
		out.Today.Summary, out.Today.GeneratedAt = &summary, &d.summary.GeneratedAt
	}
	if f := d.feedback.pm; f != nil {
		out.Feedback = &showFeedbackJSON{f.entries, f.url}
	}
	if f := d.feedback.repo; f != nil {
		out.RepoFeedback = &showFeedbackJSON{f.entries, f.url}
	}
	return out
}

// showDumps is v as JSON, indented one space a level, with non-ASCII and <, > and & as they are.
func showDumps(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(v); err != nil {
		panic(err) // showJSON always encodes
	}
	return strings.TrimSuffix(b.String(), "\n")
}
