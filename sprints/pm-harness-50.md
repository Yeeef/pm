---
type: sprint
title: Agents leave pm feedback from their own usage
bead: yeeef-agents-9va.57
---

## Goal

> What should be true when this sprint ends, and why now?

An agent that hits friction, a bug or a gap while using pm records it in one command, and the owner reads every piece of agent feedback in one fixed place, so pm improves from real usage instead of from what the owner happens to notice.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm feedback add --text "…"` (or the text on stdin): appends one entry to a single fixed feedback doc in the records store and commits it like every other `pm` write (in `WRITES`, under the store lock, through `check_planned` and `apply_writes`).
- The fixed doc is a dated doc with `project: pm-harness` (`docs/<date of first use>-pm-feedback.md`), created on first use and found by its `-pm-feedback` slug afterwards; this passes the existing record validation unchanged and shows under the project's Docs on the site.
- Each entry carries its date (UTC), the session id, the sprint or task the agent was on when it names one (`--sprint`/`--task`, optional), and the text; newest last.
- One rule in RULES.md and a row in SKILL.md: when pm gets in the way (a confusing refusal, a missing command, a rule that cost time), run `pm feedback add` once, with what happened and what would have helped; no feedback for routine use.
- Tests in `tests/test_pm.py`: first use creates the doc, later uses append to the same file, the result renders, a second session's entry lands in the same doc.

**Out:**
- Triage, dedupe or turning feedback into Beads issues automatically; the owner (or a later sprint) reads the doc and files work.
- A new record type or a top-level site section for feedback.
- Feedback about anything other than pm and its harness.

## Done when

> What evidence will show the goal is met?

- `pm feedback add` appends an entry to the one feedback doc and commits it on the `records` branch; `make test` passes with the new tests, and `make render` passes on the store afterwards.
- A real agent session (not a test) runs `pm feedback add` twice and both entries are in the same doc, visible on the pm-harness project page of the site.
- RULES.md, SKILL.md and the pm.py docstring describe the command and when to use it.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
The feedback doc is `docs/<date of first use>-<project>-feedback.md`, found by that exact name and its project header, not `docs/<date>-pm-feedback.md` as the Scope first said.
Review found the `-pm-feedback` suffix also matched hand-written docs, and two projects starting on the same day would want one path.
:::

::: decision {source=owner date=2026-10-07}
The owner asked that `pm show` also carry `pm feedback add`, so the session-start context reaches every agent; it names the command once and, per project, the feedback doc with its entry count.
Agents read `pm show` at every session start; RULES.md alone is easy to miss at the moment pm gets in the way.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Records validation allows no undated doc, so a fixed, long-lived doc has to carry the date of its first use in its name.
- The feedback doc's name takes the local date (as `pm doc new` does) while entries carry UTC time, so an entry can read the day before the name's date.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: an agent records where pm got in its way with `pm feedback add`, and every entry lands in one feedback doc per project.

Merged as 9722641 (PR #55).

- `pm feedback add --project P [--text T | stdin] [--sprint ID] [--task ID]` appends a dated entry (UTC time, session id, sprint or task, text) to `docs/<date of first use>-<project>-feedback.md`, creating it on first use, and commits it on the `records` branch.
- RULES.md, SKILL.md and the pm.py docstring describe it: use it once when pm gets in the way, not for routine use.
- `pm show`, which every session gets at start, names the command and gives each project's feedback doc with its entry count and link.
- The pm-harness feedback doc already holds two real entries: [pm feedback](../docs/2026-10-07-pm-harness-feedback.md).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: `pm feedback add` appends to the one feedback doc and commits it on `records`. `make test`: 404 passed, 7 skipped (13 new tests, one for `pm show`: first use, append from another session and stdin, refusals, render, ignored look-alike docs, `--text` with an open stdin pipe). `pm render` passed on the store afterwards (94 pages).
- Met: this session ran `pm feedback add` twice for real (records commits 4469542 and ad2d08a); both entries are in [pm feedback](../docs/2026-10-07-pm-harness-feedback.md), which the pm-harness project page links under Docs.
- Met: RULES.md (noun list and a rule), SKILL.md (command table row) and the pm.py docstring describe the command and when to use it; on the owner's request `pm show` also carries it. Real `bin/pm show` prints `feedback: 2 entries -> https://pm.yeeefs.com/docs/2026-10-07-pm-harness-feedback.html` under pm-harness and the `pm feedback add` hint after `site:`.
