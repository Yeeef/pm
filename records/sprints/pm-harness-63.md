---
type: sprint
title: Migrate the Linux server's clone onto installed pm
bead: yeeef-agents-9va.72
---

## Goal

> What should be true when this sprint ends, and why now?

The Linux server's clone of yeeef-agents runs installed pm like this Mac's, so sessions there get pm prime context and its records push through the pm service. Why now: sprint 46 migrated the Mac on 2026-10-07, and the server was unreachable then because of a network issue.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** on the server's main checkout: pull main, `uvx --from "git+https://github.com/Yeeef/yeeef-agents@pm-v0.1.0#subdirectory=pm" pm init`, `pm doctor`; the systemd user service replacing the old push timer or cron entry; the other sessions there resuming on the new store.
**Out:** new pm features; Codex parity (sprint 34).

## Done when

> What evidence will show the goal is met?

- `pm doctor` exits 0 on the server's main checkout after `pm init`. Expected: "every managed piece and the clone's setup match what pm init makes".
- A record committed on the server reaches GitHub through the pm service's push, shown in `pm service logs` and `git ls-remote origin records`.
- A headless `claude -p` session on the server lists all 4 pm rules chunks.

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

done: the Linux server's clone of yeeef-agents runs installed pm, Go pm 0.3.0 on the work store, and syncs with GitHub through its pm service.

- Moved over ssh by the [Linux server runbook](../docs/2026-10-09-linux-server-runbook.md): the old push timer stopped, `.beads/` kept as `.beads.retired`, `.records` moved to `.pm/store/records`, then `pm init`.
- The server's 2 unpushed records commits from 2026-10-07 were already superseded on origin; they stay as tag `server-records-before-go`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** `pm doctor` on the server's main checkout after `pm init`: "every managed piece and the clone's setup match what pm init makes".
- **Met:** the pm service's own push at 2026-10-10 00:29 UTC, in `pm service logs`: "records ok: pushed 1 commit(s) after rebasing onto 2 new on origin/records"; `git ls-remote origin records` shows `35fbc8a`, the server's head; the work store synced, pulling 2 commits.
- **Met, with Go pm's chunking:** a headless `claude -p` on the server lists every pm rules chunk, "pm rules (1 of 2)" and "(2 of 2)": Go pm cuts its rules into 2 chunks where Python pm cut 4.
