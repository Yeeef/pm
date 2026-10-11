---
type: sprint
title: Codex integration for pm
bead: pm-v498.1
---

## Goal

> What should be true when this sprint ends, and why now?

An agent running in Codex works on the pm harness with the same guarantees as one in Claude Code: it gets the session context, waits for owner replies, cannot leave records uncommitted or owner requests only in chat, and commits records from inside Codex's sandbox.

Moved from pm-harness sprint 34 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a gap list of pm behaviour in Codex against Claude Code; closing the gaps in hooks, setup and docs; a live check from a real Codex session.
**Out:** packaging pm for other repos (the bundled-product sprint), beyond keeping the Codex pieces inside that layer.

## Done when

> What evidence will show the goal is met?

- A gap list of Codex against Claude Code is written, each gap closed or recorded as a decision to leave it.
- A real Codex session raises a need, is woken by the owner's reply, and commits a record, with the transcript as evidence.
- The setup docs say what a Codex user runs, and nothing else is needed.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-11}
Done when gains a parity check: one scenario run by a real Claude Code session and a real Codex session in a scratch clone must give the same pm outcomes, plus a CI test that drives every installed hook with each runtime payload and holds the outcomes equal; and a mixed run where a Claude session and a Codex session work the same project together.
Owner asked in chat (2026-10-11) for a success criterion that a test checks: both runtimes work on par, and a user can have Codex and Claude working together on one project.
:::

::: decision {source=agent date=2026-10-11}
pm init writes a project Codex rule .codex/rules/pm.rules allowing the pm prefix, so pm commands run outside the Codex sandbox, as pm hooks already do; pm sets no network_access and no unix-socket allowlist.
codex-rs 0.154 source: a network-off Linux sandbox denies connect() by seccomp, so no pm command reaches work.sock; an execpolicy allow rule runs the command unsandboxed (exec_policy.rs:440, orchestrator.rs:269), project rules load once the project is trusted, and it opens nothing beyond pm itself.
:::

::: decision {source=agent date=2026-10-11}
A Codex session waits for an owner reply with a runtime-neutral pm reply wait <need>, and undelivered replies reach a Codex session at its next prompt (UserPromptSubmit) or stop (Stop block); pm does not run codex exec resume or an app-server for delivery.
Codex has no messaging socket; a live TUI runs an embedded app-server nobody outside can reach, and exec resume fails while the thread has a writer; a blocking command works in every runtime, and Codex polls background commands up to 300 s per poll.
:::

::: decision {source=agent date=2026-10-11}
pm hook inbox runs at UserPromptSubmit only, not at Stop; mid-turn replies reach a Claude session by push and a Codex session through pm reply wait or its next prompt.
Review: an inbox block at Stop makes the continuation end with stop_hook_active, so pm hook stop and the owner-request check let exactly the turn that acts on the reply through unchecked. Supersedes the Stop half of the delivery decision.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Gap list, Codex 0.154 vs Claude Code 2.1.296, from code and live runs
  (payloads captured both sides). Works as-is: output envelopes, Stop payload
  keys, SessionStart session_id, context chunks up to 9,926 chars injected
  whole, a Stop block makes Codex continue. Breaks a guarantee: (1) Codex runs
  no project hook until the user trusts it; codex exec skips untrusted hooks
  silently, and 0 of the owner's 43 Codex sessions in yeeef-agents got pm
  context; (2) a Codex subagent is its own thread, so its claims and needs
  carry another id than its root; (3) a Codex run started from Claude inherits
  CLAUDE_CODE_SESSION_ID, which pm reads first; (4) no reply push: Codex has
  no messaging socket; (8) the judge needs claude, and a Stop exit 1 is a
  silent fail-open in Codex; (10) pm clean does not see live Codex sessions.
  Degraded: (5) Codex reads hooks from the main checkout, not the worktree;
  (6) the SessionStart matcher lacks compact; (7) subagent tool inputs are
  encrypted in the parent rollout, so the parent's stop hook misses a
  subagent's records; (9) the AskUserQuestion rule has no Codex tool.
  Unverified: unix socket and records commits under the Codex sandbox (bwrap
  fails on this host: RTM_NEWADDR).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
