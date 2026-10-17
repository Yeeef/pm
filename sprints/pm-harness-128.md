---
type: sprint
title: "Search index RFC: pm search at scale"
bead: yeeef-agents-9va.138
---

## Goal

> What should be true when this sprint ends, and why now?

A design for `pm search` that stays fast and exact as the records and the work store grow, with measured performance and resource costs, so the build-or-wait call rests on numbers rather than guesses. Today every run rebuilds the index from a full scan ([pm search](../design/pm-search.md)). That costs about 0.1 s at 2 MB and grows linearly. This sprint writes the RFC, a design page with the scan as its first step, and builds no product code.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A design page for a persistent, incrementally updated search index: where it lives, how it stays exact against the records branch and the work store, the data structures, the query path, persistence and restart, drafts workspaces, and a semantic (hybrid) layer as a later stage.
- Measurements from throwaway prototypes in a scratch directory: scan versus index at 1×, 10× and 100× this store (build time, memory, on-disk size, query latency, cost of an incremental update), and the store's growth rate from history.
- Stages from today's scan to the full design, each with the measured threshold that triggers it.
- How `pm search` is judged against `rg`: an offline retrieval eval, with labels taken from the records' own cross-references and queries written by a model, run now on this store for `pm search` and `rg`; and the design of a live agent A/B eval.

**Out:**
- Product code, and any change to `pm search`'s output or semantics.
- Choosing an embedding model or provider.

## Done when

> What evidence will show the goal is met?

- The design page exists, `pm check` passes, and every number on it comes from a measurement in Findings, with the command, the machine, and the corpus size.
- The retrieval eval's numbers for `pm search` and `rg` (recall@5, recall@10, MRR, and output tokens until the first relevant hit) are in Findings, with the label set's size and how it was built.
- The page gives, for each stage, the corpus size or latency that triggers it, and the memory and disk it costs at 10× and 100×.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-17}
The sprint also designs and runs an evaluation of pm search against rg.
The owner asked how pm search beats rg; the index is worth building only if search beats grep.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Retrieval eval, pm search (PR #112 build) vs rg 15.1.0 on this store at
  records commit 41db306c. Labels: 68 sprints whose Goal or Scope cite earlier
  records (87 targets: 73 sprints, 12 design pages, 2 docs), citing record
  excluded; 3 queries per sprint written by claude-haiku-5-5 from the title
  and the Goal's first sentence, links removed. Single query R@10: pm search
  .314, rg AND by match count .301, BM25 over whole records with OR .368;
  difference pm search minus rg +0.012 [-0.047, +0.071] (paired bootstrap
  95%). Best of 3 queries R@10: pm search .551, rg .449, whole-record BM25 OR
  .588; difference +0.103 [+0.007, +0.206].

- Same eval, extended labels (98 sprints, 237 targets, 162 design pages,
  adding each sprint's existing Design pages links): pm search trails rg,
  single query R@10 -0.072 [-0.139, -0.007], union of 3 -0.140 [-0.235,
  -0.045]. Misses are ranking, not matching: in 71 of 79 extended cases where
  rg's top 10 held the target, pm search matched it but ranked it below 10.
  Short sprint sections outrank a design page split into many units;
  all-terms-in-one-unit returns nothing for 26.5% of title-word queries
  against 16.2% for rg.

- Tokens an agent reads to the first relevant record (median, queries both
  found): pm search 189; rg file list 26 but with no content; rg -i -n lines
  of the files up to that hit 9,384 (no -m or head, an upper bound). Including
  or excluding Design pages sections changes no metric measurably (|delta| <=
  0.015).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
