---
type: sprint
title: "pm search retrieval model 2: rank whole records, re-run the eval against rg"
bead: yeeef-agents-9va.139
---

## Goal

> What should be true when this sprint ends, and why now?

`pm search` beats `rg` at finding the earlier records a sprint should cite, so PR #112 can merge on evidence. The sprint 128 eval showed PR #112's unit-and-AND ranking beats `rg` only with three query wordings and loses on the extended labels; whole-record BM25 OR led every recall measure. This sprint builds retrieval model 2 from the [Search index](../design/search-index.md) design and re-runs that eval as its gate. PR #112 is held until it passes (sprint 126 decision).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The offline retrieval eval as a checked-in harness, run on demand: the labels (strict and extended), the model-written queries and the corpus commit frozen beside it; `pm search` and the `rg` variants; recall@5, recall@10, MRR, nDCG@10, zero-result rate, tokens to the first relevant record, paired bootstrap 95% CIs.
- Retrieval model 2 on PR #112's branch: rank records and items with BM25 OR, point each hit at its best unit, the all-terms / some-terms header; phrase and path terms stay filters.
- The design pages updated to the shipped model, and the live agent eval's third arm (project decision) on the Search index page.

**Out:**
- Running the live agent eval; the index, persistence and semantic stages (2-5).

## Done when

> What evidence will show the goal is met?

- The eval harness runs from one command on the frozen inputs and reproduces sprint 128's stage 0 numbers within their CIs (strict single-query R@10 .314 for `pm search`, .301 for `rg`).
- Model 2 meets the design page's gate on that harness: recall@10 and MRR CIs against stage 0 not below 0, and against the best `rg` variant above 0 on the strict set, best of 3 queries; extended-set numbers recorded in Findings either way.
- Go tests cover model 2's ranking, `--kind`, `--project`, `-n` and the no-match line; `make test` passes; one query stays under 1 s on this store.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-25}
Model 2 and the eval harness land on PR #112 branch pm-search, rebased onto main, so one PR delivers sprints 126 and 129.
Model 2 rewrites code that is not on main yet, and PR #112 merges only once this sprint eval passes; a stacked PR or a dependency either way would deadlock the two closes.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Eval harness (tests/search_eval, make search-eval; commit cdffb71) on
  records commit 41db306c, ripgrep 15.1.0, model queries frozen in
  queries.json (3 per sprint, 1-4 words). Labels rebuilt from the records'
  citations: strict 73 sprints / 95 targets, extended 111 / 260, against
  sprint 128's 68/87 and 98/237: this extractor also reads item ids (a task id
  cites its sprint) and 'sprint N' with a project name before it, and dates a
  design page by the first sprint that links it, since the records branch
  starts with a bulk move. Stage 0 (83c3785) reproduces sprint 128 within its
  95% CIs: strict single-query R@10 pm search .282 [.214, .355] (128: .314),
  rg AND by match count .326 [.251, .406] (128: .301); best of 3 .509 [.404,
  .616] (.551) and .539 [.429, .648] (.449). Whole-record BM25 OR does not:
  .521 [.443, .598] single, .767 [.671, .849] best of 3, against 128's .368
  and .588; this harness filters every system to the as-of corpus, which 128's
  baseline may not have. A variant 128 did not report, rg AND newest first, is
  the best rg on the strict set: .427 single, .692 R@10 and .505 MRR best of
  3; stage 0 trails it by -0.183 [-0.290, -0.075] R@10 best of 3.

- Gate FAILS. make search-eval SEARCH_EVAL_PM='stage0=83c3785 model2=fcd1631'
  (commit 133ac98 harness, records 41db306c, ripgrep 15.1.0, 10,000 paired
  bootstrap resamples over sprints). Strict set (73 sprints, 95 targets), best
  of 3 queries: model 2 R@10 .705 [.603, .801], MRR .531; against stage 0
  +0.196 [+0.116, +0.285] R@10 and +0.114 [+0.045, +0.182] MRR (pass: not
  below 0); against the best rg variant, rg AND newest first (R@10 .692, MRR
  .505), +0.014 [-0.089, +0.116] R@10 and +0.026 [-0.079, +0.133] MRR (fail:
  not above 0). Against sprint 128's rg (AND by match count) it passes: +0.167
  [+0.082, +0.260] R@10, +0.221 [+0.146, +0.300] MRR. Whole-record BM25 OR
  over records only leads model 2 by 0.062 [0.014, 0.123] R@10 best of 3, and
  itself would not pass against rg AND newest first: +0.075 [-0.021, +0.178]
  R@10, +0.058 [-0.045, +0.162] MRR. Most strict targets (83 of 95) are
  earlier sprints a frame cites as 'sprint N', usually the latest ones, which
  is the order newest first gives.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
