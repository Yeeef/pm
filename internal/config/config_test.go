package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
)

// TestConfigTextKeepsEveryKey: pm upgrade rewrites .pm/config.toml as install.ConfigText(install.SettingsOf(config))
// and pm doctor compares the file with it, so a key the config reads and the writer drops is reported as drift and
// deleted by pm upgrade. A config that sets every key the config accepts reads back the same after one rewrite.
func TestConfigTextKeepsEveryKey(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	path := filepath.Join(dir, config.Rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "version = \"0.8.2\"\nremote = \"origin\"\nmain_branch = \"main\"\nport = 8001\n" +
		"site_url = \"https://pm.example.com\"\naccess_team = \"team.cloudflareaccess.com\"\naccess_aud = \"aud\"\n" +
		"summary_runtime = \"codex\"\ndesign_sources = [\"internal/**\", \"prime.md\"]\n"
	for _, k := range config.Keys() {
		if !strings.Contains("\n"+text, "\n"+k+" = ") {
			t.Fatalf("the fixture sets no %s: give it a value that is not the default", k)
		}
	}
	read := func(text string) config.Config {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := config.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	want := read(text)
	rewritten := install.ConfigText(install.SettingsOf(want))
	got := read(rewritten)
	got.Version = want.Version // ConfigText writes the running pm's version, as pm upgrade --to does
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rewritten config reads %+v, want %+v; the rewrite:\n%s", got, want, rewritten)
	}
}
