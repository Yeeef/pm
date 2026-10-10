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
  version, export) plus init --import-bd/--import; pm prime's noun list named
  23 nouns, now 29 (dep, need, comment, sync, export, version; pm --help 26,
  now 32). Every one is a
  tree leaf now; only the forms pm show ID and pm task add --parent are routed
  before the tree, and their tree command's help names them.

- Single source rather than a list-vs-tree test: a work-store leaf
  carries its storeCommand and storeCommands is derived from the tree, so a
  store command cannot exist outside pm --help. Mutation check: deleting the
  need noun from commands.go fails TestTheCommandsOnceOutsideTheTreeAreInIt
  (pm need dismiss is not listed in its noun's --help) and TestNeedDismiss (no
  store command).

- Rules chunks with the six new nouns: 3,647 / 6,417 / 6,837 characters
  against the 10,000 cap (16,737 without titles); no change to hooks.Starts
  needed.

- The defect showed itself during this sprint: pm finding add --text="…" run
  with the installed release (0.5.0's pin) stored the literal "--text=" as
  this sprint's second finding, fixed here by hand.

- PR #30 CI: race (darwin-arm64) failed once on a DATA RACE in
  internal/service's test, untouched here:
  TestAReplyIsSpooledStoredOnceAndPushedIntoTheSessionsInbox reads the
  fakeStore (run_test.go:394) while the service's writer goroutine writes it
  through fakeStore.UpdateNeed (fake_test.go:140) with no lock; linux race and
  local make test-go passed. Rerun once with gh run rerun --failed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: every command pm runs is a command of the tree, so `pm --help`, each `pm <noun> --help` and `pm prime`'s noun list name it, and a test fails when one is not; PR #30, pending merge.

- The work-store commands (`task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`) are tree leaves carrying their own flags and help, unchanged; the store-command table is derived from the tree, so none can exist outside `pm --help`. `pm version` and `pm export [--store DIR]` are tree commands, and `--import-bd`/`--import` are `pm init` arguments. Only `pm show ID` and `pm task add --parent` are routed before the tree, and their tree command's help names them.
- `internal/cli/commands_test.go`: walks the tree through `pm … --help`, every store command and every agent command body.
- `pm decision close` (refusal and `--help`) and `pm need dismiss`'s help name dismiss for a need that became moot; `pm task claim --help` states that `--session` wins over the environment.
- `pm finding add` takes `--text`/`--text-file`, refuses both forms at once and a positional text starting with `--`.
- Not done: `pm dep` and `pm need` with no subcommand give the usage error naming their subcommands (`usage: pm dep [-h] {add,rm} ...`, exit 2), as every other noun does, rather than printing the full help (Scope's wording).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `pm dep --help` and `pm need --help` exit 0 and list their subcommands; `pm --help` lists `dep` and `need`; `pm task --help` lists `ready`, `edit`, `release`: met. `.go/pm dep --help` exit 0 lists `{add,rm}`; `pm need --help` lists `{dismiss}`; `pm --help` usage `{show,…,need,…,dep,comment,…,sync,…,export,version,…}`; `pm task --help` usage `{add,close,claim,move,ready,edit,release}`. Harness: `test_pm_help_lists_the_commands_that_ran_outside_the_command_tree`.
- The new test walks `storeCommands` and `goOnly`'s names and passes; removing one command from the tree makes it fail: met. `goOnly` is gone (its names are tree commands); `TestEveryCommandPmRunsIsListedInHelp` and `TestTheCommandsOnceOutsideTheTreeAreInIt` pass; deleting the `need` noun from `commands.go` fails the latter ("pm need dismiss is not listed in its noun's --help") and `TestNeedDismiss`.
- `pm prime` lists `dep` and `need`, and `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` passes: met. Noun list now 29 nouns incl. `need`, `dep`; chunks 3,647 / 6,417 / 6,837 characters (cap 10,000); `test_prime_lists_every_agent_command_pm_help_lists` and the chunks test pass.
- A harness test shows `pm task claim --session X` records X with `$CLAUDE_CODE_SESSION_ID` set, and the help says so: met. `test_task_claim_records_the_session_given_with_session_even_with_one_in_the_environment`.
- `make test`, `make test-go` and the PR's CI pass: met locally (`make test` 131 passed, 63 skipped live-model tests; `make test-go` pass). CI on PR #30: green; `race (darwin-arm64)` passed on its one rerun after a data race in `internal/service`'s test fake (untouched here; see Findings).
