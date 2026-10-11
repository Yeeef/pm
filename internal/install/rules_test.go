package install

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
)

// TestCodexRulesAllowPmAlone checks .codex/rules/pm.rules with Codex's own policy engine, codex execpolicy check
// (offline; skipped where codex is not installed): a pm command is allowed, so Codex runs it outside its sandbox, and
// nothing else is. Codex splits a shell script into plain commands before it checks them; a heredoc script is not
// split, so it is checked whole, as bash -lc <script>, which no rule allows.
func TestCodexRulesAllowPmAlone(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex is not installed")
	}
	rules := filepath.Join(t.TempDir(), "pm.rules")
	if err := os.WriteFile(rules, []byte(CodexRules), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		argv  []string
		allow bool
	}{
		{[]string{"pm", "show"}, true},
		{[]string{"pm", "commit", "-m", "x", "records/a.md"}, true},
		{[]string{"pm", "finding", "add", "--sprint", "X", "--text=one line"}, true},
		{[]string{"pmx"}, false},
		{[]string{"rm", "-rf", "x"}, false},
		{[]string{"bash", "-lc", "pm finding add --sprint X --text-file - <<'EOF'\nA finding.\nEOF"}, false},
	} {
		cmd := exec.Command(codex, append([]string{"execpolicy", "check", "--rules", rules}, c.argv...)...)
		cmd.Env = append(os.Environ(), "CODEX_HOME="+t.TempDir())
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%q: %v", c.argv, err)
		}
		var got struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("%q: %s", c.argv, out)
		}
		if (got.Decision == "allow") != c.allow {
			t.Errorf("%q: decision %q, want allowed %v", c.argv, got.Decision, c.allow)
		}
	}
}

// TestCodexRulesAreAPieceDoctorUpgradeAndUninstallKnow: pm doctor names pm.rules missing or changed, pm upgrade
// writes it back, pm uninstall removes it.
func TestCodexRulesAreAPieceDoctorUpgradeAndUninstallKnow(t *testing.T) {
	s := goldenSettings[0]
	top := t.TempDir()
	planned, err := Rewrite(top, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range planned {
		if err := os.MkdirAll(filepath.Dir(pl.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pl.Path, []byte(pl.New), pl.Piece.Mode); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(top, ".codex", "rules", "pm.rules")
	if b, err := os.ReadFile(path); err != nil || string(b) != CodexRules {
		t.Fatalf("pm upgrade did not write pm.rules: %q %v", b, err)
	}
	for text, want := range map[string]string{
		"": "",
		"prefix_rule(pattern=[\"pm\"], decision=\"prompt\")\n": ".codex/rules/pm.rules: pm's part differs from what pm " + buildinfo.Version + " writes",
		"missing": ".codex/rules/pm.rules: pm's part is missing",
	} {
		switch text {
		case "":
			if err := os.WriteFile(path, []byte(CodexRules), 0o644); err != nil {
				t.Fatal(err)
			}
		case "missing":
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		default:
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		drift, err := Drift(top, s)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(drift, "\n"); got != want {
			t.Errorf("%q: pm doctor says %q, want %q", text, got, want)
		}
	}
	if err := os.WriteFile(path, []byte(CodexRules), 0o644); err != nil {
		t.Fatal(err)
	}
	removals, err := Removals(top, s)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	for _, r := range removals {
		removed = removed || (r.Path == path && r.New == nil)
	}
	if !removed {
		t.Errorf("pm uninstall leaves pm.rules: %v", removals)
	}
}
