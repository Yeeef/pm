---
type: sprint
title: Ship pm as an installable product on top of Beads
bead: yeeef-agents-9va.38
---

## Goal

> What should be true when this sprint ends, and why now?

The owner holds an actionable design for shipping pm as a bundled product that another repo installs with one command, with all of pm's agent context (rules, skill, hooks, session context, its block beside Beads' block in AGENTS.md) managed as one structured layer on top of bd.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** an inventory of every place pm's behaviour and context lives today and its scope (machine, repo, clone, worktree); brainstorm with the owner on packaging, install, upgrade and how pm's context sits on top of bd's; a design page holding the result; implementation tasks filed from it.
**Out:** implementing the installer or the restructure (later sprints); Codex integration (its own sprint).

## Done when

> What evidence will show the goal is met?

- A design page covers packaging, the one-line install, upgrade and uninstall, and the context layer on top of bd, with alternatives considered and open questions answered or raised as needs.
- The owner has reviewed the design page and its open decisions are recorded.
- Implementation work is filed as tasks in one or more follow-up sprints, each traceable to a section of the design page.

## Design pages

> Where is the detail?

- [pm as an installable product](../design/pm-product.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
Moved yeeef-agents-9va.38.19 to yeeef-agents-9va.56: The owner-request hook decision moves to sprint 49, which the owner opened to change the hook.
Sprint 33 keeps only the pm product design; another agent runs sprint 49.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm's context lives in about 20 surfaces across 5 owners (pm, bd, Claude
  Code, Codex, git) and 4 scopes (machine, repo, clone, worktree); the
  repo-level ones (.claude/settings.json hooks, .codex/hooks.json, the
  CLAUDE.md RULES import, git hook tails, two GitHub workflows, Makefile
  targets, .gitignore lines) are hand-copied today, with no installer.

- bd owns its own regions by markers: the AGENTS.md block between BEGIN/END
  BEADS INTEGRATION (versioned, profile tag, content hash) and a block in each
  .beads/hooks git hook; pm appends its git hook code after bd's END marker,
  so a bd hook reinstall can drop it.

- Not portable as is: every hook and bin/pm hardcode
  skills/project-management/harness/; .beads/config.yaml carries this repo's
  remote and DB name; origin, main and GitHub (gh, Actions) are hardcoded;
  setup requires an existing records branch and refs/dolt/data, so a brand-new
  repo cannot bootstrap; agent messages name make docs and make render, which
  another repo lacks.

- pm needs uv, bd, python3, git and a scheduler (systemd, launchd or cron) on
  the machine; its own code is one PEP 723 script (pm.py, 2273 lines, deps
  markdown-it-py, mdit-py-plugins, pyyaml) plus stdlib hook scripts. Personal
  dotfiles (claude-settings.json, AGENTS.global.md, make setup-agent) are not
  part of pm.

- bd splits context by channel: a ~10-line marked pointer in AGENTS.md
  (template profile + version + hash, so bd setup --check/--remove touch only
  it) plus bd prime from SessionStart. Stated reasons (bd onboard, issue
  #2696, docs/getting-started/ide-setup.md:97): full text in AGENTS.md wasted
  tokens and went stale on upgrade; SessionStart re-fires after compaction and
  clear; bd prime is state-dependent (MCP vs CLI, agent.profile, memories).
  Hookless hosts get the full template. Measured: bd setup --print ~570
  tokens, bd prime ~1.7k.

- Guidance audit (127 instructions in RULES.md, SKILL.md, references/,
  status-site/; 45.5 KB, ~11.4k tokens): 24 enforced by code or hooks, 14
  already in pm <noun> --help, 25 duplicated in RULES.md, 8 obsolete or wrong,
  56 judgment-only. The judgment rules dedupe to about 30 items; a minimum set
  is about 6.7 KB (~1.7k tokens): ~600 tokens of rules printed each session,
  the rest (planning, owner writing, design pages) on demand. Two code gaps
  would retire two more rules: headers accept unknown keys (records.py:128),
  and any comment on an open request counts as a reply (beads.py:159).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the owner approved an actionable design for shipping pm as an installed, pinned product with hook-only context, and its implementation is filed as sprints 44 to 46.

- [pm as an installable product](../design/pm-product.md): a uv package in `pm/` of this repo; one idempotent `pm init`; `.pm/` for machinery with records kept at `records/`; hook-only context from `pm prime` (about 900 tokens of rules, no skill, no `pm guide`); a per-repo version pin that fails hard; pm's git hook code as a marked section in Beads' hooks; one supervised pm service for the site and the push.
- Eleven owner decisions recorded in the project (needs 9va.38.5 to .38.13 and .38.16 to .38.18), plus the merged owner-writing rules with ASD-STE100.
- Side work raised from this sprint: sprint 40 (reply delivery), sprint 49 (owner-request hook reads Beads), sprint 51 (fixed layout for decision needs).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: the design page covers packaging, the one-line install, upgrade and uninstall, and the context layer beside Beads, with alternatives considered; its open questions were raised as needs and answered; only Codex's after-compaction context is left, to sprint 34.
- Met: the owner reviewed the page and approved it on 2026-10-07 (action 9va.38.15); a fresh-context agent reviewed it once before, and its findings were fixed.
- Met: implementation is filed as sprints 44 (9va.50), 45 (9va.51) and 46 (9va.52), 15 open tasks, each naming the design section it comes from; 45 depends on 44, and 46 on 45.
