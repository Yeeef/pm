package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/service"
)

// pinsMachine is a machine with a data dir, a unit dir (this platform's) and two clones: main, its main checkout pinning 0.5.0 and a
// linked worktree pinning 0.4.0, and another, with a pm service unit, pinning 0.3.0. A unit names a third clone,
// since removed. Pins 0.2.0 to 0.6.0 are kept, each with its binary and sha256, and 0.1.9 with its sha256 alone.
func pinsMachine(t *testing.T) (main string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	kind := service.PlatformKind()
	clone := func(name, pin string) string {
		dir := filepath.Join(root, name)
		run(t, root, "git", "init", "-q", "-b", "main", name)
		run(t, dir, "git", "commit", "-q", "--allow-empty", "-m", "start")
		pinAt(t, dir, pin)
		return dir
	}
	main = clone("main", "0.5.0")
	run(t, main, "git", "worktree", "add", "-q", "-b", "old", filepath.Join(root, "wt"))
	pinAt(t, filepath.Join(root, "wt"), "0.4.0")
	if err := os.MkdirAll(service.UnitDir(kind), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{clone("other", "0.3.0"), filepath.Join(root, "removed")} {
		unit, err := service.UnitBytes(c, kind, "/bin", 8000, []string{"/bin/pm", "service", "run"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(service.UnitFile(c, kind), unit, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0", "0.6.0"} {
		if err := os.MkdirAll(launch.PinDir(v), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(launch.PinDir(v), "sha256"), strings.Repeat("1", 64)+"\n")
		if v != "0.1.9" {
			write(t, filepath.Join(launch.PinDir(v), "pm"), "binary "+v)
		}
	}
	return main
}

func pinAt(t *testing.T, checkout, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(checkout, ".pm"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(checkout, ".pm/config.toml"), "version = \""+version+"\"\n")
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// binaries is each pin that still holds its binary, and whether every pin kept its sha256.
func binaries(t *testing.T) (kept []string, sums bool) {
	t.Helper()
	entries, err := os.ReadDir(launch.Pins())
	if err != nil {
		t.Fatal(err)
	}
	sums = true
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(launch.PinDir(e.Name()), "pm")); err == nil {
			kept = append(kept, e.Name())
		}
		if _, err := os.Stat(filepath.Join(launch.PinDir(e.Name()), "sha256")); err != nil {
			sums = false
		}
	}
	return kept, sums
}

func TestPrunePinsRemovesTheBinaryOfEachPinNoCheckoutPinsAndKeepsItsSha256(t *testing.T) {
	main := pinsMachine(t)
	if got := strings.Join(UnusedPins(main), "\n"); !strings.Contains(got, "pins pm 0.2.0, 0.6.0, whose") {
		t.Fatalf("UnusedPins = %q", got)
	}
	got := strings.Join(PrunePins(main), "\n")
	if !strings.HasPrefix(got, "pins: pruned pm 0.2.0, 0.6.0 under "+launch.Pins()) {
		t.Fatalf("PrunePins = %q", got)
	}
	kept, sums := binaries(t)
	if strings.Join(kept, " ") != "0.3.0 0.4.0 0.5.0" || !sums {
		t.Fatalf("binaries kept %v, every sha256 kept %v", kept, sums)
	}
	if got := PrunePins(main); got != nil {
		t.Fatalf("a second PrunePins = %q, want nothing", got)
	}
	if got := UnusedPins(main); got != nil {
		t.Fatalf("UnusedPins after pruning = %q, want nothing", got)
	}
}

func TestPrunePinsPrunesNoneWhenWhatAClonePinsCannotBeRead(t *testing.T) {
	for name, breakIt := range map[string]func(t *testing.T, main string){
		"a unit names no working directory": func(t *testing.T, main string) {
			t.Helper()
			kind := service.PlatformKind()
			write(t, service.UnitFile("/nowhere/x", kind), "[Service]\n")
		},
		"a worktree's config does not parse": func(t *testing.T, main string) {
			t.Helper()
			write(t, filepath.Join(filepath.Dir(main), "wt/.pm/config.toml"), "version = \n")
		},
		"a clone's worktrees cannot be listed": func(t *testing.T, main string) {
			t.Helper()
			if err := os.RemoveAll(filepath.Join(filepath.Dir(main), "other/.git")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			main := pinsMachine(t)
			breakIt(t, main)
			got := strings.Join(PrunePins(main), "\n")
			if !strings.HasPrefix(got, "pins: pruned none under "+launch.Pins()+": ") {
				t.Fatalf("PrunePins = %q", got)
			}
			if kept, _ := binaries(t); len(kept) != 5 {
				t.Fatalf("binaries kept %v, want all five", kept)
			}
			if got := strings.Join(UnusedPins(main), "\n"); !strings.HasPrefix(got, "pins: cannot tell") {
				t.Fatalf("UnusedPins = %q", got)
			}
		})
	}
}
