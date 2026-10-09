package cli

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	pmsync "github.com/Yeeef/pm/internal/sync"
	"github.com/Yeeef/pm/internal/work"
)

// cmdShow is pm show: every level. Python source: show_data, show_text, show_project_text, sprint_detail,
// record_section and cmd_show in cli.py.
func cmdShow(e *env, p *Parsed) (string, error) {
	if p.Get("record") != "" || p.Get("section") != "" {
		if p.Get("record") == "" || p.Get("section") == "" || p.Get("sprint") != "" || p.Get("project") != "" ||
			p.Get("json") != "" {
			return "", refuse("--record and --section go together, without --sprint, --project or --json")
		}
		recs, err := records.Read(e.records, nil)
		if err != nil {
			return "", err
		}
		rec, err := linkTarget(recs, e.records, p.Get("record"))
		if err != nil {
			return "", err
		}
		return recordSection(rec, p.Get("section"))
	}
	r, err := e.load(true)
	if err != nil {
		return "", err
	}
	if id := p.Get("sprint"); id != "" {
		if p.Get("json") != "" || p.Get("project") != "" {
			return "", refuse("--sprint prints text only, one sprint; drop --json and --project")
		}
		return sprintDetail(r, id)
	}
	if p.Get("refresh_inbox") != "" {
		if err := refreshInbox(e, r); err != nil {
			return "", err
		}
	}
	data, err := showDataOf(e, r)
	if err != nil {
		return "", err
	}
	if name := p.Get("project"); name != "" {
		if p.Get("json") != "" {
			return "", refuse("--project prints text only; drop --json")
		}
		for _, pr := range data.projects {
			if name == pr.name || name == pr.bead {
				return showProjectText(pr), nil
			}
		}
		open := make([]string, len(data.projects))
		for i, pr := range data.projects {
			open[i] = pr.name
		}
		list := strings.Join(open, ", ")
		if list == "" {
			list = "none"
		}
		return "", refuse("no open project %s; open ones: %s", pyRepr(name), list)
	}
	if p.Get("json") != "" {
		return showDumps(data.json()), nil
	}
	return showText(data), nil
}

// refreshInbox points this session's open needs at its inbox as of now: a resumed session keeps its id but binds a new
// socket, so pushes to the stored one would fail for the rest of its life. Python source: refresh_inbox.
func refreshInbox(e *env, r *repo) error {
	sid, inbox := currentSession(), strip(os.Getenv(inboxEnv))
	if sid == "" || inbox == "" {
		return nil
	}
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	ws, err := e.work()
	if err != nil {
		return err
	}
	for _, it := range r.items {
		if rb := needRaisedBy(&it); it.Status == work.Open && rb != nil && rb.Session == sid &&
			(rb.Inbox != inbox || rb.Host != host) {
			if err := ws.UpdateNeed(it.ID, work.NeedUpdate{RaisedInbox: &inbox, RaisedHost: &host}); err != nil {
				return err
			}
		}
	}
	return nil
}

func needRaisedBy(it *work.Item) *work.RaisedBy {
	if it.Need == nil {
		return nil
	}
	return it.Need.RaisedBy
}

// cmdRecordLink is pm record link: the rendered page's URL once the pm service for this store answers on its port;
// otherwise the command that fixes it. Python source: link_target and cmd_record_link in cli.py.
func cmdRecordLink(e *env, p *Parsed) (string, error) {
	recs, err := records.Read(e.records, nil)
	if err != nil {
		return "", err
	}
	rec, err := linkTarget(recs, e.records, p.Get("target"))
	if err != nil {
		return "", err
	}
	main := store.MainOf(e.records)
	fix := "pm service install"
	if service.Installed(main) {
		fix = "pm service restart"
	}
	port, err := showPort(e.here, main)
	if err != nil {
		return "", err
	}
	served := service.Answering(port)
	if served == nil {
		return "", refuse("no site is served on :%d; start it with %s, then run this again", port, fix)
	}
	if served.Store == "" {
		return "", refuse("the server on :%d is not the pm service, so its pages may be stale; stop the old server on "+
			":%d, then %s", port, port, fix)
	}
	if showPosixAbs(served.Store) != resolvePath(e.records) {
		return "", refuse("the pm service on :%d renders another store (%s); stop it, then %s", port, served.Store, fix)
	}
	url, err := showSiteURL(e.here, main)
	if err != nil {
		return "", err
	}
	return url + "/" + rec.Out(), nil
}

// showPosixAbs is Python's Path(p) of a path as a string: repeated and trailing slashes and "." parts dropped.
func showPosixAbs(p string) string {
	if strings.HasPrefix(p, "/") {
		return "/" + strings.TrimPrefix(showPosix(p), ".")
	}
	return showPosix(p)
}

// ---------------------------------------------------------------- the data

type showData struct {
	site     string
	projects []*showProject
	push     []string
	today    string
	summary  *records.Summary
}

type showProject struct {
	name, bead, title, url, goal string
	sprints                      []*showSprint
	needs                        []showNeed
	decisions                    []showDecision
	feedback                     []showFeedback
}

type showSprint struct {
	id, name, title, state string
	record, url            *string
	done, total            int
	goal                   string
	doneWhenItems          int
	doneWhen               string
	tasks                  []showTask
}

type showTask struct {
	id, title, state, status string
	human                    bool
	kind                     string
	blockedBy                []string
	holder                   *holder
}

type showNeed struct {
	sprint, task    string // "" where absent
	id, title, kind string
	session         string // "" when no session raised it
	replied         bool
}

type showDecision struct {
	date, source, level, record, text string
	order                             int
}

type showFeedback struct {
	entries int
	url     string
}

// showShort is Python's short(): an id under `under` without that prefix.
func showShort(id, under string) string {
	if strings.HasPrefix(id, under+".") {
		return id[len(under):]
	}
	return id
}

func showDecisionsOf(rec *records.Record, level string) []showDecision {
	var out []showDecision
	for _, d := range records.Decisions(rec.Text) {
		out = append(out, showDecision{date: d.Attrs["date"], source: d.Attrs["source"], level: level, record: rec.Rel,
			text: firstSentence(d.Body, 100)})
	}
	return out
}

var (
	showSprintNumber = regexp.MustCompile(`-(\d+)$`)
	showListItem     = regexp.MustCompile(`(?m)^(?:[-*]|\d+\.) `)
)

// showDataOf is Python's show_data: every open project with its open sprints, tasks, needs, last decisions and
// feedback, the push flags and today's summary.
func showDataOf(e *env, r *repo) (*showData, error) {
	main := store.MainOf(r.records)
	url, err := showSiteURL(e.here, main)
	if err != nil {
		return nil, err
	}
	sprintRecs := map[string]*records.Record{}
	for _, rec := range r.recs {
		if rec.Type() == "sprint" {
			if id, ok := rec.ID("bead"); ok {
				sprintRecs[id] = rec
			}
		}
	}
	data := &showData{site: url}
	for _, p := range r.recs {
		if p.Type() != "project" {
			continue
		}
		epic := p.Bead()
		pi := r.item(epic)
		if pi == nil {
			return nil, fmt.Errorf("project record %s names %s, which the work store does not hold", p.Rel, pyRepr(epic))
		}
		if pi.Status == work.Closed {
			continue
		}
		decisions := showDecisionsOf(p, "project")
		for i := range decisions {
			decisions[i].order = i
		}
		var sprints []*showSprint
		for _, sp := range r.x.Children(epic) {
			if sp.Type != work.Sprint {
				continue
			}
			rec := sprintRecs[sp.ID]
			name := showShort(sp.ID, epic)
			if rec != nil {
				if m := showSprintNumber.FindStringSubmatch(rec.Name()); m != nil {
					name = "sprint " + m[1]
				} else {
					name = "sprint " + rec.Name()
				}
				for i, d := range showDecisionsOf(rec, name) {
					d.order = len(decisions) + i
					decisions = append(decisions, d)
				}
			}
			if sp.Status == work.Closed {
				continue
			}
			tasks := r.x.Children(sp.ID)
			s := &showSprint{id: sp.ID, name: name, title: sp.Title, state: site.State(sp, r.x), total: len(tasks)}
			doneWhen := ""
			if rec != nil {
				doneWhen = records.SectionText(rec.Body, "Done when")
				rel, out := rec.Rel, url+"/"+rec.Out()
				s.record, s.url = &rel, &out
				s.goal = firstSentence(records.SectionText(rec.Body, "Goal"), 110)
			}
			s.doneWhenItems = len(showListItem.FindAllStringIndex(doneWhen, -1))
			if s.doneWhenItems == 0 {
				s.doneWhen = firstSentence(doneWhen, 110)
			}
			for _, t := range tasks {
				if t.Status == work.Closed {
					s.done++
					continue
				}
				status := string(t.Status)
				if t.Holder != nil {
					status = "in_progress"
				}
				blocked := []string{}
				for _, b := range t.BlockedBy {
					if x := r.item(b); x == nil || x.Status != work.Closed {
						blocked = append(blocked, b)
					}
				}
				s.tasks = append(s.tasks, showTask{id: t.ID, title: t.Title, state: site.State(t, r.x), status: status,
					human: t.Type == work.Need, kind: site.Kind(t), blockedBy: blocked, holder: holderOf(t)})
			}
			sprints = append(sprints, s)
		}
		sort.SliceStable(decisions, func(i, j int) bool {
			if decisions[i].date != decisions[j].date {
				return decisions[i].date < decisions[j].date
			}
			return decisions[i].order < decisions[j].order
		})
		var last []showDecision
		for i := len(decisions) - 1; i >= 0 && i >= len(decisions)-3; i-- {
			last = append(last, decisions[i])
		}
		owner := site.OwnerTasks(r.x, epic)
		sort.SliceStable(owner, func(i, j int) bool { return owner[i].ID < owner[j].ID })
		var needs []showNeed
		for _, n := range owner {
			sid, tid := showRequestPlace(n, r.x, epic)
			session := ""
			if n.Need != nil && n.Need.RaisedBy != nil {
				session = n.Need.RaisedBy.Session
			}
			needs = append(needs, showNeed{sprint: sid, task: tid, id: n.ID, title: n.Title, kind: site.Kind(n),
				session: session, replied: service.ReplyWaiting(n) || service.MergeWaiting(n) != ""})
		}
		var fb []showFeedback
		for _, doc := range feedbackDocs(r.recs, p.Name()) {
			fb = append(fb, showFeedback{entries: len(feedbackEntry.FindAllStringIndex(doc.Body, -1)), url: url + "/" + doc.Out()})
		}
		data.projects = append(data.projects, &showProject{name: p.Name(), bead: epic, title: p.Title(),
			url: url + "/" + p.Out(), goal: firstSentence(records.SectionText(p.Body, "Goal"), 110), sprints: sprints,
			needs: needs, decisions: last, feedback: fb})
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return nil, err
	}
	if data.push, err = pmsync.Flags(main, r.records, cfg.Remote); err != nil {
		return nil, err
	}
	data.today = store.Today()
	summaries, err := records.ReadSummaries(r.records)
	if err != nil {
		return nil, err
	}
	data.summary = summaries[data.today]
	return data, nil
}

// showRequestPlace is site.py's request_place: the sprint a need sits under in the project (the ancestor just below
// it) and its task (its own parent when that is below the sprint); "" where absent.
func showRequestPlace(it *work.Item, items *records.Items, project string) (sprint, task string) {
	chain := items.Ancestors(it.ID)
	var below []string
	for i, a := range chain {
		if a == project {
			below = chain[:i]
			break
		}
	}
	if len(below) > 0 {
		sprint = below[len(below)-1]
	}
	if len(below) > 1 {
		task = below[0]
	}
	return sprint, task
}

// ---------------------------------------------------------------- JSON

func showOpt(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func showOptPtr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func (h *holder) json() any {
	if h == nil {
		return nil
	}
	return showObj{{"session", h.Session}, {"assignee", nil}, {"claimed_at", h.ClaimedAt.UTC().Format("2006-01-02T15:04:05Z")},
		{"live", h.Live}}
}

func (d *showData) json() any {
	projects := []any{}
	for _, p := range d.projects {
		sprints := []any{}
		for _, s := range p.sprints {
			tasks := []any{}
			for _, t := range s.tasks {
				blocked := []any{}
				for _, b := range t.blockedBy {
					blocked = append(blocked, b)
				}
				tasks = append(tasks, showObj{{"id", t.id}, {"title", t.title}, {"state", t.state}, {"status", t.status},
					{"human", t.human}, {"kind", t.kind}, {"blocked_by", blocked}, {"holder", t.holder.json()}})
			}
			sprints = append(sprints, showObj{{"id", s.id}, {"name", s.name}, {"title", s.title}, {"state", s.state},
				{"record", showOptPtr(s.record)}, {"url", showOptPtr(s.url)}, {"done", s.done}, {"total", s.total},
				{"goal", s.goal}, {"done_when_items", s.doneWhenItems}, {"done_when", s.doneWhen}, {"tasks", tasks}})
		}
		needs := []any{}
		for _, n := range p.needs {
			needs = append(needs, showObj{{"sprint", showOpt(n.sprint)}, {"task", showOpt(n.task)}, {"id", n.id},
				{"title", n.title}, {"kind", n.kind}, {"session", showOpt(n.session)}, {"replied", n.replied}})
		}
		decisions := []any{}
		for _, x := range p.decisions {
			decisions = append(decisions, showObj{{"date", x.date}, {"source", x.source}, {"level", x.level},
				{"record", x.record}, {"text", x.text}})
		}
		feedback := []any{}
		for _, f := range p.feedback {
			feedback = append(feedback, showObj{{"entries", f.entries}, {"url", f.url}})
		}
		projects = append(projects, showObj{{"name", p.name}, {"bead", p.bead}, {"title", p.title}, {"url", p.url},
			{"goal", p.goal}, {"sprints", sprints}, {"needs", needs}, {"decisions", decisions}, {"feedback", feedback}})
	}
	push := []any{}
	for _, l := range d.push {
		push = append(push, l)
	}
	var summary, generated any
	if d.summary != nil {
		summary = firstSentence(records.SummaryLine(d.summary.Text), 160)
		generated = d.summary.GeneratedAt
	}
	return showObj{{"site", d.site}, {"projects", projects}, {"push", push}, {"today", showObj{{"date", d.today},
		{"page", "days/" + d.today + ".html"}, {"summary", summary}, {"generated_at", generated}}}}
}

// ---------------------------------------------------------------- text

const (
	showReplyHint = "  [undelivered reply: pm reply read %s]"
	showTitleCut  = 60 // characters of a request's title in the top level; the project level prints it whole
)

// showPlace is where a request line's request sits, compactly: its sprint by name, then its task by short id.
func showPlace(n showNeed, under string, names map[string]string) string {
	if n.sprint == "" {
		return ""
	}
	task := ""
	if n.task != "" {
		task = ", task " + showShort(n.task, under)
	}
	name, ok := names[n.sprint]
	if !ok {
		name = showShort(n.sprint, under)
	}
	return "  (" + name + task + ")"
}

func showSprintNames(p *showProject) map[string]string {
	names := map[string]string{}
	for _, sp := range p.sprints {
		names[sp.id] = sp.name
	}
	return names
}

// showText is the top level: push failures, tasks other live sessions hold, the site, and one line per project with
// each open owner request and undelivered reply.
func showText(d *showData) string {
	var out []string
	if len(d.push) > 0 {
		out = append(out, "warning: the pm service's push needs attention (pm service status; pm service logs):")
		for _, l := range d.push {
			out = append(out, "  "+l)
		}
	}
	me := currentSession()
	var others []string
	for _, p := range d.projects {
		for _, sp := range p.sprints {
			for _, t := range sp.tasks {
				if t.holder != nil && t.holder.Live && t.holder.Session != me {
					others = append(others, "  "+t.id+"  "+holderText(t.holder))
				}
			}
		}
	}
	if len(others) > 0 {
		out = append(out, "warning: other live sessions hold these tasks; do not start or delegate them:")
		out = append(out, others...)
	}
	out = append(out, "site: "+d.site+" (the pm service); a record's page is <site>/<its path under records/, without .md>"+
		".html; pm record link <target> prints one")
	out = append(out, "projects: pm show --project NAME prints one's sprints, tasks, owner requests and last decisions")
	for _, p := range d.projects {
		e := p.bead
		running := 0
		for _, sp := range p.sprints {
			if sp.state == "running" {
				running++
			}
		}
		out = append(out, fmt.Sprintf("  %s  %s  %d open sprints, %d running, %d owner requests", p.name, e,
			len(p.sprints), running, len(p.needs)))
		names := showSprintNames(p)
		for _, n := range p.needs {
			title := n.title
			if r := []rune(title); len(r) > showTitleCut {
				title = string(r[:showTitleCut-1]) + "…"
			}
			line := "    " + n.kind + " " + showShort(n.id, e) + "  " + title + showPlace(n, e, names)
			if n.replied {
				line += fmt.Sprintf(showReplyHint, n.id)
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// showProjectText is the project level: its goal, owner requests in full, feedback, each open sprint with its goal,
// done-when and open tasks, and its last decisions.
func showProjectText(p *showProject) string {
	e := p.bead
	names := showSprintNames(p)
	out := []string{p.name + "  " + e + "  " + p.goal}
	for _, k := range []string{"decision", "action"} {
		var needs []showNeed
		for _, n := range p.needs {
			if n.kind == k {
				needs = append(needs, n)
			}
		}
		out = append(out, fmt.Sprintf("%ss await you (%d):", k, len(needs)))
		for _, n := range needs {
			line := "  " + showShort(n.id, e) + "  " + n.title + showPlace(n, e, names) + "  -> pm show " + n.id
			if n.replied {
				line += fmt.Sprintf(showReplyHint, n.id)
			}
			out = append(out, line)
		}
	}
	for _, f := range p.feedback {
		out = append(out, fmt.Sprintf("feedback: %d entries -> %s", f.entries, f.url))
	}
	out = append(out, `feedback: when pm gets in your way, run pm feedback add --project <p> --text="…"`)
	if len(p.sprints) > 0 || len(p.decisions) > 0 {
		out = append(out, p.name+"  "+e+"  sprints and decisions:")
		var quiet []string
		for _, sp := range p.sprints {
			if len(sp.tasks) == 0 {
				quiet = append(quiet, showShort(sp.id, e)+" "+sp.title)
				continue
			}
			out = append(out, fmt.Sprintf("%s  %s  %s  %d/%d done", sp.title, showShort(sp.id, e), sp.state, sp.done, sp.total))
			heldSet := map[string]bool{}
			for _, t := range sp.tasks {
				if t.holder != nil {
					heldSet[firstRunes(t.holder.Session, 8)] = true
				}
			}
			if len(heldSet) > 0 {
				held := make([]string, 0, len(heldSet))
				for h := range heldSet {
					held = append(held, h)
				}
				sort.Strings(held)
				out = append(out, "  held by: "+strings.Join(held, ", "))
			}
			if sp.goal != "" {
				out = append(out, "  goal: "+sp.goal)
			}
			if sp.doneWhenItems > 0 {
				out = append(out, fmt.Sprintf("  done when: %d items (pm show --sprint %s)", sp.doneWhenItems, sp.id))
			} else if sp.doneWhen != "" {
				out = append(out, "  done when: "+sp.doneWhen)
			}
			for _, t := range sp.tasks {
				st := t.state
				if st == "running" {
					st = "in_progress"
				}
				tail := ""
				if t.human {
					tail = "  [human " + t.kind + "]"
				}
				if len(t.blockedBy) > 0 {
					short := make([]string, len(t.blockedBy))
					for i, x := range t.blockedBy {
						short[i] = showShort(x, e)
					}
					tail += "  (by " + strings.Join(short, ", ") + ")"
				}
				if t.holder != nil {
					tail += "  [" + holderText(t.holder) + "]"
				}
				out = append(out, fmt.Sprintf("  %s  %s  %s%s", showPad(st, 11), showShort(t.id, e), t.title, tail))
			}
		}
		if len(quiet) > 0 {
			out = append(out, "open sprints without tasks: "+strings.Join(quiet, "; "))
		}
		if len(p.decisions) > 0 {
			out = append(out, fmt.Sprintf("decisions (last %d):", len(p.decisions)))
			for _, d := range p.decisions {
				out = append(out, "  "+d.date+" "+d.source+" "+d.level+"  "+d.text)
			}
		}
	}
	return strings.Join(out, "\n")
}

// showPad is Python's f"{s:<n}": s padded with spaces to n characters.
func showPad(s string, n int) string {
	if k := len([]rune(s)); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}

// sprintDetail is the sprint level: its frame, findings and every task with its state and holder.
func sprintDetail(r *repo, id string) (string, error) {
	rec, err := r.sprint(id)
	if err != nil {
		return "", err
	}
	if _, err := site.RenderRecord(rec, r.recs, r.x, nil); err != nil {
		return "", err
	}
	sp := r.item(id)
	if sp == nil {
		return "", fmt.Errorf("the work store holds no item %s", pyRepr(id))
	}
	out := []string{sp.Title + "  " + id + "  " + site.State(sp, r.x) + "  " + r.rel(rec.Path)}
	for _, name := range []string{"Goal", "Scope", "Done when", "Findings"} {
		out = append(out, "## "+name, records.SectionText(rec.Body, name))
	}
	out = append(out, "## Tasks")
	for _, t := range r.x.Children(id) {
		line := "  " + showPad(site.State(t, r.x), 7) + "  " + t.ID + "  " + t.Title
		if h := holderOf(t); h != nil {
			line += "  [" + holderText(h) + "]"
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), nil
}
