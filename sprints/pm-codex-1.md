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
- Every existing design page whose area this sprint changes is brought to the new state, and the sprint's Design pages section lists it.

## Design pages

> Where is the detail?

- [pm in Codex](../design/pm-in-codex.md): what pm installs for Codex, session identity, owner replies, hook trust, the sandbox, and the parity checks.

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

::: decision {source=owner date=2026-10-12}
Done when gains: every existing design page whose area the sprint changed is brought to the new state and listed in Design pages.
Owner, in chat 2026-10-12; the sprint had changed reply delivery, session identity, the owner-request hook and setup without editing their pages.
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

- Live parity eval (make test-live-parity, real claude -p and codex exec in a
  scratch clone, Codex with danger-full-access since bwrap cannot start on
  this host): 5 of 5 passed in 94 s, 224 s and 316 s over three runs; Stop
  hook blocked once per runtime; cross-runtime claims refused both ways naming
  the holder. Under claude -p no service push reached the session, so it
  polled pm reply read itself (solo Claude 57 s to 314 s); Codex used pm reply
  wait (66 s to 75 s).

- The live eval caught a pm defect beyond Codex parity: a typed pm init
  restarted the shared pm service whenever its PATH differed from the unit's,
  and Codex prepends its own codex-path dir to every command's PATH; so a
  Codex agent's pm init restarted the service under every live session, whose
  pm commands failed with does-not-answer meanwhile (one solo-codex run lost
  its owner reply and timed out at 600 s).

- Final live parity eval at the PR head e8e95ff
  (PM_PARITY_CODEX_SANDBOX=danger-full-access make test-live-parity): 7 passed
  in 92.07 s. Claude / Codex: Stop hook 23.0 s / 25.4 s; next prompt 7.7+6.6 s
  / 14.2+12.2 s; solo 54.5 s / 76.4 s; mixed 78.9 s / 87.6 s. No agent typed
  pm init, and no scenario restarted the service.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a Codex session gets the same pm guarantees as a Claude Code session, and both work one project at once, held by a CI parity test and a live eval on real sessions.

- Session identity: `CODEX_SESSION_ID` (root, shared with subagents), with the innermost runtime deciding when both runtimes' ids are set; a Codex session never takes a Claude Code inbox.
- Codex hooks: SessionStart on compact, SubagentStop running `pm hook stop`, and the same commands as Claude Code's per event.
- `.codex/rules/pm.rules` runs pm outside Codex's sandbox.
- Owner replies: a push as soon as the store moves, `pm reply wait`, and `pm hook inbox` at UserPromptSubmit.
- `pm doctor` names the pm hooks Codex skips, read from `codex app-server`.
- A typed `pm init` no longer restarts the service for a PATH-only difference.
- `make test-live-parity`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| A gap list of Codex against Claude Code, each gap closed or recorded as a decision to leave it | met | 12 gaps, listed with each one's status in the [pm in Codex](../design/pm-in-codex.md) design page (Gaps against Claude Code). 7 are closed in PR #47. Hook trust (1) is reported by `pm doctor` and the owner trusts once. Two are partly closed: the reply push (4), where an idle TUI is not woken, and the judge needing `claude` (8), which pm-codex sprint 2 takes on. Two are left by a pm-codex project decision: `.codex/` is read from the main checkout (5), and there is no ask tool (9). |
| A real Codex session raises a need, is woken by the owner's reply, and commits a record, with the transcript as evidence | met | `test_solo_session_gets_pm_guarantees[codex]` in `make test-live-parity`, on real `codex exec` sessions. The session raised a need and waited with `pm reply wait`. The owner replied 30 s later, and the session recorded the decision, which closed the need with `closed_by` set to the session. It committed a doc and a finding and closed its task. Run 6: 67 s, run 7: 88 s, both passing; transcripts are copied into the run's tmp dir. |
| The setup docs say what a Codex user runs, and nothing else is needed | met | The README's "In Codex" section: `pm init`, then trust the project and pm's hooks once in Codex (`/hooks`). `pm doctor` names any pm hook Codex skips. |
| Parity check (owner decision, added 2026-10-11): one scenario run by a real Claude Code session and a real Codex session gives the same pm outcomes; a CI test drives every installed hook with each runtime's payload; a mixed run of both on one project | met | CI: `TestClaudeAndCodexRunTheSameHooks` and the harness tests that feed both runtimes' captured payloads to each hook; PR #47 is green on all 9 checks at e8e95ff. Live: 7 scenarios with identical assertions per runtime: solo, Stop hook, reply at the next prompt, and a mixed run with claims refused across runtimes. 7 of 7 passed three times (92 to 94 s) before f6e3419. At the PR head e8e95ff: 7 passed in 92.07 s. |

Not run live: Codex's sandboxed path. bubblewrap cannot start on this host (`RTM_NEWADDR`), so the live eval ran Codex with `danger-full-access`.
