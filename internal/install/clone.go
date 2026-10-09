package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
	"github.com/Yeeef/yeeef-agents/pm/internal/pyjson"
	"github.com/Yeeef/yeeef-agents/pm/internal/service"
	"github.com/Yeeef/yeeef-agents/pm/internal/store"
)

const (
	branch = store.Branch
	setup  = store.Setup
)

// Exclude is what pm keeps in the clone's .git/info/exclude: the clone's own state under .pm/ and each worktree's
// records/ link. A branch made before pm has neither .pm/.gitignore nor pm's .gitignore block, and there git add -A
// would stage the store as an embedded repo and the link as a file.
var Exclude = []string{"/.pm/store/", "/.pm/run/", "/records"}

// SparsePatterns is what pm sets as a worktree's sparse checkout: everything but records/.
var SparsePatterns = []string{"/*", "!/records/"}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func isDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

func isLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

// splitLines is Python's str.splitlines() for the line breaks a text file holds.
func splitLines(s string, keepEnds bool) []string {
	var out []string
	for s != "" {
		i := strings.IndexAny(s, "\r\n")
		if i < 0 {
			out = append(out, s)
			break
		}
		end := i + 1
		if s[i] == '\r' && end < len(s) && s[end] == '\n' {
			end++
		}
		if keepEnds {
			out = append(out, s[:end])
		} else {
			out = append(out, s[:i])
		}
		s = s[end:]
	}
	return out
}

// ExcludePath is the clone's info/exclude: the main checkout's .git, which store.PathOf checked is the common dir.
func ExcludePath(main string) string { return filepath.Join(main, ".git", "info", "exclude") }

func excludeLines(path string) ([]string, error) {
	t, err := Read(path)
	if err != nil || t == nil {
		return nil, err
	}
	return splitLines(*t, false), nil
}

// ExcludeMissing is the Exclude lines the clone's info/exclude lacks.
func ExcludeMissing(main string) ([]string, error) {
	have, err := excludeLines(ExcludePath(main))
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, l := range Exclude {
		if !contains(have, l) {
			missing = append(missing, l)
		}
	}
	return missing, nil
}

// SetupExclude appends the Exclude lines missing from the clone's info/exclude, keeping the rest byte for byte.
func SetupExclude(main string) (string, error) {
	missing, err := ExcludeMissing(main)
	if err != nil || len(missing) == 0 {
		return "", err
	}
	path := ExcludePath(main)
	t, err := Read(path)
	if err != nil {
		return "", err
	}
	text := ""
	if t != nil {
		text = *t
	}
	sep := ""
	if text != "" && !strings.HasSuffix(text, "\n") {
		sep = "\n"
	}
	if err := store.WriteAtomic(path, text+sep+strings.Join(missing, "\n")+"\n"); err != nil {
		return "", err
	}
	return fmt.Sprintf("added %s to %s, so no branch stages the clone's pm state", strings.Join(missing, ", "), path), nil
}

// RemoveExclude takes the Exclude lines out of the clone's info/exclude, keeping every other byte.
func RemoveExclude(main string) (string, error) {
	path := ExcludePath(main)
	t, err := Read(path)
	if err != nil || t == nil {
		return "", err
	}
	lines := splitLines(*t, true)
	var kept []string
	for _, l := range lines {
		if !contains(Exclude, strings.TrimRight(l, "\r\n")) {
			kept = append(kept, l)
		}
	}
	if len(kept) == len(lines) {
		return "", nil
	}
	have := splitLines(*t, false)
	var gone []string
	for _, l := range Exclude {
		if contains(have, l) {
			gone = append(gone, l)
		}
	}
	if err := store.WriteAtomic(path, strings.Join(kept, "")); err != nil {
		return "", err
	}
	return fmt.Sprintf("removed %s from %s", strings.Join(gone, ", "), path), nil
}

// ---------------------------------------------------------------- Claude Code's per-worktree settings

// claudeConfigDir is $CLAUDE_CONFIG_DIR, else ~/.claude: whether Claude Code is used on this machine.
func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".claude")
}

// claudeSettings is .claude/settings.local.json's data ({} when absent) and its permissions.additionalDirectories
// (nil when it has none); fixHint ends a parse refusal.
func claudeSettings(path, fixHint string) (*pyjson.Object, []any, error) {
	t, err := Read(path)
	if err != nil {
		return nil, nil, err
	}
	data := pyjson.NewObject()
	if t != nil {
		v, err := pyjson.Loads(*t)
		if err != nil {
			return nil, nil, refuse("cannot parse %s: %s; fix it by hand%s", path, err, fixHint)
		}
		o, ok := v.(*pyjson.Object)
		if !ok {
			return nil, nil, refuse("%s: permissions.additionalDirectories is not a list of paths; fix it by hand", path)
		}
		data = o
	}
	bad := refuse("%s: permissions.additionalDirectories is not a list of paths; fix it by hand", path)
	if !data.Has("permissions") {
		return data, nil, nil
	}
	perms, ok := data.Get("permissions").(*pyjson.Object)
	if !ok {
		return nil, nil, bad
	}
	if !perms.Has("additionalDirectories") {
		return data, nil, nil
	}
	dirs, ok := perms.Get("additionalDirectories").([]any)
	if !ok {
		return nil, nil, bad
	}
	return data, dirs, nil
}

func listHas(list []any, s string) bool {
	for _, v := range list {
		if x, ok := v.(string); ok && x == s {
			return true
		}
	}
	return false
}

// ClaudeListsStore is whether the worktree's .claude/settings.local.json lists the store; refused when unreadable.
func ClaudeListsStore(top, records string) (bool, error) {
	_, dirs, err := claudeSettings(filepath.Join(top, ".claude", "settings.local.json"), "")
	return listHas(dirs, records), err
}

// SetupClaude adds the store to permissions.additionalDirectories in this worktree's .claude/settings.local.json,
// keeping the rest of the file's data: records/ resolves outside the worktree, so without it Claude Code asks before
// each write through the link. The path is absolute and per machine, so it goes in the local file, not settings.json.
// Without a Claude Code config dir it does nothing.
func SetupClaude(top, records string) (string, error) {
	if !isDir(claudeConfigDir()) {
		return "", nil
	}
	path := filepath.Join(top, ".claude", "settings.local.json")
	data, dirs, err := claudeSettings(path, " and run "+setup+" again")
	if err != nil {
		return "", err
	}
	if listHas(dirs, records) {
		return "", nil
	}
	perms, _ := data.Get("permissions").(*pyjson.Object)
	if perms == nil {
		perms = pyjson.NewObject()
		data.Set("permissions", perms)
	}
	perms.Set("additionalDirectories", append(dirs, records))
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return "", err
	}
	if err := store.WriteAtomic(path, DumpJSON(data)); err != nil {
		return "", err
	}
	return fmt.Sprintf("added %s to permissions.additionalDirectories in %s, so Claude Code writes records through "+
		"records/ without asking", records, path), nil
}

// RemoveClaude takes the store out of this worktree's .claude/settings.local.json; the file goes when nothing else is
// left.
func RemoveClaude(top, records string) (string, error) {
	path := filepath.Join(top, ".claude", "settings.local.json")
	data, dirs, err := claudeSettings(path, "")
	if err != nil || !listHas(dirs, records) {
		return "", err
	}
	var kept []any
	removed := false
	for _, d := range dirs {
		if s, ok := d.(string); ok && s == records && !removed {
			removed = true
			continue
		}
		kept = append(kept, d)
	}
	perms := data.Get("permissions").(*pyjson.Object)
	if len(kept) == 0 {
		perms.Delete("additionalDirectories")
		if len(perms.Keys) == 0 {
			data.Delete("permissions")
		}
	} else {
		perms.Set("additionalDirectories", kept)
	}
	if len(data.Keys) > 0 {
		if err := store.WriteAtomic(path, DumpJSON(data)); err != nil {
			return "", err
		}
	} else {
		if err := os.Remove(path); err != nil {
			return "", err
		}
		os.Remove(filepath.Dir(path)) // only when empty
	}
	return fmt.Sprintf("removed %s from permissions.additionalDirectories in %s", records, path), nil
}

// ---------------------------------------------------------------- the worktrees, the sparse checkout, the hooks path

// Worktrees is the clone's worktrees whose directory exists: one deleted without git worktree remove is still listed
// (prunable), and git cannot run in it.
func Worktrees(main string) ([]string, error) {
	out, err := Git(main, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var trees []string
	for _, block := range strings.Split(out, "\n\n") {
		lines := splitLines(block, false)
		if len(lines) == 0 || !strings.HasPrefix(lines[0], "worktree ") {
			continue
		}
		prunable := false
		for _, l := range lines {
			if strings.HasPrefix(l, "prunable") {
				prunable = true
			}
		}
		tree := strings.TrimPrefix(lines[0], "worktree ")
		if !prunable && isDir(tree) {
			trees = append(trees, tree)
		}
	}
	return trees, nil
}

// Sparse is the worktree's sparse-checkout patterns, none when sparse checkout is off.
func Sparse(top string) ([]string, error) {
	on, err := GitConfig(top, "core.sparseCheckout")
	if err != nil || on != "true" {
		return nil, err
	}
	res, err := proc.Run([]string{"git", "sparse-checkout", "list"}, proc.Options{Cwd: &top})
	if err != nil || res.Code != 0 {
		return nil, nil
	}
	return strings.Fields(res.Stdout), nil
}

// HooksDir is where the clone's git hooks live: pm's own .pm/hooks in the main checkout.
func HooksDir(main string) string { return filepath.Join(main, filepath.FromSlash(HooksRel)) }

// beadsHooksDir is Beads' hook directory, where Python pm 0.1.x pointed core.hooksPath; pm init and pm upgrade move it.
func beadsHooksDir(dir string) string { return filepath.Join(dir, ".beads", "hooks") }

// hooksPathIs is whether core.hooksPath, as set in top, names dir under this worktree or the main checkout.
func hooksPathIs(top, main, current string, dir func(string) string) bool {
	at := Resolve(join(top, current))
	return at == Resolve(dir(top)) || at == Resolve(dir(main))
}

// CheckHooksPath refuses a core.hooksPath other than .pm/hooks (relative, or this worktree's or the main checkout's
// absolute path); unset is fine, as pm init sets it, and so is Beads' .beads/hooks, which pm init moves to .pm/hooks.
func CheckHooksPath(top, main string) error {
	current, err := GitConfig(top, "core.hooksPath")
	if err != nil || current == "" {
		return err
	}
	if !hooksPathIs(top, main, current, HooksDir) && !hooksPathIs(top, main, current, beadsHooksDir) {
		return refuse("core.hooksPath is %s, not .pm/hooks; pm's git hooks live in its own hook files, so pm init "+
			"works only with pm's hooks path (other hook managers are not supported)", current)
	}
	return nil
}

// join is Python's Path / p: p itself when it is absolute.
func join(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// hooksReady is whether the main checkout holds pm's hook files in .pm/hooks. core.hooksPath is the clone's, so it
// moves there only then: a pin moved in another worktree, or a worktree pinned to Go pm while main still pins Python
// pm, would otherwise point every worktree at hooks that are not there, and git would run none.
func hooksReady(main string) bool {
	for _, name := range GitHooks {
		if !executable(filepath.Join(HooksDir(main), name)) {
			return false
		}
	}
	return true
}

func executable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

// SetupHooksPath points core.hooksPath at the main checkout's .pm/hooks when it is unset, names another checkout's, or
// names Beads' .beads/hooks, where Python pm 0.1.x had bd hooks install point it; the hook files there stay. It leaves
// Beads' path, whose files hold pm's section too, until the main checkout holds pm's hook files (hooksReady).
func SetupHooksPath(main string) (string, error) {
	current, err := GitConfig(main, "core.hooksPath")
	if err != nil {
		return "", err
	}
	hooks := HooksDir(main)
	if current != "" && Resolve(join(main, current)) == Resolve(hooks) {
		return "", nil
	}
	if current != "" && hooksPathIs(main, main, current, beadsHooksDir) && !hooksReady(main) {
		return "", nil // Beads' hooks run pm's section until main has its own
	}
	if _, err := Git(main, "config", "core.hooksPath", hooks); err != nil {
		return "", err
	}
	if current != "" && hooksPathIs(main, main, current, beadsHooksDir) {
		return fmt.Sprintf("moved the git hooks off Beads' %s: core.hooksPath=%s", current, hooks), nil
	}
	return "installed the git hooks: core.hooksPath=" + hooks, nil
}

// ---------------------------------------------------------------- the clone's and the worktree's setup

// SetupClone makes the clone and this worktree ready, the part of pm init's clone half that the post-checkout hook
// runs too: attach the work store, set the git hooks path, check out the records store if it is missing, link this
// worktree's records/ to it, and let Codex's sandbox write both stores and commit. pm init then installs the pm
// service.
func SetupClone(cwd, remote string) (string, error) {
	records, err := store.PathOf(cwd)
	if err != nil {
		return "", err
	}
	main := store.MainOf(records)
	top, err := Git(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if err := CheckHooksPath(top, main); err != nil {
		return "", err
	}
	if err := RefuseLegacyStore(main, records); err != nil {
		return "", err
	}
	var out []string
	if said, err := SetupHooksPath(main); err != nil {
		return "", err
	} else if said != "" {
		out = append(out, said)
	}
	if said, err := SetupExclude(main); err != nil {
		return "", err
	} else if said != "" {
		out = append(out, said)
	}
	if !exists(records) {
		if !gitOK(cwd, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) {
			if !gitOK(cwd, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+branch) {
				return "", refuse("no %s branch here or on %s; fetch it, or create it once with git subtree split "+
					"--prefix=records -b %s", branch, remote, branch)
			}
			if _, err := Git(cwd, "branch", "--track", branch, remote+"/"+branch); err != nil {
				return "", err
			}
		}
		if _, err := Git(cwd, "worktree", "add", "--quiet", records, branch); err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("checked out branch %s at %s", branch, records))
	}
	if _, err := store.Find(cwd); err != nil {
		return "", err
	}
	// after the records store, which pm init --import-bd reads when a refusal here asks for the import first
	work, err := SetupWork(main, remote)
	if err != nil {
		return "", err
	}
	out = append(out, work...)
	if Resolve(top) == Resolve(records) {
		return "", refuse("%s is the store itself; run %s from a code worktree", top, setup)
	}
	// main's copy of records/ is tracked on any branch made from it, now or after a later pull; keep it out of this
	// worktree always, or git replaces the ignored link with the copy the moment the branch tracks it.
	if on, err := GitConfig(top, "core.sparseCheckout"); err != nil {
		return "", err
	} else if on != "true" {
		if _, err := Git(top, append([]string{"sparse-checkout", "set", "--no-cone"}, SparsePatterns...)...); err != nil {
			return "", err
		}
		if _, err := Git(top, "config", "--worktree", "sparse.expectFilesOutsideOfPatterns", "true"); err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("excluded records/ from %s with sparse checkout", top))
	}
	link := filepath.Join(top, "records")
	switch {
	case isLink(link) && Resolve(link) == Resolve(records):
	case exists(link) || isLink(link):
		return "", refuse("%s exists and is not a link to %s; move it away and run %s again", link, records, setup)
	default:
		if err := os.Symlink(records, link); err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("linked %s -> %s", link, records))
	}
	if said, err := SetupClaude(top, records); err != nil {
		return "", err
	} else if said != "" {
		out = append(out, said)
	}
	done := strings.Join(out, "\n")
	if done == "" {
		done = fmt.Sprintf("already set up: %s -> %s", link, records)
	}
	codex, err := SetupCodex(main)
	if err != nil {
		return "", err
	}
	if codex != "" {
		return done + "\n" + codex, nil
	}
	return done, nil
}

// DoctorSetup is how the clone and this worktree differ from what pm init makes, one line each; port is the site port
// the service must serve on.
func DoctorSetup(top, main, records, remote string, port int) ([]string, error) {
	var out []string
	if _, err := store.Find(top); err != nil {
		out = append(out, "store: "+err.Error())
	}
	link := filepath.Join(top, "records")
	if !(isLink(link) && Resolve(link) == Resolve(records)) {
		out = append(out, fmt.Sprintf("records link: %s is not a link to %s; run pm init", link, records))
	}
	patterns, err := Sparse(top)
	if err != nil {
		return nil, err
	}
	if !contains(patterns, "!/records/") {
		out = append(out, fmt.Sprintf("sparse checkout: %s does not exclude records/; run pm init", top))
	}
	hp, err := GitConfig(main, "core.hooksPath")
	if err != nil {
		return nil, err
	}
	if hp == "" || Resolve(join(main, hp)) != Resolve(HooksDir(main)) {
		fix, shown := "run pm init", "unset"
		switch {
		case hp == "":
		case hooksPathIs(main, main, hp, beadsHooksDir):
			shown = hp + " (Beads' hooks)"
			fix = "run pm upgrade --to " + buildinfo.Version + " to move it"
			if !hooksReady(main) && Resolve(top) != Resolve(main) { // pm upgrade in main writes them, then moves it
				fix = "the main checkout " + main + " has no pm hooks in .pm/hooks yet; once it pins Go pm (merge the " +
					"pin, then pull main there), run pm init there to move it"
			}
		default:
			shown = hp
			fix = "pm works only with its own hooks path: move any hooks there into .pm/hooks (outside pm's marked " +
				"sections), run git config --unset core.hooksPath, then pm init"
		}
		out = append(out, fmt.Sprintf("hooks path: core.hooksPath is %s, not .pm/hooks; %s", shown, fix))
	}
	work, err := WorkDrift(main, remote)
	if err != nil {
		return nil, err
	}
	for _, d := range work {
		out = append(out, "work store: "+d)
	}
	for _, d := range service.Drift(main, port) {
		out = append(out, "service: "+d)
	}
	if isDir(CodexHome()) && isDir(records) {
		path := filepath.Join(CodexHome(), "config.toml")
		_, _, have, err := CodexConfig(path)
		if err != nil {
			return nil, err
		}
		roots, err := CodexRoots(main)
		if err != nil {
			return nil, err
		}
		var missing []string
		for _, r := range roots {
			if !contains(have, r) {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("codex: %s lacks writable_roots %s; run pm init", path, strings.Join(missing, ", ")))
		}
	}
	missing, err := ExcludeMissing(main)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("git exclude: %s lacks %s; run pm init", ExcludePath(main), strings.Join(missing, ", ")))
	}
	if isDir(claudeConfigDir()) {
		path := filepath.Join(top, ".claude", "settings.local.json")
		listed, err := ClaudeListsStore(top, records)
		if err != nil {
			return nil, err
		}
		if !listed {
			out = append(out, fmt.Sprintf("claude: %s does not list %s in permissions.additionalDirectories; run pm init",
				path, records))
		}
	}
	return out, nil
}
