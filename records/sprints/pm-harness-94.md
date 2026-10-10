---
type: sprint
title: The pm site runs off this Mac and reads its data from GitHub
bead: yeeef-agents-9va.103
---

## Goal

> What should be true when this sprint ends, and why now?

The owner reads the pm site at its public URL while this Mac is asleep or off. The hosted site builds every page from what is on GitHub: the `records` branch and the Beads data on `refs/dolt/data`. Agents push records and Beads soon after each change, so the hosted site stays close to the local state. Why now: today the site and the tunnel run on this Mac (`127.0.0.1:8000` behind https://pm.yeeefs.com), so the owner's view goes down whenever the Mac sleeps, and pushes run only every 10 minutes.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A design page for the hosted site. It covers where the site runs, how it reads Beads without the local embedded Dolt store, how it gets fresh data (a push trigger or a poll), and how Cloudflare Access guards it.
- A host for the site, chosen through a decision need: a static build on each push (GitHub Action to Cloudflare Pages), or `pm service` in a read-from-GitHub mode on a server or container.
- A read path for Beads from GitHub. Two candidates: `bd` clones `refs/dolt/data`, or `pm push` publishes a JSON snapshot of the Beads export.
- Push on change: `pm commit` and pm's Beads writes start a debounced push of records and Beads. The 10-minute loop stays as a backstop.
- Site features that need the local machine show an honest state on the hosted site: the push banner, the merge watcher, `/version` auto-reload and day summaries (`claude -p`).
- Moving https://pm.yeeefs.com to the hosted site, with the local site kept on localhost.
- Owner replies on pm.yeeefs.com: the hosted site takes a reply and queues it, and the Mac's pm service collects it, stores it with `bd` and delivers it to the asking session. A reply sent while the Mac is off waits in the queue.

**Out:**
- The Go port's service (`pm-go`). This sprint changes the Python pm only.
- Replacing Beads with pm's own work store.

## Done when

> What evidence will show the goal is met?

- With this Mac's pm service stopped, every page at https://pm.yeeefs.com loads and matches the local site's render of the same GitHub state. The check is a page-by-page diff of the two sites' HTML, excluding the local-only parts.
- A record committed with `pm commit`, and a task closed with `pm task close`, both show on the hosted site within 5 minutes. The check is two timed runs.
- A request to the hosted site without a valid Cloudflare Access token gets 403. The check is a `curl` run without the token.
- A reply posted on https://pm.yeeefs.com reaches the asking session and shows as a Beads comment, including a reply posted while the Mac's pm service is stopped, which arrives once the service starts. The check is two runs, one with the service running and one with it stopped.
- The PR is merged to main, and the design page describes the final state.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-08}
The pm site keeps one hostname, pm.yeeefs.com, for reading and for replying; no second hostname serves the local site.
The owner wants one consistent address on every device, stated in chat on 2026-10-08.
:::

::: decision {source=owner date=2026-10-08}
Replies on pm.yeeefs.com move to the hosted site in this sprint; the cutover waits until replies work there.
The owner chose option in on 2026-10-08, so the one hostname never loses a feature the owner uses from the phone.
Answers `yeeef-agents-9va.103.8`.
:::

::: decision {source=owner date=2026-10-08}
The hosted pm site is a static build on Cloudflare Pages: a GitHub Action renders it on each push, a Pages Function on pm.yeeefs.com queues replies in Cloudflare D1, the Mac's pm service polls and delivers them, and Cloudflare Access guards both.
The owner chose option A on 2026-10-08: least to operate, stays up while any one machine is down, and fits the existing Cloudflare and Access setup.
Answers `yeeef-agents-9va.103.2`.
:::

::: decision {source=owner date=2026-10-08}
pm pushes records and Beads on each change, debounced over 30 s, so the hosted site shows a change within about 3 minutes; the 10-minute push loop stays as a backstop.
The owner chose option fast on 2026-10-08: changes show soonest, and pushes run only when something changed.
Answers `yeeef-agents-9va.103.1`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- This sprint's frame no longer holds in Yeeef/pm. It reads Beads from
  refs/dolt/data and changes the Python pm only (Out: 'This sprint changes the
  Python pm only'). Go pm replaced both: its work store is its own Dolt
  database, and Python pm is being retired. A hosted site for Go pm needs a
  new frame; the owner's four decisions here (one hostname, Cloudflare Pages,
  push on change, replies hosted) are its inputs.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

voided: the frame reads Beads and changes the Python pm only, and Yeeef/pm has neither, so a hosted site for Go pm needs a new sprint.

- Shipped: the owner's four decisions on the hosted site, kept as inputs for that sprint. No code shipped.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Every page at https://pm.yeeefs.com loads with this Mac's pm service stopped and matches the local render: not met. No hosted site was built; the design and build tasks are closed as obsolete.
- A `pm commit` and a `pm task close` show on the hosted site within 5 minutes: not met. Push on change was not built.
- A request to the hosted site without a valid Cloudflare Access token gets 403: not met here. The token check on the pm service is sprint 68's work, now a Go port.
- A reply posted on https://pm.yeeefs.com reaches the asking session, with the service running and stopped: not met. The hosted reply queue was not built; its design assumed Beads comments.
- The PR is merged to main and the design page describes the final state: not met. No PR or design page was made.
