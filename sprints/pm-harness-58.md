---
type: sprint
title: Claude Code writes records through records/ without asking
bead: yeeef-agents-9va.67
---

## Goal

> What should be true when this sprint ends, and why now?

A Claude Code session in any worktree writes records through `records/` without a permission prompt, so hand edits of records do not stop for the owner. Why now: `records/` resolves to the store outside the worktree, so Claude Code asks before each write, and the owner saw those prompts in sprint 52.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm setup` adds the store to `permissions.additionalDirectories` in the worktree's `.claude/settings.local.json`, as it adds Codex's `writable_roots`, and the repo ignores that file.
- Tests, and a live check of a write through `records/` with and without the setting.
- CLAUDE.md, SKILL.md and the records-store design page.

**Out:**
- Sessions started with `claude -w`: their worktree isolation blocks the store whatever the settings say.
- Whether the `records/` link stays (sprint 56).

## Done when

> What evidence will show the goal is met?

- Tests show setup adds the store, keeps the file's other settings, changes nothing on a re-run, and refuses a file it cannot parse.
- A live check: the same headless auto-mode session is refused a write through `records/` without the setting and makes it with the setting.
- CLAUDE.md, SKILL.md and the design page describe the step.

## Design pages

> Where is the detail?

- [Records store](../design/records-store.md): the Claude Code constraint and the setup step.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live A/B in one worktree made from main, headless claude -p in auto mode,
  Write to records/docs/: without the setting it was refused ('resolves
  through a symlink … outside the allowed working directories'); with the
  store in additionalDirectories it wrote. The store was clean after each run.

- A session started with claude -w is refused even with the setting ('This
  path is in a different worktree'): its isolation blocks every other git
  worktree, and the store is one. Sprint 56 should weigh this.

- Claude Code applies the setting to a running session: when a child session's
  setup wrote this worktree's settings.local.json, this session got the store
  as an additional working directory without a restart.

- Full suite: 443 passed, 35 skipped.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm setup` lists the store in Claude Code's `permissions.additionalDirectories` for the worktree, so a Claude Code session writes records through `records/` without asking, except one started with `claude -w`.

Merged as ad22ce4 (PR #58).

- `setup_claude` in `pm.py`: adds the store to the worktree's `.claude/settings.local.json`, keeps its other settings, refuses a file it cannot parse, and does nothing without a Claude Code config dir.
- `.gitignore` ignores `.claude/settings.local.json`.
- CLAUDE.md, SKILL.md and the records-store design page describe the step and the `claude -w` limit.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Tests: met. `test_setup_adds_the_store_to_claude_codes_additional_directories` (adds, keeps other settings, re-run changes nothing), `test_setup_without_claude_config_dir_leaves_claude_code_alone`, `test_setup_refuses_claude_settings_it_cannot_read` (not JSON, not a list). Full suite: 443 passed, 35 skipped.
- Live check: met. In one worktree made from main, headless `claude -p` in auto mode was refused a Write to `records/docs/` without the setting ("resolves through a symlink … outside the allowed working directories") and wrote it with the setting. A `claude -w` session stays refused either way (see Findings).
- Docs: met; see the [Records store](../design/records-store.md) design page.
