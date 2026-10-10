---
type: sprint
title: New worktrees always get their records link
bead: yeeef-agents-9va.61
---

## Goal

> What should be true when this sprint ends, and why now?

Every new worktree reaches the records store through its `records/` link, whatever tool created it, so a session never starts without its records. Why now: Claude Code created a bridge worktree with no checkout and then ran `git reset`. Git does not run `post-checkout` for a reset, so `bin/pm setup` never ran and the session started with no `records/` link.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The session-start hook (`session_context_hook.py`, Claude Code and Codex) runs `bin/pm setup` at every session start, so a worktree created without a checkout and then reset gets its link (owner decision, superseding the answer to yeeef-agents-9va.61.7).
- The session-start hook also injects `pm where` beside `pm show`, so the start-up context states the link and every other location (owner decision).
- A test of a worktree made without a checkout, then reset: its `records/` link exists after session start.
- RULES.md, CLAUDE.md and the records-store design page say what covers each case.

**Out:**
- Changes to how Claude Code creates worktrees.
- The rest of `bin/pm setup`.

## Done when

> What evidence will show the goal is met?

- A test shows that session start links `records/` in a worktree that `post-checkout` skipped.
- A live check: in a new Claude Code worktree session, `bin/pm where` shows the `records/` link at session start with no manual step.
- RULES.md, CLAUDE.md and the design page match the behavior.

## Design pages

> Where is the detail?

- [Records store](../design/records-store.md): the Layout section names what links `records/` in each case.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
Session start both fixes and shows the link: the hook runs pm setup when records/ is not a link, and it injects pm where beside pm show, so the start-up context states each location and its state.
The owner asked for both, so an agent sees the link's state at start even when setup fails.
:::

::: decision {source=owner date=2026-10-07}
Session start runs no pm setup. The git hooks link records/ in every new worktree, including one created without a checkout and then reset; session start keeps showing pm where.
pm setup is a per-worktree task, not a per-agent-session task. Answers `yeeef-agents-9va.61.7`.
:::

::: decision {source=owner date=2026-10-07}
The session-start hook runs pm setup at every session start, unconditionally, and injects pm where; no git hook runs setup beyond what main had. This replaces the decision answering yeeef-agents-9va.61.7.
pm setup makes every check itself and changes nothing in a set-up worktree, so one unconditional call at start covers every new worktree an agent works in, whatever tool made it.
:::

::: decision {source=agent date=2026-10-07}
Moved yeeef-agents-9va.61.6 to yeeef-agents-9va.65: Sprint 52 shipped without it: the link it would make writable may go away in sprint 56.
Whether Claude Code needs the store in additionalDirectories depends on sprint 56's answer on the records/ link, so the task waits there.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Reproduced the bug in a test: a worktree added with --no-checkout and then
  reset gets no records/ link. The new test fails against the old hook and
  passes with the fix.

- Ran the hook as Claude Code runs it in this session's own bridge worktree,
  which had no records/ link. It linked records/ to the store, and pm where
  then showed 'records link set up'.

- Full harness suite: 393 passed, 7 skipped, 1 failed. The failure is
  test_a_push_that_failed_is_retried_by_the_sweep, which is flaky (1 of 3
  reruns fails) and is pm serve code this sprint does not change.

- Review found the real case is not a missing records/: main tracks a copy of
  records/, so a reset bridge worktree can hold a directory there. The hook
  now runs setup whenever records/ is not a link, and skips the store.

- Timing: pm setup in a fresh bridge worktree took 0.42, 0.43 and 1.61 s, and
  the whole hook took 3.9 s. Setup gets 5 s and the hook's total budget stays
  under the runtimes' 30 s timeout. Full suite after the fixes: 396 passed, 7
  skipped.

- Session start now injects pm where above pm show. In this worktree the hook
  printed 'records link set up' on the checkout line. Full suite: 397 passed,
  7 skipped.

- Claude Code's worktree sequence, from this worktree's git dir and reflog:
  git worktree add --no-checkout, a copy of the sparse-checkout config, then
  git reset --hard. A scratch repo showed the reset runs post-index-change in
  the new worktree; nothing runs post-checkout.

- Inside a hook, git exports GIT_DIR, so pm setup's git calls in the store hit
  the wrong repository ('is not a worktree on branch records'). The hook
  unsets GIT_DIR, GIT_INDEX_FILE and GIT_WORK_TREE first.

- Live check on this repo: a scratch worktree made with --no-checkout, then
  git reset --hard with this branch's hooks, got its records/ link in 1.30 s.
  Full suite: 395 passed, 7 skipped.

- Second review: git waits for post-index-change on every index write, so a
  hung or always-failing setup would stall every git command. The hook now
  tries setup once per worktree (a marker in its git dir) within 20 s. Full
  suite: 396 passed, 7 skipped.

- A plain git worktree add runs post-index-change in the new worktree before
  post-checkout, so post-index-change alone covers every new worktree;
  post-checkout's setup call was removed. Full suite: 398 passed, 7 skipped.

- Owner changed the design: session start runs pm setup every time, and the
  post-index-change hook is dropped. Pre-merge check: a scratch worktree of
  this repo made with --no-checkout and reset got its link from this branch's
  session-start hook; the whole hook took 3.16 s.

- Owner asked to drop setup from post-checkout too: session start covers every
  case it did, and it missed Claude Code's worktrees. Full suite: 396 passed,
  7 skipped.

- Live check after merge: a new Claude Code session started with claude -w
  sprint52-live had its records/ link at start. Its start-up context began
  with 'bin/pm setup at session start: linked …/sprint52-live/records ->
  …/.records', and pm where showed 'records link set up'.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: session start now runs `pm setup` and shows `pm where`, so every worktree an agent works in, including one Claude Code creates without a checkout, has its `records/` link at start.

Merged as a4f6929 (PR #57).

- The session-start hook runs `bin/pm setup` (6 s limit), then injects `pm where` and `pm show`; each fails open to one line.
- `post-checkout` no longer runs setup, so session start is the one place it runs; a worktree used without an agent session needs `bin/pm setup` by hand. A `post-index-change` hook was tried and dropped (see Decisions).
- RULES.md, CLAUDE.md and the records-store design page describe what runs setup in each case.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A test shows that session start links `records/` in a worktree that `post-checkout` skipped: met. `test_session_start_sets_up_a_worktree_post_checkout_skipped` covers no `records/` and main's tracked copy; `test_session_start_setup_fails_open_with_one_line` covers a failing setup. Full suite: 396 passed, 7 skipped, 0 failed on the last run (the flaky `test_a_push_that_failed_is_retried_by_the_sweep`, yeeef-agents-2se, fails on some runs).
- A live check in a new Claude Code worktree session: met. After the merge, a session started with `claude -w sprint52-live` had `records -> …/.records` at start; its start-up context began with `bin/pm setup` at session start: linked …, and `pm where` showed "records link set up".
- RULES.md, CLAUDE.md and the design page match the behavior: met; see the [Records store](../design/records-store.md) Layout section.
