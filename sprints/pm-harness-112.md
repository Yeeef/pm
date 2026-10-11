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

- The live pm-harness project page's progress diagram renders as Mermaid's
  'Maximum text size in diagram exceeded' box (project with 100+ sprints), so
  project.png shows pm-site instead. The live site had no open decision need;
  the golden pages' need cards are edge-case fixtures (raw script text, broken
  tables), so need.png comes from a scratch clone's pm service (harness repo
  fixture, PORT=18766) with a decision need raised by pm decision need.
  Screenshots: 1440 px, scale 2, 128-colour quantized, 54-259 KB each.

- Owner review of the README (reply on the PR #42 review, 2026-10-11 18:35
  UTC): the first section did not catch the eye and confused with
  implementation details (work store, hooks, Stop hooks). Rewritten in c6f2151
  to lead with the tagline 'Manage outcomes, not agents', a two-line pitch,
  the overview screenshot and five outcomes; mechanisms moved below.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the README presents pm as a human-agent project management suite, with each claim checked against the code and the site shown in four screenshots; merged to main as e122a0c through PR #42, which was stacked on PR #41 and carried its commits (#41 closed as superseded).

Merged as e122a0c (PR #42, which carried PR #41).

- `README.md`: the pitch, the three parts (agent project and task management, context as records tied to tasks, the site), what pm enforces structurally (cli refusals, the work store, session hooks) and what it leaves to its rules, the outcomes with their mechanisms, a Mermaid diagram, install and docs.
- `website/img/overview.png`, `project.png`, `sprint.png`, `need.png`: the site's overview, a project, a sprint and a decision need with its reply form, shared with sprint 113's website.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The README renders on GitHub with its screenshots, and each outcome names its mechanism: met. PR #41's body lists every claim with its file or command; a fresh-context review checked them against the code and found five overclaims or misstatements (only what a tool checks is enforced; subagent start gets the rules, not the state; liveness is read from a local transcript; docs are free-form; the `claude` CLI is needed for the owner-request check and `pm day summarize`), fixed in fa45dba. The Mermaid diagram parses in Mermaid 11.17.2. After the owner's review, the first section was rewritten to lead with the outcomes and the site, not the mechanisms (c6f2151). CI on c6f2151: 9 of 9 checks passed.
- The PR is merged to main: met. e122a0c (PR #42) holds #41's commits; `git diff origin/main origin/readme-suite -- README.md website/img` differs only by #42's website link line.
