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
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/pyjson"
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

// Hostname is this machine's name, as a need stores its inbox's host; a variable so tests can move a need elsewhere.
var Hostname = os.Hostname

// PushInbox writes text as one user message into the inbox socket of the session that raised the need, connecting
// only once the line is ready, and only on the machine that stored it. "" once the line is written; else why not:
// "not running: …" (no inbox stored, another machine, the socket gone or refusing) or "failed: …".
func PushInbox(n *work.Item, text string) string {
	if n.Need == nil || n.Need.RaisedBy == nil || n.Need.RaisedBy.Inbox == "" {
		return "not running: the request stores no session inbox"
	}
	rb := n.Need.RaisedBy
	if host, _ := Hostname(); rb.Host != host {
		where := rb.Host
		if where == "" {
			where = "an unknown host"
		}
		return "not running: the session's inbox is on " + where + ", not this machine"
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
	if err := s.withStore("read "+id, func(st Store) error {
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
	if err := s.withStore("delivered "+id, func(st Store) error { return st.UpdateNeed(id, *mark) }); err != nil {
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
