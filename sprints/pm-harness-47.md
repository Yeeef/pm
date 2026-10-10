---
type: sprint
title: Day pages as a rendered view with a periodic summary
bead: yeeef-agents-9va.53
---

## Goal

> What should be true when this sprint ends, and why now?

A day page is built entirely from the day's activity, and its Today summary is written by an agent on a schedule and stays current through the day, so the owner reads what actually happened instead of a stale morning paragraph (owner decision, 2026-10-06).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** remove the hand-written day record (`pm day new`, the `## Today` prompt, the rule that each day needs one); render a day page for every date with activity; a scheduled job that, when the day's activity changed since its last summary, asks a model for a short Today summary and stores it where the site renders it; existing day records keep their paragraphs as history; docs and rules updated.
**Out:** summaries for past days already closed; summaries for sprints or projects.

## Done when

> What evidence will show the goal is met?

- A day with activity renders a page with no day file written by hand; checked on a test server.
- The scheduled job regenerates today's summary after new activity and skips when nothing changed; checked with a real run.
- make test passes; RULES.md, SKILL.md and pm --help no longer ask for a day record.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-06}
`pm day summarize` has no --every gate: each run regenerates today's summary whenever the day's activity digest changed, so the 10-minute scheduled job keeps it current.
The owner wants the command simple; the digest check already stops calls when nothing changed.
:::

::: decision {source=owner date=2026-10-07}
The Today summary tells the owner what shipped and what changed for them, in plain words: no sprint or task ids or numbers, work named by what it does; then what waits on the owner, named by what they must do.
The owner reads it as a human who does not know sprint ids; a list of sprint numbers and opened tasks says nothing about what shipped.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: day pages are built from the day's activity with no hand-written record, and a scheduled job keeps each day's Today summary current with a 2–4 sentence ASD-STE100 summary from Claude Haiku.

Merged as da265af (PR #49). Merged as c020f97 (PR #51).

- `pm day new` removed; every date with activity gets a page; old day files render as history (PR #49, da265af).
- `pm day summarize [--dry-run]`: skips unchanged activity, fails hard on any `claude` error, commits `days/<date>.summary.json`; regenerates yesterday once after midnight if it changed.
- `pm push` runs it every 10 minutes as a separately logged `summary` step; a failure is flagged like a failed push and does not stop the records push.
- Summaries render as text, never HTML; a malformed summary file fails render and commit naming the file.
- A day's digest holds only that day's events (sprints finished with their outcome, tasks closed with reasons, requests raised or closed), with no ids, and the prompt leads with what shipped (PR #51, c020f97).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A day with activity renders a page with no hand-written day file: met by test, `pm serve` shows a generated day page in a test repo (test in PR #49); not yet seen on the live site.
- The scheduled job regenerates after new activity and skips when nothing changed: met, live. The 23:57 run wrote 2026-10-05 (its final pass) and 2026-10-06; after PR #51 the 00:37 run regenerated 2026-10-06 once (new digest format) and the 00:47 run left it alone while regenerating 2026-10-07 for new activity (`.git/pm-push.log`). Before PR #51, 2026-10-06 was rewritten at 00:09 and 00:18: current owner requests and sprint status were in a past day's digest; fixed. Live summary at 00:48: "pm serve now delivers owner replies and merged pull requests to the requesting session. Your daily summary rebuilds automatically after midnight. You approved the pm product design. …"
- make test passes; docs no longer ask for a day record: met. 387 passed, 7 skipped, 0 failed on dc3ed03 (`--every` removed on the owner's call); RULES.md, SKILL.md, CLAUDE.md, pm --help and the pm-cli, record-layer and views-and-site design pages updated.
- Fresh-context review: no blocking findings; the three that mattered (yesterday's tail, HTML in summaries, a malformed file) fixed in cbdbff2, feb7f19, 94c1723.
