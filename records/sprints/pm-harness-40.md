---
type: sprint
title: Deliver every owner reply at least once
bead: yeeef-agents-9va.46
---

## Goal

> What should be true when this sprint ends, and why now?

Every owner reply on the site reaches the session that raised the request at least once, however many requests are open and whenever the replies arrive. Today a reply can be lost: a waiter stops at the first reply and nothing waits for the rest. A reply delivered twice is tolerated; a reply never delivered is the bug.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** push delivery of owner replies and PR merges from `pm serve` into the raising Claude Code session's inbox socket; `pm reply read` and `pm show` for replies not delivered; removing `harness/reply_wait_hook.py`, its hook entries and the waiting `pm reply wait` (project decision answering yeeef-agents-9va.44.2).

**Out:** Codex (sprint 34, yeeef-agents-9va.39); sessions on another machine than `pm serve`; de-duplicating replies (the form storing a resent reply twice), which at-least-once delivery tolerates.

## Done when

> What evidence will show the goal is met?

- Tests show each of the three observed failures (several requests raised in one command, output piped through grep, a second reply to one request) delivered by push, and a reply to an ended session shown by `pm show` and read with `pm reply read`.
- A live check raises several needs, one with output piped through grep, and the owner replies at different times, including a second reply to one need and a reply while the session is idle: every reply arrives in the session as a new turn, and no reply-wait process runs.
- A PR merge of an open review reaches the session without an owner reply.

## Design pages

> Where is the detail?

- [Reply delivery](../design/reply-delivery.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-06}
Sprint 40 targets at-least-once delivery of owner replies; a duplicate delivery or a twice-stored reply is tolerated and out of scope.
The owner said the failure is that replies are not delivered at least once, not that they are not delivered exactly once.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Cause (a), confirmed: one waiter per raising command, and it ends on the
  first ready request. Session af7821c4 raised 9va.38.5-.8 in one Bash call
  (transcript 01:27:53Z), so reply_wait_hook.py:33-46 ran one pm reply wait
  over all four ids; cmd_reply_wait (pm.py:1450-1497) breaks on the first poll
  where any id is ready (:1484-1486), prints the ready ones, and returns. The
  01:33:41Z wake delivered .38.6 and .38.8; nothing re-armed a wait on .38.5
  and .38.7, so their replies (01:34, 01:36) went undelivered. Raising them in
  separate calls would not have helped later replies: each waiter still exits
  after its request's first reply, and a second reply to a request already
  picked up has no waiter.

- Late second wake, partly unexplained: a PostToolUse:Bash wake at 01:43:38Z
  reported .38.5/.38.7/.38.8 closed (closed 01:39:03, 01:39:05, 01:33:51) and
  the duplicate .38.6 reply (01:33:57), i.e. a waiter on ids .5-.8 that
  started after 01:39:05; pgrep at 01:39:07 found none. The only Bash call in
  between (01:42:52Z) raised .38.9 with output piped through grep -o, so its
  stdout lacks ' under ' and RAISED (reply_wait_hook.py:28) cannot match it,
  and .38.9 is not in the wake. Hypothesis, unconfirmed: Claude Code re-ran
  the earlier raising call's async hook (or replayed its event) at the next
  matching Bash call (if: Bash(*need*), settings.json:54); this needs a
  hook-invocation log to settle. Either way, delivery of later replies
  depended on an unrelated later command.

- Cause (b): the .38.6 reply is two comments, 01:33:36Z (rid 70cf1cce) and
  01:33:57Z (rid a5d3949e), identical text, 21 s apart. pm serve dedupes only
  by reply id (spool_add pm.py:1106-1119, deliver_reply :1122-1134), so two
  rids store twice. The form JS (site.py:358-363) keeps a rid only while
  this.dataset.text equals the text; every fresh render of the form
  (fill_replies site.py:372-387, after the 303 redirect at pm.py:1366 or a
  /version reload) starts with an empty rid and data-text, so a send of the
  same text from a re-rendered form mints a new rid. A double click within one
  page reuses the rid and is safe. Hypothesis, unconfirmed: the owner sent the
  text again 21 s later from a re-rendered form (browser form restore after
  back/reload, or a resend before the Saving/Sent note was noticed). Confirmed
  consequence: picked_up counts comments (beads.py:119-131), so the copy read
  as a new reply: the 01:43 wake re-delivered it and .38.6 now has
  picked_up=2.

- Seen again 2026-10-06: owner replied on the site to 9va.38.6, .38.9 and
  .38.10 at 02:08-02:09; no wake reached the session, though 3 pm reply wait
  processes were running at about 02:12; the agent learned of the replies only
  when the owner said so in chat.

- Seen a third time 2026-10-06: the owner's site reply to 9va.38.13 (15:58)
  reached the session only when the owner asked in chat; that need was raised
  alone, so the missing re-arm after a wake on several needs is not the only
  cause.

- Cause of the 15:58 miss (confirmed): uv's managed CPython 3.11.11 appeared
  at 2026-10-06 02:25Z, and uv now picks it for pm.py (requires-python
  >=3.11), but pm.py:315 uses a backslash inside an f-string expression, valid
  only from Python 3.12; so every bin/pm run on this machine fails with
  SyntaxError, including reply waiters and session hooks, unless
  UV_PYTHON=3.12 is set. The 01:34 and 02:08 misses predate it.

- Cause of the 02:08 miss (confirmed): no waiter ever watched 9va.38.9 or
  .38.10, and none was re-armed on .38.6. Session af7821c4 raised .38.9
  (01:42:52Z) and .38.10 (02:03:48Z) with output piped through grep -o 'raised
  decision need [^ ;]*', which drops ' under ', so the RAISED regex never
  matched; .38.6's only waiter ended at the 01:43 wake. Beads shows picked_up
  was never set on .38.9/.38.10, and any waiter on them would have woken
  within 5 s of the 02:08:18/02:08:41 replies and exited when they closed at
  02:09:5x, so the 3 pm reply wait processes seen at 02:12 waited on other
  ids. Fixed on branch reply-wait-rearm: one waiter per session over all its
  open requests, re-armed on every Bash call and turn end.

- Live check of push delivery, 2026-10-06 23:19Z, branch reply-push (e573bb9):
  a claude -p --input-format stream-json session (haiku, idle, stdin held
  open) had its inbox stored on [TEST] need 9va.46.3; pm serve on port 8791
  logged 'push yeeef-agents-9va.46.3: delivered' for two replies POSTed as the
  site form does, 13 s apart; the session took a new turn for each (results at
  23:19:52 and 23:19:59, 13 s and 7 s after the POSTs) quoting the reply text,
  and picked_up ended at 2. The session shows the message as a relay from
  another Claude session. Need dismissed after.

- Live check of push delivery passed (2026-10-06 22:07-22:15Z): a subagent
  posted 4 replies through the real form in a browser on a reply-push pm serve
  (port 8790; port 8000 left untouched), to [TEST] needs 9va.44.4 and .44.5
  (raised in one command) and .44.6 (output piped through grep), plus a second
  reply on .44.4. All 4 reached this bridge session as new turns within about
  1 s of being stored, 3 of them while it was idle. The serve log shows one
  'delivered' per reply, picked_up ended at 2/1/1, and no push failed or was
  retried. Not covered: a push into a busy session, and a PR merge (it is
  checked when this sprint's PR merges with port 8000 on the new code).

- Card note race seen in the live check: for 2 of the 4 replies the card said
  'not delivered yet: the session is not running, or the push failed' for a
  few seconds although the push succeeded at once (one 'delivered' log line,
  no retry); a reload showed delivered. The just-sent note is read before the
  writer records picked_up. It is cosmetic and transient, and never says
  delivered falsely.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm serve` now pushes every owner reply and every reviewed PR's merge into the session that asked, verified live for both.

Merged as 02c1fe0 (PR #50).

- `pm serve` pushes each site reply into the inbox socket the raising command recorded (`CLAUDE_CODE_MESSAGING_SOCKET`, stored as `metadata.inbox` with its host), marking it delivered only after the socket accepts it; failed pushes are retried every 60 s.
- `pm serve` watches open reviews' PRs and pushes a merge once, through the same code.
- `pm reply read` replaces `pm reply wait`; the reply-wait hook and its `.claude/settings.json` entry are gone.
- Only site replies count as undelivered, so a need answered in chat closes cleanly; closing a request with an undelivered site reply is refused.
- A resumed session's open requests are pointed at its new inbox at session start.
- Design page: reply-delivery (site-replies' Delivery and Merge sections now summarise and link it).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Tests of the three observed failures and of an ended session: met.** `test_every_reply_is_pushed_into_the_session_that_asked` (two needs raised in one command, one piped through grep, a second reply: 4 pushes, `picked_up` 1 then 2), `test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read`, `test_a_push_that_failed_is_retried_by_the_sweep`, `test_a_need_answered_in_chat_closes_and_nothing_is_flagged`. Full suite on `reply-push`: 386 passed, 7 skipped (`uv run --quiet tests/run.py`).
- **Live check: met, except a busy session.** On 2026-10-06 22:07-22:15Z a subagent posted 4 replies through the real form in a browser to a `reply-push` `pm serve` (port 8790), to 9va.44.4 and .44.5 (raised in one command) and .44.6 (piped through grep), plus a second reply on .44.4. All 4 arrived in the raising session as new turns, 3 while it was idle; one "delivered" log line each, `picked_up` 2/1/1, and no reply-wait process ran for that session. The owner asked for the subagent instead of replying themselves; port 8000 was not restarted because stopping it was refused. Not covered: a push into a busy session.
- **A PR merge reaches the session without an owner reply: met.** After PR #50 merged and the owner restarted port 8000 on main, `pm serve` pushed "PR #50 of review yeeef-agents-9va.46.4 merged to main as 02c1fe0…" into the raising session as a new turn, with no owner reply; `test_a_reviewed_prs_merge_is_pushed_into_the_session_once` also passes.
