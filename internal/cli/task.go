package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"regexp"
	"strings"
	"time"
)

func cmdTaskAdd(e *env, p *Parsed) (string, error) {
	title := strip(p.Get("title"))
	if title == "" {
		return "", refuse("--title is empty")
	}
	desc := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprintID := p.Get("sprint")
	if _, err := r.openSprint(sprintID); err != nil {
		return "", err
	}
	if err := r.checkPlanned(nil, r.with(planned("pm-planned-task", work.Task, sprintID, title))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Task, Parent: sprintID, Title: title, Description: desc})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("created task %s in sprint %s; claim it with pm task claim %s", made.ID, sprintID, made.ID), nil
}

func cmdTaskClose(e *env, p *Parsed) (string, error) {
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("task_id")
	task, err := r.openTask(id)
	if err != nil {
		return "", err
	}
	if task.Type == work.Need {
		return "", refuse("%s is a need; answer a decision with pm decision add --need %s (or pm "+
			"decision close if the answer sets no rule), close an action with pm action done %s", id, id, id)
	}
	if err := heldElsewhere(id, task, currentSession(),
		" to close it, or to release it with pm task release "+id); err != nil {
		return "", err
	}
	reason := strip(p.Get("reason"))
	ref, _ := given(p, "commit")
	resolution := work.Done
	if p.Get("dropped") != "" {
		if ref != "" {
			return "", refuse("--dropped closes %s as not done, so no commit holds its work; drop --commit", id)
		}
		if reason == "" {
			return "", refuse("--dropped needs --reason: why %s is dropped and not done", id)
		}
		resolution = work.Dismissed
	} else {
		if reason == "" {
			reason = "Done"
		}
		commit := ""
		if ref != "" {
			if commit, err = closeCommit(r.root, ref); err != nil {
				return "", err
			}
		} else {
			// HEAD names the work only if it was committed after the task started.
			started := task.StartedAt
			if started.IsZero() {
				started = task.CreatedAt
			}
			if out, err := store.Git(r.root, "log", "-1", "--format=%h %cI"); err == nil {
				head, when, _ := strings.Cut(out, " ")
				if t, err := time.Parse(time.RFC3339, when); err == nil && head != "" && !t.Before(started) {
					commit = "commit " + head
				}
			}
			if commit == "" {
				fmt.Fprintf(e.stderr, "warning: no commit since %s started, so the reason names none; name one with "+
					"--commit if the work is committed\n", id)
			}
			// HEAD stands in for the work, so a dirty tree may hold work it lacks; a named commit is the work.
			status, err := store.Git(r.root, "status", "--porcelain")
			if err != nil {
				return "", err
			}
			if status != "" {
				fmt.Fprintf(e.stderr, "warning: the working tree has uncommitted changes; the commit may not contain "+
					"the work of %s\n", id)
			}
		}
		if commit != "" {
			reason += " (" + commit + ")"
		}
	}
	closed := closedCopy(task, reason)
	closed.Resolution = resolution
	if err := r.checkPlanned(nil, r.with(closed)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(id, reason, resolution, closer(task)); err != nil {
		return "", err
	}
	if resolution == work.Dismissed {
		return fmt.Sprintf("dropped %s: %s", id, reason), nil
	}
	return fmt.Sprintf("closed %s: %s", id, reason), nil
}

var (
	otherRepoCommit = regexp.MustCompile(`^([\w.-]+/[\w.-]+)@([0-9a-fA-F]{4,40})$`)
	pullURL         = regexp.MustCompile(`^https://github\.com/([\w.-]+/[\w.-]+)/pull/[0-9]+$`)
)

// closeCommit is how a close names the work --commit ref gives: "commit <short>" for a commit of this repo,
// "commit OWNER/REPO@<short>" for OWNER/REPO@SHA and "PR <url>" (with its merge commit once merged) for a PR URL, the
// last two resolved on GitHub with gh. A ref that does not resolve is refused.
func closeCommit(root, ref string) (string, error) {
	if out, err := store.Git(root, "rev-parse", "--verify", "--quiet", "--short", ref+"^{commit}"); err == nil {
		return "commit " + out, nil // this repo's first: a branch may be named like OWNER/REPO@SHA
	}
	if m := otherRepoCommit.FindStringSubmatch(ref); m != nil {
		var c struct {
			SHA string `json:"sha"`
		}
		if err := proc.GHJSON(root, &c, "api", "repos/"+m[1]+"/commits/"+m[2]); err != nil {
			return "", refuse("--commit %s is not a commit of %s on GitHub: %v", ref, m[1], err)
		}
		if len(c.SHA) < 7 {
			return "", refuse("--commit %s is not a commit of %s on GitHub: gh api gave no sha", ref, m[1])
		}
		return "commit " + m[1] + "@" + c.SHA[:7], nil
	}
	if m := pullURL.FindStringSubmatch(strings.TrimSuffix(ref, "/")); m != nil {
		ref = strings.TrimSuffix(ref, "/")
		var pr struct {
			State       string `json:"state"`
			MergeCommit *struct {
				OID string `json:"oid"`
			} `json:"mergeCommit"`
		}
		if err := proc.GHJSON(root, &pr, "pr", "view", ref, "--json", "state,mergeCommit"); err != nil {
			return "", refuse("--commit %s is not a pull request on GitHub: %v", ref, err)
		}
		switch {
		case pr.State == "MERGED" && pr.MergeCommit != nil && len(pr.MergeCommit.OID) >= 7:
			return "PR " + ref + ", merged as " + m[1] + "@" + pr.MergeCommit.OID[:7], nil
		case pr.State == "CLOSED":
			return "", refuse("--commit %s was closed without merging, so it holds no work; name the commit or PR "+
				"that does, or close the task with --dropped", ref)
		}
		return "PR " + ref, nil
	}
	return "", refuse("--commit %s is not a commit of this repo, an OWNER/REPO@SHA on GitHub or a PR URL", ref)
}

func cmdTaskClaim(e *env, p *Parsed) (string, error) {
	id := p.Get("task_id")
	sid := strip(p.Get("session"))
	if sid == "" {
		sid = currentSession()
	}
	if sid == "" {
		return "", noSession()
	}
	root, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	dirs, err := store.Git(root, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gitDir, common, _ := strings.Cut(dirs, "\n")
	if fsutil.Resolved(gitDir) == fsutil.Resolved(common) { // the main checkout; a linked worktree has its own git dir
		cfg, err := config.Load(e.here)
		if err != nil {
			return "", err
		}
		return "", refuse("not claiming %s here: %s is the main checkout; agents change code only in a worktree of their "+
			"own. Make one with git, in any runtime, and work there: `git -C %s fetch %s %s && git -C %s worktree add -b "+
			"<branch> %s/<name> %s/%s`, then `cd %s/%s/<name>` (in Claude Code, the EnterWorktree tool does the same); "+
			"session start links its records/ (else run `pm init` there). A subagent works "+
			"in its parent's worktree.",
			id, root, root, cfg.Remote, cfg.MainBranch, root, worktreesDir, cfg.Remote, cfg.MainBranch, root, worktreesDir)
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	task, err := r.openTask(id)
	if err != nil {
		return "", err
	}
	if err := heldElsewhere(id, task, sid, ""); err != nil {
		return "", err
	}
	h := holderOf(task)
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	// Python pm records no host; the host comes with liveness across hosts (work-store page, Open questions).
	if err := ws.Claim(id, work.Holder{Session: sid}, session.Live); err != nil {
		return "", err
	}
	claimed, err := ws.Get(id) // the claim time the store stamped
	if err != nil {
		return "", err
	}
	now := claimed[0].Holder.ClaimedAt.UTC().Format("2006-01-02T15:04:05Z")
	was := ""
	if h != nil && h.Session != sid {
		was = "; took it over from idle session " + h.Session
	}
	return fmt.Sprintf("claimed %s for session %s at %s%s", id, sid, now, was), nil
}

// heldElsewhere refuses a write to task id while a live session other than sid holds it; then says what to ask that
// session for.
func heldElsewhere(id string, task *work.Item, sid, then string) error {
	h := holderOf(task)
	if h != nil && h.Session != sid && h.Live {
		return refuse("%s is held by live session %s (claimed %s ago; its transcript was written in the last %d "+
			"minutes); leave it, or ask that session%s", id, h.Session, session.Age(h.ClaimedAt), liveWindow/60, then)
	}
	return nil
}
