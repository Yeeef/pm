---
type: sprint
title: Agents change code only in their own worktree
bead: yeeef-agents-9va.84
---

## Goal

> What should be true when this sprint ends, and why now?

Every agent session that changes code in a pm repo does it in its own git worktree, never in the main checkout or another session's worktree, so that sessions never mix edits, branches or stashes. Why now: on 2026-10-08 a session working on sprint 57 cut its branch and let its subagent edit inside the main checkout, which already held another party's staged `claude-settings.json` change; moving the work out took a shared-stash round-trip that could have carried the other change with it. Nothing in `AGENTS.global.md`, pm's rules or the session-start hook says to use a worktree, and a background session is told to "work in place".

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The rule: where it lives (`AGENTS.global.md`, pm's rules, or both), its wording, and when it applies (any code change; not read-only work or records writes).
- Enforcement where cheap: for example, session start or `pm task claim` warns or refuses when the session is in the main checkout, or a pm command that makes a worktree for a task (branch from origin/main, `records/` link, `pm init`).
- Subagents and forks: they work in their parent's worktree, never in the main checkout.
- Tests for whatever pm enforces; help and rules text.

**Out:**
- Cleaning up stale worktrees (sprint 55, `pm clean`).
- Changing Claude Code's or Codex's own worktree features.

## Done when

> What evidence will show the goal is met?

- A new session in the main checkout that claims a code task is told to work in a worktree, or refused, and is shown how; a test shows it.
- The rule is in the agent-facing text that session start injects; quote the line and its location.
- An agent following the rule from a fresh session ends up in a worktree with `records/` linked and the main checkout's `git status` unchanged; show the commands and outputs.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
Background sessions in this repo start in their own worktree: remove "worktree": {"bgIsolation": "none"} from .claude/settings.json.
Sessions then start isolated rather than being refused by the main-checkout guards after the fact; read-only sessions get a worktree too, which pm clean (sprint 55) will tidy.
Answers `yeeef-agents-9va.84.2`.
:::

::: decision {source=owner date=2026-10-08}
Keep worktree.bgIsolation "none" in this repo; agents stay out of the main checkout through the pm task claim refusal and the rule text, which replaces the "worktree" choice on yeeef-agents-9va.84.2.
A fresh background session under "worktree" could not Edit or Write records/ after EnterWorktree, and a records Read stalled it on a prompt for over 10 minutes; hand edits of records are core to pm.
Answers `yeeef-agents-9va.84.7`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- PR #77 enforces the rule at four points: pm task claim, the agent-only git
  pre-commit hook, a post-checkout warning, and a Claude Code PreToolUse edit
  hook. Escape hatch PM_ALLOW_MAIN_CHECKOUT=1. make test: 104 passed, 35
  skipped. A scratch-clone demo ended in a worktree with records/ linked and
  the main checkout's git status unchanged. Gaps: shell edits are caught only
  at commit; Codex edits only by pre-commit.

- The owner judged the four-guard version over-complex. Claude Code's default
  bgIsolation "worktree" already blocks Edit and Write in the main checkout,
  which duplicated the PreToolUse hook. The slim version keeps the claim
  refusal, bgIsolation "worktree" and the rule text: 7 files, +45/-11. make
  test: 101 passed, 35 skipped.

- bgIsolation "none" was set on purpose in sprint 12 (commit 527c72f,
  2026-10-04): worktree isolation blocked background sessions from editing the
  records store, and additionalDirectories did not lift it. PR #77 sets
  "worktree" again. A record edit from a background session under "worktree"
  is not yet tested against the current store layout (.pm/store/records via
  records/).

- Tested on Claude Code 2.1.295 in a scratch repo laid out like this one, with
  a fresh claude --bg session and bgIsolation "worktree". After EnterWorktree,
  Edit and Write to records/ and to the store's absolute path are refused:
  "This session is isolated in the worktree …; its writes are limited to that
  folder". additionalDirectories does not lift it. Reading a store file raised
  a permission prompt that stalled the unattended session for over 10 minutes.
  On disk: the store kept only the edit made before EnterWorktree; the
  worktree's own code edit landed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: PR #77 refuses `pm task claim` in the main checkout and adds the own-worktree rule to the text that session start injects.

Merged as 47b1fd1 (PR #77).

- `pm task claim` refuses in the main checkout and in the store. It prints the `git worktree add` command to run instead.
- `bgIsolation` stays `"none"`. A test showed that `"worktree"` blocks record edits from background sessions (see Findings).
- The edit hook, commit hooks, escape hatch and exclude line of the first version were dropped as over-complex.
- Not covered: edits and Bash in the main checkout are not blocked. Only the claim refusal and the rule text guard it.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met.** A claim from the main checkout is refused with the how-to. Evidence: `test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_worktree` in `pm/tests/test_pm.py`. `make test` after merging main: 106 passed, 136 skipped.
- **Met.** `pm/src/pm/prime.md`, Invariants, which `pm prime` injects at SessionStart and SubagentStart: "**Change code only in a worktree of your own**, never in the main checkout or another session's worktree, so sessions never mix edits, branches or stashes." `AGENTS.global.md` §4 has the same rule.
- **Met, in a scratch clone.** In the main checkout, `pm task claim` gives `error: not claiming demo.1.2 here: …`. Then `git worktree add -b demo-task .claude/worktrees/demo origin/main` gives `linked …/worktrees/demo/records -> …/.pm/store/records`. The claim and commit succeed in the worktree. Main's `git status` is `## main...origin/main` before and after, with no stash.
