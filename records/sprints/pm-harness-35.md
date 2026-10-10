---
type: sprint
title: Scheduled push runs on systemd
bead: yeeef-agents-9va.40
---

## Goal

> What should be true when this sprint ends, and why now?

The scheduled push installed by `bin/pm setup` on a systemd machine actually runs every 10 minutes, and setup or `pm where` fail loudly when it does not.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the systemd unit `pm setup` writes (unquoted `WorkingDirectory=`), a check after install that the timer is active, `pm where` reporting a timer systemd did not load, a test validating the generated unit, reinstalling this machine's unit and pushing the backlog.
**Out:** launchd and cron schedules beyond keeping their tests passing; the push job's own logic.

## Done when

> What evidence will show the goal is met?

- The generated service passes `systemd-analyze --user verify` and the timer on this machine is active with a next run time.
- `pm setup` fails, and `pm where` reports the schedule as not running, when the timer is installed but not active.
- A scheduled run has pushed: `origin/records` has the local records commits and `pm show` no longer flags the push as overdue.

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

voided: the same fix shipped first as PR #39 (task yeeef-agents-9va.37.5, sprint 32), so this sprint delivers nothing.

- PR #39 writes `WorkingDirectory=` unquoted and makes `installed()` check `systemctl is-active`, so `pm setup` repairs a dead timer.
- This sprint's extra commit (28116ed on an unpushed worktree branch: fail setup when the timer does not start, `pm where` saying "not running", a `systemd-analyze verify` test) was not shipped.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Generated service verifies and the timer is active: met by PR #39; `systemctl --user list-timers` shows the timer firing every 10 min.
- `pm setup` fails and `pm where` reports not running for an inactive timer: not met here; with PR #39 setup reinstalls an inactive timer instead.
- A scheduled run pushed: met; `.git/pm-push.log` at 2026-10-06T00:56 shows `beads ok … Push complete` and `records ok: pushed 5 commit(s)`.
