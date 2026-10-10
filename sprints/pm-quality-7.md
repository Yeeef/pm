---
type: sprint
title: Every pm command is listed in pm --help
bead: pm-d2k5.7
---

## Goal

> What should be true when this sprint ends, and why now?

`pm --help`, each `pm <noun> --help` and `pm prime`'s noun list name every command pm runs, and each help text says what the code does.

Today `goOnly` (`internal/cli/work.go`) dispatches `task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`, `show ID`, `version` and `export` outside the command tree. So `pm dep` answers "invalid choice: 'dep'", although prime.md says to run `pm dep add` (formal-methods 2026-10-10 03:23). `pm need dismiss`, the close for a replaced review or a moot need, was found only by grepping the source (pm 2026-10-10 03:59). `pm task claim --help` says `--session` applies only when the environment has no session id, but the code uses the flag first (pm 2026-10-06 23:10).

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Move the store commands into `commands.go`'s tree with unchanged flags and behaviour, so they show in `pm --help`, in `pm <noun> --help` and in `hooks.Commands`' noun list.
- `pm dep` and `pm need` with no subcommand print their noun's help.
- `pm decision close --help`, and its refusal on a need the owner never answered, name `pm need dismiss` for a need that became moot.
- Fix the `--session` help of `pm task claim`.
- A test that fails when a command the dispatcher runs is missing from the `pm --help` tree.
- A CHANGELOG entry.

**Out:**
- New commands, and renamed ones.
- Hand edits to prime.md beyond what the generated noun list adds. If the longer list outgrows a chunk, move a heading in `hooks.Starts`.

## Done when

> What evidence will show the goal is met?

- `pm dep --help` and `pm need --help` exit 0 and list their subcommands. `pm --help` lists `dep` and `need`, and `pm task --help` lists `ready`, `edit` and `release`.
- The new test walks `storeCommands` and `goOnly`'s names and passes. Removing one command from the tree makes it fail.
- `pm prime` lists `dep` and `need`, and `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` passes.
- A harness test shows that `pm task claim --session X` records X even with `$CLAUDE_CODE_SESSION_ID` set, and the help text says so.
- `make test`, `make test-go` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Before this sprint 11 commands ran outside the argparse tree (goOnly: task
  ready/edit/release, dep add/rm, comment add, need dismiss, reply add, sync,
  version, export) plus init --import-bd/--import; pm --help listed 23 nouns,
  now 29 (dep, need, comment, sync, export, version added). Every one is a
  tree leaf now; only the forms pm show ID and pm task add --parent are routed
  before the tree, and their tree command's help names them.

- --text=Single source rather than a list-vs-tree test: a work-store leaf
  carries its storeCommand and storeCommands is derived from the tree, so a
  store command cannot exist outside pm --help. Mutation check: deleting the
  need noun from commands.go fails TestTheCommandsOnceOutsideTheTreeAreInIt
  (pm need dismiss is not listed in its noun's --help) and TestNeedDismiss (no
  store command).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
