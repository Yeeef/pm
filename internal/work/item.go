// Package work is pm's work store: the items that replace Beads issues, and the store that holds them. This file and
// store.go fix the Item type and the Store interface that the records, site and service code builds against; the
// embedded Dolt store behind the interface comes later. The model is the Data model section of the work-store design
// page: a fixed set of typed fields, no free metadata bag.
package work

import "time"

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
)

// CommentKind is reply (the owner's answer, counted against Need.Delivered) or note (anything else).
type CommentKind string

const (
	Reply CommentKind = "reply"
	Note  CommentKind = "note"
)

// Item is one unit in the work store. Timestamps are UTC and whole seconds (YYYY-MM-DDTHH:MM:SSZ). A field that does
// not apply is zero: Number on all but sprints, Need on all but needs, Holder on an item nobody holds, the close
// fields on an open item; pm export writes each as null (export.go).
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
	Labels      []string   `json:"labels,omitempty"`       // free tags; pm's logic reads none
	Holder      *Holder    `json:"holder,omitempty"`       // nil on every closed item
	StartedAt   time.Time  `json:"started_at,omitzero"`    // the first claim
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    time.Time  `json:"closed_at,omitzero"`
	ClosedBy    string     `json:"closed_by,omitempty"` // the session that closed it
	Comments    []Comment  `json:"comments,omitempty"`
	Need        *NeedInfo  `json:"need,omitempty"` // needs only
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
