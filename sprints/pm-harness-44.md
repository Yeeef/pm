---
type: sprint
title: Package pm as an installable tool
bead: yeeef-agents-9va.50
---

## Goal

> What should be true when this sprint ends, and why now?

pm runs as an installed `pm` tool built from the `pm/` package in this repo, with its agent context and hooks served by `pm prime` and `pm hook <name>` and its version pinned per repo, so the installer sprint has something to install. Design: [pm as an installable product](../design/pm-product.md).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the `pm/` package (pyproject, src layout, `pm` entry point) built from today's pm.py, harness package, render.py and hook scripts; `pm prime`, `pm prime --subagent`, `pm hook <name>` for every current hook; `pm guide <topic>` serving RULES, SKILL and reference text as package data; `.pm/config.toml` read for version, remote, main branch and port, failing hard on a version mismatch; hardcoded paths, origin/main and make targets removed from messages.
**Out:** writing pieces into a repo (`pm init`, `upgrade`, `doctor`, `uninstall`) and migrating this repo; Codex parity (sprint 34).

## Done when

> What evidence will show the goal is met?

- `uvx --from "git+…#subdirectory=pm" pm show` works in this repo, and the package's tests pass.
- Every current hook has a `pm prime` or `pm hook` equivalent with the same behaviour, shown by tests.
- A version mismatch against `.pm/config.toml` fails with the fixing command, shown by a test.

## Design pages

> Where is the detail?

- [pm as an installable product](../design/pm-product.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
pm prime's rules are rewritten from SKILL.md's structure: keep its organization, cut what is useless or wrong, and fold in RULES.md and the other guidance so each topic (design pages, decisions and needs) sits in one place; length is not a limit in this pass, and the size choice (9va.50.10.1) waits on the measured draft.
The owner said on 2026-10-07 that SKILL.md is well organized and today's prime text is too simplistic and scattered.
:::

::: decision {source=owner date=2026-10-07}
Version B of the pm prime rewrite (branch prime-rewrite-b) is the base for later revisions, and they are written with Fable at medium effort.
The owner said on 2026-10-07 that they like version B a bit more.
Answers `yeeef-agents-9va.50.10.2`.
:::

::: decision {source=agent date=2026-10-07}
Moved yeeef-agents-9va.50.13 to yeeef-agents-9va.52: It waits on PR #60, which merges into pm-package and so reaches main with sprints 45 and 46.
Sprint 44 is on main and closes now; sprint 46 is the open sprint whose PR carries pm-package to main.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Second guidance audit (prime.md against the old RULES, SKILL, references,
  status-site): 2 key rules lost (claim only with pm task claim; push the
  branch and open the PR without asking), 7 useful, 7 minor; 7 rules dropped
  as enforced are enforced only partly or not at all (record headers, prompt
  lines, fenced-block reading lines, report evidence, need format on the
  branch); prime.md named a pm service that does not exist yet. Fix: 6 lines
  added, the service line removed; 3 owner-writing rules put to the owner
  (9va.50.7).

- Third audit, two independent agents (document-based and situation-based):
  both found the write-records and hand-edit lines contradict each other, a
  wrong 'move the decision' line, and lost rules on delivery-report evidence
  and reading one record section. Only the situation-based audit found that
  the 10,000-character cap cut pm show's owner-request lines (prime output
  9,951 chars: rules 4,194, commands 1,089, pm show 4,636 of 7,079), that
  subagents get no rules, and that the 'Beads comment reads as a reply' line
  is false (beads.py site_replies). Only the document-based audit found 6 lost
  rules (owner answers as decisions, pm action done, stacked base, pm show
  again, bd ready --exclude-type=epic, design sub page links).

- The three guidance audits missed SKILL.md's object table (concept to Beads
  type to record path, records never copy status); the owner found it. Cause:
  every audit brief asked for 'instructions' or 'rules', so descriptive
  knowledge (definitions, maps, paths) was out of scope, and prime.md's
  two-line Model looked like coverage. An audit of guidance must also list the
  knowledge an agent needs, not only the rules.

- Knowledge audits (two agents; one went through SKILL.md section by section,
  one simulated a new agent doing ten assignments): both found that nothing
  tells an agent how owner replies arrive (as new turns; no polling) and that
  the allowed fenced blocks and the rule against decisions on design pages
  were lost. Only the section audit found the shared store, pm decision close
  for small answers, follow-the-reason and goal-as-mechanism; only the
  simulation found that a voided sprint can close without a PR, that a push
  failure has no remedy, and that subagents lose the --help pointer. Fix:
  about 1.5 KB more in prime.md, a 'Reading pm show' table, and 8 --help
  texts.

- Owner review of PR #52 (2026-10-07): 'pm writes records' misstates pm's
  role; pm is the orchestration layer over the work layer (Beads) and the
  record layer (records); fixed in 583429e. The owner also finds pm prime (8.0
  KB, ~2,000 tokens, from 49 KB of SKILL.md, RULES.md and pages) too
  condensed; the size is put back to the owner as 9va.50.10.1.

- pm prime rewritten from SKILL.md's structure with RULES.md folded in
  (prime-rewrite 070653a): 29,787 characters (~7,400 tokens) against 8,079
  before; 10 sections, largest Records 5.8 KB and Decisions and needs 5.6 KB.
  A review against the code found 8 false statements in SKILL.md and RULES.md
  (e.g. 'Decisions await you' listed as a record section, the doc header
  missing date, an invalid record breaking only its own page), fixed in the
  draft. 2 hook cap tests fail (10,000 characters per hook).

- Owner review of pm prime (2026-10-07): the rule 'a sprint closes only when
  its PR is on main; a voided sprint with no PR closes without one' is wrong
  for sprints with no code change (investigation, design page). pm sprint
  close already blocks only on PR reviews under the sprint and closes a
  review-less sprint with no merge stamp, whatever its verdict; the text, from
  RULES.md and SKILL.md, is corrected in prime-rewrite-b.

- Hook cap measured (Claude Code 2.1.292, 2026-10-07, headless claude -p,
  tools forbidden): the 10,000-character limit is per hook, not in total. One
  SessionStart hook of 12,027 characters reached the model only as a 2 KB
  preview plus a file path ('Output too large (11.7KB). Full output saved to:
  …'); three hooks of 8,652 characters each (about 26,000 in all) all arrived
  inline and in full. The three arrived in the order 3, 1, 2: hooks in one
  entry run in parallel, so a split prime must not depend on order (each part
  needs its own heading).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm is an installable uv package at `pm/` with a `pm` command, its session context and Stop hook run as `pm prime` and `pm hook stop`, and it requires `.pm/config.toml` and fails hard on a version mismatch.

Merged as 35ef486 (PR #52).

- `pm/` package (hatchling, 0.1.0, console script `pm`), moved by a renames-only commit so other branches rebase; `bin/pm` runs it with `uv run --project pm`.
- `pm prime` prints all of pm's guidance at every session start, after each compaction and for each subagent: the rules were rewritten from SKILL.md and RULES.md (about 24,900 characters: Part 1 what — layers, objects, interfaces, working with them; Part 2 how — reading state, projects, sprints and tasks, records, needs and actions; Part 3 writing to the owner) and run as 4 hook entries of 4,708 to 8,447 characters, each under Claude Code's per-hook 10,000-character cap and under its own heading, since hooks arrive in any order; `--state` prints setup, `pm where` and `pm show`; `pm prime --subagent` prints the profile line; `pm hook stop` and `pm hook owner-request`; every old hook script is deleted.
- Guidance for developing pm itself lives in `pm/AGENTS.md` (with a `pm/CLAUDE.md` link), not in anything the package ships.
- The package's tests follow sprint 59: pruned to the flows agents and the owner depend on, run in parallel, split into `make test` and `make test-full`.
- `.pm/config.toml` holds version, remote, main branch, port and site URL; pm takes them from it instead of hardcoded values and git config.
- Stop hook entries exit 1, never 2, when `bin/pm` cannot run, so a broken install cannot trap a session.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: `uvx --from "git+https://github.com/Yeeef/yeeef-agents@pm-package#subdirectory=pm" pm show` prints the project state in a checkout of the branch; `make test` gives 36 passed, 35 skipped on f9741e8 (the skips are live-model eval cases); `make test-full` gave 48 passed after main's suite was ported (69637fa) and was not rerun after the chunk change, as the owner asked for fast targeted tests only; the chunk tests (each chunk at most 10,000 characters, the chunks add up to the whole text) pass, and two headless `claude -p` sessions got all 4 chunks whole and inline.
- Met: `pm prime`, `pm prime --subagent` and `pm hook stop` match the old scripts' stdout and exit code on 10 inputs, and `pm hook owner-request` matches main's script on 5 inputs (exit code, stdout, Beads calls and the judge's input); the tests cover each; the reply-wait hook no longer exists on main.
- Met: with the pin set to 9.9.9, `bin/pm show` and `bin/pm hook stop` exit 1 naming both versions and the `uv tool install` command; `pm/tests/test_config.py` covers missing config and mismatch.
