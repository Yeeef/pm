package service

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The unit files Go pm writes equal Python pm's for the same inputs: testdata/units holds Python's output for each
// case (test_service.py checks Python against the same files), and Go must write it byte for byte.
func TestUnitFilesEqualPythonsForTheSameInputs(t *testing.T) {
	data, err := os.ReadFile("testdata/units/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Main, Path string
		Port             int
		Argv             []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 3 {
		t.Fatalf("%d cases", len(cases))
	}
	for _, c := range cases {
		for kind, ext := range map[string]string{Launchd: "plist", Systemd: "service"} {
			want, err := os.ReadFile(filepath.Join("testdata/units", c.Name+"."+ext))
			if err != nil {
				t.Fatal(err)
			}
			got, err := UnitBytes(c.Main, kind, c.Path, c.Port, c.Argv)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("%s %s:\n%s\nwant (Python's):\n%s", c.Name, kind, got, want)
			}
		}
	}
}

func TestUnitPortAndCommandReadBackWhatInstallWrote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	main := "/pm-test/my clone%x"
	argv := []string{"/opt/p m/pm & <x>", "service", "run", `it's "%h" \ x`}
	for _, kind := range []string{Launchd, Systemd} {
		unit := UnitFile(main, kind)
		os.MkdirAll(filepath.Dir(unit), 0o755)
		b, _ := UnitBytes(main, kind, "/bin", 8765, argv)
		os.WriteFile(unit, b, 0o644)
		if p, err := UnitPort(main, kind); p != 8765 || err != nil {
			t.Errorf("%s port %d %v", kind, p, err)
		}
		if got := UnitCommand(main, kind); strings.Join(got, "|") != strings.Join(argv, "|") {
			t.Errorf("%s command %q", kind, got)
		}
		os.WriteFile(unit, []byte("nothing"), 0o644)
		if _, err := UnitPort(main, kind); err == nil || !strings.Contains(err.Error(), "sets no PORT; run pm service install again") {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

func TestALabelNamesTheCloneAndHashesItsPath(t *testing.T) {
	// sha1("/pm-test/clone")[:8], as Python's hashlib gives it
	if got := Label("/pm-test/clone"); got != "local.pm.clone.27807ca8" {
		t.Fatal(got)
	}
}

func TestFreePortSkipsHeldPortsAndOtherClonesUnits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	start := held.Addr().(*net.TCPAddr).Port
	saved := FirstPort
	FirstPort = start
	defer func() { FirstPort = saved }()
	unit := UnitFile("/elsewhere/other", PlatformKind())
	os.MkdirAll(unit+".d", 0o755) // a systemd drop-in directory matches the glob too
	b, _ := UnitBytes("/elsewhere/other", PlatformKind(), "/bin", start+1, Command("/pm"))
	os.WriteFile(unit, b, 0o644)
	if !UnitPorts()[start+1] {
		t.Fatalf("unit ports %v", UnitPorts())
	}
	got := FreePort()
	if got < start+2 || !PortFree(got) || PortFree(start) {
		t.Fatalf("free port %d from %d", got, start)
	}
	for p := start + 2; p < got; p++ {
		if PortFree(p) {
			t.Fatalf("%d is free, but %d was given", p, got)
		}
	}
}

func TestAPortInTimeWaitIsFree(t *testing.T) {
	srv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := srv.Addr().(*net.TCPAddr).Port
	cli, _ := net.Dial("tcp", srv.Addr().String())
	conn, _ := srv.Accept()
	conn.Close() // the active close: TIME_WAIT on the server's port
	cli.Close()
	srv.Close()
	if !PortFree(port) {
		t.Fatal("a port in TIME_WAIT counts as held")
	}
}

func TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	main := t.TempDir()
	held, err := net.Listen("tcp", "127.0.0.1:0") // holds the port, answers no HTTP
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port
	if err := CheckPort(main, port); err == nil || !strings.Contains(err.Error(),
		"the site port :"+strconv.Itoa(port)+" is held by a process that does not answer HTTP") {
		t.Fatal(err)
	}
	unit := UnitFile(main, PlatformKind())
	os.MkdirAll(filepath.Dir(unit), 0o755)
	b, _ := UnitBytes(main, PlatformKind(), "/bin", port+1, Command("/pm"))
	os.WriteFile(unit, b, 0o644)
	if err := CheckPort(main, port); err == nil { // its unit serves on another port
		t.Fatal("refused nothing")
	}
	b, _ = UnitBytes(main, PlatformKind(), "/bin", port, Command("/pm"))
	os.WriteFile(unit, b, 0o644)
	if err := CheckPort(main, port); err != nil {
		t.Fatal(err)
	}
}
