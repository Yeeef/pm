package install

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// Codex's workspace-write sandbox keeps .git read-only even inside the workspace and leaves the records store and the
// work store outside a worktree's workspace, so pm cannot commit records or write items until all are writable roots.
// A writable root's own git dir stays read-only unless listed itself, so the store's (.git/worktrees/<name>) is listed
// too, and so is .pm/run, which holds the work store's gate lock (the pm-go page, Open question 12).
const CodexTable = "sandbox_workspace_write"

var (
	codexHeader = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*` + CodexTable + `[ \t]*\][ \t]*(?:#.*)?$`)
	tableStart  = regexp.MustCompile(`(?m)^[ \t]*\[`)
	rootsKey    = regexp.MustCompile(`(?m)^[ \t]*writable_roots[ \t]*=[ \t]*\[`)
)

// home is $HOME, else the user's home directory.
func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// CodexHome is $CODEX_HOME, else ~/.codex.
func CodexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(home(), ".codex")
}

// Resolve is Python's Path.resolve(): absolute, with the symlinks of the part that exists followed.
func Resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs
	}
	return filepath.Join(Resolve(parent), filepath.Base(abs))
}

// CodexRoots is the writable roots the clone needs: its git dir, the records store, the store's git dir, the work
// store and the run directory (its gate lock), each resolved.
func CodexRoots(main string) ([]string, error) {
	records := filepath.Join(main, config.Store)
	gitDir, err := Git(records, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	dir, run := work.Locations(main)
	var out []string
	for _, p := range []string{filepath.Join(main, ".git"), records, gitDir, dir, run} {
		out = append(out, Resolve(p))
	}
	return out, nil
}

// CodexConfig is the Codex config's text ("" when absent), its parsed data, and the writable roots it already lists;
// it refuses what it cannot read.
func CodexConfig(path string) (string, map[string]any, []string, error) {
	t, err := Read(path)
	if err != nil {
		return "", nil, nil, err
	}
	text := ""
	if t != nil {
		text = *t
	}
	data := map[string]any{}
	if _, err := toml.Decode(text, &data); err != nil {
		return "", nil, nil, refuse("cannot parse %s: %s; fix it by hand and run pm init again", path, tomlError(err))
	}
	bad := refuse("%s: %s.writable_roots is not a list of paths; fix it by hand", path, CodexTable)
	table, ok := data[CodexTable]
	if !ok {
		return text, data, nil, nil
	}
	t2, ok := table.(map[string]any)
	if !ok {
		return "", nil, nil, bad
	}
	roots, ok := t2["writable_roots"]
	if !ok {
		return text, data, nil, nil
	}
	list, ok := roots.([]any)
	if !ok {
		return "", nil, nil, bad
	}
	have := make([]string, len(list))
	for i, r := range list {
		if have[i], ok = r.(string); !ok {
			return "", nil, nil, bad
		}
	}
	return text, data, have, nil
}

func tomlError(err error) string {
	if pe, ok := err.(toml.ParseError); ok {
		return pe.Message
	}
	return err.Error()
}

func tomlLoads(text string) (map[string]any, error) {
	data := map[string]any{}
	_, err := toml.Decode(text, &data)
	return data, err
}

func items(roots []string) string {
	q := make([]string, len(roots))
	for i, r := range roots {
		q[i] = pyjson.String(r, false)
	}
	return strings.Join(q, ", ")
}

func anyList(roots []string) []any {
	out := make([]any, len(roots))
	for i, r := range roots {
		out[i] = r
	}
	return out
}

// AddCodexRoots is text with missing added to the writable roots by a minimal edit; every other byte is kept, and an
// edit that would change anything else is refused.
func AddCodexRoots(path, text string, have, missing []string) (string, error) {
	list := items(missing)
	expected, err := tomlLoads(text)
	if err != nil {
		return "", err
	}
	header := codexHeader.FindStringIndex(text)
	var n string
	if _, ok := expected[CodexTable]; !ok {
		sep := "\n\n"
		switch {
		case text == "" || strings.HasSuffix(text, "\n\n"):
			sep = ""
		case strings.HasSuffix(text, "\n"):
			sep = "\n"
		}
		n = text + sep + "[" + CodexTable + "]\nwritable_roots = [" + list + "]\n"
		expected[CodexTable] = map[string]any{"writable_roots": anyList(missing)}
	} else if header == nil {
		return "", refuse("%s sets %s without a [%s] header; add these to its writable_roots by hand: %s", path,
			CodexTable, CodexTable, list)
	} else {
		end := len(text)
		if m := tableStart.FindStringIndex(text[header[1]:]); m != nil {
			end = header[1] + m[0]
		}
		section := text[header[1]:end]
		table, _ := expected[CodexTable].(map[string]any)
		if _, ok := table["writable_roots"]; !ok {
			n = text[:header[1]] + "\nwritable_roots = [" + list + "]" + text[header[1]:]
		} else if key := rootsKey.FindStringIndex(section); key == nil {
			return "", refuse("cannot find writable_roots under [%s] in %s; add these by hand: %s", CodexTable, path, list)
		} else {
			at := header[1] + key[1]
			rest := text[at:]
			comma := ", "
			if strings.HasPrefix(strings.TrimLeftFunc(rest, config.IsPySpace), "]") {
				comma = ""
			}
			n = text[:at] + list + comma + rest
		}
		table["writable_roots"] = anyList(append(append([]string(nil), missing...), have...))
	}
	got, err := tomlLoads(n)
	if err != nil || !reflect.DeepEqual(got, expected) { // the edit must add the roots and change nothing else
		return "", refuse("editing %s would change more than writable_roots; add these by hand: %s", path, list)
	}
	return n, nil
}

// RemoveCodexRoots is path's text without roots, by a minimal edit undoing AddCodexRoots; refused when the edit would
// change anything else.
func RemoveCodexRoots(path string, roots []string) (string, error) {
	text, _, have, err := CodexConfig(path)
	if err != nil {
		return "", err
	}
	var gone, kept []string
	for _, r := range roots {
		if contains(have, r) {
			gone = append(gone, r)
		}
	}
	if len(gone) == 0 {
		return text, nil
	}
	for _, r := range have {
		if !contains(gone, r) {
			kept = append(kept, r)
		}
	}
	list := items(gone)
	header := codexHeader.FindStringIndex(text)
	if header == nil {
		return "", refuse("%s sets %s without a [%s] header; remove these from its writable_roots by hand: %s", path,
			CodexTable, CodexTable, list)
	}
	end := len(text)
	if m := tableStart.FindStringIndex(text[header[1]:]); m != nil {
		end = header[1] + m[0]
	}
	section := text[header[1]:end]
	for _, r := range gone {
		item := pyjson.String(r, false)
		for _, form := range []string{item + ", ", ", " + item, item} {
			if strings.Contains(section, form) {
				section = strings.Replace(section, form, "", 1)
				break
			}
		}
	}
	n := text[:header[1]] + section + text[end:]
	expected, err := tomlLoads(text)
	if err != nil {
		return "", err
	}
	table := expected[CodexTable].(map[string]any)
	table["writable_roots"] = anyList(kept)
	if len(kept) == 0 { // what AddCodexRoots made from nothing goes too: the key, then the table
		block := "[" + CodexTable + "]\nwritable_roots = []\n"
		onlyRoots := len(table) == 1
		if onlyRoots && strings.HasSuffix(n, block) {
			n = strings.TrimSuffix(n, block)
			if strings.HasSuffix(n, "\n\n") {
				n = n[:len(n)-1]
			}
			delete(expected, CodexTable)
		} else if strings.HasPrefix(section, "\nwritable_roots = []") {
			n = text[:header[1]] + section[len("\nwritable_roots = []"):] + text[end:]
			delete(table, "writable_roots")
		}
	}
	got, err := tomlLoads(n)
	if err != nil { // a writable_roots spread over lines, say: its commas stay behind
		return "", refuse("editing %s would leave it invalid TOML (%s); remove these from its writable_roots by hand: %s",
			path, tomlError(err), list)
	}
	if !reflect.DeepEqual(got, expected) {
		return "", refuse("editing %s would change more than writable_roots; remove these by hand: %s", path, list)
	}
	return n, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// SetupCodex adds the roots pm needs to the user's Codex config, keeping the rest of the file byte for byte; what it
// did, "" when nothing was missing.
func SetupCodex(main string) (string, error) {
	h := CodexHome()
	if st, err := os.Stat(h); err != nil || !st.IsDir() {
		return "Codex: no " + h + ", so Codex is not used here; left its sandbox config alone", nil
	}
	path := filepath.Join(h, "config.toml")
	text, _, have, err := CodexConfig(path)
	if err != nil {
		return "", err
	}
	roots, err := CodexRoots(main)
	if err != nil {
		return "", err
	}
	var missing []string
	for _, r := range roots {
		if !contains(have, r) {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return "", nil
	}
	n, err := AddCodexRoots(path, text, have, missing)
	if err != nil {
		return "", err
	}
	if err := store.WriteAtomic(path, n); err != nil {
		return "", err
	}
	return "added to " + CodexTable + ".writable_roots in " + path + ", so Codex can commit records: " +
		strings.Join(missing, ", "), nil
}
