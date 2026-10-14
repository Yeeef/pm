package hooks

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// An agent may ask the owner with its runtime's ask tool instead of raising a need, when the owner is plainly in the
// chat: Claude Code's AskUserQuestion, Codex's request_user_input_async (Default mode) or request_user_input (Plan
// mode). The owner sees such a question in the session, so the owner-request check must not take it, or the reply's
// repeat of it, for an ask left only in chat; Codex's async tool returns at once, and its question can also end up in
// the turn's final message. ToolAsks finds those questions in the turn that is stopping; the judge gets them beside
// the session's open needs, so a sentence asking the same thing matches one, and any other ask still blocks.

// askTools are the ask tools' names, as the transcripts record a call: Claude Code's tool_use name, Codex's
// function_call name.
var askTools = map[string]bool{"AskUserQuestion": true, "request_user_input": true, "request_user_input_async": true}

// toolQuestion is one question of an ask tool's input: AskUserQuestion's and request_user_input's question (with a
// header), request_user_input_async's title.
type toolQuestion struct{ Question, Title, Header string }

// text is the question's first field that is not blank, as asked: Claude Code keys AskUserQuestion's answers by it.
func (q toolQuestion) text() string {
	for _, s := range []string{q.Question, q.Title, q.Header} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// transcriptLine is one line of a Claude Code transcript or a Codex rollout, as far as ToolAsks reads it.
type transcriptLine struct {
	Type    string `json:"type"`
	IsMeta  bool   `json:"isMeta"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Payload struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"payload"`
}

// ToolAsks is the questions the session asked with its ask tool in its last turn, read from its transcript (a Claude
// Code session's JSONL or a Codex rollout): the calls after the turn's start, which is the last prompt of the user in
// Claude Code and the last turn_context in Codex. None when the transcript cannot be read: then every ask is judged.
// Lines that cannot start a turn or hold an ask call are not parsed.
func ToolAsks(transcript string) []string {
	f, err := os.Open(transcript)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read only
	var asks []string
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if strings.Contains(line, `"turn_context"`) || strings.Contains(line, `"user"`) ||
			strings.Contains(line, "AskUserQuestion") || strings.Contains(line, "request_user_input") {
			var e transcriptLine
			if json.Unmarshal([]byte(line), &e) == nil {
				if turnStart(e) {
					asks = nil
				}
				asks = append(asks, toolCallAsks(e)...)
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil
			}
			return asks
		}
	}
}

// turnStart is whether the line starts a turn: Codex's turn_context, or a prompt of the user in Claude Code (a user
// message with text, not a tool result or a message Claude Code adds itself).
func turnStart(e transcriptLine) bool {
	if e.Type == "turn_context" {
		return true
	}
	if e.Type != "user" || e.IsMeta {
		return false
	}
	var text string
	if json.Unmarshal(e.Message.Content, &text) == nil {
		return text != ""
	}
	var blocks []struct{ Type string }
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return false
		}
	}
	for _, b := range blocks {
		if b.Type == "text" {
			return true
		}
	}
	return false
}

// toolCallAsks is the questions of the ask tool calls on the line.
func toolCallAsks(e transcriptLine) []string {
	var inputs []json.RawMessage
	switch {
	case e.Type == "assistant":
		var blocks []struct {
			Type, Name string
			Input      json.RawMessage
		}
		if json.Unmarshal(e.Message.Content, &blocks) != nil {
			return nil
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && askTools[b.Name] {
				inputs = append(inputs, b.Input)
			}
		}
	case e.Type == "response_item" && e.Payload.Type == "function_call" && askTools[e.Payload.Name]:
		inputs = append(inputs, json.RawMessage(e.Payload.Arguments))
	}
	var out []string
	for _, in := range inputs {
		var args struct{ Questions []toolQuestion }
		if json.Unmarshal(in, &args) != nil {
			continue
		}
		for _, q := range args.Questions {
			if t := strings.TrimSpace(q.text()); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}
