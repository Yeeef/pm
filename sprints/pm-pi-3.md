---
type: sprint
title: pm ask tool and owner replies in pi
bead: pm-puvw.3
---

## Goal

> What should be true when this sprint ends, and why now?

A pi agent asks the owner through a pm-registered ask tool, the question shows on the site, and the owner's reply reaches the live pi session. pm's Stop-time checks read pi's session format, so an ask left only in chat is caught.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- An ask tool registered by the pm extension (`pi.registerTool`), raising the question through pm.
- Reply delivery from the pm service into a live pi session.
- `internal/hooks` (stop, asktool, owner request) reading pi's session JSONL: touched paths and ask-tool calls; pi cases in `hook_payloads.json`.

**Out:**
- The judge running on pi; prompt wording for pi (sprint 4).

## Done when

> What evidence will show the goal is met?

- Go and harness tests with pi payloads pass.
- Live check in a scratch clone: a pi session asks through the tool, the owner answers on the site, and the session receives the reply.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
