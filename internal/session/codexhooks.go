package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

// codexHooksDir holds, under the main checkout's .pm/run, one file per Codex session pm checked: "ran" or "warned".
const codexHooksDir = "codex-hooks"

// WarnCodexHooksSkipped tells Codex session id, running a pm command in here, once, that Codex ran none of pm's hooks
// in it: its rollout holds no pm session-start context before its first prompt, as when the user has not trusted pm's
// hooks (Codex skips an untrusted hook silently). Without them the session has no pm rules, no Stop checks and no
// replies at its prompts. Each session is checked until pm can tell, then remembered in .pm/run, so a later command
// costs one git rev-parse and one stat. It never fails the command: anything it cannot read is no warning.
func WarnCodexHooksSkipped(here, id string, stderr io.Writer) {
	res, err := proc.Run([]string{"git", "rev-parse", "--path-format=absolute", "--git-common-dir"},
		proc.Options{Cwd: &here})
	if err != nil || res.Code != 0 {
		return
	}
	main := filepath.Dir(strings.TrimSpace(res.Stdout))
	if _, err := os.Stat(filepath.Join(main, config.Rel)); err != nil { // not a pm repo, or a repo before pm init
		return
	}
	mark := filepath.Join(main, config.Run, codexHooksDir, filepath.Base(id))
	if _, err := os.Stat(mark); err == nil {
		return
	}
	ran, known := CodexHooksRan(id)
	if !known {
		return
	}
	said := "ran"
	if !ran {
		said = "warned"
		fmt.Fprintf(stderr, "warning: Codex ran none of pm's hooks in this session: it started without pm's rules and "+
			"state, and its Stop checks and the owner's replies at each prompt do not run. Codex skips a hook the user "+
			"has not trusted: open Codex in %s, trust pm's hooks in /hooks (pm doctor names them), then start a new "+
			"session.\n", main)
	}
	if os.MkdirAll(filepath.Dir(mark), 0o755) != nil {
		return
	}
	if err := os.WriteFile(mark, []byte(said+"\n"), 0o644); err != nil {
		return // unremembered: the next command checks again
	}
}
