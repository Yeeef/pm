package hooks

// pm hook inbox, on UserPromptSubmit and Stop in Claude Code and Codex: the owner's replies to this session's needs
// that have not reached it. The pm service pushes a reply into a Claude Code session's inbox and marks it delivered, so
// there the hook finds nothing unless a push failed; Codex has no inbox, so this is how a reply reaches a Codex session
// that does not wait with pm reply wait. Each reply goes in as the push writes it, and is marked delivered as the push
// marks it: on UserPromptSubmit as context beside the prompt, on Stop as a block whose reason holds the replies (none
// on stop_hook_active, so it never loops). With nothing undelivered it prints nothing. It fails open: on its own error
// it says why on stderr and lets the turn go on.

import (
	"fmt"
	"io"
	"strings"

	"github.com/Yeeef/pm/internal/pyjson"
)

// Pending is the session's undelivered replies, each as the push writes it, marked delivered as it reads them; dir is
// where the hook runs.
type Pending func(dir, session string) ([]string, error)

// HookInbox is pm hook inbox. here is the process's directory.
func HookInbox(here string, pending Pending, stdin io.Reader, stdout, stderr io.Writer) error {
	event, err := ReadEvent(stdin)
	if err != nil {
		return err
	}
	if event == nil {
		fmt.Fprintln(stderr, "pm hook inbox: the hook input is not a JSON object; no reply handed over")
		return nil
	}
	name, _ := event.Get("hook_event_name").(string)
	if name != "UserPromptSubmit" && pyjson.Truthy(event.Get("stop_hook_active")) {
		return nil
	}
	session, _ := event.Get("session_id").(string)
	if session == "" {
		return nil // no session, so no request of its own
	}
	dir := here
	if cwd, ok := event.Get("cwd").(string); ok && cwd != "" {
		dir = cwd
	}
	texts, err := pending(dir, session)
	if err != nil {
		fmt.Fprintf(stderr, "pm hook inbox: reading this session's replies failed (%s); no reply handed over\n",
			strings.Join(strings.Fields(err.Error()), " "))
		return nil
	}
	if len(texts) == 0 {
		return nil
	}
	text := "The owner replied to requests this session raised; pm marked the replies delivered:\n\n" +
		strings.Join(texts, "\n\n")
	var out any
	if name == "UserPromptSubmit" {
		out = [][2]any{{"hookSpecificOutput", [][2]any{{"hookEventName", name}, {"additionalContext", text}}}}
	} else {
		out = [][2]any{{"decision", "block"}, {"reason", text}}
	}
	_, err = fmt.Fprintln(stdout, pyjson.Dumps(out, true))
	return err
}
