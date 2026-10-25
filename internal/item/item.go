// Package item is the work store's item model: the Item type and its fields' types, the natural order of ids, the
// sprint moves a sprint's comments record, and the JSON lines pm export prints (export.go). It is the Data model
// section of the work-store design page: a fixed set of typed fields, no free metadata bag. It links no Dolt, so the
// code that only reads items (records, site) builds and tests without the engine; package work, the store on embedded
// Dolt, aliases each name here.
package item

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Type is what an item is. It replaces bd's issue_type plus the human and action labels.
type Type string

const (
	Project Type = "project" // a standing goal; no parent
	Sprint  Type = "sprint"  // one step toward a project's goal; parent a project
	Task    Type = "task"    // one unit of work; parent a sprint, a task or a project
	Need    Type = "need"    // what waits on the owner; parent any item
)

// Status is open or closed; "in progress" is an open item with a holder, and blocked is computed.
type Status string

const (
	Open   Status = "open"
	Closed Status = "closed"
)

// Resolution is how a closed item ended; empty while the item is open.
type Resolution string

const (
	Done       Resolution = "done"
	Answered   Resolution = "answered"
	NoDecision Resolution = "no-decision"
	Dismissed  Resolution = "dismissed"
)

// NeedKind is what a need asks of the owner.
type NeedKind string

const (
	Decision NeedKind = "decision"
	Action   NeedKind = "action"
	Review   NeedKind = "review"
	Question NeedKind = "question" // a session's ask-tool call: its answer steers that session and records no decision
)

// CommentKind is reply (the owner's answer, counted against Need.Delivered) or note (anything else).
type CommentKind string

const (
	Reply CommentKind = "reply"
	Note  CommentKind = "note"
)

// Item is one unit in the work store. Timestamps are UTC and whole seconds (YYYY-MM-DDTHH:MM:SSZ). A field that does
// not apply is zero: Number on all but sprints, Need on all but needs, Holder on an item nobody holds, the close
// fields on an open item; pm export writes each as null (work's export.go).
type Item struct {
	ID          string     `json:"id"` // minted at create, never changes; <prefix>-<root>(.<n>)*
	Type        Type       `json:"type"`
	Parent      string     `json:"parent,omitempty"` // "" only for a project; the id does not follow a move
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"` // Markdown
	Status      Status     `json:"status"`
	Resolution  Resolution `json:"resolution,omitempty"`
	CloseReason string     `json:"close_reason,omitempty"` // a review keeps "merged as <sha>"
	Number      int        `json:"number,omitempty"`       // a sprint's number in its record name; sprints only
	BlockedBy   []string   `json:"blocked_by,omitempty"`   // ids of the items that must close first
	Labels      []string   `json:"labels,omitempty"`       // free tags; pm's logic reads one, ProposalLabel
	Holder      *Holder    `json:"holder,omitempty"`       // nil on every closed item
	StartedAt   time.Time  `json:"started_at,omitzero"`    // the first claim
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    time.Time  `json:"closed_at,omitzero"`
	ClosedBy    string     `json:"closed_by,omitempty"` // the session that closed it
	Comments    []Comment  `json:"comments,omitempty"`
	Need        *NeedInfo  `json:"need,omitempty"` // needs only
}

// ProposalLabel marks a sprint that pm sprint propose made; it is never removed (the sprint-proposals design page).
const ProposalLabel = "proposal"

// Unstarted is whether sprint s is a proposal nobody started: labelled ProposalLabel, with no task among kids, its
// children (needs do not count). Open, it is a proposal; closed, a dropped proposal. Derived on read, never stored.
func Unstarted(s *Item, kids []*Item) bool {
	if s.Type != Sprint || !slices.Contains(s.Labels, ProposalLabel) {
		return false
	}
	return !slices.ContainsFunc(kids, func(k *Item) bool { return k.Type == Task })
}

// HasReply is whether the item holds an owner's reply: a need the owner answered.
func HasReply(it *Item) bool {
	for _, c := range it.Comments {
		if c.Kind == Reply {
			return true
		}
	}
	return false
}

// Holder is the session that holds an item it works on. It replaces bd's assignee and claimed_by metadata.
type Holder struct {
	Session   string    `json:"session"`
	Host      string    `json:"host"`
	ClaimedAt time.Time `json:"claimed_at"`
}

// NeedInfo is what only a need carries.
type NeedInfo struct {
	Kind      NeedKind    `json:"kind"`
	RaisedBy  *RaisedBy   `json:"raised_by,omitempty"` // nil when no session raised it
	Delivered int         `json:"delivered"`           // owner replies that reached RaisedBy.Session
	Review    *ReviewInfo `json:"review,omitempty"`    // kind review only
	Ask       *AskInfo    `json:"ask,omitempty"`       // kind question only
}

// AskInfo is a question need's ask: the key of the ask-tool call that asked it and its questions, in the order asked.
// It is written once, when the need is raised.
type AskInfo struct {
	// Call is an opaque key the raiser picks, stable for one call, at most 128 bytes: for Claude Code, whose hook input
	// carries no tool_use id, the prompt_id with a hash of the tool input; for Codex, the call_id.
	Call      string        `json:"call"`
	Questions []AskQuestion `json:"questions"`
}

// AskQuestion is one question of an ask: its text, its short header, whether the owner may pick more than one option,
// and its options, in order (none for a free-text question).
type AskQuestion struct {
	Text    string      `json:"text"`
	Header  string      `json:"header"`
	Multi   bool        `json:"multi"`
	Options []AskOption `json:"options"`
}

// AskOption is one option of a question: its label and what it means.
type AskOption struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// RaisedBy is the session that raised a need and where its owner replies are delivered.
type RaisedBy struct {
	Session string `json:"session"`
	Inbox   string `json:"inbox,omitempty"`
	Host    string `json:"host,omitempty"`
}

// ReviewInfo is a PR review: the PR, what it delivers, and its merge as the pm service saw and reported it.
type ReviewInfo struct {
	PR            string   `json:"pr"`
	Sprints       []string `json:"sprints"`
	Designs       []string `json:"designs,omitempty"`
	Focus         string   `json:"focus"`
	Merged        string   `json:"merged,omitempty"`         // the merge commit on main the service saw
	MergeReported string   `json:"merge_reported,omitempty"` // the merge commit that reached the raising session
}

// Comment is one comment on an item.
type Comment struct {
	ID        string      `json:"id"` // a UUID; bd's comment ids are kept
	Kind      CommentKind `json:"kind"`
	Author    string      `json:"author"` // "owner", or the writing session's id
	Text      string      `json:"text"`
	CreatedAt time.Time   `json:"created_at"`
}

// CompareIDs is the natural order of ids, which pm's lists and pages use: the prefix and root as text, then each
// child part in turn, a number by its value before any other part, so 9va.9 < 9va.39 < 9va.100 and 9va.9.2 <
// 9va.9.10; the text breaks ties ("01" before "1"), and a shorter id comes first. Python's beads.id_key is the same
// order.
func CompareIDs(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	if c := strings.Compare(as[0], bs[0]); c != 0 {
		return c
	}
	for i := 1; i < len(as) && i < len(bs); i++ {
		if c := comparePart(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return len(as) - len(bs)
}

// comparePart orders two child parts: numbers first, by value (any length), then the text.
func comparePart(a, b string) int {
	an, bn := isNumber(a), isNumber(b)
	switch {
	case an && !bn:
		return -1
	case !an && bn:
		return 1
	case an:
		at, bt := strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(at) != len(bt) {
			return len(at) - len(bt)
		}
		if c := strings.Compare(at, bt); c != 0 {
			return c
		}
	}
	return strings.Compare(a, b)
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MoveAuthor is the author of a move note: pm itself, which is no session id and not the owner.
const MoveAuthor = "pm"

var moveLine = regexp.MustCompile(`^pm sprint move: from (\S+) sprint ([0-9]+) to (\S+) sprint ([0-9]+)$`)

// SprintMove is one move of a sprint, as its move note records it: from a project and its number there, to a project
// and its number there, and why.
type SprintMove struct {
	From       string
	FromNumber int
	To         string
	ToNumber   int
	Reason     string
}

// Text is the move note's text: the move on its first line, then the reason.
func (m SprintMove) Text() string {
	return fmt.Sprintf("pm sprint move: from %s sprint %d to %s sprint %d\n%s", m.From, m.FromNumber, m.To, m.ToNumber,
		m.Reason)
}

// SprintMoves is the sprint's moves, in the order its comments hold them: each comment of kind note by MoveAuthor
// whose first line is a move.
func SprintMoves(it *Item) []SprintMove {
	var out []SprintMove
	for _, c := range it.Comments {
		if c.Kind != Note || c.Author != MoveAuthor {
			continue
		}
		first, reason, _ := strings.Cut(c.Text, "\n")
		m := moveLine.FindStringSubmatch(first)
		if m == nil {
			continue
		}
		from, errFrom := strconv.Atoi(m[2])
		to, errTo := strconv.Atoi(m[4])
		if errFrom != nil || errTo != nil { // a number past int's range: no move pm wrote
			continue
		}
		out = append(out, SprintMove{From: m[1], FromNumber: from, To: m[3], ToNumber: to, Reason: reason})
	}
	return out
}

// TaskMoveText is the move note pm task move leaves on the task or need it moves: the parent it left and the one it
// joined.
func TaskMoveText(from, to string) string { return "pm task move: from " + from + " to " + to }

var taskMoveLine = regexp.MustCompile(`^pm task move: from \S+ to (\S+)$`)

// Arrived is when the item came under parent: its latest move note into parent, else its creation (made there, or
// moved before move notes were kept).
func Arrived(it *Item, parent string) time.Time {
	at := it.CreatedAt
	for _, c := range it.Comments {
		if m := taskMoveLine.FindStringSubmatch(c.Text); c.Kind == Note && c.Author == MoveAuthor && m != nil &&
			m[1] == parent {
			at = c.CreatedAt
		}
	}
	return at
}
