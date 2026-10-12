package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
)

// ReplyAuthor is the author of the comment that holds an owner's site reply (the work store's import maps bd's
// "owner (site reply)" to it).
const ReplyAuthor = "owner"

var (
	MergePoll   = 60 * time.Second // between the service's looks at GitHub for open reviews' merges to main
	PushTimeout = 5 * time.Second  // how long a session's inbox has to take a message
	GHTimeout   = 60 * time.Second // a gh or git call of the merge watch, before it counts as not merged yet
)

// kindWord is what a need waits for as its messages name it: an action (a review is one) or a decision.
func kindWord(n *work.Item) string {
	if n.Need != nil && (n.Need.Kind == work.Action || n.Need.Kind == work.Review) {
		return "action"
	}
	return "decision"
}

// reviewPR is the PR an open review waits on; "" for any other item.
func reviewPR(n *work.Item) string {
	if n.Status == work.Closed || n.Need == nil || n.Need.Review == nil {
		return ""
	}
	return n.Need.Review.PR
}

// siteReplies is a need's owner replies, oldest first: its comments of kind reply made before it closed. The reply
// work.Answer writes as a session records the answer (pm decision add --need, pm decision close) is stamped with the
// close itself and is the session's own: it is no reply to deliver, as bd's "Response:" note is none for Python pm.
func siteReplies(n *work.Item) []work.Comment {
	var out []work.Comment
	for _, c := range n.Comments {
		if c.Kind == work.Reply && (n.Status != work.Closed || c.CreatedAt.Before(n.ClosedAt)) {
			out = append(out, c)
		}
	}
	return out
}

// ReplyWaiting is whether an open need holds a reply not delivered yet: more replies than its delivered count.
func ReplyWaiting(n *work.Item) bool {
	return n.Status != work.Closed && n.Need != nil && len(siteReplies(n)) > n.Need.Delivered
}

// MergeWaiting is the merge commit of a review's PR that the service saw but that has not reached the session; "".
func MergeWaiting(n *work.Item) string {
	if n.Need == nil || n.Need.Review == nil {
		return ""
	}
	r := n.Need.Review
	if r.Merged != "" && r.MergeReported != r.Merged {
		return r.Merged
	}
	return ""
}

var markLine = regexp.MustCompile(`\s*<!-- pm-reply [A-Za-z0-9-]+ -->\s*$`)

// ReplyBody is a reply's comment text as the owner wrote it, without its mark line.
func ReplyBody(text string) string { return markLine.ReplaceAllString(text, "") }

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote is Python's shlex.quote.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Texts are what a delivery says: they need the clone's main checkout and its remote and main branch.
type Texts struct{ Main, Remote, MainBranch string }

// pullMain is the command that fast-forwards this clone's main checkout, which hooks, rules and links read.
func (t Texts) pullMain() string {
	return fmt.Sprintf("git -C %s pull --ff-only %s %s", shellQuote(t.Main), t.Remote, t.MainBranch)
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// replyText is the message for a need's undelivered replies, and what the agent does next.
func (t Texts) replyText(n *work.Item, replies []work.Comment) string {
	var said []string
	for _, c := range replies {
		said = append(said, fmt.Sprintf("  [%s] ", stamp(c.CreatedAt))+strings.ReplaceAll(ReplyBody(c.Text), "\n", "\n  "))
	}
	k, id := kindWord(n), n.ID
	var next string
	switch {
	case n.Status == work.Closed:
		next = "it is closed already; check the reply is handled"
	case k == "decision":
		next = fmt.Sprintf("record the answer: pm decision add --need %s --level … --decision '<the answer>' "+
			"--reason '<why>' if it sets a rule, else pm decision close %s --reason \"<why it sets no rule>\" "+
			"--text-file - <<'EOF' (the answer, then EOF)", id, id)
	default:
		next = fmt.Sprintf("check the evidence, then pm action done %s --reason \"<what you saw>\", or ask again if it "+
			"falls short", id)
		if reviewPR(n) != "" {
			next += "; once its PR is on main, update the main checkout: " + t.pullMain()
		}
	}
	return fmt.Sprintf("pm: owner reply to %s %s (%s), relayed from the site:\n%s\nnext: %s", k, id, n.Title,
		strings.Join(said, "\n"), next)
}

var prNumber = regexp.MustCompile(`/pull/(\d+)`)

func (t Texts) mergeText(n *work.Item, sha string) string {
	pr := "?"
	if n.Need != nil && n.Need.Review != nil && n.Need.Review.PR != "" {
		pr = n.Need.Review.PR
	}
	if m := prNumber.FindStringSubmatch(pr); m != nil {
		pr = "#" + m[1]
	}
	return fmt.Sprintf("pm: PR %s of review %s (%s) merged to %s as %s\nnext: pm action done %s --reason \"merged as "+
		"%s\", then update the main checkout: %s", pr, n.ID, n.Title, t.MainBranch, sha, n.ID, sha, t.pullMain())
}

// Undelivered is what of a need has not reached its session, the replies since its delivered count and a merge not
// reported: the message, and the update that marks it delivered (nil when there is nothing).
func (t Texts) Undelivered(n *work.Item) (string, *work.NeedUpdate) {
	var parts []string
	var u work.NeedUpdate
	if n.Need == nil {
		return "", nil
	}
	replies := siteReplies(n)
	if len(replies) > n.Need.Delivered {
		parts = append(parts, t.replyText(n, replies[n.Need.Delivered:]))
		total := len(replies)
		u.Delivered = &total
	}
	if sha := MergeWaiting(n); sha != "" {
		parts = append(parts, t.mergeText(n, sha))
		u.ReviewMergeReported = &sha
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n"), &u
}

// Since is a need's site replies stamped at or after since, as Undelivered words them, delivered or not; "" when it
// has none. The store stamps to the second, so since counts from its second.
func (t Texts) Since(n *work.Item, since time.Time) string {
	var replies []work.Comment
	for _, c := range siteReplies(n) {
		if !c.CreatedAt.Before(since.Truncate(time.Second)) {
			replies = append(replies, c)
		}
	}
	if len(replies) == 0 {
		return ""
	}
	return t.replyText(n, replies)
}

// ReplyPoll is how often WaitReply reads the need.
var ReplyPoll = 2 * time.Second

// WaitReply is pm reply wait: block until need id has a new reply, from start for at most timeout, then hand it over:
// what has not reached a session (a reply, or its review's merge), marked delivered, as pm reply read does, else the
// replies stamped since the wait began, which a push or pm hook inbox delivered meanwhile. A reply delivered before
// the wait is no new reply: an agent that asks again after "not yet" waits for the next one. It refuses an id that is
// no need, at the timeout, and for a need closed with no new reply.
func (t Texts) WaitReply(st work.Store, id string, timeout time.Duration, start time.Time) (string, error) {
	if all, err := st.Items(); err != nil { // once: an unknown id is a refusal, not a store error
		return "", err
	} else if !slices.ContainsFunc(all, func(it work.Item) bool { return it.ID == id && it.Type == work.Need }) {
		return "", fmt.Errorf("%s is not a request to the owner (a need in the work store)", id)
	}
	deadline := start.Add(timeout)
	for {
		got, err := st.Get(id)
		if err != nil {
			return "", err
		}
		need := &got[0]
		if text, mark := t.Undelivered(need); text != "" {
			return text, st.UpdateNeed(id, *mark)
		}
		if text := t.Since(need, start); text != "" { // delivered while it waited: a push, or pm hook inbox
			return text, nil
		}
		if need.Status == work.Closed {
			return "", fmt.Errorf("%s is closed (%s), so no reply will come", id, need.Resolution)
		}
		left := time.Until(deadline)
		if left <= 0 {
			return "", fmt.Errorf("no reply to %s within %s; it is still open: wait again with pm reply wait %s, or "+
				"the reply reaches this session at its next prompt", id, timeout, id)
		}
		time.Sleep(min(ReplyPoll, left))
	}
}

// Mark is the mark that delivers one need's replies.
type Mark struct {
	ID     string
	Update work.NeedUpdate
}

// SessionUndelivered is what of the session's open needs has not reached it, each as the push writes it, and the
// marks that deliver it: pm hook inbox's read.
func (t Texts) SessionUndelivered(st work.Store, session string) ([]string, []Mark, error) {
	needs, err := st.Needs(session)
	if err != nil {
		return nil, nil, err
	}
	var texts []string
	var marks []Mark
	for i := range needs {
		if needs[i].Status == work.Closed {
			continue
		}
		if text, u := t.Undelivered(&needs[i]); text != "" {
			texts = append(texts, text)
			marks = append(marks, Mark{needs[i].ID, *u})
		}
	}
	return texts, marks, nil
}

// MarkWithin makes the marks, one write per need (the store has no batch write), all within budget: lockWait gets
// what is left before each write, as the most it may wait for the write lock. What is not marked by the budget's end
// stays undelivered, and the error says how many.
func MarkWithin(st work.Store, marks []Mark, budget time.Duration, now func() time.Time,
	lockWait func(time.Duration)) error {
	deadline := now().Add(budget)
	var errs []error
	for i, m := range marks {
		left := deadline.Sub(now())
		if left <= 0 {
			return errors.Join(append(errs, fmt.Errorf("%d of %d left undelivered past %s", len(marks)-i, len(marks),
				budget))...)
		}
		lockWait(left)
		errs = append(errs, st.UpdateNeed(m.ID, m.Update))
	}
	return errors.Join(errs...)
}

// Hostname is this machine's name, as a need stores its inbox's host; a variable so tests can move a need elsewhere.
var Hostname = os.Hostname

// SessionLive is whether a session's transcript changed within the live window, and CodexLive whether a Codex
// session's rollout under the CODEX_HOME its need stores did; variables so tests can end one.
var (
	SessionLive = session.Live
	CodexLive   = session.CodexLive
)

// PushInbox hands text, as one user message, to the session that raised the need, only on the machine that stored its
// inbox: written into a Claude Code session's inbox socket, connecting only once the line is ready, or queued into a
// live Codex session with codex queue (queueCodex). "" once handed over; else why not: "no push: …" (a live session
// that stores no inbox: the reply waits for it), "no codex: …" (codex is not on the service's PATH), "not running: …"
// (no session, or one whose transcript has ended, another machine, the socket gone or refusing) or "failed: …".
func PushInbox(n *work.Item, text string) string {
	if n.Need == nil || n.Need.RaisedBy == nil || n.Need.RaisedBy.Session == "" {
		return "not running: the request names no session"
	}
	rb := n.Need.RaisedBy
	if rb.Inbox == "" {
		if !SessionLive(rb.Session) {
			return "not running: the session that asked has ended"
		}
		return "no push: the session that asked gets no push; it sees the reply at its next prompt, or now if it " +
			"waits with pm reply wait"
	}
	if host, _ := Hostname(); rb.Host != host {
		where := rb.Host
		if where == "" {
			where = "an unknown host"
		}
		return "not running: the session's inbox is on " + where + ", not this machine"
	}
	if home, codex := session.CodexHomeOf(rb.Inbox); codex {
		if !CodexLive(home, rb.Session) {
			return "not running: the session that asked has ended"
		}
		return queueCodex(home, rb.Session, text)
	}
	msg := &pyjson.Object{Keys: []string{"role", "content"}, Values: map[string]any{"role": "user", "content": text}}
	line := pyjson.Dumps(&pyjson.Object{Keys: []string{"type", "message"},
		Values: map[string]any{"type": "user", "message": msg}}, true) + "\n"
	conn, err := net.DialTimeout("unix", rb.Inbox, PushTimeout)
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return "not running: the session is not running"
	}
	if err != nil {
		return "failed: " + err.Error()
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(PushTimeout))
	if _, err := conn.Write([]byte(line)); err != nil {
		return "failed: " + err.Error()
	}
	return ""
}

// pushUndelivered is the one delivery path, for replies and merges alike: push what of the need has not reached its
// session into that session's inbox, and mark it delivered only once the push went through. It never holds the store
// across the push. "delivered", "nothing to deliver", or why not ("not running: …", "failed: …"); the need then stays
// flagged by pm show, and the sweep tries it again.
func (s *server) pushUndelivered(id string) (string, error) {
	var n work.Item
	if err := s.withStore("read "+id, func(st work.Store) error {
		got, err := st.Get(id)
		if err == nil {
			n = got[0]
		}
		return err
	}); err != nil {
		return "", err
	}
	text, mark := s.texts.Undelivered(&n)
	if text == "" {
		return "nothing to deliver", nil
	}
	if why := PushInbox(&n, text); why != "" {
		return why, nil
	}
	if err := s.withStore("delivered "+id, func(st work.Store) error { return st.UpdateNeed(id, *mark) }); err != nil {
		return "", err
	}
	return "delivered", nil
}

// run runs a merge-watch command in dir with GHTimeout; timedOut when it ran past it.
func run(dir string, argv ...string) (code int, stdout, stderr string, timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), GHTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Dir = dir
	var out, errb strings.Builder
	c.Stdout, c.Stderr = &out, &errb
	c.WaitDelay = time.Second
	err = c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, "", "", true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), out.String(), errb.String(), false, nil
	}
	return 0, out.String(), errb.String(), false, err
}

// mergedOnMain is the merge commit of pr once it is merged and on the remote's main branch (a PR merged into a
// stacked base is not); "" otherwise. A gh or git failure is logged and counts as not merged yet, so the wait goes on.
func (s *server) mergedOnMain(pr string) string {
	main, remote, branch := s.d.Main, s.d.Remote, s.d.MainBranch
	code, out, errs, late, err := run(main, "gh", "pr", "view", pr, "--json", "state,mergeCommit")
	switch {
	case late:
		s.logf("warning: gh pr view %s took over %ds, still waiting", pr, int(GHTimeout.Seconds()))
		return ""
	case err != nil:
		s.logf("warning: gh pr view %s did not run, still waiting: %v", pr, err)
		return ""
	case code != 0:
		s.logf("warning: gh pr view %s failed, still waiting: %s", pr, strings.TrimSpace(errs))
		return ""
	}
	var info struct {
		State       string `json:"state"`
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		s.logf("warning: gh pr view %s printed no JSON, still waiting: %v", pr, err)
		return ""
	}
	if info.State != "MERGED" || info.MergeCommit == nil || info.MergeCommit.OID == "" {
		return ""
	}
	sha := info.MergeCommit.OID
	code, _, errs, late, err = run(main, "git", "fetch", "--quiet", remote, branch)
	switch {
	case late:
		s.logf("warning: git fetch %s %s took over %ds, still waiting", remote, branch, int(GHTimeout.Seconds()))
		return ""
	case err != nil || code != 0:
		why := strings.TrimSpace(errs)
		if err != nil {
			why = err.Error()
		}
		s.logf("warning: git fetch %s %s failed, still waiting: %s", remote, branch, why)
		return ""
	}
	if code, _, _, _, err := run(main, "git", "merge-base", "--is-ancestor", sha, remote+"/"+branch); err != nil || code != 0 {
		return ""
	}
	return sha
}
