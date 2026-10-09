package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/work"
)

// The work-store commands with no Python counterpart: the work-store page's Commands table, "For agents" (task
// ready, edit and release, dep add and rm, comment add, need dismiss, reply add, sync, and two forms of commands Python
// has: pm show ID and pm task add --parent TASK). Each opens the store once, under the gate, and closes it at exit.
// Like pm export they stay out of the argparse tree, whose help and noun list the parity tests hold equal to Python
// pm's, until Python pm is deleted.

// storeCommand is one of them.
type storeCommand struct {
	usage string // after "pm "
	about string
	flags func(fs *pflag.FlagSet)
	// args is the number of positional arguments it takes.
	args int
	run  func(c *storeCall) error
}

// storeCall is one run of a store command: its parsed flags and arguments, the store, who runs it, and its output.
type storeCall struct {
	fs      *pflag.FlagSet
	args    []string
	d       *work.Dolt
	session string                    // this command's agent session; "" for the owner at a shell
	live    func(session string) bool // whether a session is live
	stdin   io.Reader
	stdout  io.Writer
}

var storeCommands = map[string]storeCommand{
	"task ready": {
		usage: "task ready [--sprint ID] [--json]",
		about: "The tasks an agent can claim now: open, under open ancestors, held by no live session, with no open " +
			"blocker of their own or an ancestor's. By sprint number, then id; tasks directly under a project last. A " +
			"task whose holder is not live is ready and marked a stale holder; pm task claim takes it over.",
		flags: func(fs *pflag.FlagSet) {
			fs.String("sprint", "", "only the tasks under this sprint")
			fs.Bool("json", false, "the ready tasks as a JSON array of items")
		},
		run: taskReady,
	},
	"task edit": {
		usage: "task edit ID [--title TITLE] [--text TEXT | --text-file FILE]",
		about: "Set a task's title, its description (the body: --text, or --text-file - <<'EOF' … EOF), or both. A " +
			"scope change is still pm task move or a sprint decision.",
		flags: func(fs *pflag.FlagSet) {
			fs.String("title", "", "the new title")
			textFlags(fs, "the new description")
		},
		args: 1,
		run:  taskEdit,
	},
	"task release": {
		usage: "task release ID",
		about: "Clear the holder of a task this session ($CLAUDE_CODE_SESSION_ID, else $CODEX_THREAD_ID) holds.",
		args:  1,
		run:   taskRelease,
	},
	"dep add": {
		usage: "dep add ID --on BLOCKER",
		about: "BLOCKER blocks ID: ID, and every task under it, waits until BLOCKER closes. Refuses a blocker that " +
			"does not exist and a cycle over blocked_by, ancestors included.",
		flags: onFlag,
		args:  1,
		run:   func(c *storeCall) error { return dep(c, true) },
	},
	"dep rm": {
		usage: "dep rm ID --on BLOCKER",
		about: "BLOCKER no longer blocks ID.",
		flags: onFlag,
		args:  1,
		run:   func(c *storeCall) error { return dep(c, false) },
	},
	"comment add": {
		usage: "comment add ID (--text TEXT | --text-file FILE)",
		about: "Add a note to an item: --text, or --text-file - <<'EOF' … EOF. Its author is this session, or owner " +
			"when no session runs the command.",
		flags: func(fs *pflag.FlagSet) { textFlags(fs, "the note") },
		args:  1,
		run:   commentAdd,
	},
	"need dismiss": {
		usage: "need dismiss ID --reason REASON",
		about: "Close an open need as dismissed, for a [TEST] need or a replaced review.",
		flags: func(fs *pflag.FlagSet) { fs.String("reason", "", "why it is dismissed (required)") },
		args:  1,
		run:   needDismiss,
	},
	"reply add": {
		usage: "reply add ID (--text TEXT | --text-file FILE)",
		about: "The owner's answer to an open need, at a shell: a reply, as a reply on the site writes. The need " +
			"stays open; the session that raised it reads the reply and records it, which closes the need.",
		flags: func(fs *pflag.FlagSet) { textFlags(fs, "the answer") },
		args:  1,
		run:   replyAdd,
	},
	"show ID": {
		usage: "show ID [--json]",
		about: "One item, any type: its fields, holder and whether that session is live, blockers, children, needs " +
			"and comments. Without ID, pm show prints project state level by level (pm show --help).",
		flags: func(fs *pflag.FlagSet) { fs.Bool("json", false, "the item as one JSON object, as pm export prints it") },
		args:  1,
		run:   showItem,
	},
	"task add --parent": {
		usage: "task add --parent TASK --title TITLE [--text TEXT | --text-file FILE]",
		about: "Add a sub-task under the open task TASK; a sprint's task is pm task add --sprint ID. Its description " +
			"is the body: --text, or --text-file - <<'EOF' … EOF.",
		flags: func(fs *pflag.FlagSet) {
			fs.String("parent", "", "the open task it is a part of (required)")
			fs.String("title", "", "the sub-task's title (required)")
			textFlags(fs, "the description")
		},
		run: subTaskAdd,
	},
	"sync": {
		usage: "sync",
		about: "Sync the work store with the repo's remote now: pull, resolve conflicts by the merge rules, push. A " +
			"conflict no rule settles fails, names the item and field, and leaves the store as it was.",
		run: syncNow,
	},
}

func onFlag(fs *pflag.FlagSet) { fs.String("on", "", "the blocker (required)") }

func textFlags(fs *pflag.FlagSet, what string) {
	fs.String("text", "", what+", as one argument")
	fs.String("text-file", "", what+", from a file; - reads stdin, from a pipe or heredoc only")
}

// storeCommandOf is the store command argv names, and its arguments after the name. pm show with an id first and pm
// task add with --parent are store commands; any other pm show or pm task add is the argparse tree's.
func storeCommandOf(argv []string) (string, []string, bool) {
	if len(argv) >= 2 && argv[0] == "show" && !strings.HasPrefix(argv[1], "-") {
		return "show ID", argv[1:], true
	}
	if len(argv) >= 2 && argv[0] == "task" && argv[1] == "add" {
		for _, a := range argv[2:] {
			if a == "--parent" || strings.HasPrefix(a, "--parent=") {
				return "task add --parent", argv[2:], true
			}
		}
		return "", nil, false
	}
	for _, n := range []int{2, 1} {
		if len(argv) >= n {
			name := strings.Join(argv[:n], " ")
			if _, ok := storeCommands[name]; ok {
				return name, argv[n:], true
			}
		}
	}
	return "", nil, false
}

// runStoreCommand parses args for the named command and runs it on the store open opens, closing it after.
func runStoreCommand(name string, args []string, open func() (*work.Dolt, error), stdin io.Reader,
	stdout io.Writer) error {
	sc := storeCommands[name]
	fs := pflag.NewFlagSet("pm "+name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if sc.flags != nil {
		sc.flags(fs)
	}
	usage := "usage: pm " + sc.usage
	if err := fs.Parse(args); errors.Is(err, pflag.ErrHelp) {
		fmt.Fprintf(stdout, "%s\n\n%s\n\noptions:\n%s", usage, sc.about, fs.FlagUsages())
		return nil
	} else if err != nil {
		return fmt.Errorf("%v\n%s", err, usage)
	}
	if fs.NArg() != sc.args {
		return fmt.Errorf("pm %s takes %d argument(s), not %d\n%s", name, sc.args, fs.NArg(), usage)
	}
	d, err := open()
	if err != nil {
		return err
	}
	c := &storeCall{fs: fs, args: fs.Args(), d: d, session: currentSession(), live: live, stdin: stdin,
		stdout: stdout}
	return errors.Join(sc.run(c), d.Shutdown())
}

func (c *storeCall) flag(name string) string {
	v, _ := c.fs.GetString(name)
	return v
}

// text is the body from --text or --text-file, and whether one was given; both, or an empty body, are refused.
func (c *storeCall) text() (string, bool, error) {
	t, f := c.fs.Changed("text"), c.fs.Changed("text-file")
	switch {
	case t && f:
		return "", false, errors.New("give the body with --text or --text-file, not both")
	case t:
		v := strings.TrimSpace(c.flag("text"))
		if v == "" {
			return "", false, errors.New("--text is empty")
		}
		return v, true, nil
	case f:
		v, err := readTextFile(c.flag("text-file"), c.stdin)
		if err == nil && v == "" {
			err = fmt.Errorf("--text-file %s is empty", c.flag("text-file"))
		}
		return v, err == nil, err
	}
	return "", false, nil
}

// readTextFile is the body --text-file names. - reads stdin, but only a pipe or a file (a heredoc is one): an agent's
// shell may hold stdin open as a socket or tty that never ends, so anything else is refused without reading.
func readTextFile(path string, stdin io.Reader) (string, error) {
	if path != "-" {
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("--text-file %s: no such file", path)
		}
		if err != nil {
			return "", fmt.Errorf("--text-file %s: %w", path, err)
		}
		return config.PyStrip(string(b)), nil
	}
	if f, ok := stdin.(*os.File); ok {
		st, err := f.Stat()
		if err != nil || st.Mode()&(os.ModeNamedPipe) == 0 && !st.Mode().IsRegular() {
			return "", errors.New("--text-file - reads stdin, which here is not a pipe or a file; pass the body " +
				"with a quoted heredoc: pm … --text-file - <<'EOF' … EOF")
		}
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("--text-file -: %w", err)
	}
	return config.PyStrip(string(b)), nil
}

// item is the item with this id, refused when it is missing or not of type want.
func (c *storeCall) item(id string, want work.Type) (work.Item, error) {
	items, err := c.d.Get(id)
	if err != nil {
		return work.Item{}, err
	}
	if items[0].Type != want {
		return work.Item{}, fmt.Errorf("%s is a %s, not a %s", id, items[0].Type, want)
	}
	return items[0], nil
}

func taskReady(c *storeCall) error {
	items, err := c.d.Items()
	if err != nil {
		return err
	}
	x, err := work.NewIndex(items)
	if err != nil {
		return err
	}
	sprint := c.flag("sprint")
	if sprint != "" {
		if s := x.Item(sprint); s == nil || s.Type != work.Sprint {
			return fmt.Errorf("--sprint %s names no sprint", sprint)
		}
	}
	ready := []work.Item{}
	for _, it := range x.ReadyTasks(c.live) {
		if sprint == "" || contains(x.Ancestors(it.ID), sprint) {
			ready = append(ready, it)
		}
	}
	if json, _ := c.fs.GetBool("json"); json {
		return writeJSON(c.stdout, ready)
	}
	if len(ready) == 0 {
		fmt.Fprintln(c.stdout, "no ready tasks")
		return nil
	}
	for _, it := range ready {
		line := it.ID + "  " + it.Title
		if it.Holder != nil {
			line += fmt.Sprintf("  (stale holder %s, claimed %s)", short(it.Holder.Session),
				it.Holder.ClaimedAt.Format(time.RFC3339))
		}
		fmt.Fprintln(c.stdout, line)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func short(session string) string {
	if len(session) > 8 {
		return session[:8]
	}
	return session
}

// showItem is pm show ID: the item, then its children, needs and comments.
func showItem(c *storeCall) error {
	items, err := c.d.Items()
	if err != nil {
		return err
	}
	x, err := work.NewIndex(items)
	if err != nil {
		return err
	}
	id := c.args[0]
	it := x.Item(id)
	if it == nil {
		return fmt.Errorf("no item %s in the work store", id)
	}
	if asJSON, _ := c.fs.GetBool("json"); asJSON {
		return writeJSON(c.stdout, it)
	}
	line := func(i *work.Item) string {
		state := string(i.Status)
		if i.Resolution != "" {
			state += " (" + string(i.Resolution) + ")"
		}
		return fmt.Sprintf("%s  %s  %s  %s", i.ID, i.Type, state, i.Title)
	}
	stamp := func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }
	out := []string{line(it)}
	if p := x.Item(it.Parent); p != nil {
		out = append(out, "parent: "+line(p))
	}
	if it.Holder != nil {
		liveness := "not live"
		if c.live(it.Holder.Session) {
			liveness = "live"
		}
		out = append(out, fmt.Sprintf("holder: session %s (%s), claimed %s", it.Holder.Session, liveness,
			stamp(it.Holder.ClaimedAt)))
	}
	for _, b := range it.BlockedBy {
		if bi := x.Item(b); bi != nil {
			out = append(out, "blocked by: "+line(bi))
		} else {
			out = append(out, "blocked by: "+b)
		}
	}
	if len(it.Labels) > 0 {
		out = append(out, "labels: "+strings.Join(it.Labels, ", "))
	}
	if n := it.Need; n != nil {
		need := "need: " + string(n.Kind)
		if n.RaisedBy != nil {
			need += ", raised by session " + n.RaisedBy.Session
		}
		if n.Review != nil && n.Review.PR != "" {
			need += ", PR " + n.Review.PR
		}
		out = append(out, need+fmt.Sprintf(", owner replies delivered: %d", n.Delivered))
	}
	dates := "created " + stamp(it.CreatedAt) + ", updated " + stamp(it.UpdatedAt)
	if !it.ClosedAt.IsZero() {
		dates += ", closed " + stamp(it.ClosedAt)
		if it.ClosedBy != "" {
			dates += " by " + it.ClosedBy
		}
	}
	out = append(out, dates)
	if it.CloseReason != "" {
		out = append(out, "close reason: "+it.CloseReason)
	}
	if d := strings.TrimSpace(it.Description); d != "" {
		out = append(out, "", d)
	}
	var children, needs []string
	for _, ch := range items {
		if ch.Parent != id {
			continue
		}
		if ch.Type == work.Need {
			needs = append(needs, "  "+line(&ch))
		} else {
			children = append(children, "  "+line(&ch))
		}
	}
	if len(children) > 0 {
		out = append(out, "", "children:")
		out = append(out, children...)
	}
	if len(needs) > 0 {
		out = append(out, "", "needs:")
		out = append(out, needs...)
	}
	if len(it.Comments) > 0 {
		out = append(out, "", "comments:")
		for _, cm := range it.Comments {
			out = append(out, fmt.Sprintf("  %s  %s (%s)", stamp(cm.CreatedAt), cm.Author, cm.Kind))
			for _, l := range strings.Split(strings.TrimSpace(cm.Text), "\n") {
				out = append(out, "    "+l)
			}
		}
	}
	_, err = fmt.Fprintln(c.stdout, strings.Join(out, "\n"))
	return err
}

// subTaskAdd is pm task add --parent TASK: a sub-task under an open task.
func subTaskAdd(c *storeCall) error {
	parent, title := strings.TrimSpace(c.flag("parent")), strings.TrimSpace(c.flag("title"))
	if parent == "" {
		return errors.New("--parent TASK is empty")
	}
	if title == "" {
		return errors.New("--title is required")
	}
	body, _, err := c.text()
	if err != nil {
		return err
	}
	p, err := c.item(parent, work.Task)
	if err != nil {
		return err
	}
	if p.Status != work.Open {
		return fmt.Errorf("task %s is closed; a sub-task goes under an open task", parent)
	}
	made, err := c.d.Create(work.New{Type: work.Task, Parent: parent, Title: title, Description: body})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "created task %s under task %s; claim it with pm task claim %s\n", made.ID, parent, made.ID)
	return nil
}

func taskEdit(c *storeCall) error {
	id := c.args[0]
	var title, desc *string
	if c.fs.Changed("title") {
		t := strings.TrimSpace(c.flag("title"))
		if t == "" {
			return errors.New("--title is empty")
		}
		title = &t
	}
	body, ok, err := c.text()
	if err != nil {
		return err
	}
	if ok {
		desc = &body
	}
	if title == nil && desc == nil {
		return errors.New("nothing to edit: give --title, a description (--text or --text-file), or both")
	}
	if _, err := c.item(id, work.Task); err != nil {
		return err
	}
	if err := c.d.Edit(id, title, desc); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "edited %s\n", id)
	return nil
}

func taskRelease(c *storeCall) error {
	id := c.args[0]
	if c.session == "" {
		return errors.New("no session to release for: $CLAUDE_CODE_SESSION_ID and $CODEX_THREAD_ID are unset")
	}
	if _, err := c.item(id, work.Task); err != nil {
		return err
	}
	if err := c.d.Release(id, c.session); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "released %s\n", id)
	return nil
}

func dep(c *storeCall, add bool) error {
	id, on := c.args[0], strings.TrimSpace(c.flag("on"))
	if on == "" {
		return errors.New("--on BLOCKER is required")
	}
	if add {
		if err := c.d.DepAdd(id, on); err != nil {
			return err
		}
		fmt.Fprintf(c.stdout, "%s now waits on %s\n", id, on)
		return nil
	}
	if err := c.d.DepRemove(id, on); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "%s no longer waits on %s\n", id, on)
	return nil
}

func commentAdd(c *storeCall) error {
	id := c.args[0]
	body, ok, err := c.text()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no note: give --text or --text-file")
	}
	author := c.session
	if author == "" {
		author = "owner"
	}
	if _, err := c.d.Comment(id, work.Note, author, body); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "added a note to %s\n", id)
	return nil
}

// openNeed is the open need with this id, refused when it is not one.
func (c *storeCall) openNeed(id string) error {
	it, err := c.item(id, work.Need)
	if err != nil {
		return err
	}
	if it.Status != work.Open {
		return fmt.Errorf("%s is closed already (%s)", id, it.Resolution)
	}
	return nil
}

func needDismiss(c *storeCall) error {
	id, reason := c.args[0], strings.TrimSpace(c.flag("reason"))
	if reason == "" {
		return errors.New("--reason REASON is required")
	}
	if err := c.openNeed(id); err != nil {
		return err
	}
	if err := c.d.Close(id, reason, work.Dismissed, c.session); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "dismissed %s\n", id)
	return nil
}

func replyAdd(c *storeCall) error {
	id := c.args[0]
	body, ok, err := c.text()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no answer: give --text or --text-file")
	}
	if err := c.openNeed(id); err != nil {
		return err
	}
	if _, err := c.d.Comment(id, work.Reply, "owner", body); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "added the owner's reply to %s; it stays open until the session that raised it records "+
		"the answer\n", id)
	return nil
}

func syncNow(c *storeCall) error {
	r, err := c.d.Sync()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "synced: pulled %d commits, resolved %d items both sides changed, pushed %d commits\n",
		r.Pulled, r.Resolved, r.Pushed)
	for _, o := range r.Overrides {
		fmt.Fprintf(c.stdout, "warning: %s: the claim by %s (%s) was overridden by the later claim of %s (%s)\n", o.ID,
			o.Lost.Session, o.Lost.ClaimedAt.Format(time.RFC3339), o.Kept.Session, o.Kept.ClaimedAt.Format(time.RFC3339))
	}
	return nil
}

// ---------------------------------------------------------------- sessions

// LiveWindow is how recently a session's transcript must have changed for it to be live.
const LiveWindow = 30 * time.Minute

var sessionID = regexp.MustCompile(`^[\w-]+$`)

// currentSession is this command's agent session: Claude Code's session id, else Codex's thread id, else "".
func currentSession() string {
	for _, name := range []string{"CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID"} {
		if s := strings.TrimSpace(os.Getenv(name)); s != "" {
			return s
		}
	}
	return ""
}

// live is whether one of the session's transcripts changed within LiveWindow: Claude Code's
// <config>/projects/<project dir>/<id>.jsonl (any project dir, since each worktree has its own), or Codex's
// $CODEX_HOME/sessions/<date>/rollout-<time>-<id>.jsonl.
func live(session string) bool {
	if !sessionID.MatchString(session) {
		return false
	}
	home, _ := os.UserHomeDir()
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	paths, _ := filepath.Glob(filepath.Join(claude, "projects", "*", session+".jsonl"))
	_ = filepath.WalkDir(filepath.Join(codex, "sessions"), func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasPrefix(e.Name(), "rollout-") &&
			strings.HasSuffix(e.Name(), "-"+session+".jsonl") {
			paths = append(paths, p)
		}
		return nil
	})
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < LiveWindow {
			return true
		}
	}
	return false
}
