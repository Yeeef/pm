package hooks

// pm hook ask: a session's ask-tool question raised as a question need, so the owner can answer it on the site as well
// as in the session; the first answer to close the need wins (work.ErrClosed). Every ending reaches pm through a hook
// event, so no transcript or rollout is read (AGENTS.md: a transcript only when no event gives the fact).
//
// Claude Code (--runtime claude): at PermissionRequest of AskUserQuestion, beside the question's dialog, for up to a
// day, it raises the question, then holds one blocking call to the pm service (service.WaitNeed), which answers once
// the need has a site reply or is closed. On a site reply it closes the need as answered on the site and prints the
// decision that answers the tool, which closes the dialog; on a close elsewhere it exits silent. The dialog's answer
// closes the need at PostToolUse of AskUserQuestion, from the answers the tool returned; Esc fires no hook, but Claude
// Code sends the waiting hook SIGTERM, on which it closes the need as dismissed. The dialog's "Chat about this", and a
// skip in the Claude app through Remote Control, fire neither: the agent gets the rejection and goes on, the hook waits
// on. So at Stop it closes the
// session's open questions as dismissed, since a dialog never outlives its turn. A subagent's call, or another tool's,
// is no dialog the owner sees: it steps aside.
//
// Codex (--runtime codex): at PostToolUse of request_user_input_async, which returns at once, it raises the question
// once, by its call id, and returns; at UserPromptSubmit it closes the session's open questions with the prompt the
// owner typed. The pm service closes a question before it queues the site's answer into the session, so that close
// decides which answer wins.
//
// It fails open: on its own error it says why on stderr, prints nothing and exits 0, so the dialog stays usable.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/store"
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

// AskRetry is the wait before pm hook ask calls the pm service again after a call that failed (the service down or
// restarting) or answered with no answer the store confirms: 1, 2, 4, then 5 s, never sooner than a second.
var AskRetry = func(tries int) time.Duration { return min(5*time.Second, time.Second<<min(tries, 3)) }

// AskStopBudget bounds the close a signal makes pm hook ask write: Claude Code kills the hook about 1.2 s after its
// SIGTERM, so the close must be done well before.
var AskStopBudget = 800 * time.Millisecond

// stopped is the close reason of a question whose hook the session stopped before any answer: Esc in the dialog, or
// the session ending with the dialog open. Claude Code tells the two apart to no hook.
const stopped = "the session stopped waiting for the answer: Esc in the dialog, or the session ended"

// skipped is the close reason of a Claude Code question still open when its session's turn ended: the owner skipped it,
// with the dialog's "Chat about this" (2.1.311) or in the Claude app through Remote Control (2.1.310), which stop no hook
// and fire no PostToolUse.
const skipped = "the turn ended without an answer: the owner skipped the question (Chat about this, or a skip in the " +
	"Claude app)"

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

// askClaude handles an AskUserQuestion call: at PermissionRequest it raises the call and waits for its first answer;
// at PostToolUse it closes the call's question with the answer the tool returned. At Stop it closes the session's
// questions that are still open (skippedAtStop).
func askClaude(event Event, dir string, open OpenStore, stdout io.Writer) error {
	if event["hook_event_name"] == "Stop" {
		return skippedAtStop(event, dir, open)
	}
	if _, sub := event["agent_id"]; sub || event["tool_name"] != "AskUserQuestion" {
		return nil // a subagent's call or another tool: no dialog the owner sees
	}
	sid, _ := event["session_id"].(string) //nolint:errcheck // checked below
	input, ok := event["tool_input"].(map[string]any)
	if sid == "" || !ok {
		return errors.New("the hook input has no session_id or tool_input")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	info, err := askOf(raw, "")
	if err != nil {
		return err
	}
	prompt, _ := event["prompt_id"].(string) //nolint:errcheck // "" is a key still: the questions' hash tells calls apart
	if info.Call, err = claudeCall(prompt, info); err != nil {
		return err
	}
	switch event["hook_event_name"] {
	case "PermissionRequest":
		return raiseAndWait(dir, open, sid, info, input, stdout)
	case "PostToolUse":
		return answeredInDialog(event, dir, open, sid, info)
	}
	return nil
}

// claudeCall is an AskUserQuestion call's key: the hook input has no tool-use id at PermissionRequest, so the key is
// the prompt's id, "-", then the first 12 hex digits of the SHA-256 of the questions as askOf reads them. It reads
// only the questions, so PermissionRequest's input and PostToolUse's, which adds the answers, give the same key.
func claudeCall(prompt string, info work.AskInfo) (string, error) {
	raw, err := json.Marshal(info.Questions)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return prompt + "-" + hex.EncodeToString(sum[:6]), nil
}

// raiseAndWait raises the call at its PermissionRequest, beside the dialog, then waits for the first answer.
func raiseAndWait(dir string, open OpenStore, sid string, info work.AskInfo, input map[string]any, stdout io.Writer) error {
	sigs := make(chan os.Signal, 1) // from the start: an Esc during the raise still dismisses the question it raised
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigs)
	var err error
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
	records, err := store.PathOf(dir)
	if err != nil {
		return err
	}
	main, nid := store.MainOf(records), id[0]
	q := &claudeQuestion{open: func() (AskStore, error) { return open(dir) }, id: nid, session: sid, ask: info,
		input: input, waitSite: func(ctx context.Context) error { return service.WaitNeed(ctx, main, nid) }}
	return q.wait(sigs, stdout)
}

// answeredInDialog closes the session's open question of the call with the answers the tool returned, as PostToolUse
// reports them (tool_response.answers, keyed by question text), 17 to 24 ms after the dialog closed (Claude Code
// 2.1.296). It fires after the hook's own answer too: that question is closed already, as answered on the site, and
// stays so. Esc fires no PostToolUse; the waiting hook's signal closes that question (claudeQuestion.wait).
func answeredInDialog(event Event, dir string, open OpenStore, sid string, info work.AskInfo) (err error) {
	answers := map[string]string{}
	if r, ok := event["tool_response"].(map[string]any); ok {
		if a, ok := r["answers"].(map[string]any); ok {
			for k, v := range a {
				answers[k] = fmt.Sprint(v)
			}
		}
	}
	st, err := open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Shutdown()) }()
	needs, err := st.Needs(sid)
	if err != nil {
		return err
	}
	for i := range needs {
		n := &needs[i]
		if service.IsQuestion(n) && n.Status != work.Closed && n.Need.Ask.Call == info.Call {
			if err := service.AnswerInChat(st, n, answerText(info, answers), sid); err != nil &&
				!errors.Is(err, work.ErrClosed) {
				return err
			}
		}
	}
	return nil
}

// skippedAtStop closes the session's open questions as dismissed (skipped): at Stop no dialog of the session is open, so
// a question still open was skipped where no hook saw it. The hook still waiting on it sees the close and exits.
func skippedAtStop(event Event, dir string, open OpenStore) (err error) {
	sid, _ := event["session_id"].(string) //nolint:errcheck // checked below
	if sid == "" {
		return errors.New("the hook input has no session_id")
	}
	st, err := open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Shutdown()) }()
	needs, err := st.Needs(sid)
	if err != nil {
		return err
	}
	for _, n := range needs {
		if service.IsQuestion(&n) && n.Status != work.Closed {
			if err := st.Close(n.ID, skipped, work.Dismissed, sid); err != nil && !errors.Is(err, work.ErrClosed) {
				return err
			}
		}
	}
	return nil
}

// claudeQuestion is a raised AskUserQuestion call that waits for its answer.
type claudeQuestion struct {
	open        func() (AskStore, error)
	id, session string
	ask         work.AskInfo
	input       map[string]any // the tool's input, which the decision repeats with the answers
	// waitSite is one call to the pm service that returns once the need has a reply waiting or is closed (nil), or
	// fails; it ends when ctx does.
	waitSite func(ctx context.Context) error
	storeErr error // the first failed look at the need, said once the wait ends
}

// wait holds one waitSite call until a site reply or a close elsewhere (the dialog's answer, closed at PostToolUse)
// ends it; when the call answers it looks at the need (step), and when the call fails, or the look finds nothing to
// take, it calls again after AskRetry. A signal ends it, closing the need as dismissed: Claude Code sends the hook
// SIGTERM 79 to 81 ms after Esc closes the dialog, and 21 ms after the session stops with it open, but none on a chosen
// answer (2.1.296), so no transcript read is needed to tell an Esc from an answer. Its return ends the call under way.
func (q *claudeQuestion) wait(stop <-chan os.Signal, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	moved := make(chan error, 1) // one call at a time: the next starts only once its answer was taken
	call := func(after time.Duration) {
		go func() {
			select {
			case <-time.After(after):
			case <-ctx.Done():
				return
			}
			moved <- q.waitSite(ctx)
		}()
	}
	call(0)
	tries := 0
	for {
		select {
		case <-stop:
			return errors.Join(q.storeErr, q.dismiss())
		case err := <-moved:
			if err != nil {
				q.storeErr = cmpOr(q.storeErr, err)
			} else if done, err := q.step(stdout); done || err != nil {
				return errors.Join(q.storeErr, err)
			}
			call(AskRetry(tries))
			tries++
		}
	}
}

// step is one look: a site reply on the need closes it as answered on the site and prints the decision; a need closed
// elsewhere ends the wait. A store that cannot be reached is looked at again next time.
func (q *claudeQuestion) step(stdout io.Writer) (done bool, err error) {
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
	answers := answersFrom(q.ask, service.SiteAnswer(n))
	if err := st.Close(q.id, service.AnsweredOnSite, work.Answered, q.session); errors.Is(err, work.ErrClosed) {
		return true, nil // answered in the dialog meanwhile: that answer won
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

// dismiss closes the need as dismissed (stopped), within AskStopBudget. A need closed already (PostToolUse's chat
// answer, or the site's) stays as it is.
func (q *claudeQuestion) dismiss() error {
	done := make(chan error, 1)
	go func() {
		st, err := q.open()
		if err != nil {
			done <- err
			return
		}
		err = st.Close(q.id, stopped, work.Dismissed, q.session)
		if errors.Is(err, work.ErrClosed) {
			err = nil
		}
		done <- errors.Join(err, st.Shutdown())
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(AskStopBudget):
		return fmt.Errorf("closing the question as dismissed took over %s", AskStopBudget)
	}
}

func cmpOr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// answerText is the dialog's answers, keyed by question text, as a need records them: one line per question answered,
// "<question>: <answer>", as the site writes it (item.AnswerText).
func answerText(info work.AskInfo, answers map[string]string) string {
	var lines []string
	for _, q := range info.Questions {
		if v, ok := answers[q.Text]; ok {
			lines = append(lines, q.Line()+": "+v)
		}
	}
	if len(lines) == 0 {
		return "answered in the session"
	}
	return strings.Join(lines, "\n")
}

// answersFrom is the site's answer to each question, keyed by its text as asked, as AskUserQuestion's answers are. The
// site writes one line per question, "<question>: <answer>" (item.AnswerText: the question's Line, a multiple choice's
// labels joined with ", "); an answer naming an option's label in any case is that label, any other is the owner's own
// text. A reply with no such line (pm reply add's free text) answers every question whole.
func answersFrom(info work.AskInfo, reply string) map[string]string {
	byLength := slices.Clone(info.Questions) // a question whose line starts with another's is matched first
	sort.SliceStable(byLength, func(i, j int) bool { return len(byLength[i].Line()) > len(byLength[j].Line()) })
	values := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(reply, "\n") {
		matched := false
		for _, q := range byLength {
			if rest, ok := strings.CutPrefix(line, q.Line()+":"); ok {
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

// askCodex handles a Codex session's ask tool: at PostToolUse of request_user_input_async it raises the call's
// question; at UserPromptSubmit it closes the session's open questions with the prompt the owner typed.
func askCodex(event Event, dir string, open OpenStore) error {
	switch event["hook_event_name"] {
	case "PostToolUse":
		return raiseCodex(event, dir, open)
	case "UserPromptSubmit":
		return answeredInChat(event, dir, open)
	}
	return nil
}

// raiseCodex raises the request_user_input_async call the PostToolUse event reports, by its call id (tool_use_id, the
// rollout's call_id); another tool steps aside. A blocking request_user_input is answered in the session before the
// turn goes on, so it raises nothing.
func raiseCodex(event Event, dir string, open OpenStore) error {
	if event["tool_name"] != "request_user_input_async" {
		return nil
	}
	sid, _ := event["session_id"].(string)             //nolint:errcheck // checked below
	transcript, _ := event["transcript_path"].(string) //nolint:errcheck // checked below
	call, _ := event["tool_use_id"].(string)           //nolint:errcheck // checked below
	input, ok := event["tool_input"].(map[string]any)
	if sid == "" || transcript == "" || call == "" || !ok {
		return errors.New("the hook input has no session_id, transcript_path, tool_use_id or tool_input")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	info, err := askOf(raw, call)
	if err != nil {
		return err
	}
	home, ok := session.CodexHomeOfRollout(transcript) // Codex runs hooks without its CODEX_* variables
	if !ok {
		if home, err = filepath.Abs(session.CodexHome()); err != nil {
			return err
		}
	}
	host, err := service.Hostname()
	if err != nil {
		return err
	}
	_, err = raise(open, dir, work.RaisedBy{Session: sid, Inbox: session.CodexInbox(home), Host: host},
		[]work.AskInfo{info}, true)
	return err
}

// answeredInChat closes each open question of the session with the prompt the owner typed, which answers it: Codex's
// async question returns at once, and its answer is the next message the owner sends. A message the pm service queued
// (a question's site answer, or a delivery's "pm: " text) may come as a prompt too: the service closed that question
// before it queued the answer, and service.Relayed leaves the message out for the session's other questions.
func answeredInChat(event Event, dir string, open OpenStore) (err error) {
	sid, _ := event["session_id"].(string) //nolint:errcheck // checked below
	prompt, _ := event["prompt"].(string)  //nolint:errcheck // "": nothing typed
	if sid == "" {
		return errors.New("the hook input has no session_id")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	st, err := open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Shutdown()) }()
	needs, err := st.Needs(sid)
	if err != nil || service.Relayed(needs, prompt) {
		return err
	}
	for i := range needs {
		if n := &needs[i]; service.IsQuestion(n) && n.Status != work.Closed {
			if err := service.AnswerInChat(st, n, strings.TrimSpace(prompt), sid); err != nil &&
				!errors.Is(err, work.ErrClosed) {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- both

// raise creates a question need for each ask whose call is no open need of the session (with once, no need at all:
// a Codex call is raised once, whatever became of its need), under the newest open task the session holds (none
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
