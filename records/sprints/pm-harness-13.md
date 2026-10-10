---
type: sprint
title: Decisions and actions awaiting the owner
bead: yeeef-agents-9va.13
---

## Goal

> What should be true when this sprint ends, and why now?

What waits on the owner splits into two kinds, each cheap to raise and easy to see: decisions (the owner chooses, and the answer is recorded) and actions (the owner does something: review a PR, run a command, apply a setting). Today every request is a `human` need, and an answered need must become a decision record, which is too heavy for a small input or an action.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the two kinds and how an agent raises, the owner answers or completes, and the agent closes each; which answers must become decision records and which need not; how both show on the overview and day pages ("Decisions await you", "Actions await you") and in `pm show`; the rules and docs.

**Out:** notifications to the owner outside the site; changing how decisions are recorded once they are made.

## Done when

> What evidence will show the goal is met?

- An agent raises a small question and an action for the owner with one `pm` command each; both show on the site under their own heading.
- A small answer can be closed without a decision record; an answer that sets a rule still becomes a decision, and the render check still catches one that was skipped.
- RULES.md and the skill describe the two kinds and when to use each.
- An agent that needs the owner's decision or action cannot end its turn with only a chat question: it must have raised the need or action first; checked in a live session.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): the decision and action commands
- [Work layer: Beads](../design/work-layer.md): a need is a decision or an action (the Need row)
- [Views and the site](../design/views-and-site.md): the Decisions await you and Actions await you lists

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-04}
Sprint 13 also makes `pm commit` commit only the records its caller changed, and makes a failed `pm` write leave nothing behind for another session to commit.
A failed sandboxed write was swept into another session's `pm commit` under the wrong name (finding on sprint 10); sprint 13 is the next sprint that changes `pm`.
:::

::: decision {source=owner date=2026-10-04}
A PR review request is an action awaiting the owner, and it must say what to read first: the PR link, the sprint(s) it delivers, the design pages behind it, and what to focus on (risky changes, open choices).
Without that context the owner reviews a diff without knowing the goal or the design, which makes the review slower and less useful.
:::

::: decision {source=agent date=2026-10-04}
A need stays a Beads issue labelled `human` and has one of two kinds: a decision (no further label, so every existing need is a decision) or an action (also labelled `action`). `pm need add --kind decision|action` raises either; `--kind` is required, like `--level`.
Labels are what `bd human list` and `bd label` already read, so both kinds show in `bd human list` and nothing new is invented in Beads; issue types were rejected because `decision` already means an ADR in bd.
:::

::: decision {source=agent date=2026-10-04}
A decision need whose answer sets no rule closes with `pm need respond <id> --no-decision "<why>"`: the answer and the reason go into Beads and the need gets the label `no-decision`; on a need the owner already closed with `bd human respond`, it adds only the reason (a comment) and the label. The answered-need check skips needs labelled `no-decision` and actions, and still fails on any other closed decision need no decision cites.
The skip must be an explicit, recorded act by the agent, so a rule-setting answer closed any other way is still caught; a label works on an issue the owner already closed, where the close reason can no longer be set.
:::

::: decision {source=agent date=2026-10-04}
An action closes with `pm need done <id> --reason "<what showed it done>"` and needs no decision. A PR review request is an action raised with `pm need review --pr URL --sprint ID... --focus "…" [--design SLUG...]`; its context lives in the issue metadata (`metadata.review`), and the site card links the PR, each sprint record and the design pages (those named plus those the sprint records list) and shows the focus.
Metadata round-trips through `bd list --json` (checked on bd 1.3.1), so the card can link each item from either page depth; design pages listed by a sprint are read at render time, so a page added later still shows.
:::

::: decision {source=agent date=2026-10-04}
The overview and day pages show "Decisions await you" and "Actions await you" in place of "Needs you"; `pm show` lists them under the same two headings, and its JSON keeps one `needs` list with a `kind` field.
One list with a kind keeps the JSON shape that callers read; the two headings are what the owner asked for.
:::

::: decision {source=agent date=2026-10-04}
`pm commit -m "…" <path>…` commits only the named records; with no path it refuses and lists what is uncommitted in the store, so the caller names its own files. A `pm` write whose write or commit fails restores the files it wrote (removing a new one) and unstages them, and says what it restored.
The store is shared by every session of the clone, so "everything dirty" can hold another session's edit or failed write; only the caller knows which files are its own, and a listing makes naming them cheap. Validation still reads the whole working store, as every pm write does.
:::

::: decision {source=agent date=2026-10-04}
Sprint 13 also makes agents reliably raise decisions and actions: an agent cannot end its turn with an owner request asked only in chat. Relayed from the owner by the context-efficiency session on 2026-10-04.
Of six things that waited on the owner in context-efficiency sprint 1, four were asked only in chat and never reached the site; a rule and a memory did not prevent it, so the owner wants it enforced, not remembered.
:::

::: decision {source=agent date=2026-10-04}
Owner requests are named by what they are: `pm decision need` raises a decision, `pm action need` raises an action (and `pm action review` a PR review), `pm decision add` records a decision (with `--need ID` when it answers one), and closing commands sit under the same nouns. Old names are removed, not aliased, and every format refusal shows the expected body shape. Proposed by the owner via the context-efficiency session on 2026-10-04.
A flag (`--kind`) that older pm rejects and newer pm requires, two paths for one owner answer, and format errors without the expected shape cost agents retries.
:::

::: decision {source=agent date=2026-10-04}
An owner request asked only in chat is caught by a Stop hook (`harness/owner_request_hook.py`), wired in `.claude/settings.json` and `.codex/hooks.json`: when the final reply matches a small set of request phrases or a question to the owner and cites no open `human` issue id, it blocks once with a reason naming `pm need add --kind decision|action` and `pm need review`. It lets the stop through on `stop_hook_active`, and fails open (stderr note) when the reply text or `bd` is unavailable. SubagentStop is not wired, so a subagent's report to its parent is exempt.
Block-once is the strongest check that cannot loop; failing open is chosen because a hook bug that blocked every stop costs more than one missed request; subagents report to the main agent, not the owner, and the main agent's own final reply is still checked.
:::

::: decision {source=agent date=2026-10-04}
A decision need whose answer sets no rule is closed with `pm decision close <need-id> --reason "<why it sets no rule>"`, the answer on stdin; it replaces `pm need respond --no-decision`.
It reads like `pm task close --reason` and `pm action done --reason`, and beside `pm decision add --need`, which records an answer, "close" says this one only closes; `answered --no-record` would add a second flag to say what the verb could.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Before this sprint, once the owner closed a need with `bd human respond` (as
  the site told them to), every pm write refused on the answered-need check,
  including `pm decision add --need` that records the answer; the recording
  command now skips the check for its own need and checks it with the planned
  change.

- Version skew: Beads and the records store are shared by every checkout, but
  pm's code is per branch. The demo needs .13.3-.13.5, closed with the new
  action and no-decision labels, made pm on main and every other branch refuse
  (closed need with no citing decision) until they were deleted. Data written
  under new rules breaks older pm until the code merges; new kinds of record
  or label must stay readable by the pm on main, or merge first.

- The owner-request Stop hook takes 0.03 s (3 runs) when the reply has no
  request or cites no id, and 0.48 s when it cites an id, since it then runs
  bd list --label human (0.44 s alone). Live check on Claude Code 2.1.289
  (claude --bg, session 0c4e4170): the hook blocked a reply ending 'needs
  input: should we rename X? please decide'; the agent then raised decision
  need yeeef-agents-9va.13.9 with pm need add and ended citing it, and the
  second stop passed. The need was dismissed afterwards.

- Not yet covered by the owner-request hook: the Codex Stop wiring is untested
  live (Codex 0.131.0 has Stop with block and stop_hook_active, but each hook
  runs only after the owner trusts it in /hooks); pm setup does not install
  the hook in other repos; and no check lists open agent PRs (gh pr list
  --author @me) lacking a review action. A reply that cites any open need
  passes even if it also asks something else.

- Codex Stop hook not yet live: a codex exec run (0.160.0) in the pm-harness
  worktree ran SessionStart and UserPromptSubmit but no Stop hook. Codex
  trusts hooks per file path and hash, and ~/.codex/config.toml trusts only
  the main checkout's .codex/hooks.json entries (post_compact, pre_compact,
  session_start, user_prompt_submit); main has no Stop entry until PR #15
  merges. After the merge the owner trusts the Stop hook once in /hooks from
  the main checkout, then the live check reruns.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: what waits on the owner is split into decisions and actions, each
raised with one command, and a Stop hook stops a turn that asks the owner
only in chat.

- New commands: `pm decision need`, `pm action need` (with `--pr` for a PR
  review), `pm decision add --need`, `pm decision close`, `pm action done`;
  `pm need *` removed with no aliases.
- The site and `pm show` list "Decisions await you" and "Actions await you";
  a review card links its PR, sprints, design pages and focus.
- A small answer closes without a decision record; a rule-setting answer
  still needs one, and `make render` catches a skipped one.
- A Stop hook blocks a turn that asks the owner for something only in chat.
- `pm commit` commits only the records it names; writes are checked against
  the records branch, and a failed write restores what it wrote.
- Seven review fixes; PR #14 (merged as 700be24) lets pm on main read needs
  closed by newer pm.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- One `pm` command each raises a small question and an action, and both
  show under their own heading: met. `pm decision need` and `pm action need`
  (95a67af); the overview and day pages show "Decisions await you" and
  "Actions await you" (df458d4), checked on a live `pm serve` and in
  `test_site_shows_decisions_and_actions_under_their_own_headings`. Review
  requests: `pm action review` links the PR, sprints, design pages and focus
  (`test_action_review_card_links_pr_sprints_design_pages_and_focus`); used
  for real for PR #9 (`9va.16`) and PR #14 (`9va.13.8`).
- A small answer closes without a decision record, and the check still
  catches a rule-setting answer that skipped its decision: met.
  `pm decision close --reason`;
  `test_decision_close_closes_small_answer_without_record`,
  `test_render_still_fails_on_rule_answer_that_skipped_its_decision`.
- RULES.md and the skill describe the two kinds and when to use each: met
  (df458d4, 95a67af).
- An agent that needs the owner cannot end its turn with only a chat
  question: met in Claude Code. A live `claude --bg` session that ended with
  "needs input: …" was blocked by the Stop hook, then raised a need and cited
  it (a6a351b); the hook also blocked one of the main agent's own replies.
  Codex is wired but untested live until the owner trusts the hook
  (action `9va.13.10`).
- Also delivered: `pm commit` takes the paths to commit, writes and commits
  are checked against the records branch rather than other sessions'
  uncommitted files, and a failed write restores what it wrote (0515f44,
  4570f10); seven review findings fixed with tests; 225 tests pass.
