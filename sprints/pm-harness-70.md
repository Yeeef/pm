---
type: sprint
title: pm decision need takes its parts as flags
bead: yeeef-agents-9va.79
---

## Goal

> What should be true when this sprint ends, and why now?

`pm decision need` takes the question, facts, options with their costs and the default as flags, so an agent can raise a well-formed need from `--help` alone. Today it parses stdin lines that start with `Question:`, `Fact:`, `Option <label>:`, `Cost:` and `Default:`, a layout that only `pm prime` states in full. A flag per part makes the form structural: argparse refuses a missing part, and an option cannot be given without its cost.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `--question`, repeatable `--fact`, repeatable `--option LABEL TEXT COST`, `--default LABEL REASON` on `pm decision need`; the command no longer reads stdin; the same Markdown description in Beads; the existing checks (two options or more, unique labels, a default that names an option, the 25-word sentence limit); `--help`, `prime.md`, the design page and tests.

**Out:** `pm action need` and `pm decision close`, which keep stdin; `pm decision add` (PR #70, sprint 67); needs already raised.

## Done when

> What evidence will show the goal is met?

- `uv run --project pm pm decision need --help` shows the four flags, and `prime.md` names them; no text in `src/pm/` tells an agent to give a decision need on stdin.
- A test raises a need with the flags and checks the exact Markdown description; a test shows each malformed form (one option, an option missing its cost, a default naming no option, a duplicate label, a multi-line value, a sentence over the limit) is refused with nothing written.
- `make test` passes, and the PR's CI is green.

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

- pm decision need takes --question, repeatable --fact, repeatable --option
  LABEL TEXT with one --cost LABEL TEXT each, and --default LABEL REASON, and
  reads no stdin; the Markdown in Beads keeps its layout. The cost is its own
  flag keyed by label, not a third --option value, because the design page had
  rejected flags for positional mix-ups. make test: 76 passed, 35 skipped; the
  2 integration tests that raise needs pass; PR #71 CI: light, integration and
  guard pass. The 9 refusal cases each exit non-zero with no bd write.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm decision need` takes its question, facts, options with their costs and its default as flags, and reads no stdin, so the form comes from `--help` and argparse.

Merged as bcd770c (PR #71).

- PR #71: `--question`, repeatable `--fact`, repeatable `--option LABEL TEXT` with one `--cost LABEL TEXT` each, and `--default LABEL REASON`. The Markdown in Beads keeps its layout.
- A cost names its option by label, so a cost cannot land on the wrong option. Values go in single quotes; pm cannot detect a `code span` the shell has already run in double quotes.
- The design pages [Decision need layout](../design/decision-need-layout.md) and [pm CLI](../design/pm-cli.md) describe the flags.
- Agents in other repos get the flags at the next pm release; repos pinned to 0.1.2 keep the stdin form until then.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** `pm decision need --help` shows `--question`, `--fact`, `--option`, `--cost` and `--default`; `prime.md` names them. A grep of `src/pm/` for `Question:`, `Fact:`, `Option <label>` and "one part per line" finds only the `**Question:**` heading of the written Markdown. The owner-request hook message gives the flag form.
- **Met:** `test_decision_need_writes_its_flags_in_the_one_layout` gives the design page's example as flags and checks the exact Markdown. `test_decision_need_refuses_a_malformed_part_and_writes_nothing` has 11 cases: a missing `--question`, one option, an option without a cost, a cost naming no option, a second cost, a duplicate label, a default naming no option, a multi-line fact, a 26-word sentence, a second `--default`, and a PR-review ask that gets the review-form refusal first. None of them writes to Beads.
- **Met:** `make test`: 78 passed, 35 skipped. The 2 integration tests that raise needs pass. PR #71 CI on 1737b34: the light, integration and guard jobs pass.
