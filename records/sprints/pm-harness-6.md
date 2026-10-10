---
type: sprint
title: pm CLI v1
bead: yeeef-agents-9va.6
---

## Goal

> What should be true when this sprint ends, and why now?

Agents write records and run multi-step project actions through a `pm` CLI,
so the harness's rules are enforced by commands rather than remembered.
Sprint 1 showed that written rules alone get missed: a task renamed instead
of a scope decision, findings with no home, needs inferred from assignment.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- The CLI design, reviewed by the owner before building
  (`yeeef-agents-9va.6.4`).
- `pm` core: shared record parsing with the renderer, `pm show`, `pm day
  new`, `pm finding add`, `pm sprint open` and `pm sprint close`
  (`yeeef-agents-9va.6.6`).
- Decision commands: `pm decision add` with the level choice
  (`yeeef-agents-9va.1.7`); `pm need respond` turning answers into
  decisions (`yeeef-agents-9va.2.1`).
- A free-form doc record type and `pm doc new` (`yeeef-agents-9va.6.3`).
- The harness rules file, loaded from the repo `AGENTS.md`
  (`yeeef-agents-9va.6.2`).
- The skill brought in line with the design (`yeeef-agents-9va.6.5`).
- The Beads `decision` type evaluation, as the first doc
  (`yeeef-agents-9va.6.1`).

**Out:** adopting the harness in poker-ai; renderer and view changes beyond
what the commands need.

## Done when

> What evidence will show the goal is met?

- The CLI design is on the design page and the owner has approved it.
- An agent can run a sprint day using only `pm` and `bd`: `pm show`,
  `pm day new`, `pm finding add`, `pm decision add`, `pm need add` and
  `pm need respond`, `pm doc new`, `pm sprint close`. Each write passes
  `make render`.
- `pm need respond` on a real need records a decision citing it.
- `skills/project-management/harness/RULES.md` exists, is loaded from the
  repo `AGENTS.md`, and points agents at `pm`.
- The skill text matches the design page.
- The Beads `decision` type evaluation is a doc, and its conclusion is a
  project decision.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): the commands, how records are edited, the code structure and `pm show`

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-03}
When sprint 6's work is done, the agent pushes the `dogfood-pm-harness`
branch with its Beads data and opens a PR for review, without merging,
because the owner reviews before anything reaches main.
:::

::: decision {source=owner date=2026-10-03}
The autonomous run after the design review covers sprint 6, then sprint 7,
then sprint 5 if time allows; sprints 3 and 4 wait for framing with the
owner. Agents coordinate as subagents; each claims and closes its own task.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The sprint prompt line "Each part holds 'Not closed yet.' until then"
  contains the placeholder itself, so a text replace of the first "Not closed
  yet." rewrites the prompt, not the Outcome; pm edits by section line range
  for this reason (found by a failing test in 9va.6.6). `pm show` on this repo
  prints about 1.9k characters, roughly 470 tokens.

- bd human dismiss closes a need with close_reason "Dismissed" and bd human
  respond with "Responded" (bd 1.3.1), so the answered-need check in make
  render exempts dismissed needs; on this repo it passes with the two closed
  needs (9va.1.6, 9va.6.4) cited in project decisions.

- Beads decision issues are returned by bd ready, also with
  --exclude-type=epic, so a standing decision would show as claimable work;
  git blame dates differ from the recorded decision date for 4 of 16 project
  decisions. Both led to keeping ::: decision blocks with their date
  (9va.6.1).

- git push did not push Beads data: refs/dolt/data appeared on the remote only
  after an explicit bd dolt push, despite the pre-push hook.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: agents write records and run project actions through the `pm` CLI,
which refuses writes that break the harness's rules.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **CLI design on the design page and approved:** met. Section 5 of the
  design page; approved 2026-10-03 (`yeeef-agents-9va.6.4`).
- **An agent can run a sprint day with only `pm` and `bd`:** met. `bin/pm`
  has show, day new, finding add, decision add, need add and respond, doc
  new, sprint and project open and close, and render; 93 tests at sprint 6's
  last commit, each refusal tested; every write validates the whole record
  set.
- **`pm need respond` on a real need records a decision citing it:** met.
  `yeeef-agents-9va.6.7` was answered with it on 2026-10-03 (commit
  `118f03e`); `make render` checks that every answered need is cited.
- **`RULES.md` exists, is loaded from `AGENTS.md`, points at `pm`:** met.
  `skills/project-management/harness/RULES.md`, loaded from `CLAUDE.md`.
- **Skill text matches the design page:** met. `skills/project-management/`
  rewritten (task `yeeef-agents-9va.6.5`).
- **Beads decision type evaluated as a doc, conclusion a decision:** met.
  `records/docs/2026-10-03-beads-decision-type.md`; confirmed by the owner.
