---
type: sprint
title: Move pm-harness tracking to Yeeef/pm
bead: yeeef-agents-9va.116
---

## Goal

> What should be true when this sprint ends, and why now?

pm's own development is tracked in Yeeef/pm, where its code lives: Yeeef/pm runs pm on itself, and its work store and records hold all of pm-harness, so unfinished pm work continues there. Why now: pm moved to its own public repo on 2026-10-09, and its tracking still sits in yeeef-agents, so sprint PRs and records live in different repos.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm init` in Yeeef/pm: work store, records branch, hooks and a pm service on its own port; Yeeef/pm AGENTS.md says pm tracks itself.
- Move pm-harness's work-store items (the project, its sprints, tasks and needs, open and closed) into Yeeef/pm's store, keeping ids so the records still resolve.
- Move every pm-harness record (project, sprints, design pages, docs, postmortems), scrubbed of hostnames, emails, machine paths and session ids before they go public.
- Triage the open sprints on arrival: move, reframe for Go, or close as obsolete (81 Dolt GC, 94 hosted site on Beads).
- In yeeef-agents: close pm-harness here with a pointer to Yeeef/pm, keeping sprint 79's deletion of `pm/` here.

**Out:**
- The open sprints' own work (Access token, records copy, pm clean, ...): it continues in Yeeef/pm after the move.
- Which clone pm.yeeefs.com serves; the new site stays on localhost until a decision.
- New pm tooling for moving projects between stores, unless the import path needs a small fix.

## Done when

> What evidence will show the goal is met?

- In a clone of Yeeef/pm, `pm show --project pm-harness` lists the open sprints with their tasks and owner needs, and `pm check` passes on its records branch.
- `git ls-remote https://github.com/Yeeef/pm` shows the `records` branch and the work-store ref.
- A grep of Yeeef/pm's records for `yeeefs.com`, the owner's email, `~`, `~` and session UUIDs finds nothing.
- In yeeef-agents, `pm show` no longer lists pm-harness as open, and its project record links to Yeeef/pm.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-10}
In yeeef-agents, pm-harness records stay as they are; after the export to Yeeef/pm, every open pm-harness sprint here closes as done with a note that its work moved to Yeeef/pm, and its open tasks and owner needs close with it
the work continues in Yeeef/pm; closing here, after the export so the copy keeps the open state, leaves no duplicate open work in two stores
:::

::: decision {source=owner date=2026-10-10}
The moved pm-harness records and work store keep pm.yeeefs.com, yeeefs.com and the Cloudflare team domain unchanged; the scrub removes only the email, home paths, session ids, the machine hostname and claude.ai links
the hostname is public DNS behind Cloudflare Access, and working links matter more than hiding it
Answers `yeeef-agents-9va.116.6`.
:::

::: decision {source=owner date=2026-10-10}
Run the move now and deliver it end to end: init, import, records, push, triage, and close pm-harness in yeeef-agents
the owner gave the go with full agency after the offline rehearsal
Answers `yeeef-agents-9va.116.2.1`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Offline rehearsal (scratch clone, local bare remote): pm export of
  yeeef-agents filtered to pm-harness gives 571 items (1 project, 104 sprints
  with 10 open, 270 tasks with 18 open, 196 needs with 4 open, 274 comments,
  63 deps); imported with ids kept, re-export identical; pm check renders all
  148 pages; 137 records moved, 0 matches left for yeeefs.com, the owner
  email, home paths or session UUIDs. Blocker: pm init --import-bd takes bd
  format only, so a small Go change adds pm init --import for pm's own export
  (about +60/-11 in internal/work/export.go and internal/cli/work.go).

- The first linux CI run of PR 6 failed in internal/work with a Dolt fatal
  during GC (UpdateGCGen); the rerun passed, and 6 local runs passed, so it is
  a flaky GC test, tracked as pm-d2k5.3.1 in pm-quality.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm-harness is tracked in Yeeef/pm with its full history, and the yeeef-agents copy is closed.

- Work store: 594 items imported with ids kept (14 open and 95 closed sprints, 20 open and 261 closed tasks, 4 open and 199 closed needs, 280 comments, 63 deps); a re-export equals the import.
- Records: 142 moved (109 sprints, 19 design pages, 11 docs, 2 postmortems, the project), scrubbed of the owner email, home paths, session ids, the machine hostname and claude.ai links; pm.yeeefs.com kept by owner decision.
- `pm init --import FILE` merged in PR 6 as 080c352; pm's repo files, the AGENTS.md change and `site_url` merged in PR 11 as 310df2d.
- pm.yeeefs.com serves this clone's pm site and agents.yeeefs.com serves yeeef-agents', by owner decision.
- Triage: sprints 81 and 94 voided; 56 and 68 port yeeef-agents PRs 76 and 72 to Go (new tasks .65.6 and .77.6); 55 and 92 retargeted to Go.
- In yeeef-agents, all 14 open pm-harness sprints closed as moved, and the project closed with an Outcome that links here.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| `pm show --project pm-harness` in Yeeef/pm lists the open sprints with tasks and needs; `pm check` passes | met | `pm show`: 11 open sprints, 2 owner requests; `pm check`: all 153 pages render |
| `git ls-remote` shows the records branch and the work-store ref | met | `refs/heads/records` 2db31ce, `refs/pm/work` ab0cbf3 |
| A grep finds no `yeeefs.com`, owner email, home paths or session UUIDs | met, with one change | 0 matches for the email, `/home/yeeef`, `/Users/yeeef`, the machine hostname and claude.ai links; `yeeefs.com` is kept by the owner decision on .116.6; the moved records hold 0 session ids, and `pm feedback add` writes the session id into each new entry (2 since the move) |
| In yeeef-agents, `pm show` no longer lists pm-harness, and its project record links to Yeeef/pm | met | `pm show` in yeeef-agents lists 4 projects without pm-harness; its project Outcome names Yeeef/pm |
