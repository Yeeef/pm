package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
)

const (
	branch = store.Branch
	setup  = store.Setup
)

// Exclude is what pm keeps in the clone's .git/info/exclude: the clone's own state under .pm/ and each worktree's
// records/ link. A branch made before pm has neither .pm/.gitignore nor pm's .gitignore block, and there git add -A
// would stage the store as an embedded repo and the link as a file.
var Exclude = []string{"/.pm/store/", "/.pm/run/", "/records"}

// SparsePatterns is the sparse checkout an earlier pm set in each worktree, everything but records/, to keep the main
// branch's records/ copy from replacing the link; pm init turns it off (OldSparse).
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
func claudeSettings(path, fixHint string) (*object, []any, error) {
	t, err := Read(path)
	if err != nil {
		return nil, nil, err
	}
	data := newObject()
	if t != nil {
		v, err := parseJSON(*t)
		if err != nil {
			return nil, nil, refuse("cannot parse %s: %s; fix it by hand%s", path, err, fixHint)
		}
		o, ok := v.(*object)
		if !ok {
			return nil, nil, refuse("%s: permissions.additionalDirectories is not a list of paths; fix it by hand", path)
		}
		data = o
	}
	bad := refuse("%s: permissions.additionalDirectories is not a list of paths; fix it by hand", path)
	if !data.Has("permissions") {
		return data, nil, nil
	}
	perms, ok := data.Get("permissions").(*object)
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
	perms, ok := data.Get("permissions").(*object)
	if !ok {
		perms = newObject()
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
	perms := data.Get("permissions").(*object) //nolint:errcheck // claudeSettings checked it: dirs came from it
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
	if err != nil {
		return nil, err
	}
	if res.Code != 0 { // sparse checkout on with no sparse-checkout file: no patterns
		return nil, nil
	}
	return strings.Fields(res.Stdout), nil
}

// TracksRecords is whether the worktree's HEAD or index tracks anything under records/: a branch cut from a main
// branch that still carried the records/ copy an earlier pm kept.
func TracksRecords(top string) (bool, error) {
	if gitOK(top, "rev-parse", "--verify", "--quiet", "HEAD") {
		head, err := Git(top, "ls-tree", "--name-only", "HEAD", "records")
		if err != nil || head != "" {
			return head != "", err
		}
	}
	index, err := Git(top, "ls-files", "--", "records")
	return index != "", err
}

// OldSparse is whether the worktree keeps an earlier pm's sparse checkout (SparsePatterns) that can go: while its HEAD
// or index tracks records/, turning sparse checkout off writes that copy over the link, so it stays.
func OldSparse(top string) (bool, error) {
	patterns, err := Sparse(top)
	if err != nil || strings.Join(patterns, "\x00") != strings.Join(SparsePatterns, "\x00") {
		return false, err
	}
	tracks, err := TracksRecords(top)
	return !tracks, err
}

// Unsparse turns the worktree's sparse checkout off, then takes out what it leaves behind: its settings in the
// worktree's config (git sparse-checkout disable leaves core.sparseCheckout false, and git worktree add copies that
// into each new worktree) and its patterns file.
func Unsparse(top string) error {
	if _, err := Git(top, "sparse-checkout", "disable"); err != nil {
		return err
	}
	for _, key := range []string{"core.sparseCheckout", "core.sparseCheckoutCone", "sparse.expectFilesOutsideOfPatterns"} {
		proc.Run([]string{"git", "config", "--worktree", "--unset", key}, proc.Options{Cwd: &top}) // unset already: fine
	}
	file, err := Git(top, "rev-parse", "--path-format=absolute", "--git-path", "info/sparse-checkout")
	if err != nil {
		return err
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// localHooksPath is core.hooksPath as the clone's own config sets it ("" when unset).
func localHooksPath(main string) (string, error) {
	return Git(main, "config", "--local", "--get", "--default=", "core.hooksPath")
}

// pmHooksPath is whether a core.hooksPath value is pm's alone: it names pm's retired hook directory .pm/hooks
// (relative, or under any checkout, as an earlier pm set the main checkout's absolute path), and that directory, read
// from the main checkout, holds no hook of the user's: each file in it is empty, or a shebang, once pm's section is
// out. A user who kept hooks of their own there (an earlier pm's doctor said to) keeps the path, and so their hooks.
func pmHooksPath(main, value string) (bool, error) {
	p := filepath.ToSlash(filepath.Clean(value))
	if value == "" || (p != HooksRel && !strings.HasSuffix(p, "/"+HooksRel)) {
		return false, nil
	}
	dir := value
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(main, dir)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			return false, nil
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return false, err
		}
		if rest := section.ReplaceAllString(string(b), ""); rest != "" && rest != shebang {
			return false, nil
		}
	}
	return true, nil
}

// RetireHooksPath unsets the clone's core.hooksPath where an earlier pm pointed it at its .pm/hooks and nothing but
// pm's sections is there (HooksWhy); one naming another directory, or holding hooks of the user's, stays. "" when
// nothing changed.
func RetireHooksPath(main string) (string, error) {
	current, err := localHooksPath(main)
	if err != nil {
		return "", err
	}
	if pm, err := pmHooksPath(main, current); err != nil || !pm {
		return "", err
	}
	if _, err := Git(main, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		return "", err
	}
	return fmt.Sprintf("unset core.hooksPath=%s, %s", current, HooksWhy), nil
}

// ---------------------------------------------------------------- the clone's and the worktree's setup

// SetupClone makes the clone and this worktree ready, pm init's clone half, which session start runs in each new
// worktree: unset an earlier pm's git hooks path, check out the records store if it is missing, link this worktree's
// records/ to it, and let Codex's sandbox write both stores and commit. pm init then brings the pm service up and has
// it attach the work store (SetupWork), which the service alone opens.
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
	if err := RefuseLegacyStore(main, records); err != nil {
		return "", err
	}
	var out []string
	if said, err := RetireHooksPath(main); err != nil {
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
				return "", refuse("no %s branch here or on %s; fetch it (git fetch %s %s)", branch, remote, remote, branch)
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
	if Resolve(top) == Resolve(records) {
		return "", refuse("%s is the store itself; run %s from a code worktree", top, setup)
	}
	// an earlier pm kept the main branch's records/ copy out of each worktree with a sparse checkout
	if old, err := OldSparse(top); err != nil {
		return "", err
	} else if old {
		if err := Unsparse(top); err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("turned off the sparse checkout an earlier pm set in %s", top))
	}
	link := filepath.Join(top, "records")
	if said, err := setupRecordsLink(top, records); err != nil {
		return "", err
	} else if said != "" {
		out = append(out, said)
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
	if old, err := OldSparse(top); err != nil {
		return nil, err
	} else if old {
		out = append(out, fmt.Sprintf("sparse checkout: %s keeps the one an earlier pm set; run pm init", top))
	}
	if tracks, err := TracksRecords(top); err != nil {
		return nil, err
	} else if tracks {
		out = append(out, fmt.Sprintf("records copy: %s tracks records/, a copy no branch keeps now; untrack it with git "+
			"rm -r -q --cached --sparse --ignore-unmatch records and commit, or merge the main branch once it has", top))
	}
	hp, err := localHooksPath(main)
	if err != nil {
		return nil, err
	}
	if pm, err := pmHooksPath(main, hp); err != nil {
		return nil, err
	} else if pm {
		out = append(out, fmt.Sprintf("hooks path: core.hooksPath is %s, %s; run pm init to unset it", hp, HooksWhy))
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
	if d := service.CodexDrift(main); d != "" {
		out = append(out, "service: "+d)
	}
	if said, err := doctorCodex(main, records); err != nil {
		return nil, err
	} else if said != "" {
		out = append(out, said)
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

func setupRecordsLink(top, records string) (string, error) {
	link := filepath.Join(top, "records")
	switch {
	case isLink(link) && Resolve(link) == Resolve(records):
	case exists(link) || isLink(link):
		if tracks, err := TracksRecords(top); err != nil {
			return "", err
		} else if tracks && !isLink(link) {
			return "", refuse("%s is this branch's tracked copy of records/, not the link to %s; merge the main branch, "+
				"which tracks none, then run %s again", link, records, setup)
		}
		return "", refuse("%s exists and is not a link to %s; move it away and run %s again", link, records, setup)
	default:
		if err := os.Symlink(records, link); err != nil {
			return "", err
		}
		return fmt.Sprintf("linked %s -> %s", link, records), nil
	}
	return "", nil
}

func doctorCodex(main, records string) (string, error) {
	if isDir(CodexHome()) && isDir(records) {
		path := filepath.Join(CodexHome(), "config.toml")
		_, _, have, err := CodexConfig(path)
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
		if len(missing) > 0 {
			return fmt.Sprintf("codex: %s lacks writable_roots %s; run pm init", path, strings.Join(missing, ", ")), nil
		}
	}
	return "", nil
}
