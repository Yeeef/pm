---
type: design
title: Agent git authority and Beads sync
project: pm-harness
---

## Problem

> What are we solving, and why now?

Part of the [Project management harness](pm-harness.md) design. Generic guidance reaches agents alongside the harness rules: Beads' managed CLAUDE.md block and `bd prime` select an agent profile that decides whether agents may commit, push and sync, and Beads' Dolt data needs pushing to the remote. The default profile forbade what the harness needs (committing and pushing records), and nothing pushed Beads data or records automatically, so agents handed those steps to the owner or skipped them. How records are pushed is in [Shared records store](records-store.md).

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals:**

- Agents commit and push as routine work wherever the harness needs it, without asking the owner.
- Beads data and records reach the remote without any session having to remember it, and a failed or overdue push is visible.

**Non-goals:**

- Changing Beads' or Claude Code's defaults upstream.
- Races between pushes from different machines (sprint 25 scope).

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- Beads' block defines three profiles: conservative (default; forbids every git commit, git push and Dolt sync repo-wide, not only Beads data), minimal, and team-maintainer (agents commit, push and sync at session close; an explicit no-commit or no-push instruction still wins).
- `bd prime`, run by the SessionStart hook, carries the profile's rules to main sessions only: neither Claude Code nor Codex passes SessionStart output to a subagent (checked live with a Codex subagent spawned without history, 2026-10-05).
- Codex runs a project hook only once the project is trusted and the hook's hash is trusted in `$CODEX_HOME/config.toml`, and a worktree nested in the main checkout runs the main checkout's `.codex/hooks.json`.
- `bd prime`'s team-maintainer checklist says `git add .`, which would sweep unrelated files; the harness commits named paths only.
- `dolt.auto-push` is off by default in Beads (`cmd/bd/dolt_autopush.go`): concurrent pushes to a git remote race on the remote manifest and can leave it referencing chunks never uploaded. When on, it debounces 5 minutes, times out at 30 s, only warns on failure and is skipped in a sandbox.
- Git ref updates are compare-and-swap, so git itself is safe; the risk is Dolt storage on a plain git remote, updated in several non-transactional steps (per the Beads source comment; Dolt's code not checked). Dolt remote servers (DoltHub, remotesapi) update the manifest atomically.
- Nothing pulls Beads or records at session start; a fresh clone with `bin/pm setup` works for one machine at a time.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Agent profile

The repo runs the team-maintainer profile: `agent.profile: team-maintainer` in `.beads/config.yaml`, set by `pm setup` and shown by `pm where` (PR #31). Agents commit, push and sync as routine work; an explicit no-commit or no-push instruction still wins. Commits still name their paths, never `git add .`.

### Subagent profile line

A SubagentStart hook in both runtimes (`.claude/settings.json`, `.codex/hooks.json`) injects one line into every subagent, read live from `bd config get agent.profile`: `Beads agent profile: team-maintainer (commit and push are routine unless your brief says otherwise).` Nothing else is injected; the brief stays the subagent's instructions (owner decision on need yeeef-agents-9va.30.9, built in task yeeef-agents-9va.30.11). It is `harness/session_context_hook.py` in its SubagentStart mode; when bd fails the line reads `Beads agent profile: unknown (bd config failed: …)`, and the subagent still starts.

### Scheduled push of Beads data and records

Sessions never push Beads data or records: `dolt.auto-push` stays off and `pm` only commits. Both are pushed by one scheduled job per machine, built in task yeeef-agents-9va.30.8 (owner decisions: Beads data on need yeeef-agents-9va.30.7, records on need yeeef-agents-9va.30.8.1). The job is `bin/pm push`, run from the main checkout every 10 minutes by a launchd user agent (macOS), a systemd user timer, or cron where systemd has no user instance; `pm setup` installs it once per clone (label `local.pm-push.<dir>.<hash of the clone path>`) and `pm where` shows it with each store's last attempt. A run takes a non-blocking lock (`.git/pm-push.lock`), so a second run exits at once; each network step has a 120 s timeout. It records each store's last attempt in `.git/pm-push.json` and appends to `.git/pm-push.log`; `pm show` and the served home and project pages flag a failed push, or one with no success for 30 minutes (three intervals). `bd dolt push` in embedded mode (bd 1.3.1) neither waited on nor failed against another bd process during the check: two concurrent pushes both succeeded in about 10 s. On macOS, a launchd job cannot read a clone under `~/Desktop`, `~/Documents` or `~/Downloads` (privacy protection) unless `/bin/sh` has Full Disk Access; it then exits 126 and shows as overdue. For records, when the store is ahead of `origin/records`, it fetches, rebases onto `origin/records` and pushes; a rebase conflict or failed push is logged and flagged. A single job per machine keeps local pushes from racing. A failed or overdue push of either store shows in `pm show` and on the site. Accepted cost: up to one interval before a record reaches `origin/records`, so a PR merged in that window copies records into `main` without the latest ones.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| Conservative profile (default) | Forbids the records commits and pushes the harness relies on |
| `dolt.auto-push` in sessions | Concurrent sessions race on the remote manifest; failures only warn |
| A SessionEnd hook push | A hook runs per session, so concurrent sessions race on the remote manifest, and a session that crashes never pushes |
| `pm` pushes records after every write (earlier decision on need yeeef-agents-9va.30.2) | The owner kept `pm` light and local; the job already pushes Beads data |
| Both the hook and the job | Keeps the hook's race for little gain; the job already bounds the delay to about 10 minutes |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

None yet.

## Open questions

> What is still unresolved?

- Two machines at once: nothing pulls at session start, and cross-machine push races are out of scope.
