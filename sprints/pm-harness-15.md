---
type: sprint
title: Rendered record links
bead: yeeef-agents-9va.15
---

## Goal

> What should be true when this sprint ends, and why now?

An agent asked to link a sprint or design page gives the owner the rendered page straight from `pm` output, never raw Markdown and with no investigation. Today an agent must work out where the site is served, on which port, how a record maps to its page and whether the records are pushed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** (owner choices relayed from the context-efficiency session on 2026-10-04)
- `pm show` gains one constant-size line naming the site and how a record maps to its page; no per-item URLs in plain `pm show`, which must not grow with project history (`--json` may carry URLs).
- `pm record link <target>`: target is a sprint id, project name, design slug or record path; prints the rendered URL, and fails with the fixing command when no site is served instead of printing a dead link.
- Whether a raw record link in a reply is blocked, and how the owner views pages from other devices: both raised as needs.

**Out:** a `pm status` command for store state (parked by the owner).

## Done when

> What evidence will show the goal is met?

- Asked to link a sprint or design page, an agent gets a rendered URL straight from `pm` output; checked for real.
- `pm show` grows by one line regardless of project size; a test pins it.
- The two needs are answered, and whatever they choose is built or moved to a sprint.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): `pm record link` and the `site:` line of `pm show`
- [Views and the site](../design/views-and-site.md): `pm serve` and the `X-PM-Store` header

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-04}
Raw record links in agent replies are covered by a RULES.md rule ("link records only with the URL pm prints"); no hook blocks them.
The owner judged a rule enough; a hook would add a stop-time check and false positives for little gain.
Answers `yeeef-agents-9va.15.2`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The site the owner browses (make docs on port 8000, a static server started
  Oct 2 over the main checkout's site/) went stale: its last render predated
  four open needs, so the owner did not see them under Needs you. Nothing
  re-renders when Beads or records change; a fresh make render showed them. PR
  reviews never appear on the site, since nothing in records tracks them,
  unless raised as needs.

- pm action review files the review under the first open sprint it names, so
  an unreviewed PR blocks that sprint's close; review actions 9va.13.8,
  9va.13.10 and 9va.15.4 were moved under the project epic by hand. Review
  actions belong under the project.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: an agent links a record for the owner with `pm record link`, which
gives the rendered page only when this store's site is served, and `pm show`
names the site in one fixed-size line.

- `pm record link <target>` (sprint or project id, project name, design slug
  or record path); refuses when nothing or another server answers.
- `pm show` ends with one `site:` line; `--json` carries URLs.
- RULES.md: link records only with the URL pm prints (owner decision on
  9va.15.2: a rule, no hook).
- Pages from other devices (owner answer on 9va.15.3): the site is reached
  through the owner's Cloudflare tunnel behind Cloudflare Access;
  `pm setup --site-url URL` (9va.15.5.2) makes links use it and lets the site
  take replies from that host (PR #37, 2683c27).
- A data refresh keeps the reader's scroll position instead of jumping back
  to the replied card (PR #40, b1d7989).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- An agent asked for a sprint or design page gets a rendered URL from pm:
  met. With no server on :8768 it refused and named `PORT=8768 make docs`;
  with one, sprint 15 and `records-store` resolved to URLs that returned 200
  (cb9c52d).
- `pm show` grows by one line regardless of project size: met,
  `test_show_site_line_has_constant_size`.
- Both needs answered and built or moved: met. 9va.15.2 answered (a rule,
  no hook) and built; 9va.15.3 answered (Cloudflare tunnel) and built in
  PR #37: `pm record link yeeef-agents-9va.15` printed
  https://pm.yeeefs.com/sprints/pm-harness-15.html, an anonymous GET there
  gets 302 to the Access login, and the owner's test reply from
  pm.yeeefs.com reached 9va.15.6 as a comment. PR #40's scroll fix was
  checked in a browser on a test server: scrollY 725 before and after a
  data reload, hash dropped.
