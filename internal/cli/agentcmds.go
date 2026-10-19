package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Yeeef/pm/internal/store"
	"slices"
)

// agentCommand is the body of a command that runs against the records store: what it prints, or why it refused.
type agentCommand func(e *env, p *Parsed) (string, error)

// agentCommands is the commands Go pm runs on the records and the work store, by name.
var agentCommands = map[string]agentCommand{
	"show":             cmdShow,
	"day summarize":    cmdDaySummarize,
	"record link":      cmdRecordLink,
	"commit":           cmdCommit,
	"task add":         cmdTaskAdd,
	"task close":       cmdTaskClose,
	"task claim":       cmdTaskClaim,
	"task move":        cmdTaskMove,
	"finding add":      cmdFindingAdd,
	"feedback add":     cmdFeedbackAdd,
	"doc new":          cmdDocNew,
	"doc rename":       cmdDocRename,
	"design new":       cmdDesignNew,
	"design unchanged": cmdDesignUnchanged,
	"design rename":    cmdDesignRename,
	"postmortem new":   cmdPostmortemNew,
	"project open":     cmdProjectOpen,
	"project close":    cmdProjectClose,
	"project reopen":   cmdProjectReopen,
	"sprint open":      cmdSprintOpen,
	"sprint close":     cmdSprintClose,
	"sprint move":      cmdSprintMove,
	"sprint edit":      cmdSprintEdit,
	"sprint reopen":    cmdSprintReopen,
	"need edit":        cmdNeedEdit,
	"need close":       cmdNeedClose,
	"decision add":     cmdDecisionAdd,
	"decision need":    cmdDecisionNeed,
	"action need":      cmdActionNeed,
	"reply read":       cmdReplyRead,
}

// writes is the commands that write records: each runs under the records store's lock, the only lock a command that
// writes both stores takes (the pm-go page, Store access).
var writes = map[string]bool{"finding add": true, "feedback add": true, "decision add": true, "decision need": true,
	"need close": true, "action need": true, "doc new": true, "doc rename": true, "design new": true,
	"design rename": true, "design unchanged": true,
	"postmortem new": true, "project open": true, "sprint open": true, "sprint close": true,
	"sprint move": true, "sprint edit": true, "project reopen": true, "sprint reopen": true, "task add": true,
	"task close": true, "task move": true, "commit": true}

// runAgent runs an agent command as Python's main() does: the body from --text-file first (a slow pipe holds no
// lock), then the records store, then the command, which checks its arguments before it connects to the work store; a
// write takes the records lock as it connects (env.work). It prints what the command returns.
func runAgent(name string, cmd agentCommand, p *Parsed, here string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	e := &env{here: here, stdin: stdin, stdout: stdout, stderr: stderr}
	for _, a := range p.Cmd.Args { // --text is taken without its surrounding whitespace
		if slices.Contains(a.Flags, "--text") {
			for i, v := range p.Values["text"] {
				p.Values["text"][i] = strip(v)
			}
		}
	}
	if _, given := p.Values["text_file"]; given {
		text, err := e.body(p)
		if err != nil {
			return err
		}
		p.Values["text"] = []string{text}
		delete(p.Values, "text_file")
	}
	if e.records, err = store.Find(here); err != nil {
		return err
	}
	e.lockRecords = writes[name]
	defer func() { err = errors.Join(err, e.release()) }()
	if name == "commit" { // it reads the store's state first: under the lock from the start, as in Python pm
		if _, err := e.work(); err != nil {
			return err
		}
	}
	out, err := cmd(e, p)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(stdout, out); err != nil || !e.lockRecords || name == "commit" {
		return err
	}
	return e.followWrite(stdout)
}

// closedSprints is which sprints the work store holds closed (store.Follow reads it).
func (e *env) closedSprints() (store.SprintClosed, error) {
	ws, err := e.work()
	if err != nil {
		return nil, err
	}
	items, err := ws.Items()
	return store.ClosedSprints(items), err
}

// followWrite brings this worktree's records workspace, if it has one, onto the records branch the write just
// committed on (store.Follow), and names a file the follow left with conflict markers.
func (e *env) followWrite(stdout io.Writer) error {
	w, err := store.WorkspaceOf(e.here)
	if err != nil || w == nil {
		return err
	}
	if _, err := e.work(); err != nil { // the records lock, which a write took as it connected
		return err
	}
	f, err := store.Follow(e.records, *w, e.closedSprints)
	if err != nil {
		return fmt.Errorf("the write is committed on the records branch, but bringing the workspace %s onto it failed, "+
			"which changed nothing there: %w", w.Dir, err)
	}
	for _, rel := range f.Conflicts {
		if _, err := fmt.Fprintf(stdout, "records/%s: the records branch changed it under your uncommitted edit; "+
			"resolve the conflict markers there, then pm commit it\n", rel); err != nil {
			return err
		}
	}
	return nil
}
