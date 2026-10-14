---
type: design
title: AGENTS.md rubric and its agentic evaluation
project: pm-quality
covers:
  - "tests/agents_md/**"
  - "tests/test_agents_md_eval.py"
  - "tests/test_agents_md_live.py"
  - ".github/workflows/agents-md.yml"
---

## Problem

> What are we solving, and why now?

AGENTS.md (CLAUDE.md links to it) is the one file every Claude Code and Codex session developing pm loads whole. It
drifts: claims about the code go stale, narration of the code piles up, rules get copied from `prime.md`, which these
sessions load too. A drifting AGENTS.md costs every session context and misleads it. The defects that matter are
semantic (a behaviour claim the code refutes, a passage an agent could infer, a copy reworded), which a lint of names
cannot see and an agent reading the code can.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

Goals:

- A rubric for a good AGENTS.md, each criterion judged pass or fail with evidence.
- An evaluation: an agent with read access to the checkout grades AGENTS.md on the rubric, accuracy against the code
  included.
- It runs in CI on every PR, and `make merge-ready` refuses a PR where it did not pass.
- The evaluation is itself tested: labelled cases, the clean file and planted defects, each given its verdict in every
  run.
- One path for a local run and CI: `make agents-md-check`.

Non-goals:

- The size cap: a plain test (`tests/test_agents_md.py`, 20,480 bytes), not a criterion.
- A doc-gardening agent that rewrites AGENTS.md; the evaluation only reports.
- Grading `prime.md`, `--help` texts or nested AGENTS.md files.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

| Fact | Consequence |
|---|---|
| Codex reads at most 32 KiB of AGENTS.md files (`project_doc_max_bytes`) | The 20 KiB cap test; the rubric need not judge size |
| `claude -p --setting-sources ""` loads no CLAUDE.md, no settings and no hooks | The grader sees only the prompt's AGENTS.md, and pm's session-start hooks do not run in it |
| `claude -p --json-schema` returns `structured_output` beside `total_cost_usd` | A strict answer and its cost from one call |
| A Linux argument is at most 128 KiB | The prompt goes on stdin |
| GitHub Actions to the Anthropic API works through workload identity federation (sprint 23 Findings) | No stored API key; a fresh OIDC token per call, as the GitHub JWT lives 5 min |
| `release/merge_ready.py` requires every workflow with a `pull_request` trigger and no `paths` | The workflow has no paths filter; a conditional job skips itself instead |
| A stochastic grader false-fails a clean file on borderline readings: one call per criterion failed half the clean full runs (sprint 23 Findings), and one diff-mode call's finding in 1 of 5 chunks failed a PR that only added a golden file (run 38567955117) | The rubric fails only clear defects, every fail must quote AGENTS.md, every part in both modes takes a majority of 5 calls (3 agreeing), and the clean case must pass every run |
| A full-mode fail is mostly 3 of 3 calls agreeing on new AGENTS.md text, such as a rule a test already enforces restated (sprint 33 Findings): the check working, which a rerun gives again | The failure output says so: fix AGENTS.md as the agreed findings say; a rerun helps only after an error or a timeout |
| A stochastic grader also misses a planted defect that reads either way: `map_detail` and `accurate_wrong_behaviour` each passed 1 of 5 local runs | A planted case's text is unmistakable under its criterion; a case that misses gets clearer text, never a new label |

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### The rubric

`tests/agents_md/rubric.md`: a preamble every call gets (who reads AGENTS.md, what it may infer, how to judge), then
one `## <id>` section per criterion.

| Criterion | Fails on |
|---|---|
| `accurate` | A claim about pm (code, Make targets, files, flags, environment variables, tests, CI, numbers) the checkout refutes |
| `non_inferable` | A passage that restates the code, its doc comments or docstrings, the Makefile or a `--help` text; a Map row's one-line summary that points at its detail passes |
| `map` | Detail at a package doc's depth where a line and a pointer would do |
| `no_copies` | A copy of `prime.md` or the `CHANGELOG.md` preamble |
| `actionable` | A "don't" without the "do"; a directive with no command or check |
| `no_history` | A dated measurement, run result or decision trail |
| `consistent` | A contradiction within AGENTS.md, with `prime.md`, or with a check the code enforces |
| `complete` | A must-know rule of its list of 13 missing (merge-ready, no local full integration run, changelog entry files, tags, golden `-update`, `go-test-work UPDATE=1`, the integration marker, `PM_LAUNCHED`, one rule in three texts, labels from the rule, both hook functions, `[TEST]` needs, scratch clones) |

Judging rules, in the preamble: a fail quotes the AGENTS.md text verbatim and gives evidence (a `file:line`, or the
passage it copies); no evidence, no fail; only clear defects, a passage that reads either way passes; a rule of the
complete list is never a non_inferable, map or no_copies defect.

### The runner

`tests/agents_md/evaluate.py`, stdlib only. Each check is one call:

`claude -p --model M --setting-sources "" --strict-mcp-config --tools Read,Grep,Glob --allowedTools Read,Grep,Glob
--disallowedTools Bash,Edit,Write --no-session-persistence --output-format json --json-schema S`, from the checkout
root, the prompt on stdin, in a process group of its own killed whole at the timeout (240 s).

| Mode | When | Calls |
|---|---|---|
| full | `--mode full`, or `auto` when the diff since the merge base changes AGENTS.md or the grader (`GRADER`: `tests/agents_md/`, the live test, the workflow), since a changed grader must still pass the real AGENTS.md | Parts: each criterion on the whole file; `accurate` on each `## ` section of AGENTS.md (the whole file as context); `no_copies` and `consistent` get `prime.md` (and the preamble) in the prompt. 5 calls per part (the vote): 90 calls on an AGENTS.md of 11 sections |
| diff | `--mode diff --base REF`, or `auto` otherwise | Parts: `accurate` (does the diff make an AGENTS.md claim false?) and `complete` (does it add a rule AGENTS.md lacks?) on each chunk of `git diff <merge-base>...HEAD`, cut at file boundaries at 60,000 characters; testdata, `uv.lock`, `go.sum` and the work-test seconds file left out. 5 calls per part (the vote): 10 per chunk. An empty diff makes no call and passes |

All calls of a run go at once (`--jobs`, 16). The answer schema is `{"findings": [{"quote", "problem", "evidence"}]}`.

The vote is on passages. A finding's lines are the lines of the graded AGENTS.md its quote is on: each part between
ellipses, compared without Markdown marks and spacing, at every place it occurs. The calls on one part (a criterion
on one section, on the whole file, or on one diff chunk) vote: a finding is agreed when a majority of the part's
calls (3 of 5) each report a finding whose lines intersect its lines, its own call counted, and the part fails on an
agreed finding. So calls that each flag a different passage do not fail the part; three that flag one passage do.
A `complete` finding names a missing rule and quotes nothing, so it has no lines; two such findings agree, so
`complete` fails a part when a majority of its calls each report a missing rule. A criterion fails when one of its
parts fails. The table prints every call's findings with their lines, a finding no majority agrees with marked
`outvoted`, and per criterion the count of calls with findings and of agreed findings; `--out` keeps each finding's
lines. Under the table, a run that failed on agreed findings prints one line naming the criteria and what to do: fix
AGENTS.md as the findings say (cut text that restates what the code, a test or a lint already says; correct a false
claim; add a missing rule), since a rerun will not change agreed findings. A run that failed on an error or a timeout
prints that a rerun may help.

An answer is malformed, and the run fails, when it is not JSON, has no structured output, has a finding without
quote or evidence, or quotes text that is not in AGENTS.md (compared without Markdown marks and spacing, ellipses
splitting a quote; ellipses alone quote nothing); a complete finding has an empty quote. A timed-out, failed or malformed call is no vote: it fails
the run. Never a retry: the number of calls is fixed before the run, so a verdict means the same thing every time. The run prints a table of criteria, findings and errors,
writes every call's answer, seconds and cost to `--out`, and exits 1 on anything but every criterion passing.
`--agents-md PATH` grades another file against the same checkout; `--criteria` narrows full mode.

Credentials: with `ACTIONS_ID_TOKEN_REQUEST_URL` set, each call first fetches a GitHub OIDC token for the audience
`https://api.anthropic.com` and writes it atomically to `$ANTHROPIC_IDENTITY_TOKEN_FILE`, and the call runs without
`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` or `CLAUDE_CODE_OAUTH_TOKEN`. Elsewhere it runs on the user's own login.

### The labelled cases

`tests/agents_md/cases.py`: the clean case (the checkout as it is: every criterion passes), and planted defects, each
a set of text edits that fail hard when the text they look for is gone. A case is materialized as a git repo holding
the checkout's tracked files, committed, then its edits committed on top, so a diff-mode case reviews a real diff.

| Case | Edit | Must fail |
|---|---|---|
| `accurate_unknown_target` | A `make test-smoke` that does not exist, under Tests | accurate |
| `accurate_wrong_behaviour` | "`make lint` runs only `ruff check`", which the Makefile's `lint` prerequisites refute | accurate |
| `non_inferable_tour` | A file-by-file tour of `internal/work` | non_inferable |
| `map_detail` | A subsection on the work store host: its socket and clients, then a table of every stored procedure with its arguments, bound and effect | map |
| `no_copies_bodies` | `prime.md`'s Bodies paragraph, as `prime.md` has it | no_copies |
| `actionable_dont` | "Don't break the hooks." | actionable |
| `no_history_dated` | A dated judge eval result | no_history |
| `consistent_merge` | "Merge a PR with `gh pr merge <n> --squash` directly" | consistent |
| `complete_merge_ready`, `complete_pm_launched` | Every line naming merge-ready, or `PM_LAUNCHED`, deleted | complete |
| `diff_renamed_target` | Diff mode: the Makefile's `merge-ready` renamed | accurate |
| `diff_neutral` | Diff mode: a docstring reworded | nothing |

A planted run counts as failing only when its criterion fails on an agreed finding that names the marker or quotes a
line the case added; only that criterion runs. Labels come from the rubric, never from the answers: a miss is a rubric or
prompt change, or a fix to AGENTS.md when the clean case fails on a real defect.

`make test-live-agents-md` (`tests/test_agents_md_live.py`, `PM_LIVE_TESTS=1`) runs every case `PM_LIVE_RUNS` times
(3) on each of `PM_AGENTS_MD_MODELS` (default: the runner's model), every call in one pool, and requires every run to
give its case's verdict; it prints pass counts, per-call and per-run latency (median, p90, max) and cost per run.
`PM_AGENTS_MD_CASES` runs only the cases it names, `PM_AGENTS_MD_SKIP` leaves out the ones it names.
`tests/test_agents_md_eval.py` tests the runner offline against a fake `claude`.

### The model

Haiku, the cheapest model that gave every case its verdict in every run; Sonnet false-failed the clean file. The
numbers are in sprint 23's Findings.

### CI

`.github/workflows/agents-md.yml`, `on: pull_request` without paths, `id-token: write`:

- `check`: checkout with full history, Claude Code from its native installer, `make agents-md-check
  BASE=origin/<base>` (auto mode: full on a change to AGENTS.md or the grader), the answers uploaded as an artifact.
  30 min job timeout: 90 calls on an AGENTS.md of 11 sections, 16 at a time, each killed at 240 s, end within
  6 × 240 s = 24 min.
- `cases`: when the PR changes the grader (its grep names `GRADER`'s paths, which `tests/test_agents_md_eval.py`
  holds equal), the planted and diff cases once each (`PM_LIVE_RUNS=1`, `PM_AGENTS_MD_SKIP=clean`); otherwise a step
  says so and the job passes. The harness files the live test runs under (`conftest.py`, `run.py`, the Makefile) do
  not run it: the light suite covers them. The clean case is left out because `check` grades the same commit's
  AGENTS.md in full mode on every grader change, so it would only repeat that verdict; a local
  `make test-live-agents-md` keeps it.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Not adopted because |
|---|---|
| A reference lint (every path, target and flag AGENTS.md names exists) | Sees names, not behaviour: "make lint runs go-build first" passes it. Replaced by the owner's decision (sprint 23) |
| A number guard (numbers in AGENTS.md must match the code) | Same blind spot, and most stale numbers were run results that belong in Findings, which no_history catches |
| A duplication check against `prime.md` (shared n-grams) | Misses a copy reworded; the no_copies criterion judges the rule, not the words |
| `anthropics/claude-code-action` in CI | Two paths, the action's in CI and a script locally; the runner is one path for `make` and CI |
| The Agent SDK (Python) | A dependency and a second agent loop for what `claude -p` with a schema already does |
| One call grading every criterion | Long, unfocused calls; one per criterion keeps each call under the timeout and its answer about one thing |
| Retrying a failed or malformed call | A retry changes what a verdict means; a flaky grader shows in the labelled cases instead |
| One call per part, failing on any finding | Measured on the clean file: half the full runs false-failed (map, non_inferable, accurate); in diff mode one call's finding in 1 of 5 chunks failed a PR |
| A majority of 3 calls (2 agreeing) | Scattered noise at a rate p per call on a passage stands at about 3p² there, against 10p³ for 3 of 5: on the clean file 1 of 10 full runs and 1 of 5 clean cases false-failed when 2 of 3 non_inferable calls flagged one line (sprint 33 Findings) |
| `complete` left single-call | It did not false-fail in its measurement (0 of 14 runs), but one rule for every part is simpler, and its vote costs 4 calls of 90 |
| `complete` findings agreeing only on the same rule | Telling two wordings of one missing rule apart is a semantic match no line test gives; complete did not false-fail on a single call, so any-rule agreement is enough |
| The clean case in CI's `cases` job | It grades the commit's own AGENTS.md, as `check` does in full mode on the same change, so it only repeated `check`'s verdict and failed the job on its noise |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

| Source | Taken |
|---|---|
| Anthropic's Claude Code memory docs and best practices | Keep CLAUDE.md concise and specific to what the agent cannot discover; commands and rules, not prose |
| OpenAI, "Harness engineering" (Codex) | AGENTS.md as a table of contents into deeper docs, not an encyclopedia; rules held by mechanical checks |
| Grok Build's project-rules guide | Short, repo-specific rules; detail in the referenced files |
| Gloaguen et al., "Evaluating AGENTS.md" (arXiv 2602.11988) | Context files that restate the repository tend to lower task success and raise cost by over 20%; agents follow their instructions, so unneeded ones cost. Hence non_inferable and map |
| Augment, "How to write good AGENTS.md files" | Top files ran about 100-150 lines plus focused reference docs; warnings without a concrete alternative underperformed. Hence map and actionable |
| pm's owner-request judge eval (`make test-live`) | Labelled cases with verdicts from the rule, every run of every case must agree, latency printed |

## Open questions

> What is still unresolved?

- A full run's slowest accuracy call has reached 149 s of its 240 s timeout; a longer AGENTS.md section may need a
  finer split.
- How Claude Code behaves when a call outlives the 10 min federated token (not reached at 240 s per call).
- Residual variance under the vote: a passage that three of five calls flag on a borderline reading still fails a
  clean file; fixing the passages that draw repeated findings lowers it.
