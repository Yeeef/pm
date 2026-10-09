package hooks

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/yeeef-agents/pm"
)

func TestParseJudgedTakesTheSpanFromTheFirstToTheLastBrace(t *testing.T) {
	items, ok := parseJudged("Here it is:\n```json\n{\"items\": [{\"quote\": \"Q?\", \"kind\": \"decision\", " +
		"\"match\": \"x.1\"}, {\"quote\": \"R\", \"kind\": \"offer\"}]}\n```")
	if !ok || len(items) != 2 || items[0].quote != "Q?" || *items[0].match != "x.1" || items[1].match != nil {
		t.Fatalf("got %v %v", items, ok)
	}
	if items, ok := parseJudged(`{"items": []}`); !ok || len(items) != 0 {
		t.Errorf("an empty list is in shape: %v %v", items, ok)
	}
}

func TestParseJudgedRefusesAnAnswerOutOfShape(t *testing.T) {
	for _, out := range []string{
		"", "no JSON here", `{"items": null}`, `{"verdict": []}`, `{"items": [1]}`, `{"items": {}}`,
		`{"items": [{"quote": "Q", "kind": "decision", "match": 7}]}`, `{"items": [{"quote": "Q"}]}`,
		`{"items": [{"quote": 1, "kind": "decision"}]}`, `{"items": []} and {"items": []}`, `{"items": [}`,
	} {
		if items, ok := parseJudged(out); ok {
			t.Errorf("%q: got %v, want out of shape", out, items)
		}
	}
}

func TestJudgePromptListsEachRequestWithItsDescriptionOnOneLine(t *testing.T) {
	got := judgePrompt("Ask?", []Request{{ID: "a.1", Title: "One", Description: "two\n\n  words " +
		strings.Repeat("é", 500)}, {ID: "a.2", Title: "Bare"}})
	want := "OPEN REQUESTS:\n- a.1: One\n  two words " + strings.Repeat("é", 390) + "\n- a.2: Bare\n\n" +
		"REPLY:\n<<<\nAsk?\n>>>"
	if got != want {
		t.Errorf("got %q", got)
	}
	if got := judgePrompt("Ask?", nil); !strings.HasPrefix(got, "OPEN REQUESTS:\n(none)\n\n") {
		t.Errorf("no request: %q", got)
	}
}

func TestQuotedCutsEachQuoteAndTheWhole(t *testing.T) {
	long := strings.Repeat("ü", 200)
	if got := quoted([]string{"A?", "B"}); got != `"A?"; "B"` {
		t.Errorf("got %q", got)
	}
	if got := quoted([]string{long, long, long, long, long}); len([]rune(got)) != 600 ||
		!strings.HasPrefix(got, `"`+strings.Repeat("ü", 150)+`"; "`) {
		t.Errorf("got %d characters", len([]rune(got)))
	}
}

func TestDecideReadsNothingAfterABlockOrForABlankReply(t *testing.T) {
	read := func(dir, session string) ([]Request, error) { return nil, errors.New("read") }
	for _, in := range []string{`{"stop_hook_active": true}`, `{"session_id": "s", "last_assistant_message": " \n"}`} {
		var out, errb bytes.Buffer
		code, err := HookOwnerRequest(t.TempDir(), read, strings.NewReader(in), &out, &errb)
		if code != 0 || err != nil || out.Len() != 0 || errb.Len() != 0 {
			t.Errorf("%s: code %d, err %v, stdout %q, stderr %q", in, code, err, out.String(), errb.String())
		}
	}
}

func TestAFailedCheckExitsOneWithItsCause(t *testing.T) {
	read := func(dir, session string) ([]Request, error) { return nil, errors.New("work store: none at /x") }
	for in, want := range map[string]string{
		`[1]`:                 "the hook input is not a JSON object",
		`{}`:                  "the hook input has no session_id",
		`{"session_id": "s"}`: "the hook input has no last_assistant_message",
		`{"session_id": "s", "last_assistant_message": "Ask?"}`: "reading this session's open needs failed: " +
			"work store: none at /x",
	} {
		var out, errb bytes.Buffer
		code, err := HookOwnerRequest(t.TempDir(), read, strings.NewReader(in), &out, &errb)
		if code != 1 || err != nil || out.Len() != 0 ||
			errb.String() != "pm hook owner-request: "+want+"; the owner-request check did not run\n" {
			t.Errorf("%s: code %d, err %v, stdout %q, stderr %q", in, code, err, out.String(), errb.String())
		}
	}
}

// fakeClaude puts a shell script standing in for claude first on PATH.
func fakeClaude(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestABlockNamesTheNeedlessAskThenTheUncoveredRequest(t *testing.T) {
	fakeClaude(t, `[ "$MAX_THINKING_TOKENS" = 0 ] || exit 3
echo 'Verdict: {"items": [{"quote": "Merge it?", "kind": "decision", "match": "theirs.1"},
 {"quote": "Push the branch?", "kind": "authorized", "match": "mine.1"},
 {"quote": "Rename X?", "kind": "decision", "match": "mine.1"}, {"quote": "Done.", "kind": "not asked", "match": null}]}'`)
	read := func(dir, session string) ([]Request, error) {
		if session != "s" {
			t.Errorf("session %q", session)
		}
		return []Request{{ID: "mine.1", Title: "Rename X?"}}, nil
	}
	var out, errb bytes.Buffer
	in := `{"session_id": "s", "last_assistant_message": "Merge it? Push the branch? Rename X?"}`
	code, err := HookOwnerRequest(t.TempDir(), read, strings.NewReader(in), &out, &errb)
	if code != 0 || err != nil || errb.Len() != 0 {
		t.Fatalf("code %d, err %v, stderr %q", code, err, errb.String())
	}
	reason := strings.ReplaceAll(pm.OwnerRequestNeedless, "{asks}", `"Push the branch?"`) + "\n\n" +
		strings.ReplaceAll(pm.OwnerRequestReason, "{asks}", `"Merge it?"`)
	want := `{"decision": "block", "reason": "` + strings.ReplaceAll(strings.ReplaceAll(reason, `"`, `\"`), "\n", `\n`) +
		"\"}\n"
	if out.String() != want {
		t.Errorf("got %s\nwant %s", out.String(), want)
	}
}

func TestAJudgeThatFailsIsNamedWithWhatItSaid(t *testing.T) {
	fakeClaude(t, `echo '  Not   logged
in' >&2; exit 2`)
	read := func(dir, session string) ([]Request, error) { return nil, nil }
	var out, errb bytes.Buffer
	in := `{"session_id": "s", "last_assistant_message": "Ask?"}`
	if code, _ := HookOwnerRequest(t.TempDir(), read, strings.NewReader(in), &out, &errb); code != 1 ||
		errb.String() != "pm hook owner-request: claude -p failed (exit 2): Not logged in; the owner-request check did not run\n" {
		t.Errorf("code %d, stderr %q", code, errb.String())
	}
}
