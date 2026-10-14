package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/work"
)

// askStore is an in-memory AskStore: Close and Answer of a closed need fail with work.ErrClosed, as the store's do.
type askStore struct {
	mu     sync.Mutex
	items  map[string]*work.Item
	opens  int
	closed bool // a connection the test made fail
	clock  int  // seconds since the store's epoch: each write's stamp, one later than the last
}

// tick is the next write's stamp; under s.mu.
func (s *askStore) tick() time.Time {
	s.clock++
	return time.Date(2026, 10, 14, 12, 0, s.clock, 0, time.UTC)
}

func newAskStore(items ...work.Item) *askStore {
	s := &askStore{items: map[string]*work.Item{}}
	for i := range items {
		it := items[i]
		s.items[it.ID] = &it
	}
	return s
}

func (s *askStore) open(string) (AskStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("the pm service is down")
	}
	s.opens++
	return s, nil
}

func (s *askStore) item(id string) work.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.items[id]
}

func (s *askStore) Items() ([]work.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []work.Item
	for _, it := range s.items {
		out = append(out, *it)
	}
	return out, nil
}

func (s *askStore) Get(ids ...string) ([]work.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []work.Item
	for _, id := range ids {
		out = append(out, *s.items[id])
	}
	return out, nil
}

func (s *askStore) Needs(session string) ([]work.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []work.Item
	for _, it := range s.items {
		if it.Need != nil && it.Need.RaisedBy != nil && it.Need.RaisedBy.Session == session {
			out = append(out, *it)
		}
	}
	return out, nil
}

func (s *askStore) Create(n work.New) (work.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("q-%d", len(s.items)+1)
	it := work.Item{ID: id, Type: n.Type, Parent: n.Parent, Title: n.Title, Description: n.Description,
		Status: work.Open, Need: n.Need}
	s.items[id] = &it
	return it, nil
}

func (s *askStore) closeOpen(id string) (*work.Item, error) {
	it := s.items[id]
	if it.Status == work.Closed {
		return nil, fmt.Errorf("work store: %s %w", id, work.ErrClosed)
	}
	return it, nil
}

func (s *askStore) Close(id, reason string, r work.Resolution, session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.closeOpen(id)
	if err == nil {
		it.Status, it.CloseReason, it.Resolution, it.ClosedBy, it.ClosedAt = work.Closed, reason, r, session, s.tick()
	}
	return err
}

func (s *askStore) Answer(id, text, session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.closeOpen(id)
	if err == nil {
		when := s.tick() // the answer is stamped with the close, as the store's Answer does
		it.Comments = append(it.Comments, work.Comment{Kind: work.Reply, Author: "owner", Text: text, CreatedAt: when})
		it.Status, it.CloseReason, it.Resolution, it.ClosedBy, it.ClosedAt = work.Closed, "Responded", work.Answered, session, when
	}
	return err
}

func (s *askStore) Comment(id string, kind work.CommentKind, author, text string) (work.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := work.Comment{Kind: kind, Author: author, Text: text, CreatedAt: s.tick()}
	s.items[id].Comments = append(s.items[id].Comments, c)
	return c, nil
}

func (s *askStore) UpdateNeed(id string, u work.NeedUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.Delivered != nil {
		s.items[id].Need.Delivered = *u.Delivered
	}
	return nil
}

func (s *askStore) Shutdown() error { return nil }

// reply is the owner's site reply on need id, with the mark line the site adds.
func (s *askStore) reply(id, text string) {
	if _, err := s.Comment(id, work.Reply, "owner", text+"\n\n<!-- pm-reply 0b6c7c1e-2d3f -->"); err != nil {
		panic(err)
	}
}

var colorInput = map[string]any{"questions": []any{map[string]any{"question": "Which color?", "header": "Color",
	"options": []any{map[string]any{"label": "Red", "description": "warm"},
		map[string]any{"label": "Blue", "description": "cool"}}, "multiSelect": false}}}

// colorAsk is colorInput as askOf reads it.
func colorAsk(call string) work.AskInfo {
	return work.AskInfo{Call: call, Questions: []work.AskQuestion{{Text: "Which color?", Header: "Color",
		Options: []work.AskOption{{Label: "Red", Detail: "warm"}, {Label: "Blue", Detail: "cool"}}}}}
}

// TestAnswersFromMapsTheSitesReplyToEachQuestion: one "<question>: <answer>" line per question, an option's label in
// any case is that label, a multiple choice's labels are joined by ", ", other text is the owner's own (lines after
// a question's line belong to its answer), and a reply with no such line answers every question whole.
func TestAnswersFromMapsTheSitesReplyToEachQuestion(t *testing.T) {
	info := work.AskInfo{Questions: []work.AskQuestion{
		{Text: "Which color?", Options: []work.AskOption{{Label: "Red"}, {Label: "Blue"}}},
		{Text: "Which color? Really", Options: []work.AskOption{{Label: "Yes"}}},
		{Text: "Which sizes?", Multi: true, Options: []work.AskOption{{Label: "S"}, {Label: "M"}, {Label: "L"}}},
	}}
	for _, c := range []struct {
		reply string
		want  map[string]string
	}{
		{"Which color?: blue\nWhich color? Really: YES\nWhich sizes?: s, L",
			map[string]string{"Which color?": "Blue", "Which color? Really": "Yes", "Which sizes?": "S, L"}},
		{"Which color?: green, like moss\nand leaves\nWhich sizes?: M, XL",
			map[string]string{"Which color?": "green, like moss\nand leaves", "Which sizes?": "M, XL"}},
		{"red", map[string]string{"Which color?": "Red", "Which color? Really": "red", "Which sizes?": "red"}},
	} {
		if got := answersFrom(info, c.reply); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.reply, got, c.want)
		}
	}
}

// TestTheSitesAnswerReachesEachQuestionAsAsked: the questions of an AskUserQuestion input, as askOf reads them, answered
// on the site's card (item.AnswerText) come back from answersFrom keyed by each question's text as the tool asked it:
// one with a newline and a double space, one with whitespace at its edges, one that is a prefix of another, one with
// ": " in it; each asked alone and all asked together.
func TestTheSitesAnswerReachesEachQuestionAsAsked(t *testing.T) {
	type question struct {
		text, pick, other string
		multi             bool
		labels            []string
		want              string
	}
	questions := []question{
		{text: "Which DB?", pick: "Postgres", labels: []string{"Postgres", "MySQL"}, want: "Postgres"},
		{text: "Deploy  where?\nPick one.", pick: "EU", labels: []string{"EU", "US"}, want: "EU"},
		{text: "  Which DB, really?  ", other: "my  own\nanswer", labels: []string{"Yes"}, want: "my own answer"},
		{text: "Note", pick: "A", labels: []string{"A", "B"}, want: "A"},
		{text: "Note: which regions?", pick: "EU,US", multi: true, labels: []string{"EU", "US"}, want: "EU, US"},
	}
	ask := func(qs []question) (json.RawMessage, map[string][]string, map[string]string) {
		var in []map[string]any
		fields, want := map[string][]string{}, map[string]string{}
		for i, q := range qs {
			var opts []map[string]any
			for _, l := range q.labels {
				opts = append(opts, map[string]any{"label": l, "description": "d"})
			}
			in = append(in, map[string]any{"question": q.text, "header": "H", "multiSelect": q.multi, "options": opts})
			if q.pick != "" {
				fields[item.AnswerField(i)] = strings.Split(q.pick, ",")
			}
			if q.other != "" {
				fields[item.OtherField(i)] = []string{q.other}
			}
			want[q.text] = q.want
		}
		raw, err := json.Marshal(map[string]any{"questions": in})
		if err != nil {
			t.Fatal(err)
		}
		return raw, fields, want
	}
	sets := [][]question{questions}
	for _, q := range questions {
		sets = append(sets, []question{q})
	}
	for _, qs := range sets {
		raw, fields, want := ask(qs)
		info, err := askOf(raw, "call")
		if err != nil {
			t.Fatal(err)
		}
		reply, err := item.AnswerText(info, fields)
		if err != nil {
			t.Fatal(err)
		}
		if got := answersFrom(info, reply); !reflect.DeepEqual(got, want) {
			t.Errorf("reply %q: got %q, want %q", reply, got, want)
		}
	}
}

// TestASubagentsCallOrAnotherToolStepsAside: no dialog the owner sees, so nothing is raised and nothing printed.
func TestASubagentsCallOrAnotherToolStepsAside(t *testing.T) {
	st := newAskStore()
	for name, in := range map[string]string{
		"a subagent": `{"hook_event_name":"PermissionRequest","agent_id":"a1","tool_name":"AskUserQuestion",` +
			`"session_id":"s"}`,
		"another tool": `{"hook_event_name":"PermissionRequest","tool_name":"Bash","session_id":"s"}`,
	} {
		var out, errb bytes.Buffer
		if err := HookAsk("/here", ask.Claude, st.open, strings.NewReader(in), &out, &errb); err != nil {
			t.Fatal(err)
		}
		if out.Len() != 0 || errb.Len() != 0 || st.opens != 0 {
			t.Errorf("%s: out %q, stderr %q, store opened %d times", name, out.String(), errb.String(), st.opens)
		}
	}
	var out, errb bytes.Buffer // fails open: one line on stderr, nothing printed
	if err := HookAsk("/here", ask.Claude, st.open, strings.NewReader(`{"tool_name":"AskUserQuestion"}`), &out,
		&errb); err != nil || out.Len() != 0 || !strings.HasPrefix(errb.String(), "pm hook ask: the hook input has no") {
		t.Errorf("no session: %v, out %q, stderr %q", err, out.String(), errb.String())
	}
}

// raised is a question colorInput raised by session s in prompt p1 under the newest task it holds, as askClaude raises
// it.
func raised(t *testing.T, st *askStore) *claudeQuestion {
	t.Helper()
	key, err := claudeCall("p1", colorAsk(""))
	if err != nil {
		t.Fatal(err)
	}
	ids, err := raise(st.open, "/here", work.RaisedBy{Session: "s"}, []work.AskInfo{colorAsk(key)}, false)
	if err != nil || len(ids) != 1 {
		t.Fatalf("raise: %v %v", ids, err)
	}
	q := &claudeQuestion{open: func() (AskStore, error) { return st.open("") }, id: ids[0], session: "s",
		ask: colorAsk(key), input: colorInput}
	q.waitSite = (&siteWaits{st: st}).wait(q.id)
	return q
}

// siteWaits fakes the pm service's wait: a call returns nil once the store shows the need with a reply waiting or
// closed, looking in memory (no dial), and ctx's error once ctx ends; the first down calls fail at once, as calls to a
// service that is restarting do. It counts the calls and keeps how each ended.
type siteWaits struct {
	st    *askStore
	mu    sync.Mutex
	down  int
	calls int
	ended []error
}

func (f *siteWaits) wait(id string) func(ctx context.Context) error {
	return func(ctx context.Context) (err error) {
		f.mu.Lock()
		f.calls++
		failing := f.calls <= f.down
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.ended = append(f.ended, err)
			f.mu.Unlock()
		}()
		if failing {
			return errors.New("the pm service is restarting")
		}
		for {
			if n := f.st.item(id); n.Status == work.Closed || service.ReplyWaiting(&n) {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
}

func (f *siteWaits) seen() (calls int, ended []error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, slices.Clone(f.ended)
}

// waitAsHooked runs q.wait as askClaude does, on a goroutine: its error once it returns.
func waitAsHooked(q *claudeQuestion, stop chan os.Signal, out *bytes.Buffer) <-chan error {
	done := make(chan error, 1)
	go func() { done <- q.wait(stop, out) }()
	return done
}

func ended(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the hook still waits 5 s on")
	}
	return nil
}

// TestAClaudeQuestionWaitsOnOneCallToTheService: while nobody answers, the hook holds one call to the service and
// reaches the store no more; the site's reply ends that call, and one look at the store answers the tool.
func TestAClaudeQuestionWaitsOnOneCallToTheService(t *testing.T) {
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	f := &siteWaits{st: st}
	q.waitSite = f.wait(q.id)
	opens := st.opens
	var out bytes.Buffer
	done := waitAsHooked(q, make(chan os.Signal), &out)
	select { // a test of idleness: what happens in a second while nobody answers
	case err := <-done:
		t.Fatalf("the wait ended with no answer: %v", err)
	case <-time.After(time.Second):
	}
	if calls, _ := f.seen(); calls != 1 || st.opens != opens {
		t.Fatalf("idle for 1 s: %d calls to the service, %d dials of the store", calls, st.opens-opens)
	}
	start := time.Now()
	st.reply(q.id, "Which color?: blue")
	if err := ended(t, done); err != nil {
		t.Fatal(err)
	}
	t.Logf("site reply to the decision: %v", time.Since(start))
	calls, how := f.seen()
	if n := st.item(q.id); calls != 1 || how[0] != nil || st.opens != opens+1 || n.CloseReason != service.AnsweredOnSite ||
		!strings.Contains(out.String(), `"answers":{"Which color?":"Blue"}`) {
		t.Errorf("%d calls (%v), %d dials; printed %q; need %+v", calls, how, st.opens-opens, out.String(), n)
	}
}

// TestADialogAnswerEndsTheCallToTheService: the dialog's answer, closed at PostToolUse, ends the wait with nothing
// printed, and the call under way ends.
func TestADialogAnswerEndsTheCallToTheService(t *testing.T) {
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	f := &siteWaits{st: st}
	q.waitSite = f.wait(q.id)
	var out bytes.Buffer
	done := waitAsHooked(q, make(chan os.Signal), &out)
	var errb bytes.Buffer
	if err := HookAsk("/here", ask.Claude, st.open, strings.NewReader(postToolUse()), &out, &errb); err != nil ||
		errb.Len() != 0 {
		t.Fatalf("PostToolUse: %v %q", err, errb.String())
	}
	if err := ended(t, done); err != nil || out.Len() != 0 {
		t.Fatalf("%v, printed %q", err, out.String())
	}
	if n := st.item(q.id); n.Resolution != work.Answered || n.Comments[0].Text != "Which color?: Red" {
		t.Errorf("need %+v", n)
	}
	if calls, how := f.seen(); calls != 1 || len(how) != 1 || how[0] != nil {
		t.Errorf("%d calls, ended %v", calls, how)
	}
}

// TestAClaudeQuestionClosedElsewhereEndsTheWait: the service's answer to a need closed elsewhere ends the wait, with
// nothing printed.
func TestAClaudeQuestionClosedElsewhereEndsTheWait(t *testing.T) {
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	var out bytes.Buffer
	done := waitAsHooked(q, make(chan os.Signal), &out)
	if err := st.Close(q.id, "elsewhere", work.Dismissed, "x"); err != nil {
		t.Fatal(err)
	}
	if err := ended(t, done); err != nil || out.Len() != 0 {
		t.Fatalf("%v, printed %q", err, out.String())
	}
}

// TestAClaudeQuestionCallsTheServiceAgainAfterItFails: calls that fail (the service restarting) are made again after
// AskRetry, and the next call that holds takes the site's reply; the failure is said once the wait ends.
func TestAClaudeQuestionCallsTheServiceAgainAfterItFails(t *testing.T) {
	saved := AskRetry
	t.Cleanup(func() { AskRetry = saved })
	var waited []int
	AskRetry = func(tries int) time.Duration { waited = append(waited, tries); return 20 * time.Millisecond }
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	f := &siteWaits{st: st, down: 2}
	q.waitSite = f.wait(q.id)
	var out bytes.Buffer
	done := waitAsHooked(q, make(chan os.Signal), &out)
	if !within(time.Second, func() bool { calls, _ := f.seen(); return calls == 3 }) {
		t.Fatal("not called again after two failures")
	}
	st.reply(q.id, "Which color?: red")
	err := ended(t, done)
	if calls, _ := f.seen(); err == nil || !strings.Contains(err.Error(), "restarting") || calls != 3 ||
		!slices.Equal(waited, []int{0, 1}) || !strings.Contains(out.String(), `"Which color?":"Red"`) {
		t.Errorf("err %v, %d calls, retries %v, printed %q", err, calls, waited, out.String())
	}
}

// TestAskRetryNeverCallsMoreOftenThanEverySecondNorWaitsOverFive: 1, 2, 4, then 5 s.
func TestAskRetryNeverCallsMoreOftenThanEverySecondNorWaitsOverFive(t *testing.T) {
	var got []time.Duration
	for tries := range 6 {
		got = append(got, AskRetry(tries))
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second, 5 * time.Second}
	if !slices.Equal(got, want) {
		t.Errorf("AskRetry: %v", got)
	}
}

// within checks cond every 5 ms until it holds, for up to d; whether it held.
func within(d time.Duration, cond func() bool) bool {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(d)
	for !cond() {
		select {
		case <-tick.C:
		case <-deadline:
			return cond()
		}
	}
	return true
}

// heldTasks are the tasks of the Claude question tests: session s holds two, the later claimed last.
var heldTasks = []work.Item{
	{ID: "p-1.1", Type: work.Task, Status: work.Open,
		Holder: &work.Holder{Session: "s", ClaimedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}},
	{ID: "p-1.2", Type: work.Task, Status: work.Open,
		Holder: &work.Holder{Session: "s", ClaimedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}},
	{ID: "p-1.3", Type: work.Task, Status: work.Open,
		Holder: &work.Holder{Session: "other", ClaimedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}},
}

// TestAClaudeQuestionAnsweredOnTheSiteAnswersTheTool: raised under the newest open task the session holds; a site
// reply closes it as answered on the site, delivered, and answers the tool with it.
func TestAClaudeQuestionAnsweredOnTheSiteAnswersTheTool(t *testing.T) {
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	if n := st.item(q.id); n.Parent != "p-1.2" || n.Need.Ask.Call != q.ask.Call {
		t.Fatalf("raised under %q with call %q", n.Parent, n.Need.Ask.Call)
	}
	var out bytes.Buffer
	if done, err := q.step(&out); done || err != nil || out.Len() != 0 {
		t.Fatalf("no answer yet: %v %v %q", done, err, out.String())
	}
	st.reply(q.id, "Which color?: blue")
	if done, err := q.step(&out); !done || err != nil {
		t.Fatalf("site answer: %v %v", done, err)
	}
	want := `{"hookSpecificOutput":{"decision":{"behavior":"allow","updatedInput":{"answers":{"Which color?":"Blue"},` +
		`"questions":[{"header":"Color","multiSelect":false,"options":[{"description":"warm","label":"Red"},` +
		`{"description":"cool","label":"Blue"}],"question":"Which color?"}]}},"hookEventName":"PermissionRequest"}}` + "\n"
	n := st.item(q.id)
	if out.String() != want || n.Status != work.Closed || n.Resolution != work.Answered ||
		n.CloseReason != "answered on the site" || n.Need.Delivered != 1 || n.ClosedBy != "s" {
		t.Errorf("site answer: printed %s; need %+v", out.String(), n)
	}
}

// claudePostToolUse is the PostToolUse input Claude Code 2.1.296 sent once an AskUserQuestion dialog was answered
// (captured 2026-10-14 under a pty), every key kept, with the prompt id and the answer a test's. tool_input and
// tool_response are the same object: the questions, the answers by question text, and annotations.
const claudePostToolUse = `{"session_id": "s", "transcript_path": "/t/s.jsonl", "cwd": "/here", ` +
	`"scratchpad_dir": "/t/scratchpad", "prompt_id": %q, "permission_mode": "default", "effort": {"level": "medium"}, ` +
	`"hook_event_name": "PostToolUse", "tool_name": "AskUserQuestion", "tool_input": %s, "tool_response": %s, ` +
	`"tool_use_id": "toolu_015hnvv6TtZypF9MyoZ9EamU", "duration_ms": 1}`

// postToolUse is claudePostToolUse for colorInput answered Red under the prompt p1, the prompt raised asks under.
func postToolUse() string {
	result := map[string]any{"answers": map[string]any{"Which color?": "Red"}, "annotations": map[string]any{}}
	for k, v := range colorInput {
		result[k] = v
	}
	raw, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(claudePostToolUse, "p1", raw, raw)
}

// TestClaudesCallKeyReadsOnlyTheQuestions: PermissionRequest's input and PostToolUse's, which adds the answers and
// annotations, give one key; another prompt or other questions give another.
func TestClaudesCallKeyReadsOnlyTheQuestions(t *testing.T) {
	key := func(prompt string, input map[string]any) string {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		info, err := askOf(raw, "")
		if err != nil {
			t.Fatal(err)
		}
		k, err := claudeCall(prompt, info)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(postToolUse()), &event); err != nil {
		t.Fatal(err)
	}
	answered, ok := event["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("no tool_input")
	}
	other := map[string]any{"questions": []any{map[string]any{"question": "Which size?"}}}
	if k := key("p1", colorInput); k != key("p1", answered) || k == key("p2", colorInput) || k == key("p1", other) ||
		!strings.HasPrefix(k, "p1-") || len(k) != len("p1-")+12 {
		t.Errorf("keys: %q %q %q %q", k, key("p1", answered), key("p2", colorInput), key("p1", other))
	}
}

// TestPostToolUseClosesTheQuestionWithTheDialogsAnswer: the open question of the call closes with the answers the tool
// returned, by the session; a site reply that came meanwhile lost the race and counts as delivered with a note. A
// question closed already (the hook's own answer) stays as it is, and another prompt's question is left open.
func TestPostToolUseClosesTheQuestionWithTheDialogsAnswer(t *testing.T) {
	key, err := claudeCall("p1", colorAsk(""))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		reply string
		want  func(n work.Item) bool
	}{
		"in the dialog": {"", func(n work.Item) bool {
			return n.Resolution == work.Answered && len(n.Comments) == 1 && n.Comments[0].Text == "Which color?: Red" &&
				n.ClosedBy == "s"
		}},
		"in the dialog, the site late": {"Which color?: Blue", func(n work.Item) bool {
			return n.Resolution == work.Answered && len(n.Comments) == 3 && n.Comments[1].Text == "Which color?: Red" &&
				n.Comments[2].Kind == work.Note && n.Need.Delivered == 1
		}},
	} {
		st := newAskStore(heldTasks...)
		ids, err := raise(st.open, "/here", work.RaisedBy{Session: "s"}, []work.AskInfo{colorAsk(key)}, false)
		if err != nil {
			t.Fatal(err)
		}
		if c.reply != "" {
			st.reply(ids[0], c.reply)
		}
		var out, errb bytes.Buffer
		if err := HookAsk("/here", ask.Claude, st.open, strings.NewReader(postToolUse()), &out, &errb); err != nil ||
			out.Len() != 0 || errb.Len() != 0 {
			t.Fatalf("%s: %v, out %q, stderr %q", name, err, out.String(), errb.String())
		}
		if n := st.item(ids[0]); n.Status != work.Closed || !c.want(n) {
			t.Errorf("%s: %+v", name, n)
		}
	}

	st := newAskStore(heldTasks...)
	ids, err := raise(st.open, "/here", work.RaisedBy{Session: "s"}, []work.AskInfo{colorAsk(key), colorAsk("p2-x")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(ids[0], service.AnsweredOnSite, work.Answered, "s"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := HookAsk("/here", ask.Claude, st.open, strings.NewReader(postToolUse()), &out, &errb); err != nil ||
		out.Len() != 0 || errb.Len() != 0 {
		t.Fatalf("closed already: %v, out %q, stderr %q", err, out.String(), errb.String())
	}
	if n := st.item(ids[0]); n.CloseReason != service.AnsweredOnSite || len(n.Comments) != 0 {
		t.Errorf("closed on the site, then PostToolUse: %+v", n)
	}
	if n := st.item(ids[1]); n.Status != work.Open {
		t.Errorf("another prompt's question: %+v", n)
	}
}

// TestASignalDismissesTheQuestionAtOnce: Claude Code's SIGTERM (Esc, or the session stopping) closes the question as
// dismissed within AskStopBudget, reading no transcript; a question PostToolUse closed first stays answered; a store
// that does not answer in time ends the hook with the reason.
func TestASignalDismissesTheQuestionAtOnce(t *testing.T) {
	st := newAskStore(heldTasks...)
	q := raised(t, st)
	var out bytes.Buffer
	stop := make(chan os.Signal, 1)
	done := waitAsHooked(q, stop, &out)
	start := time.Now()
	stop <- syscall.SIGTERM
	if err := ended(t, done); err != nil || out.Len() != 0 {
		t.Fatalf("%v, printed %q", err, out.String())
	}
	if n := st.item(q.id); n.Resolution != work.Dismissed || n.CloseReason != stopped || n.ClosedBy != "s" {
		t.Errorf("stopped: %+v", n)
	}
	t.Logf("signal to dismissed: %v", time.Since(start))

	st = newAskStore(heldTasks...)
	q = raised(t, st)
	if err := service.AnswerInChat(st, ptr(st.item(q.id)), "Which color?: Red", "s"); err != nil {
		t.Fatal(err)
	}
	stop = make(chan os.Signal, 1)
	stop <- syscall.SIGTERM
	if err := q.wait(stop, &out); err != nil || st.item(q.id).Resolution != work.Answered {
		t.Errorf("answered, then signalled: %v %+v", err, st.item(q.id))
	}

	saved := AskStopBudget
	t.Cleanup(func() { AskStopBudget = saved })
	AskStopBudget = 20 * time.Millisecond
	q = raised(t, newAskStore(heldTasks...))
	hung := make(chan struct{})
	t.Cleanup(func() { close(hung) })
	q.open = func() (AskStore, error) { <-hung; return nil, errors.New("late") }
	stop = make(chan os.Signal, 1)
	stop <- syscall.SIGTERM
	start = time.Now()
	if err := q.wait(stop, &out); err == nil || !strings.Contains(err.Error(), "took over") || time.Since(start) > 500*time.Millisecond {
		t.Errorf("a store that does not answer: %v after %v", err, time.Since(start))
	}
}

func ptr[T any](v T) *T { return &v }

// codexPostToolUse is the PostToolUse input Codex 0.162.1 sent for a request_user_input_async call (captured
// 2026-10-14 in its TUI), every key kept, with the transcript path a test's.
const codexPostToolUse = `{"session_id": "01a13bd6-f478-73e2-b37d-8485a2e11c09", ` +
	`"turn_id": "01a13bd6-f49a-7890-8570-bea6ac6db4ff", "transcript_path": %q, "cwd": "/r", ` +
	`"hook_event_name": "PostToolUse", "model": "gpt-6.1-sol", "permission_mode": "default", ` +
	`"tool_name": %q, "tool_input": {"questions": [{"title": "Red or blue?", "options": ["Red", "Blue"]}]}, ` +
	`"tool_response": "{\"accepted\":true}", "tool_use_id": "call_4bc62352c7ef4ed2aa545bfc42dbfa71"}`

// TestCodexPostToolUseRaisesTheAsyncQuestionOnce: request_user_input_async's question, options as labels, by its
// tool_use_id, under the rollout's CODEX_HOME; the same call again raises none; another tool steps aside.
func TestCodexPostToolUseRaisesTheAsyncQuestionOnce(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-10-14T21-15-05-01a13bd6-f478-73e2-b37d-8485a2e11c09.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"turn_context","payload":{"cwd":"/r"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := newAskStore()
	for _, tool := range []string{"request_user_input_async", "request_user_input_async", "request_user_input", "Bash"} {
		var out, errb bytes.Buffer
		in := fmt.Sprintf(codexPostToolUse, path, tool)
		if err := HookAsk("/here", ask.Codex, st.open, strings.NewReader(in), &out, &errb); err != nil ||
			out.Len() != 0 || errb.Len() != 0 {
			t.Fatalf("%s: %v, out %q, stderr %q", tool, err, out.String(), errb.String())
		}
	}
	needs, err := st.Needs("01a13bd6-f478-73e2-b37d-8485a2e11c09")
	if err != nil || len(needs) != 1 || st.opens != 2 {
		t.Fatalf("raised %d questions, store opened %d times (%v)", len(needs), st.opens, err)
	}
	n := needs[0]
	want := work.AskInfo{Call: "call_4bc62352c7ef4ed2aa545bfc42dbfa71", Questions: []work.AskQuestion{{
		Text: "Red or blue?", Options: []work.AskOption{{Label: "Red"}, {Label: "Blue"}}}}}
	if !reflect.DeepEqual(*n.Need.Ask, want) || n.Need.RaisedBy.Inbox != "codex-queue:"+home || n.Parent != "" {
		t.Errorf("raised %+v under %q, inbox %q", *n.Need.Ask, n.Parent, n.Need.RaisedBy.Inbox)
	}
	var out, errb bytes.Buffer // fails open: one line on stderr, nothing raised
	in := `{"hook_event_name":"PostToolUse","tool_name":"request_user_input_async","session_id":"s"}`
	if err := HookAsk("/here", ask.Codex, st.open, strings.NewReader(in), &out, &errb); err != nil || out.Len() != 0 ||
		!strings.HasPrefix(errb.String(), "pm hook ask: the hook input has no") {
		t.Errorf("no call id: %v, out %q, stderr %q", err, out.String(), errb.String())
	}
}

// codexUserPromptSubmit is the UserPromptSubmit input Codex 0.154.0 sent (captured 2026-10-11), every key kept, with
// the session and the prompt a test's.
const codexUserPromptSubmit = `{"session_id": %q, "turn_id": "01a12be7-d295-73c0-bf06-91db5a02a58c", ` +
	`"transcript_path": "/t/rollout.jsonl", "cwd": "/here", "hook_event_name": "UserPromptSubmit", ` +
	`"model": "gpt-6-astra", "permission_mode": "bypassPermissions", "prompt": %q}`

// TestCodexUserPromptSubmitClosesTheOpenQuestionsWithThePrompt: what the owner types answers each open question of
// the session, by the session; the site answer the service queued for a question it closed first, or a delivery's
// text, answers none; a closed question, another session's and an empty prompt are left alone.
func TestCodexUserPromptSubmitClosesTheOpenQuestionsWithThePrompt(t *testing.T) {
	codex := work.RaisedBy{Session: "t1", Inbox: "codex-queue:/h"}
	st := newAskStore()
	ids, err := raise(st.open, "/here", codex, []work.AskInfo{colorAsk("call_1"), colorAsk("call_2"), colorAsk("call_3")},
		true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := raise(st.open, "/here", work.RaisedBy{Session: "t2", Inbox: "codex-queue:/h"},
		[]work.AskInfo{colorAsk("call_9")}, true)
	if err != nil {
		t.Fatal(err)
	}
	st.reply(ids[0], "Which color?: Blue") // answered on the site: the service closed it, then queued the answer
	if err := st.Close(ids[0], service.AnsweredOnSite, work.Answered, ""); err != nil {
		t.Fatal(err)
	}
	prompt := func(text string) {
		t.Helper()
		var out, errb bytes.Buffer
		in := fmt.Sprintf(codexUserPromptSubmit, "t1", text)
		if err := HookAsk("/here", ask.Codex, st.open, strings.NewReader(in), &out, &errb); err != nil || out.Len() != 0 ||
			errb.Len() != 0 {
			t.Fatalf("%q: %v, out %q, stderr %q", text, err, out.String(), errb.String())
		}
	}
	for _, relayed := range []string{"Which color?: Blue", "pm: owner reply to decision x", "  "} {
		prompt(relayed)
	}
	if st.item(ids[1]).Status != work.Open || st.item(ids[2]).Status != work.Open {
		t.Fatalf("a relayed message or an empty prompt closed a question: %+v", st.item(ids[1]))
	}
	prompt("Red, thanks")
	for _, id := range ids[1:] {
		if n := st.item(id); n.Resolution != work.Answered || n.Comments[0].Text != "Red, thanks" || n.ClosedBy != "t1" {
			t.Errorf("%s: %+v", id, n)
		}
	}
	if n := st.item(ids[0]); n.CloseReason != service.AnsweredOnSite || len(n.Comments) != 1 {
		t.Errorf("answered on the site, then a prompt: %+v", n)
	}
	if st.item(other[0]).Status != work.Open {
		t.Error("another session's question was closed")
	}
	prompt("and another thing") // no open question left: nothing to close
}
