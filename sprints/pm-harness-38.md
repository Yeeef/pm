---
type: sprint
title: Request cards show the conversation
bead: yeeef-agents-9va.43
---

## Goal

> What should be true when this sprint ends, and why now?

A request card on the site shows what the owner sent and where it stands, so a reply never looks lost. Today the card says "Reply sent; waiting for the agent" only until the agent picks the reply up (seconds, via the reply-wait hook), then falls back to looking unanswered, and never shows the reply's text.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** each open request card lists its comments (owner replies and agent notes) with time, and states whether the latest reply was picked up; pm serve's transient notes stay consistent with that; pages stop reloading themselves and load newer data only on the owner's reload (owner request, 2026-10-06).
**Out:** editing or deleting replies; replies on closed requests.

## Done when

> What evidence will show the goal is met?

- After a reply is sent and picked up, its card shows the reply's text and "picked up by the agent", checked in a browser on a test server.
- make test passes apart from the known test_review_wait_* failures.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: a request card shows every reply on it and whether the agent picked it up, and pages load newer data only when the owner reloads.

- Cards list their comments oldest first with author, UTC time and "waiting for the agent" or "picked up by the agent"; a sent reply's note stays until the snapshot holds its comment (PR #42, 4dcdb7e).
- Pages never reload themselves: a sticky "Newer data: reload" banner appears; reloading keeps scroll and asks before discarding an unsent reply (PR #43, 69a131f).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- After a reply is sent and picked up, its card shows the text and "picked up by the agent": met. Browser check on a test server (fake bd): the reply showed "waiting…", then after setting picked_up=1 "picked up by the agent", text kept (PR #42).
- make test passes apart from the known test_review_wait_* failures: met. #42 373 passed / 3 known failed; #43 371 passed / 3 known failed; main after both merges, 376 passed, 7 skipped.
- On-demand reload (scope added 2026-10-06): browser check, a window marker survived 15 s after a data change, banner shown, clicking it loaded the new content at the same scrollY (340); with a draft, a confirm appeared and cancel kept it (PR #43).
- Fresh-context review of both PRs: no high or medium findings; the two low findings that could lose owner text were fixed (9a298b0, 92b39a3).
