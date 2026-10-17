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

- [pm in Codex](../design/pm-in-codex.md)
- [pm as an installable product](../design/pm-product.md)

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
pm enables Codex auto mode (approval_policy on-request, approvals_reviewer auto_review, sandbox_mode workspace-write) in the repo's tracked .codex/config.toml, never in the user's global ~/.codex/config.toml.
Only pm repos change and the setting travels with the repo; codex 0.162.1 parses these keys from a trusted repo's .codex/config.toml (bad value rejected, good values load).
:::

::: decision {source=agent date=2026-10-17}
A key in the repo .codex/config.toml is pm s only while it holds pm s value; any other value is the user s override, which doctor ignores, upgrade keeps and uninstall leaves.
The repo file overrides the global config, so a different value is the only way a user can override the mode, and an override must survive pm upgrade; this matches how pm keeps user-owned writable_roots and hooks.
:::

::: decision {source=agent date=2026-10-17}
The design page pm-cli stays as it is for this sprint's code changes.
commands.go changes only pm init's and pm doctor's --help text; the command tree and the CLI's design are unchanged
:::

::: decision {source=agent date=2026-10-17}
The parity eval keeps PM_PARITY_CODEX_SANDBOX as an opt-in override of the repo mode; unset, Codex runs on the repo .codex/config.toml.
A host where bwrap cannot start may need danger-full-access to run the eval at all; the default path tests what users run, and an assertion checks the mode when the override is unset.
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

- codex 0.163.0 on Linux runs its codex-linux-sandbox helper, which runs the
  system /usr/bin/bwrap (0.9.0) with --unshare-user --unshare-pid
  --unshare-ipc --unshare-net. On this host (Ubuntu 24.04.1,
  kernel.apparmor_restrict_unprivileged_userns=1, no AppArmor profile for
  bwrap) 'codex sandbox -c sandbox_mode="read-only" -- true' fails with
  'bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted' in about 46
  ms; pm doctor now runs this probe and exits 1. The suggested unconfined
  bwrap profile passes apparmor_parser -Q -K; it was not loaded here, so it is
  not confirmed that it fixes the probe.

- Live check in a scratch clone (harness agents.Clone: scratch bare origin,
  HOME and CODEX_HOME under a temp dir, only auth.json copied), set up by pm
  init from the branch build and trusted in CODEX_HOME: 'codex exec "run echo
  hi and stop"' (codex 0.163.0) printed 'approval: on-request' and 'sandbox:
  workspace-write [workdir, /tmp, $TMPDIR]', and the rollout turn_context read
  approval_policy on-request, approvals_reviewer auto_review, sandbox_policy
  workspace-write, all from the repo's .codex/config.toml.

- make test-live-parity with Codex in the repo's own auto mode (no
  --approve-for-me or other mode flag): 15 passed in 102.6 s. A new assertion
  checked that every Codex turn's turn_context was on-request / auto_review /
  workspace-write. The expected gap-11 apply_patch failure did not happen with
  codex 0.163.0, although 'codex sandbox -- true' still fails here with the
  bwrap loopback error. Across the Codex sessions: 41 escalated requests, none
  denied, and one bwrap failure in each of 4 sessions. In the Stop scenario,
  apply_patch on the records/ link path failed with 'Not a directory (os error
  20)', and a retry on .pm/store/records/... succeeded.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a repo that pm sets up runs Codex in auto mode from its tracked `.codex/config.toml`, and `pm doctor` names a host where Codex's sandbox cannot start.

Merged as 2a59ef7 (PR #111).

- `pm init` and `pm upgrade` write pm's block: `approval_policy = "on-request"`, `approvals_reviewer = "auto_review"`, `sandbox_mode = "workspace-write"`. `pm doctor` names a missing key. A key with another value is the user's override, which is kept. `pm uninstall` removes only pm's block, so the user's file comes back byte for byte.
- `pm doctor` runs `codex sandbox -- true` on Linux and names the AppArmor profile or sysctl fix when bwrap cannot start.
- The parity live eval runs Codex with the repo's own config and asserts the mode on every Codex turn.
- PR [#111](https://github.com/Yeeef/pm/pull/111).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Effective config in a scratch clone set up by `pm init`: met.** `codex exec` printed `approval: on-request` and `sandbox: workspace-write`, and the rollout `turn_context` read on-request / auto_review / workspace-write (Findings). Controls with an untrusted repo, no file, and a read-only file show the values come from the repo file.
- **Golden test covers the piece, `make test` passes, harness checks doctor/upgrade/uninstall: met.** `internal/install/testdata/pieces.json` has the new inputs. `make test`: 152 passed. Integration `-k 'uninstall or upgrade or doctor or bootstraps'`: 14 passed. CI on #111: all jobs pass.
- **`make test-live-parity`: met.** 15 passed in 102.6 s on this host. No assertion was weakened, and one was added for the mode. The expected apply_patch failure did not reproduce (Findings).
- **README and pm-in-codex page describe the mode; changed design pages listed: met.** pm-in-codex is listed under Design pages and drafted for this sprint.
