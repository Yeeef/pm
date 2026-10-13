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

- [AGENTS.md rubric and its agentic evaluation](../design/agents-md-evaluation.md): the rubric's criteria, the runner's modes and calls, the labelled cases, the model, and the CI workflow.

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

- Anthropic workload identity federation works end to end from a GitHub
  Actions pull_request run of Yeeef/pm, first try
  (https://github.com/Yeeef/pm/actions/runs/38459842702, draft PR #64,
  closed): GitHub OIDC token sub
  repo:Yeeef@34886010/pm@1411907115:pull_request, aud
  https://api.anthropic.com; exchange at /v1/oauth/token HTTP 200, token_type
  Bearer, expires_in 598 s, scope workspace:developer; Claude Code 2.1.296
  from the native installer (curl https://claude.ai/install.sh | bash, 11 s);
  claude -p --model haiku with ANTHROPIC_FEDERATION_RULE_ID, _ORGANIZATION_ID,
  _SERVICE_ACCOUNT_ID, _WORKSPACE_ID and ANTHROPIC_IDENTITY_TOKEN_FILE set, no
  ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN or CLAUDE_CODE_OAUTH_TOKEN, no
  --bare, exited 0 in 2.6 s with output WIF OK. No token appears in the job
  log. The OIDC token was fetched fresh right before claude -p; how claude
  behaves past the GitHub JWT's 5 min lifetime in a longer run was not tested.

- AGENTS.md evaluation calibrated on its 13 labelled cases (make
  test-live-agents-md, PM_LIVE_RUNS=3, AGENTS.md at 06ef57c, 11.6 KB, 24 calls
  at a time, 2026-10-13): haiku 39/39 runs gave the case's verdict, the clean
  file passing all 8 criteria 3/3 and each of the 12 planted defects failing
  on its criterion with a finding on the planted text 3/3; 156 calls in 165 s,
  per call median 15 s, p90 30 s, max 149 s (timeout 240 s); a full-mode run
  of the clean file is 18 calls, slowest call median 91 s, max 149 s, $0.12;
  all 156 calls $0.82. sonnet 38/39 (the clean file failed non_inferable in 1
  run of 3), per call median 13 s, max 37 s, clean run $0.99, all calls $8.17.
  Default model: haiku, the cheaper one and the only one that held every case.
  Before the calibration, haiku's first runs on the rewritten AGENTS.md
  (a0410a4) failed it on real defects, fixed in AGENTS.md, never in a label: 4
  claims the code refutes (every --help text in commands.go; merge-ready
  refusing every head without origin/main, ignoring the release-commit
  exception; every merge releasing, ignoring release/hold; every hook failing
  open, while the owner-request judge fails hard), passages copying prime.md
  (design-page rule, records branch, covers rule, style.css) and the CHANGELOG
  preamble, Make target rows restating the Makefile, and a claim that CI runs
  the evaluation before agents-md.yml existed. One clean run of three still
  failed on borderline readings until the rubric asked for clear defects only
  (a passage that reads either way passes); 4 clean runs then passed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
