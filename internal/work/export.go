package work

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

// Export writes items as pm export prints them: one JSON object per line, in the order given. Every item carries every
// field, in the work-store page's order, with null where a field does not apply and [] for an empty list, so one
// shape serves jq and the tests' transcripts (tests/work_items.py writes the same).
func Export(w io.Writer, items []Item) error {
	bw := bufio.NewWriter(w)
	for i := range items {
		b, err := json.Marshal(exported(&items[i]))
		if err != nil {
			return err
		}
		bw.Write(b)
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// exportItem and the types under it are an item's pm export form: field order and nulls as Export says.
type exportItem struct {
	ID          string          `json:"id"`
	Type        Type            `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      Status          `json:"status"`
	Resolution  *string         `json:"resolution"`
	CloseReason *string         `json:"close_reason"`
	Number      *int            `json:"number"`
	Parent      *string         `json:"parent"`
	BlockedBy   []string        `json:"blocked_by"`
	Labels      []string        `json:"labels"`
	Holder      *exportHolder   `json:"holder"`
	StartedAt   *string         `json:"started_at"`
	CreatedAt   *string         `json:"created_at"`
	UpdatedAt   *string         `json:"updated_at"`
	ClosedAt    *string         `json:"closed_at"`
	ClosedBy    *string         `json:"closed_by"`
	Comments    []exportComment `json:"comments"`
	Need        *exportNeed     `json:"need"`
}

type exportHolder struct {
	Session   string  `json:"session"`
	Host      *string `json:"host"`
	ClaimedAt *string `json:"claimed_at"`
}

type exportComment struct {
	ID        string      `json:"id"`
	Kind      CommentKind `json:"kind"`
	Author    string      `json:"author"`
	Text      string      `json:"text"`
	CreatedAt *string     `json:"created_at"`
}

type exportNeed struct {
	Kind      NeedKind      `json:"kind"`
	RaisedBy  *exportRaised `json:"raised_by"`
	Delivered int           `json:"delivered"`
	Review    *exportReview `json:"review"`
}

type exportRaised struct {
	Session string  `json:"session"`
	Inbox   *string `json:"inbox"`
	Host    *string `json:"host"`
}

type exportReview struct {
	PR            string   `json:"pr"`
	Sprints       []string `json:"sprints"`
	Designs       []string `json:"designs"`
	Focus         string   `json:"focus"`
	Merged        *string  `json:"merged"`
	MergeReported *string  `json:"merge_reported"`
}

// orNull is s, or null for "".
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stampOrNull is t as YYYY-MM-DDTHH:MM:SSZ, or null for the zero time.
func stampOrNull(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format("2006-01-02T15:04:05Z")
	return &s
}

// listOf is l, or an empty list for nil.
func listOf(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func exported(it *Item) exportItem {
	e := exportItem{ID: it.ID, Type: it.Type, Title: it.Title, Description: it.Description, Status: it.Status,
		Resolution: orNull(string(it.Resolution)), CloseReason: orNull(it.CloseReason), Parent: orNull(it.Parent),
		BlockedBy: listOf(it.BlockedBy), Labels: listOf(it.Labels), StartedAt: stampOrNull(it.StartedAt),
		CreatedAt: stampOrNull(it.CreatedAt), UpdatedAt: stampOrNull(it.UpdatedAt), ClosedAt: stampOrNull(it.ClosedAt),
		ClosedBy: orNull(it.ClosedBy), Comments: []exportComment{}}
	if it.Number > 0 {
		n := it.Number
		e.Number = &n
	}
	if h := it.Holder; h != nil {
		e.Holder = &exportHolder{Session: h.Session, Host: orNull(h.Host), ClaimedAt: stampOrNull(h.ClaimedAt)}
	}
	for _, c := range it.Comments {
		e.Comments = append(e.Comments, exportComment{ID: c.ID, Kind: c.Kind, Author: c.Author, Text: c.Text,
			CreatedAt: stampOrNull(c.CreatedAt)})
	}
	if n := it.Need; n != nil {
		e.Need = &exportNeed{Kind: n.Kind, Delivered: n.Delivered}
		if r := n.RaisedBy; r != nil {
			e.Need.RaisedBy = &exportRaised{Session: r.Session, Inbox: orNull(r.Inbox), Host: orNull(r.Host)}
		}
		if r := n.Review; r != nil {
			e.Need.Review = &exportReview{PR: r.PR, Sprints: listOf(r.Sprints), Designs: listOf(r.Designs),
				Focus: r.Focus, Merged: orNull(r.Merged), MergeReported: orNull(r.MergeReported)}
		}
	}
	return e
}
