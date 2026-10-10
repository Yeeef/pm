---
type: sprint
title: Answer owner requests on the site
bead: yeeef-agents-9va.19
---

## Goal

> What should be true when this sprint ends, and why now?

The owner answers what awaits them on the site itself: for a decision, states what they want; for an action, pastes the evidence that it is done. The answer reaches the agent that raised the request without the owner switching to a terminal or a chat.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a reply box on each decision and action card served by `pm serve`; storing the reply in Beads (the response on the need, or a comment on the action); getting the reply to the requesting agent (recording which session raised a request, and how that session learns of the reply: a message to it, a hook at its next turn, or `pm show`); what the agent must then do (record a decision, check the evidence, close the request).

**Out:** answering from other devices, which depends on need `yeeef-agents-9va.15.3`; authentication beyond the server listening on 127.0.0.1 only.

## Done when

> What evidence will show the goal is met?

- The owner submits a reply on a decision card and on an action card in the browser, and each lands in Beads on the right issue; checked for real.
- The agent that raised each request receives the reply without the owner telling it, and acts on it (decision recorded or action closed with the evidence); checked with a live session.
- RULES.md and the site docs describe the reply flow.

## Design pages

> Where is the detail?

- [Replies on the site](../design/site-replies.md): the reply form, storage in Beads, delivery to the raising session, and the CSRF token.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-05}
A site reply is a Beads comment (author "owner (site reply)") plus the label `replied` on the still-open request; the raising session is stored in the issue's metadata.session, and it receives the reply by running `pm reply wait` in the background, whose exit wakes it; `pm show` marks undelivered replies. The POST needs a per-server token embedded in the served page and a localhost Host header.
Closing a decision need from the site would fail every render until the agent records it; Claude Code has no command to message a running session and hooks cannot wake an idle one, while a finished background command does. Design: records/design/site-replies.md.
:::

::: decision {source=owner date=2026-10-05}
`pm reply wait` keeps its polling design and reads only the waited issues: `bd show <ids> --json` instead of `bd list --all --json` (and, with no ids, a narrow query for this session's open human requests instead of the whole list).
The owner judged the current architecture fine; listing every issue to watch a few is the part to change, for context efficiency.
Answers `yeeef-agents-9va.19.3`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Sprint numbers and epic ids now differ: sprint 16 is epic
  yeeef-agents-9va.19, because the review actions filed under the project
  (9va.16, 9va.17, 9va.18) took child numbers. Review actions under the
  project make sprint ids harder to read.

- Live check of site replies (commit e797356): a claude --bg session
  (4977a269) raised [TEST] decision yeeef-agents-9va.19.1.1 and [TEST] action
  yeeef-agents-9va.19.1.2, both with metadata.session set, ran pm reply wait
  in the background and went idle. Two curl POSTs with the page's token
  returned 303 and landed as comments by 'owner (site reply)' on the right
  issues; a POST without the token and one with Host evil.example returned
  403. The wait's exit woke the session, which closed the decision with pm
  decision close and the action with pm action done after checking the
  evidence file, 15 s after the first reply (14:07:40 to 14:07:55 UTC). make
  test: 286 passed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the owner replies to a decision or action on its site card, the reply
lands on the issue in Beads, and the agent that raised the request is woken
by `pm reply wait` and acts on it.

- Reply form on each decision and action card, served by `pm serve` with a
  per-server token; POSTs from another host or without the token are refused.
- A reply is a Beads comment by "owner (site reply)" plus the `replied`
  label; the server never closes anything.
- Requests record the raising session (`metadata.session`); `pm reply wait`
  wakes it, and `pm show` flags replies not picked up.
- `pm reply wait` reads only the requests it waits on (`bd show <ids>`, or a
  query for this session's open requests), never the whole issue list
  (owner decision on 9va.19.3).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A reply on a decision card and on an action card lands on the right
  issue: met with form POSTs sent by curl using the page's token, as the
  browser form does; not clicked in a real browser. Each landed as a comment
  on its own `[TEST]` issue; POSTs without the token or from another host
  got 403.
- The requesting agent receives each reply without the owner relaying it and
  acts on it: met. A live `claude --bg` session waiting in `pm reply wait`
  woke, closed the decision with `pm decision close` and the action with
  `pm action done` after checking the evidence, 15 s after the first reply.
- RULES.md and the site docs describe the flow: met (e797356; design page
  `site-replies`).
