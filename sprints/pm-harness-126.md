---
type: sprint
title: "pm search: find prior sprints, decisions and design sections by topic"
bead: yeeef-agents-9va.136
---

## Goal

> What should be true when this sprint ends, and why now?

An agent can find earlier sprints, decisions, design sections, docs and work items on a topic in one command, before it frames a sprint. Today it finds anything outside its project's drill-down only by thinking to grep `records/`, which returns lines with no status, date or link and misses work-store titles and dropped tasks. Design: [Work context](../design/work-context.md), pm search and Procedure. Split from sprint 125, which ships the design-page index.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm search <terms…> [--kind] [--project] [-n]`: BM25 over record sections, decision blocks, design `###` sections, docs, postmortems and work-store task and need titles, one capped line per hit, as the design page gives.
- `prime.md`: the open-a-sprint clause (search before writing the frame); `--help` text and golden; the new files in the design page's `covers:`.

**Out:**
- The design-page index (sprint 125).
- Marking decisions superseded; semantic search; a persistent index; searching a worktree's drafts workspace.

## Done when

> What evidence will show the goal is met?

- Go tests: a seeded unit holding the query terms ranks first; `--kind` and `--project` filter; output stops at `-n`; no hit prints the no-match line and exits 0.
- On this repo's store, `pm search` for three topics with a known prior sprint (claim history, design covers, owner-request judge) lists that sprint's record or decision in its top 5, and one run takes under 1 s (wall time recorded in Findings).
- The `prime.md` rules still fit the hook chunk cap (`tests/test_hooks.py` passes).

## Design pages

> Where is the detail?

- [Work context](../design/work-context.md): `pm search` and the open-a-sprint clause of the procedure
- [pm search](../design/pm-search.md): the units, tokenizer, match, ranking and output of `pm search`
- [pm CLI](../design/pm-cli.md): `search` in the State family

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
This sprint also adds the index line "prior work: pm search <topic>" that sprint 125 left out until search shipped.
The pm search design page puts it in the release that ships the command; sprint 125 kept it Out only because search did not exist yet.
:::

::: decision {source=agent date=2026-10-17}
The design page record-layer stays as it is for this sprint's code changes.
The heading, section and outline parser moved from internal/cli into internal/records unchanged; the record format and its checks are the same.
:::

::: decision {source=agent date=2026-10-17}
The design page agent-lifecycle stays as it is for this sprint's code changes.
prime.md's new clause is on the open-a-sprint bullet; a session's start, claim and hand-back are unchanged.
:::

::: decision {source=agent date=2026-10-17}
The design page pm-product stays as it is for this sprint's code changes.
One clause in prime.md; what pm is as a product and how it installs are unchanged.
:::

::: decision {source=agent date=2026-10-17}
The design page reply-delivery stays as it is for this sprint's code changes.
show.go only calls the heading parser from its new home in internal/records; reply delivery is untouched.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Quality on this repo's store with the exact Done-when wordings, branch built
  as 0.15.4: design covers ranks pm-quality-22 at #2; claim history gives no
  hit for pm-harness-22 (its record never says history); owner-request judge
  ranks pm-harness-14 at #41, below the later sprints that built the judge
  (pm-harness-62, 107, 119). Other wordings reach both: session claims puts
  pm-harness-22 at #2, owner-request hook puts pm-harness-14 at #4.

- Dropping each record's Design pages section from the units moved design
  covers' pm-quality-22 from #7 to #2; those short link lists outranked the
  sections with the substance. It moved pm-harness-14 from #8 to #41, because
  its #8 hit was its own Design pages section.

- Wall time per query on this repo's store: 0.15-0.16 s (4 runs each), against
  0.10-0.15 s for pm show; all 1,370 record addresses printed for pm search
  the open with pm show --record.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: `pm search` ships, and the design-page index now names it, but the quality bar holds for 1 of its 3 topics with the exact wordings.

- `internal/search`: units, tokenizer with S-stemmer, all-terms match, BM25. `internal/cli/search.go` wires it, and the heading parser moved to `internal/records` to stay under `internal/cli`'s size cap.
- The index's `prior work: pm search <topic>` line, the `prime.md` open-a-sprint clause, `--help` text and golden, and a changelog entry.
- PR #112.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| Go tests: a seeded unit with the query terms ranks first; `--kind` and `--project` filter; output stops at `-n`; no hit prints the no-match line and exits 0 | yes | `internal/search` tests, including a 3-unit BM25 corpus scored by hand; `make test-go` 20 packages pass; CI on PR #112 green |
| Three topics with a known prior sprint list it in the top 5, and one run takes under 1 s | partly | Time: met, 0.15–0.16 s per query. Ranking: design covers puts pm-quality-22 at #2. claim history never hits pm-harness-22, which never says "history". owner-request judge puts pm-harness-14 at #41, below the later sprints that built the judge. Other wordings reach both (Findings). |
| The `prime.md` rules still fit the hook chunk cap | yes | `tests/test_hooks.py`: 25 passed; CI `light` passes |
