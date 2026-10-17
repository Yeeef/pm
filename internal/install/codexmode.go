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

// A key pm writes is pm's while it holds pm's value: a key set to another value is the user's override, which pm
// doctor does not name, pm upgrade keeps, and pm uninstall leaves, as pm leaves the user's own writable_roots and hook
// entries. Only a missing key is pm's part missing.

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

// codexModeApply adds each missing key, by a minimal edit: at the end of a file with no table, else at its start, where
// a key is top-level; a key already set keeps its value. An edit that would change anything else is refused.
func codexModeApply(text *string) (string, error) {
	data, err := loadCodexMode(text)
	if err != nil {
		return "", err
	}
	t := ""
	if text != nil {
		t = *text
	}
	block := ""
	var missing []string
	for _, kv := range CodexMode {
		if _, ok := data[kv[0]]; !ok {
			block += codexModeLine(kv[0], kv[1])
			missing = append(missing, kv[0])
			data[kv[0]] = kv[1]
		}
	}
	if block == "" {
		return t, nil
	}
	if !strings.Contains(t, codexModeComment) {
		block = codexModeComment + block
	}
	var n string
	switch {
	case tableStart.MatchString(t):
		n = block + "\n" + t
	case t == "":
		n = block
	case strings.HasSuffix(t, "\n"):
		n = t + "\n" + block
	default:
		n = t + "\n\n" + block
	}
	if got, err := tomlLoads(n); err != nil || !reflect.DeepEqual(got, data) {
		return "", refuse("editing %s would change more than Codex's auto-mode keys; add these by hand: %s", CodexModeRel,
			strings.Join(missing, ", "))
	}
	return n, nil
}

// codexModeRemove is the file without each key that holds pm's value, and without pm's comment: the block pm added
// goes whole where it is still whole, else line by line; nil when nothing else is left. A key holding another value is
// the user's and stays. An edit that would change anything else is refused.
func codexModeRemove(text string) (*string, error) {
	data, err := loadCodexMode(&text)
	if err != nil {
		return nil, err
	}
	block := codexModeComment
	var mine []string
	for _, kv := range CodexMode {
		if v, ok := data[kv[0]]; ok && v == any(kv[1]) {
			block += codexModeLine(kv[0], kv[1])
			mine = append(mine, kv[0])
			delete(data, kv[0])
		}
	}
	if len(mine) == 0 {
		return &text, nil
	}
	var n string
	switch {
	case strings.HasPrefix(text, block+"\n") && tableStart.MatchString(text):
		n = strings.TrimPrefix(text, block+"\n")
	case strings.HasSuffix(text, block):
		n = strings.TrimSuffix(text, block)
		if strings.HasSuffix(n, "\n\n") {
			n = n[:len(n)-1]
		}
	default:
		n = strings.Replace(text, codexModeComment, "", 1)
		for _, key := range mine {
			top := len(n) // a top-level key comes before the first table
			if m := tableStart.FindStringIndex(n); m != nil {
				top = m[0]
			}
			re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(key) + `[ \t]*=[^\n]*(?:\n|\z)`)
			if m := re.FindStringIndex(n[:top]); m != nil {
				n = n[:m[0]] + n[m[1]:]
			}
		}
	}
	if got, err := tomlLoads(n); err != nil || !reflect.DeepEqual(got, data) {
		return nil, refuse("editing %s would change more than Codex's auto-mode keys; remove these by hand: %s",
			CodexModeRel, strings.Join(mine, ", "))
	}
	if n == "" {
		return nil, nil
	}
	return &n, nil
}

func codexModePiece() Piece {
	return Piece{CodexModeRel, codexModePresent, codexModeApply, codexModePart, codexModeRemove, 0o644}
}
