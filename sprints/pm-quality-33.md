---
type: sprint
title: The AGENTS.md check gives the same verdict on every rerun
bead: pm-d2k5.33
---

## Goal

> What should be true when this sprint ends, and why now?

The `agents md` workflow's grading of AGENTS.md is deterministic enough to gate PRs: a rerun of the same commit gives the same verdict. Now a PR either fails until it is rerun, or teaches agents to rerun past a red check. Over its last 27 completed runs, it failed 6 times, passed 18 times and was cancelled 3 times. Failures hit 5 branches (agents-md-map, refactor/cli-nouns, cheaper-tests twice, ask-tool-on-site). PR #85 (test-select) passed only on attempt 3. Sampled failure: run 38567955117, where the model grader's `accurate` criterion failed on 1 of 5 calls ("1 with findings, 1 agreed"), for a diff that only added a golden file.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Measure the flake rate: rerun the check N times on fixed commits, both ones that should pass and ones that should fail. Get each criterion's pass rate per call, and its variance.
- Find the root cause:
  - grader nondeterminism (model, sampling, the prompt);
  - the agreement and voting rule ("N calls, M with findings, K agreed");
  - findings that judge the diff instead of AGENTS.md;
  - the pinned Claude Code version;
  - timeouts.
- Fix it so verdicts are stable, without lowering what the check catches. The labelled cases (`make test-live-agents-md`) must still pass, and a known-bad AGENTS.md must still fail.
- The agents-md design page is updated to the new state.

**Out:**
- Removing the check, or making it non-blocking, without an owner decision.
- AGENTS.md content changes beyond what a fix needs.

## Done when

> What evidence will show the goal is met?

- On 3 fixed known-good commits, 10 reruns each pass 30 of 30.
- On a known-bad AGENTS.md, 10 reruns fail 10 of 10.
- The labelled cases pass.
- The per-criterion pass rates, before and after, are in Findings.

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

- Triage of the 6 failed agents md runs and PR #85's 3 attempts (2026-10-14,
  job logs): (1) 38478854312 agents-md-map: token exchange 401, the shared
  token file race fixed in PR #80; (2) 38479772413 agents-md-map: a real
  AGENTS.md defect, fixed in PR #80; (3) 38527207760 refactor/cli-nouns and
  (4) 38530785297 cheaper-tests: check in full mode failed non_inferable (and
  map in 3) with 3 of 3 calls agreeing, on new AGENTS.md text that restates a
  rule a test enforces (e.g. the doc_test.go func-comment rule): consistent
  verdicts on the PR's own edit, not rerun noise; (5) 38532391506
  cheaper-tests: accurate 2 agreed plus cases' clean case non_inferable; (6)
  38567955117 ask-tool-on-site: diff mode, where each chunk gets one unvoted
  call (5 chunks, 10 calls), so one finding fails the run. PR #85
  (test-select): check passed on all 3 attempts; cases failed attempts 1 and
  2: planted map_detail passed (grader false negative) both times, and the
  clean case failed non_inferable once. Rerun noise is in cases and in diff
  mode's single calls; the clean case grades the branch's own AGENTS.md, so it
  repeats check's verdict on an AGENTS.md edit; cases now runs on every PR
  touching tests/conftest.py, tests/run.py or the Makefile.

- Planted cases, 5 local haiku runs each (branch agents-md-stable): map_detail
  4/5 with the old text, 5/5 with the new (a host-procedure table);
  accurate_wrong_behaviour 4/5 old ('make lint runs make go-build first',
  blurred by the Makefile's comment on shared compiles), 5/5 new ('make lint
  runs only ruff check'); complete_merge_ready, complete_pm_launched,
  diff_renamed_target and diff_neutral 5/5 each under the 3-call vote; every
  other planted case 5/5. Smoke on the fixed grader: full mode pass (54 calls,
  135 s, $0.35); diff mode on PR #85's merge pass (6 calls, 21 s, $0.02); diff
  mode on ask-tool-on-site head de575a9, the 5-chunk diff run 38567955117
  failed on, pass (30 calls, 51 s, $0.24).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
