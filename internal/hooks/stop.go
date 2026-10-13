package hooks

// pm hook stop: an agent may not hand back with records it edited by hand left uncommitted in the store. The store is
// <main checkout>/.pm/store/records, found from the clone's common git dir. When it has uncommitted files that this
// session's tool calls name (Claude Code tool_use inputs; Codex function_call arguments, custom_tool_call inputs and
// local_shell_call actions), the hook blocks the stop once with a reason naming them. A path that only the prompt of a
// subagent call still running names is left out: the subagent may be writing it. On stop_hook_active it always
// lets the stop through, so it never loops. Prints {"decision": "block", "reason": ...} to block, nothing otherwise. At a
// Codex SubagentStop it reads the subagent's own rollout, agent_transcript_path, instead of its parent's.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

const stopReason = "These records in the store (%[1]s) have uncommitted changes, and this session's tool calls name them:\n" +
	"%[2]s\n" +
	"Commit the ones you edited with `pm commit -m \"<why>\" <path>...` (paths as listed, under records/), or " +
	"revert them with `git -C %[1]s checkout -- <path>` (`rm` for a new file). Leave a file you did not edit: " +
	"another session is writing it."

// git is Python's git(): git with check=True and a 5 s timeout; a non-zero exit is CalledProcessError.
func git(cwd *string, args ...string) (string, *proc.Error) {
	argv := append([]string{"git"}, args...)
	res, err := proc.Run(argv, proc.Options{Cwd: cwd, Timeout: 5 * time.Second, TimeoutText: "5"})
	if e := (*proc.Error)(nil); errors.As(err, &e) {
		return "", e
	}
	if res.Code != 0 {
		return "", &proc.Error{Type: "CalledProcessError",
			Msg: fmt.Sprintf("Command %q returned non-zero exit status %d.", argv, res.Code)}
	}
	return res.Stdout, nil
}

// StoreOf is the records store of the clone containing cwd: .pm/store/records beside the clone's common .git dir.
func StoreOf(cwd *string) (string, *proc.Error) {
	common, err := git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(config.PyStrip(common)), config.Store), nil
}

// Dirty is the paths, relative to the store, of tracked files with changes and of untracked files.
func Dirty(store string) ([]string, *proc.Error) {
	out, err := git(&store, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	entries := strings.Split(out, "\x00")
	var paths []string
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len([]rune(e)) > 3 {
			paths = append(paths, string([]rune(e)[3:]))
			if strings.ContainsAny(e[:2], "RC") { // a rename or copy, staged or not, is followed by its source path
				i++
			}
		}
	}
	return paths, nil
}

// call is one tool call in a transcript line: its input as text, and its id when it starts a subagent (Claude Code's
// Agent or Task tool), else "".
type call struct{ input, agent string }

// toolCalls is the tool calls in one transcript line.
func toolCalls(entry map[string]any) ([]call, error) {
	switch entry["type"] {
	case "assistant":
		var content any
		switch m := entry["message"].(type) {
		case map[string]any:
			content = m["content"]
		default:
			if truthy(m) {
				return nil, fmt.Errorf("a transcript line's message is a %s, not an object", jsonType(m))
			}
		}
		blocks, ok := content.([]any)
		if !ok {
			return nil, nil
		}
		var out []call
		for _, b := range blocks {
			if o, ok := b.(map[string]any); ok && o["type"] == "tool_use" {
				c := call{input: dumps(o["input"])}
				if name := o["name"]; name == "Agent" || name == "Task" {
					if id, ok := o["id"].(string); ok {
						c.agent = id
					}
				}
				out = append(out, c)
			}
		}
		return out, nil
	case "response_item":
		payload, ok := entry["payload"].(map[string]any)
		if !ok {
			return nil, nil
		}
		field := func(key string) string { // a string as it is, anything else as JSON
			if v, ok := payload[key].(string); ok {
				return v
			}
			return dumps(payload[key])
		}
		switch payload["type"] {
		case "function_call":
			return []call{{input: field("arguments")}}, nil
		case "custom_tool_call":
			return []call{{input: field("input")}}, nil
		case "local_shell_call":
			return []call{{input: dumps(payload["action"])}}, nil
		}
	}
	return nil, nil
}

// returns is the ids among pending of the subagent calls that this transcript line says have returned: a tool result
// that is not a background launch, or a task notification naming the call.
func returns(entry map[string]any, pending []string) []string {
	var texts []string                          // the line's text, where a task notification can be
	if c, ok := entry["content"].(string); ok { // a queued message
		texts = append(texts, c)
	}
	var blocks []any
	if m, ok := entry["message"].(map[string]any); ok {
		switch c := m["content"].(type) {
		case string:
			texts = append(texts, c)
		case []any:
			blocks = c
		}
	}
	launch, ok := entry["toolUseResult"].(map[string]any)
	background := ok && truthy(launch["isAsync"])
	var out []string
	for _, id := range pending {
		done := false
		for _, b := range blocks {
			o, ok := b.(map[string]any)
			if !ok {
				continue
			}
			switch o["type"] {
			case "tool_result":
				done = done || (o["tool_use_id"] == id && !background)
			case "text":
				if t, ok := o["text"].(string); ok {
					texts = append(texts, t)
				}
			}
		}
		for _, t := range texts {
			done = done || (strings.Contains(t, "<task-notification>") && strings.Contains(t, "<tool-use-id>"+id+"</tool-use-id>"))
		}
		if done {
			out = append(out, id)
		}
	}
	return out
}

// touches is what a scan of a transcript has found so far.
type touches struct {
	paths    []string
	own      map[string]bool     // paths a tool call other than a subagent's start names
	byAgent  map[string][]string // a subagent call's id: the paths its prompt names
	returned map[string]bool     // subagent calls that have returned
}

// Touched is the paths among paths that some tool call in the transcript names, leaving out a path that only the
// prompt of a subagent call still running names: that subagent may be writing it. Lines that name none of the paths
// and no running subagent call that names one are not parsed.
func Touched(transcript string, paths []string) ([]string, error) {
	f, err := os.Open(transcript)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t := &touches{paths: paths, own: map[string]bool{}, byAgent: map[string][]string{}, returned: map[string]bool{}}
	r := bufio.NewReader(f)
	for {
		raw, rerr := r.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return nil, rerr
		}
		if raw == "" {
			break
		}
		if err := t.scanLines(raw); err != nil {
			return nil, err
		}
		if rerr == io.EOF {
			break
		}
	}
	found := map[string]bool{}
	for p := range t.own {
		found[p] = true
	}
	for id, ps := range t.byAgent {
		for _, p := range ps {
			found[p] = found[p] || t.returned[id]
		}
	}
	var mine []string
	for _, p := range paths {
		if found[p] {
			mine = append(mine, p)
		}
	}
	return mine, nil
}

// scanLines records the paths that the tool calls on these lines name, and the subagent calls they say returned. raw
// ends at a newline or the file's end; read as Python reads text (invalid UTF-8 replaced, a lone \r ending a line too),
// it can hold more than one line.
func (t *touches) scanLines(raw string) error {
	text := strings.ToValidUTF8(raw, string(utf8.RuneError))
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		var hits, pending []string
		for _, p := range t.paths {
			if !t.own[p] && strings.Contains(line, p) {
				hits = append(hits, p)
			}
		}
		for id := range t.byAgent {
			if !t.returned[id] && strings.Contains(line, id) {
				pending = append(pending, id)
			}
		}
		if len(hits) == 0 && len(pending) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry == nil {
			continue
		}
		for _, id := range returns(entry, pending) {
			t.returned[id] = true
		}
		calls, err := toolCalls(entry)
		if err != nil {
			return err
		}
		for _, p := range hits {
			for _, c := range calls {
				switch {
				case !strings.Contains(c.input, p):
				case c.agent == "":
					t.own[p] = true
				default:
					t.byAgent[c.agent] = append(t.byAgent[c.agent], p)
				}
			}
		}
	}
	return nil
}

// StopReason is the block reason for this stop, or "" to let it through; what it lets through for an error it says on
// stderr.
func StopReason(event Event, stderr io.Writer) (string, error) {
	if truthy(event["stop_hook_active"]) {
		return "", nil
	}
	cwd, err := eventString(event, "cwd")
	if err != nil {
		return "", err
	}
	store, paths, perr := func() (string, []string, *proc.Error) {
		store, err := StoreOf(cwd)
		if err != nil {
			return "", nil, err
		}
		if st, err := os.Stat(store); err != nil || !st.IsDir() {
			return "", nil, nil //nolint:nilerr // a clone without a store has no records to commit
		}
		top, err := git(&store, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", nil, err
		}
		if resolve(config.PyStrip(top)) != resolve(store) {
			fmt.Fprintf(stderr, "pm hook stop: %s is not a worktree of its own; letting the stop through\n", store)
			return "", nil, nil
		}
		paths, err := Dirty(store)
		return store, paths, err
	}()
	if perr != nil {
		fmt.Fprintf(stderr, "pm hook stop: git is unavailable (%s); letting the stop through\n", perr.Type)
		return "", nil //nolint:nilerr // the hook fails open on its own errors, named on stderr
	}
	if len(paths) == 0 {
		return "", nil
	}
	var mine []string
	readable := false
	key := "transcript_path"
	if truthy(event["agent_transcript_path"]) { // a subagent's stop: its own transcript
		key = "agent_transcript_path"
	}
	if truthy(event[key]) {
		transcript, err := eventString(event, key)
		if err != nil {
			return "", err
		}
		if mine, err = Touched(*transcript, paths); err == nil {
			readable = true
		} else if pe := (*os.PathError)(nil); !errors.As(err, &pe) {
			return "", err
		}
	}
	if !readable {
		fmt.Fprintf(stderr, "pm hook stop: no readable transcript, so the records this session touched are unknown; "+
			"letting the stop through with %d uncommitted in %s\n", len(paths), store)
		return "", nil
	}
	if len(mine) == 0 {
		return "", nil
	}
	files := make([]string, len(mine))
	for i, p := range mine {
		files[i] = "- records/" + p
	}
	return fmt.Sprintf(stopReason, store, strings.Join(files, "\n")), nil
}

// dumps is v, a value encoding/json read, as compact JSON with <, > and & as they are: a tool call's input as text, in
// which to look for a path.
func dumps(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // a value json.Unmarshal read always encodes
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// resolve is Python's Path.resolve(): symlinks followed where the path exists.
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// HookStop is pm hook stop.
func HookStop(stdin io.Reader, stdout, stderr io.Writer) error {
	event, err := ReadEvent(stdin)
	if err != nil {
		return err
	}
	if event == nil {
		fmt.Fprintln(stderr, "pm hook stop: hook input is not JSON; letting the stop through")
		return nil
	}
	reason, err := StopReason(event, stderr)
	if err != nil || reason == "" {
		return err
	}
	return writeJSON(stdout, blockFor(reason))
}
