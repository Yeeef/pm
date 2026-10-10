---
type: sprint
title: Move the clone out of ~/Desktop
bead: yeeef-agents-9va.36
---

## Goal

> What should be true when this sprint ends, and why now?

This Mac's clone lives outside macOS-protected folders, so the scheduled push job runs, with nothing on the machine still pointing at the old path. The launchd push job installed by PR #34 exits 126 ("Operation not permitted") because background jobs cannot read ~/Desktop; the owner chose to move the clone rather than grant access (need yeeef-agents-9va.30.10).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** an inventory of everything that names the absolute path (the ~/.claude links from make setup-agent: CLAUDE.md, settings.json, commands, skills; 17 git worktrees including the .records store; Claude's per-project transcript and memory folders, named after the path, which sprint 22's liveness check reads; Codex writable_roots in $CODEX_HOME/config.toml; the launchd plist; editor and terminal setups), a move plan that orders the steps and says which sessions must stop first, the move itself, and a check that each item points at the new path.

**Out:** other machines; changing the harness to avoid absolute paths.

## Done when

> What evidence will show the goal is met?

- An inventory lists every place on this Mac that names the old path, each with how it is updated.
- After the move, `bin/pm where` shows the store, links, worktrees and schedule at the new path, `make setup-agent` and `bin/pm setup` change nothing, and no ~/.claude link or worktree points at ~/Desktop.
- The launchd job pushes a fresh records commit by itself (launchctl exit status 0, `git -C .records status -sb` in sync).

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

- Inventory (doc clone-move-inventory): the old path is named by 6 ~/.claude
  links, 17 worktrees (16 gitdir pointers + 16 .git files), core.hooksPath,
  ~/.codex/config.toml (1 project key + 4 writable roots), the launchd plist
  (3 fields), 6 Claude project folders, 1 ~/.claude.json key and 5 VS Code
  recent entries; 0 in shell rc, PATH, crontab, terminals, Beads; tracked
  files name only the Linux /home path. 9 Claude sessions plus pm serve run
  from the clone now. The liveness check globs every project folder, so
  renaming is not needed for it.

- Move done 2026-10-05: a plain `git worktree repair` left all 17 worktrees on
  the Desktop path, so each path had to be passed to it; 17 old `records`
  links had to be deleted before `bin/pm setup` would relink them;
  ~/.codex/config.toml also held 7 `hooks.state` keys with the old path, which
  the inventory missed. `pm setup` installed the new launchd job
  (local.pm-push.yeeef-agents.34034d47), which pushed Beads and records at
  21:17 UTC. Checks: 0 Desktop paths in ~/.claude links, worktrees and the
  Codex config; re-running setup-agent and pm setup changes nothing.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the Mac's clone moved out of ~/Desktop and its launchd push job ran from the new path.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Inventory of every place naming the old path: met.** Doc clone-move-inventory (see Findings); the move also found 7 Codex `hooks.state` keys the inventory missed.
- **After the move, `pm where`, links and worktrees use the new path and setup re-runs change nothing: met.** 0 Desktop paths in ~/.claude links, the 17 worktrees and the Codex config; `make setup-agent` and `bin/pm setup` changed nothing (Findings, 2026-10-05).
- **The launchd job pushes a fresh records commit by itself: met.** Job `local.pm-push.yeeef-agents.34034d47` pushed Beads and records at 21:17 UTC on 2026-10-05.
