---
type: sprint
title: Compact views for agents
bead: yeeef-agents-9va.8
---

## Goal

> What should be true when this sprint ends, and why now?

Agents read project state and records through compact `pm` views instead of
whole files, so a session starts and works with a small context.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- `pm show --record <path> --section <name>`: print one section of a record.
- Session start through `pm show` and a trimmed `bd prime`, instead of
  re-reading records.

**Out:** hooks, outlines and delegation rules, which belong to the
context-efficiency project.

## Done when

> What evidence will show the goal is met?

- `pm show --record <path> --section <name>` prints one section of a record,
  so an agent reads what it needs instead of the whole file; a test covers it.
- The pm-harness design page is split into sub design pages, one per area,
  the main page links each, and every sprint's Design pages entry links the
  specific sub page; RULES.md states the rule.
- Session start through `pm show` waits on the hooks answer
  (`yeeef-agents-9va.14.2`) and is not part of this sprint.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): `pm show --record --section`
- [Record layer](../design/record-layer.md): one area per design page (The design page)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the pm-harness design page is split into five focused sub pages that
sprints link directly, and `pm show --record --section` prints one section of
a record, so agents and the owner read only the part they need.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `pm show --record <path> --section <name>` prints one section: met
  (4a7a3cd). Headings are found with the renderer's parser, so a `#` line
  in a code fence or HTML block is not a section; `test_show_record_section`
  (4 cases) and `test_show_record_section_refuses` (5 cases).
- The design page is split into sub pages, the main page links each, every
  sprint links its specific sub page, and RULES.md states the rule: met.
  The main page went from 635 to 153 lines; work-layer (120), record-layer
  (269), pm-cli (185) and views-and-site (178) hold the moved text, checked
  line by line; a link check over the rendered site found 0 broken of 389
  relative links (e92a94e; records d4e2801, 6264d02).
- Session start through `pm show` waits on need `yeeef-agents-9va.14.2`, as
  framed.
