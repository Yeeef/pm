---
type: sprint
title: Codex's auto mode in the live eval, and the release that ships the Codex work
bead: pm-v498.3
---

## Goal

> What should be true when this sprint ends, and why now?

The live parity eval runs Codex the way a careful user does: with `--approve-for-me`, Codex's automatic approval review, the counterpart of Claude Code's auto mode, instead of full access. The Codex work merged in PR #47 then reaches this machine's own pm through a release and a pin move. Now, because the owner pointed out that every live run so far used danger-full-access, and because none of the Codex work is in a release yet.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- What `--approve-for-me` does: sandbox, approval policy, reviewer, escalations, and how it interacts with `pm.rules`.
- The live eval's Codex mode `approve-for-me`, with one full live run on this host and honest per-scenario results.
- AGENTS.md naming the realistic Codex mode, and the pm-in-codex gap 11 entry left by owner decision.
- After the release: `pm service install` from a shell with codex 0.149.0 or later, with `pm doctor` showing no codex line.

**Out:**
- Changing this machine's AppArmor setup (gap 11 is left, owner decision).
- Cutting the release itself, which is its own PR by the release procedure.

## Done when

> What evidence will show the goal is met?

- `PM_PARITY_CODEX_SANDBOX=approve-for-me make test-live-parity` has run on this host, each scenario's result and cause is recorded in Findings, and no assertion was weakened.
- AGENTS.md and the pm-in-codex page say which Codex mode the eval and a real user run.
- Once a release with the Codex work is pinned here, `pm doctor` in the main checkout prints no codex service line.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
Scope gains cutting pm 0.8.0 from main now with the pm-codex work only (PRs #47 and #56); the design-page gate (#54) goes into a later release.
Owner, in chat 2026-10-13: release now rather than wait for #54.
:::

::: decision {source=agent date=2026-10-13}
Moved pm-v498.1.3 to pm-v498.4: Sprint 3 delivers one PR, the Codex auto mode in the live eval (#58); the release follow-up is a separate deliverable.
This task belongs to sprint 4, which ships 0.8.0 into this repo and closes sprint 2.
:::

::: decision {source=agent date=2026-10-13}
Moved pm-v498.1.5 to pm-v498.4: Sprint 3 delivers one PR, the Codex auto mode in the live eval (#58); the release follow-up is a separate deliverable.
This task belongs to sprint 4, which ships 0.8.0 into this repo and closes sprint 2.
:::

::: decision {source=agent date=2026-10-13}
Moved pm-v498.3.2 to pm-v498.4: Sprint 3 delivers one PR, the Codex auto mode in the live eval (#58); the release follow-up is a separate deliverable.
This task belongs to sprint 4, which ships 0.8.0 into this repo and closes sprint 2.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Codex's auto mode is --approve-for-me (codex 0.154):
  approvals_reviewer=auto_review, approval_policy=on-request,
  sandbox_mode=workspace-write; the reviewer is a separate read-only thread
  (codex-auto-review on a ChatGPT login) with a 90 s deadline, and an allow
  rule covering every segment, such as pm.rules for a plain pm command, skips
  both the sandbox and the reviewer. The live set in this mode on this host
  (bwrap blocked by AppArmor): 11 passed, 1 failed in 112.68 s. The failure is
  test_stop_hook_blocks_uncommitted_records[codex]: apply_patch reads through
  bwrap, which cannot start, even when escalated, so the edit never happened.
  The reviewer made 10 decisions, all allowed, 1.1 to 4.1 s each (median 1.7
  s), about 8.9k input tokens each. No assertion was changed.

- pm 0.8.0 released: notes PR #59 merged as c43ebbb directly on the Codex
  merge 1cd0fa4, tagged pm-v0.8.0; the release workflow succeeded with
  install.sh, darwin-arm64 and linux-amd64 tarballs and SHA256SUMS. The
  design-page gate (#54) merged after the tag, as 31e2730, so it is not in
  0.8.0 (owner: Codex work only).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
