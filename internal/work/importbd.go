package work

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The import from bd: `bd export` JSONL to work-store items, by the work-store page's "Migration from bd" field
// mapping. It fails hard on an input it cannot map: an unknown key, type, status or dependency type, a dangling parent
// or blocker, an open item outside the type tree, a blocked_by cycle, a timestamp not in UTC whole seconds, a sprint
// whose number its record does not confirm. It maps nothing by guess.

// BDRecords is what the import reads from the records store: each project's record name and each sprint's, by the
// item id in the record's header (bead:). A sprint's number comes from its bd title, "Sprint N: …", and its record
// must be named <project>-N.
type BDRecords struct {
	Projects map[string]string // project item id -> record name, e.g. yeeef-agents-9va -> pm-harness
	Sprints  map[string]string // sprint item id -> record name, e.g. yeeef-agents-9va.87 -> pm-harness-78
}

// bdIssue is one line of bd export. Every key bd writes is named here, so an unknown one fails the decode; the
// fields the work store drops are only checked for shape.
type bdIssue struct {
	Type               string          `json:"_type"`
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Notes              string          `json:"notes"`
	Status             string          `json:"status"`
	IssueType          string          `json:"issue_type"`
	CloseReason        string          `json:"close_reason"`
	Parent             string          `json:"parent"`
	Labels             []string        `json:"labels"`
	Dependencies       []bdDependency  `json:"dependencies"`
	Comments           []bdComment     `json:"comments"`
	Metadata           json.RawMessage `json:"metadata"`
	ExternalRef        string          `json:"external_ref"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
	StartedAt          string          `json:"started_at"`
	ClosedAt           string          `json:"closed_at"`
	CommentCount       *int            `json:"comment_count"`
	Design             string          `json:"design"`
	AcceptanceCriteria string          `json:"acceptance_criteria"`
	// dropped (work-store page, "bd fields dropped")
	Priority        json.RawMessage `json:"priority"`
	Owner           json.RawMessage `json:"owner"`
	CreatedBy       json.RawMessage `json:"created_by"`
	Assignee        json.RawMessage `json:"assignee"`
	LeaseExpiresAt  json.RawMessage `json:"lease_expires_at"`
	HeartbeatAt     json.RawMessage `json:"heartbeat_at"`
	DependencyCount json.RawMessage `json:"dependency_count"`
	DependentCount  json.RawMessage `json:"dependent_count"`
}

type bdDependency struct {
	IssueID     string          `json:"issue_id"`
	DependsOnID string          `json:"depends_on_id"`
	Type        string          `json:"type"`
	CreatedAt   string          `json:"created_at"`
	CreatedBy   json.RawMessage `json:"created_by"` // dropped: always the git user
	Metadata    json.RawMessage `json:"metadata"`   // dropped: always {}
}

type bdComment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type bdMetadata struct {
	ClaimedBy     string          `json:"claimed_by"`
	ClaimedAt     string          `json:"claimed_at"`
	Session       string          `json:"session"`
	Inbox         string          `json:"inbox"`
	InboxHost     string          `json:"inbox_host"`
	PickedUp      *int            `json:"picked_up"`
	Review        *bdReview       `json:"review"`
	Merged        string          `json:"merged"`
	MergeReported string          `json:"merge_reported"`
	Probe         json.RawMessage `json:"probe"` // dropped: one test item
}

type bdReview struct {
	PR      string   `json:"pr"`
	Sprints []string `json:"sprints"`
	Designs []string `json:"designs"`
	Focus   string   `json:"focus"`
}

// siteReply is the comment author the pm site writes for the owner's reply.
const siteReply = "owner (site reply)"

// flagLabels are bd labels that became fields: type need, need kind action, resolution no-decision.
var flagLabels = []string{"human", "action", "no-decision"}

var sprintTitle = regexp.MustCompile(`^Sprint ([0-9]+): `)

// strictly decodes b into v: an unknown key or a second value fails.
func strictly(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("more than one JSON value")
	}
	return nil
}

// stamp parses a bd timestamp, which must be UTC in whole seconds: YYYY-MM-DDTHH:MM:SSZ.
func stamp(id, field, s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil || t.Format("2006-01-02T15:04:05Z") != s {
		return time.Time{}, fmt.Errorf("bd import: %s: %s %q is not a UTC timestamp in whole seconds", id, field, s)
	}
	return t, nil
}

// optStamp is stamp, or the zero time for "".
func optStamp(id, field, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return stamp(id, field, s)
}

// FromBD reads a bd export and maps every issue to an item, then checks the result as a store write would.
func FromBD(r io.Reader, recs BDRecords) ([]Item, error) {
	var issues []bdIssue
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var is bdIssue
		if err := strictly(line, &is); err != nil {
			return nil, fmt.Errorf("bd import: line %d: %w", n, err)
		}
		if is.Type != "issue" {
			return nil, fmt.Errorf("bd import: line %d: record kind _type %q, not issue", n, is.Type)
		}
		issues = append(issues, is)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("bd import: %w", err)
	}
	by := make(map[string]*bdIssue, len(issues))
	for i := range issues {
		if by[issues[i].ID] != nil {
			return nil, fmt.Errorf("bd import: %s appears twice", issues[i].ID)
		}
		by[issues[i].ID] = &issues[i]
	}
	items := make([]Item, 0, len(issues))
	for i := range issues {
		it, err := fromIssue(&issues[i], by, recs)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := Check(items); err != nil {
		return nil, fmt.Errorf("bd import: %w", err)
	}
	return items, nil
}

// parentOf is the issue's parent: its one parent-child dependency, which must agree with a parent key when both are
// there. It checks every dependency's shape.
func parentOf(is *bdIssue) (string, []string, error) {
	parent := ""
	var blockers []string
	for _, d := range is.Dependencies {
		if d.IssueID != is.ID {
			return "", nil, fmt.Errorf("bd import: %s has a dependency of %s", is.ID, d.IssueID)
		}
		m := strings.TrimSpace(string(d.Metadata))
		if s, err := strconv.Unquote(m); err == nil { // bd writes it as a JSON string: "{}"
			m = s
		}
		if m != "" && m != "{}" && m != "null" {
			return "", nil, fmt.Errorf("bd import: %s: dependency metadata %s", is.ID, m)
		}
		if _, err := optStamp(is.ID, "dependency created_at", d.CreatedAt); err != nil {
			return "", nil, err
		}
		switch d.Type {
		case "parent-child":
			if parent != "" {
				return "", nil, fmt.Errorf("bd import: %s has two parents, %s and %s", is.ID, parent, d.DependsOnID)
			}
			parent = d.DependsOnID
		case "blocks":
			blockers = append(blockers, d.DependsOnID)
		default:
			return "", nil, fmt.Errorf("bd import: %s: unknown dependency type %q", is.ID, d.Type)
		}
	}
	if is.Parent != "" {
		if parent != "" && parent != is.Parent {
			return "", nil, fmt.Errorf("bd import: %s: parent %s, but its parent-child dependency is on %s", is.ID,
				is.Parent, parent)
		}
		parent = is.Parent
	}
	return parent, blockers, nil
}

func fromIssue(is *bdIssue, by map[string]*bdIssue, recs BDRecords) (Item, error) {
	id := is.ID
	fail := func(format string, a ...any) (Item, error) {
		return Item{}, fmt.Errorf("bd import: %s: %s", id, fmt.Sprintf(format, a...))
	}
	if is.Design != "" || is.AcceptanceCriteria != "" {
		return fail("has design or acceptance_criteria, which the work store has no field for")
	}
	if is.CommentCount != nil && *is.CommentCount != len(is.Comments) {
		return fail("comment_count %d, but %d comments: a partial export", *is.CommentCount, len(is.Comments))
	}
	var meta bdMetadata
	if m := strings.TrimSpace(string(is.Metadata)); m != "" && m != "null" {
		if err := strictly(is.Metadata, &meta); err != nil {
			return fail("metadata: %v", err)
		}
	}
	parent, blockers, err := parentOf(is)
	if err != nil {
		return Item{}, err
	}
	labels := slices.Clone(is.Labels)
	has := func(l string) bool { return slices.Contains(labels, l) }

	it := Item{ID: id, Parent: parent, Title: is.Title}
	if len(blockers) > 0 {
		it.BlockedBy = slices.Sorted(slices.Values(blockers))
	}
	switch is.IssueType {
	case "epic":
		if parent == "" {
			it.Type = Project
			break
		}
		up := by[parent]
		if up == nil || up.IssueType != "epic" {
			return fail("an epic under %s, which is no project", parent)
		}
		if upParent, _, err := parentOf(up); err != nil || upParent != "" {
			return fail("an epic under %s, which is no project", parent)
		}
		it.Type = Sprint
	case "task", "bug":
		it.Type = Task
		if has("human") {
			it.Type = Need
		}
	default:
		return fail("unknown issue_type %q", is.IssueType)
	}
	if (has("human") || has("action")) && it.Type != Need {
		return fail("label human or action on a %s", it.Type)
	}

	switch is.Status {
	case "open", "in_progress":
		it.Status = Open
	case "closed":
		it.Status = Closed
	default:
		return fail("unknown status %q", is.Status)
	}
	if it.Status == Open && (is.CloseReason != "" || is.ClosedAt != "" || has("no-decision")) {
		return fail("is %s with close fields (close_reason, closed_at or label no-decision)", is.Status)
	}
	if it.Status == Closed {
		it.CloseReason = is.CloseReason
		switch {
		case has("no-decision"):
			it.Resolution = NoDecision
		case strings.HasPrefix(is.CloseReason, "Responded"):
			it.Resolution = Answered
		case strings.HasPrefix(is.CloseReason, "Dismissed"):
			it.Resolution = Dismissed
		default:
			it.Resolution = Done
		}
	}

	if it.Type == Sprint {
		m := sprintTitle.FindStringSubmatch(is.Title)
		if m == nil {
			return fail("a sprint whose title does not start \"Sprint N: \"")
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return fail("sprint number %q", m[1])
		}
		project, ok := recs.Projects[parent]
		if !ok {
			return fail("a sprint of %s, which has no project record", parent)
		}
		want := fmt.Sprintf("%s-%d", project, n)
		if got, ok := recs.Sprints[id]; !ok {
			return fail("sprint %d has no sprint record; %s.md should name it", n, want)
		} else if got != want {
			return fail("its title says sprint %d, so record %s, but its record is %s", n, want, got)
		}
		it.Number = n
	}

	it.Description = is.Description
	if is.Notes != "" {
		if it.Description != "" {
			it.Description += "\n\n"
		}
		it.Description += "## Notes\n\n" + is.Notes
	}

	for _, f := range []struct {
		name string
		s    string
		t    *time.Time
	}{{"created_at", is.CreatedAt, &it.CreatedAt}, {"updated_at", is.UpdatedAt, &it.UpdatedAt},
		{"started_at", is.StartedAt, &it.StartedAt}, {"closed_at", is.ClosedAt, &it.ClosedAt}} {
		if *f.t, err = optStamp(id, f.name, f.s); err != nil {
			return Item{}, err
		}
	}

	if (meta.ClaimedBy == "") != (meta.ClaimedAt == "") {
		return fail("claimed_by and claimed_at come together")
	}
	switch is.Status {
	case "in_progress":
		if meta.ClaimedBy == "" {
			return fail("in_progress without claimed_by: held by no session")
		}
		at, err := stamp(id, "claimed_at", meta.ClaimedAt)
		if err != nil {
			return Item{}, err
		}
		it.Holder = &Holder{Session: meta.ClaimedBy, ClaimedAt: at}
	case "open":
		if meta.ClaimedBy != "" {
			return fail("open but claimed_by %s: neither held (in_progress) nor released", meta.ClaimedBy)
		}
	case "closed":
		it.ClosedBy = meta.ClaimedBy
	}

	for _, l := range labels {
		if !slices.Contains(flagLabels, l) {
			it.Labels = append(it.Labels, l)
		}
	}
	if is.IssueType == "bug" {
		it.Labels = append(it.Labels, "bug")
	}
	it.Labels = slices.Compact(slices.Sorted(slices.Values(it.Labels)))
	if len(it.Labels) == 0 {
		it.Labels = nil
	}

	for _, c := range is.Comments {
		if c.IssueID != id {
			return fail("holds a comment of %s", c.IssueID)
		}
		at, err := stamp(id, "comment "+c.ID+" created_at", c.CreatedAt)
		if err != nil {
			return Item{}, err
		}
		cm := Comment{ID: c.ID, Kind: Note, Author: c.Author, Text: c.Text, CreatedAt: at}
		if c.Author == siteReply {
			cm.Kind, cm.Author = Reply, "owner"
		}
		it.Comments = append(it.Comments, cm) // in bd's order, which the store keeps
	}

	if it.Type != Need {
		if meta.Session != "" || meta.Inbox != "" || meta.InboxHost != "" || meta.PickedUp != nil ||
			meta.Review != nil || meta.Merged != "" || meta.MergeReported != "" || is.ExternalRef != "" {
			return fail("need metadata or external_ref on a %s", it.Type)
		}
		return it, nil
	}
	need := &NeedInfo{Kind: Decision}
	switch {
	case meta.Review != nil:
		need.Kind = Review
	case has("action"):
		need.Kind = Action
	}
	if meta.PickedUp != nil {
		if *meta.PickedUp < 0 {
			return fail("picked_up %d", *meta.PickedUp)
		}
		need.Delivered = *meta.PickedUp
	}
	if meta.Session != "" {
		need.RaisedBy = &RaisedBy{Session: meta.Session, Inbox: meta.Inbox, Host: meta.InboxHost}
	} else if meta.Inbox != "" || meta.InboxHost != "" {
		return fail("an inbox without the session that raised it")
	}
	if need.Kind == Review {
		rv := meta.Review
		if is.ExternalRef != "" && is.ExternalRef != rv.PR {
			return fail("external_ref %s is not the review's PR %s", is.ExternalRef, rv.PR)
		}
		need.Review = &ReviewInfo{PR: rv.PR, Sprints: slices.Clone(rv.Sprints), Designs: slices.Clone(rv.Designs),
			Focus: rv.Focus, Merged: meta.Merged, MergeReported: meta.MergeReported}
		if need.Review.Sprints == nil {
			need.Review.Sprints = []string{}
		}
		if len(need.Review.Designs) == 0 {
			need.Review.Designs = nil
		}
	} else if meta.Merged != "" || meta.MergeReported != "" || is.ExternalRef != "" {
		return fail("review data on a %s need", need.Kind)
	}
	it.Need = need
	return it, nil
}

// ReadBDRecords reads the project and sprint records' names and item ids (the bead: line of each record's header)
// from a records store, for FromBD. Two records of one item fail; a record without an item id names none.
func ReadBDRecords(records string) (BDRecords, error) {
	recs := BDRecords{Projects: map[string]string{}, Sprints: map[string]string{}}
	for kind, into := range map[string]map[string]string{"projects": recs.Projects, "sprints": recs.Sprints} {
		paths, err := filepath.Glob(filepath.Join(records, kind, "*.md"))
		if err != nil {
			return recs, err
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				return recs, fmt.Errorf("bd import: %w", err)
			}
			id := headerBead(string(b))
			if id == "" {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(p), ".md")
			if other, dup := into[id]; dup {
				return recs, fmt.Errorf("bd import: records %s and %s both name %s", other, name, id)
			}
			into[id] = name
		}
	}
	return recs, nil
}

// headerBead is the bead: value of a record's front matter, or "".
func headerBead(text string) string {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	header, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return ""
	}
	for _, line := range strings.Split(header, "\n") {
		if v, ok := strings.CutPrefix(line, "bead:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
