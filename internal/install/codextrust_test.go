package install

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// trustRepo is a main checkout holding pm 0.6's .codex/hooks.json, the file Codex answered
// testdata/codex-hooks-list.json for.
func trustRepo(t *testing.T) string {
	t.Helper()
	main := t.TempDir()
	text, err := os.ReadFile(filepath.Join("testdata", "codex-hooks-0.6.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(main, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, ".codex", "hooks.json"), text, 0o644); err != nil {
		t.Fatal(err)
	}
	return main
}

// fakeCodex puts first on PATH a codex whose app-server answers initialize, then hooks/list with answer (or, with
// answer "", never).
func fakeCodex(t *testing.T, answer string) {
	t.Helper()
	dir := t.TempDir()
	list := filepath.Join(dir, "list.json")
	if err := os.WriteFile(list, []byte(answer), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1\" = app-server ] || exit 2\nread -r l\necho '{\"id\":0,\"result\":{\"userAgent\":\"fake\"}}'\n" +
		"read -r l\nread -r l\necho '{\"method\":\"configWarning\",\"params\":{\"summary\":\"x\"}}'\n"
	if answer == "" {
		script += "sleep 30\n"
	} else {
		script += "cat " + list + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// listAnswer is Codex's real hooks/list answer for pm 0.6's hooks (every one untrusted) moved to main, with the
// statuses set by key suffix.
func listAnswer(t *testing.T, main string, status map[string]string, enabled map[string]bool) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "codex-hooks-list.json"))
	if err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(raw), "@MAIN@", main)), &msg); err != nil {
		t.Fatal(err)
	}
	for _, h := range msg["result"].(map[string]any)["data"].([]any)[0].(map[string]any)["hooks"].([]any) {
		h := h.(map[string]any)
		suffix := strings.TrimPrefix(h["key"].(string), filepath.Join(main, ".codex", "hooks.json")+":")
		if s, ok := status[suffix]; ok {
			h["trustStatus"] = s
		}
		if e, ok := enabled[suffix]; ok {
			h["enabled"] = e
		}
	}
	b, _ := json.Marshal(msg)
	return string(b) + "\n"
}

func TestCodexTrustNamesPmsHooksCodexSkips(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	main := trustRepo(t)
	hooks := filepath.Join(main, ".codex", "hooks.json")
	fix := "; open Codex in " + main + " and trust pm's hooks in /hooks"
	trusted := map[string]string{}
	for _, k := range []string{"session_start:0:0", "session_start:0:1", "session_start:0:2", "session_start:0:3",
		"subagent_start:0:0", "subagent_start:0:1", "subagent_start:0:2", "subagent_start:0:3", "stop:0:0", "stop:0:1"} {
		trusted[k] = "trusted"
	}
	mixed := map[string]string{}
	for k, v := range trusted {
		mixed[k] = v
	}
	mixed["session_start:0:3"], mixed["stop:0:1"] = "untrusted", "modified"
	for name, c := range map[string]struct {
		answer string
		want   []string
	}{
		"all trusted": {listAnswer(t, main, trusted, nil), nil},
		"as Codex answered": {listAnswer(t, main, nil, nil), []string{"codex: Codex skips all of pm's 10 hooks in " +
			hooks + ", which you have not trusted" + fix}},
		"mixed": {listAnswer(t, main, mixed, map[string]bool{"stop:0:0": false}), []string{
			"codex: Codex skips 1 of pm's 10 hooks in " + hooks + ", which you have not trusted: sessionStart `pm prime --state --hook-json`" + fix,
			"codex: Codex skips 1 of pm's 10 hooks in " + hooks + ", which changed since you trusted them: stop `pm hook stop || exit 1`" + fix,
			"codex: Codex skips 1 of pm's 10 hooks in " + hooks + ", which you disabled: stop `pm hook owner-request || exit 1`" + fix}},
		"project untrusted": {`{"id":1,"result":{"data":[{"cwd":"` + main + `","hooks":[],"warnings":[],"errors":[]}]}}` + "\n",
			[]string{"codex: Codex runs none of pm's hooks in " + hooks + ": it lists none of them, as it does until you " +
				"trust the project; open Codex in " + main + ", trust the project, then trust pm's hooks in /hooks"}},
	} {
		fakeCodex(t, c.answer)
		got, note, err := CodexTrust(main, 5*time.Second)
		if err != nil || note != "" {
			t.Fatalf("%s: %q %v", name, note, err)
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s:\ngot  %q\nwant %q", name, got, c.want)
		}
	}
}

func TestCodexTrustSaysWhyItWasNotCheckedAndIsBounded(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	main := trustRepo(t)
	fakeCodex(t, `{"id":1,"error":{"code":-32600,"message":"no such method"}}`+"\n")
	if got, note, err := CodexTrust(main, 5*time.Second); err != nil || got != nil || note != "codex: cannot ask "+
		"Codex (codex app-server, hooks/list) whether it runs pm's hooks, so that is not known: no such method" {
		t.Errorf("an error answer: %q %q %v", got, note, err)
	}
	fakeCodex(t, "")
	start := time.Now()
	got, note, err := CodexTrust(main, 500*time.Millisecond)
	if err != nil || got != nil || !strings.HasSuffix(note, "so that is not known: no answer within 500ms") {
		t.Errorf("a codex that never answers: %q %q %v", got, note, err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %s", took)
	}
	// no $CODEX_HOME: Codex never ran here, and the call is not made, so it creates no Codex state
	home := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", home)
	fakeCodex(t, "")
	if got, note, err := CodexTrust(main, time.Second); got != nil || err != nil || note != "codex: "+home+
		" does not exist, so Codex never ran here and whether it runs pm's hooks was not checked" {
		t.Errorf("without CODEX_HOME: %q %q %v", got, note, err)
	}
	if _, err := os.Stat(home); err == nil {
		t.Error("CODEX_HOME was made")
	}
	t.Setenv("PATH", t.TempDir())
	if got, note, err := CodexTrust(main, time.Second); got != nil || err != nil ||
		note != "codex: codex is not on PATH, so whether Codex runs pm's hooks was not checked" {
		t.Errorf("without codex: %q %q %v", got, note, err)
	}
}

// TestCodexTrustAgainstRealCodex asks the codex on PATH (skipped without one) about a repo in a temp CODEX_HOME: an
// untrusted project, then a trusted one whose pm hooks the user never trusted. It checks that the call writes no
// config.toml and no hook trust there.
func TestCodexTrustAgainstRealCodex(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex is not installed")
	}
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	main := trustRepo(t)
	if r, err := filepath.EvalSymlinks(main); err == nil {
		main = r
	}
	got, note, err := CodexTrust(main, CodexTrustTimeout)
	if err != nil || note != "" || len(got) != 1 || !strings.HasPrefix(got[0], "codex: Codex runs none of pm's hooks") {
		t.Fatalf("an untrusted project: %q %q %v", got, note, err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err == nil {
		t.Error("codex app-server wrote a config.toml")
	}
	config := "[projects." + quote(main) + "]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err = CodexTrust(main, CodexTrustTimeout)
	if err != nil || len(got) != 1 || !strings.HasPrefix(got[0], "codex: Codex skips all of pm's 10 hooks in ") {
		t.Fatalf("a trusted project, pm's hooks not: %q %v", got, err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "config.toml")); string(b) != config {
		t.Errorf("codex app-server changed config.toml: %q", b)
	}
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
