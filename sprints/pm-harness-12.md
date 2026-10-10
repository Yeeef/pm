---
type: sprint
title: Sandboxed sessions can write the records store
bead: yeeef-agents-9va.12
---

## Goal

> What should be true when this sprint ends, and why now?

A sandboxed session, which may write only inside its own worktree, can edit records in `.records` directly and commit them with `pm`, like an unsandboxed one. Today `.records` lies outside the worktree, so such a session cannot frame or close a sprint or edit a design page, and every background job works in a worktree (finding on sprint 9).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** letting sandboxed sessions write `.records` (the sandbox's allowed paths, for each runtime we use); the rules and setup docs that describe it.

**Out:** new `pm` commands for hand-written sections; per-worktree copies of the records; the setup command (sprint 10).

## Done when

> What evidence will show the goal is met?

- From a worktree session whose sandbox otherwise refuses paths outside the worktree, an agent edits a sprint Scope and a design page in `.records` and commits them with `pm commit`; checked for real.
- The setup docs say what each runtime needs.

## Design pages

> Where is the detail?

- [Shared records store](../design/records-store.md): setup and the sandbox writable roots

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-04}
Sandboxed sessions get write access to `.records` (option 1); `pm section set` and per-worktree record copies are not built.
Records stay free-form and are constrained by validation (make render, pm commit), not by the write path; a command per hand-written section would make design pages and docs rigid and cost a full-section rewrite per small edit. The owner chose option 1 and is taking it to another agent.
:::

::: decision {source=agent date=2026-10-04}
Moved yeeef-agents-9va.12.2 to yeeef-agents-9va.10: Task 9va.12.2 moves to sprint 10: the Claude Code switch (worktree.bgIsolation: none) is in PR #8, not merged, a freshly started background session has not yet edited a Scope and a design page in .records, and Codex is not done.
Per-runtime setup belongs with the one-command clone setup, and sprint 12 closes on what it found rather than waiting on a PR review.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Claude Code has no per-path exemption from background-job worktree
  isolation: additionalDirectories does not lift the Edit refusal on .records,
  and git -C on the shared checkout is refused too. The only switch is
  worktree.bgIsolation ("worktree" or "none"; a docs page saying false is
  wrong), and it applies only to newly started background jobs. The
  context-efficiency session set "none" in .claude/settings.json (commit
  30c0d75 on context-efficiency-1); Codex is still open.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Partial: sandboxed sessions will write `.records` directly instead of
through new `pm` commands, and Claude Code's only switch for that was found
and set in PR #8; the real check with a fresh background session and the Codex
side moved to sprint 10.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A worktree session whose sandbox otherwise refuses paths outside the
  worktree edits a sprint Scope and a design page in `.records` and commits
  them with `pm commit`: not met. Partial evidence: after the owner applied
  `worktree.bgIsolation: "none"`, the context-efficiency session left its
  worktree, edited its sprint 1 Scope and Done when in `.records` with the
  normal Edit tool and committed with `bin/pm commit` (store commit
  `4255696`); the same edit was refused before the setting. That session was
  not freshly started under the setting, and no design page was edited.
  Moved to sprint 10 with task `9va.12.2`.
- The setup docs say what each runtime needs: not met. Claude Code has no
  per-path exemption; its only switch is `worktree.bgIsolation` (finding
  above), set in PR #8 (`49c2b13`, not merged). Codex is not done. Moved to
  sprint 10.
