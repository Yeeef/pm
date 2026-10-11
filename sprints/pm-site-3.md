---
type: sprint
title: The day summary tells what happened, with no to-do for the owner
bead: pm-bfli.3
---

## Goal

> What should be true when this sprint ends, and why now?

The Today summary on a day page tells only what happened that day. It no longer ends with a "For you" bullet that
tells the owner what to do: the owner asked for this on 2026-10-11, and the open needs already show in the
"Actions await you" and "Decisions await you" sections.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `prompts/day_summary.txt`: drop the "For you" theme; a request raised or answered that day stays an event of the day.
- A changelog entry.

**Out:**
- The activity the summary reads, its digest and the day page.

## Done when

> What evidence will show the goal is met?

`pm day summarize --dry-run` with the new prompt, on today's activity, prints bullets with no "For you" theme and no
instruction to the owner.

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

- pm day summarize --dry-run with the new prompt on 2026-10-11's activity, 3
  runs: 0 of 3 gave a For you bullet or an instruction to the owner; the
  stored summary from the old prompt ended with For you: Review the pull
  request that adds the main checkout flag.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the Today summary tells only what happened that day, with no "For you" bullet that tells the owner what to do.

Merged as 9e9e40b (PR #46).

- `prompts/day_summary.txt`: the "For you" theme is gone; a request raised or answered that day stays an event of the day.
- A changelog entry.
- A stored summary is rewritten with the new prompt once that day's activity next changes: its digest hashes the
  activity only.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: `pm day summarize --dry-run`, pm built from the PR, on 2026-10-11's activity: 3 of 3 runs give no "For you"
  theme and no instruction to the owner. The stored summary from the old prompt ends with "For you: Review the pull
  request that adds the main checkout flag."
