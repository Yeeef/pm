package hooks

// pm hook ask: a session's ask-tool question raised as a question need, so the owner can answer it on the site as
// well as in the session. Claude Code (--runtime claude) runs it on PermissionRequest for AskUserQuestion, beside the
// question's dialog, for up to a day: it raises the question, then waits for the first answer, the dialog's (the
// transcript shows the tool's result, or its rejection) or the site's (a reply on the need), and closes the need with
// it; on the site's it prints the decision that answers the tool, which closes the dialog. Of two answers the first to
// close the need wins (work.ErrClosed). A subagent's call, or another tool's, is no dialog the owner sees: it steps
// aside. Codex (--runtime codex) runs it first at Stop: request_user_input_async returns at once, so the hook raises
// each such question of the turn once, by its call id; the pm service queues the site's answer into the session, and
// pm hook inbox closes the question with an answer typed in the chat. It fails open: on its own error it says why on
// stderr, prints nothing and exits 0, so the dialog stays usable.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/work"
)

// AskStore is the work store as pm hook ask reads and writes it.
type AskStore interface {
	service.QuestionStore
	Items() ([]work.Item, error)
	Get(ids ...string) ([]work.Item, error)
	Needs(session string) ([]work.Item, error)
	Create(n work.New) (work.Item, error)
	Shutdown() error
}

// OpenStore connects to the work store of the clone containing dir.
type OpenStore func(dir string) (AskStore, error)

// DialStore is OpenStore through the clone's pm service.
func DialStore(dir string) (AskStore, error) {
	st, err := dial(dir)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// AskPoll is how often pm hook ask looks for an answer to a Claude Code question: the transcript, then the need.
var AskPoll = time.Second

// declined is the close reason of a question the owner rejected in the session; stopped, of one whose hook was
// stopped before any answer.
const (
	declined = "declined in the session"
	stopped  = "the session stopped waiting for the answer"
)

// HookAsk is pm hook ask: the hook input JSON on stdin; on a Claude Code question answered on the site, the decision
// that answers the tool on stdout. here is the process's directory; runtime is the session's.
func HookAsk(here string, runtime ask.Runtime, open OpenStore, stdin io.Reader, stdout, stderr io.Writer) error {
	event, err := ReadEvent(stdin)
	if err != nil {
		return err
	}
	if event == nil {
		fmt.Fprintln(stderr, "pm hook ask: the hook input is not a JSON object; the question stays in the session only")
		return nil
	}
	dir := here
	if cwd, ok := event["cwd"].(string); ok && cwd != "" {
		dir = cwd
	}
	if runtime == ask.Codex {
		err = askCodex(event, dir, open)
	} else {
		err = askClaude(event, dir, open, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pm hook ask: %s; the question stays in the session only\n", words(err.Error()))
	}
	return nil
}

// ---------------------------------------------------------------- Claude Code

// askClaude raises the AskUserQuestion call the event asks permission for, then waits for its first answer.
func askClaude(event Event, dir string, open OpenStore, stdout io.Writer) error {
	if _, sub := event["agent_id"]; sub || event["tool_name"] != "AskUserQuestion" {
		return nil // a subagent's call or another tool: no dialog the owner sees
	}
	sid, _ := event["session_id"].(string)             //nolint:errcheck // checked below
	transcript, _ := event["transcript_path"].(string) //nolint:errcheck // checked below
	input, ok := event["tool_input"].(map[string]any)
	if sid == "" || transcript == "" || !ok {
		return errors.New("the hook input has no session_id, transcript_path or tool_input")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	prompt, _ := event["prompt_id"].(string) //nolint:errcheck // "" is a key still: the input's hash tells calls apart
	sum := sha256.Sum256(raw)
	info, err := askOf(raw, prompt+"-"+hex.EncodeToString(sum[:6]))
	if err != nil {
		return err
	}
	tail := newAskTail(transcript, input)
	rb := work.RaisedBy{Session: sid, Inbox: session.Inbox("claude")}
	if rb.Inbox != "" {
		if rb.Host, err = service.Hostname(); err != nil {
			return err
		}
	}
	id, err := raise(open, dir, rb, []work.AskInfo{info}, false)
	if err != nil || len(id) == 0 {
		return err
	}
	q := &claudeQuestion{open: func() (AskStore, error) { return open(dir) }, id: id[0], session: sid, ask: info,
		input: input, tail: tail}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigs)
	ticks := time.NewTicker(AskPoll)
	defer ticks.Stop()
	return q.wait(sigs, ticks.C, stdout)
}

// claudeQuestion is a raised AskUserQuestion call that waits for its answer.
type claudeQuestion struct {
	open        func() (AskStore, error)
	id, session string
	ask         work.AskInfo
	input       map[string]any // the tool's input, which the decision repeats with the answers
	tail        *askTail
	storeErr    error // the first failed look at the need, said once the wait ends
}

// wait looks for the answer at each tick until one closes the need, or the need is closed elsewhere; a signal (the
// session stopping the hook) ends it, closing the need with the dialog's answer if the transcript holds one, else as
// dismissed.
func (q *claudeQuestion) wait(stop <-chan os.Signal, tick <-chan time.Time, stdout io.Writer) error {
	for {
		select {
		case <-stop:
			return errors.Join(q.storeErr, q.closeLocal(q.tail.read(), true))
		case <-tick:
		}
		done, err := q.step(stdout)
		if done || err != nil {
			return errors.Join(q.storeErr, err)
		}
	}
}

// step is one look: the dialog's answer in the transcript closes the need with it; a site reply on the need closes it
// as answered on the site and prints the decision; a need closed elsewhere ends the wait. A store that cannot be
// reached is looked at again next time.
func (q *claudeQuestion) step(stdout io.Writer) (done bool, err error) {
	if local := q.tail.read(); local != nil {
		return true, q.closeLocal(local, false)
	}
	st, err := q.open()
	if err != nil {
		q.storeErr = cmpOr(q.storeErr, err)
		return false, nil
	}
	defer func() { err = errors.Join(err, st.Shutdown()) }()
	got, err := st.Get(q.id)
	if err != nil {
		q.storeErr = cmpOr(q.storeErr, err)
		return false, nil
	}
	n := &got[0]
	if n.Status == work.Closed {
		return true, nil
	}
	if !service.ReplyWaiting(n) {
		return false, nil
	}
	if local := q.tail.read(); local != nil { // answered in the dialog meanwhile: that answer won
		return true, q.closeWith(st, n, local, false)
	}
	answers := answersFrom(q.ask, service.SiteAnswer(n))
	if err := st.Close(q.id, service.AnsweredOnSite, work.Answered, q.session); errors.Is(err, work.ErrClosed) {
		return true, nil
	} else if err != nil {
		return true, err
	}
	total := 0
	for _, c := range n.Comments {
		if c.Kind == work.Reply {
			total++
		}
	}
	delivered := st.UpdateNeed(q.id, work.NeedUpdate{Delivered: &total})
	updated := map[string]any{}
	for k, v := range q.input {
		updated[k] = v
	}
	updated["answers"] = answers
	return true, errors.Join(writeJSON(stdout, map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PermissionRequest",
		"decision":      map[string]any{"behavior": "allow", "updatedInput": updated}}}), delivered)
}

// closeLocal closes the need with the dialog's answer (nil: none), reaching the store for it.
func (q *claudeQuestion) closeLocal(local *localAnswer, final bool) error {
	if local == nil && !final {
		return nil
	}
	st, err := q.open()
	if err != nil {
		return err
	}
	got, err := st.Get(q.id)
	if err == nil {
		err = q.closeWith(st, &got[0], local, final)
	}
	return errors.Join(err, st.Shutdown())
}

// closeWith closes the open need n: answered with the dialog's answer, dismissed when the owner rejected the dialog
// or, at the end of the wait (final), when there is no answer. A need closed already is left as it is.
func (q *claudeQuestion) closeWith(st AskStore, n *work.Item, local *localAnswer, final bool) error {
	var err error
	switch {
	case n.Status == work.Closed:
		return nil
	case local != nil && local.rejected:
		err = st.Close(q.id, declined, work.Dismissed, q.session)
	case local != nil:
		err = service.AnswerInChat(st, n, local.text(q.ask), q.session)
	case final:
		err = st.Close(q.id, stopped, work.Dismissed, q.session)
	}
	if errors.Is(err, work.ErrClosed) {
		return nil
	}
	return err
}

func cmpOr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// localAnswer is the dialog's answer as the transcript records the tool's result: the answers by question text, or
// the owner's rejection.
type localAnswer struct {
	rejected bool
	answers  map[string]string
}

// text is the answer as a need records it: one line per question answered, "<question>: <answer>".
func (a *localAnswer) text(info work.AskInfo) string {
	var lines []string
	for _, q := range info.Questions {
		if v, ok := a.answers[q.Text]; ok {
			lines = append(lines, q.Text+": "+v)
		}
	}
	if len(lines) == 0 {
		return "answered in the session"
	}
	return strings.Join(lines, "\n")
}

// askTail reads what Claude Code appends to the transcript after the hook started. While the dialog is up, the call
// is not in the transcript; once the dialog closes, Claude Code appends the assistant's tool_use of AskUserQuestion,
// with the hook's tool input, and the user's tool_result for it, which holds the answers, or is_error on a rejection.
type askTail struct {
	path  string
	off   int64
	input map[string]any
	use   string // the call's tool_use id, once seen
	part  []byte // a line not ended yet
}

func newAskTail(path string, input map[string]any) *askTail {
	t := &askTail{path: path, input: input}
	if st, err := os.Stat(path); err == nil {
		t.off = st.Size()
	}
	return t
}

// read is the dialog's answer once the transcript holds it; nil before.
func (t *askTail) read() *localAnswer {
	f, err := os.Open(t.path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read only
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil
	}
	data, _ := io.ReadAll(f) //nolint:errcheck // what was read counts; the rest is read next time
	t.off += int64(len(data))
	t.part = append(t.part, data...)
	for {
		i := bytes.IndexByte(t.part, '\n')
		if i < 0 {
			break
		}
		line := t.part[:i]
		t.part = t.part[i+1:]
		if a := t.line(line); a != nil {
			return a
		}
	}
	return nil
}

// line reads one transcript line: the call's tool_use, or its tool_result.
func (t *askTail) line(line []byte) *localAnswer {
	if !bytes.Contains(line, []byte("AskUserQuestion")) && !bytes.Contains(line, []byte("tool_result")) {
		return nil
	}
	var e struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result json.RawMessage `json:"toolUseResult"`
	}
	var blocks []struct {
		Type, ID, Name string
		Input          json.RawMessage
		ToolUseID      string `json:"tool_use_id"`
		IsError        bool   `json:"is_error"`
	}
	if json.Unmarshal(line, &e) != nil || json.Unmarshal(e.Message.Content, &blocks) != nil {
		return nil
	}
	for _, b := range blocks {
		switch {
		case e.Type == "assistant" && b.Type == "tool_use" && b.Name == "AskUserQuestion" && sameJSON(b.Input, t.input):
			t.use = b.ID
		case e.Type == "user" && b.Type == "tool_result" && t.use != "" && b.ToolUseID == t.use:
			if b.IsError {
				return &localAnswer{rejected: true}
			}
			var r struct{ Answers map[string]any }
			_ = json.Unmarshal(e.Result, &r) //nolint:errcheck // no answers read: the need records that it was answered
			a := &localAnswer{answers: map[string]string{}}
			for k, v := range r.Answers {
				a.answers[k] = fmt.Sprint(v)
			}
			return a
		}
	}
	return nil
}

// sameJSON is whether raw holds the value v.
func sameJSON(raw json.RawMessage, v map[string]any) bool {
	var got map[string]any
	return json.Unmarshal(raw, &got) == nil && reflect.DeepEqual(got, v)
}

// answersFrom is the site's answer to each question, keyed by its text, as AskUserQuestion's answers are. The site
// writes one line per question, "<question>: <answer>" (a multiple choice's labels joined with ", "); an answer
// naming an option's label in any case is that label, any other is the owner's own text. A reply with no such line (pm
// reply add's free text) answers every question whole.
func answersFrom(info work.AskInfo, reply string) map[string]string {
	byLength := slices.Clone(info.Questions) // a question that starts with another's text is matched first
	sort.SliceStable(byLength, func(i, j int) bool { return len(byLength[i].Text) > len(byLength[j].Text) })
	values := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(reply, "\n") {
		matched := false
		for _, q := range byLength {
			if rest, ok := strings.CutPrefix(line, q.Text+":"); ok {
				cur, matched = q.Text, true
				values[cur] = []string{strings.TrimSpace(rest)}
				break
			}
		}
		if !matched && cur != "" {
			values[cur] = append(values[cur], line)
		}
	}
	out := map[string]string{}
	for _, q := range info.Questions {
		v, ok := values[q.Text]
		text := strings.TrimSpace(strings.Join(v, "\n"))
		if len(values) == 0 {
			text, ok = strings.TrimSpace(reply), true
		}
		if ok {
			out[q.Text] = optionsOf(q, text)
		}
	}
	return out
}

// optionsOf is the answer with each option it names as that option's label: the whole answer, or for a multiple
// choice each of its parts joined by ", ".
func optionsOf(q work.AskQuestion, answer string) string {
	parts := []string{answer}
	if q.Multi {
		parts = strings.Split(answer, ", ")
	}
	for i, p := range parts {
		for _, o := range q.Options {
			if strings.EqualFold(strings.TrimSpace(p), o.Label) {
				parts[i] = o.Label
			}
		}
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------- Codex

// askCodex raises each request_user_input_async question of the stopping turn that is not a need of the session yet.
func askCodex(event Event, dir string, open OpenStore) error {
	if event["hook_event_name"] != "Stop" {
		return nil
	}
	sid, _ := event["session_id"].(string)             //nolint:errcheck // checked below
	transcript, _ := event["transcript_path"].(string) //nolint:errcheck // checked below
	if sid == "" || transcript == "" {
		return errors.New("the hook input has no session_id or transcript_path")
	}
	var asks []work.AskInfo
	var errs []error
	for _, c := range turnAskCalls(transcript) {
		if c.name != "request_user_input_async" || c.call == "" {
			continue // a blocking request_user_input is answered in the session before the turn goes on
		}
		info, err := askOf(c.input, c.call)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		asks = append(asks, info)
	}
	if len(asks) == 0 {
		return errors.Join(errs...)
	}
	home, ok := session.CodexHomeOfRollout(transcript) // Codex runs hooks without its CODEX_* variables
	if !ok {
		var err error
		if home, err = filepath.Abs(session.CodexHome()); err != nil {
			return err
		}
	}
	host, err := service.Hostname()
	if err != nil {
		return err
	}
	_, err = raise(open, dir, work.RaisedBy{Session: sid, Inbox: session.CodexInbox(home), Host: host}, asks, true)
	return errors.Join(append(errs, err)...)
}

// ---------------------------------------------------------------- both

// raise creates a question need for each ask whose call is no open need of the session (with once, no need at all:
// Codex's Stop sees a call again at each later stop of the turn), under the newest open task the session holds (none
// when it holds none); the ids of the needs, in order.
func raise(open OpenStore, dir string, by work.RaisedBy, asks []work.AskInfo, once bool) (ids []string, err error) {
	st, err := open(dir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, st.Shutdown()) }()
	needs, err := st.Needs(by.Session)
	if err != nil {
		return nil, err
	}
	items, err := st.Items()
	if err != nil {
		return nil, err
	}
	parent, claimed := "", time.Time{}
	for _, it := range items {
		if it.Type == work.Task && it.Status != work.Closed && it.Holder != nil && it.Holder.Session == by.Session &&
			!it.Holder.ClaimedAt.Before(claimed) {
			parent, claimed = it.ID, it.Holder.ClaimedAt
		}
	}
	for _, info := range asks {
		if slices.ContainsFunc(needs, func(n work.Item) bool {
			return n.Need != nil && n.Need.Ask != nil && n.Need.Ask.Call == info.Call && (once || n.Status != work.Closed)
		}) {
			continue
		}
		it, err := st.Create(work.NewQuestion(parent, by, info))
		if err != nil {
			return ids, err
		}
		ids = append(ids, it.ID)
	}
	return ids, nil
}

// askOf is the ask an ask tool's input holds, under the call key call: each question's text (AskUserQuestion's and
// request_user_input's question, request_user_input_async's title), header, whether it is a multiple choice, and its
// options, each a label with what it means or (request_user_input_async) a label alone.
func askOf(input json.RawMessage, call string) (work.AskInfo, error) {
	var args struct {
		Questions []struct {
			toolQuestion
			MultiSelect bool
			Options     []json.RawMessage
		}
	}
	if err := json.Unmarshal(input, &args); err != nil || len(args.Questions) == 0 {
		return work.AskInfo{}, errors.New("the ask tool's input holds no questions")
	}
	info := work.AskInfo{Call: call}
	for _, q := range args.Questions {
		aq := work.AskQuestion{Text: q.text(), Header: strings.TrimSpace(q.Header), Multi: q.MultiSelect}
		for _, raw := range q.Options {
			var label string
			var o struct{ Label, Description string }
			if json.Unmarshal(raw, &label) == nil {
				o.Label = label
			} else if json.Unmarshal(raw, &o) != nil {
				return work.AskInfo{}, errors.New("the ask tool's input has an option that is neither a label nor one")
			}
			aq.Options = append(aq.Options, work.AskOption{Label: o.Label, Detail: o.Description})
		}
		info.Questions = append(info.Questions, aq)
	}
	return info, nil
}
