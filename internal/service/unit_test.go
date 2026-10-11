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
	// The unit files pm writes for each case of testdata/units, byte for byte: the files Python pm 0.1.x wrote for the
	// same inputs, frozen.
	t.Run("UnitFilesEqualTheFrozenOnes", func(t *testing.T) {
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
	})
}
