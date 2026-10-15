package hooks

// How an agent may ask the owner depends on the session's mode (session.Interactive): in autopilot, never only in
// chat, which pm hook owner-request enforces; in interactive mode, in chat when it needs to. So the rule is no part of
// prime.md, which every session and subagent gets alike: a SessionStart hook of its own (pm prime --request-rule, on
// startup, resume, clear and compact in both runtimes, beside the rules chunks) gives the rule of the session's mode,
// reading nothing but the mode file, so it arrives when the state hook fails or times out; and pm interactive and pm
// autopilot print the rule of the mode they set, since no start hook fires at a switch mid-session. A subagent gets
// neither: it reports to its parent, never to the owner, and its stop is not judged.

import (
	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/session"
)

// RequestRule is the request rule of a mode, under a heading naming it.
func RequestRule(interactive bool) string {
	if interactive {
		return "# Asking the owner: this session is in interactive mode\n\n" + pm.RequestRuleInteractive
	}
	return "# Asking the owner: this session is in autopilot\n\n" + pm.RequestRuleAutopilot
}

// requestRuleFor is the request rule of session id's mode in the clone containing cwd (nil: the process's directory);
// autopilot's, the strict one, when no session is named or its mode cannot be read.
func requestRuleFor(cwd *string, id string) string {
	if id == "" {
		return RequestRule(false)
	}
	main, perr := mainOf(cwd)
	if perr != nil {
		return RequestRule(false)
	}
	on, err := session.Interactive(main, id)
	return RequestRule(err == nil && on)
}

// SetMode is pm interactive (on) and pm autopilot for session id in the clone containing here: the mode set
// (session.SetMode), then what they print, its line and the request rule of the mode set.
func SetMode(here, id string, on bool) (string, error) {
	main, perr := mainOf(&here)
	if perr != nil {
		return "", perr
	}
	line, err := session.SetMode(main, id, on)
	if err != nil {
		return "", err
	}
	return line + "\n\n" + RequestRule(on), nil
}
