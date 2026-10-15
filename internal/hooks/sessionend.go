package hooks

// pm hook session-end, on SessionEnd in Claude Code and Codex: the ended session's mode file removed
// (session.SetInteractive), so a session id never carries interactive mode past its session; pm clean removes what a
// session that crashed left. Claude Code fires it when a session ends (claude -p: at the end of each run); Codex at the
// end of codex exec, and about 90 s after its TUI exits. It fails open: on its own error it says why on stderr.

import (
	"fmt"
	"io"

	"github.com/Yeeef/pm/internal/session"
)

// HookSessionEnd is pm hook session-end. here is the process's directory.
func HookSessionEnd(here string, stdin io.Reader, stderr io.Writer) error {
	event, err := ReadEvent(stdin)
	if err != nil {
		return err
	}
	id, ok := event["session_id"].(string)
	if event == nil || event["hook_event_name"] != "SessionEnd" || !ok || id == "" {
		fmt.Fprintln(stderr, "pm hook session-end: the hook input names no ended session; no mode file removed")
		return nil
	}
	dir := here
	if cwd, ok := event["cwd"].(string); ok && cwd != "" {
		dir = cwd
	}
	main, perr := mainOf(&dir)
	if perr != nil {
		fmt.Fprintf(stderr, "pm hook session-end: finding the main checkout failed (%s); no mode file removed\n",
			words(perr.Error()))
		return nil
	}
	if err := session.SetInteractive(main, id, false); err != nil {
		fmt.Fprintf(stderr, "pm hook session-end: removing the session's mode file failed (%s)\n", words(err.Error()))
	}
	return nil
}
