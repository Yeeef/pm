package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/work"
	"regexp"
	"time"
)

// The shapes, patterns and helpers shared by the commands that write records and work items (each in its noun's
// file): findings, feedback, new docs, design pages and postmortems,
// projects and sprints opened and closed, tasks added, closed, claimed and moved. Each refuses before any write; a
// write that touches both stores writes the work store first, then the records.

const (
	frameShape = "--text is the sprint's frame, e.g.:\n" +
		"  ## Goal\n  Ship the parser, because the site needs tables.\n" +
		"  ## Scope\n  **In:** the parser and its tests.\n  **Out:** the site's styling.\n" +
		"  ## Done when\n  - make test passes with a table record."
	decisionShape = "a decision body is the decision on its first line, then its reason on the next line, e.g.:\n" +
		"  Records live on their own branch, in one store every worktree shares.\n" +
		"  A record kept on a code branch is invisible to the other branches until a merge."
	worktreesDir = ".claude/worktrees" // where pm task claim tells agents to make theirs
)

var (
	frame       = []string{"Goal", "Scope", "Done when"}
	slugRE      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	feedbackRE  = regexp.MustCompile(`(?m)^### \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC, session ` + "`")
	mergedAs    = regexp.MustCompile(`^merged as ([0-9a-f]{7,40})\b`)
	sectionHead = regexp.MustCompile(`(?m)^## (.+?)` + records.PyS + `*$`) // Python's \s
	scopeRE     = regexp.MustCompile(`^(?s)\*\*In:\*\*(.*?)\*\*Out:\*\*(.*)`)
)

// feedbackEntry matches the heading of each entry in a feedback doc.
var feedbackEntry = feedbackRE

func isEpic(it *work.Item) bool { return it.Type == work.Project || it.Type == work.Sprint }

func strip(s string) string { return config.PyStrip(s) }

// given is the dest's value and whether the command line gave it (argparse's None when not).
func given(p *Parsed, dest string) (string, bool) {
	v := p.Values[dest]
	if len(v) == 0 {
		return "", false
	}
	return v[len(v)-1], true
}

// planned is a new item as a check sees it before the work store mints it.
func planned(id string, t work.Type, parent, title string) work.Item {
	now := time.Now().UTC().Truncate(time.Second)
	return work.Item{ID: id, Type: t, Parent: parent, Title: title, Status: work.Open, CreatedAt: now, UpdatedAt: now}
}

// closedCopy is an item as a close leaves it, for the check before the close.
func closedCopy(it *work.Item, reason string) work.Item {
	c := *it
	c.Status, c.Resolution, c.CloseReason, c.Holder = work.Closed, work.Done, reason, nil
	c.ClosedAt = time.Now().UTC().Truncate(time.Second)
	return c
}

// closer is the session a close records: the holder's, else this command's. Python pm's Beads keep the claiming
// session, which the work-store import reads as the closer.
func closer(it *work.Item) string {
	if it.Holder != nil {
		return it.Holder.Session
	}
	return currentSession()
}

// undoCreate closes an item a command made before its records step failed, so the failure leaves no stray item; the
// work store has no delete.
func undoCreate(ws work.Store, id string, err error) error {
	if cerr := ws.Close(id, "the records step of the command that made it failed", work.Dismissed, ""); cerr != nil {
		return fmt.Errorf("%w; closing %s as dismissed failed too: %v", err, id, cerr)
	}
	return fmt.Errorf("%w; the work-store step is undone: %s closed as dismissed", err, id)
}

// ---------------------------------------------------------------- finding, feedback, doc, design, postmortem
