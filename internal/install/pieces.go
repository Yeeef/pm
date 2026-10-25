// Package install is what pm init, pm doctor, pm upgrade and pm uninstall manage: the pieces pm puts in a repo's
// tracked files, the clone's and the worktree's setup (the work store, the records store, the main checkout's link to
// it and each other worktree's records workspace, the excludes, the Codex and Claude Code settings), pm's own binary
// on the machine, and the bootstrap of a brand-new repo's records branch.
//
// A piece is either a whole file pm owns (.pm/config.toml, .pm/README.md, .pm/.gitignore) or pm's part of a shared
// file: its hook entries in .claude/settings.json and .codex/hooks.json (a hook is pm's when its command starts with
// "pm prime" or "pm hook "), Codex's auto-mode keys in .codex/config.toml (a key set to another value is the user's),
// and its marked block in .gitignore. Each piece has Present, whether pm's part is there,
// Apply, the file with pm's part as this version writes it and every other byte kept, Part, pm's part alone (what pm
// doctor compares with Part(Apply(text))), and Remove, the file without pm's part (nil: nothing else is left, so the
// file goes). pm init applies a piece only when it is not present, so a second run changes nothing; pm upgrade applies
// every piece; pm uninstall removes every piece. Each writes the bytes Python pm 0.1.x writes.
//
// Go pm runs no bd, so pm init and pm upgrade also take Beads' own pieces out of the tracked files (the work-store
// page, Cut-over): bd's hook entries in the runtimes' settings and the Beads block in CLAUDE.md and AGENTS.md
// (leftovers below); pm doctor names each one left. pm uninstall leaves them, as they are not pm's.
//
// A retired piece (Retired) is one an earlier pm wrote and this one does not: pm doctor names each one left, and pm
// init, pm upgrade and pm uninstall remove it. pm installs no git hook: its sections in its own hook files
// .pm/hooks/post-checkout and pre-commit are retired pieces, and the clone's core.hooksPath, which an earlier pm
// pointed at .pm/hooks, is unset (RetireHooksPath); a core.hooksPath naming another directory, Beads' .beads/hooks
// among them, is not pm's and stays.
//
// The files: pieces.go the pieces, among them the hook entries (claudeHooks and codexHooks: a hook change edits both;
// TestClaudeAndCodexRunTheSameHooks fails on a difference runtimeDifferences gives no reason for), and Retired (the
// main branch's records/ copy and its copy and guard workflows, the pre-commit section; the post-checkout section);
// clone.go the clone's setup, which also gives the clone its id once (store.MakeCloneID), turns off the per-worktree
// sparse checkout once the worktree tracks no records/, and unsets a core.hooksPath at .pm/hooks; workstore.go the
// work store attached to the remote's refs/pm/work (cloned, or created and pushed); codex.go the Codex roots;
// codexmode.go Codex's auto mode in the repo's .codex/config.toml;
// codextrust.go and sessiontrust.go which of pm's hooks Codex trusts (SessionTrust, the state hook's check in a Codex
// session, kept in .pm/run/codex-trust.json); codexsandbox.go whether Codex's Linux sandbox (bwrap) starts, which
// pm doctor checks; binary.go pm copied into the bin dir; pins.go the launcher's pins that
// no checkout on the machine pins, which pm upgrade prunes and pm doctor names; legacy.go the pre-package harness's
// pieces, found and refused; jsonobject.go a JSON settings file read and rewritten with every key in its order; git.go
// the git calls, among them the records branch's bootstrap (CreateRecordsBranch). golden_test.go holds the pieces on a
// table of inputs to testdata/pieces.json.
package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/hooks"
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
	Listen     string // listen, "" when unset (the site binds loopback)
	Access     string // access, cloudflare or none; "" when unset
	AccessTeam string // the Cloudflare Access keys, set by hand: carried so a config holding them is pm's own
	AccessAud  string
	// SummaryRuntime is summary_runtime, set by hand ("" when unset): carried so a config holding it is pm's own
	SummaryRuntime string
	// DesignSources is design_sources, set by hand (nil when unset): carried so a config holding it is pm's own
	DesignSources []string
}

// SettingsOf is the settings a checked config holds: what pm upgrade rewrites the config with and pm doctor compares it
// to, so every key the config reads is here.
func SettingsOf(c config.Config) Settings {
	return Settings{Remote: c.Remote, MainBranch: c.MainBranch, Port: int(c.Port), SiteURL: c.SiteURL,
		Listen: c.Listen, Access: c.Access, AccessTeam: c.AccessTeam, AccessAud: c.AccessAud, SummaryRuntime: c.SummaryRuntime, DesignSources: c.DesignSources}
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
	for _, kv := range [][2]string{{"site_url", s.SiteURL}, {"listen", s.Listen}, {"access", s.Access}, {"access_team", s.AccessTeam}, {"access_aud", s.AccessAud},
		{"summary_runtime", s.SummaryRuntime}} {
		if kv[1] != "" {
			site += kv[0] + " = " + tomlString(kv[1]) + "\n"
		}
	}
	if len(s.DesignSources) > 0 {
		quoted := make([]string, len(s.DesignSources))
		for i, p := range s.DesignSources {
			quoted[i] = tomlString(p)
		}
		site += "design_sources = [" + strings.Join(quoted, ", ") + "]\n"
	}
	return "version = " + tomlString(buildinfo.Version) + "   # the pm version every session must run\n" +
		"remote = " + tomlString(s.Remote) + "\n" +
		"main_branch = " + tomlString(s.MainBranch) + "\n" +
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

// CodexRules is .codex/rules/pm.rules, a Codex project rule that runs pm outside Codex's sandbox: a pm command
// reaches the pm service's socket, which a network-off sandbox blocks (on Linux, seccomp denies connect()), and Codex
// runs a command whose every part a rule allows unsandboxed (codex-rs 0.154 core/src/exec_policy.rs, the
// bypass_sandbox of an allowed command). It allows pm alone. A shell script Codex cannot split into plain commands,
// a heredoc's among them, matches no rule and stays in the sandbox.
const CodexRules = "# pm reaches the pm service's socket, which Codex's sandbox blocks: run pm outside it (pm init writes this)\n" +
	"prefix_rule(pattern=[\"pm\"], decision=\"allow\")\n"

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
	o, ok := h.(*object)
	if !ok {
		return false
	}
	cmd, ok := o.Get("command").(string)
	return ok && (strings.HasPrefix(cmd, "pm prime") || strings.HasPrefix(cmd, "pm hook "))
}

func obj(pairs ...any) *object {
	o := newObject()
	for i := 0; i < len(pairs); i += 2 {
		o.Set(pairs[i].(string), pairs[i+1])
	}
	return o
}

// entries is pm's matcher group per event, in the order pm writes the events.
type entries struct {
	events []string
	groups map[string]func() *object // a fresh copy each call
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
		events: []string{"SessionStart", "SubagentStart", "PermissionRequest", "PostToolUse", "Stop", "SessionEnd"},
		groups: map[string]func() *object{
			"SessionStart": func() *object {
				return obj("hooks", append(rules(30), h("pm prime --state --hook-json", 30),
					h("pm prime --request-rule --hook-json", 10)), "matcher", "")
			},
			"SubagentStart": func() *object {
				return obj("hooks", append(rules(15), h("pm prime --subagent --hook-json", 15)), "matcher", "")
			},
			"PermissionRequest": func() *object { // beside the question's dialog, up to a day for the owner's answer
				return obj("hooks", []any{h("pm hook ask --runtime claude || exit 1", 86400)}, "matcher", "AskUserQuestion")
			},
			"PostToolUse": func() *object { // the dialog's answer closes the question
				return obj("hooks", []any{h("pm hook ask --runtime claude || exit 1", 10)}, "matcher", "AskUserQuestion")
			},
			"Stop": func() *object {
				return obj("hooks", []any{h("pm hook owner-request --runtime claude || exit 1", 30), h("pm hook stop || exit 1", 10)})
			},
			"SessionEnd": func() *object {
				return obj("hooks", []any{h("pm hook session-end || exit 1", 10)})
			},
		},
	}
}

// codexHooks is pm's entries in .codex/hooks.json. Codex keys its trust in each hook by its group and index, so a new
// entry goes after the released ones of its group (TestNewHookEntriesFollowTheReleasedOnes). Codex's SessionStart
// matcher is a pattern over its sources, which name each one: startup, resume, clear and compact. Its SubagentStop runs
// pm hook stop on the subagent's own rollout: Codex keeps a subagent's prompt from its parent's rollout (encrypted), so
// the parent's Stop cannot see what the subagent was asked to write, as Claude Code's parent transcript shows it. Its
// PostToolUse runs pm hook ask on each request_user_input_async call, which returns at once and asks no permission, to
// raise its question (Claude Code raises AskUserQuestion's at PermissionRequest), with time for a question under a
// task, whose id the remote mints; its UserPromptSubmit runs pm hook ask too, since the owner's next message is the
// chat answer to an open question (Claude Code's comes at AskUserQuestion's PostToolUse).
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
		events: []string{"SessionStart", "SubagentStart", "UserPromptSubmit", "PostToolUse", "Stop", "SubagentStop", "SessionEnd"},
		groups: map[string]func() *object{
			"SessionStart": func() *object {
				return obj("hooks", append(rules(30), h("pm prime --state --hook-json", "Loading pm init, pm where and pm show", 30),
					h("pm prime --request-rule --hook-json", "Loading the rule for asking the owner", 10)), "matcher", "startup|resume|clear|compact")
			},
			"SubagentStart": func() *object {
				return obj("hooks", append(rules(15), h("pm prime --subagent --hook-json", "Loading pm's git rule for agents", 15)))
			},
			"UserPromptSubmit": func() *object {
				return obj("hooks", []any{h("pm hook ask --runtime codex || exit 1", "Closing the questions this prompt answers", 10)})
			},
			"PostToolUse": func() *object {
				return obj("hooks", []any{h("pm hook ask --runtime codex || exit 1", "Raising the question for the site", 30)},
					"matcher", "request_user_input_async")
			},
			"Stop": func() *object {
				return obj("hooks", []any{h("pm hook owner-request --runtime codex || exit 1", "Checking for owner requests asked only in chat", 45),
					h("pm hook stop || exit 1", "Checking for uncommitted records", 10)})
			},
			"SubagentStop": func() *object {
				return obj("hooks", []any{h("pm hook stop || exit 1", "Checking for uncommitted records", 10)})
			},
			"SessionEnd": func() *object {
				return obj("hooks", []any{h("pm hook session-end || exit 1", "Clearing this session's interactive mode", 10)})
			},
		},
	}
}

// settingsFile is a settings file's data, its `hooks` checked to map events to lists of matcher groups.
type settingsFile struct {
	data   *object
	events *object // nil when the file has no hooks
}

// loadHooks is the settings file's data, refused unless it is a JSON object whose hooks maps events to lists of matcher
// groups; with layout, also unless it is laid out as pm writes JSON (two-space indent), so rewriting it keeps every
// byte that is not pm's.
func loadHooks(rel string, text *string, layout bool) (settingsFile, error) {
	if text == nil {
		return settingsFile{data: newObject()}, nil
	}
	v, err := parseJSON(*text)
	if err != nil {
		return settingsFile{}, refuse("%s is not valid JSON (%s); fix it by hand and run pm init again", rel, err)
	}
	bad := refuse("%s: hooks is not a map of events to lists of matcher groups; fix it by hand", rel)
	data, ok := v.(*object)
	if !ok {
		return settingsFile{}, bad
	}
	var events *object
	if data.Has("hooks") {
		if events, ok = data.Get("hooks").(*object); !ok {
			return settingsFile{}, bad
		}
		for _, e := range events.Keys {
			gs, ok := events.Get(e).([]any)
			if !ok {
				return settingsFile{}, bad
			}
			for _, g := range gs {
				g, ok := g.(*object)
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
	o := g.(*object) //nolint:errcheck // loadHooks checked that every group is an object
	hs, _ := o.Get("hooks").([]any)
	return hs
}

// hooksPresent is whether the file runs each of pm's hooks at its event and no pm hook this version does not write
// there, such as an earlier pm's pm hook inbox: pm init writes the piece otherwise, which takes that one out.
func hooksPresent(rel string, want entries) func(*string) (bool, error) {
	return func(text *string) (bool, error) {
		f, err := loadHooks(rel, text, false)
		if err != nil {
			return false, err
		}
		wanted := map[string]map[string]bool{}
		for _, event := range want.events {
			wanted[event] = map[string]bool{}
			for _, h := range groupHooks(want.groups[event]()) {
				wanted[event][h.(*object).Get("command").(string)] = true //nolint:errcheck // pm's own hook, built with a command
			}
		}
		have := map[string]map[string]bool{}
		if f.events != nil {
			for _, event := range f.events.Keys {
				have[event] = map[string]bool{}
				gs := f.events.Get(event).([]any) //nolint:errcheck // loadHooks checked that it is a list
				for _, g := range gs {
					for _, h := range groupHooks(g) {
						if !isPMHook(h) {
							continue
						}
						c := h.(*object).Get("command").(string) //nolint:errcheck // isPMHook checked both
						if !wanted[event][c] {
							return false, nil
						}
						have[event][c] = true
					}
				}
			}
		}
		for event, cs := range wanted {
			for c := range cs {
				if !have[event][c] {
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
		o := g.(*object) //nolint:errcheck // loadHooks checked that every group is an object
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
// goes too, and an event left empty that this version writes no hook at), then pm's group appended to each event.
// Everything else keeps its place.
func hooksApply(rel string, want entries) func(*string) (string, error) {
	return func(text *string) (string, error) {
		f, err := loadHooks(rel, text, true)
		if err != nil {
			return "", err
		}
		events := f.events
		if events == nil {
			events = newObject()
			f.data.Set("hooks", events)
		}
		for _, e := range append([]string(nil), events.Keys...) {
			gs := events.Get(e).([]any) //nolint:errcheck // loadHooks checked that it is a list
			kept := dropPMHooks(gs)
			if _, wanted := want.groups[e]; !wanted && len(gs) > 0 && len(kept) == 0 {
				events.Delete(e) // an event only an earlier pm's hook ran at
			} else {
				events.Set(e, kept)
			}
		}
		for _, e := range want.events {
			gs, _ := events.Get(e).([]any)
			events.Set(e, append(gs, want.groups[e]()))
		}
		return DumpJSON(f.data), nil
	}
}

// hooksPart is pm's hooks by event, in order, each as written and with the matcher of its group, with object keys
// sorted so that it compares equal however a file orders them.
func hooksPart(rel string) func(*string) (string, bool, error) {
	return func(text *string) (string, bool, error) {
		if text == nil {
			return "", false, nil
		}
		f, err := loadHooks(rel, text, false)
		if err != nil {
			return "", false, err
		}
		mine := newObject()
		if f.events != nil {
			for _, e := range f.events.Keys {
				var hs []any
				for _, g := range f.events.Get(e).([]any) {
					var matcher any
					if o, ok := g.(*object); ok { // loadHooks checked it is
						matcher = o.Get("matcher")
					}
					for _, h := range groupHooks(g) {
						if isPMHook(h) {
							hs = append(hs, []any{matcher, h})
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

// HooksRel is the git hook directory an earlier pm kept under a worktree and pointed core.hooksPath at; its hook
// sections are retired (Retired), and so is that hooks path (RetireHooksPath).
const HooksRel = ".pm/hooks"

const (
	sectionEnd = "# --- END PM ---"
	shebang    = "#!/usr/bin/env sh\n"
)

var (
	section  = regexp.MustCompile(`(?ms)^# --- BEGIN PM v[^\n]* ---\n.*?^# --- END PM ---\n?`)
	beginAny = regexp.MustCompile(`(?m)^# --- BEGIN PM v[^\n]* ---$`)
)

func stripSection(rel, text string, pattern, begin *regexp.Regexp) (string, error) {
	out := pattern.ReplaceAllLiteralString(text, "")
	if begin.MatchString(out) {
		return "", refuse("%s has a pm begin marker without its end marker (%s); fix it by hand", rel, sectionEnd)
	}
	return out, nil
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
	o, ok := h.(*object)
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
					if c := h.(*object).Get("command").(string); !contains(cmds, c) { //nolint:errcheck // isBeadsHook checked both
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
		whole(".codex/rules/pm.rules", CodexRules),
		codexModePiece(),
		{".gitignore", marked(gitignoreBeginRe), gitignoreApply, sectionPart(gitignoreSection),
			sectionRemove(".gitignore", gitignoreSection, gitignoreBeginRe, ""), 0o644},
	}
	return out
}

// gitHookPiece is an earlier pm's marked section in its git hook file .pm/hooks/<name>, which held only a shebang
// besides when pm made it; retired, so it has no Apply.
func gitHookPiece(name string) Piece {
	rel := HooksRel + "/" + name
	return Piece{rel, marked(beginAny), nil, sectionPart(section), sectionRemove(rel, section, beginAny, "", shebang),
		0o755}
}

// Retired is each piece an earlier pm wrote and this one does not: with the main branch's records/ copy, the workflow
// that copied the records branch into it, the one that failed a pull request editing it, and pm's pre-commit section
// that refused a code-branch commit editing it; with pm's git hooks (HooksWhy), its post-checkout section, which set a
// new worktree up as session start does. Its Apply is nil.
var Retired = []Piece{
	whole(".github/workflows/pm-records-copy.yml", ""),
	whole(".github/workflows/pm-records-guard.yml", ""),
	gitHookPiece("pre-commit"),
	gitHookPiece("post-checkout"),
}

// HooksWhy is why pm's git hooks, its post-checkout section and core.hooksPath at .pm/hooks, are retired.
const HooksWhy = "retired with pm's git hooks, as session start sets each new worktree up"

// whyRetired is why the retired piece at rel is retired.
func whyRetired(rel string) string {
	if rel == HooksRel+"/post-checkout" {
		return HooksWhy
	}
	return "retired with the main branch's records/ copy"
}

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
			out = append(out, "removed "+p.Piece.Rel+", "+whyRetired(p.Piece.Rel))
		case p.Retired:
			out = append(out, "removed pm's section from "+p.Piece.Rel+", "+whyRetired(p.Piece.Rel))
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
		var n string
		wrote := false
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
			line = fmt.Sprintf("%s: an earlier pm's %s, %s; pm %s removes it", p.Rel, what, whyRetired(p.Rel),
				buildinfo.Version)
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
		if n != nil && *n == *text { // pm's part is all the user's: Codex mode keys set outside pm's block
			continue
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
// empty, and .pm/hooks once that leaves it empty; the paths written or deleted, as pieces name them.
func Write(planned []Planned) ([]string, error) {
	var out []string
	for _, p := range planned {
		if p.Gone {
			if err := os.Remove(p.Path); err != nil {
				return out, err
			}
			if err := RemoveEmptyHooksDir(p.Piece.Rel, p.Path); err != nil {
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

// RemoveEmptyHooksDir removes .pm/hooks, the directory of the file at path, once removing a retired hook file at rel
// leaves it empty; a directory that still holds a file stays.
func RemoveEmptyHooksDir(rel, path string) error {
	if !strings.HasPrefix(rel, HooksRel+"/") {
		return nil
	}
	err := os.Remove(filepath.Dir(path))
	if err == nil || errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return nil
	}
	return err
}
