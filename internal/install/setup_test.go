package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/work"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return string(out)
}

// cloneOf is a bare remote with one commit on main and a clone of it.
func cloneOf(t *testing.T, bare, name string) string {
	t.Helper()
	dir := filepath.Dir(bare)
	run(t, dir, "git", "clone", "-q", bare, name)
	return filepath.Join(dir, name)
}

// shortDir is a new directory under a short temp root, removed at the test's end: a clone's service socket must fit
// the kernel's limit, which t.TempDir's paths can pass on macOS.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pm") //nolint:usetesting // short: the clone's service socket must fit the kernel's limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func bareRemote(t *testing.T) string {
	t.Helper()
	dir := shortDir(t)
	bare := filepath.Join(dir, "origin.git")
	run(t, dir, "git", "init", "-q", "--bare", "-b", "main", bare)
	w := cloneOf(t, bare, "seed")
	run(t, w, "git", "commit", "-q", "--allow-empty", "-m", "first")
	run(t, w, "git", "push", "-q", "origin", "main")
	return bare
}

// serve starts the clone's work-store host with the setup operation, as pm service run does, stopped at the test's
// end.
func serve(t *testing.T, main string) {
	t.Helper()
	h, err := work.NewHost(work.HostOptions{Main: main, Version: buildinfo.Version, Ops: work.Ops{
		Setup: func(c context.Context, d *work.Dolt) ([]string, error) { return SetupStore(c, d, main, "origin") }}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
}

// emptyStore makes an empty store in the clone's service, as pm init --import-bd does before its import.
func emptyStore(t *testing.T, main string) {
	t.Helper()
	d, err := work.DialSetup(main)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown()
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
}

// The work store's attach (the work-store page, Storage: Remote): the first clone creates the store and pushes it to
// refs/pm/work, a second clone clones it from there, and a run on a set-up clone changes nothing.
func TestSetupWorkCreatesPushesAndClones(t *testing.T) {
	bare := bareRemote(t)
	first := cloneOf(t, bare, "first")
	serve(t, first)
	said, err := SetupWork(first)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := work.Locations(first)
	if want := "created the work store at " + dir + " and pushed it to origin's refs/pm/work"; len(said) != 1 || said[0] != want {
		t.Fatalf("first clone: %q, want %q", said, want)
	}
	if out := run(t, first, "git", "ls-remote", "origin", work.RemoteRef); !strings.Contains(out, work.RemoteRef) {
		t.Fatalf("the remote has no %s: %q", work.RemoteRef, out)
	}
	if said, err := SetupWork(first); err != nil || len(said) != 0 {
		t.Fatalf("a set-up clone: %q, %v; want nothing done", said, err)
	}
	if state, err := WorkState(first, "origin"); err != nil || state != "0 ahead, 0 behind origin refs/pm/work (as of the last sync)" {
		t.Fatalf("where: %q, %v", state, err)
	}

	second := cloneOf(t, bare, "second")
	serve(t, second)
	said, err = SetupWork(second)
	if err != nil {
		t.Fatal(err)
	}
	dir2, _ := work.Locations(second)
	if want := "cloned the work store from origin's refs/pm/work into " + dir2; len(said) != 1 || said[0] != want {
		t.Fatalf("second clone: %q, want %q", said, want)
	}
	if drift, err := WorkDrift(second, "origin"); err != nil || len(drift) != 0 {
		t.Fatalf("doctor: %q, %v", drift, err)
	}
}

// What pm init refuses rather than fork the project's work: an empty store beside the remote's Beads data, and a store
// made here beside the remote's own, which share no history; pm uninstall refuses to delete what the remote lacks.
func TestSetupWorkRefusesAFork(t *testing.T) {
	beads := cloneOf(t, bareRemote(t), "beads")
	run(t, beads, "git", "push", "-q", "origin", "HEAD:refs/dolt/data")
	serve(t, beads)
	if _, err := SetupWork(beads); err == nil || !strings.Contains(err.Error(), "holds Beads data (refs/dolt/data)") {
		t.Fatalf("got %v, want the import asked for first", err)
	}
	if _, err := work.Dial(beads); !errors.Is(err, work.ErrNoStore) {
		t.Fatalf("a store was made: %v", err)
	}

	bare := bareRemote(t)
	first := cloneOf(t, bare, "first")
	serve(t, first)
	if _, err := SetupWork(first); err != nil {
		t.Fatal(err)
	}
	if why, err := WorkUnsynced(first, "origin"); err != nil || why != "" {
		t.Fatalf("a pushed store: %q, %v", why, err)
	}
	other := cloneOf(t, bare, "other")
	serve(t, other)
	emptyStore(t, other)
	if why, err := WorkUnsynced(other, "origin"); err != nil || !strings.Contains(why, "has no remote") {
		t.Fatalf("a store with no remote: %q, %v", why, err)
	}
	if _, err := SetupWork(other); err == nil || !strings.Contains(err.Error(), "share no history") {
		t.Fatalf("got %v, want the unrelated store refused", err)
	}
	if drift, err := WorkDrift(other, "origin"); err != nil || len(drift) != 1 || !strings.Contains(drift[0], "share no history") {
		t.Fatalf("doctor: %q, %v", drift, err)
	}
	if why, err := WorkUnsynced(other, "origin"); err != nil || !strings.Contains(why, "was never pushed") {
		t.Fatalf("an unpushed store: %q, %v", why, err)
	}
}

// pm in the bin dir: copied there when another binary is, left alone when it is this one.
func TestInstallBinary(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PM_BIN_DIR", bin)
	old := buildinfo.Version
	buildinfo.Version = "9.9.9"
	t.Cleanup(func() { buildinfo.Version = old })
	if err := os.WriteFile(filepath.Join(bin, "pm"), []byte("another pm"), 0o755); err != nil {
		t.Fatal(err)
	}
	said, err := InstallBinary()
	if err != nil || said != "installed pm 9.9.9 at "+filepath.Join(bin, "pm") {
		t.Fatalf("got %q, %v", said, err)
	}
	me, _ := os.Executable()
	a, _ := os.ReadFile(me)
	b, _ := os.ReadFile(filepath.Join(bin, "pm"))
	if string(a) != string(b) {
		t.Fatal("the bin-dir pm is not this binary")
	}
	if st, _ := os.Stat(filepath.Join(bin, "pm")); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}
	os.Remove(filepath.Join(bin, "pm"))
	if err := os.Symlink(me, filepath.Join(bin, "pm")); err != nil {
		t.Fatal(err)
	}
	if said, err := InstallBinary(); err != nil || said != "" {
		t.Fatalf("a link to this pm: %q, %v; want nothing done", said, err)
	}
	// A link in the bin dir, such as the retired pm uv tool's, gives way to the binary; what it pointed at stays.
	t.Run("InstallBinaryReplacesALinkAndLeavesItsTarget", func(t *testing.T) {
		bin, tools := t.TempDir(), t.TempDir()
		t.Setenv("PM_BIN_DIR", bin)
		old := buildinfo.Version
		buildinfo.Version = "9.9.9"
		t.Cleanup(func() { buildinfo.Version = old })
		tool := filepath.Join(tools, "pm/bin/pm")
		if err := os.MkdirAll(filepath.Dir(tool), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tool, filepath.Join(bin, "pm")); err != nil {
			t.Fatal(err)
		}
		said, err := InstallBinary()
		want := "installed pm 9.9.9 at " + filepath.Join(bin, "pm")
		if err != nil || said != want {
			t.Fatalf("got %q, %v", said, err)
		}
		if st, err := os.Lstat(filepath.Join(bin, "pm")); err != nil || !st.Mode().IsRegular() {
			t.Fatalf("the bin-dir pm is not a file: %v", err)
		}
		if b, err := os.ReadFile(tool); err != nil || string(b) != "#!/bin/sh\n" {
			t.Fatalf("the link's target changed: %q, %v", b, err)
		}
	})
}

// The clone's exclude lines: added once after the user's own, then removed byte for byte.
func TestExcludeRoundTrip(t *testing.T) {
	main := t.TempDir()
	path := ExcludePath(main)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("# git's\n*.swp"), 0o644)
	if said, err := SetupExclude(main); err != nil || !strings.HasPrefix(said, "added /.pm/store/, /.pm/run/, /records to ") {
		t.Fatalf("%q, %v", said, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "# git's\n*.swp\n/.pm/store/\n/.pm/run/\n/records\n" {
		t.Fatalf("%q", b)
	}
	if said, _ := SetupExclude(main); said != "" {
		t.Fatalf("a second run: %q", said)
	}
	if _, err := RemoveExclude(main); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "# git's\n*.swp\n" {
		t.Fatalf("%q", b)
	}
}

// pieceAt is the piece at rel.
func pieceAt(t *testing.T, rel string) Piece {
	t.Helper()
	for _, p := range Pieces(Settings{"origin", "main", 8000, "", "", "", "", nil}) {
		if p.Rel == rel {
			return p
		}
	}
	t.Fatalf("no piece %s", rel)
	return Piece{}
}

// applied is text with pm's part as this version writes it.
func applied(t *testing.T, rel, text string) string {
	t.Helper()
	out, err := pieceAt(t, rel).Apply(&text)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeTree(t *testing.T, top string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(top, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A repo as Python pm 0.1.x leaves it with Beads (the work-store page, Cut-over): bd's hook entries beside pm's, the
// Beads block in CLAUDE.md (AGENTS.md a link to it), pm's sections in .beads/hooks. pm upgrade's rewrite takes Beads'
// pieces out, keeps pm's entries and everything else byte for byte, writes pm's own hook files, and leaves .beads/
// alone; then pm doctor's drift is empty and a second rewrite plans nothing.
func TestRewriteTakesOutBeadsPieces(t *testing.T) {
	s := Settings{"origin", "main", 8000, "", "", "", "", nil}
	top := t.TempDir()
	// an event Beads' removal empties keeps its place where pm writes its own group
	userClaude := "{\n  \"hooks\": {\n    \"SessionStart\": [],\n    \"PreToolUse\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"./lint.sh\",\n            \"type\": \"command\"\n          }\n        ],\n" +
		"        \"matcher\": \"Bash\"\n      }\n    ]\n  },\n  \"model\": \"x\"\n}\n"
	bdClaude := "{\n  \"hooks\": {\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"bd prime --hook-json\",\n            \"type\": \"command\"\n          }\n        ],\n" +
		"        \"matcher\": \"\"\n      }\n    ],\n    \"PreToolUse\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"./lint.sh\",\n            \"type\": \"command\"\n          }\n        ],\n" +
		"        \"matcher\": \"Bash\"\n      }\n    ]\n  },\n  \"model\": \"x\"\n}\n"
	userCodex := "{\n  \"hooks\": {\n    \"SessionStart\": [],\n    \"UserPromptSubmit\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"./mine.sh\",\n            \"type\": \"command\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	bdCodex := "{\n  \"hooks\": {\n    \"PostCompact\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"bd codex-hook PostCompact\",\n            \"type\": \"command\"\n          }\n        ]\n" +
		"      }\n    ],\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n" +
		"            \"command\": \"bd codex-hook SessionStart\",\n            \"type\": \"command\"\n          }\n        ],\n" +
		"        \"matcher\": \"startup|resume|clear\"\n      }\n    ],\n    \"UserPromptSubmit\": [\n      {\n        \"hooks\": [\n" +
		"          {\n            \"command\": \"bd codex-hook UserPromptSubmit\",\n            \"type\": \"command\"\n          },\n" +
		"          {\n            \"command\": \"./mine.sh\",\n            \"type\": \"command\"\n          }\n        ]\n      }\n" +
		"    ]\n  }\n}\n"
	block := "<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->\n## Beads Issue Tracker\n\nUse bd.\n" +
		"<!-- END BEADS INTEGRATION -->\n"
	beadsHook := "#!/usr/bin/env sh\n# --- BEGIN BEADS INTEGRATION v1.3.1 ---\n# beads' part\n# --- END BEADS INTEGRATION v1.3.1 ---\n"
	pmSection := func(name string) string {
		return "# --- BEGIN PM v0.1.4 ---\npm hook git-" + name + " \"$@\" || exit $?\n# --- END PM ---\n"
	}
	files := map[string]string{
		".claude/settings.json":      applied(t, ".claude/settings.json", bdClaude),
		".codex/hooks.json":          applied(t, ".codex/hooks.json", bdCodex),
		"CLAUDE.md":                  "# Repo\n\nIntro.\n\n" + block,
		".beads/hooks/pre-commit":    beadsHook + pmSection("pre-commit"),
		".beads/hooks/post-checkout": beadsHook + pmSection("post-checkout"),
	}
	writeTree(t, top, files)
	if err := os.Symlink("CLAUDE.md", filepath.Join(top, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	for _, p := range Pieces(s) { // the rest of pm's pieces as Python pm wrote them
		if _, ok := files[p.Rel]; !ok {
			text, _ := p.Apply(nil)
			writeTree(t, top, map[string]string{p.Rel: text})
		}
	}

	drift, err := Drift(top, s)
	if err != nil {
		t.Fatal(err)
	}
	v := buildinfo.Version
	wantDrift := []string{
		".claude/settings.json: holds Beads' hook entries (bd prime --hook-json), which pm " + v + " removes",
		".codex/hooks.json: holds Beads' hook entries (bd codex-hook PostCompact, bd codex-hook SessionStart, " +
			"bd codex-hook UserPromptSubmit), which pm " + v + " removes",
		"CLAUDE.md: holds the Beads block (<!-- BEGIN BEADS INTEGRATION … -->), which pm " + v + " removes",
	}
	if strings.Join(drift, "\n") != strings.Join(wantDrift, "\n") {
		t.Fatalf("drift:\n%s\nwant:\n%s", strings.Join(drift, "\n"), strings.Join(wantDrift, "\n"))
	}

	planned, err := Rewrite(top, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(planned); err != nil {
		t.Fatal(err)
	}
	wantSaid := []string{
		"removed Beads' hook entries (bd prime --hook-json) from .claude/settings.json",
		"removed Beads' hook entries (bd codex-hook PostCompact, bd codex-hook SessionStart, bd codex-hook " +
			"UserPromptSubmit) from .codex/hooks.json",
		"removed the Beads block (<!-- BEGIN BEADS INTEGRATION … -->) from CLAUDE.md",
	}
	if got := Said(planned); strings.Join(got, "\n") != strings.Join(wantSaid, "\n") {
		t.Fatalf("said:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantSaid, "\n"))
	}
	want := map[string]string{
		".claude/settings.json":      applied(t, ".claude/settings.json", userClaude),
		".codex/hooks.json":          applied(t, ".codex/hooks.json", userCodex),
		"CLAUDE.md":                  "# Repo\n\nIntro.\n",
		".beads/hooks/pre-commit":    files[".beads/hooks/pre-commit"],
		".beads/hooks/post-checkout": files[".beads/hooks/post-checkout"],
	}
	for rel, text := range want {
		if b, err := os.ReadFile(filepath.Join(top, rel)); err != nil || string(b) != text {
			t.Errorf("%s:\n%s\nwant:\n%s", rel, b, text)
		}
	}
	if _, err := os.Stat(filepath.Join(top, HooksRel)); !os.IsNotExist(err) {
		t.Errorf("%s: %v; want none, as pm installs no git hook", HooksRel, err)
	}
	if !isLink(filepath.Join(top, "AGENTS.md")) {
		t.Error("AGENTS.md is no longer the link it was")
	}
	if drift, err := Drift(top, s); err != nil || len(drift) != 0 {
		t.Fatalf("drift after the rewrite: %q, %v", drift, err)
	}
	if planned, err := Rewrite(top, s); err != nil || len(planned) != 0 {
		t.Fatalf("a second rewrite: %v, %v; want nothing", Said(planned), err)
	}
	if planned, err := Plan(top, s); err != nil || len(planned) != 0 {
		t.Fatalf("pm init's plan after the rewrite: %v, %v; want nothing", Said(planned), err)
	}
}

// core.hooksPath at an earlier pm's .pm/hooks, relative or absolute, is unset once, and pm doctor names it until then;
// one naming another directory, Beads' .beads/hooks or another hook manager's, is not pm's and stays, and so does one
// whose .pm/hooks holds a hook of the user's.
func TestRetireHooksPath(t *testing.T) {
	main := t.TempDir()
	run(t, main, "git", "init", "-q")
	records := filepath.Join(main, ".pm", "store", "records")
	doctor := func() []string {
		t.Helper()
		var lines []string
		drift, err := DoctorSetup(main, main, records, "origin", 8000)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range drift {
			if strings.HasPrefix(l, "hooks path: ") {
				lines = append(lines, l)
			}
		}
		return lines
	}
	for _, hp := range []string{filepath.Join(main, ".pm", "hooks"), ".pm/hooks", "/elsewhere/clone/.pm/hooks/"} {
		run(t, main, "git", "config", "core.hooksPath", hp)
		want := "hooks path: core.hooksPath is " + hp + ", " + HooksWhy + "; run pm init to unset it"
		if got := doctor(); len(got) != 1 || got[0] != want {
			t.Errorf("doctor on %s: %q, want %q", hp, got, want)
		}
		said, err := RetireHooksPath(main)
		if want := "unset core.hooksPath=" + hp + ", " + HooksWhy; err != nil || said != want {
			t.Fatalf("%q, %v; want %q", said, err, want)
		}
		if got, err := GitConfig(main, "core.hooksPath"); err != nil || got != "" {
			t.Fatalf("core.hooksPath %q after the unset", got)
		}
		if said, err := RetireHooksPath(main); err != nil || said != "" {
			t.Fatalf("a second run: %q, %v", said, err)
		}
		if got := doctor(); len(got) != 0 {
			t.Errorf("doctor after the unset: %q", got)
		}
	}
	hooks := filepath.Join(main, ".pm", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	old := shebang + "# --- BEGIN PM v0.9.1 ---\npm hook git-post-checkout \"$@\" || exit $?\n# --- END PM ---\n"
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, main, "git", "config", "core.hooksPath", ".pm/hooks")
	if said, err := RetireHooksPath(main); err != nil || said == "" {
		t.Fatalf("pm's section alone: %q, %v; want the path unset", said, err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(old+"echo mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, hp := range []string{filepath.Join(main, ".beads", "hooks"), ".husky", "/x/.pm/hooks-old", ".pm/hooks"} {
		run(t, main, "git", "config", "core.hooksPath", hp)
		if said, err := RetireHooksPath(main); err != nil || said != "" {
			t.Fatalf("%s: %q, %v; want it left", hp, said, err)
		}
		if got, err := GitConfig(main, "core.hooksPath"); err != nil || got != hp {
			t.Fatalf("core.hooksPath %q, want %q left", got, hp)
		}
		if got := doctor(); len(got) != 0 {
			t.Errorf("doctor on %s: %q", hp, got)
		}
	}
}

// pm upgrade's rewrite takes an earlier pm's hook files out, and .pm/hooks with them once nothing else is left there; a
// file of the user's keeps the directory.
func TestWriteTakesAnEmptyHooksDirectoryAway(t *testing.T) {
	s := Settings{"origin", "main", 8000, "", "", "", "", nil}
	old := func(name string) string {
		return shebang + "# --- BEGIN PM v0.9.1 ---\npm hook git-" + name + " \"$@\" || exit $?\n# --- END PM ---\n"
	}
	for _, mine := range []bool{false, true} {
		top := t.TempDir()
		files := map[string]string{HooksRel + "/post-checkout": old("post-checkout"), HooksRel + "/pre-commit": old("pre-commit")}
		if mine {
			files[HooksRel+"/pre-push"] = "#!/bin/sh\necho mine\n"
		}
		writeTree(t, top, files)
		planned, err := Rewrite(top, s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Write(planned); err != nil {
			t.Fatal(err)
		}
		left, err := os.ReadDir(filepath.Join(top, HooksRel))
		switch {
		case !mine && !os.IsNotExist(err):
			t.Errorf("%s: %v, %v; want it gone", HooksRel, left, err)
		case mine && (err != nil || len(left) != 1 || left[0].Name() != "pre-push"):
			t.Errorf("%s: %v, %v; want the user's pre-push alone", HooksRel, left, err)
		}
	}
}

// A repo that retired Beads by replacing .beads/ with a file (which blocks every bd command) holds no Beads pieces, as
// a repo without .beads/: the legacy check, pm doctor's drift and pm upgrade's rewrite read past it. Any other error
// reading under .beads/ still fails.
func TestABeadsFileIsNoBeads(t *testing.T) {
	s := Settings{"origin", "main", 8000, "", "", "", "", nil}
	top := t.TempDir()
	if err := os.WriteFile(filepath.Join(top, ".beads"), []byte("Beads is retired here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if found, err := LegacyRepo(top); err != nil || len(found) != 0 {
		t.Fatalf("legacy: %q, %v; want nothing", found, err)
	}
	planned, err := Rewrite(top, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(planned); err != nil {
		t.Fatal(err)
	}
	if drift, err := Drift(top, s); err != nil || len(drift) != 0 {
		t.Fatalf("drift: %q, %v; want nothing", drift, err)
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads a directory without permissions")
	}
	locked := filepath.Join(t.TempDir(), ".beads", "hooks")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	if _, err := LegacyRepo(filepath.Dir(filepath.Dir(locked))); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("legacy under an unreadable .beads/hooks: %v; want permission denied", err)
	}
}
