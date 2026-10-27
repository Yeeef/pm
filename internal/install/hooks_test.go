package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
)

// runtimeDifference is one way pm's hook entries for Claude Code and for Codex differ: an event's commands, or the
// matcher of its group.
type runtimeDifference struct{ event, what string }

// runtimeDifferences is every difference between claudeHooks and codexHooks, each with its reason. Any other difference
// fails TestClaudeAndCodexRunTheSameHooks, and so does a difference listed here that is gone.
var runtimeDifferences = map[runtimeDifference]string{
	{"SessionStart", "matcher"}: `Claude Code's "" matches every source; Codex's matcher is a pattern over its ` +
		`sources, so pm names each: startup, resume, clear and compact`,
	{"SubagentStart", "matcher"}: `both match every subagent: Claude Code's group has "", Codex's has none`,
	{"SubagentStop", "commands"}: "Codex only: Codex keeps a subagent's prompt from its parent's rollout " +
		"(encrypted), so the parent's Stop cannot tell which records the subagent was asked to write; pm hook stop " +
		"reads the subagent's own rollout at its SubagentStop instead. Claude Code's parent transcript holds the " +
		"prompt, which pm hook stop reads at the parent's Stop",
	{"PermissionRequest", "commands"}: "Claude Code only: AskUserQuestion waits for its dialog's answer, and its " +
		"PermissionRequest hook runs beside the dialog, so pm hook ask raises the question there and waits for the " +
		"site's answer. Codex's request_user_input_async returns at once and asks no permission; pm hook ask raises " +
		"it at its PostToolUse",
	{"PermissionRequest", "matcher"}: "Claude Code only: its group matches AskUserQuestion alone",
	{"PostToolUse", "matcher"}: "both run pm hook ask on their ask tool: Claude Code's AskUserQuestion, whose " +
		"PostToolUse carries the dialog's answer and closes the question with it; Codex's request_user_input_async, " +
		"whose PostToolUse raises the question by its call id (tool_use_id)",
	{"Stop", "commands"}: "Claude Code only: AskUserQuestion's Chat about this, and a skip in the Claude app through Remote " +
		"Control, stop no hook and fire no PostToolUse, and no dialog outlives its turn, so pm hook ask closes the " +
		"session's questions still open at Stop. A Codex question outlives its turn by design",
	{"UserPromptSubmit", "commands"}: "Codex only: request_user_input_async returns at once, and the owner's next " +
		"message is its chat answer, so pm hook ask closes the session's open questions with the prompt; Claude " +
		"Code's dialog answer comes at AskUserQuestion's PostToolUse",
}

// ownRuntime is the one part of a command that differs by design and is no runtimeDifferences entry, so that the rest
// of the command stays held equal: the owner-request judge runs on the session's own runtime (owner decision,
// 2026-10-12: Haiku through claude -p for Claude Code, a small model through codex exec for Codex, so a machine with one
// runtime keeps the check), and each entry names it. commandsAndMatcher reads it as "--runtime <own>", and
// TestEachRuntimesJudgeRunsOnThatRuntime holds what each entry names.
var ownRuntime = regexp.MustCompile(`--runtime (claude|codex)\b`)

// commandsAndMatcher is the commands of event's group, in order, with ownRuntime read as "--runtime <own>", and its
// matcher (nil when it has none).
func commandsAndMatcher(t *testing.T, e entries, event string) ([]string, any) {
	t.Helper()
	group, ok := e.groups[event]
	if !ok {
		return nil, nil
	}
	g := group()
	var cmds []string
	for _, h := range groupHooks(g) {
		o, ok := h.(*object)
		if !ok {
			t.Fatalf("%s: a hook that is no object: %v", event, h)
		}
		c, ok := o.Get("command").(string)
		if !ok {
			t.Fatalf("%s: a hook with no command: %v", event, o)
		}
		cmds = append(cmds, ownRuntime.ReplaceAllString(c, "--runtime <own>"))
	}
	return cmds, g.Get("matcher")
}

func TestEachRuntimesJudgeRunsOnThatRuntime(t *testing.T) {
	for runtime, e := range map[string]entries{"claude": claudeHooks(), "codex": codexHooks()} {
		var judges []string
		for _, h := range groupHooks(e.groups["Stop"]()) {
			o, ok := h.(*object)
			if !ok {
				t.Fatalf("%s's Stop: a hook that is no object: %v", runtime, h)
			}
			if c, ok := o.Get("command").(string); ok && strings.Contains(c, "hook owner-request") {
				judges = append(judges, c)
			}
		}
		if want := "pm hook owner-request --runtime " + runtime + " || exit 1"; !reflect.DeepEqual(judges, []string{want}) {
			t.Errorf("%s's Stop runs the judge as %q, not %q", runtime, judges, want)
		}
	}
}

func TestClaudeAndCodexRunTheSameHooks(t *testing.T) {
	claude, codex := claudeHooks(), codexHooks()
	events := append([]string(nil), claude.events...)
	for _, e := range codex.events {
		if !strings.Contains(" "+strings.Join(events, " ")+" ", " "+e+" ") {
			events = append(events, e)
		}
	}
	seen := map[runtimeDifference]bool{}
	for _, e := range events {
		cc, cm := commandsAndMatcher(t, claude, e)
		xc, xm := commandsAndMatcher(t, codex, e)
		for what, same := range map[string]bool{"commands": reflect.DeepEqual(cc, xc), "matcher": cm == xm} {
			d := runtimeDifference{e, what}
			if same {
				continue
			}
			seen[d] = true
			if _, ok := runtimeDifferences[d]; !ok {
				t.Errorf("%s %s differ, and runtimeDifferences gives no reason: Claude Code %q %v, Codex %q %v", e, what,
					cc, cm, xc, xm)
			}
		}
	}
	for d := range runtimeDifferences {
		if !seen[d] {
			t.Errorf("%s %s no longer differ: take them out of runtimeDifferences", d.event, d.what)
		}
	}
}

// codexPiece is the .codex/hooks.json piece.
func codexPiece(t *testing.T) Piece {
	t.Helper()
	for _, p := range Pieces(goldenSettings[0]) {
		if p.Rel == ".codex/hooks.json" {
			return p
		}
	}
	t.Fatal("no .codex/hooks.json piece")
	return Piece{}
}

// TestAnEarlierPmsCodexHooksAreRewritten: the .codex/hooks.json pm 0.6 wrote (no compact in the SessionStart
// matcher, no SubagentStop) is drift for pm doctor, missing for pm init, and rewritten by pm upgrade; so is a file
// whose SessionStart matcher alone is the old one.
func TestAnEarlierPmsCodexHooksAreRewritten(t *testing.T) {
	old, err := os.ReadFile(filepath.Join("testdata", "codex-hooks-0.6.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := codexPiece(t)
	now, err := p.Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	matcherOnly := strings.Replace(now, `"matcher": "startup|resume|clear|compact"`, `"matcher": "startup|resume|clear"`, 1)
	if matcherOnly == now {
		t.Fatal("no SessionStart matcher to change")
	}
	for name, text := range map[string]string{"pm 0.6's": string(old), "the old matcher's": matcherOnly} {
		top := t.TempDir()
		path := filepath.Join(top, ".codex", "hooks.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		lines, err := Drift(top, goldenSettings[0])
		if err != nil {
			t.Fatal(err)
		}
		want := ".codex/hooks.json: pm's part differs from what pm " + buildinfo.Version + " writes"
		if !strings.Contains("\n"+strings.Join(lines, "\n")+"\n", "\n"+want+"\n") {
			t.Errorf("%s: pm doctor does not report it: %q", name, lines)
		}
		planned, err := Rewrite(top, goldenSettings[0])
		if err != nil {
			t.Fatal(err)
		}
		rewritten := false
		for _, pl := range planned {
			if pl.Piece.Rel == ".codex/hooks.json" {
				line, err := drift(p, &pl.New) // an event pm adds goes after the file's own: compare pm's part
				rewritten = err == nil && line == "" && pl.Wrote
			}
		}
		if !rewritten {
			t.Errorf("%s: pm upgrade does not rewrite it as this pm writes it", name)
		}
	}
	if there, err := p.Present(ptr(string(old))); err != nil || there {
		t.Errorf("pm 0.6's file has every hook pm init writes: %v %v", there, err)
	}
}

// TestAnEarlierPmsInboxHookIsTakenOut: the UserPromptSubmit hook pm hook inbox, which pm 0.9 wrote in both runtimes'
// files and this pm no longer has, is missing-or-stale for pm init (which then writes the piece), drift for pm doctor,
// and taken out by pm init and pm upgrade alike; the user's own UserPromptSubmit hook stays, pm's own group there
// (Codex's pm hook ask) is written after it, and an event that held only pm's hook and gets none from this pm goes
// (Claude Code's).
func TestAnEarlierPmsInboxHookIsTakenOut(t *testing.T) {
	user := obj("hooks", []any{obj("command", "./mine.sh", "type", "command")})
	inbox := obj("hooks", []any{obj("command", "pm hook inbox || exit 1", "type", "command", "timeout", 10)})
	for _, p := range Pieces(goldenSettings[0]) {
		if p.Rel != ".claude/settings.json" && p.Rel != ".codex/hooks.json" {
			continue
		}
		now, err := p.Apply(nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(now, "pm hook inbox") {
			t.Fatalf("%s: this pm still writes pm hook inbox", p.Rel)
		}
		pms := promptGroups(t, now) // what this pm writes at UserPromptSubmit, if anything
		inboxTakenOut(t, p, "beside the user's own hook", withPrompt(t, now, user, inbox),
			withPrompt(t, now, append([]any{user}, pms...)...))
		inboxTakenOut(t, p, "alone at its event", withPrompt(t, now, inbox), now)
	}
}

// promptGroups is the file's UserPromptSubmit groups; nil when it has none.
func promptGroups(t *testing.T, text string) []any {
	t.Helper()
	v, err := parseJSON(text)
	o, ok := v.(*object)
	if err != nil || !ok {
		t.Fatalf("not a JSON object (%v): %s", err, text)
	}
	events, ok := o.Get("hooks").(*object)
	if !ok {
		t.Fatalf("no hooks: %s", text)
	}
	groups, ok := events.Get("UserPromptSubmit").([]any)
	if !ok {
		return nil // the file runs nothing there
	}
	return groups
}

// withPrompt is the settings text with groups at UserPromptSubmit.
func withPrompt(t *testing.T, text string, groups ...any) string {
	t.Helper()
	v, err := parseJSON(text)
	o, ok := v.(*object)
	if err != nil || !ok {
		t.Fatalf("not a JSON object (%v): %s", err, text)
	}
	events, ok := o.Get("hooks").(*object)
	if !ok {
		t.Fatalf("no hooks: %s", text)
	}
	events.Set("UserPromptSubmit", groups)
	return DumpJSON(o)
}

// inboxTakenOut checks the piece's file as old: pm init writes it, pm doctor reports it, and pm init and pm upgrade
// both make it want.
func inboxTakenOut(t *testing.T, p Piece, name, old, want string) {
	t.Helper()
	if there, err := p.Present(&old); err != nil || there {
		t.Errorf("%s, %s: pm init takes the piece as present: %v %v", p.Rel, name, there, err)
	}
	top := t.TempDir()
	path := filepath.Join(top, p.Rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := Drift(top, goldenSettings[0])
	said := p.Rel + ": pm's part differs from what pm " + buildinfo.Version + " writes"
	if err != nil || !strings.Contains("\n"+strings.Join(lines, "\n")+"\n", "\n"+said+"\n") {
		t.Errorf("%s, %s: pm doctor does not report it: %q %v", p.Rel, name, lines, err)
	}
	for cmd, run := range map[string]func(string, Settings) ([]Planned, error){"init": Plan, "upgrade": Rewrite} {
		planned, err := run(top, goldenSettings[0])
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		for _, pl := range planned {
			if pl.Piece.Rel == p.Rel {
				got = pl.New
			}
		}
		if got != want {
			t.Errorf("%s, %s: pm %s writes\n%s\nwant\n%s", p.Rel, name, cmd, got, want)
		}
	}
}

// TestNewHookEntriesFollowTheReleasedOnes: Codex keys its trust in a hook by its event, group and index
// (<file>:session_start:<group>:<index>) beside the entry's hash, so an entry put before a released one shifts that
// one's index: Codex then takes it as modified and the one after as untrusted, and skips both until the owner trusts
// them again. Each event's entries as the last release wrote them (testdata/released-hooks, pm 0.11.0's
// .claude/settings.json and .codex/hooks.json) must be the first of that event's entries now, unchanged and in order;
// a new entry goes after them. Claude Code keys no trust by position; its entries follow the same order. A release
// that changes an entry on purpose rewrites the file it changed here.
func TestNewHookEntriesFollowTheReleasedOnes(t *testing.T) {
	for file, e := range map[string]entries{"claude-settings.json": claudeHooks(), "codex-hooks.json": codexHooks()} {
		raw, err := os.ReadFile(filepath.Join("testdata", "released-hooks", file))
		if err != nil {
			t.Fatal(err)
		}
		var released struct {
			Hooks map[string][]struct{ Hooks []any } `json:"hooks"`
		}
		if err := json.Unmarshal(raw, &released); err != nil {
			t.Fatal(err)
		}
		for event, groups := range released.Hooks {
			group, ok := e.groups[event]
			if !ok {
				t.Errorf("%s: pm no longer writes %s, which the last release did", file, event)
				continue
			}
			text, err := json.Marshal(group())
			if err != nil {
				t.Fatal(err)
			}
			var now struct{ Hooks []any }
			if err := json.Unmarshal(text, &now); err != nil {
				t.Fatal(err)
			}
			was := groups[0].Hooks // pm writes one group per event
			if len(now.Hooks) < len(was) || !reflect.DeepEqual(now.Hooks[:len(was)], was) {
				t.Errorf("%s %s: the released entries %v are not the first of the entries now %v", file, event, was,
					now.Hooks)
			}
		}
	}
}
