package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"sort"
	"strings"
	"time"
)

func noteAuthor() string {
	if s := currentSession(); s != "" {
		return s
	}
	return "owner"
}

func cmdActionDone(e *env, p *Parsed) (string, error) {
	reason := strip(p.Get("reason"))
	if reason == "" {
		return "", refuse("--reason is empty; say what showed you the owner did it")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	id := p.Get("need_id")
	need, err := r.humanIssue(id, "action")
	if err != nil {
		return "", err
	}
	if need.Status == work.Closed {
		return restampReview(e, r, need, reason)
	}
	if err := refuseUnread(need); err != nil {
		return "", err
	}
	if err := r.checkPlanned(nil, r.with(closedCopy(need, reason))); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.Close(id, reason, work.Done, closer(need)); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed action %s: %s", id, reason), nil
}

// restampReview gives a PR review closed without "merged as <sha>" (its PR merged into a stacked base) that reason
// once sha is on the remote's main, so its sprint can close; the old reason stays after it. Any other closed action
// is refused.
func restampReview(e *env, r *repo, need *work.Item, reason string) (string, error) {
	id, old := need.ID, need.CloseReason
	if need.Need == nil || need.Need.Kind != work.Review || need.Resolution != work.Done || mergedAs.MatchString(old) {
		return "", refuse("action %s is already closed", id)
	}
	m := mergedAs.FindStringSubmatch(reason)
	if m == nil {
		return "", refuse("review %s is closed as %s, without 'merged as <sha>'; once its work is on main, restamp it "+
			"with pm action done %s --reason \"merged as <sha>\"", id, pyRepr(old), id)
	}
	if _, err := onMain(e, "merged as", m[1]); err != nil {
		return "", err
	}
	reason += "; first closed as: " + old
	planned := *need
	planned.CloseReason = reason
	if err := r.checkPlanned(nil, r.with(planned)); err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	if err := ws.SetCloseReason(id, reason); err != nil {
		return "", err
	}
	return fmt.Sprintf("restamped review %s: %s", id, reason), nil
}

// ---------------------------------------------------------------- pm reply read

func cmdReplyRead(e *env, p *Parsed) (string, error) {
	if _, err := store.CodeRoot(e.here, e.records); err != nil {
		return "", err
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	var needs []work.Item
	if ids := dedupe(p.Values["ids"]); ids != nil {
		all, err := ws.Items()
		if err != nil {
			return "", err
		}
		byID := map[string]work.Item{}
		for _, it := range all {
			byID[it.ID] = it
		}
		for _, id := range ids {
			it, ok := byID[id]
			if !ok || it.Type != work.Need {
				return "", refuse("%s is not a request to the owner (a need in the work store)", id)
			}
			needs = append(needs, it)
		}
	} else {
		sid := currentSession()
		if sid == "" {
			return "", refuse("name the requests to read: pm reply read <id>…; without %s or %s pm cannot tell "+
				"which requests this session raised", sessionEnv, codexSessionEnv)
		}
		mine, err := ws.Needs(sid)
		if err != nil {
			return "", err
		}
		for _, it := range mine {
			if it.Status != work.Closed { // open ones only
				needs = append(needs, it)
			}
		}
		sort.SliceStable(needs, func(i, j int) bool { return needs[i].ID < needs[j].ID })
	}
	texts := service.Texts{Main: store.MainOf(e.records), Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	var out, names []string
	for i := range needs {
		names = append(names, needs[i].ID)
		text, mark := texts.Undelivered(&needs[i])
		if text == "" {
			continue
		}
		out = append(out, text)
		if err := ws.UpdateNeed(needs[i].ID, *mark); err != nil {
			return "", err
		}
	}
	if out == nil {
		what := strings.Join(names, ", ")
		if what == "" {
			what = "any open request of this session"
		}
		return "nothing undelivered on " + what, nil
	}
	return strings.Join(out, "\n"), nil
}

// ---------------------------------------------------------------- pm reply wait

// cmdReplyWait is pm reply wait: block until the need has a new reply, then print it: what has not reached a session
// (a reply, or its review's merge) marked delivered, as pm reply read does, else the replies stamped since the wait
// began, which a push or pm hook inbox delivered meanwhile. A reply delivered before the wait is no new reply: an
// agent that asks again after "not yet" waits for the next one. It refuses at the timeout, and for a need closed with
// no new reply.
func cmdReplyWait(e *env, p *Parsed) (string, error) {
	id, given := p.Get("id"), p.Get("timeout")
	if given == "" {
		given = "30m"
	}
	timeout, err := time.ParseDuration(given)
	if err != nil || timeout <= 0 {
		return "", refuse("--timeout %s is not a duration such as 30m or 90s", given)
	}
	if _, err := store.CodeRoot(e.here, e.records); err != nil {
		return "", err
	}
	cfg, err := config.Load(e.here)
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	texts := service.Texts{Main: store.MainOf(e.records), Remote: cfg.Remote, MainBranch: cfg.MainBranch}
	return texts.WaitReply(ws, id, timeout, time.Now())
}
