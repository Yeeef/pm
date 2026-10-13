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
- Checks in CI: a size cap (16 KiB, leaving room for a user's own `~/.codex/AGENTS.md` under 32 KiB); a reference lint (every backticked path, Make target, test name, flag and env var in AGENTS.md exists); no hand-kept numbers the code holds unless a test checks them; no sentence shared with `prime.md` or the CHANGELOG preamble.

**Out:**
- A recurring doc-gardening agent.
- Nested per-directory AGENTS.md files.
- `prime.md` and the `--help` texts (pm's users' context, not pm's developers').

## Done when

> What evidence will show the goal is met?

- `wc -c AGENTS.md` is 16,384 or less, and a test fails above it.
- The reference lint passes on main, and fails on a planted stale path.
- Each rule an agent would get wrong untold (merge-ready, no local full integration run, changelog entry files, never move a tag, golden `-update`, `go-test-work UPDATE=1`, the integration marker, `PM_LAUNCHED`, one rule in three texts, labels from the rule, both hook functions, `[TEST]` needs, scratch clones for live checks) is still in AGENTS.md, or held by a check named there.
- `make test`, `make test-go` and `make lint` pass, and the PR's CI is green.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
