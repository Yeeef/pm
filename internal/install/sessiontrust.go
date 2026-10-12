package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/pm/internal/proc"
)

// A Codex session's SessionStart can run while Codex skips others of pm's hooks: a pm upgrade that rewrites one entry
// (the Stop judge's, say) leaves Codex's trust in it stale, so Codex skips that hook and keeps running the unchanged
// ones. SessionTrust is the state hook's check for it, in a Codex session: CodexTrust's answer, asked only when the
// main checkout's .codex/hooks.json or the [hooks] table of the user's Codex config.toml (where the trust lives)
// changed since the last
// answer, which .pm/run/codex-trust.json keeps; so it costs a hooks/list call (a few seconds) once per change of
// either, and a hash of two small files otherwise.

// SessionTrustTimeout bounds the hooks/list call inside the state hook's own budget.
const SessionTrustTimeout = 8 * time.Second

// sessionTrustCache is .pm/run/codex-trust.json: the key of the files Codex's answer was for, and its skips.
type sessionTrustCache struct {
	Key   string   `json:"key"`
	Skips []string `json:"skips"`
}

// SessionTrust is the one line a Codex session's start context carries when Codex skips any of pm's hooks of the
// clone containing cwd, naming them and the fix; "" when it skips none, or when pm cannot tell (no Codex home, no
// answer, not a pm clone). It never fails.
func SessionTrust(cwd string) string {
	res, err := proc.Run([]string{"git", "rev-parse", "--path-format=absolute", "--git-common-dir"},
		proc.Options{Cwd: &cwd})
	if err != nil || res.Code != 0 {
		return ""
	}
	main := filepath.Dir(strings.TrimSpace(res.Stdout))
	hooks, err := os.ReadFile(filepath.Join(main, ".codex", "hooks.json"))
	if err != nil {
		return ""
	}
	trusted, ok := codexHooksState()
	if !ok {
		return ""
	}
	sum := sha256.New()
	for _, b := range [][]byte{hooks, {0}, trusted} {
		sum.Write(b) //nolint:errcheck // a hash.Hash never fails a write
	}
	key := hex.EncodeToString(sum.Sum(nil))
	path := filepath.Join(main, ".pm", "run", "codex-trust.json")
	var cache sessionTrustCache
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &cache) == nil && cache.Key == key {
		return trustLine(cache.Skips)
	}
	skips, note, err := CodexTrust(main, SessionTrustTimeout)
	if err != nil || note != "" { // not known: say nothing, and ask again at the next start
		return ""
	}
	if b, err := json.Marshal(sessionTrustCache{Key: key, Skips: skips}); err == nil && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return trustLine(skips) // unremembered: the next start asks again
		}
	}
	return trustLine(skips)
}

// codexHooksState is the [hooks] table of the user's Codex config.toml, where Codex keeps its trust in each hook, as
// canonical JSON (nil for none), and whether it could be read. Only that table: pm init edits the same file's
// writable_roots at every session start.
func codexHooksState() ([]byte, bool) {
	text, err := os.ReadFile(filepath.Join(CodexHome(), "config.toml"))
	if os.IsNotExist(err) {
		return nil, true
	}
	var data map[string]any
	if err != nil || toml.Unmarshal(text, &data) != nil {
		return nil, false
	}
	b, err := json.Marshal(data["hooks"]) // encoding/json sorts map keys
	return b, err == nil
}

// trustLine is SessionTrust's line for CodexTrust's skips; "" for none.
func trustLine(skips []string) string {
	if len(skips) == 0 {
		return ""
	}
	said := make([]string, len(skips))
	for i, s := range skips {
		said[i] = strings.TrimPrefix(s, "codex: ")
	}
	return "warning: this Codex session runs only the pm hooks Codex trusts. " + strings.Join(said, " ") +
		". Until then the checks those hooks run do not run in Codex sessions."
}
