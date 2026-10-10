package install

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The pieces the project-management harness put in a repo and a clone before pm was a package (bin/pm and
// skills/project-management/harness/). Python pm 0.1.x moves them (legacy.py); Go pm does not (the pm-go page, Open
// question 8a): it finds them, the same lists legacy.py holds, and refuses, naming the Python release to run first.

// LegacyRelease is the Python pm release whose pm init moves a clone off the pre-package harness.
const LegacyRelease = "0.1.4"

var (
	legacyInstructionFiles = []string{"CLAUDE.md", "AGENTS.md"}
	legacyInstructionLines = []string{
		"@skills/project-management/harness/RULES.md",
		"Runtimes that do not expand `@` imports (such as Codex): read `skills/project-management/harness/RULES.md` " +
			"before working; its rules are binding here.",
	}
	legacySettings  = []string{".claude/settings.json", ".codex/hooks.json"}
	legacyWorkflows = []string{".github/workflows/records-guard.yml", ".github/workflows/records-copy.yml"}
	legacyGitHook   = map[string]string{
		"post-checkout": "\n# A new worktree or clone (previous HEAD is the null id) links its records/ to the shared store,\n" +
			"# whichever tool created it (records/design/records-store.md).\n" +
			"if [ \"$1\" = 0000000000000000000000000000000000000000 ] && [ -x bin/pm ]; then\n" +
			"  bin/pm setup >&2 || exit $?\n" +
			"fi\n",
		"pre-commit": "\n# Records live on the records branch (records/design/records-store.md): a code-branch commit must not edit " +
			"records/.\n" +
			"if [ ! -f \"$(git rev-parse --git-dir)/MERGE_HEAD\" ] && [ -n \"$(git diff --cached --name-only -- records/)\" ]; " +
			"then\n" +
			"  echo >&2 \"error: this commit edits records/, which only the records branch may change; write records with " +
			"bin/pm\"\n" +
			"  echo >&2 \"and unstage these edits: git restore --staged records/\"\n" +
			"  exit 1\n" +
			"fi\n",
	}
	legacyGitignoreMark = "/.records/"
	legacyOldStore      = ".records"
	legacyPushState     = []string{"pm-push.json", "pm-push.lock", "pm-push.log"}
	legacyHookCommands  = func() map[string]bool {
		hooks := []string{"session_context", "owner_request", "uncommitted_records", "reply_wait"}
		bin := []string{"prime --hook-json", "prime --rules --hook-json", "prime --state --hook-json",
			"prime --subagent --hook-json", "hook owner-request || exit 1", "hook stop || exit 1", "hook stop"}
		for n := 1; n < 5; n++ {
			bin = append(bin, fmt.Sprintf("prime --rules %d --hook-json", n))
		}
		out := map[string]bool{}
		for _, h := range hooks {
			out[`python3 "$CLAUDE_PROJECT_DIR"/skills/project-management/harness/`+h+`_hook.py`] = true
			out[`python3 "$CLAUDE_PROJECT_DIR/skills/project-management/harness/`+h+`_hook.py"`] = true
			out[`python3 "$(git rev-parse --show-toplevel)/skills/project-management/harness/`+h+`_hook.py"`] = true
			out[`f="$(git rev-parse --show-toplevel)/skills/project-management/harness/`+h+`_hook.py"; [ -f "$f" ] || exit 0; `+
				`python3 "$f"`] = true
		}
		for _, c := range bin {
			out[`"$CLAUDE_PROJECT_DIR"/bin/pm `+c] = true
			out[`f="$(git rev-parse --show-toplevel)/bin/pm"; [ -f "$f" ] || exit 0; "$f" `+c] = true
			out[`f="$(git rev-parse --show-toplevel)/bin/pm"; [ -f "$f" ] || { echo "pm hook: $f is missing" >&2; exit 1; }; `+
				`"$f" `+c] = true
		}
		return out
	}()
)

// LegacyRepo is each legacy piece in the tracked files under top, one line each; read-only. A settings file that is
// not readable JSON is an Error, since the pieces in it cannot be looked for.
func LegacyRepo(top string) ([]string, error) {
	var out []string
	for _, rel := range legacyInstructionFiles { // AGENTS.md is often a link to CLAUDE.md: a link is left alone
		if isLink(filepath.Join(top, rel)) {
			continue
		}
		t, err := Read(filepath.Join(top, rel))
		if err != nil {
			return nil, err
		}
		if t == nil {
			continue
		}
		for _, l := range splitLines(*t, false) {
			if contains(legacyInstructionLines, strings.TrimRight(l, "\r\n")) {
				out = append(out, rel+": the RULES.md import and the Codex line naming it (pm prime gives the rules now)")
				break
			}
		}
	}
	for _, rel := range legacySettings {
		t, err := Read(filepath.Join(top, rel))
		if err != nil {
			return nil, err
		}
		if t == nil {
			continue
		}
		f, err := loadHooks(rel, t, false)
		if err != nil {
			return nil, err
		}
		found := 0
		if f.events != nil {
			for _, e := range f.events.Keys {
				for _, g := range f.events.Get(e).([]any) {
					for _, h := range groupHooks(g) {
						if isLegacyHook(h) {
							found++
						}
					}
				}
			}
		}
		if found > 0 {
			out = append(out, fmt.Sprintf("%s: %d hook entries running bin/pm or harness/*_hook.py by path", rel, found))
		}
	}
	for _, name := range []string{"post-checkout", "pre-commit"} { // the harness's, pre-commit included
		rel := ".beads/hooks/" + name
		t, err := Read(filepath.Join(top, rel))
		if err != nil {
			return nil, err
		}
		if t != nil && strings.Contains(*t, legacyGitHook[name]) {
			out = append(out, rel+": the unmarked pm code after Beads' section (pm's marked section replaces it)")
		}
	}
	for _, rel := range legacyWorkflows {
		if exists(filepath.Join(top, rel)) {
			out = append(out, rel+": the pre-package harness's workflow")
		}
	}
	t, err := Read(filepath.Join(top, ".gitignore"))
	if err != nil {
		return nil, err
	}
	if t != nil && contains(splitLines(*t, false), legacyGitignoreMark) {
		out = append(out, ".gitignore: "+legacyGitignoreMark+", the old store's line")
	}
	return out, nil
}

func isLegacyHook(h any) bool {
	o, ok := h.(interface{ Get(string) any })
	if !ok {
		return false
	}
	c, ok := o.Get("command").(string)
	return ok && legacyHookCommands[c]
}

// LegacyClone is each legacy piece in the clone: the old store and the old push job's state files.
func LegacyClone(main string) []string {
	var out []string
	if old := filepath.Join(main, legacyOldStore); exists(old) {
		out = append(out, "the records store is still at "+old)
	}
	var state []string
	for _, f := range legacyPushState {
		if exists(filepath.Join(main, ".git", f)) {
			state = append(state, f)
		}
	}
	if len(state) > 0 {
		out = append(out, fmt.Sprintf("the old push job's state is in %s: %s", filepath.Join(main, ".git"),
			strings.Join(state, ", ")))
	}
	return out
}

// LegacyFix is how to move a clone off the pre-package harness: Python pm's pm init, which pins its own retired
// release, then the pin moved to a release pm runs, then pm init.
func LegacyFix() string {
	return fmt.Sprintf("Go pm does not move them: run pm init once with Python pm %s, which moves them "+
		"(uvx --from \"git+https://github.com/Yeeef/pm@pm-v%s\" pm init; this needs uv), then move the pin it "+
		"leaves to a release from 0.2.0 on (pm upgrade --to <X>) and run pm init",
		LegacyRelease, LegacyRelease)
}

// RefuseLegacy refuses a clone holding the pre-package harness's pieces, naming each: with repo, those in the
// worktree's tracked files too (the files pm writes or rewrites: a first install, pm upgrade).
func RefuseLegacy(top, main string, repo bool) error {
	var found []string
	if repo {
		var err error
		if found, err = LegacyRepo(top); err != nil {
			return err
		}
	}
	found = append(found, LegacyClone(main)...)
	if len(found) == 0 {
		return nil
	}
	return refuse("this clone holds the pre-package harness's pieces (%s); %s", strings.Join(found, "; "), LegacyFix())
}

// RefuseLegacyStore refuses a clone whose records store is still where the pre-package harness kept it.
func RefuseLegacyStore(main, records string) error {
	old := filepath.Join(main, legacyOldStore)
	if isDir(old) && !exists(records) {
		return refuse("the records store is still at %s, where the project-management harness kept it; %s", old,
			LegacyFix())
	}
	return nil
}
