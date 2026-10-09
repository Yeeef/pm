package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
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

func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "origin.git")
	run(t, dir, "git", "init", "-q", "--bare", "-b", "main", bare)
	w := cloneOf(t, bare, "seed")
	run(t, w, "git", "commit", "-q", "--allow-empty", "-m", "first")
	run(t, w, "git", "push", "-q", "origin", "main")
	return bare
}

// The work store's attach (the work-store page, Storage: Remote): the first clone creates the store and pushes it to
// refs/pm/work, a second clone clones it from there, and a run on a set-up clone changes nothing.
func TestSetupWorkCreatesPushesAndClones(t *testing.T) {
	bare := bareRemote(t)
	first := cloneOf(t, bare, "first")
	said, err := SetupWork(first, "origin")
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
	if said, err := SetupWork(first, "origin"); err != nil || len(said) != 0 {
		t.Fatalf("a set-up clone: %q, %v; want nothing done", said, err)
	}
	if state, err := WorkState(first, "origin"); err != nil || state != "0 ahead, 0 behind origin refs/pm/work (as of the last sync)" {
		t.Fatalf("where: %q, %v", state, err)
	}

	second := cloneOf(t, bare, "second")
	said, err = SetupWork(second, "origin")
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

// A store made before the clone had a remote (pm init --import-bd, say) is pointed at it and pushed.
func TestSetupWorkAttachesAnExistingStore(t *testing.T) {
	bare := bareRemote(t)
	c := cloneOf(t, bare, "c")
	dir, run := work.Locations(c)
	d, err := work.CreateStore(work.Options{Dir: dir, RunDir: run})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if drift, err := WorkDrift(c, "origin"); err != nil || len(drift) != 1 || !strings.Contains(drift[0], "has no remote") {
		t.Fatalf("doctor before: %q, %v", drift, err)
	}
	said, err := SetupWork(c, "origin")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pointed the work store " + dir + " at origin's refs/pm/work", "pushed the work store to origin's refs/pm/work"}
	if strings.Join(said, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", said, want)
	}
}

// What pm init refuses rather than fork the project's work: an empty store beside the remote's Beads data, and a store
// made here beside the remote's own, which share no history; pm uninstall refuses to delete what the remote lacks.
func TestSetupWorkRefusesAFork(t *testing.T) {
	beads := cloneOf(t, bareRemote(t), "beads")
	run(t, beads, "git", "push", "-q", "origin", "HEAD:refs/dolt/data")
	if _, err := SetupWork(beads, "origin"); err == nil || !strings.Contains(err.Error(), "holds Beads data (refs/dolt/data)") {
		t.Fatalf("got %v, want the import asked for first", err)
	}
	if dir, _ := work.Locations(beads); work.Exists(dir) {
		t.Fatal("a store was made")
	}

	bare := bareRemote(t)
	first := cloneOf(t, bare, "first")
	if _, err := SetupWork(first, "origin"); err != nil {
		t.Fatal(err)
	}
	if why, err := WorkUnsynced(first, "origin"); err != nil || why != "" {
		t.Fatalf("a pushed store: %q, %v", why, err)
	}
	other := cloneOf(t, bare, "other")
	dir, runDir := work.Locations(other)
	d, err := work.CreateStore(work.Options{Dir: dir, RunDir: runDir})
	if err != nil {
		t.Fatal(err)
	}
	d.Shutdown()
	if why, err := WorkUnsynced(other, "origin"); err != nil || !strings.Contains(why, "has no remote") {
		t.Fatalf("a store with no remote: %q, %v", why, err)
	}
	if _, err := SetupWork(other, "origin"); err == nil || !strings.Contains(err.Error(), "share no history") {
		t.Fatalf("got %v, want the unrelated store refused", err)
	}
	if drift, err := WorkDrift(other, "origin"); err != nil || len(drift) != 1 || !strings.Contains(drift[0], "share no history") {
		t.Fatalf("doctor: %q, %v", drift, err)
	}
	if why, err := WorkUnsynced(other, "origin"); err != nil || !strings.Contains(why, "was never pushed") {
		t.Fatalf("an unpushed store: %q, %v", why, err)
	}
}

// A repo without the remote cannot get a store, and one with a store but no remote keeps it as it is.
func TestSetupWorkWithoutTheRemote(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	if _, err := SetupWork(dir, "origin"); err == nil || !strings.Contains(err.Error(), "has no remote origin") {
		t.Fatalf("got %v, want a refusal naming the remote", err)
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
}

// The pm uv tool's link in the bin dir gives way to the Go binary, and the uv tool stays: repos pinned to Python pm
// run their pin through it, and their service units run its interpreter.
func TestInstallBinaryReplacesTheUVToolsLinkAndKeepsTheTool(t *testing.T) {
	bin, tools := t.TempDir(), t.TempDir()
	t.Setenv("PM_BIN_DIR", bin)
	t.Setenv("UV_TOOL_DIR", tools)
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
	want := "replaced the pm uv tool's link " + filepath.Join(bin, "pm") + "; the uv tool stays installed for repos " +
		"pinned to Python pm; installed pm 9.9.9 at " + filepath.Join(bin, "pm")
	if err != nil || said != want {
		t.Fatalf("got %q, %v", said, err)
	}
	if st, err := os.Lstat(filepath.Join(bin, "pm")); err != nil || !st.Mode().IsRegular() {
		t.Fatalf("the bin-dir pm is not a file: %v", err)
	}
	if b, err := os.ReadFile(tool); err != nil || string(b) != "#!/bin/sh\n" {
		t.Fatalf("the uv tool's pm changed: %q, %v", b, err)
	}
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
