package hooks

// pm hook stop: an agent may not hand back with records it edited by hand left uncommitted in the store. The store is
// <main checkout>/.pm/store/records, found from the clone's common git dir. When it has uncommitted files that this
// session's tool calls name (Claude Code tool_use inputs; Codex function_call arguments, custom_tool_call inputs and
// local_shell_call actions), the hook blocks the stop once with a reason naming them. On stop_hook_active it always
// lets the stop through, so it never loops. Prints {"decision": "block", "reason": ...} to block, nothing otherwise.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
	"github.com/Yeeef/yeeef-agents/pm/internal/pyjson"
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
	if e, ok := err.(*proc.Error); ok {
		return "", e
	}
	if res.Code != 0 {
		return "", &proc.Error{Type: "CalledProcessError",
			Msg: fmt.Sprintf("Command '%s' returned non-zero exit status %d.", proc.ReprList(argv), res.Code)}
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

// ToolInputs is the tool-call inputs in one transcript line, as text.
func ToolInputs(entry *pyjson.Object) ([]string, error) {
	switch entry.Get("type") {
	case "assistant":
		var content any
		switch m := entry.Get("message").(type) {
		case *pyjson.Object:
			content = m.Get("content")
		default:
			if pyjson.Truthy(m) {
				return nil, fmt.Errorf("a transcript line's message is a %s, not an object", pyjson.TypeName(m))
			}
		}
		blocks, ok := content.([]any)
		if !ok {
			return nil, nil
		}
		var out []string
		for _, b := range blocks {
			if o, ok := b.(*pyjson.Object); ok && o.Get("type") == "tool_use" {
				out = append(out, pyjson.Dumps(o.Get("input"), false))
			}
		}
		return out, nil
	case "response_item":
		payload, ok := entry.Get("payload").(*pyjson.Object)
		if !ok {
			return nil, nil
		}
		field := func(key string) any {
			if v, ok := payload.Values[key]; ok {
				return v
			}
			return ""
		}
		switch payload.Get("type") {
		case "function_call":
			return []string{pyjson.Str(field("arguments"))}, nil
		case "custom_tool_call":
			return []string{pyjson.Str(field("input"))}, nil
		case "local_shell_call":
			return []string{pyjson.Dumps(payload.Get("action"), false)}, nil
		}
	}
	return nil, nil
}

// Touched is the paths among paths that some tool call in the transcript names. Lines that name none are not parsed.
func Touched(transcript string, paths []string) ([]string, error) {
	f, err := os.Open(transcript)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	found := map[string]bool{}
	r := bufio.NewReader(f)
	for {
		raw, rerr := r.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return nil, rerr
		}
		if raw == "" {
			break
		}
		if err := scanLines(raw, paths, found); err != nil {
			return nil, err
		}
		if rerr == io.EOF {
			break
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

// scanLines marks the paths that the tool calls on these lines name. raw ends at a newline or the file's end; read as
// Python reads text (invalid UTF-8 replaced, a lone \r ending a line too), it can hold more than one line.
func scanLines(raw string, paths []string, found map[string]bool) error {
	text := strings.ToValidUTF8(raw, string(utf8.RuneError))
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		var hits []string
		for _, p := range paths {
			if !found[p] && strings.Contains(line, p) {
				hits = append(hits, p)
			}
		}
		if len(hits) == 0 {
			continue
		}
		v, err := pyjson.Loads(line)
		if err != nil {
			continue
		}
		entry, ok := v.(*pyjson.Object)
		if !ok {
			continue
		}
		calls, err := ToolInputs(entry)
		if err != nil {
			return err
		}
		for _, p := range hits {
			for _, c := range calls {
				if strings.Contains(c, p) {
					found[p] = true
					break
				}
			}
		}
	}
	return nil
}

// StopReason is the block reason for this stop, or "" to let it through; what it lets through for an error it says on
// stderr.
func StopReason(event *pyjson.Object, stderr io.Writer) (string, error) {
	if pyjson.Truthy(event.Get("stop_hook_active")) {
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
			return "", nil, nil // a clone without a store has no records to commit
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
		return "", nil
	}
	if len(paths) == 0 {
		return "", nil
	}
	var mine []string
	readable := false
	if pyjson.Truthy(event.Get("transcript_path")) {
		transcript, err := eventString(event, "transcript_path")
		if err != nil {
			return "", err
		}
		if mine, err = Touched(*transcript, paths); err == nil {
			readable = true
		} else if _, isRead := err.(*os.PathError); !isRead {
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
	_, err = fmt.Fprintln(stdout, pyjson.Dumps([][2]any{{"decision", "block"}, {"reason", reason}}, true))
	return err
}
