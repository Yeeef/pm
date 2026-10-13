---
type: sprint
title: "AGENTS.md: a map under the cap that every runtime reads in full, held by checks"
bead: pm-d2k5.23
---

## Goal

> What should be true when this sprint ends, and why now?

AGENTS.md (CLAUDE.md links to it) is a short map of what an agent developing pm cannot infer from the code: commands, non-obvious rules, pointers to deeper docs. Every runtime reads it whole, and CI keeps it that way. Why now: at 40,265 bytes it is over Codex's 32 KiB `project_doc_max_bytes` default, so Codex drops lines 252-337 (the hook-entry rules, The site, Live checks, Elsewhere); 6 of its claims are stale; about 55% restates the code or the tests; and the one check on it is the parity scenario count (`tests/test_ci.py`). The sources (Anthropic's CLAUDE.md docs and best practices, OpenAI's harness-engineering post and Codex docs, Grok Build's project-rules guide, the 2026 AGENTS.md studies) agree on: only what cannot be inferred, a table of contents rather than an encyclopedia, roughly 100-200 lines, rules enforced by tools rather than prose.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Fix the 6 stale claims (rule chunk sizes, judge case count, "every target runs go-build", the chunks' source, the projects named, `-j2`).
- Move the Layout map and the test narration next to their code (package doc comments, test module docstrings, a release doc), with AGENTS.md linking to them.
- Cut what copies `prime.md`, `--help` texts and the CHANGELOG preamble.
- A size cap in a test: 20 KiB, the owner's choice (room for the rules an agent would get wrong untold, still well under Codex's 32 KiB `project_doc_max_bytes`, which the AGENTS.md files from the repo root down to the working directory share; a user's `~/.codex/AGENTS.md` is outside it).
- A rubric for a good AGENTS.md, drawn from the sources, and an agentic evaluation against it: an agent with read access to the checkout grades AGENTS.md on each criterion (accuracy against the code included), with evidence. It runs in CI as a check every PR must pass.
- Labelled cases for the evaluation: the new AGENTS.md (passes) and copies with one planted defect per criterion (each fails on that criterion), so the evaluation is itself tested, as the owner-request judge is.

**Out:**
- A recurring doc-gardening agent.
- Nested per-directory AGENTS.md files.
- `prime.md` and the `--help` texts (pm's users' context, not pm's developers').

## Done when

> What evidence will show the goal is met?

- `wc -c AGENTS.md` is 20,480 or less, and a test fails above it.
- The evaluation passes the new AGENTS.md and fails each planted-defect case on its criterion, in every one of several runs per case; its pass counts and latency are in Findings.
- The evaluation runs in CI on every PR, and `make merge-ready` refuses a PR where it did not pass.
- Each rule an agent would get wrong untold (merge-ready, no local full integration run, changelog entry files, never move a tag, golden `-update`, `go-test-work UPDATE=1`, the integration marker, `PM_LAUNCHED`, one rule in three texts, labels from the rule, both hook functions, `[TEST]` needs, scratch clones for live checks) is still in AGENTS.md.
- `make test`, `make test-go` and `make lint` pass, and the PR's CI is green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
The AGENTS.md size cap is 20 KiB (20,480 bytes), not 16 KiB.
Owner chose it in chat: it leaves room for the rules an agent would get wrong untold and stays well under the 32 KiB Codex reads.
:::

::: decision {source=owner date=2026-10-13}
Of the proposed mechanical checks only the 20 KiB size cap stays; a rubric for a good AGENTS.md and an agentic evaluation against it, run in CI and required to pass, replace the reference lint, the number guard and the duplication check.
Owner chose it in chat: the defects that matter (stale behaviour claims, narration, copies) are semantic, which an agent reading the code can judge and a lint of names cannot.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
