// Package work is pm's work store: the items that replace Beads issues, and the store that holds them, on embedded
// Dolt. The item model is package item, which links no Dolt; this file aliases its names, so work's code and its
// callers name them as work's. store.go fixes the Store interface that the service code builds against.
//
// The files: schema.go the schema; check.go the invariant checks; ids.go the ids; ready.go ready and blocked;
// importbd.go the bd import; export.go the import of pm export's lines (pm init --import), whose form is item's; sync.go the sync through the git
// remote under refs/pm/work, and the child-id compare-and-swap on the scratch branch pm-cas, which a sprint move
// (move.go) runs too; merge.go the merge rules. host.go is the host, which only pm service run starts: the Dolt engine
// held open and served on <main>/.pm/run/work.sock with pm_version(), pm_sync(), pm_create(?), pm_move_sprint(?) and
// pm_setup(); host_gc.go holds its collections to one at a time, each waiting out the last one's visits (a Dolt
// bug). dolt.go is the client every command and the service's own loops use (Dial), with the version handshake,
// and every write under the store's fair write lock (lock.go), which the host serves as pm_lock() and pm_unlock().
//
// Tests: sync_test.go runs two hosted clones on a local bare repo; access_test.go holds the one access path (only
// host.go loads an engine, only pm service run starts a host); bench_test.go the store benchmarks, which run with
// PM_BENCH_STORE=<a copy of a clone's .pm/store/work>; roundtrip_test.go the bd import's round trip on testdata, and
// with PM_BD_EXPORT=<bd export > file> PM_BD_RECORDS=$(pm where records) on a real export. Package worktest holds the
// test doubles.
package work

// imported as model: work's tests have a helper named item
import (
	"io"

	model "github.com/Yeeef/pm/internal/item"
)

type (
	Type        = model.Type
	Status      = model.Status
	Resolution  = model.Resolution
	NeedKind    = model.NeedKind
	CommentKind = model.CommentKind
	Item        = model.Item
	Holder      = model.Holder
	NeedInfo    = model.NeedInfo
	RaisedBy    = model.RaisedBy
	ReviewInfo  = model.ReviewInfo
	AskInfo     = model.AskInfo
	AskQuestion = model.AskQuestion
	AskOption   = model.AskOption
	Comment     = model.Comment
	SprintMove  = model.SprintMove
)

const (
	Project = model.Project
	Sprint  = model.Sprint
	Task    = model.Task
	Need    = model.Need

	Open   = model.Open
	Closed = model.Closed

	Done       = model.Done
	Answered   = model.Answered
	NoDecision = model.NoDecision
	Dismissed  = model.Dismissed

	Decision = model.Decision
	Action   = model.Action
	Review   = model.Review
	Question = model.Question

	Reply = model.Reply
	Note  = model.Note

	MoveAuthor = model.MoveAuthor
)

// HasReply is item.HasReply.
func HasReply(it *Item) bool { return model.HasReply(it) }

// CompareIDs is item.CompareIDs.
func CompareIDs(a, b string) int { return model.CompareIDs(a, b) }

// Export is item.Export.
func Export(w io.Writer, items []Item) error { return model.Export(w, items) }

// SprintMoves is item.SprintMoves.
func SprintMoves(it *Item) []SprintMove { return model.SprintMoves(it) }
