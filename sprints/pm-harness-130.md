---
type: sprint
title: A question skipped in the Claude app closes at the turn's end
bead: yeeef-agents-9va.140
---

## Goal

> What should be true when this sprint ends, and why now?

A Claude Code question the owner skips in the Claude app (Remote Control) leaves "Sessions ask you" once the session's turn ends, instead of staying open until the session's process exits (18 h in one case). The owner reported it. Through Remote Control, a skip returns "Denied by user" to the agent, but Claude Code sends the waiting `pm hook ask` no signal and fires no PostToolUse, so nothing closes the need.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Claude Code's Stop hook runs `pm hook ask`, which closes the session's open questions as dismissed: a Claude Code dialog never outlives its turn.
- The runtime-difference reason, the hooks' doc comments and the owner-questions design page.

**Out:**
- Codex: its questions outlive the turn by design and close at the next prompt.
- A skip that Claude Code never records (the dialog still open in the session): that question is still open.

## Done when

> What evidence will show the goal is met?

- A Go test: a Stop event closes the session's open question as dismissed and leaves another session's open; `make test-go` for `internal/hooks` and `internal/install` passes.
- Live: the built pm run on a Stop event for this session closes the open `[TEST] skip probe` question, and its waiting `pm hook ask` process exits.

## Design pages

> Where is the detail?

- [Owner questions](../design/owner-questions.md): the measured fact of an app's skip, its branch in the
  Claude Code sequence, and the chat-answer table.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- On Claude Code 2.1.311 in a terminal, Esc and a killed terminal both still
  close a question as dismissed within 3 s (needs pm-u6wb, pm-mw4l). Through
  Remote Control (2.1.310), a skip in the Claude app returns Denied by user to
  the agent but fires no PostToolUse and signals no hook: the probe question
  stayed open with its hook waiting 16 min. Across the owner's 6 repos, one
  question (poker-n1cu) stayed open 18 h; its session's transcript ends at the
  AskUserQuestion call.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a Claude Code question skipped in the Claude app closes as dismissed when the session's turn ends, instead of
staying open until the session's process exits. PR #124.

- Claude Code's Stop group runs `pm hook ask`, which closes the session's open questions as dismissed.
- Codex unchanged; `runtimeDifferences` gives the Stop difference its reason.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Go test: met. `TestStopDismissesTheSessionsQuestionsStillOpen` closes the session's open question as dismissed and
  leaves an answered one and another session's open one; it fails with the Stop branch disabled.
  `go test ./internal/hooks ./internal/install ./internal/cli` ok; `make test` 159 passed, 169 skipped.
- Live: met. The `[TEST] skip probe` question, skipped in the Claude app at 18:24:53 UTC, was still open with its hook
  waiting 16 min later; the built pm (stamped 0.15.9) run on a Stop event for this session closed it as dismissed
  with the new reason, the waiting hook exited, and the site's count of the probe went from 1 to 0.
