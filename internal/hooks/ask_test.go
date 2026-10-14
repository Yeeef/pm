package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/item"
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

// dialogLines is what Claude Code appends to the transcript once the dialog closes: the call, then its result, which
// holds the answers or, rejected (Esc), is an error.
func dialogLines(input map[string]any, answer string) string {
	in, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	use := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1",` +
		`"name":"AskUserQuestion","input":` + string(in) + `}]}}`
	result := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"answered",` +
		`"tool_use_id":"toolu_1"}]},"toolUseResult":{"answers":{"Which color?":"` + answer + `"},"annotations":{}}}`
	if answer == "" {
		result = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result",` +
			`"content":"User rejected tool use","is_error":true,"tool_use_id":"toolu_1"}]},` +
			`"toolUseResult":"User rejected tool use","toolDenialKind":"user-rejected"}`
	}
	return use + "\n" + result + "\n"
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
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

// TestTheTailFindsTheDialogsAnswerAppendedAfterTheHookStarted: lines written before the hook started, a call with
// other input and a half-written line are no answer; the call with the hook's input and its result are, read as the
// answers or, on a rejection, as one.
func TestTheTailFindsTheDialogsAnswerAppendedAfterTheHookStarted(t *testing.T) {
	path := transcript(t, strings.TrimSpace(dialogLines(colorInput, "Red"))) // an earlier call, before the hook
	tail := newAskTail(path, colorInput)
	if got := tail.read(); got != nil {
		t.Fatalf("before the dialog closed: %+v", got)
	}
	other := map[string]any{"questions": []any{map[string]any{"question": "Other?"}}}
	appendTo(t, path, dialogLines(other, "x"))
	lines := dialogLines(colorInput, "Blue")
	appendTo(t, path, lines[:len(lines)-20])
	if got := tail.read(); got != nil {
		t.Fatalf("another call, and half the answer: %+v", got)
	}
	appendTo(t, path, lines[len(lines)-20:])
	got := tail.read()
	if got == nil || got.rejected || got.text(colorAsk("")) != "Which color?: Blue" {
		t.Fatalf("the answer: %+v", got)
	}

	path = transcript(t, `{"type":"user","message":{"role":"user","content":"go"}}`)
	tail = newAskTail(path, colorInput)
	appendTo(t, path, dialogLines(colorInput, ""))
	if got := tail.read(); got == nil || !got.rejected {
		t.Fatalf("rejected with Esc: %+v", got)
	}
}

// TestASubagentsCallOrAnotherToolStepsAside: no dialog the owner sees, so nothing is raised and nothing printed.
func TestASubagentsCallOrAnotherToolStepsAside(t *testing.T) {
	st := newAskStore()
	for name, in := range map[string]string{
		"a subagent":   `{"agent_id":"a1","tool_name":"AskUserQuestion","session_id":"s","transcript_path":"/t"}`,
		"another tool": `{"tool_name":"Bash","session_id":"s","transcript_path":"/t"}`,
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

// raised is a question colorInput raised by session s under the newest task it holds, as askClaude raises it.
func raised(t *testing.T, st *askStore, path string) *claudeQuestion {
	t.Helper()
	ids, err := raise(st.open, "/here", work.RaisedBy{Session: "s"}, []work.AskInfo{colorAsk("p1-abc")}, false)
	if err != nil || len(ids) != 1 {
		t.Fatalf("raise: %v %v", ids, err)
	}
	return &claudeQuestion{open: func() (AskStore, error) { return st.open("") }, id: ids[0], session: "s",
		ask: colorAsk("p1-abc"), input: colorInput, tail: newAskTail(path, colorInput)}
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
	q := raised(t, st, transcript(t, `{"type":"user","message":{"role":"user","content":"go"}}`))
	if n := st.item(q.id); n.Parent != "p-1.2" || n.Need.Ask.Call != "p1-abc" {
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

// TestAClaudeQuestionTakesTheDialogsAnswerOrEnds: the dialog's answer closes it with that answer, and wins over a site
// reply that comes in meanwhile, which counts as delivered with a note; a rejection or a stop dismisses it; a need
// closed elsewhere ends the wait with nothing printed.
func TestAClaudeQuestionTakesTheDialogsAnswerOrEnds(t *testing.T) {
	path := transcript(t, `{"type":"user","message":{"role":"user","content":"go"}}`)
	var out bytes.Buffer
	for name, c := range map[string]struct {
		answer, reply string
		want          func(n work.Item) bool
	}{
		"in the dialog": {"Red", "", func(n work.Item) bool {
			return n.Resolution == work.Answered && len(n.Comments) == 1 && n.Comments[0].Text == "Which color?: Red"
		}},
		"in the dialog, the site late": {"Red", "Which color?: Blue", func(n work.Item) bool {
			return n.Resolution == work.Answered && len(n.Comments) == 3 && n.Comments[1].Text == "Which color?: Red" &&
				n.Comments[2].Kind == work.Note && n.Need.Delivered == 1
		}},
		"rejected": {"", "", func(n work.Item) bool {
			return n.Resolution == work.Dismissed && n.CloseReason == declined
		}},
	} {
		st := newAskStore(heldTasks...)
		q := raised(t, st, transcript(t, `{"type":"user","message":{"role":"user","content":"go"}}`))
		if c.reply != "" {
			st.reply(q.id, c.reply)
		}
		appendTo(t, q.tail.path, dialogLines(colorInput, c.answer))
		var out bytes.Buffer
		if done, err := q.step(&out); !done || err != nil || out.Len() != 0 {
			t.Fatalf("%s: %v %v %q", name, done, err, out.String())
		}
		if n := st.item(q.id); n.Status != work.Closed || !c.want(n) {
			t.Errorf("%s: %+v", name, n)
		}
	}

	st := newAskStore(heldTasks...)
	q := raised(t, st, path)
	if err := st.Close(q.id, "elsewhere", work.Dismissed, "x"); err != nil {
		t.Fatal(err)
	}
	if done, err := q.step(&out); !done || err != nil {
		t.Errorf("closed elsewhere: %v %v", done, err)
	}

	st = newAskStore(heldTasks...)
	q = raised(t, st, path)
	stop := make(chan os.Signal, 1)
	stop <- os.Interrupt
	if err := q.wait(stop, nil, &out); err != nil {
		t.Fatal(err)
	}
	if n := st.item(q.id); n.Resolution != work.Dismissed || n.CloseReason != stopped {
		t.Errorf("stopped: %+v", n)
	}
}

// TestCodexStopRaisesEachAsyncQuestionOnce: request_user_input_async's questions of the stopping turn, options as
// labels, under the rollout's CODEX_HOME; a later stop of the turn raises none again; a blocking request_user_input,
// answered before the turn goes on, raises nothing.
func TestCodexStopRaisesEachAsyncQuestionOnce(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-10-14T10-00-00-01a12bee-db2f-7050-ba39-116b615a19b8.jsonl")
	lines := strings.Join([]string{`{"type":"turn_context","payload":{"cwd":"/r"}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"request_user_input_async",` +
			`"arguments":"{\"questions\":[{\"title\":\"Red or blue?\",\"options\":[\"Red\",\"Blue\"]}]}","call_id":"call_1"}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"request_user_input",` +
			`"arguments":"{\"questions\":[{\"id\":\"q\",\"header\":\"Cache\",\"question\":\"Disk?\",\"options\":[]}]}","call_id":"call_2"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	st := newAskStore()
	in := fmt.Sprintf(`{"hook_event_name":"Stop","session_id":"01a12bee-db2f-7050-ba39-116b615a19b8",`+
		`"transcript_path":%q,"cwd":"/r"}`, path)
	for range 2 {
		var out, errb bytes.Buffer
		if err := HookAsk("/here", ask.Codex, st.open, strings.NewReader(in), &out, &errb); err != nil ||
			out.Len() != 0 || errb.Len() != 0 {
			t.Fatalf("%v, out %q, stderr %q", err, out.String(), errb.String())
		}
	}
	needs, err := st.Needs("01a12bee-db2f-7050-ba39-116b615a19b8")
	if err != nil || len(needs) != 1 {
		t.Fatalf("raised %d questions (%v)", len(needs), err)
	}
	n := needs[0]
	want := work.AskInfo{Call: "call_1", Questions: []work.AskQuestion{{Text: "Red or blue?",
		Options: []work.AskOption{{Label: "Red"}, {Label: "Blue"}}}}}
	if !reflect.DeepEqual(*n.Need.Ask, want) || n.Need.RaisedBy.Inbox != "codex-queue:"+home || n.Parent != "" {
		t.Errorf("raised %+v under %q, inbox %q", *n.Need.Ask, n.Parent, n.Need.RaisedBy.Inbox)
	}
}
