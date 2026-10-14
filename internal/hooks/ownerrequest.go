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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

const (
	authorized    = "authorized"                          // a needless ask: blocks even when an open request matches
	askedWithTool = "(asked with the ask tool this turn)" // how the judge sees a question asked with the ask tool
)

// askTool is how a block reason names the runtime's ask tool.
var askTool = map[ask.Runtime]string{ask.Claude: "the AskUserQuestion tool",
	ask.Codex: "your ask tool (request_user_input_async; request_user_input in Plan mode)"}

// judgeTimeout is how long the judge may take on each runtime, inside its hook entry's timeout (Claude Code's 30 s,
// Codex's 45 s). Measured by the live eval, 8 calls at a time: Claude's verdict median 2 s, max 12 s; Codex's median
// 5.5 s, p90 14 s, max 20 s (codex exec starts slower, and its model reasons at medium effort).
var judgeTimeout = map[ask.Runtime]time.Duration{ask.Claude: 15 * time.Second, ask.Codex: 40 * time.Second}

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
		if err := writeJSON(stdout, blockFor(reason)); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// decide is the block reason for this stop, or "" to let it through.
func decide(event Event, here string, runtime ask.Runtime, requests Requests) (string, error) {
	if truthy(event["stop_hook_active"]) {
		return "", nil
	}
	session, ok := event["session_id"].(string)
	if !ok || session == "" {
		return "", hookErrorf("the hook input has no session_id")
	}
	var reply string // Codex sends null for a turn that ended without a message, as missing is: nothing was asked
	switch m := event["last_assistant_message"].(type) {
	case nil:
	case string:
		reply = m
	default:
		return "", hookErrorf("the hook input's last_assistant_message is a %s, not a string", jsonType(m))
	}
	if config.PyStrip(reply) == "" {
		return "", nil
	}
	dir := here
	if cwd, ok := event["cwd"].(string); ok && cwd != "" { // Python ran bd there; None is the process's directory
		dir = cwd
	}
	open, err := requests(dir, session)
	if err != nil {
		return "", hookErrorf("reading this session's open needs failed: %s", strings.Join(strings.Fields(err.Error()), " "))
	}
	if path, ok := event["transcript_path"].(string); ok && path != "" { // what the owner sees asked by the tool
		for i, q := range ToolAsks(path) {
			open = append(open, Request{ID: fmt.Sprintf("ask-tool-%d", i+1), Title: q + " " + askedWithTool})
		}
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
		reasons = append(reasons, strings.NewReplacer("{asks}", quoted(unmatched), "{ask_tool}", askTool[runtime]).
			Replace(pm.OwnerRequestReason))
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
	stop := stopJudgeOnSignal()
	defer stop()
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
		return nil, hookErrorf("%s answered out of shape: %q", runtime.Command(), head(out, 300))
	}
	return items, nil
}

// stopJudgeOnSignal makes a SIGTERM, SIGINT or SIGHUP to the hook (the runtime killing it at its entry's timeout) kill
// the judge's process group too, which runs in a group of its own and would outlive the hook, then exit 1 as a check
// that did not run. The func it returns stops the handling.
func stopJudgeOnSignal() func() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigs:
			proc.KillGroups()
			fmt.Fprintf(os.Stderr, "pm hook owner-request: stopped by %s; the owner-request check did not run\n", sig)
			os.Exit(1)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}

var jsonSpan = regexp.MustCompile(`(?s)\{.*\}`) // the first { to the last }, as Python's re.search(r"\{.*\}", re.S)

// parseJudged is the judge's answer, {"items": [{"quote": str, "kind": str, "match": str or null}]} in the span from
// its first { to its last }; false when it is out of that shape.
func parseJudged(out string) ([]judged, bool) {
	span := jsonSpan.FindString(out)
	if span == "" {
		return nil, false
	}
	var answer struct {
		Items *[]struct {
			Quote *string `json:"quote"`
			Kind  *string `json:"kind"`
			Match *string `json:"match"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(span), &answer); err != nil || answer.Items == nil {
		return nil, false
	}
	items := make([]judged, 0, len(*answer.Items))
	for _, e := range *answer.Items {
		if e.Quote == nil || e.Kind == nil {
			return nil, false
		}
		items = append(items, judged{*e.Quote, *e.Kind, e.Match})
	}
	return items, true
}
