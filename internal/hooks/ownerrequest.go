package hooks

// pm hook owner-request, the Stop hook wired in Claude Code and Codex: an agent may not end its turn asking the owner
// for something only in chat. Every such request in the final reply must match an open need that
// this session raised. It reads this session's open needs from the work store, then asks a small model on the session's
// own runtime (--runtime, which each runtime's hook entry names: Claude Haiku through `claude -p`, or a small Codex model
// through `codex exec`; internal/ask) to list the owner requests in the reply and the open need each one matches; a
// request that matches none, or a needless ask for leave to take an authorized step, blocks the stop once. On
// stop_hook_active, or a blank, null or missing reply, it lets the stop through without reading anything. It fails
// hard, never silently, the same on either runtime: when the input is unusable, or the work store or the model call
// fails, it exits 1 with the cause on stderr.

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/pyjson"
)

const authorized = "authorized" // a needless ask: blocks even when an open request matches

// judgeTimeout is how long the judge may take on each runtime, inside the hook entry's 30 s: a Claude verdict takes
// about 2 s, a Codex one about 5 s (codex exec starts slower).
var judgeTimeout = map[ask.Runtime]time.Duration{ask.Claude: 15 * time.Second, ask.Codex: 20 * time.Second}

var asks = map[string]bool{"decision": true, "review": true, "action": true, "clarification": true} // kinds that block unless one matches

// Request is one open need of the session, as the judge sees it.
type Request struct{ ID, Title, Description string }

// Requests reads the open needs the session raised; dir is where the hook runs.
type Requests func(dir, session string) ([]Request, error)

// hookError is a check that could not run.
type hookError struct{ msg string }

func (e *hookError) Error() string { return e.msg }

func hookErrorf(format string, a ...any) error { return &hookError{fmt.Sprintf(format, a...)} }

// HookOwnerRequest is pm hook owner-request: the hook input JSON on stdin; a block verdict on stdout, or nothing. It
// returns the exit code: 1 when the check did not run, with the cause on stderr. here is the process's directory;
// runtime is the session's, which the judge runs on.
func HookOwnerRequest(here string, runtime ask.Runtime, requests Requests, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	event, err := ReadEvent(stdin)
	if err != nil {
		return 0, err
	}
	if event == nil {
		fmt.Fprintln(stderr, "pm hook owner-request: the hook input is not a JSON object; the owner-request check did not run")
		return 1, nil
	}
	reason, err := decide(event, here, runtime, requests)
	var he *hookError
	if errors.As(err, &he) {
		fmt.Fprintf(stderr, "pm hook owner-request: %s; the owner-request check did not run\n", he.msg)
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if reason != "" {
		if _, err := fmt.Fprintln(stdout, pyjson.Dumps([][2]any{{"decision", "block"}, {"reason", reason}}, true)); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// decide is the block reason for this stop, or "" to let it through.
func decide(event *pyjson.Object, here string, runtime ask.Runtime, requests Requests) (string, error) {
	if pyjson.Truthy(event.Get("stop_hook_active")) {
		return "", nil
	}
	session, ok := event.Get("session_id").(string)
	if !ok || session == "" {
		return "", hookErrorf("the hook input has no session_id")
	}
	var reply string // Codex sends null for a turn that ended without a message, as missing is: nothing was asked
	switch m := event.Get("last_assistant_message").(type) {
	case nil:
	case string:
		reply = m
	default:
		return "", hookErrorf("the hook input's last_assistant_message is a %s, not a string", pyjson.TypeName(m))
	}
	if config.PyStrip(reply) == "" {
		return "", nil
	}
	dir := here
	if cwd, ok := event.Get("cwd").(string); ok && cwd != "" { // Python ran bd there; None is the process's directory
		dir = cwd
	}
	open, err := requests(dir, session)
	if err != nil {
		return "", hookErrorf("reading this session's open needs failed: %s", strings.Join(strings.Fields(err.Error()), " "))
	}
	items, err := judge(runtime, reply, open)
	if err != nil {
		return "", err
	}
	ids := map[string]bool{}
	for _, r := range open {
		ids[r.ID] = true
	}
	var needless, unmatched []string
	for _, it := range items {
		if it.kind == authorized {
			needless = append(needless, it.quote)
		}
		if asks[it.kind] && (it.match == nil || !ids[*it.match]) {
			unmatched = append(unmatched, it.quote)
		}
	}
	var reasons []string
	if needless != nil {
		reasons = append(reasons, strings.ReplaceAll(pm.OwnerRequestNeedless, "{asks}", quoted(needless)))
	}
	if unmatched != nil {
		reasons = append(reasons, strings.ReplaceAll(pm.OwnerRequestReason, "{asks}", quoted(unmatched)))
	}
	return strings.Join(reasons, "\n\n"), nil
}

// quoted is the quotes, each cut to 150 characters and in double quotes, joined and cut to 600 characters.
func quoted(quotes []string) string {
	parts := make([]string, len(quotes))
	for i, q := range quotes {
		parts[i] = `"` + head(q, 150) + `"`
	}
	return head(strings.Join(parts, "; "), 600)
}

// head is s[:n] in Python: its first n characters.
func head(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// words is Python's " ".join(s.split()).
func words(s string) string { return strings.Join(strings.FieldsFunc(s, config.IsPySpace), " ") }

// judgePrompt is the user prompt: this session's open requests, then the reply.
func judgePrompt(reply string, open []Request) string {
	lines := make([]string, len(open))
	for i, r := range open {
		lines[i] = "- " + r.ID + ": " + r.Title
		if r.Description != "" {
			lines[i] += "\n  " + head(words(r.Description), 400)
		}
	}
	listed := strings.Join(lines, "\n")
	if listed == "" {
		listed = "(none)"
	}
	return "OPEN REQUESTS:\n" + listed + "\n\nREPLY:\n<<<\n" + reply + "\n>>>"
}

// judged is one sentence of the reply that addresses the owner, as the judge sees it.
type judged struct {
	quote, kind string
	match       *string
}

// judge asks the model on runtime which sentences of reply address the owner, and the open request each matches.
func judge(runtime ask.Runtime, reply string, open []Request) ([]judged, error) {
	out, err := ask.Ask(runtime, ask.Call{System: pm.OwnerRequestSystem, Prompt: judgePrompt(reply, open),
		Timeout: judgeTimeout[runtime]})
	var ae *ask.Error
	if errors.As(err, &ae) {
		return nil, hookErrorf("%s", ae.Error())
	}
	if err != nil {
		return nil, err
	}
	items, ok := parseJudged(out)
	if !ok {
		return nil, hookErrorf("%s answered out of shape: %s", runtime.Command(), pyjson.StrRepr(head(out, 300)))
	}
	return items, nil
}

var jsonSpan = regexp.MustCompile(`(?s)\{.*\}`) // the first { to the last }, as Python's re.search(r"\{.*\}", re.S)

// parseJudged is the judge's answer, {"items": [{"quote": str, "kind": str, "match": str or null}]} in the span from
// its first { to its last }; false when it is out of that shape.
func parseJudged(out string) ([]judged, bool) {
	span := jsonSpan.FindString(out)
	if span == "" {
		return nil, false
	}
	v, err := pyjson.Loads(span)
	if err != nil {
		return nil, false
	}
	obj, ok := v.(*pyjson.Object)
	if !ok {
		return nil, false
	}
	list, ok := obj.Get("items").([]any)
	if !ok {
		return nil, false
	}
	items := make([]judged, 0, len(list))
	for _, e := range list {
		o, ok := e.(*pyjson.Object)
		if !ok {
			return nil, false
		}
		quote, ok1 := o.Get("quote").(string)
		kind, ok2 := o.Get("kind").(string)
		if !ok1 || !ok2 {
			return nil, false
		}
		var match *string
		switch m := o.Get("match").(type) {
		case nil:
		case string:
			match = &m
		default:
			return nil, false
		}
		items = append(items, judged{quote, kind, match})
	}
	return items, true
}
