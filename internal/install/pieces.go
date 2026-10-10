// Package install is what pm init, pm doctor, pm upgrade and pm uninstall manage: the pieces pm puts in a repo's
// tracked files, the clone's and the worktree's setup (the work store, the records store and its link, the git hooks
// path, the excludes, the Codex and Claude Code settings), pm's own binary on the machine, and the bootstrap of a
// brand-new repo's records branch.
//
// A piece is either a whole file pm owns (.pm/config.toml, .pm/README.md, .pm/.gitignore) or pm's part of a shared
// file: its hook entries in .claude/settings.json and .codex/hooks.json (a hook is pm's when its command starts with
// "pm prime" or "pm hook "), its marked section in its own git hook file .pm/hooks/post-checkout, and its marked block
// in .gitignore. Each piece has Present, whether pm's part is there,
// Apply, the file with pm's part as this version writes it and every other byte kept, Part, pm's part alone (what pm
// doctor compares with Part(Apply(text))), and Remove, the file without pm's part (nil: nothing else is left, so the
// file goes). pm init applies a piece only when it is not present, so a second run changes nothing; pm upgrade applies
// every piece; pm uninstall removes every piece. Each writes the bytes Python pm 0.1.x writes, but for the hook files'
// place: Python pm 0.1.x puts its sections in Beads' .beads/hooks.
//
// Go pm runs no bd, so pm init and pm upgrade also take Beads' own pieces out of the tracked files (the work-store
// page, Cut-over): bd's hook entries in the runtimes' settings and the Beads block in CLAUDE.md and AGENTS.md
// (leftovers below); pm doctor names each one left. pm uninstall leaves them, as they are not pm's.
//
// A retired piece (Retired) is one an earlier pm wrote and this one does not: pm doctor names each one left, and pm
// init, pm upgrade and pm uninstall remove it.
package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/hooks"
	"github.com/Yeeef/pm/internal/pyjson"
)

// Error is a piece that cannot be written without changing what is not pm's, or a setup step refused; nothing was
// written for it. Python's InstallError and the Refuse of the setup steps.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func refuse(format string, a ...any) error { return &Error{fmt.Sprintf(format, a...)} }

// Settings is what .pm/config.toml holds; the pieces that name the remote or the main branch take them from here.
type Settings struct {
	Remote     string
	MainBranch string
	Port       int
	SiteURL    string
}

// Piece is one managed piece. A text is nil when the file is absent.
type Piece struct {
	Rel     string // the path under the worktree root
	Present func(text *string) (bool, error)
	Apply   func(text *string) (string, error)
	// Part is pm's part alone in a comparable form, and whether it is there (Python's truthiness of the part).
	Part   func(text *string) (part string, there bool, err error)
	Remove func(text string) (*string, error)
	Mode   os.FileMode // for a file pm creates
}

func ptr(s string) *string { return &s }

// ---------------------------------------------------------------- whole files pm owns

// ConfigText is .pm/config.toml as pm init writes it.
func ConfigText(s Settings) string {
	site := ""
	if s.SiteURL != "" {
		site = "site_url = " + pyjson.String(s.SiteURL, true) + "\n"
	}
	return "version = " + pyjson.String(buildinfo.Version, true) + "   # the pm version every session must run\n" +
		"remote = " + pyjson.String(s.Remote, true) + "\n" +
		"main_branch = " + pyjson.String(s.MainBranch, true) + "\n" +
		fmt.Sprintf("port = %d         # the site port; the PORT environment variable overrides it for one run\n", s.Port) +
		site
}

// README is .pm/README.md.
const README = "# pm\n" +
	"\n" +
	"This repo uses pm: the work store holds the work, Markdown records hold the context, and pm writes both and serves\n" +
	"them as a site.\n" +
	"\n" +
	"- Install the pinned version (`version` in `config.toml`):\n" +
	"  `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<version>/install.sh | sh`, then run `pm init`\n" +
	"  in each clone.\n" +
	"- The work store is a Dolt database each clone keeps at `.pm/store/work`; pm syncs it through the remote's\n" +
	"  `refs/pm/work`.\n" +
	"- Records live on the `records` branch. Each clone checks it out once at `.pm/store/records`, and each worktree reads\n" +
	"  it through `records/`, a git-ignored link; no other branch tracks `records/`.\n" +
	"- Agents get pm's rules and the project's state from hooks (`pm prime`, `pm hook <name>`); run `pm --help` for the\n" +
	"  commands.\n" +
	"- `store/` and `run/` here are per clone and git-ignored; `config.toml`, this file and `.gitignore` are tracked.\n"

// PMGitignore is .pm/.gitignore.
const PMGitignore = "store/\nrun/\n"

func whole(rel, content string) Piece {
	return Piece{
		Rel:     rel,
		Present: func(text *string) (bool, error) { return text != nil, nil },
		Apply:   func(*string) (string, error) { return content, nil },
		Part: func(text *string) (string, bool, error) {
			if text == nil {
				return "", false, nil
			}
			return *text, *text != "", nil
		},
		Remove: func(string) (*string, error) { return nil, nil },
		Mode:   0o644,
	}
}

// ---------------------------------------------------------------- hook entries in the runtimes' settings

// isPMHook is whether a hook entry is pm's: its command starts with "pm prime" or "pm hook ".
func isPMHook(h any) bool {
	o, ok := h.(*pyjson.Object)
	if !ok {
		return false
	}
	cmd, ok := o.Get("command").(string)
	return ok && (strings.HasPrefix(cmd, "pm prime") || strings.HasPrefix(cmd, "pm hook "))
}

func obj(pairs ...any) *pyjson.Object {
	o := pyjson.NewObject()
	for i := 0; i < len(pairs); i += 2 {
		o.Set(pairs[i].(string), pairs[i+1])
	}
	return o
}

// entries is pm's matcher group per event, in the order pm writes the events.
type entries struct {
	events []string
	groups map[string]func() *pyjson.Object // a fresh copy each call
}

func rulesCount() int { return len(hooks.Starts) }

// ClaudeHooks is pm's entries in .claude/settings.json, by event: the matcher group pm adds, holding only pm's hooks.
func claudeHooks() entries {
	h := func(command string, timeout int) any {
		return obj("command", command, "type", "command", "timeout", timeout)
	}
	rules := func(timeout int) []any {
		var out []any
		for i := 1; i <= rulesCount(); i++ {
			out = append(out, h(fmt.Sprintf("pm prime --rules %d --hook-json", i), timeout))
		}
		return out
	}
	return entries{
		events: []string{"SessionStart", "SubagentStart", "Stop"},
		groups: map[string]func() *pyjson.Object{
			"SessionStart": func() *pyjson.Object {
				return obj("hooks", append(rules(30), h("pm prime --state --hook-json", 30)), "matcher", "")
			},
			"SubagentStart": func() *pyjson.Object {
				return obj("hooks", append(rules(15), h("pm prime --subagent --hook-json", 15)), "matcher", "")
			},
			"Stop": func() *pyjson.Object {
				return obj("hooks", []any{h("pm hook owner-request || exit 1", 30), h("pm hook stop || exit 1", 10)})
			},
		},
	}
}

// codexHooks is pm's entries in .codex/hooks.json. Codex's SessionStart has no compact event.
func codexHooks() entries {
	h := func(command, status string, timeout int) any {
		return obj("command", command, "statusMessage", status, "type", "command", "timeout", timeout)
	}
	rules := func(timeout int) []any {
		n := rulesCount()
		var out []any
		for i := 1; i <= n; i++ {
			out = append(out, h(fmt.Sprintf("pm prime --rules %d --hook-json", i), fmt.Sprintf("Loading pm rules (%d of %d)", i, n), timeout))
		}
		return out
	}
	return entries{
		events: []string{"SessionStart", "SubagentStart", "Stop"},
		groups: map[string]func() *pyjson.Object{
			"SessionStart": func() *pyjson.Object {
				return obj("hooks", append(rules(30), h("pm prime --state --hook-json", "Loading pm init, pm where and pm show", 30)),
					"matcher", "startup|resume|clear")
			},
			"SubagentStart": func() *pyjson.Object {
				return obj("hooks", append(rules(15), h("pm prime --subagent --hook-json", "Loading pm's git rule for agents", 15)))
			},
			"Stop": func() *pyjson.Object {
				return obj("hooks", []any{h("pm hook owner-request || exit 1", "Checking for owner requests asked only in chat", 30),
					h("pm hook stop || exit 1", "Checking for uncommitted records", 10)})
			},
		},
	}
}

// DumpJSON is how pm writes a JSON settings file: two-space indent, non-ASCII kept, a final newline.
func DumpJSON(data any) string { return pyjson.DumpsIndent(data, 2, false) + "\n" }

// settingsFile is a settings file's data, its `hooks` checked to map events to lists of matcher groups.
type settingsFile struct {
	data   *pyjson.Object
	events *pyjson.Object // nil when the file has no hooks
}

// loadHooks is the settings file's data, refused unless it is a JSON object whose hooks maps events to lists of matcher
// groups; with layout, also unless it is laid out as pm writes JSON (two-space indent), so rewriting it keeps every
// byte that is not pm's.
func loadHooks(rel string, text *string, layout bool) (settingsFile, error) {
	if text == nil {
		return settingsFile{data: pyjson.NewObject()}, nil
	}
	v, err := pyjson.Loads(*text)
	if err != nil {
		return settingsFile{}, refuse("%s is not valid JSON (%s); fix it by hand and run pm init again", rel, err)
	}
	bad := refuse("%s: hooks is not a map of events to lists of matcher groups; fix it by hand", rel)
	data, ok := v.(*pyjson.Object)
	if !ok {
		return settingsFile{}, bad
	}
	var events *pyjson.Object
	if data.Has("hooks") {
		if events, ok = data.Get("hooks").(*pyjson.Object); !ok {
			return settingsFile{}, bad
		}
		for _, e := range events.Keys {
			gs, ok := events.Get(e).([]any)
			if !ok {
				return settingsFile{}, bad
			}
			for _, g := range gs {
				g, ok := g.(*pyjson.Object)
				if !ok {
					return settingsFile{}, bad
				}
				if g.Has("hooks") {
					if _, ok := g.Get("hooks").([]any); !ok {
						return settingsFile{}, bad
					}
				}
			}
		}
	}
	if layout && *text != DumpJSON(data) {
		return settingsFile{}, refuse("%s is not laid out as pm writes JSON (2-space indent, one key per line), so adding "+
			"pm's hooks would change other lines; reformat it (python3 -m json.tool --indent 2) and run pm init again", rel)
	}
	return settingsFile{data: data, events: events}, nil
}

func groupHooks(g any) []any {
	o := g.(*pyjson.Object)
	hs, _ := o.Get("hooks").([]any)
	return hs
}

func hooksPresent(rel string, want entries) func(*string) (bool, error) {
	return func(text *string) (bool, error) {
		f, err := loadHooks(rel, text, false)
		if err != nil {
			return false, err
		}
		for _, event := range want.events {
			have := map[string]bool{}
			if f.events != nil {
				gs, _ := f.events.Get(event).([]any)
				for _, g := range gs {
					for _, h := range groupHooks(g) {
						if o, ok := h.(*pyjson.Object); ok {
							if c, ok := o.Get("command").(string); ok {
								have[c] = true
							}
						}
					}
				}
			}
			for _, h := range groupHooks(want.groups[event]()) {
				if !have[h.(*pyjson.Object).Get("command").(string)] {
					return false, nil
				}
			}
		}
		return true, nil
	}
}

// dropPMHooks takes pm's hooks out of each matcher group of gs; a group left without hooks goes.
func dropPMHooks(gs []any) []any { return dropHooks(gs, isPMHook) }

// dropHooks takes the hooks is picks out of each matcher group of gs; a group left without hooks goes.
func dropHooks(gs []any, is func(any) bool) []any {
	kept := []any{}
	for _, g := range gs {
		o := g.(*pyjson.Object)
		hs := groupHooks(g)
		var others []any
		mine := false
		for _, h := range hs {
			if is(h) {
				mine = true
			} else {
				others = append(others, h)
			}
		}
		if mine {
			if len(others) == 0 {
				continue
			}
			o.Set("hooks", others)
		}
		kept = append(kept, g)
	}
	return kept
}

// hooksApply is the settings with pm's hooks as this version writes them: pm's old hooks removed (a group left empty
// goes too), then pm's group appended to each event. Everything else keeps its place.
func hooksApply(rel string, want entries) func(*string) (string, error) {
	return func(text *string) (string, error) {
		f, err := loadHooks(rel, text, true)
		if err != nil {
			return "", err
		}
		events := f.events
		if events == nil {
			events = pyjson.NewObject()
			f.data.Set("hooks", events)
		}
		for _, e := range events.Keys {
			events.Set(e, dropPMHooks(events.Get(e).([]any)))
		}
		for _, e := range want.events {
			gs, _ := events.Get(e).([]any)
			events.Set(e, append(gs, want.groups[e]()))
		}
		return DumpJSON(f.data), nil
	}
}

// hooksPart is pm's hooks by event, in order, each as written, with object keys sorted so that it compares as the
// Python dicts do.
func hooksPart(rel string) func(*string) (string, bool, error) {
	return func(text *string) (string, bool, error) {
		if text == nil {
			return "", false, nil
		}
		f, err := loadHooks(rel, text, false)
		if err != nil {
			return "", false, err
		}
		mine := pyjson.NewObject()
		if f.events != nil {
			for _, e := range f.events.Keys {
				var hs []any
				for _, g := range f.events.Get(e).([]any) {
					for _, h := range groupHooks(g) {
						if isPMHook(h) {
							hs = append(hs, h)
						}
					}
				}
				if len(hs) > 0 {
					mine.Set(e, hs)
				}
			}
		}
		return canonical(mine), len(mine.Keys) > 0, nil
	}
}

// canonical is v as JSON with every object's keys sorted: equal for values Python compares equal.
func canonical(v any) string {
	switch t := v.(type) {
	case *pyjson.Object:
		keys := append([]string(nil), t.Keys...)
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(pyjson.String(k, true) + ":" + canonical(t.Values[k]))
		}
		b.WriteByte('}')
		return b.String()
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = canonical(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return pyjson.Dumps(v, true)
}

// hooksRemove is the settings without pm's hooks: a group or event pm's removal empties goes too, and so does hooks and
// then the file when nothing else is left.
func hooksRemove(rel string) func(string) (*string, error) {
	return func(text string) (*string, error) {
		f, err := loadHooks(rel, &text, true)
		if err != nil {
			return nil, err
		}
		events := f.events
		if events == nil || len(events.Keys) == 0 {
			return &text, nil
		}
		for _, e := range append([]string(nil), events.Keys...) {
			gs := events.Get(e).([]any)
			kept := dropPMHooks(gs)
			if len(gs) > 0 && len(kept) == 0 {
				events.Delete(e)
			} else {
				events.Set(e, kept)
			}
		}
		if len(events.Keys) == 0 {
			f.data.Delete("hooks")
		}
		if len(f.data.Keys) == 0 {
			return nil, nil
		}
		return ptr(DumpJSON(f.data)), nil
	}
}

func hooksPiece(rel string, want entries) Piece {
	return Piece{rel, hooksPresent(rel, want), hooksApply(rel, want), hooksPart(rel), hooksRemove(rel), 0o644}
}

// ---------------------------------------------------------------- marked sections

// GitHooks are the git hooks pm runs: its own files in .pm/hooks (HooksRel), and the Beads hook files Python pm put its
// section in.
var GitHooks = []string{"post-checkout"}

// HooksRel is pm's git hook directory under a worktree, which core.hooksPath names (the work-store page, Constraints
// resolved, row 8: pm owns its hook directory).
const HooksRel = ".pm/hooks"

const (
	sectionEnd = "# --- END PM ---"
	shebang    = "#!/usr/bin/env sh\n"
)

var (
	section  = regexp.MustCompile(`(?ms)^# --- BEGIN PM v[^\n]* ---\n.*?^# --- END PM ---\n?`)
	beginAny = regexp.MustCompile(`(?m)^# --- BEGIN PM v[^\n]* ---$`)
	beadsEnd = regexp.MustCompile(`(?m)^# --- END BEADS INTEGRATION[^\n]*(\n|\z)`)
)

// GitHookSection is pm's section in a git hook file: one line, so the logic ships in pm. `|| exit $?` keeps a
// failure (pm missing, say) from being lost when lines follow it.
func GitHookSection(name string) string {
	return "# --- BEGIN PM v" + buildinfo.Version + " ---\npm hook git-" + name + " \"$@\" || exit $?\n" + sectionEnd + "\n"
}

func stripSection(rel, text string, pattern, begin *regexp.Regexp) (string, error) {
	out := pattern.ReplaceAllLiteralString(text, "")
	if begin.MatchString(out) {
		return "", refuse("%s has a pm begin marker without its end marker (%s); fix it by hand", rel, sectionEnd)
	}
	return out, nil
}

// gitHookApply puts pm's section right after Beads' end marker (or at the end when Beads has none); a new file gets
// a shebang, and so holds pm's section alone.
func gitHookApply(rel, name string) func(*string) (string, error) {
	return func(text *string) (string, error) {
		if text == nil {
			return shebang + GitHookSection(name), nil
		}
		t, err := stripSection(rel, *text, section, beginAny)
		if err != nil {
			return "", err
		}
		at := len(t)
		if m := beadsEnd.FindStringIndex(t); m != nil {
			at = m[1]
		}
		head := t[:at]
		if head != "" && !strings.HasSuffix(head, "\n") {
			head += "\n"
		}
		return head + GitHookSection(name) + t[at:], nil
	}
}

const (
	gitignoreBegin = "# --- BEGIN PM ---"
	gitignoreEnd   = "# --- END PM ---"
	// GitignoreBlock is pm's block in .gitignore.
	GitignoreBlock = gitignoreBegin + "\n" +
		"# each worktree's records/ is a link to the clone's records store\n" +
		"/records\n" +
		"# per-machine Claude Code settings: pm adds the store's absolute path to them\n" +
		"/.claude/settings.local.json\n" +
		gitignoreEnd + "\n"
)

var (
	gitignoreSection = regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(gitignoreBegin) + `\n.*?^` + regexp.QuoteMeta(gitignoreEnd) + `\n?`)
	gitignoreBeginRe = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(gitignoreBegin) + `$`)
)

func gitignoreApply(text *string) (string, error) {
	t := ""
	if text != nil {
		t = *text
	}
	t, err := stripSection(".gitignore", t, gitignoreSection, gitignoreBeginRe)
	if err != nil {
		return "", err
	}
	sep := ""
	if t != "" && !strings.HasSuffix(t, "\n") {
		sep = "\n"
	}
	return t + sep + GitignoreBlock, nil
}

func marked(begin *regexp.Regexp) func(*string) (bool, error) {
	return func(text *string) (bool, error) { return text != nil && begin.MatchString(*text), nil }
}

// sectionPart is pm's marked sections in the file, as written.
func sectionPart(pattern *regexp.Regexp) func(*string) (string, bool, error) {
	return func(text *string) (string, bool, error) {
		if text == nil {
			return "", false, nil
		}
		found := pattern.FindAllString(*text, -1)
		parts := make([]any, len(found))
		for i, f := range found {
			parts[i] = f
		}
		return canonical(parts), len(found) > 0, nil
	}
}

// sectionRemove is the file without pm's section; nil when only what pm writes into a new file (bare) is left.
func sectionRemove(rel string, pattern, begin *regexp.Regexp, bare ...string) func(string) (*string, error) {
	return func(text string) (*string, error) {
		out, err := stripSection(rel, text, pattern, begin)
		if err != nil {
			return nil, err
		}
		for _, b := range bare {
			if out == b {
				return nil, nil
			}
		}
		return &out, nil
	}
}

// ---------------------------------------------------------------- Beads' pieces, which pm removes

// leftover is a piece of Beads' in a tracked file, which pm removes: find names what of it the text holds ("" when
// none), clean is the text without it, every other byte kept.
type leftover struct {
	find  func(text string) (string, error)
	clean func(text string) (string, error)
}

// leftoverRels is each file that may hold one, in the order pm names them. A symlinked CLAUDE.md or AGENTS.md is
// skipped: its target is the other one, or not pm's to edit.
var leftoverRels = []string{".claude/settings.json", ".codex/hooks.json", "CLAUDE.md", "AGENTS.md"}

func leftoverOf(rel string) (leftover, bool) {
	switch rel {
	case ".claude/settings.json", ".codex/hooks.json":
		return leftover{beadsHooksFind(rel), beadsHooksClean(rel)}, true
	case "CLAUDE.md", "AGENTS.md":
		return leftover{beadsBlockFind(rel), beadsBlockClean(rel)}, true
	}
	return leftover{}, false
}

// isBeadsHook is whether a hook entry is Beads': its command starts with "bd " (bd prime --hook-json in Claude Code's
// settings, bd codex-hook <event> in Codex's), so pm prime is the only context (the work-store page, Constraints
// resolved, row 7).
func isBeadsHook(h any) bool {
	o, ok := h.(*pyjson.Object)
	if !ok {
		return false
	}
	cmd, ok := o.Get("command").(string)
	return ok && strings.HasPrefix(cmd, "bd ")
}

func beadsHooksFind(rel string) func(string) (string, error) {
	return func(text string) (string, error) {
		f, err := loadHooks(rel, &text, false)
		if err != nil || f.events == nil {
			return "", err
		}
		var cmds []string
		for _, e := range f.events.Keys {
			for _, g := range f.events.Get(e).([]any) {
				for _, h := range groupHooks(g) {
					if !isBeadsHook(h) {
						continue
					}
					if c := h.(*pyjson.Object).Get("command").(string); !contains(cmds, c) {
						cmds = append(cmds, c)
					}
				}
			}
		}
		if len(cmds) == 0 {
			return "", nil
		}
		return "Beads' hook entries (" + strings.Join(cmds, ", ") + ")", nil
	}
}

// beadsHooksClean is the settings without Beads' hooks: a group or event their removal empties goes too, and so does
// hooks when nothing is left in it.
func beadsHooksClean(rel string) func(string) (string, error) {
	return func(text string) (string, error) {
		if what, err := beadsHooksFind(rel)(text); err != nil || what == "" {
			return text, err
		}
		f, err := loadHooks(rel, &text, true)
		if err != nil {
			return "", err
		}
		for _, e := range append([]string(nil), f.events.Keys...) {
			gs := f.events.Get(e).([]any)
			if kept := dropHooks(gs, isBeadsHook); len(gs) > 0 && len(kept) == 0 {
				f.events.Delete(e)
			} else {
				f.events.Set(e, kept)
			}
		}
		if len(f.events.Keys) == 0 {
			f.data.Delete("hooks")
		}
		return DumpJSON(f.data), nil
	}
}

var (
	beadsBlock      = regexp.MustCompile(`(?ms)^<!-- BEGIN BEADS INTEGRATION[^\n]*\n.*?^<!-- END BEADS INTEGRATION[^\n]*(?:\n|\z)`)
	beadsBlockBegin = regexp.MustCompile(`(?m)^<!-- BEGIN BEADS INTEGRATION`)
)

func beadsBlockFind(rel string) func(string) (string, error) {
	return func(text string) (string, error) {
		if beadsBlock.MatchString(text) {
			return "the Beads block (<!-- BEGIN BEADS INTEGRATION … -->)", nil
		}
		if beadsBlockBegin.MatchString(text) {
			return "", refuse("%s has a Beads begin marker without its end marker (<!-- END BEADS INTEGRATION -->); fix "+
				"it by hand", rel)
		}
		return "", nil
	}
}

// beadsBlockClean is the text without each Beads block and the blank line that set it apart.
func beadsBlockClean(rel string) func(string) (string, error) {
	return func(text string) (string, error) {
		if _, err := beadsBlockFind(rel)(text); err != nil {
			return "", err
		}
		for {
			m := beadsBlock.FindStringIndex(text)
			if m == nil {
				return text, nil
			}
			before, after := text[:m[0]], text[m[1]:]
			if (before == "" || strings.HasSuffix(before, "\n\n")) && (after == "" || strings.HasPrefix(after, "\n")) {
				if after != "" {
					after = after[1:]
				} else if before != "" {
					before = before[:len(before)-1]
				}
			}
			text = before + after
		}
	}
}

// ---------------------------------------------------------------- the pieces

// Pieces is every managed piece, in the order pm writes and names them.
func Pieces(s Settings) []Piece {
	out := []Piece{
		whole(".pm/config.toml", ConfigText(s)),
		whole(".pm/README.md", README),
		whole(".pm/.gitignore", PMGitignore),
		hooksPiece(".claude/settings.json", claudeHooks()),
		hooksPiece(".codex/hooks.json", codexHooks()),
	}
	for _, name := range GitHooks {
		out = append(out, gitHookPiece(name))
	}
	return append(out,
		Piece{".gitignore", marked(gitignoreBeginRe), gitignoreApply, sectionPart(gitignoreSection),
			sectionRemove(".gitignore", gitignoreSection, gitignoreBeginRe, ""), 0o644},
	)
}

func gitHookPiece(name string) Piece {
	rel := HooksRel + "/" + name
	return Piece{rel, marked(beginAny), gitHookApply(rel, name), sectionPart(section),
		sectionRemove(rel, section, beginAny, "", shebang), 0o755}
}

// Retired is each piece an earlier pm wrote and this one does not, all retired with the main branch's records/ copy:
// the workflow that copied the records branch into it, the one that failed a pull request editing it, and pm's
// pre-commit section that refused a code-branch commit editing it. Its Apply is never used.
var Retired = []Piece{
	whole(".github/workflows/pm-records-copy.yml", ""),
	whole(".github/workflows/pm-records-guard.yml", ""),
	gitHookPiece("pre-commit"),
}

const retiredWhy = "retired with the main branch's records/ copy"

// retire is the file at path without the retired piece p: nil when it holds none, Gone when nothing else is left.
// Read-only.
func retire(p Piece, path string) (*Planned, error) {
	text, err := Read(path)
	if err != nil || text == nil {
		return nil, err
	}
	if there, err := p.Present(text); err != nil || !there {
		return nil, err
	}
	n, err := p.Remove(*text)
	if err != nil {
		return nil, err
	}
	pl := Planned{Piece: p, Path: path, Text: text, Retired: true, Gone: n == nil}
	if n != nil {
		pl.New = *n
	}
	return &pl, nil
}

// Planned is one file to write: its piece (for a file holding only Beads' pieces, one with just its Rel and Mode), path,
// current text (nil: absent) and new text; Wrote is whether pm's part in it changes, Removed what of Beads' it loses,
// Retired whether it loses a retired piece, and Gone whether the file goes with it.
type Planned struct {
	Piece   Piece
	Path    string
	Text    *string
	New     string
	Wrote   bool
	Removed []string
	Retired bool
	Gone    bool
}

// Said is what writing the planned files does, one line each: "wrote <file>" for pm's part, "removed <what> from
// <file>" for Beads', and "removed <file>" or "removed pm's section from <file>" for a retired piece.
func Said(planned []Planned) []string {
	var out []string
	for _, p := range planned {
		switch {
		case p.Retired && p.Gone:
			out = append(out, "removed "+p.Piece.Rel+", "+retiredWhy)
		case p.Retired:
			out = append(out, "removed pm's section from "+p.Piece.Rel+", "+retiredWhy)
		}
		if p.Wrote {
			out = append(out, "wrote "+p.Piece.Rel)
		}
		for _, r := range p.Removed {
			out = append(out, "removed "+r+" from "+p.Piece.Rel)
		}
	}
	return out
}

// plan is each file under top to write: with all, every piece as this version writes it (pm upgrade); else each piece
// not present (pm init); either way without Beads' pieces and the retired ones. Read-only, so a refusal leaves the
// worktree as it was.
func plan(top string, s Settings, all bool) ([]Planned, error) {
	var out []Planned
	add := func(p Piece, path string, text *string, n string, wrote bool) error {
		var removed []string
		if l, ok := leftoverOf(p.Rel); ok && !isLink(path) {
			what, err := l.find(n)
			if err != nil {
				return err
			}
			if what != "" {
				if n, err = l.clean(n); err != nil {
					return err
				}
				removed = append(removed, what)
			}
		}
		if text == nil || n != *text {
			out = append(out, Planned{Piece: p, Path: path, Text: text, New: n, Wrote: wrote, Removed: removed})
		}
		return nil
	}
	seen := map[string]bool{}
	for _, p := range Pieces(s) {
		seen[p.Rel] = true
		path := filepath.Join(top, p.Rel)
		text, err := Read(path)
		if err != nil {
			return nil, err
		}
		apply := all
		if !all {
			ok, err := p.Present(text)
			if err != nil {
				return nil, err
			}
			apply = !ok
		}
		n, wrote := "", false
		switch {
		case apply:
			if n, err = p.Apply(text); err != nil {
				return nil, err
			}
			wrote = text == nil || n != *text
		case text == nil:
			continue
		default:
			n = *text
		}
		if err := add(p, path, text, n, wrote); err != nil {
			return nil, err
		}
	}
	for _, rel := range leftoverRels {
		path := filepath.Join(top, rel)
		if seen[rel] || isLink(path) {
			continue
		}
		text, err := Read(path)
		if err != nil {
			return nil, err
		}
		if text != nil {
			if err := add(Piece{Rel: rel, Mode: 0o644}, path, text, *text, false); err != nil {
				return nil, err
			}
		}
	}
	for _, p := range Retired {
		pl, err := retire(p, filepath.Join(top, p.Rel))
		if err != nil {
			return nil, err
		}
		if pl != nil {
			out = append(out, *pl)
		}
	}
	return out, nil
}

// Read is a file's text, or nil when it is absent: missing, or under a path whose parent is a file, as Python's
// Path.exists() has it (a repo that retired Beads keeps a file .beads, so .beads/hooks/* is absent); any other error
// fails.
func Read(path string) (*string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ptr(string(b)), nil
}

// Plan is each piece not present under top, and each file holding Beads' or retired pieces, with its path, current text and new
// text. Planning reads only, so a refusal leaves the worktree as it was.
func Plan(top string, s Settings) ([]Planned, error) { return plan(top, s, false) }

// Drift is each piece whose pm part under top is not what this version writes, and each Beads or retired piece left,
// as one line saying how.
func Drift(top string, s Settings) ([]string, error) {
	var out []string
	note := func(rel string, line string, err error) (bool, error) {
		var ie *Error
		if err != nil && !errors.As(err, &ie) {
			return false, err
		}
		if ie != nil {
			line = rel + ": " + ie.Msg
		}
		if line != "" {
			out = append(out, line)
		}
		return ie == nil, nil
	}
	left := func(rel string, text *string) error {
		l, ok := leftoverOf(rel)
		if !ok || text == nil || isLink(filepath.Join(top, rel)) {
			return nil
		}
		what, err := l.find(*text)
		line := ""
		if what != "" {
			line = fmt.Sprintf("%s: holds %s, which pm %s removes", rel, what, buildinfo.Version)
		}
		_, err = note(rel, line, err)
		return err
	}
	seen := map[string]bool{}
	for _, p := range Pieces(s) {
		seen[p.Rel] = true
		text, err := Read(filepath.Join(top, p.Rel))
		if err != nil {
			return nil, err
		}
		line, err := drift(p, text)
		readable, err := note(p.Rel, line, err)
		if err != nil {
			return nil, err
		}
		if readable { // an unreadable file's line says so already
			if err := left(p.Rel, text); err != nil {
				return nil, err
			}
		}
	}
	for _, rel := range leftoverRels {
		if seen[rel] {
			continue
		}
		text, err := Read(filepath.Join(top, rel))
		if err != nil {
			return nil, err
		}
		if err := left(rel, text); err != nil {
			return nil, err
		}
	}
	for _, p := range Retired {
		pl, err := retire(p, filepath.Join(top, p.Rel))
		line := ""
		if pl != nil {
			what := "section"
			if pl.Gone {
				what = "file"
			}
			line = fmt.Sprintf("%s: an earlier pm's %s, %s; pm %s removes it", p.Rel, what, retiredWhy, buildinfo.Version)
		}
		if _, err := note(p.Rel, line, err); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func drift(p Piece, text *string) (string, error) {
	applied, err := p.Apply(text)
	if err != nil {
		return "", err
	}
	want, _, err := p.Part(&applied)
	if err != nil {
		return "", err
	}
	have, there, err := p.Part(text)
	if err != nil {
		return "", err
	}
	switch {
	case !there:
		return p.Rel + ": pm's part is missing", nil
	case have != want:
		return fmt.Sprintf("%s: pm's part differs from what pm %s writes", p.Rel, buildinfo.Version), nil
	}
	return "", nil
}

// Rewrite is every file under top that differs from the file with pm's part as this version writes it and without
// Beads' pieces. Read-only, so a refusal leaves the worktree as it was.
func Rewrite(top string, s Settings) ([]Planned, error) { return plan(top, s, true) }

// Removal is one file holding a pm part and its text without it (nil: delete the file).
type Removal struct {
	Path string
	New  *string
}

// Removals is each file under top holding a pm part, a retired one too, with the file without it; read-only.
func Removals(top string, s Settings) ([]Removal, error) {
	var out []Removal
	for _, p := range Pieces(s) {
		path := filepath.Join(top, p.Rel)
		text, err := Read(path)
		if err != nil {
			return nil, err
		}
		if text == nil {
			continue
		}
		if _, there, err := p.Part(text); err != nil {
			return nil, err
		} else if !there {
			continue
		}
		n, err := p.Remove(*text)
		if err != nil {
			return nil, err
		}
		out = append(out, Removal{path, n})
	}
	for _, p := range Retired {
		pl, err := retire(p, filepath.Join(top, p.Rel))
		if err != nil {
			return nil, err
		}
		if pl != nil {
			r := Removal{Path: pl.Path}
			if !pl.Gone {
				r.New = ptr(pl.New)
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// Write writes the planned pieces, a new file with its piece's mode, and deletes each file a retired piece leaves
// empty; the paths written or deleted, as pieces name them.
func Write(planned []Planned) ([]string, error) {
	var out []string
	for _, p := range planned {
		if p.Gone {
			if err := os.Remove(p.Path); err != nil {
				return out, err
			}
			out = append(out, p.Piece.Rel)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
			return out, err
		}
		if err := os.WriteFile(p.Path, []byte(p.New), 0o644); err != nil {
			return out, err
		}
		if p.Text == nil {
			if err := os.Chmod(p.Path, p.Piece.Mode); err != nil {
				return out, err
			}
		}
		out = append(out, p.Piece.Rel)
	}
	return out, nil
}
