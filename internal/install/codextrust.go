package install

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Codex runs a project's hooks only once the user trusts the project and then each hook (in /hooks); a hook changed
// since it was trusted is "modified" and skipped too. pm never writes that trust (an owner decision): it asks Codex
// itself, through `codex app-server`'s hooks/list, which hooks of the main checkout's .codex/hooks.json (Codex reads
// the main checkout's) Codex would skip. The call reads Codex's config; Codex 0.154's app-server also opens its state
// databases and logs under $CODEX_HOME, as any Codex run does, and writes no trust or config.

// CodexTrustTimeout bounds the whole hooks/list call.
const CodexTrustTimeout = 10 * time.Second

// codexListedHook is one hook of hooks/list's answer, as far as pm reads it.
type codexListedHook struct {
	Key         string `json:"key"`
	EventName   string `json:"eventName"`
	Command     string `json:"command"`
	SourcePath  string `json:"sourcePath"`
	Enabled     bool   `json:"enabled"`
	TrustStatus string `json:"trustStatus"`
}

// codexHooksList runs `codex app-server` in dir and answers hooks/list for dir: the JSON-RPC lines of its stdio, the
// initialize handshake, then the request, within timeout.
func codexHooksList(codex, dir string, timeout time.Duration) ([]codexListedHook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, codex, "app-server")
	cmd.Dir = dir
	// at the timeout, kill its process group: a child holding its stdout would keep the read waiting
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// teardown once the answer is read: closing stdin ends the app-server, whose exit status (an EOF exit, or the
	// timeout's kill) says nothing pm reads
	defer func() {
		stdin.Close() //nolint:errcheck // the pipe to a process that is ending; nothing is written after
		cancel()
		cmd.Wait() //nolint:errcheck // an EOF exit or the timeout's kill: the answer was read, or the error said why not
	}()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 0, 1<<16), 16<<20)
	send := func(v any) error {
		b, err := json.Marshal(v)
		if err == nil {
			_, err = stdin.Write(append(b, '\n'))
		}
		return err
	}
	// answer is the result of request id, skipping notifications and other answers
	answer := func(id int) (json.RawMessage, error) {
		for lines.Scan() {
			var msg struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(lines.Bytes(), &msg) != nil || msg.ID == nil || *msg.ID != id {
				continue
			}
			if msg.Error != nil {
				return nil, errors.New(msg.Error.Message)
			}
			return msg.Result, nil
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("no answer within %s", timeout)
		}
		return nil, errors.New("it ended without an answer")
	}
	if err := send(map[string]any{"id": 0, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]any{"name": "pm", "title": nil, "version": "0"}}}); err != nil {
		return nil, err
	}
	if _, err := answer(0); err != nil {
		return nil, err
	}
	if err := send(map[string]any{"method": "initialized"}); err != nil {
		return nil, err
	}
	if err := send(map[string]any{"id": 1, "method": "hooks/list", "params": map[string]any{"cwds": []string{dir}}}); err != nil {
		return nil, err
	}
	raw, err := answer(1)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []struct {
			Cwd   string            `json:"cwd"`
			Hooks []codexListedHook `json:"hooks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("hooks/list answered out of shape: %v", err)
	}
	var out []codexListedHook
	for _, d := range res.Data {
		out = append(out, d.Hooks...)
	}
	return out, nil
}

// CodexTrust is what pm doctor and pm init say of Codex's trust in pm's hooks of the main checkout. skips names the
// project when Codex lists none of its hooks (it disables a project's hooks until the project is trusted), else each
// of pm's hooks Codex would skip: untrusted, modified since trusted, or disabled; each with the fix. note says why the
// trust was not checked: codex is not on PATH, $CODEX_HOME does not exist (the call would create Codex's state
// there), or the call failed.
func CodexTrust(main string, timeout time.Duration) (skips []string, note string, err error) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		return nil, "codex: codex is not on PATH, so whether Codex runs pm's hooks was not checked", nil //nolint:nilerr // no codex is a note, not a failure
	}
	if home := CodexHome(); !isDir(home) { // Codex never ran here, and app-server would make its state there
		return nil, fmt.Sprintf("codex: %s does not exist, so Codex never ran here and whether it runs pm's hooks "+
			"was not checked", home), nil
	}
	hooksPath := filepath.Join(main, ".codex", "hooks.json")
	text, err := Read(hooksPath)
	if err != nil || text == nil {
		return nil, "", err // pm doctor names the missing file itself
	}
	f, err := loadHooks(hooksPath, text, false)
	if err != nil || f.events == nil {
		return nil, "", nil //nolint:nilerr // pm doctor names an unreadable file itself, as its drift
	}
	mine := 0
	for _, e := range f.events.Keys {
		gs, ok := f.events.Get(e).([]any) // loadHooks checked it is a list
		if !ok {
			continue
		}
		for _, g := range gs {
			for _, h := range groupHooks(g) {
				if isPMHook(h) {
					mine++
				}
			}
		}
	}
	if mine == 0 {
		return nil, "", nil
	}
	listed, err := codexHooksList(codex, main, timeout)
	if err != nil {
		return nil, fmt.Sprintf("codex: cannot ask Codex (codex app-server, hooks/list) whether it runs pm's hooks, so "+
			"that is not known: %s", strings.Join(strings.Fields(err.Error()), " ")), nil
	}
	fix := fmt.Sprintf("open Codex in %s and trust pm's hooks in /hooks", main)
	var seen int
	skipped := map[string][]string{}
	for _, h := range listed {
		if filepath.Clean(h.SourcePath) != hooksPath || !(strings.HasPrefix(h.Command, "pm prime") ||
			strings.HasPrefix(h.Command, "pm hook ")) {
			continue
		}
		seen++
		why := h.TrustStatus
		if !h.Enabled && why == "trusted" {
			why = "disabled"
		}
		if why != "trusted" && why != "managed" {
			skipped[why] = append(skipped[why], h.EventName+" `"+h.Command+"`")
		}
	}
	if seen == 0 {
		return []string{fmt.Sprintf("codex: Codex runs none of pm's hooks in %s: it lists none of them, as it does "+
			"until you trust the project; open Codex in %s, trust the project, then trust pm's hooks in /hooks",
			hooksPath, main)}, "", nil
	}
	if len(skipped["untrusted"]) == seen { // a new clone's or a never trusted project's: one line, not one per hook
		return []string{fmt.Sprintf("codex: Codex skips all of pm's %d hooks in %s, which you have not trusted; %s",
			seen, hooksPath, fix)}, "", nil
	}
	for _, why := range []string{"untrusted", "modified", "disabled"} {
		if hs := skipped[why]; hs != nil {
			skips = append(skips, fmt.Sprintf("codex: Codex skips %d of pm's %d hooks in %s, %s: %s; %s", len(hs), seen,
				hooksPath, map[string]string{"untrusted": "which you have not trusted",
					"modified": "which changed since you trusted them", "disabled": "which you disabled"}[why],
				strings.Join(hs, ", "), fix))
		}
	}
	return skips, "", nil
}
