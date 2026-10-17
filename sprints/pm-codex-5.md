---
type: sprint
title: pm init turns on Codex's auto mode in the repo's .codex/config.toml
bead: pm-v498.5
---

## Goal

> What should be true when this sprint ends, and why now?

A Codex session in a repo that pm set up runs in Codex's auto mode: commands run in the workspace-write sandbox, approval is on request, and the automatic reviewer decides each request (`approval_policy = "on-request"`, `approvals_reviewer = "auto_review"`, `sandbox_mode = "workspace-write"`, the config form of `--approve-for-me`). Only repos that use pm change, and the user's global `~/.codex/config.toml` is left alone. Now, because the owner wants Codex agents to use the same auto-mode setup as Claude Code's without each user setting it by hand. Sprint 3 found this mode passes 11 of 12 live scenarios.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm init` writes the three keys into the repo's tracked `.codex/config.toml`, as one of pm's managed pieces. `pm doctor` names a piece that differs, `pm upgrade` rewrites it, and `pm uninstall` removes it, keeping any key the user added.
- Check that Codex applies `sandbox_mode` and `approval_policy` from a trusted repo's `.codex/config.toml`, not only `approvals_reviewer`. Codex 0.162.1 is known to parse all three there. If Codex ignores a key there, record it as a finding before building on it.
- `pm doctor` names a host where Codex's bubblewrap sandbox cannot start, since `apply_patch` fails there in this mode (sprint 3's finding, gap 11).
- The README's Codex setup and the pm-in-codex design page give the mode and how a user overrides it.
- The parity live eval runs with the repo's own config, so `--approve-for-me` is not needed there.

**Out:**
- Editing the user's global `~/.codex/config.toml` beyond the `writable_roots` pm writes now (owner decision).
- Changing this machine's AppArmor setup (gap 11 stays, owner decision).
- Trusting pm's hooks for the user (`/hooks` stays the user's step).

## Done when

> What evidence will show the goal is met?

- In a scratch clone that `pm init` set up, a Codex session's effective config shows all three keys from the repo file. Shown by `codex exec` output or its rollout, with the command recorded in Findings.
- `internal/install/golden_test.go` covers the new piece, and `make test` passes. The harness checks `pm doctor`, `pm upgrade` and `pm uninstall` on it.
- `make test-live-parity` passes on a host where bwrap starts, or on this host with only the known apply_patch scenario failing and that failure recorded. No assertion is weakened.
- README and the pm-in-codex page describe the mode, and every design page this sprint changes is listed in Design pages.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
pm enables Codex auto mode (approval_policy on-request, approvals_reviewer auto_review, sandbox_mode workspace-write) in the repo's tracked .codex/config.toml, never in the user's global ~/.codex/config.toml.
Only pm repos change and the setting travels with the repo; codex 0.162.1 parses these keys from a trusted repo's .codex/config.toml (bad value rejected, good values load).
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- codex-cli 0.163.0 applies all three keys from a trusted repo's
  .codex/config.toml, and codex exec does not override them. Command: cd
  <repo> && CODEX_HOME=<scratch home> codex exec 'run echo hi and stop', and
  the session rollout's turn_context. Trusted with file: approval on-request,
  approvals_reviewer auto_review, sandbox workspace-write. Untrusted with
  file: never / user / read-only (whole file ignored, with no warning).
  Trusted with no file: never / user / workspace-write. Trusted with
  sandbox_mode=read-only: read-only, so sandbox_mode is read from the file.
  approvals_reviewer=bogus_reviewer fails the config load with exit 1
  (accepted: user, auto_review, guardian_subagent). Only codex exec and codex
  debug prompt-input were run, not the interactive TUI; the scratch repos had
  a .git marker but no commit.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
