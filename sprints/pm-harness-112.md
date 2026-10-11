---
type: sprint
title: README presents pm as a human-agent project management suite
bead: yeeef-agents-9va.122
---

## Goal

> What should be true when this sprint ends, and why now?

The README tells a first-time reader, in its first screen, what pm is: a human-agent project management suite, not a
record layer and not a skill pack. It names the three things pm does (agent project and task management, context
managed as records tied to tasks, the site as the owner's interface to a swarm of agents), how it enforces them
structurally (the `pm` cli, the work store and the session hooks), and the outcomes that follow. Why now: the README
describes pm only as two storage layers, which undersells it and gives a reader no reason to choose it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `README.md` rewritten: positioning, the three parts, the structural enforcement (cli and hooks, versus prompt files),
  the outcomes, the site with screenshots of the pages the owner uses, install, docs.
- Each claim checked against the code before it goes in.
- Screenshot images the README shows, in the repo.

**Out:**
- `prime.md`, `--help` texts and the site's code.
- A docs site or a website.

## Done when

> What evidence will show the goal is met?

- The PR's README renders on GitHub with its screenshots, and each outcome it claims names the mechanism that backs it
  (listed in the PR body with the file or command).
- The PR is merged to main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-11}
The marketing website for pm is its own sprint (113), beside this one; this sprint stays the README and its screenshots.
The owner asked for a marketing website mid-sprint; it shares the positioning and screenshots with the README but ships independently (a new site directory and a Pages deploy), so the two run in parallel and neither waits on the other.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Claims corrected while checking the README against the code: nothing blocks
  a PLANNING.md in the code tree (README says pm gives context a checked home
  and does not block a stray file); pm init/upgrade do edit CLAUDE.md and
  AGENTS.md, only to remove a Beads block (leftoverRels in
  internal/install/pieces.go); pm injects a rules text (prime.md) by hook, so
  the README says rules arrive by hook and are enforced by refusals, not that
  there is no prompt; pm task close names HEAD or warns, never refuses for a
  missing commit, so it is not listed as a refusal; record hand edits go
  through pm commit, so the cli is not the only record writer.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
