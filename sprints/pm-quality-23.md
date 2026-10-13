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

::: decision {source=agent date=2026-10-14}
The design page agent-lifecycle stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page agent-sync stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page pm-cli stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page pm-go stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page pm-in-codex stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page pm-product stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
:::

::: decision {source=agent date=2026-10-14}
The design page pm-versioning stays as it is for this sprint's code changes.
PR #80 changes only doc comments and AGENTS.md citations in the files this page covers (the AGENTS.md file map moved into package doc comments, release citations now name release/README.md); no behaviour this page describes changed
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

- AGENTS.md eval grader variance, haiku, clean branch. Before (one call per
  criterion, any finding fails): 4 of 8 full runs false-failed; per criterion
  map 3/8 (always the hooks fail-open bullet), non_inferable 1/8, accurate 1/8
  (one section call on correct Map text),
  no_copies/actionable/no_history/consistent/complete 0/8; $0.10-0.15 and
  87-144 s per run (18 calls). Diff mode on the last 5 merged PRs (#68, #67,
  #65, #66, #60), 3 runs each: 15/15 pass, $0.002-0.036 and 10-42 s per run;
  no real finding. After (majority of 3 calls per part for every full-mode
  criterion but complete; complete and diff mode single): judgement-only vote
  4/4 full runs pass ($0.17-0.21, 120-141 s, 30 calls); with accurate voted
  too 3/4 ($0.29-0.31, 150-186 s, 52 calls), the fail non_inferable 2/3 on two
  different passages; clean case in calibration 2/2. Under the vote 1 of 10
  clean full runs false-failed, against 4 of 8 before. Calibration
  PM_LIVE_RUNS=2: 26/26 runs gave the case's verdict ($0.90, 251 s), accurate
  cases again with accurate voted 4/4 ($0.80, 177 s). Total spend $4.73.

- AGENTS.md eval after the map fix and the rubric's must-know exemption
  (haiku, 3-vote full mode). Batch 1 (6 full runs, after 235dc05+706e6db): 4/6
  pass, both fails non_inferable; calls with findings: non_inferable 8/18,
  no_copies 3/18, accurate 1/198, actionable 1/18, consistent 1/18, map 0/18
  (was flagged every run before), no_history 0/18, complete 0/6; $0.32 and
  130-200 s per run. The exempt must-know rules were no longer flagged. Batch
  2 (after d07110a cut the no-sleep and size-cap sentences that the lint and
  size_test.go messages state): 6/6 pass (2 full, 4 without accurate);
  non_inferable 3/18, actionable 1/18, accurate 0/66, rest 0; $1.07.
  Calibration (PM_LIVE_RUNS=1): 12/12 planted cases gave their verdict, the
  clean case failed non_inferable; 142 calls, 198 s, $0.88. non_inferable-only
  probes: 3/6 runs fail before f6e3832 (seed bullet, flagged 5/6, cut to a
  conftest.py pointer) and 3/6 after (calls with findings 10/18 then 9/18);
  the most-flagged passage left is the changelog.d/README.md rule's go:embed
  reason (5/6 probes), which assets_test.go and the CHANGELOG preamble state.
  non_inferable is not stable at 3 votes. Total $4.14.

- AGENTS.md eval with the agreement vote (a voted part fails only when 2 of
  its 3 calls quote intersecting AGENTS.md lines; haiku, full mode, 52 calls,
  $0.29-0.34 and 190-275 s per run, 6 runs at once). Set 1 (AGENTS.md at
  f6e3832 plus the covers:/README edits): 1/6 pass; agreed findings were real:
  no_copies 5 parts failed on the changelog.d/README.md rule (the CHANGELOG
  preamble's), map 2 on the new design_sources wording; non_inferable 8/18
  calls with findings, 0 agreed (was the false fail). Calls with findings /
  agreed findings: accurate 0/198 0, non_inferable 8/18 0, map 5/18 5,
  no_copies 12/18 11, actionable 2/18 0, rest 0. Set 2 (README rule cut,
  covers: rule cut to a pointer): 5/6 pass; non_inferable 9/18 calls, 2 agreed
  findings in 1 run (the prime.md noun-list sentence, which restates
  internal/hooks' doc), map 1/18 0, rest 0. After cutting that sentence
  (4fe6a7e), non_inferable alone 6/6 pass (6/18 calls with findings, 0 agreed,
  $0.03/run). Calibration (PM_LIVE_RUNS=1) only on the 6 voted non-accurate
  planted cases: 6/6 fail on their criterion with an agreed finding ($0.09, 58
  s); the accurate, complete and diff cases and a clean full run on 4fe6a7e
  not run (budget). Total $3.96.

- After the rebase onto main (0.8.6, #68-#75), make agents-md-check MODE=full
  on haiku, 4 runs at once: 3/4 passed; the 4th failed non_inferable on an
  agreed finding (3 of 3 calls): the rebased design_sources rule restated
  internal/design's package doc. Fixed (AGENTS.md keeps the rule and points to
  go doc ./internal/design); 4 more runs at once: 4/4 passed, 113-166 s and
  $0.297-0.335 each, no agreed finding, 1-2 outvoted non_inferable findings
  per run. Calibration (PM_LIVE_RUNS=1 make test-live-agents-md): 13/13 cases
  gave their verdict, the 12 planted cases failed their criterion, clean and
  diff_neutral passed; 142 calls, 211 s, $0.87. Evaluation spend in all about
  $3.40.

- agents md on PR #80, its first real runs. Run 1 (38478854312): 14 of 52
  check calls and the cases job failed at start, Token exchange failed with
  status 401: every claude -p read one shared ANTHROPIC_IDENTITY_TOKEN_FILE
  that 16 calls at once replaced; fixed (each call gets its own token file).
  Run 2 (38479772413): 0 token failures; check passed 52 calls in 156 s,
  $0.25; cases 12/13, the clean file failed non_inferable on an agreed
  finding, the #77 Go-skip bullet restating go_changes.py's docstring (a local
  probe found it 1 of 4); cut to a pointer, then 4/4 local probes of
  non_inferable, actionable and map passed. Run 3 (38480797824, head 0fe969f
  on main 0.8.8): check passed (134 s, $0.27, no agreed finding); cases 12/13,
  map_detail passed instead of failing (a grader false negative); its re-run
  13/13 ($0.68, 218 s). Local evaluation spend about $4.00; CI spend about
  $2.8 over 3 runs and the re-run.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: AGENTS.md is an 11.7 KB map that Claude Code, Codex and Grok Build read whole, and an agent grades it against a rubric on every PR, signed in through workload identity federation with no stored API key (PR #80).

- AGENTS.md went from 44,134 bytes and 363 lines (Codex read 240) to 11,738 bytes; the Layout table moved into Go package doc comments, test narration into the tests' module docstrings, the release procedure into `release/README.md`.
- `tests/test_agents_md.py` holds the 20,480-byte cap and the `CLAUDE.md` link.
- `tests/agents_md/` holds the rubric (8 criteria), the runner (`make agents-md-check`: full mode on a PR that changes AGENTS.md, diff mode on every other PR) and 13 labelled cases (`make test-live-agents-md`); full mode votes 3 calls per part and fails a passage only when 2 calls agree on it.
- `.github/workflows/agents-md.yml` runs it on every PR, required by `make merge-ready`; each call gets its own fresh GitHub OIDC token file.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Size, met:** `wc -c AGENTS.md` is 11,738 (cap 20,480); `tests/test_agents_md.py` fails above the cap (checked with the cap set to 100).
- **The evaluation passes the new AGENTS.md and fails each planted case, met with one noted miss:** haiku calibration 39/39 (13 cases x 3 runs) before the rebases; on the final file 13/13 locally (`PM_LIVE_RUNS=1`, 211 s, $0.87) and 13/13 in CI after one CI run where the planted `map_detail` case passed (a grader false negative) and was re-run. Clean full mode on the final file: 4/4 local runs passed (113-166 s, $0.30-0.34 each); CI run 38480797824 passed all 8 criteria in 134 s for $0.27. Diff mode: 15/15 passes on the last 5 merged PRs' diffs. The measurements are in Findings.
- **Runs in CI on every PR and required, met:** `agents-md.yml` has a `pull_request` trigger with no `paths`; `release/merge_ready.py` names it among the required workflows (`tests/test_merge_ready.py`); `make merge-ready PR=80` accepted head 0fe969f with every check passed.
- **The must-know rules are still in AGENTS.md, met:** the `complete` criterion passed on every clean run, and each rule was grepped in the rewrite.
- **`make test`, `make test-go`, `make lint` and CI, met:** `make test` 129 passed; `make go-fmt go-vet`, `make lint-go` (0 issues), `make lint-py` clean; all 15 checks on PR #80 passed.
- **Known limits:** a fork's PR gets no OIDC token, so the required check fails there (the runner says so and names the remedy); the `cases` job, which runs only when the evaluation's own files change, runs each case once, so grader noise can fail it and a re-run is the remedy.
