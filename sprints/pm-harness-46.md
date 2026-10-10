---
type: sprint
title: Move yeeef-agents onto installed pm
bead: yeeef-agents-9va.52
---

## Goal

> What should be true when this sprint ends, and why now?

yeeef-agents uses pm the way any other repo does, installed and pinned, with its legacy pieces gone, so the product is proven on its first real user. Design: [pm as an installable product](../design/pm-product.md), section "Migrating yeeef-agents".

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm init` here: dropping the RULES import from CLAUDE.md, replacing path-based hook entries, wrapping pm's unmarked git hook code, renaming the workflows, `.gitignore`; moving the store to `.pm/store` and reinstalling the push job in each clone; removing `bin/pm` and the harness copy from `skills/`; updating CLAUDE.md and docs.
**Out:** new pm features.

## Done when

> What evidence will show the goal is met?

- `pm doctor` is clean in this repo, on the Linux and macOS clones.
- A new session here gets its context from `pm prime`, and sessions, the site and the scheduled push work as before.
- No file in this repo still refers to `bin/pm` or `skills/project-management/harness/`.

## Design pages

> Where is the detail?

- [pm as an installable product](../design/pm-product.md)

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

partial: this Mac's clone runs installed pm with its legacy pieces gone; the Linux server's clone was unreachable and moves to sprint 63.

Merged as 7a0f7c2 (PR #63). Merged as d4e64a6 (PR #65).

- `pm init` migrates a clone set up by the old harness (`pm/src/pm/legacy.py`): it refuses while the old `.records` store holds uncommitted or unpushed records, removes the old push job, moves the store to `.pm/store/records` under locks, repoints every worktree's `records` link and local settings, and strips the RULES import, path-based hook entries, unmarked git-hook code, old workflow names and old `.gitignore` lines; `pm doctor` reports any left.
- `bin/pm` and all of `skills/project-management/` are removed; CLAUDE.md points at `pm init`, the `pm prime` hooks and `pm/AGENTS.md`.
- Reached main through [PR #65](https://github.com/Yeeef/yeeef-agents/pull/65) as d4e64a6, tagged `pm-v0.1.0`; the Mac migrated the same hour.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`pm doctor` is clean in this repo, on the Linux and macOS clones: met on macOS, not on Linux.** On the Mac, `uvx --from "git+…@pm-v0.1.0#subdirectory=pm" pm init` moved the store, removed launchd job `local.pm-push.yeeef-agents.34034d47`, repointed 22 `records` links (the main checkout and 21 worktrees) and installed `local.pm.yeeef-agents.34034d47`; `pm doctor` then printed "every managed piece and the clone's setup match what pm init makes" (exit 0). The Linux server was unreachable; sprint 63 holds its migration.
- **A new session gets its context from `pm prime`, and sessions, the site and the push work as before: met on macOS.** A headless `claude -p` session in the main checkout listed all 4 rules chunks (arriving 1, 4, 3, 2) and the `pm show` state; `pm commit` wrote 34eba6e on the new store; the pm service serves the site on :8000 (index and `style.css` 200) and its first push at 19:43 UTC sent 7 records commits, 34eba6e among them, to GitHub.
- **No file still refers to `bin/pm` or `skills/project-management/harness/`: met.** `git grep -l -e bin/pm -e project-management/harness origin/pm-migrate -- ':!records'` gives `legacy.py` and `test_migrate.py`, which must name the old forms, and `test_tool.py`, whose `pm/bin/pm` is the uv tool's own path; no file is left under `skills/project-management/`.
