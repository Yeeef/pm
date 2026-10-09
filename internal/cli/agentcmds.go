package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Yeeef/yeeef-agents/pm/internal/store"
)

// agentCommand is the body of a command that runs against the records store: what it prints, or why it refused.
type agentCommand func(e *env, p *Parsed) (string, error)

// agentCommands is the commands Go pm runs on the records and the work store, by name.
var agentCommands = map[string]agentCommand{
	"show":           cmdShow,
	"record link":    cmdRecordLink,
	"commit":         cmdCommit,
	"task add":       cmdTaskAdd,
	"task close":     cmdTaskClose,
	"task claim":     cmdTaskClaim,
	"task move":      cmdTaskMove,
	"finding add":    cmdFindingAdd,
	"feedback add":   cmdFeedbackAdd,
	"doc new":        cmdDocNew,
	"design new":     cmdDesignNew,
	"postmortem new": cmdPostmortemNew,
	"project open":   cmdProjectOpen,
	"project close":  cmdProjectClose,
	"sprint open":    cmdSprintOpen,
	"sprint close":   cmdSprintClose,
}

// writes is the commands that write records: each runs under the records store's lock, taken after the work store's
// gate (the work-store page: a command that writes both stores takes the gate first and the lock second).
var writes = map[string]bool{"finding add": true, "feedback add": true, "decision add": true, "decision need": true,
	"decision close": true, "action need": true, "action done": true, "doc new": true, "design new": true,
	"postmortem new": true, "project open": true, "sprint open": true, "sprint close": true, "task add": true,
	"task close": true, "task move": true, "commit": true}

// runAgent runs an agent command as Python's main() does: the body from --text-file first (a slow pipe holds no
// lock), then the records store, then the command, which checks its arguments before it opens the work store; a write
// takes the records lock as it opens the store (env.work), so the gate always comes first. It prints what the command
// returns.
func runAgent(name string, cmd agentCommand, p *Parsed, here string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	e := &env{here: here, stdin: stdin, stdout: stdout, stderr: stderr}
	for _, a := range p.cmd.args { // argparse's type=str.strip on --text
		if contains(a.flags, "--text") {
			for i, v := range p.values["text"] {
				p.values["text"][i] = strip(v)
			}
		}
	}
	if _, given := p.values["text_file"]; given {
		text, err := e.body(p)
		if err != nil {
			return err
		}
		p.values["text"] = []string{text}
		delete(p.values, "text_file")
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
	_, err = fmt.Fprintln(stdout, out)
	return err
}
