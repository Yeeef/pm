package work

import (
	"fmt"
	"time"
)

// The merge rules, as the work-store page's Data model gives them ("Concurrent writers" and its table). Dolt merges
// each cell three-way against the base; a row in which both sides changed a cell lands in dolt_conflicts_items with
// its base, ours and theirs, and the working row keeps ours whole. mergeItem resolves one such row field by field: a
// field only one side changed from the base takes that side's value; a field both sides changed takes the table's
// rule; a rule that names no winner fails hard, naming the item and the field. Lists (labels, blocked_by, comments,
// review targets) are rows of their own tables, which Dolt merges; mergeItem sees the items row only.

// Override is a claim the merge dropped: two clones claimed one item, and the later claim won.
type Override struct {
	ID   string // the item
	Lost Holder // the claim that lost
	Kept Holder // the claim that won
}

// conflictError is a field no merge rule settles.
func conflictError(id, field, why string) error {
	return fmt.Errorf("work store: merge: %s field %s: %s; no merge rule settles it", id, field, why)
}

// three is the three-way merge of one field: the value when both sides agree or only one side changed it from the
// base, and ok false when both changed it to different values.
func three[T comparable](base, ours, theirs T) (T, bool) {
	switch {
	case ours == theirs:
		return ours, true
	case ours == base:
		return theirs, true
	case theirs == base:
		return ours, true
	}
	var zero T
	return zero, false
}

// later is the value from the side with the later updated_at; equal stamps name no winner.
func later[T any](id, field string, ours, theirs T, o, t *Item) (T, error) {
	switch {
	case o.UpdatedAt.After(t.UpdatedAt):
		return ours, nil
	case t.UpdatedAt.After(o.UpdatedAt):
		return theirs, nil
	}
	var zero T
	return zero, conflictError(id, field, fmt.Sprintf("both sides changed it at updated_at %s",
		o.UpdatedAt.Format(time.RFC3339)))
}

// closeFields is the close group the table merges as one: closed_at, closed_by, close_reason.
type closeFields struct {
	at     time.Time
	by     string
	reason string
}

func closeOf(it *Item) closeFields { return closeFields{it.ClosedAt, it.ClosedBy, it.CloseReason} }

// holderOf is the holder as a comparable value; the zero value is no holder.
func holderOf(it *Item) Holder {
	if it.Holder == nil {
		return Holder{}
	}
	return *it.Holder
}

// needFields is a need's scalar columns, each merged as "other scalars" except delivered.
type needFields struct {
	kind                             NeedKind
	raised                           RaisedBy
	pr, focus, merged, mergeReported string
}

func needOf(it *Item) needFields {
	n := it.Need
	var f needFields
	if n == nil {
		return f
	}
	f.kind = n.Kind
	if n.RaisedBy != nil {
		f.raised = *n.RaisedBy
	}
	if r := n.Review; r != nil {
		f.pr, f.focus, f.merged, f.mergeReported = r.PR, r.Focus, r.Merged, r.MergeReported
	}
	return f
}

// mergeItem resolves one items row both sides changed: base, ours and theirs are the row's three versions (scalar
// fields only), and the result is the row to write. It reports a claim it dropped.
func mergeItem(base, ours, theirs Item) (Item, *Override, error) {
	id := ours.ID
	b, o, t := &base, &ours, &theirs
	m := cloneItem(ours)

	// id, type, number: never changed; unequal values fail.
	if o.ID != t.ID {
		return Item{}, nil, conflictError(id, "id", fmt.Sprintf("ours %s, theirs %s", o.ID, t.ID))
	}
	if o.Type != t.Type {
		return Item{}, nil, conflictError(id, "type", fmt.Sprintf("ours %s, theirs %s", o.Type, t.Type))
	}
	if o.Number != t.Number {
		return Item{}, nil, conflictError(id, "number", fmt.Sprintf("ours %d, theirs %d", o.Number, t.Number))
	}
	if (o.Need == nil) != (t.Need == nil) {
		return Item{}, nil, conflictError(id, "need_kind", "one side has need fields and the other none")
	}

	// status: closed wins.
	if v, ok := three(b.Status, o.Status, t.Status); ok {
		m.Status = v
	} else {
		m.Status = Closed
	}

	// closed_at, closed_by, close_reason: from the earlier close when both closed, else from the side that closed.
	if v, ok := three(closeOf(b), closeOf(o), closeOf(t)); ok {
		m.ClosedAt, m.ClosedBy, m.CloseReason = v.at, v.by, v.reason
	} else {
		oc, tc := o.Status == Closed, t.Status == Closed
		var from *Item
		switch {
		case oc && tc && o.ClosedAt.Before(t.ClosedAt):
			from = o
		case oc && tc && t.ClosedAt.Before(o.ClosedAt):
			from = t
		case oc && tc:
			return Item{}, nil, conflictError(id, "closed_at, closed_by, close_reason",
				fmt.Sprintf("both sides closed it at %s, differently", o.ClosedAt.Format(time.RFC3339)))
		case oc:
			from = o
		case tc:
			from = t
		default:
			return Item{}, nil, conflictError(id, "closed_at, closed_by, close_reason",
				"both sides changed them on an item neither side closed")
		}
		m.ClosedAt, m.ClosedBy, m.CloseReason = from.ClosedAt, from.ClosedBy, from.CloseReason
	}

	// resolution: the side's value that is not null; when both set it, the later updated_at wins.
	if v, ok := three(b.Resolution, o.Resolution, t.Resolution); ok {
		m.Resolution = v
	} else {
		switch {
		case o.Resolution == "":
			m.Resolution = t.Resolution
		case t.Resolution == "":
			m.Resolution = o.Resolution
		default:
			r, err := later(id, "resolution", o.Resolution, t.Resolution, o, t)
			if err != nil {
				return Item{}, nil, err
			}
			m.Resolution = r
		}
	}

	// started_at: the earliest start. updated_at: the latest.
	if v, ok := three(b.StartedAt, o.StartedAt, t.StartedAt); ok {
		m.StartedAt = v
	} else {
		m.StartedAt = o.StartedAt
		if m.StartedAt.IsZero() || !t.StartedAt.IsZero() && t.StartedAt.Before(m.StartedAt) {
			m.StartedAt = t.StartedAt
		}
	}
	if v, ok := three(b.UpdatedAt, o.UpdatedAt, t.UpdatedAt); ok {
		m.UpdatedAt = v
	} else if t.UpdatedAt.After(o.UpdatedAt) {
		m.UpdatedAt = t.UpdatedAt
	} else {
		m.UpdatedAt = o.UpdatedAt
	}
	if v, ok := three(b.CreatedAt, o.CreatedAt, t.CreatedAt); ok {
		m.CreatedAt = v
	} else {
		return Item{}, nil, conflictError(id, "created_at", "both sides changed it")
	}

	// other scalars: the later updated_at wins.
	for _, f := range []struct {
		name       string
		dst        *string
		bv, ov, tv string
	}{
		{"title", &m.Title, b.Title, o.Title, t.Title},
		{"description", &m.Description, b.Description, o.Description, t.Description},
		{"parent", &m.Parent, b.Parent, o.Parent, t.Parent},
	} {
		if v, ok := three(f.bv, f.ov, f.tv); ok {
			*f.dst = v
			continue
		}
		v, err := later(id, f.name, f.ov, f.tv, o, t)
		if err != nil {
			return Item{}, nil, err
		}
		*f.dst = v
	}
	if m.Need != nil {
		nf, err := mergeNeed(id, b, o, t)
		if err != nil {
			return Item{}, nil, err
		}
		m.Need = nf
	}

	// holder: a closed result has none (close beats claim); else a claim beats a release, and between two claims the
	// later claimed_at wins.
	var over *Override
	if m.Status == Closed {
		m.Holder = nil
	} else if v, ok := three(holderOf(b), holderOf(o), holderOf(t)); ok {
		m.Holder = nil
		if v != (Holder{}) {
			m.Holder = &v
		}
	} else {
		oh, th := holderOf(o), holderOf(t)
		var win, lose Holder
		switch {
		case oh == Holder{}:
			win = th
		case th == Holder{}:
			win = oh
		case oh.ClaimedAt.After(th.ClaimedAt):
			win, lose = oh, th
		case th.ClaimedAt.After(oh.ClaimedAt):
			win, lose = th, oh
		default:
			return Item{}, nil, conflictError(id, "holder", fmt.Sprintf("sessions %s and %s both claimed it at %s",
				oh.Session, th.Session, oh.ClaimedAt.Format(time.RFC3339)))
		}
		m.Holder = &win
		if lose != (Holder{}) && lose.Session != win.Session {
			over = &Override{ID: id, Lost: lose, Kept: win}
		}
	}
	return m, over, nil
}

// mergeNeed merges a need's scalar columns: delivered takes the max, the rest the later updated_at.
func mergeNeed(id string, b, o, t *Item) (*NeedInfo, error) {
	m := cloneItem(*o).Need
	bn := needOf(b)
	on, tn := needOf(o), needOf(t)
	if v, ok := three(bn.kind, on.kind, tn.kind); ok {
		m.Kind = v
	} else {
		v, err := later(id, "need_kind", on.kind, tn.kind, o, t)
		if err != nil {
			return nil, err
		}
		m.Kind = v
	}
	raised, ok := three(bn.raised, on.raised, tn.raised)
	if !ok {
		var err error
		if raised, err = later(id, "raised_session, raised_inbox, raised_host", on.raised, tn.raised, o, t); err != nil {
			return nil, err
		}
	}
	m.RaisedBy = nil
	if raised != (RaisedBy{}) {
		m.RaisedBy = &raised
	}
	var bd, od, td int
	if b.Need != nil {
		bd = b.Need.Delivered
	}
	od, td = o.Need.Delivered, t.Need.Delivered
	if v, ok := three(bd, od, td); ok {
		m.Delivered = v
	} else {
		m.Delivered = max(od, td)
	}
	if m.Kind != Review {
		m.Review = nil
		return m, nil
	}
	if m.Review == nil {
		m.Review = &ReviewInfo{}
	}
	r := m.Review
	for _, f := range []struct {
		name       string
		dst        *string
		bv, ov, tv string
	}{
		{"review_pr", &r.PR, bn.pr, on.pr, tn.pr},
		{"review_focus", &r.Focus, bn.focus, on.focus, tn.focus},
		{"review_merged", &r.Merged, bn.merged, on.merged, tn.merged},
		{"review_merge_reported", &r.MergeReported, bn.mergeReported, on.mergeReported, tn.mergeReported},
	} {
		if v, ok := three(f.bv, f.ov, f.tv); ok {
			*f.dst = v
			continue
		}
		v, err := later(id, f.name, f.ov, f.tv, o, t)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return m, nil
}
