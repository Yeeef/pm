---
type: sprint
title: "pm explains its own setup: one install command, every setting in --help"
bead: yeeef-agents-9va.73
---

## Goal

> What should be true when this sprint ends, and why now?

An agent in any repo finds every pm setting and setup step from pm itself (`pm --help`, a command's `--help`, `pm prime`), with `pm init` as the one install command, so nobody reads pm's source to configure it. Why now: on 2026-10-07, the first install into another repo (formal-methods) needed the source to find the site URL setting, and failed on a port that another server held.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm setup` becomes a hidden hook entry point (`pm hook …`) that session start runs, gone from `pm --help`; `--site-url` only on `pm init`, its help naming every effect (links, accepted reply host); `pm --help`'s summary and `pm prime` name where the site URL and port are set; a test that every `.pm/config.toml` key is documented by a command's `--help`; `pm init` checks the site port is free before writing anything and gives a new repo a free default port; a release tag after the change.
**Out:** new settings; Codex parity (sprint 34); pm show levels (sprint 57).

## Done when

> What evidence will show the goal is met?

- `pm --help` lists no `setup`, and session start still sets up a new worktree (a test). Expected: the session-start hook runs the hidden entry point.
- A test fails when a `.pm/config.toml` key is not named in any command's `--help`. Expected: it passes for `version`, `remote`, `main_branch`, `port` and `site_url`.
- `pm init` in a repo whose default port is held refuses before writing any file, naming a free port. Expected: shown by a test with a held port.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm explains its own setup and has one install command, `pm init` ([PR #67](https://github.com/Yeeef/yeeef-agents/pull/67), pm 0.1.1).

Merged as 82a58ee (PR #67).

- `pm init`'s repo step runs only on first install; its clone and worktree step runs every time, and session start runs it as `pm init --session-start`, which installs a missing service but only reports a stale or down one; repairs are `pm doctor` then `pm upgrade`; `pm setup` is removed.
- `--site-url` only on `pm init`, its help naming every effect; `pm --help` and `pm prime` say where the site URL and port are set.
- `pm init` refuses a site port another server holds before writing anything and names a free one; a new repo's default port is the first free one no other pm clone uses; an install lock guards parallel installs.
- One fresh-context review: 9 findings, 7 fixed with tests, 1 kept as fail-hard (a tool installed from a local folder), 1 (the release tag) at merge.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`pm --help` lists no `setup`, and session start still sets up a new worktree: met.** `test_pm_init_is_the_one_install_command_and_session_start_runs_it` checks the help and that `pm setup` is rejected (argparse exit 2); the session-start tests run `pm init --session-start` and set up a new worktree.
- **A test fails when a `.pm/config.toml` key is not named in any command's `--help`: met.** `test_every_config_key_is_named_in_some_commands_help` failed for `remote` and `main_branch` before init's help named them, and passes for all five keys.
- **`pm init` with a held default port refuses before writing any file, naming a free port: met.** `test_init_refuses_a_held_site_port_before_writing_anything` holds the port with a socket: the refusal leaves every file unchanged, with no `bd init`, no records branch and no service, and a later init writes the free port it named.
- Evidence on 0b3c857: `make test` 49 passed, 35 skipped (live-model eval); CI on PR #67 passed light (23 s), integration (59 s) and guard ([run](https://github.com/Yeeef/yeeef-agents/actions/runs/37685672539)).
