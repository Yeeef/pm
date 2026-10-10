---
type: sprint
title: Owner-request check reads Beads, not ids in the reply
bead: yeeef-agents-9va.56
---

## Goal

> What should be true when this sprint ends, and why now?

A reply that asks the owner for something passes the Stop hook when every request in it matches an open need or action this session raised, with no id in the text; a request that exists only in chat is still blocked. Why now: the id check conflicts with the owner-writing rule (no ids in messages, ASD-STE100) and blocked at least four replies that did cite ids on 2026-10-06.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the Claude Code owner-request Stop hook (today a Haiku prompt hook in `.claude/settings.json`) becomes a pm command hook that reads this session's open needs and actions from Beads and asks Haiku whether each request in the reply matches one; the Codex check (`harness/owner_request_hook.py`) follows the same rule; RULES.md and the decision-need hints stop telling agents to cite ids in replies.
**Out:** the reply-wait delivery bug (sprint 40); packaging pm (sprints 44-46), beyond keeping the hook as a `pm hook` subcommand.

## Done when

> What evidence will show the goal is met?

- Tests cover: a request matching an open need passes without an id; a request with no matching need blocks; a reply with no request passes; another session's needs do not count.
- In a live Claude Code session, a reply asking for an open decision without its id passes, and a chat-only request is blocked.
- The decision on the approach (need 9va.38.19) is recorded.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- A `type: prompt` hook cannot run commands: it is one model call on the hook
  input JSON (docs, Claude Code 2.1.290). A `type: agent` hook (experimental)
  gets Read, Write, Bash and other tools, in don't-ask mode: Bash runs only
  commands that a permission allow rule covers.

- Measured: a Haiku agent Stop hook that runs `bd list --label human --status
  open --json` under the rule `Bash(bd list:*)` returned the correct count (6)
  in 3 of 3 runs. Timed from the debug log, the hook itself took 6.5 s, 2.9 s
  and 10.1 s (2 to 5 Haiku calls, 1 to 3 Bash calls): each Haiku call is one
  1 to 3 s step, and Haiku chose to rerun the command. An earlier figure of
  "about 10 s more per stop" came from an unmeasured baseline and was wrong.
  When the command was not allowed,
  Haiku tried other tools and once answered ok=false, which blocks the main
  turn by mistake.

- Measured stop-hook latency: today's prompt hook takes 1.0 to 1.5 s (3 runs).
  A command hook would take about 4 s: 0.5 s for `bd list` plus 3.5 s for a
  bare `claude -p` Haiku call (3 runs, 3.47 to 3.61 s). The machine has no API
  key, so the command hook cannot call the API directly.

- Why `claude -p` is slow as a judge: with the real judge prompt it took 4.8
  to 36 s, because `claude -p` turns on Haiku's extended thinking (766 to 1864
  output tokens for a 664-character answer). With MAX_THINKING_TOKENS=0 and
  CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 (which also stops a second side
  request), it took 1.7 to 2.0 s in 5 of 5 runs, 79 to 176 output tokens.
  `--bare` (0.7 s) needs an API key, so it does not work with OAuth. The
  verdict was wrong in 3 of 5 runs for 'Should I merge the PR now or wait for
  review?': the judge prompt needs tuning whichever hook runs it.

- Prompt tuning: a flat request list let 'Two designs… Which one do you want
  me to build?' pass in 3 of 3 runs; a list-then-classify prompt (each
  sentence addressed to the owner gets a kind and a matching open id) fixed
  it. A question echoed in a code block blocked once in 5 runs until code
  blocks, command output and logs were made 'not asked'. Final prompt: 28
  labelled cases (15 pass, 13 block), each correct in 10 of 10 runs (280
  runs); judge call median 1.18 s, max 5.11 s.

- End-to-end hook time with real bd and Haiku: median 1.62 s, max 4.59 s (cold
  first run) over 12 runs; bd list alone 0.53 to 0.6 s. `--output-format json
  --json-schema` was slower (3.7 to 6.2 s against 3.1 to 3.4 s per bare run),
  so the hook parses text output strictly and fails hard on a bad shape.

- An id in a reply no longer counts by itself: a match needs one of this
  session's open needs, so 'Please decide 9va.15.3' with no open need blocks.
  A Codex session now owns its requests: raised_by records CODEX_THREAD_ID.
  Not checked live: whether Codex's Stop payload session_id equals
  CODEX_THREAD_ID.

- In `claude -p` mode the reply-wait PostToolUse hook (asyncRewake) holds the
  turn after `pm decision need` until its timeout (226 s in one live check).

- Offer loophole: 'If you want, I can push the branch and open the PR so the
  sprint can close' passed 3 of 3 as an optional offer. The judge now treats
  an offer of a step the task or sprint cannot finish or close without as a
  request (3b3b357); 32 labelled cases, each correct in 10 of 10 live runs.

- Replay with real bd: 'If you want, I can merge PR #50 now.' with no open
  need blocked in 45 of 46 judged runs; one early sequential run passed and
  could not be reproduced. Under 3 to 20 concurrent hooks, bd list timed out
  after 10 s in 5 of 50 runs: the hook exits 1 and the reply is not checked.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the owner-request Stop hook now matches each request in a reply against the open needs and actions this session raised in Beads, so a reply needs no id, and a chat-only request still blocks.

Merged as 61962b7 (PR #56).

- `harness/owner_request_hook.py` is one command hook for Claude Code and Codex: it reads this session's open requests with `bd list`, then asks Haiku through `claude -p` with thinking off; it fails hard (exit 1, cause on stderr) when bd or the judge fails.
- The judge prompt lists every sentence addressed to the owner, classifies it, and names the open request it matches; an id in the reply counts for nothing by itself.
- An offer of a step the task or sprint cannot finish or close without (merge its PR, pick a design) counts as a request, however it is worded. Asking leave for a step agents may take without asking (work the sprint, push its branch, open its PR) blocks as a needless ask, and RULES.md and SKILL.md state that standing authorization.
- 35 labelled cases (`tests/owner_request_cases.json`) run against real Haiku with `make test-live`; 28 unit tests cover the plumbing in `make test`.
- RULES.md says a raised request is asked in plain words with no id; `pm` records a Codex thread as the raising session.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Tests cover a request matching an open need passing without an id, an unmatched request blocking, a reply with no request passing, and another session's needs not counting: met. `make test` 395 passed, 35 skipped (live), after rebasing onto main; `make test-live` with PM_LIVE_RUNS=10: all 35 cases correct in 10 of 10 runs (350 runs), including an offer to open the sprint's PR (needless ask, with or without a matching need) and an offer to merge a PR with no open review (block), including the chat-only merge question (block), another session's need (block) and a status report quoting example requests (pass).
- In a live Claude Code session, a reply asking for an open decision without its id passes and a chat-only request is blocked: met. A `claude -p` session raised [TEST] need yeeef-agents-9va.56.2 and, resumed, asked for it in plain words: the hook ran about 1.7 s after the stop with no block. A reply "Should I merge the PR now or wait for review?" was blocked (debug log `{"decision": "block", …}`); the [TEST] need is dismissed. Hook time end to end: median 1.62 s, max 4.59 s over 12 runs. Codex was not run live; task yeeef-agents-9va.39.2 checks its session id.
- The decision on the approach (need 9va.38.19) is recorded: met. It is a source=owner project decision in pm-harness, answering yeeef-agents-9va.38.19: a command hook, with the judge prompt tuned against labelled test cases.
