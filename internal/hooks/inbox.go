package hooks

// pm hook inbox, on UserPromptSubmit in Claude Code and Codex: the owner's replies to this session's needs that have
// not reached it, as context beside the prompt. The pm service pushes a reply into a Claude Code session's inbox and
// marks it delivered, so there the hook finds nothing unless a push failed; Codex has no inbox, so this is how a reply
// reaches a Codex session that does not wait with pm reply wait. Each reply goes in as the push writes it, and is
// marked delivered only once printed. It is not on Stop: a block there makes the turn's continuation end with
// stop_hook_active, which lets the stop checks through on the very turn that acts on the reply. With nothing
// undelivered it prints nothing. It fails open: on its own error it says why on stderr and lets the prompt go on.

import (
	"fmt"
	"io"
	"strings"

	"github.com/Yeeef/pm/internal/pyjson"
)

// Pending is the session's undelivered replies, each as the push writes it, and deliver, which marks them delivered
// and lets go of the store; dir is where the hook runs. deliver is nil when err is set.
type Pending func(dir, session string) (texts []string, deliver func() error, err error)

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
	session, ok := event.Get("session_id").(string)
	if event.Get("hook_event_name") != "UserPromptSubmit" || !ok || session == "" {
		return nil // no prompt to add to, or no session with requests of its own
	}
	dir := here
	if cwd, ok := event.Get("cwd").(string); ok && cwd != "" {
		dir = cwd
	}
	texts, deliver, err := pending(dir, session)
	if err != nil {
		fmt.Fprintf(stderr, "pm hook inbox: reading this session's replies failed (%s); no reply handed over\n",
			words(err.Error()))
		return nil
	}
	if len(texts) > 0 {
		text := "The owner replied to requests this session raised:\n\n" + strings.Join(texts, "\n\n")
		out := [][2]any{{"hookSpecificOutput", [][2]any{{"hookEventName", "UserPromptSubmit"}, {"additionalContext", text}}}}
		if _, err := fmt.Fprintln(stdout, pyjson.Dumps(out, true)); err != nil {
			return err // not printed, so not marked: the next prompt hands it over again
		}
	}
	if err := deliver(); err != nil { // printed but not marked: it may come again, never lost
		fmt.Fprintf(stderr, "pm hook inbox: marking the replies delivered failed (%s); they may be handed over "+
			"again\n", words(err.Error()))
	}
	return nil
}
