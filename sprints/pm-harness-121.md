---
type: sprint
title: Claude Code's auto memory is off in this repo
bead: yeeef-agents-9va.131
---

## Goal

> What should be true when this sprint ends, and why now?

Claude Code sessions in pm's repo keep no auto memory of their own: the owner asked for it off for the whole repo, so every session (main checkout and worktrees alike) works from the repo, its records and pm alone, never from a per-machine note that no other session or runtime sees.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `"autoMemoryEnabled": false` in the committed `.claude/settings.json`, beside the hooks that `pm init` writes there and keeps.

**Out:** pm's own install (`pm init` writes no memory setting into users' repos); Codex; the existing memory files on this machine.

## Done when

> What evidence will show the goal is met?

- `.claude/settings.json` on main holds `"autoMemoryEnabled": false` and its hooks unchanged.
- `pm init` in a worktree of the branch leaves the key in place (`git diff --exit-code .claude/settings.json` after it).
- The PR's CI is green and its review need is raised.

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

done: the committed `.claude/settings.json` sets `"autoMemoryEnabled": false`, so no Claude Code session in this repo saves or loads an auto memory (PR #92).

Merged as dcfbe4e (PR #92).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`.claude/settings.json` on main holds `"autoMemoryEnabled": false` and its hooks unchanged:** met on the PR's branch, at commit `47f45bd`. The diff adds that one key and leaves the hooks byte for byte. It is on main once PR #92 merges.
- **`pm init` in a worktree of the branch leaves the key in place:** met. After `pm init` in the `auto-memory-off` worktree, `git diff --exit-code .claude/settings.json` exited 0.
- **The PR's CI is green and its review need is raised:** met. All 13 checks of PR #92 pass, and its review need is raised with this report.
