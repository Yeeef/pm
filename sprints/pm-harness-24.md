---
type: sprint
title: Requests wait for their answer by themselves
bead: yeeef-agents-9va.29
---

## Goal

> What should be true when this sprint ends, and why now?

An agent that raises a request for the owner is woken when it is answered, without remembering to start a waiter: `pm decision need` and `pm action need` start the wait themselves, and a PR review also wakes the agent when the PR merges. Today the agent must start `pm reply wait` by hand after a hint, and on 2026-10-05 the main session skipped it for the PR #29 review.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the raising commands starting the wait in the background (or the cleanest way the agent's runtime allows), so the reply reaches the agent that asked; a PR review's wait also ending on the PR's merge to main; reading stdin before taking the records-store lock, since an open stdin held the lock for over 4 minutes on 2026-10-05 (finding on sprint 23).

**Out:** answering from other devices (need 9va.15.3); the site's reply speed (sprint 21).

## Done when

> What evidence will show the goal is met?

- A request raised with `pm decision need` or `pm action need` wakes the raising agent on the owner's site reply with no separate command; checked live with a `claude --bg` session.
- A PR review wakes the agent when its PR merges to main, even with no site reply; checked live.
- No pm command reads stdin while holding the store lock; a test with an open stdin shows other writes are not blocked.

## Design pages

> Where is the detail?

- [Replies on the site](../design/site-replies.md): delivery by the reply-wait hook and the wait on a reviewed PR's merge.
- [pm CLI](../design/pm-cli.md): `pm reply wait`.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-05}
A Claude Code PostToolUse hook with asyncRewake (harness/reply_wait_hook.py) starts the wait after pm decision need or pm action need; Codex agents still run pm reply wait in the background.
Only a process the session's runtime owns can wake it, and of those only the hook needs nothing from the agent: a raising command that blocks must still be backgrounded by the agent, a hint must still be acted on, and a detached child wakes nobody. Checked live: an idle claude --bg session woke 2 s after a site reply (15:55:00 to 15:55:02 UTC).
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Claude Code 2.1.289: a command hook with asyncRewake wakes an idle claude
  --bg session on exit 2, its stderr shown as a system reminder (idle
  15:48:30, woken 15:48:50 UTC, 20 s hook). A 5 s timeout kills it silently
  with no wake, so the reply-wait hook uses a 7-day timeout and its pm reply
  wait ends 10 minutes earlier to report.

- Live checks on 2026-10-05: a [TEST] decision need raised by a claude --bg
  session (9va.29.2) woke it 2 s after a POST to a pm serve on port 8765; a
  [TEST] review of the already merged PR #29 (9va.29.3) woke its session 4 s
  after the raise with 'merged to main as c4bc408'. Both dismissed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: in Claude Code a raised request wakes its agent when the owner
replies, and a PR review also wakes it when the PR merges to main, with no
command to remember; no pm command reads stdin while holding the store lock.

- PostToolUse `reply_wait_hook.py` (`asyncRewake`) runs `pm reply wait` on the
  ids a raising command printed and wakes the session on exit 2.
- A review's wait also ends on its PR's merge to main (`gh` plus
  `git merge-base`); a merge into a stacked base keeps waiting.
- Stdin is read whole before the lock.
- Codex has no such hook: Codex agents still start `pm reply wait` by hand.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A raised request wakes its agent on the owner's site reply with no
  separate command: met. A live `claude --bg` session raised a `[TEST]`
  decision with `bin/pm decision need`, went idle at 15:54:45, and was woken
  at 15:55:02, 2 s after a site reply was posted.
- A PR review wakes the agent when its PR merges to main: met with the real
  `gh` on the already-merged PR #29; a session was woken 3 s after going idle
  with "merged to main as c4bc408". The raise was a stand-in (`printf`), not
  `pm action need --pr`; an open PR's wait was checked only by tests.
- No pm command reads stdin under the lock: met,
  `test_a_write_with_open_stdin_does_not_block_other_writes` fails on the old
  code and passes now (e45e8e3).
