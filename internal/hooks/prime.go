// Package hooks is what the runtimes' hooks run: pm prime (session and subagent context) and pm hook stop.
//
// Every hook fails open: it exits 0 and says on stderr why it let the event through, because a broken hook must never
// stop a session from starting or an agent from stopping. The owner-request judge, when it cannot run, exits 1 with the
// cause on stderr instead, which still lets the stop through: Claude Code shows the error, Codex takes it as a pass.
// The exception runs before any hook: pm's check of the repo's .pm/config.toml, which fails hard.
//
// pm prime prints prime.md then the noun list, which Commands builds from the command tree (Machinery, the commands
// left out). Claude Code passes a hook's additionalContext inline only up to Cap characters, per hook, and the hooks of
// one entry arrive in any order, so the rules run as one hook per chunk: Chunks cuts them at the headings in Starts and
// puts a title line naming each chunk's place and sections on top (pm prime --rules N); the state runs as its own hook,
// cut at a line. TestChunksFitTheCapAndAddUpToTheHead fails when a chunk outgrows Cap. Beside prime: stop.go pm hook
// stop (uncommitted records), ownerrequest.go pm hook owner-request (the judge on the session's runtime, --runtime
// claude or codex, through internal/ask), asktool.go the runtimes' ask tools the judge lets through, inbox.go pm hook
// inbox (UserPromptSubmit, never Stop).
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/proc"
)

const (
	Cap     = 10_000 // Claude Code's additionalContext limit, in characters (runes, as Python's len counts)
	timeout = 20     // seconds; pm show takes about 1 s
	// seconds; pm init in a set-up clone takes about 1 s, and one that installs a missing pm service waits up to 15 s
	// for its site (session start never restarts an installed one)
	initTimeout  = 18
	whereTimeout = 3  // seconds; pm where takes about 0.5 s
	budget       = 28 // seconds for init, where and show together, under the state hook's 30 s timeout
	header       = "Project state from `pm show` at session start, %s UTC: a snapshot to orient by, which other " +
		"sessions may have changed since; run `pm show` again before stating project state to the owner.\n\n"
	cut = "\n… cut at the hook's 10,000-character limit; run `pm show` for the rest."
)

// Machinery is what the runtimes and the scheduler call, not agents: left out of the command list.
var Machinery = map[string]bool{"prime": true, "hook": true, "push": true}

// Starts are where Chunks cuts Head: the heading line each chunk starts with. Claude Code passes each hook's
// additionalContext inline only up to Cap characters (longer reaches the model as a 2 KB preview and a file path), so
// the rules run as one hook per chunk. The hooks of one entry run in parallel and arrive in any order, so each chunk
// starts with a title naming its place and its sections. The hook entries name each chunk by number: a new chunk is a
// new hook entry in every runtime's settings.
var Starts = []string{"# pm rules", "## 3. Records", "# How"}

// Self is the pm that pm prime --state runs for init, where and show: this binary, as Python runs its own
// interpreter. A test may point it at another program.
var Self = os.Executable

// Rules is prime.md, as Python reads it: text-mode newlines, stripped.
func Rules() string {
	return config.PyStrip(strings.ReplaceAll(strings.ReplaceAll(pm.Rules, "\r\n", "\n"), "\r", "\n"))
}

// Commands is the command list: the agent-facing nouns in the order the command tree defines them.
func Commands(nouns []string) string {
	var named []string
	for _, n := range nouns {
		if !Machinery[n] {
			named = append(named, "`"+n+"`")
		}
	}
	return "# Commands\n\n`pm` nouns: " + strings.Join(named, ", ") + "."
}

// Head is the rules and the command list, whole and in order: what Chunks splits.
func Head(nouns []string) string { return Rules() + "\n\n" + Commands(nouns) }

// Chunks is Head cut at the lines in Starts, each chunk under a title line naming its place and sections. Without the
// titles, the chunks joined by blank lines are Head. A heading in Starts that is missing or out of order is an error.
func Chunks(nouns []string) ([]string, error) {
	lines := strings.Split(Head(nouns), "\n")
	at := make([]int, len(Starts))
	for i, s := range Starts {
		at[i] = -1
		for j, l := range lines {
			if l == s {
				at[i] = j
				break
			}
		}
		if at[i] < 0 || (i > 0 && at[i] < at[i-1]) || at[0] != 0 {
			return nil, fmt.Errorf("the chunk headings %q are not in order at the top of prime.md", Starts)
		}
	}
	var out []string
	part := ""
	for i := range at {
		end := len(lines)
		if i+1 < len(at) {
			end = at[i+1]
		}
		body := config.PyStrip(strings.Join(lines[at[i]:end], "\n"))
		type group struct {
			part     string
			sections []string
		}
		var groups []*group
		if !strings.HasPrefix(body, "# ") { // a chunk that starts inside a part names it
			groups = append(groups, &group{part: part})
		}
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "# ") {
				part = line[2:]
				groups = append(groups, &group{part: part})
			} else if strings.HasPrefix(line, "## ") {
				groups[len(groups)-1].sections = append(groups[len(groups)-1].sections, line[3:])
			}
		}
		var what []string
		for _, g := range groups {
			name := g.part
			if name == "pm rules" {
				name = "the introduction"
			}
			if len(g.sections) > 0 {
				name += " — " + strings.Join(g.sections, ", ")
			}
			what = append(what, name)
		}
		out = append(out, fmt.Sprintf("# pm rules (%d of %d): %s\n\n%s", i+1, len(at), strings.Join(what, "; "), body))
	}
	return out, nil
}

// failure is the line Python prints for a command that exited non-zero: (stderr or stdout).strip().splitlines().
func failure(res proc.Result) []string {
	why := res.Stderr
	if why == "" {
		why = res.Stdout
	}
	return splitlines(config.PyStrip(why))
}

func exitText(code int) string { return fmt.Sprintf("exit %d", code) }

func envWithout(keys ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !slices.ContainsFunc(keys, func(key string) bool { return strings.HasPrefix(kv, key+"=") }) {
			env = append(env, kv)
		}
	}
	return env
}

func self(args ...string) ([]string, error) {
	exe, err := Self()
	if err != nil {
		return nil, err
	}
	return append([]string{exe}, args...), nil
}

// Init is what pm init did, ending in a blank line, or one line saying why it did not run. $PORT is left out: a
// session's environment is no request to move the clone's service.
func Init(cwd *string) (string, error) {
	argv, err := self("init", "--session-start")
	if err != nil {
		return "", err
	}
	// The child is this binary for the same pin: a pm a launcher launched passes its markers on, so pm init knows the
	// launcher owns the bin dir and leaves it alone (the markers are out of this process's environment by now).
	env := append(envWithout("PORT"), launch.Markers()...)
	res, err := proc.Run(argv, proc.Options{Cwd: cwd, Env: env, Timeout: initTimeout * time.Second,
		TimeoutText: fmt.Sprint(initTimeout)})
	if e := (*proc.Error)(nil); errors.As(err, &e) {
		return fmt.Sprintf("pm init did not run at session start (%s: %s); run `pm init` by hand.\n\n", e.Type, e.Msg), nil
	}
	if res.Code != 0 {
		line := exitText(res.Code)
		if why := failure(res); len(why) > 0 {
			line = why[len(why)-1]
			for _, l := range why {
				if strings.HasPrefix(l, "error: ") {
					line = l
					break
				}
			}
		}
		return fmt.Sprintf("pm init failed at session start (%s); run `pm init` by hand.\n\n", line), nil
	}
	return fmt.Sprintf("`pm init` at session start:\n%s\n\n", config.PyStrip(res.Stdout)), nil
}

// Where is pm where under a heading, ending in a blank line, or one line saying why it did not run.
func Where(cwd *string) (string, error) {
	argv, err := self("where")
	if err != nil {
		return "", err
	}
	res, err := proc.Run(argv, proc.Options{Cwd: cwd, Timeout: whereTimeout * time.Second,
		TimeoutText: fmt.Sprint(whereTimeout)})
	if e := (*proc.Error)(nil); errors.As(err, &e) {
		return fmt.Sprintf("pm where did not run at session start (%s: %s); run `pm where` by hand.\n\n", e.Type, e.Msg), nil
	}
	if res.Code != 0 {
		line := exitText(res.Code)
		if why := failure(res); len(why) > 0 {
			line = why[len(why)-1]
		}
		return fmt.Sprintf("pm where failed at session start (%s); run `pm where` by hand.\n\n", line), nil
	}
	return fmt.Sprintf("Locations from `pm where` at session start:\n%s\n\n", config.PyStrip(res.Stdout)), nil
}

// Context is pm show under a header, cut at a line to capacity characters, or one line when it fails. It runs as the
// starting session, with --refresh-inbox. A Claude Code hook's environment is its session's ($CLAUDE_CODE_SESSION_ID
// is the input's session_id), messaging socket included. Any other is Codex's, whose hooks get no CODEX_* variables
// and may inherit a Claude Code session's (Codex started from one): it runs as $CODEX_SESSION_ID, without the Claude
// Code session's variables, so its needs never point at that session's inbox.
func Context(cwd *string, session string, capacity int, wait time.Duration, waitText string, now func() time.Time) (string, error) {
	argv, err := self("show", "--refresh-inbox")
	if err != nil {
		return "", err
	}
	var env []string
	switch {
	case session == "":
	case os.Getenv("CLAUDE_CODE_SESSION_ID") == session:
		env = os.Environ()
	default:
		env = append(envWithout("CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN",
			"CODEX_SESSION_ID", "CODEX_THREAD_ID"), "CODEX_SESSION_ID="+session)
	}
	res, err := proc.Run(argv, proc.Options{Cwd: cwd, Env: env, Timeout: wait, TimeoutText: waitText})
	if e := (*proc.Error)(nil); errors.As(err, &e) {
		return fmt.Sprintf("pm show did not run at session start (%s: %s); run `pm show` by hand.", e.Type, e.Msg), nil
	}
	if res.Code != 0 {
		line := exitText(res.Code)
		if why := failure(res); len(why) > 0 {
			line = why[len(why)-1]
		}
		return fmt.Sprintf("pm show failed at session start (%s); run `pm show` by hand.", line), nil
	}
	text := []rune(fmt.Sprintf(header, now().UTC().Format("2006-01-02 15:04")) + config.PyStrip(res.Stdout))
	if len(text) > capacity {
		at := rfindNewline(text, capacity-utf8.RuneCountInString(cut))
		if at < 0 { // Python's text[:-1]
			at = len(text) - 1
		}
		return string(text[:at]) + cut, nil
	}
	return string(text), nil
}

// rfindNewline is Python's text.rfind("\n", 0, end), end counted as a slice bound.
func rfindNewline(text []rune, end int) int {
	if end < 0 {
		end += len(text)
		if end < 0 {
			end = 0
		}
	}
	if end > len(text) {
		end = len(text)
	}
	for i := end - 1; i >= 0; i-- {
		if text[i] == '\n' {
			return i
		}
	}
	return -1
}

// State is pm prime --state: what pm init did and pm where, then pm show, all within Cap, so pm show gets what the
// init and where lines leave and loses its last part first. trust, when set, gives a line to open it with.
func State(cwd *string, session string, trust func(cwd string) string) (string, error) {
	start := time.Now()
	said := make(chan string, 1) // trust's line, asked beside init and where, which take seconds of their own
	if trust != nil && cwd != nil {
		go func() { said <- trust(*cwd) }()
	} else {
		said <- ""
	}
	in, err := Init(cwd)
	if err != nil {
		return "", err
	}
	wh, err := Where(cwd)
	if err != nil {
		return "", err
	}
	first := in + wh
	if line := <-said; line != "" { // bounded by trust itself, well inside the budget
		first = line + "\n\n" + first
	}
	// pm show gets what init and where left: min(TIMEOUT, max(1.0, BUDGET - elapsed))
	left := float64(timeout)
	if rest := budget - time.Since(start).Seconds(); rest < timeout {
		left = max(1.0, rest)
	}
	leftText := strconv.FormatFloat(left, 'f', -1, 64)
	show, err := Context(cwd, session, Cap-utf8.RuneCountInString(first), time.Duration(left*float64(time.Second)),
		leftText, time.Now)
	if err != nil {
		return "", err
	}
	return first + show, nil
}

// Subagent is what pm prime --subagent prints: pm's git rule for an agent, in one line. Python pm printed the Beads
// agent profile there (bd config get agent.profile); the work store has no profile, so the rule is pm's own (the
// work-store page, Constraints resolved).
const Subagent = "Git: commit and push are routine for agents unless your brief says otherwise; only the PR review " +
	"and the merge wait on the owner."

// Event is a hook's input: a JSON object as encoding/json reads one into any.
type Event map[string]any

// ReadEvent is the hook input JSON on stdin ({} when empty), or nil when it is not a JSON object.
func ReadEvent(stdin io.Reader) (Event, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return Event{}, nil
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, nil //nolint:nilerr // input that is not JSON reads as no event: a hook fails open
	}
	return event, nil // nil for JSON null
}

// eventString is an event field that pm passes on as a path or an id: absent or null is nil, a string is itself, and
// anything else is an input the runtimes never send, which fails the hook.
func eventString(event Event, key string) (*string, error) {
	switch v := event[key].(type) {
	case nil:
		return nil, nil
	case string:
		return &v, nil
	default:
		return nil, fmt.Errorf("the hook input's %s is a %s, not a string", key, jsonType(v))
	}
}

// truthy is whether a JSON value is set: not null, false, 0, "", [] or {}.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}

// jsonType is the JSON type of a value encoding/json read into any.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// writeJSON prints v as one line of JSON, with <, > and & as they are.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// hookContext is the output that adds context for the event that ran the hook, as Claude Code and Codex both read it.
type hookContext struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func contextFor(event, text string) hookContext {
	var c hookContext
	c.HookSpecificOutput.HookEventName, c.HookSpecificOutput.AdditionalContext = event, text
	return c
}

// block is a Stop hook's output that blocks the stop, with the reason the agent is given.
type block struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func blockFor(reason string) block { return block{"block", reason} }

// Part is which part pm prime prints: a rules chunk (1-based), the state, the subagent line, or everything.
type Part struct {
	Chunk    int // 1.. for --rules N; 0 otherwise
	State    bool
	Subagent bool
	// Trust is the line the state opens with when the runtime skips some of pm's hooks (a Codex session's check of
	// Codex's trust); nil asks nothing.
	Trust func(cwd string) string
}

// CmdPrime is pm prime: the rules, the commands and the state, or one part of them; with hookJSON it reads the hook
// input on stdin and prints the envelope Claude Code and Codex both read, named for the event that ran it.
func CmdPrime(part Part, hookJSON bool, nouns []string, stdin io.Reader, stdout io.Writer) error {
	event := Event{}
	if hookJSON {
		e, err := ReadEvent(stdin)
		if err != nil {
			return err
		}
		if e != nil {
			event = e
		}
	}
	var text string
	if part.Chunk > 0 {
		chunks, err := Chunks(nouns)
		if err != nil {
			return err
		}
		text = chunks[part.Chunk-1]
	} else {
		cwd, err := eventString(event, "cwd")
		if err != nil {
			return err
		}
		if part.Subagent {
			text = Subagent
		} else {
			var session string // only pm show reads it
			if truthy(event["session_id"]) {
				s, err := eventString(event, "session_id")
				if err != nil {
					return err
				}
				session = *s
			}
			if text, err = State(cwd, session, part.Trust); err != nil {
				return err
			}
			if !part.State {
				text = Head(nouns) + "\n\n" + text
			}
		}
	}
	if hookJSON {
		name, ok := event["hook_event_name"].(string)
		if !ok || name == "" {
			name = "SessionStart"
			if part.Subagent {
				name = "SubagentStart"
			}
		}
		return writeJSON(stdout, contextFor(name, text))
	}
	_, err := fmt.Fprintln(stdout, text)
	return err
}

// splitlines is Python's str.splitlines().
func splitlines(s string) []string {
	var out []string
	start := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			out = append(out, string(rs[start:i]))
			if rs[i] == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}
