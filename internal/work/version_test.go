package work

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
)

// The version handshake (the pm-go page, Store access): a command checks the service's pm_version() on the connection
// it then uses, and refuses a service on another version. Which refusal depends on the main checkout's pin: a pin
// that is the command's means the service is stale; a pin that is the service's means this checkout pins another
// version than the main checkout.
func TestAServiceOnAnotherVersionIsRefused(t *testing.T) {
	main := shortMain(t)
	pin := func(v string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(main, ".pm"), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := "version = \"" + v + "\"\nremote = \"origin\"\nmain_branch = \"main\"\nport = 8000\n"
		if err := os.WriteFile(filepath.Join(main, ".pm", "config.toml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "init", "-q", main).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	h, err := NewHost(HostOptions{Main: main, Version: "9.9.9-old"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	v := buildinfo.Version
	pin(v)
	if _, err := Dial(main); err == nil || err.Error() != "the pm service runs pm 9.9.9-old, not pm "+v+": run pm "+
		"service restart" {
		t.Fatalf("a stale service: %v", err)
	}
	pin("9.9.9-old")
	if _, err := Dial(main); err == nil || err.Error() != "this checkout pins pm "+v+", but the clone's pm service "+
		"runs pm 9.9.9-old, which "+filepath.Join(main, ".pm", "config.toml")+" pins: run it from a checkout that pins "+
		"pm 9.9.9-old" {
		t.Fatalf("a checkout on another pin: %v", err)
	}
	if _, err := DialSetup(main); err == nil {
		t.Fatal("pm init's connection skipped the handshake")
	}
	h.Close()
	if _, err := Dial(main); err == nil || err.Error() != "the pm service does not answer on "+Sock(main)+"; pm "+
		"reaches the work store only through it: run pm service restart" {
		t.Fatalf("no service: %v", err)
	}
}

// The socket: mode 0600, under the main checkout's .pm/run; a path past the kernel's limit fails the start.
func TestTheSocketIsPrivateAndItsPathBounded(t *testing.T) {
	_, o := newStore(t)
	st, err := os.Stat(o.h.Sock())
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSocket == 0 || st.Mode().Perm() != 0o600 || o.h.Sock() != filepath.Join(o.main, ".pm/run/work.sock") {
		t.Fatalf("%s: %v", o.h.Sock(), st.Mode())
	}
	long := filepath.Join(shortMain(t), "a-directory-name-that-makes-the-socket-path-too-long-for-the-kernel-to-bind-"+
		"it-on-macos-or-linux")
	if _, err := NewHost(HostOptions{Main: long, Version: "x"}); err == nil ||
		err.Error() != "the pm service's socket "+Sock(long)+" is "+itoa(len(Sock(long)))+" bytes, over this "+
			"system's limit of 103; move the clone to a shorter path" {
		t.Fatalf("a long path: %v", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
