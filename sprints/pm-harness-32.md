---
type: sprint
title: Continue work from a clone on another server
bead: yeeef-agents-9va.37
---

## Goal

> What should be true when this sprint ends, and why now?

The owner continues this repo's work from a fresh clone on another server, with the harness (Beads, records store, site, scheduled push, ~/.claude links, Claude and Codex hooks) working there exactly as on the Mac, and no Beads or records data lost or forked between the two clones.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a checklist of what a fresh clone on the server needs (tools, auth, secrets, `make setup-agent`, `bin/pm setup`, the scheduler) and what does not travel through git (Claude project memory, `.env`, editor and agent settings); a handoff plan that stops the Mac from writing so the server takes over cleanly; the clone and setup on the server; a check that the server's scheduled push works and the site serves.
**Out:** both clones writing at once (the scheduled `pm push` never pulls, so concurrent Beads writes would diverge; that is a harness change for a later sprint); moving existing Claude transcripts beyond the main project folder.

## Done when

> What evidence will show the goal is met?

- On the server, `bin/pm where` shows the store, links and schedule installed, and re-running `make setup-agent` and `bin/pm setup` changes nothing.
- The server's scheduled job pushes a fresh records commit and Beads by itself, and the Mac sees both after a pull.
- `make docs` on the server serves the site with current data.

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

- Server push never ran: pm setup wrote WorkingDirectory="…" quoted; systemd
  reads the quotes as part of the path, refused the unit as not absolute, and
  the timer stayed dead, while pm where said installed (it checked
  is-enabled). Fixed in PR #39 (unquoted, check is-active); after re-running
  bin/pm setup the timer's first run exited 0, pushed Beads and 5 records
  commits, and .records was level with origin/records.

- Running make setup-agent from a worktree repoints every ~/.claude link
  (CLAUDE.md, settings.json, commands, skills) at that worktree, as
  documented; checking the server from a session worktree did this once and it
  was undone by re-running it from the main checkout.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: this repo's work continues from the clone on the Linux server, with the harness working there as on the Mac.

- Server setup checklist and handoff plan (doc server-clone-plan).
- PR #39 (merged as 22353b6): the systemd push unit loads (WorkingDirectory unquoted) and `pm where` reports a timer that is not running as not installed.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`bin/pm where` shows the store, links and schedule; re-running setup changes nothing: met.** `pm where` on the server lists the store, records link, Beads (team-maintainer), hooks, Codex roots and the systemd timer `local.pm-push.yeeef-agents.c9b30263` (active). From the main checkout, `make setup-agent` linked 0 items and `bin/pm setup` printed only "already set up".
- **The scheduled job pushes records and Beads by itself; the Mac sees both: met on the server side.** After the PR #39 fix, the timer's first run exited 0 and logged `beads ok … Push complete` and `records ok: pushed 5 commit(s)`; `.records` is level with `origin/records`. The Mac is handed off and was not pulled.
- **`make docs` on the server serves current data: met.** `pm serve` listens on 127.0.0.1:8000, returns 200, and the page read "Data as of 02:56:51 (1 s ago)".
