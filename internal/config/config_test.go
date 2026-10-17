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
		"site_url = \"https://pm.example.com\"\nlisten = \"0.0.0.0\"\naccess = \"cloudflare\"\naccess_team = \"team.cloudflareaccess.com\"\naccess_aud = \"aud\"\n" +
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

// writeConfig writes a git repo whose config is the required keys and extra, and reads it.
func writeConfig(t *testing.T, extra string) (config.Config, error) {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	path := filepath.Join(dir, config.Rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "version = \"0.8.2\"\nremote = \"origin\"\nmain_branch = \"main\"\nport = 8001\n" + extra
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return config.Read(dir)
}

// TestListenAndAccessAreCheckedAndTheGateNamesWhatItLacks: listen is an IP and access one of its values, else the
// config fails hard; once the site is exposed (a non-loopback listen, or a site_url) access is required, and access =
// "cloudflare" needs the Access keys. AccessMissing is what pm doctor names and the pm service refuses to start on.
func TestListenAndAccessAreCheckedAndTheGateNamesWhatItLacks(t *testing.T) {
	for _, c := range []struct{ extra, said string }{
		{"listen = \"my-host\"\n", `listen is "my-host"; set it to an IP address`},
		{"listen = \"\"\n", `listen is ""; set it to an IP address`},
		{"access = \"open\"\n", `access is "open"; set it to one of cloudflare, none`},
	} {
		if _, err := writeConfig(t, c.extra); err == nil || !strings.Contains(err.Error(), c.said) {
			t.Errorf("%q: %v, want %q", c.extra, err, c.said)
		}
	}
	keys := "access_team = \"t.cloudflareaccess.com\"\naccess_aud = \"aud\"\n"
	for _, c := range []struct {
		extra, listen string
		exposed       bool
		missing       []string
	}{
		{"", "127.0.0.1", false, nil},
		{"listen = \"::1\"\n", "::1", false, nil},
		{"listen = \"127.0.0.2\"\n", "127.0.0.2", false, nil},
		{"listen = \"0.0.0.0\"\n", "0.0.0.0", true, []string{"access"}},
		{"listen = \"192.168.1.5\"\n", "192.168.1.5", true, []string{"access"}},
		{"site_url = \"https://pm.example.com\"\n", "127.0.0.1", true, []string{"access"}},
		{"site_url = \"https://pm.example.com\"\n" + keys, "127.0.0.1", true, []string{"access"}}, // pm init migrates it
		{"listen = \"0.0.0.0\"\naccess = \"none\"\n", "0.0.0.0", true, nil},
		{"listen = \"0.0.0.0\"\naccess = \"cloudflare\"\n", "0.0.0.0", true, []string{"access_team", "access_aud"}},
		{"site_url = \"https://pm.example.com\"\naccess = \"cloudflare\"\n" + keys, "127.0.0.1", true, nil},
		{"access = \"cloudflare\"\n", "127.0.0.1", false, []string{"access_team", "access_aud"}},
	} {
		cfg, err := writeConfig(t, c.extra)
		if err != nil {
			t.Fatalf("%q: %v", c.extra, err)
		}
		if cfg.ListenAddr() != c.listen || cfg.Exposed() != c.exposed || !reflect.DeepEqual(cfg.AccessMissing(), c.missing) {
			t.Errorf("%q: listen %s exposed %v missing %v, want %s %v %v", c.extra, cfg.ListenAddr(), cfg.Exposed(),
				cfg.AccessMissing(), c.listen, c.exposed, c.missing)
		}
	}
}

// Migrated gives a config from before access existed (the Access keys, no access) access = "cloudflare", which its
// keys meant, and changes no other config.
func TestMigratedGivesAccessKeysWithNoAccessCloudflare(t *testing.T) {
	for _, c := range []struct {
		in   config.Config
		want string
		ok   bool
	}{
		{config.Config{SiteURL: "https://pm.example.com", AccessTeam: "t", AccessAud: "a"}, "cloudflare", true},
		{config.Config{AccessTeam: "t", Access: "none"}, "none", false},
		{config.Config{SiteURL: "https://pm.example.com"}, "", false},
		{config.Config{}, "", false},
	} {
		got, ok := c.in.Migrated()
		if got.Access != c.want || ok != c.ok {
			t.Errorf("%+v: access %q %v, want %q %v", c.in, got.Access, ok, c.want, c.ok)
		}
	}
}

// A non-loopback listen with no site_url serves nobody beyond this machine: a request from the network must name
// site_url's host. Unaddressed says so, naming --site-url; it is "" otherwise.
func TestUnaddressedNamesSiteURLForANetworkListenWithoutOne(t *testing.T) {
	for _, c := range []struct {
		in   config.Config
		said bool
	}{
		{config.Config{Listen: "0.0.0.0", Access: "none"}, true},
		{config.Config{Listen: "192.168.1.5", Access: "cloudflare"}, true},
		{config.Config{Listen: "0.0.0.0", Access: "none", SiteURL: "http://devbox:8123"}, false},
		{config.Config{}, false},
		{config.Config{SiteURL: "https://pm.example.com", Access: "cloudflare"}, false},
	} {
		got := c.in.Unaddressed()
		if (got != "") != c.said || (c.said && !strings.Contains(got, "pm init --site-url")) {
			t.Errorf("%+v: %q", c.in, got)
		}
	}
}
