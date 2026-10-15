---
type: doc
title: "pm command surface: quality audit and simplifications"
date: 2026-10-16
project: pm-quality
---

An audit of the `pm` command surface at 0.14.0 (main 899bd3c, 2026-10-16), made by sprint pm-quality 37. It measures three things: what the surface is, how agents use it, and what the records say it should be. Then it ranks simplifications. It changes no code: each proposal is a candidate for its own sprint.

## Summary

- **The surface doubled in 8 days.** It grew from 39 command nodes (10-07, Python) to 81 (10-15). Over 27 releases it reached 63 leaf commands and 48 distinct flags.
- **Agents mostly use it correctly.** 6.7% of 3,768 real invocations failed, and only 1.5% were syntax guesses.
- **Usage is concentrated.** The top 15 of 65 commands used carry 83% of invocations, the top 20 carry 91%, and 13 leaves were used 3 times or fewer.
- **Friction sits in a few spots.** `show --record` without `--section` fails 98% of the time. The two need forms fail 25–30%, mostly as intended guards. Id, name and path confusion caused 33 failures.
- **The cost is in naming, not in count.** A scope is named 5 ways, a body is given 6 ways, an id positional has 8 names, and a need closes 3 ways. That is the surface an agent must remember, and prime.md (3,218 words) spells much of it out.
- **The biggest wins are cheap.** An outline for `show --record`, refusing everything at once in the need forms, removing dead and legacy pieces, and one scope vocabulary.

## Method and sources

| Source | What | Scope |
|---|---|---|
| Command tree | `--help` walked recursively on the dev build, plus `internal/cli/commands.go` | HEAD 899bd3c |
| Code size | `wc` over `internal/cli`, `refuse(` call sites, `help.golden`, `prime.md` | HEAD |
| Growth | Command nodes counted at each commit of `commands.go`, and CHANGELOG Added/Removed | 10-07 to 10-15 |
| Usage | Every `pm` invocation in Claude Code transcripts: 61 sessions and 252 files (main and subagent), across the pm, yeeef-agents, poker and formal-methods repos | 10-05 to 10-15 |
| Records | Design pages (pm-cli, pm-product, sprint-lifecycle, owner-questions, decision-need-layout), 105 `::: decision` blocks about commands (35 owner, 70 agent), the pm feedback doc (36 entries) and its triage | All |

Limits of the usage data:

- **Window.** It covers 11 days and one owner's repos.
- **Codex.** There are no Codex invocations: 1,761 rollouts hold no pm calls.
- **Filtered rows.** 245 dev and test-binary rows and 242 `--help` rows are excluded from the 3,768.
- **Undercounted failures.** Agents often pipe through `2>&1 | tail`, which can hide an error line.
- **Mixed pins.** The transcripts span pins 0.3 to 0.14.

## The surface

::: result {title="Command surface at 0.14.0"}
| Measure | Value |
|---|---|
| Top-level entries | 34: 18 nouns with verbs, 16 top-level leaves |
| Verbs under nouns | 47 |
| Leaf commands | 63, plus 2 forms missing from usage lines (`pm show ID`, `pm task add --parent`) |
| Flag slots / distinct flags | 121 / 48; per leaf: median 1, max 8 (`decision add`, `action need`) |
| Machinery leaves, not typed by agents | 13: `hook` ×6, `prime`, `push`, `service run`, `sync`, `export`, `day summarize`, `autopilot` |
| Single-verb nouns | 7: day, finding, feedback, comment, postmortem, record, reply (for agents) |
| `internal/cli` Go | 9,680 lines without tests; 1,673 lines of tests |
| Refusals | 194 `refuse(` sites, 176 distinct formats |
| `--help` text | 829 lines live; `help.golden` 911 lines, 74 KB |
| `prime.md` | 140 lines, 3,218 words, 18 KB; names 38 of 63 leaves and 30 flags |
| Hook chunks | About 3.7, 6.7 and 7.6 KB against a 10 KB cap; the third is at 76% |
:::

Reading: the surface is wide and shallow (depth 2). About a fifth of it is machinery that agents never type, yet it shows in the same help.

::: result {title="Growth of the command tree"}
| Date | Nodes | What drove it |
|---|---|---|
| 10-07 | 39 → 56 | Python CLI: `day summarize`, `feedback add`, `show` levels |
| 10-08 | 55 | Go port |
| 10-10 | 58 → 74 | Work-store commands replace raw `bd`; every dispatched command enters the help tree (pm-quality 7); `sprint move`, `clean`, `service stop` |
| 10-13 | 77 | `design unchanged`, `hook ask`, `doctor`, `upgrade` |
| 10-15 | 76 → 81 | `reply wait`, `hook inbox` removed; `design/doc rename`, `interactive`, `autopilot`, `hook session-end` added |
:::

Reading: nearly every addition answered a real gap, most of them from pm feedback (lifecycle edits: `sprint edit/move`, `need edit`, `task move`). Only one pass so far has removed anything (pm-quality 34).

## How agents use it

::: result {title="Most used commands (3,768 invocations, 10-05 to 10-15)"}
| Command | Count | Failed |
|---|---|---|
| show (all forms) | 525 | 12.8% |
| commit | 473 | 6.6% |
| finding add | 347 | 2.6% |
| where records | 255 | 0.4% |
| task close | 235 | 3.4% |
| task add | 202 | 1.5% |
| decision add | 197 | 2.0% |
| task claim | 182 | 2.7% |
| check | 158 | 5.1% |
| sprint close | 145 | 16.6% |
| action need | 137 | 24.8% |
| sprint open | 129 | 3.1% |
| action done | 92 | 3.3% |
| record link | 84 | 6.0% |
| decision need | 66 | 30.3% |
| need edit | 9 | 44.4% |
:::

Reading: the daily loop of show, commit, finding, task and decision is healthy, with 2–7% failures. The failures cluster in the owner-facing and gate commands.

Usage depth:

- **Rarely used, 3 or fewer invocations:** `day summarize`, `decision close`, `postmortem new`, `project close`, `task ready`, `task release`, `dep rm`, `sync`, `service install/logs/stop`.
- **Never typed:** `hook ask`, `hook git-post-checkout`, `hook git-pre-commit`.
- **Stale names still typed:** `pm setup` 5 times and `pm render` 2 times. Both were removed earlier.

::: result {title="Why invocations failed (252 failures)"}
| Count | Cluster | Kind |
|---|---|---|
| 41 | `show --record X` without `--section` | Friction: the agent wanted the record |
| 35 | Design `covers:` gate (`check`, `sprint close`) | Guard doing its job; 4 repeats on one sprint |
| 33 | Id, name or path confusion: `show --record <sprint id>` (12), `commit sprints/x.md` (6), "is a sprint, not a task", a name that is both design and project | Friction |
| 26 | Store state: nothing to commit, uncommitted changes, conflict markers | Mostly guard |
| 25 | `action need --pr` before the Delivery report is written (20), open tasks | Guard, by design (pm-harness 42) |
| 22 | Need-form rules: label not alphanumeric, sentence over 25 words, PR review raised as a plain action (6), `need edit` on a review (3) | Guard; one rule per retry |
| 16 | Pin and service version skew | Environment |
| 17 | Argparse: `commit` without `-m` (6), unknown flags | Syntax guess |
| 37 | Already closed or held, claim from main checkout, other | Mixed |
:::

Reading: about half of the failures are guards that refused the right thing. The real friction is the `show --record` shape, names versus ids, and need-form rules that surface one at a time.

Behaviour around failures:

- **Retries.** 57% of failures were retried within 3 calls, and 82% of those retries succeeded.
- **Help after failure.** Only 5 failures were followed by `--help`, so the refusal text is what agents read.
- **Help before use.** 5% of commands had a `--help` of the same noun shortly before.
- **Bodies are solved.** Commands with a body failed 6.8% of the time, against 6.7% without one, and only 2 `--text` values held a backtick or `$`.

## Against the design intent

The records state these rules for the CLI (pm-cli page, pm-harness project decisions of 10-04 and 10-07):

- pm is self-contained, with progressive disclosure: prime, then show, then `--help`.
- pm wraps only actions that touch both layers or need a check.
- Every write names its target.
- Refusals show the expected shape, and old names are removed, not aliased.
- Bodies take one of two forms.

| Rule | Where the surface drifts | Evidence |
|---|---|---|
| Progressive disclosure: prime names, `--help` details | prime.md inlines flag shapes and refusal lists for about 45 forms | 3,218 words; each inlined refusal is a sync duty that `test_pm.py` polices |
| `show` goes down level by level | The record level refuses instead of showing its next level (its sections) | 41 of 42 bare `--record` calls failed |
| Every write names its target, one way | A scope is named 5 ways: `--parent`, `--sprint`, `--project`, `--bead`, `--level` plus `--project/--sprint` | `commands.go` 113, 131, 160, 252, 413 |
| Old names removed, not kept | `--bead` survives Beads (retired in 0.2.0); two retired hooks remain as no-op commands | `commands.go` 252, 723, 730 |
| Fail hard, no silent default | `task close` without `--reason` stores "Done" | `internal/cli/task.go:75`; feedback 10-11, still open |
| One help renderer | 9 store commands use a second renderer (no `[-h]`, different layout) | `commands.go` 205–565 |
| A design page holds the final state | The pm-cli page still describes `bd` subprocesses, `pm serve`, `pm render` and `pm setup` | pm-cli design page |
| Usage lines tell the truth | `task add` marks `--sprint` required but takes `--parent`; `decision add --level` reads optional but is required | `commands.go` 112, 413 |

Naming spread an agent must hold:

| Concept | Spellings |
|---|---|
| Create | `open` (project, sprint), `add` (task, finding, feedback, comment, dep, reply, decision), `new` (doc, design, postmortem), `need` (decision, action) |
| Close a need | `decision add --need`, `decision close`, `action done`, `need dismiss` |
| Body | `--text`/`--text-file`, structured flags (`decision add/need`), a positional (`finding add`), `--reason`, `-m` |
| Id positional | `need_id`, `ID`, `task_id`, `sprint_id`, `name`, `project`, `slug`, `target` |
| `--to` | a project, a sprint, a new slug or a version |

## Owner decisions this audit must respect

These are closed:

- **No regrouping (pm-harness 18, 10-04).** "pm commands are not regrouped and the help output stays as it is." It was made when the tree had about 30 nodes and the owner found the help already good.
- **The CLI is interface (pm-quality 21, 10-13).** "pm keeps its CLI grammar … its help text and its refusal wording": these are the interface that prime.md and agents rely on. This was ruled during the retirement of the Python port.
- **Structured flags (pm-harness 67, 10-07).** `decision add` takes `--decision` and `--reason` as flags.
- **No `pm section set` (pm-harness 12).** Records stay free-form hand edits.
- **The prime.md shape and deliberate redundancy (pm-harness project, 10-07).**
- **The removal of `reply wait` and `hook inbox` (pm-quality 34).**

Any proposal below marked **owner** changes one of these and needs a new owner decision, not an agent one.

## Proposals, ranked

Ranked by evidence-backed failures or effort removed, per unit of cost.

| # | Proposal | Evidence | Cost | Touches |
|---|---|---|---|---|
| 1 | **`pm show --record X` alone prints the record's outline**: header ids, each section with its line count, and the `--section` command for each. It also accepts a sprint id, project name or slug wherever it accepts a path. | 41 of 42 failed; 12 more "no record matches" | Small: `show.go`, golden, prime line | Only the refusal wording; the level model gains its missing step |
| 2 | **The need forms check every rule, then refuse once with the full list**: labels, sentence length, missing costs. | `decision need` 30% failed, 20 retries; it stops at the first violation (`needs.go` 47–140) | Small | No wording change per rule |
| 3 | **Remove dead and legacy pieces**: the no-op `hook git-post-checkout` and `hook git-pre-commit` (once `pm upgrade` has stripped them from pinned repos), `doc new --bead` (to `--sprint/--task`), and the 9-command second help renderer. | 0 uses; Beads retired at 0.2.0 | Small | Help text (pm-quality 21 covers wording, not dead commands) |
| 4 | **`task close` requires `--reason`.** Drop the "Done" default. | Feedback 10-11, open; the fail-hard rule | Trivial; a refusal pointing at `--reason` | None |
| 5 | **One scope vocabulary**: `--sprint`, `--task`, `--project`, with the same meaning on every command. `decision/action need --parent` → `--sprint/--task`; drop `decision add --level`, which the flag already implies; `task add --parent` shows in the usage line. | 5 spellings; `--level` is redundant with the exclusive `--project/--sprint` | Medium: flags, golden, prime, tests; one release keeps old flags refused with the new name | **owner** (pm-quality 21 grammar) |
| 6 | **Hide the 13 machinery leaves from `pm --help`** behind `pm --help --all`. They stay documented and dispatched. | 0–5 typed uses each; they show beside the 50 agent commands | Small | **owner** (pm-harness 18 "help stays as it is"; pm-quality 7 put every command in the tree) |
| 7 | **Thin prime.md by usage.** Keep full procedures for the 20 commands that carry 91% of invocations. Name the rest in one line pointing at `--help`, and move inlined refusal lists into `--help`. | 3,218 words; the third hook chunk at 76% of its cap; agents read refusals, not help | Medium: prime, `test_pm.py` assertions, hook chunks | **owner** (the redundancy decision); a tuning, not a reversal |
| 8 | **One verb to close a need that sets no rule**: `need close <id> --reason` replaces `decision close`, `action done` and `need dismiss`. `decision add --need` stays the path that records a rule. | 4 verbs for one act; `decision close` used 1–3 times, `need dismiss` 9 | Medium | **owner** (pm-harness 13: close verbs sit under their noun) |
| 9 | **Rewrite the pm-cli design page to the final state**: no `bd`, `serve`, `render` or `setup`. Add the naming table above as the CLI's grammar, so new commands follow it. | The page contradicts prime.md | Small, records only | None |
| 10 | **A grammar check in CI**: a Go test over `commands.go` that refuses a new scope flag, create verb or positional name outside the table from #9. | Each spread above grew one command at a time | Small, after #5 and #9 | Follows #5 |

Not proposed:

- **Removing low-use commands** (`postmortem new`, `project close`, `dep rm`, `task release`). They are rare by nature, not redundant.
- **Merging `check` into `commit`, or `sync` into `push`.** Each is a distinct, used step: `check` 158 calls.
- **Relaxing the PR-review ordering refusal or the covers gate.** These are guards working as designed (pm-harness 42, pm-quality 28).

## Suggested order

Wave 1 (one sprint, no owner decision needed) is #1, #2, #3, #4 and #9. Evidence says this removes about 60 of the 252 observed failures: 41 from `show --record`, plus most need-form retries. It also clears every drift item in the design-intent table except naming.

Wave 2 needs owner decisions: #5 and #8 (naming), then #10 to hold it. #6 and #7 are independent and can run in parallel with them.

## Open questions

- Whether Codex agents fail differently. No Codex pm calls exist in the corpus.
- Whether the numbers hold past 11 days. Re-running the extraction after wave 1 would give a before-and-after on the same method.
