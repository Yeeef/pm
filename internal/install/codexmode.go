package install

import (
	"reflect"
	"regexp"
	"strings"
)

// CodexModeRel is the repo's Codex config, where pm sets Codex's auto mode: the workspace-write sandbox, approval on
// request, and the automatic reviewer deciding each request (the config form of codex --approve-for-me). Codex reads
// it only in a repo it trusts, and fails to load a config whose value it does not know (the pm-codex sprint 5's
// finding); the user's own ~/.codex/config.toml is left alone (an owner decision).
const CodexModeRel = ".codex/config.toml"

// CodexMode is the keys pm sets there, in the order it writes them.
var CodexMode = [][2]string{
	{"approval_policy", "on-request"},
	{"approvals_reviewer", "auto_review"},
	{"sandbox_mode", "workspace-write"},
}

// codexModeComment heads the keys pm writes.
const codexModeComment = "# Codex's auto mode, which pm sets: a value you change here stays yours\n"

// A key pm writes is pm's while it holds pm's value in pm's block, its comment and the lines directly under it: a key
// set to another value is the user's override, which pm doctor does not name, pm upgrade keeps, and pm uninstall
// leaves, as pm leaves the user's own writable_roots and hook entries; a key the user set outside the block, to pm's
// value too, counts as set and stays the user's. Only a missing key is pm's part missing.

func codexModeLine(key, value string) string { return key + " = " + tomlString(value) + "\n" }

func loadCodexMode(text *string) (map[string]any, error) {
	if text == nil {
		return map[string]any{}, nil
	}
	data, err := tomlLoads(*text)
	if err != nil {
		return nil, refuse("cannot parse %s: %s; fix it by hand and run pm init again", CodexModeRel, tomlError(err))
	}
	return data, nil
}

func codexModePresent(text *string) (bool, error) {
	data, err := loadCodexMode(text)
	if err != nil {
		return false, err
	}
	for _, kv := range CodexMode {
		if _, ok := data[kv[0]]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// codexModePart is the keys set, each with its value, whoever set it.
func codexModePart(text *string) (string, bool, error) {
	data, err := loadCodexMode(text)
	if err != nil {
		return "", false, err
	}
	mine := map[string]any{}
	for _, kv := range CodexMode {
		if v, ok := data[kv[0]]; ok {
			mine[kv[0]] = v
		}
	}
	return canonical(mine), len(mine) > 0, nil
}

// codexModeBlockAt is pm's block in text: pm's comment, at the start of a line before the first table, and the lines
// directly under it that set one of pm's keys, whatever their value; ok false when text has no such comment.
func codexModeBlockAt(text string) (start, end int, ok bool) {
	top := len(text) // a top-level key comes before the first table
	if m := tableStart.FindStringIndex(text); m != nil {
		top = m[0]
	}
	m := codexModeCommentLine.FindStringIndex(text[:top])
	if m == nil {
		return 0, 0, false
	}
	start, end = m[0], m[1]
	for end < len(text) {
		line := codexModeNextLine(text[end:])
		if !codexModeKey.MatchString(line) {
			break
		}
		end += len(line)
	}
	return start, end, true
}

var codexModeCommentLine = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(codexModeComment))

var codexModeKey = regexp.MustCompile(`^[ \t]*(?:` + strings.Join(func() []string {
	var keys []string
	for _, kv := range CodexMode {
		keys = append(keys, regexp.QuoteMeta(kv[0]))
	}
	return keys
}(), "|") + `)[ \t]*=`)

// codexModeNextLine is text's first line with its newline, if it has one.
func codexModeNextLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i+1]
	}
	return text
}

// codexModeApply adds each missing key, by a minimal edit, in pm's block: under pm's comment where the file has it,
// else as a new block, the whole file for no file, at the end of a file with no table after one newline, else at its
// start, where a key is top-level, before one blank line. A key already set, wherever and to whatever value, stays as it
// is. An edit that would change anything else is refused.
func codexModeApply(text *string) (string, error) {
	data, err := loadCodexMode(text)
	if err != nil {
		return "", err
	}
	lines := ""
	var missing []string
	for _, kv := range CodexMode {
		if _, ok := data[kv[0]]; !ok {
			lines += codexModeLine(kv[0], kv[1])
			missing = append(missing, kv[0])
			data[kv[0]] = kv[1]
		}
	}
	if text == nil {
		return codexModeComment + lines, nil
	}
	t := *text
	if lines == "" {
		return t, nil
	}
	var n string
	if _, end, ok := codexModeBlockAt(t); ok {
		if t[end-1] != '\n' {
			lines = "\n" + strings.TrimSuffix(lines, "\n")
		}
		n = t[:end] + lines + t[end:]
	} else if tableStart.MatchString(t) {
		n = codexModeComment + lines + "\n" + t
	} else {
		n = t + "\n" + codexModeComment + lines
	}
	if got, err := tomlLoads(n); err != nil || !reflect.DeepEqual(got, data) {
		return "", refuse("editing %s would change more than Codex's auto-mode keys; add these by hand: %s", CodexModeRel,
			strings.Join(missing, ", "))
	}
	return n, nil
}

// codexModeRemove is the file without pm's block: pm's comment and each line under it that sets a key to pm's value as
// pm writes it; nil when the file is that block alone, as pm init writes a new file. A key elsewhere, whatever its value,
// and a key in the block holding another value, are the user's and stay. Where the whole block goes, so does the
// newline codexModeApply put before or after it, so pm uninstall gives back the file pm init found. An edit that would
// change anything else is refused.
func codexModeRemove(text string) (*string, error) {
	data, err := loadCodexMode(&text)
	if err != nil {
		return nil, err
	}
	start, end, ok := codexModeBlockAt(text)
	if !ok {
		return &text, nil
	}
	kept := ""
	var mine []string
	for i := start + len(codexModeComment); i < end; {
		line := codexModeNextLine(text[i:end])
		i += len(line)
		pm := false
		for _, kv := range CodexMode {
			if l := codexModeLine(kv[0], kv[1]); line == l || (i == len(text) && line+"\n" == l) {
				pm = true
				mine = append(mine, kv[0])
				delete(data, kv[0])
			}
		}
		if !pm {
			kept += line
		}
	}
	n := text[:start] + kept + text[end:]
	gone := false
	if kept == "" {
		switch {
		case start == 0 && end == len(text):
			gone = true
		case end == len(text) && start > 0:
			n = text[:start-1] // the newline pm put before the block
		case start == 0 && strings.HasPrefix(text[end:], "\n") && tableStart.MatchString(text[end:]):
			n = text[end+1:] // the blank line pm put after the block
		}
	}
	if got, err := tomlLoads(n); err != nil || !reflect.DeepEqual(got, data) {
		return nil, refuse("editing %s would change more than Codex's auto-mode block; remove its comment and these by "+
			"hand: %s", CodexModeRel, strings.Join(mine, ", "))
	}
	if gone {
		return nil, nil
	}
	return &n, nil
}

func codexModePiece() Piece {
	return Piece{CodexModeRel, codexModePresent, codexModeApply, codexModePart, codexModeRemove, 0o644}
}
