---
type: sprint
title: Split internal/cli by noun and retire the Python-port leftovers
bead: pm-d2k5.21
---

## Goal

> What should be true when this sprint ends, and why now?

`internal/cli` stops being a god package: 8.9k LOC, 34% of non-test code, `writes.go` 1,463 code lines. Its Python-port leftovers go: the hand-rolled argparse emulation (`parse`, cognitive complexity 105) and `pyjson` where no output depends on them. The trimmed suite (sprint 20) and the lint ratchet (sprint 19) are the safety net.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Split `internal/cli` by noun.
- Break up the 5 longest functions: `fromIssue`, `cmdUninstall`, `parse`, `cmdSprintClose`, `mergeItem`.
- Retire the argparse emulation: the parser keeps only pm's CLI grammar that pflag cannot express (two-value options, alternative-flag groups, required flags); abbreviated flags and argparse naming go.
- Retire `internal/pyjson`: JSON through `encoding/json`, Go quoting in messages and TOML; a user's key order kept only where pm rewrites a file the user owns.
- Lower the package-size allowance and the ratchet counts to the new state.
- Fix PR #62's review findings: the help text the field rename changed, and the doc comments the split broke.

**Out:**
- Behaviour changes beyond the JSON bytes, quoted values and abbreviated flags the retirement changes.
- Changing pm's CLI grammar, help text or refusal wording.
- Clearing the errcheck backlog outside the touched code.

## Done when

> What evidence will show the goal is met?

- No file in `internal/cli` is over 800 lines.
- No function has gocyclo over 30.
- `internal/cli`'s package-size allowance has dropped.
- No Go file imports `internal/pyjson`, and the package is gone.
- An abbreviated flag is refused, and a test holds it.
- Every command's `--help` output equals origin/main's.
- A changelog entry names each byte change, with an upgrade step if a clone's files change.
- A scratch clone set up by the previous release reports `pm doctor` clean after `pm upgrade` to this build.
- `make test-live-parity` passes: real Claude Code and Codex sessions read the new hook output.
- `make lint`, `make test` and `make test-go` pass, and CI is green.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): command packages, parser stages and permanent limits.
- [pm in Go](../design/pm-go.md): module boundaries and JSON compatibility.
- [pm as an installable product](../design/pm-product.md): setup coverage.
- [Decision need layout](../design/decision-need-layout.md), [Sprint lifecycle](../design/sprint-lifecycle.md), [Session claims](../design/session-claims.md), [Reply delivery](../design/reply-delivery.md): new command-file ownership.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
Apply the cyclomatic-complexity threshold to every Go function, including existing tests.
The Done when says no function exceeds 30; applying it globally gives an exact check and removes ambiguity about the named five.
:::

::: decision {source=owner date=2026-10-13}
Rescope: the sprint retires the Python-port leftovers in full, not only where no output depends on them. internal/pyjson goes (JSON through encoding/json, Go quoting in messages and TOML), the parser drops argparse emulation (abbreviated flags, argparse naming); pm keeps its CLI grammar (two-value options, alternative-flag groups, required flags), its help text and its refusal wording apart from how a quoted value is quoted. The resulting byte changes in JSON output, rewritten hook files and quoted values ship with a changelog entry.
The owner asked in chat on 2026-10-13 to take the sprint over and deliver the retirement: the byte-identical-to-Python guarantee served the migration from Python pm, which is retired (0.1.x), so nothing reads those bytes as Python wrote them; the CLI grammar and refusal wording are pm interface that prime.md and agents rely on, not emulation.
:::

::: decision {source=agent date=2026-10-13}
The design page agent-lifecycle stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page owner-request-hook stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page pm-in-codex stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page pm-versioning stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page record-layer stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page records-store stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page site-replies stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page views-and-site stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

::: decision {source=agent date=2026-10-13}
The design page work-store stays as it is for this sprint's code changes.
the PR moves this page's covered code into noun files and subpackages, quotes values with Go quoting and writes its JSON with encoding/json; the page names no file layout, quoting or JSON byte form, so what it says stays true
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline: internal/cli has 8,896 non-test Go lines; writes.go has 1,576
  total lines, install.go 1,013, needs.go 950. Before changes, make test
  passed 83 tests, skipped 136, in 11.27 seconds.

- Verification: make test passed 83 tests (136 skipped) in 11.31 s; focused
  setup integration passed 18 tests in 12.69 s. make lint passed all gates and
  the ratchet with zero findings. make test-go passed, including eight race
  tests in eight shards (11.4 s). All rendered site goldens remain unchanged.
  Command package: 7,445 non-test lines (down from 8,896); largest file is
  commands.go at 714 lines. Every Go function is at most cyclomatic complexity
  30, now a global CI gate; every cli Go file is capped at 800 lines.

- Final implementation commit d4acff4 is PR #62. After review restored
  existing best-effort cleanup semantics, make lint passed with zero global
  and ratchet findings; uninstall integration passed 3 tests in 4.87 s;
  focused merge/sync tests passed in 55.665 s. Final light harness passed 83
  tests, skipped 136, in 11.14 s. Fresh review found no correctness issues,
  including a second read of the cleanup patch. Human-facing/stored pyjson
  forms are retained; only machine inbox JSON encoding was replaced. The
  parser already used pflag before this sprint; its remaining normalization
  and explicit checks are now staged in the parse package.

- Review of PR #62 found the field rename had also capitalised words inside 11
  commands' help strings (Required:, Kind, Description); no test held help
  text. A walk of all 77 command paths against a main build went from 11
  differing to 0, and help.golden now freezes every --help output. A scan of
  doc comments found 14 the split had moved off their functions; doc_test.go
  now fails on a doc comment that does not open with its function's name.

- Abbreviated flags had no real users: 0 in prime.md, prompts, help, tests,
  hook entries, website and records; 0 in ~1,766 pm commands with a flag
  across 137 agent transcripts (1 false positive inside quoted text). The
  store commands that parse their own flags never accepted abbreviations. pm
  task add --tit x now exits 2 with 'unrecognized arguments: --tit x'; unknown
  arguments are now reported before missing required ones, so the refusal
  names the real mistake.

- pyjson's real requirement was narrower than its 429 lines: of its uses only
  the user-owned settings files (.claude/settings.json, .codex/hooks.json,
  settings.local.json) need key order kept, now a token-based ordered object
  in internal/install. For ASCII content every file pm writes is
  byte-identical (pieces.json golden and the site goldens unchanged); bytes
  change in hook output and pm show --json (compact or raw UTF-8), /version,
  and in every message that quotes a value ('x' -> "x"; 11 harness assertions
  updated). config.toml used to get a surrogate pair for a character outside
  the BMP, which is invalid TOML; the new quoter is checked by decoding back
  with BurntSushi/toml.

- Live upgrade check, scratch origin and clone with scratch HOME/CODEX_HOME
  and the harness's fake supervisor: a clone set up by pm 0.8.2, with user
  content in .claude/settings.json, settings.local.json and .codex/hooks.json
  (unusual key order, non-ASCII, &<>, floats), then pm upgrade to this build,
  pm service restart, pm doctor (exit 0, every piece matches), pm init and
  every hook run: all hook files byte-identical to what 0.8.2 left; only the
  pin line in .pm/config.toml and the post-checkout marker changed. With pm's
  Stop entry removed, pm upgrade rewrote the file byte-identical to the
  original. Hook output is valid JSON, non-ASCII raw. Pre-existing wart, both
  versions: the 'not laid out as pm writes JSON' refusal suggests python3 -m
  json.tool, whose default output (ensure_ascii) is refused again for
  non-ASCII content.

- Parity live eval: on this host Codex's bubblewrap sandbox cannot start
  (bwrap: loopback: Failed RTM_NEWADDR;
  apparmor_restrict_unprivileged_userns=1), so under the default
  workspace-write sandbox every Codex scenario that runs git or edits a file
  fails: branch run 5 failed, 7 passed (603 s); origin/main baseline 4 failed,
  8 passed (605 s), the same scenarios plus chat-ask passing by luck. PR head
  with PM_PARITY_CODEX_SANDBOX=danger-full-access, Codex scenarios: 6 passed,
  1 failed (607 s); all 6 Claude scenarios passed on the branch run. The one
  failure, Codex's ask tool, fails the same on origin/main: Shift+Left does
  not open the request_user_input_async pick list on codex-cli 0.162.1, a
  test-harness issue outside this sprint. No transcript shows a hook error, a
  JSON parse error, an unrecognized-argument refusal or a quoting problem;
  Codex's judge parsed the compact hook JSON and blocked the chat-only ask.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

done: `internal/cli` is split by noun, every function is under cyclomatic complexity 30, and the Python-port leftovers are retired (`internal/pyjson` deleted, the parser's argparse emulation gone), in [PR #62](https://github.com/Yeeef/pm/pull/62).

Merged as bb44e51 (PR #62).

- Command bodies live by noun; the parser and setup have their own packages (`internal/cli/parse`, `internal/cli/setup`, `internal/cli/fsutil`). The `internal/cli` package is 7,457 non-test lines, down from 8,896; that is code moved into subpackages, not removed: everything under `internal/cli` totals 9,347 lines, against 8,859 on main before the merge.
- `internal/pyjson` (429 lines) is deleted: JSON goes through `encoding/json`, a small ordered object keeps a user's own settings files' keys, values and order on rewrite, messages quote values with Go quoting, and `.pm/config.toml` values go through a tested TOML quoter.
- The parser keeps pm's CLI grammar and names it in pm's terms; abbreviated flags are refused.
- The help text the refactor had changed is restored and frozen by a golden of every `--help`; a test holds every doc comment to its function.
- Each byte change is in `changelog.d/retire-python-port-leftovers.md`; no clone needs an upgrade step.

### Against "Done when"

| Check | Result | Evidence |
|---|---|---|
| No `internal/cli` file over 800 lines | Met | Largest is `commands.go`, 715 lines; `size_test.go` caps every Go file under `internal/cli`, tests included |
| No function above gocyclo 30 | Met | gocyclo gates every function in `make lint` at 30: 0 issues |
| `internal/cli` package-size allowance dropped | Met | 8,896 to 7,457 (`oversized` in `size_test.go`) |
| No Go file imports `internal/pyjson`, the package is gone | Met | `git grep -n pyjson -- '*.go'` is empty on the PR head; `internal/pyjson/` deleted |
| An abbreviated flag is refused, a test holds it | Met | `pm task add --tit x` exits 2, "unrecognized arguments: --tit x"; `TestAnAbbreviatedOptionIsRefused` fails against the old parser |
| Every `--help` equals origin/main's | Met | All 77 command paths, origin/main build against the branch: 11 differed after the original refactor, 0 after the fix; `internal/cli/testdata/help.golden` freezes them, its only rebase changes main's own `doctor` and `upgrade` lines |
| Changelog names each byte change, upgrade step if needed | Met | One entry file; the upgrade check below found no file a clone must rewrite, so no step |
| A clone of the previous release reports `pm doctor` clean after `pm upgrade` | Met | Scratch origin and clone set up by pm 0.8.2 with user content in all three settings files: after `pm upgrade` and `pm service restart`, `pm doctor` exits 0; every settings file byte-identical; only the pin line and the post-checkout marker changed (Findings) |
| `make test-live-parity` passes | Partly met | All 6 Claude scenarios pass; Codex with `PM_PARITY_CODEX_SANDBOX=danger-full-access` (this host's bubblewrap cannot start): 6 of 7 pass. The seventh, Codex's ask tool, fails the same way on origin/main: the TUI never opens the pick list. No transcript shows a hook, JSON or parser error (Findings) |
| `make lint`, `make test`, `make test-go`, CI green | Met | lint 0 issues; `make test` 129 passed, 149 skipped (the live evals); `make -j2 test-go` ok, 31 work tests and 8 race tests in 8 shards; CI on PR #62's rebased head all green |

Reviews: a fresh review of the original refactor found the help-text change; a fresh review of the retirement found no correctness defect, and its edge case (a raw U+2028 in a settings file is now refused) is in the changelog.
