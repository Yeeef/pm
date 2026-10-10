---
type: sprint
title: Harness rules win over generic defaults
bead: yeeef-agents-9va.30
---

## Goal

> What should be true when this sprint ends, and why now?

Agents follow the harness's own rules where generic defaults (Claude Code's system prompt, the bd-managed Beads block in CLAUDE.md, `bd prime`) say otherwise, and never hand the owner a step without checking it is needed. On 2026-10-05, closing sprint 22, the agent told the owner to push the `records` branch "because the records commits are local only". It had not checked: the branch was already on `origin/records`. It cited a "don't push without authority" policy from the Beads Conservative profile and Claude Code's default, which conflict with RULES.md (pm commits every write on `records`; CLAUDE.md says to push it on its own).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a study of this pattern: every place where generic guidance (system prompt, Beads managed block, `bd prime`, skill text) conflicts with the harness rules, which one agents follow in practice (transcripts), and earlier cases of handing the owner a step that was unneeded or the agent's own job. Then fix the gaps, for example an explicit precedence rule in RULES.md, a records-push rule, `pm` pushing records itself, or narrowing the Beads block.

**Out:** changing Claude Code's or bd's own defaults upstream; code-branch push policy beyond what the study shows is needed.

## Done when

> What evidence will show the goal is met?

- A study doc lists the conflicts between generic guidance and the harness rules, with transcript evidence of which one agents followed, and earlier cases of the pattern.
- Each conflict the study finds has a resolution the owner chose, written where every agent reads it.
- A fresh session closing a sprint pushes, or confirms pushed, the `records` branch without handing it to the owner, checked in a real run.

## Design pages

> Where is the detail?

- [Shared records store](http://localhost:8000/design/records-store.html): records branch pushed by the per-machine scheduled job; pm only commits.
- [Agent git authority and Beads sync](http://localhost:8000/design/agent-sync.html): team-maintainer profile, scheduled push of Beads data and records.
- [Session claims](http://localhost:8000/design/session-claims.html): cross-machine liveness as an open question.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
Sprint 25 covers pushes from one machine only; races between pushes from different machines are out of scope.
The owner set this scope on 2026-10-05 while choosing how Beads data is pushed.
:::

::: decision {source=agent date=2026-10-05}
PR #31 (the team-maintainer profile and task 9va.30.5, pm setup sets it) is delivered by sprint 28 (yeeef-agents-9va.33), not by sprint 25.
It is a complete change of its own; a PR review needs its sprint's draft delivery report, and sprint 25 is far from one.
:::

::: decision {source=agent date=2026-10-05}
Sprint 25's last Done-when item (a real session pushes, or confirms pushed, the records branch without the owner) moves to sprint 31 (yeeef-agents-9va.36), whose last item checks the launchd job pushes a fresh records commit.
The job is built and a manual pm push works, but on this Mac it can only run once the clone leaves ~/Desktop; holding sprint 25 open would hold PR #34 with it.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- 2026-10-05: closing sprint 22 the agent told the owner to push the records
  branch ('local only') without checking; git -C .records status was in sync
  with origin/records and 49d9ef4 was already on origin. Asked why, it cited
  the Beads Conservative profile and Claude Code's default, not RULES.md.
  Also: a pm task add hung over 2 minutes with no child process (killed; the
  retry took under 60 s).

- The hung pm task add (state SN, no child, 2:16) was killed without a stack
  trace, so its cause is unproven. Likely: waiting for LOCK_EX on the store
  (pm.py:232) while pm serve held LOCK_SH during a render that runs bd
  (pm.py:1038). Next time, before killing: lsof -p <pid> for the records dir,
  and a py-spy dump of both processes.

- design/records-store.md covers the records push in one sentence ('pushed and
  pulled on its own') and never says who pushes or when; no hook or pm code
  pushes it (.beads/hooks/pre-push only syncs Beads). This session's records
  were 3 commits ahead until pushed by hand (5d12ec8..099426d).

- The Beads profile reaches agents only via bd prime, run by the SessionStart
  hook in .claude/settings.json; SessionStart does not fire for Claude Code
  subagents (no SubagentStart hook here), and CLAUDE.md's Beads block does not
  say which profile is active. So subagents commit/push only when their brief
  says so (unverified in a live subagent; the study should check).

- Cross-machine: nothing pulls Beads or records at session start (bd dolt pull
  / git -C .records pull are manual), and sprint 22's liveness check reads
  local transcripts only, so a session live on another machine looks idle and
  pm task claim takes its claim over. Fine for one machine at a time; two
  concurrent machines need an auto-pull and a liveness signal stored in Beads.

- Beads data push: dolt.auto-push is off by default (cmd/bd/dolt_autopush.go):
  opt-in because concurrent pushes to a git remote race on the remote manifest
  and can leave it referencing chunks never uploaded; when on it debounces 5
  min, times out at 30 s, only warns on failure and is skipped in a sandbox.
  Git refs themselves are safe (compare-and-swap, non-fast-forward rejected);
  the risk is Dolt storage on a plain git remote being updated in several
  non-transactional steps (per the Beads source comment; Dolt code not
  checked). Dolt remote servers (DoltHub, remotesapi) update the manifest
  atomically. Auto-push stays off for now (recommendation accepted, not an
  owner decision); the mechanism waits on need yeeef-agents-9va.30.7.

- bd prime's team-maintainer session-close checklist says 'git add .', which
  would sweep unrelated files (other sessions' work, the owner's edits) into a
  commit; RULES.md commits named paths only.

- Study
  (http://localhost:8000/docs/2026-10-05-generic-defaults-vs-harness.html): 7
  generic-vs-harness conflicts; 2 already decided (records push job,
  team-maintainer), 3 open (precedence rule, profile invisible to subagents,
  team-maintainer 'git add .'/push close protocol), 2 agree. Transcripts: 19
  hand-off phrase hits in 8 of 15 sessions, 3 real hand-offs (0836868b records
  push 2026-10-05; 0a6715ef nested-claude check and git merge 2026-10-04).
  Subagents get no SessionStart output: 0 of 80 subagent transcripts carry a
  hook attachment vs 30 in main ones.

- Scheduled push real check (task 9va.30.8): bin/pm push by hand pushed both
  stores (bd dolt push ok in about 8 s; records 1 commit pushed, store 0 ahead
  and 0 behind). The launchd agent pm setup installed cannot run here: macOS
  privacy protection (TCC) denies a background job access to ~/Desktop, so the
  job exits 126 with Operation not permitted until /bin/sh gets Full Disk
  Access or the clone moves out of ~/Desktop.

- Sprint 25's last Done-when item (a real session confirms records pushed
  without the owner) waits on sprint 31 (yeeef-agents-9va.36), which moves the
  clone out of ~/Desktop so the launchd job can run.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the conflicts between generic defaults and the harness are studied and each has the owner's resolution, and pushing Beads data and records no longer depends on an agent remembering.

- `pm push` pushes Beads data and the records branch (fetch, rebase, push) under a per-clone lock with timeouts, records each outcome, and the site and `pm show` flag a failed or overdue push; `pm setup` installs it as a launchd agent (macOS) or systemd timer / cron (Linux). `pm` itself only commits.
- A `SubagentStart` hook in Claude Code and Codex tells every subagent the active Beads profile.
- The study (docs/2026-10-05-generic-defaults-vs-harness) and design pages records-store and agent-sync describe the result. The team-maintainer profile itself shipped in sprint 28.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Study doc with transcript evidence and earlier cases: met; docs/2026-10-05-generic-defaults-vs-harness: 7 conflicts, 3 real earlier hand-offs, 0 of 80 subagent transcripts carried SessionStart output.
- Each conflict has an owner-chosen resolution where every agent reads it: met; project decisions on needs 9va.30.2, 9va.30.7, 9va.30.8.1, 9va.30.9 and the team-maintainer decision, all in the pm-harness project record; the profile reaches subagents through the new hook (live: a Claude Code subagent and a Codex subagent each quoted "Beads agent profile: team-maintainer …").
- A real session pushes or confirms pushed the records branch without the owner: moved to sprint 31 (sprint decision); the launchd job cannot read ~/Desktop on this Mac. A manual `bin/pm push` pushed both stores (exit 0), and PR #34, merged as f75a3a1, passes 351 tests with `make render` clean.
