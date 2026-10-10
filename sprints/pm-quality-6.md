---
type: sprint
title: pm clean removes agent worktrees nobody needs
bead: pm-d2k5.6
---

## Goal

> What should be true when this sprint ends, and why now?

The owner or an agent runs `pm clean` and every agent worktree that no live session owns and whose work is saved is removed, while anything live, dirty or unsaved is kept with the reason printed.

Moved from pm-harness sprint 55 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a `pm clean` command, a dry run by default and `--apply` to act. It finds owners from Claude Code's worktree lock (pid plus process start time) and from transcript activity in the worktree's `~/.claude/projects/` dir within `LIVE_WINDOW`. It removes a worktree only when it is clean and its branch is on `origin/main` (ancestor, or a squash merge checked with `gh`, kept when `gh` is unavailable) or has nothing beyond its pushed upstream. It deletes local branches only once merged, and uses `git worktree remove` without `--force`, then prunes. Tests in a temp repo cover each case.

**Out:** flagging leftovers from the scheduled `pm push` job or in `pm show`; Codex worktrees; reading macOS lock start times (a live pid counts as owning the worktree there).

## Done when

> What evidence will show the goal is met?

- `pm clean` lists each worktree with keep or remove and a reason; `--apply` removes exactly the remove set and never the main checkout, the records store (`.pm/store/records`) or the calling worktree.
- Harness tests cover dirty, unpushed commit, merged, squash-merged, live lock, stale lock and the store, and `make test` passes.
- A run on this clone removes the merged leftovers and keeps any worktree with a commit never pushed.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
`pm clean` is built in Go pm in Yeeef/pm (internal/cli), not in the Python harness.
Carried from pm-harness sprint 55 (agent, 2026-10-10): pm-harness moved to Yeeef/pm on 2026-10-10, and Go pm is its only implementation; the Python pm is being retired.
:::

::: decision {source=owner date=2026-10-10}
This sprint is pm-harness sprint 55, moved to pm-quality; its Done when names the Go records store (.pm/store/records) instead of the retired .records store, and drops the 2026-10-06 worktree names.
The owner asked in chat on 2026-10-10 to move it to pm-quality; the .records store and those worktrees no longer exist.
:::

::: decision {source=agent date=2026-10-10}
pm clean counts a worktree as used by a live session when a Claude Code transcript entry from the last 30 minutes has its cwd in the worktree or a tool call naming its path; it reads no per-worktree transcript dir, and only agent worktrees under .claude/worktrees/ are ever removed.
Subagents run in their parent session cwd and hold no worktree lock, so a per-worktree transcript dir misses them (7 live worktrees read as removable on this clone); tool outputs do not count, so listing worktrees uses none; limiting removal to .claude/worktrees/ leaves hand-made worktrees alone.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- --text=From pm-harness sprint 55: Claude Code locks each agent worktree with
  `claude agent <name> (pid N start T)` in `.git/worktrees/<name>/locked`, so
  a lock whose pid is dead or whose start time differs is stale.

- --text=From pm-harness sprint 55: one `claude rc` process (pid 3711007) held
  the locks on three bridge worktrees, so a live pid cannot tell bridge
  sessions apart; recent transcript writes in the worktree's
  `~/.claude/projects/` dir can.

- --text=From pm-harness sprint 55: on 2026-10-06 the clone had 8 agent
  worktrees: 4 locked by live processes, 2 clean with nothing beyond main
  (removable), and one with 1 commit never pushed, which must be kept.

- --text=On 2026-10-10 the clone had 11 agent worktrees, none locked, and
  their sprint agents' transcripts record cwd = the main checkout (a subagent
  runs in its parent's directory), so neither Claude Code's lock nor a
  transcript dir per worktree saw them. Without another signal, pm clean's dry
  run marked 7 of 13 worktrees remove while their agents were working.
  Counting a live transcript entry (last 30 min) whose tool call names the
  worktree's path keeps all 11: 0 of 13 to remove.

- --text=git worktree add -b X .claude/worktrees/X origin/main (the form
  agents use) sets X's upstream to origin/main, so 'nothing beyond its
  upstream' is no proof of a push. pm clean takes the upstream only when it is
  not the main branch, else <remote>/<branch>; the harness case 'plain'
  (pushed without -u) holds it.

- --text=A fresh-context review reproduced two data-loss cases in the first
  cut, both fixed in a2a4f7d with a harness test that fails on the old code.
  (1) git worktree remove deletes a worktree nested in the removed one: a
  dirty inner worktree was lost. (2) status.showUntrackedFiles=no hid an
  untracked file from the clean check, and the file was then deleted. The
  global git worktree prune also dropped entries judged keep; pm clean now
  removes a vanished worktree by its own entry. Not fixed, and left open:
  Codex sessions in .claude/worktrees are not seen as owners (Codex is out of
  scope), and a lock held from another PID namespace reads as stale
  (unverified).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: `pm clean` shipped in PR #19, merged as 0f65958, with its harness tests; the run that removes this clone's merged leftovers waits until the sprint agents now working here are idle.

- `pm clean` (dry run) and `pm clean --apply`: each worktree listed with keep or remove and the reason. Only agent worktrees under `.claude/worktrees/` are ever removed.
- Owners: Claude Code's worktree lock (pid plus `/proc` start time), and a live session's transcript entry from the last 30 minutes, matched by its cwd or by a tool call naming the worktree's path.
- Saved: clean, and on `origin/main` (ancestor, or a squash merge that `gh` confirms), or pushed with nothing beyond. A branch is deleted only once merged.
- Never deletes what a removal would take with it: a worktree that holds another worktree is kept, and untracked files are listed whatever `status.showUntrackedFiles` says. Both cases came from a review reproduction and are now harness cases.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Status | Evidence |
|---|---|---|
| `pm clean` lists each worktree with keep or remove and a reason; `--apply` removes exactly the remove set and never the main checkout, the records store or the calling worktree | met | `tests/test_clean.py`: the dry run's lines match the expected verdicts exactly and change nothing. `--apply` removes exactly the 4 listed worktrees and keeps the main checkout, the store (still clean) and the caller |
| Harness tests cover dirty, unpushed commit, merged, squash-merged, live lock, stale lock and the store, and `make test` passes | met | 5 integration tests in `tests/test_clean.py`, all passing. Besides those cases they cover gh unavailable, pushed with and without `-u`, a foreign lock, a reused pid, live transcript use, a nested worktree, hidden untracked files, a PR merged into another base and vanished directories. `make test`: 112 passed, 40 skipped. CI on PR #19 |
| A run on this clone removes the merged leftovers and keeps any worktree with a commit never pushed | not met yet | Dry run of the built binary on this clone on 2026-10-10: 0 of 13 to remove; after the rebase on 6a0bb49, 1 of 13 (ci-fast-release-probe: pushed, unused for over 30 minutes). All 11 agent worktrees were kept as used by live session ffa2f713, whose sprint agents were working in them. With `CLAUDE_CONFIG_DIR` pointed at an empty dir: 7 of 13 to remove (4 merged, 3 pushed), and kept were the worktrees with uncommitted changes and those with commits never pushed. `--apply` on this clone waits until those agents are idle (30 minutes after their last use) and PR #19 is merged (merged as 0f65958) |
