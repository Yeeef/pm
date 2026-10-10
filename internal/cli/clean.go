package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

// agentWorktrees is where Claude Code makes an agent's worktree (claude -w, a subagent's isolation), under the main
// checkout; pm clean removes only worktrees there.
const agentWorktrees = ".claude/worktrees"

// worktree is one entry of git worktree list.
type worktree struct {
	path, head, branch string // branch is "" for a detached HEAD
	locked             bool
	lockReason         string
	prunable           bool
}

// verdict is pm clean's decision on one worktree: keep or remove it, why, and whether its branch goes too.
type verdict struct {
	wt           worktree
	remove       bool
	reason       string
	deleteBranch bool // the branch is on the main branch, so it goes with the worktree
	unlock       bool // a stale lock, taken off before git worktree remove
}

// cmdClean is pm clean: each worktree with keep or remove and the reason; with --apply, the removals done.
func cmdClean(p *Parsed, here string, stdout io.Writer) error {
	cfg, err := config.Load(here)
	if err != nil {
		return err
	}
	trees, err := listWorktrees(here)
	if err != nil {
		return err
	}
	main := trees[0].path // git lists the main checkout first
	caller, err := cleanGit(here, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	c := &cleaner{main: main, caller: realPath(strings.TrimSpace(caller)), remote: cfg.Remote, mainBranch: cfg.MainBranch,
		mainRef: "refs/remotes/" + cfg.Remote + "/" + cfg.MainBranch, mainName: cfg.Remote + "/" + cfg.MainBranch}
	if _, err := cleanGit(main, "rev-parse", "--verify", "--quiet", c.mainRef); err != nil {
		return refuse("no %s in this clone to compare branches with; run git fetch %s %s", c.mainName, cfg.Remote,
			cfg.MainBranch)
	}
	if c.sessions, err = liveUses(trees, main); err != nil {
		return refuse("pm clean cannot tell which worktrees live sessions use, so it removes none: %v", err)
	}
	c.trees = trees
	apply := p.Get("apply") == "true"
	var removing, failed int
	for _, wt := range trees {
		v := c.judge(wt)
		word := "keep"
		if v.remove {
			word = "remove"
			removing++
		}
		name := v.wt.branch
		if name == "" {
			name = "detached " + shortSHA(v.wt.head)
		}
		line := fmt.Sprintf("%-6s  %s  (%s): %s", word, v.wt.path, name, v.reason)
		if v.remove && apply {
			if err := c.remove(v); err != nil {
				line += "; NOT removed: " + err.Error()
				failed++
			}
		}
		fmt.Fprintln(stdout, line)
	}
	if apply {
		if failed > 0 {
			return refuse("%d of %d worktrees to remove were not removed; each line above says why", failed, removing)
		}
		fmt.Fprintf(stdout, "removed %d of %d worktrees\n", removing, len(trees))
		return nil
	}
	fmt.Fprintf(stdout, "dry run: %d of %d worktrees to remove; pm clean --apply removes them\n", removing, len(trees))
	return nil
}

type cleaner struct {
	main, caller, remote, mainBranch, mainRef, mainName string
	sessions                                            map[string]liveUse // by worktree path: the latest live session that used it
	trees                                               []worktree
}

// judge decides one worktree. Kept: the main checkout, the records store, the calling worktree, anything outside
// .claude/worktrees/, a worktree a live process locks or a live session used, a dirty one, and one whose commits are
// neither on the main branch nor pushed.
func (c *cleaner) judge(wt worktree) verdict {
	v := verdict{wt: wt}
	var notes []string // a stale lock, said beside the verdict
	note := func(reason string) string { return strings.Join(append([]string{reason}, notes...), "; ") }
	keep := func(reason string) verdict { return verdict{wt: wt, reason: note(reason)} }
	switch path := realPath(wt.path); {
	case wt.path == c.main:
		return keep("the main checkout")
	case path == realPath(filepath.Join(c.main, config.Store)):
		return keep("the records store")
	case path == c.caller:
		return keep("the worktree pm clean runs in")
	case !strings.HasPrefix(wt.path, filepath.Join(c.main, agentWorktrees)+"/"):
		return keep("not an agent worktree (outside " + agentWorktrees + "/)")
	}
	if wt.locked {
		owner, stale := lockOwner(wt.lockReason)
		if !stale {
			return keep(owner)
		}
		v.unlock = true
		notes = append(notes, owner)
	}
	if use, ok := c.sessions[wt.path]; ok {
		return keep(fmt.Sprintf("live session %s used it %s ago", use.session, age(use.at)))
	}
	for _, other := range c.trees { // git worktree remove deletes the whole directory, a worktree inside it too
		if strings.HasPrefix(other.path, wt.path+"/") {
			return keep("holds the worktree " + other.path)
		}
	}
	if wt.prunable {
		notes = append(notes, "its directory is gone")
	} else {
		// every untracked file, whatever status.showUntrackedFiles says: git worktree remove would delete a hidden one
		status, err := cleanGit(wt.path, "status", "--porcelain", "--untracked-files=all")
		if err != nil {
			return keep("git status failed: " + err.Error())
		}
		if n := len(strings.Split(strings.TrimRight(status, "\n"), "\n")); status != "" {
			return keep(fmt.Sprintf("uncommitted changes (%d paths)", n))
		}
	}
	if _, err := cleanGit(c.main, "merge-base", "--is-ancestor", wt.head, c.mainRef); err == nil {
		v.remove, v.deleteBranch = true, wt.branch != ""
		v.reason = note("merged: its commits are on " + c.mainName)
		return v
	}
	if wt.branch == "" {
		return keep("detached HEAD with commits not on " + c.mainName)
	}
	pr, merged, ghErr := squashMerged(c.main, c.mainBranch, wt.branch, wt.head)
	if merged {
		v.remove, v.deleteBranch = true, true
		v.reason = note(fmt.Sprintf("squash-merged: PR #%d merged this branch at its tip", pr))
		return v
	}
	up := c.pushedRef(wt)
	if up == "" {
		return keep(fmt.Sprintf("commits not on %s, and the branch was never pushed%s", c.mainName, ghNote(ghErr)))
	}
	ahead, err := cleanGit(c.main, "rev-list", "--count", up+".."+wt.head)
	if err != nil {
		return keep(err.Error())
	}
	if ahead = strings.TrimSpace(ahead); ahead == "0" {
		v.remove = true
		v.reason = note("nothing beyond its pushed branch " + strings.TrimPrefix(up, "refs/remotes/") +
			"; the branch stays, as it is not merged")
		return v
	}
	return keep(fmt.Sprintf("%s commits not pushed to %s, and not on %s%s", ahead, strings.TrimPrefix(up, "refs/remotes/"),
		c.mainName, ghNote(ghErr)))
}

// pushedRef is where the branch was pushed: its upstream, unless that is the main branch (git worktree add -b from
// origin/main sets it so), else <remote>/<branch>; "" when neither exists.
func (c *cleaner) pushedRef(wt worktree) string {
	if up, err := cleanGit(c.main, "rev-parse", "--verify", "--quiet", "--symbolic-full-name",
		wt.branch+"@{upstream}"); err == nil && strings.TrimSpace(up) != c.mainRef {
		return strings.TrimSpace(up)
	}
	ref := "refs/remotes/" + c.remote + "/" + wt.branch
	if _, err := cleanGit(c.main, "rev-parse", "--verify", "--quiet", ref); err == nil {
		return ref
	}
	return ""
}

// ghNote says why a squash merge could not be checked: gh missing or failing keeps the worktree.
func ghNote(err error) string {
	if err == nil {
		return ""
	}
	return " (gh could not check for a squash merge: " + err.Error() + ")"
}

// remove applies one removal: the stale lock off, git worktree remove (never --force), the merged branch deleted.
func (c *cleaner) remove(v verdict) error {
	if v.unlock {
		if _, err := cleanGit(c.main, "worktree", "unlock", v.wt.path); err != nil {
			return err
		}
	}
	// a missing directory too: git drops its entry, and leaves every other worktree's alone
	if _, err := cleanGit(c.main, "worktree", "remove", v.wt.path); err != nil {
		return err
	}
	if v.deleteBranch {
		// merged into the main branch at the tip judged above, so only that tip is deleted; git branch -d would
		// compare with this checkout's HEAD instead
		if _, err := cleanGit(c.main, "update-ref", "-d", "refs/heads/"+v.wt.branch, v.wt.head); err != nil {
			return err
		}
	}
	return nil
}

// listWorktrees is git worktree list --porcelain -z, parsed.
func listWorktrees(dir string) ([]worktree, error) {
	out, err := cleanGit(dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var trees []worktree
	var wt *worktree
	for _, field := range strings.Split(out, "\x00") {
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			trees = append(trees, worktree{path: value})
			wt = &trees[len(trees)-1]
		case "HEAD":
			wt.head = value
		case "branch":
			wt.branch = strings.TrimPrefix(value, "refs/heads/")
		case "locked":
			wt.locked, wt.lockReason = true, value
		case "prunable":
			wt.prunable = true
		}
	}
	if len(trees) == 0 {
		return nil, refuse("git worktree list listed no worktree in %s", dir)
	}
	return trees, nil
}

// claudeLock is the reason Claude Code locks a worktree with: claude <kind> <name> (pid N start T), T the process's
// start time in clock ticks since boot (/proc/<pid>/stat's 22nd field), or (pid N) where it has none.
var claudeLock = regexp.MustCompile(`^claude \S+ .*\(pid (\d+)(?: start (\S+))?\)\s*$`)

// lockOwner names who holds a lock, and whether the lock is stale: Claude Code's lock with its process gone, or with
// its pid now another process's (another start time). Any other lock is someone's, so never stale.
func lockOwner(reason string) (owner string, stale bool) {
	m := claudeLock.FindStringSubmatch(strings.TrimSpace(reason))
	if m == nil {
		if reason == "" {
			return "locked (no reason given)", false
		}
		return "locked: " + reason, false
	}
	pid, _ := strconv.Atoi(m[1])
	switch start, ok := procStart(pid); {
	case !ok:
		return fmt.Sprintf("stale lock: pid %s is gone", m[1]), true
	case m[2] != "" && start != "" && start != m[2]:
		return fmt.Sprintf("stale lock: pid %s is another process now (started %s, not %s)", m[1], start, m[2]), true
	}
	return fmt.Sprintf("locked by live process pid %s", m[1]), false
}

// procStart is whether pid is alive and, on Linux, its start time from /proc/<pid>/stat. Elsewhere (macOS) the start
// time is "": a live pid counts as the lock's owner.
func procStart(pid int) (start string, alive bool) {
	if pid <= 0 {
		return "", false
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		// fields after the command name, which is in parentheses and may hold spaces: state is field 3, start 22
		if i := bytes.LastIndexByte(b, ')'); i >= 0 {
			if f := strings.Fields(string(b[i+1:])); len(f) > 19 {
				return f[19], true
			}
		}
		return "", true
	} else if _, err := os.Stat("/proc/self/stat"); err == nil {
		return "", false // a /proc without this pid: it is gone
	}
	err := syscall.Kill(pid, 0)
	return "", err == nil || errors.Is(err, syscall.EPERM)
}

// liveUse is a live session's latest use of a worktree.
type liveUse struct {
	session string
	at      time.Time
}

// liveUses finds, for each worktree under .claude/worktrees/, the latest live session that used it: an entry of a
// Claude Code transcript written within LiveWindow (pm's notion of a live session) whose cwd is in the worktree, or
// whose tool call names its path (a subagent runs in its parent's directory and reaches its worktree by path). The
// path's form relative to the main checkout counts when the entry's cwd is in the main checkout. The output of a
// tool call does not count, so listing worktrees, or pm clean's own output, uses none of them.
func liveUses(trees []worktree, main string) (map[string]liveUse, error) {
	type token struct{ path, abs, rel string }
	var tokens []token
	for _, wt := range trees {
		if rel, err := filepath.Rel(main, wt.path); err == nil && strings.HasPrefix(rel, agentWorktrees+"/") {
			tokens = append(tokens, token{wt.path, wt.path, rel})
		}
	}
	uses := map[string]liveUse{}
	if len(tokens) == 0 {
		return uses, nil
	}
	now := time.Now()
	projects := filepath.Join(claudeConfigDir(), "projects")
	err := filepath.WalkDir(projects, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			if p == projects && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll // no transcripts at all
			}
			return err
		}
		if e.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		st, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil // removed since it was listed
		} else if err != nil {
			return err
		}
		if now.Sub(st.ModTime()) >= LiveWindow {
			return nil
		}
		f, err := os.Open(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for sc.Scan() {
			var entry struct {
				Type      string `json:"type"`
				Timestamp string `json:"timestamp"`
				Cwd       string `json:"cwd"`
				SessionID string `json:"sessionId"`
				Message   struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(sc.Bytes(), &entry) != nil {
				continue // not an entry: Claude Code writes one JSON object per line
			}
			at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil || now.Sub(at) >= LiveWindow {
				continue
			}
			var calls []byte // the tool calls' inputs: what the session did, never what it was shown
			if entry.Type == "assistant" {
				var content []struct {
					Type  string          `json:"type"`
					Input json.RawMessage `json:"input"`
				}
				if json.Unmarshal(entry.Message.Content, &content) == nil {
					for _, c := range content {
						if c.Type == "tool_use" {
							calls = append(append(calls, c.Input...), '\n')
						}
					}
				}
			}
			inMain := entry.Cwd == main || strings.HasPrefix(entry.Cwd, main+"/")
			for _, t := range tokens {
				used := entry.Cwd == t.abs || strings.HasPrefix(entry.Cwd, t.abs+"/") || namesPath(calls, t.abs) ||
					(inMain && namesPath(calls, t.rel))
				if used && at.After(uses[t.path].at) {
					uses[t.path] = liveUse{entry.SessionID, at}
				}
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		return nil
	})
	return uses, err
}

// namesPath is whether text names path itself or a path under it: an occurrence not followed by a name character, so
// .claude/worktrees/a names neither .claude/worktrees/ab nor .claude/worktrees/a.b.
func namesPath(text []byte, path string) bool {
	for rest := text; ; {
		i := bytes.Index(rest, []byte(path))
		if i < 0 {
			return false
		}
		rest = rest[i+len(path):]
		if len(rest) == 0 || !isNameByte(rest[0]) {
			return true
		}
	}
}

func isNameByte(b byte) bool {
	return b == '.' || b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
		b >= 0x80
}

// squashMerged is whether a merged PR of branch has head as its tip: the branch's work reached the main branch through
// it. err is set when gh is missing or fails, so the merge could not be checked.
func squashMerged(main, base, branch, head string) (pr int, merged bool, err error) {
	res, err := proc.Run([]string{"gh", "pr", "list", "--head", branch, "--base", base, "--state", "merged",
		"--json",
		"number,headRefOid", "--limit", "100"}, proc.Options{Cwd: &main, Timeout: 30 * time.Second, TimeoutText: "30"})
	if err != nil {
		return 0, false, err
	}
	if res.Code != 0 {
		return 0, false, errors.New(firstLine(res.Stderr))
	}
	var prs []struct {
		Number     int    `json:"number"`
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &prs); err != nil {
		return 0, false, fmt.Errorf("gh pr list printed no JSON list: %v", err)
	}
	for _, p := range prs {
		if p.HeadRefOid == head {
			return p.Number, true, nil
		}
	}
	return 0, false, nil
}

// cleanGit is git's stdout in dir, or an error with its stderr's first line.
func cleanGit(dir string, args ...string) (string, error) {
	res, err := proc.Run(append([]string{"git"}, args...), proc.Options{Cwd: &dir})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		msg := firstLine(res.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit %d", res.Code)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return res.Stdout, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// realPath is p with symlinks resolved, or p when it does not exist.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
