// Package ask runs one prompt through an agent runtime's CLI and returns its answer: Claude Code's `claude -p` or
// Codex's `codex exec`, each on a small model, non-interactive, from an empty temp directory outside any repo, with no
// tools it needs, no hooks, no project instructions and no saved session, in a process group killed whole at the
// timeout. The owner-request judge (internal/hooks) and pm day summarize use it; there is no fallback from one runtime to
// the other.
//
// No hooks matters most: the judge runs inside a Stop hook, and a child session that ran the project's or the user's
// Stop hooks would run the judge again. Claude Code loads no settings (--setting-sources ""), so no hook; Codex runs with
// its hooks feature off (--disable hooks), without the user's config.toml (--ignore-user-config, which also drops its
// MCP servers and notify) and without execpolicy rules (--ignore-rules). Neither child finds a repo's CLAUDE.md,
// AGENTS.md or .codex/ from the temp directory. Codex still reads the user's own $CODEX_HOME/AGENTS.md, which no flag
// turns off; the judge's instructions replace Codex's base prompt, so that file is context, not the rules.
package ask

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

// Runtime is an agent runtime whose CLI answers a prompt.
type Runtime string

const (
	Claude Runtime = "claude"
	Codex  Runtime = "codex"
)

// Runtimes are the runtimes pm can ask, by name.
var Runtimes = []string{string(Claude), string(Codex)}

// Model is the small model each runtime answers on: Claude Haiku, and Codex's fast listed model.
func (r Runtime) Model() string {
	if r == Codex {
		return "gpt-5.6-luna"
	}
	return "claude-haiku-5-5"
}

// Command is how pm names the call in its messages.
func (r Runtime) Command() string {
	if r == Codex {
		return "codex exec"
	}
	return "claude -p"
}

// Call is one prompt: System replaces the runtime's own system prompt when set.
type Call struct {
	System, Prompt string
	Timeout        time.Duration
}

// Error is a call that gave no answer: the CLI is missing, ran past the timeout, or failed (or answered nothing).
type Error struct {
	Runtime Runtime
	Missing bool
	Timeout time.Duration // set when it timed out
	Code    int
	Said    string // the CLI's own words, whitespace collapsed and cut to 300 characters; "no output" when none
}

func (e *Error) Error() string {
	switch {
	case e.Missing:
		return string(e.Runtime) + " is not installed or not on PATH"
	case e.Timeout > 0:
		return fmt.Sprintf("%s timed out after %ds", e.Runtime.Command(), int(e.Timeout.Seconds()))
	}
	return fmt.Sprintf("%s failed (exit %d): %s", e.Runtime.Command(), e.Code, e.Said)
}

// claudeEnv turns extended thinking off (with it, a judge's verdict took 5 to 36 s instead of about 2 s) and skips
// nonessential traffic.
var claudeEnv = [][2]string{{"MAX_THINKING_TOKENS", "0"}, {"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1"}}

// codexOff is what a Codex call leaves out besides hooks, rules and the user's config: the context blocks Codex adds to
// every turn (permissions, apps, environment, collaboration mode, the skills list, recommended plugins).
var codexOff = []string{"--disable", "plugins", "--disable", "recommended_plugins",
	"-c", "skills.include_instructions=false", "-c", "include_permissions_instructions=false",
	"-c", "include_apps_instructions=false", "-c", "include_environment_context=false",
	"-c", "include_collaboration_mode_instructions=false"}

// Args is the runtime's argv for c, run in dir (the temp directory, where a Codex call keeps its instructions file and
// its answer); the prompt goes on stdin.
func (r Runtime) Args(c Call, dir string) []string {
	if r == Codex {
		args := []string{"codex", "exec", "--model", r.Model(), "-c", `model_reasoning_effort="low"`, "--ephemeral",
			"--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "--ignore-rules",
			"--disable", "hooks"}
		args = append(args, codexOff...)
		if c.System != "" {
			args = append(args, "-c", "model_instructions_file="+tomlString(filepath.Join(dir, "instructions.md")))
		}
		return append(args, "--color", "never", "--output-last-message", filepath.Join(dir, "answer.md"), "-")
	}
	args := []string{"claude", "-p", "--model", r.Model(), "--setting-sources", "", "--strict-mcp-config",
		"--tools", "", "--no-session-persistence", "--output-format", "text"}
	if c.System != "" {
		args = append(args, "--system-prompt", c.System)
	}
	return args
}

// Ask is the runtime's answer to c, stripped; an *Error when it gives none.
func Ask(r Runtime, c Call) (string, error) {
	dir, err := os.MkdirTemp("", "pm-ask-") // outside any repo: no project settings, hooks or instructions apply
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir) //nolint:errcheck // a temp dir left behind breaks nothing
	if r == Codex && c.System != "" {
		if err := os.WriteFile(filepath.Join(dir, "instructions.md"), []byte(c.System), 0o600); err != nil {
			return "", err
		}
	}
	env := os.Environ()
	if r == Claude {
		for _, kv := range claudeEnv {
			env = setEnv(env, kv[0], kv[1])
		}
	}
	secs := int(c.Timeout.Seconds())
	res, err := proc.Run(r.Args(c, dir), proc.Options{Cwd: &dir, Env: env, Stdin: c.Prompt, Timeout: c.Timeout,
		TimeoutText: fmt.Sprint(secs), Group: true})
	var pe *proc.Error
	switch {
	case errors.As(err, &pe) && pe.Type == "FileNotFoundError":
		return "", &Error{Runtime: r, Missing: true}
	case errors.As(err, &pe) && pe.Type == "TimeoutExpired":
		return "", &Error{Runtime: r, Timeout: c.Timeout}
	case err != nil:
		return "", err
	}
	answer := res.Stdout
	if r == Codex && res.Code == 0 { // Codex prints its progress too; the last message is the answer
		b, err := os.ReadFile(filepath.Join(dir, "answer.md"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		answer = string(b)
	}
	if text := config.PyStrip(answer); res.Code == 0 && text != "" {
		return text, nil
	}
	said := res.Stderr
	if said == "" {
		said = res.Stdout
	}
	said = strings.Join(strings.FieldsFunc(said, config.IsPySpace), " ")
	if r := []rune(said); len(r) > 300 {
		said = string(r[len(r)-300:]) // Codex's error comes last, after its banner
	}
	if said == "" {
		said = "no output"
	}
	return "", &Error{Runtime: r, Code: res.Code, Said: said}
}

// tomlString is s as a TOML basic string, which `codex -c key=value` parses: JSON's escapes are TOML's.
func tomlString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // a string always marshals
	}
	return string(b)
}

// setEnv is env with key set to value.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}
