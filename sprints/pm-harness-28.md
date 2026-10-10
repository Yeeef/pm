---
type: sprint
title: Beads team-maintainer profile
bead: yeeef-agents-9va.33
---

## Goal

> What should be true when this sprint ends, and why now?

Agents in every clone run under the Beads team-maintainer profile, so committing, pushing and syncing are routine instead of forbidden by the conservative default.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `agent.profile: team-maintainer` in `.beads/config.yaml`; `pm setup` sets it where missing and `pm where` shows it (PR #31).

**Out:** carrying the profile to subagents, and the other conflicts between generic defaults and the harness (sprint 25).

## Done when

> What evidence will show the goal is met?

- `.beads/config.yaml` on main sets `agent.profile: team-maintainer`, and `bd prime` on main reports team-maintainer active.
- `bin/pm setup` sets the profile in a clone where it is missing and changes nothing on a second run; `bin/pm where` shows it.

## Design pages

> Where is the detail?

- [Agent git authority and Beads sync](http://localhost:8000/design/agent-sync.html): the agent profile.

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

Done: the repo runs under the Beads team-maintainer profile, and `pm setup` sets it in any clone where it is missing.

- `.beads/config.yaml` sets `agent.profile: team-maintainer`.
- `pm setup` sets the profile only when it is not already team-maintainer; `pm where` shows it and says "run bin/pm setup" when it is wrong.
- CLAUDE.md and SKILL.md describe the new setup step.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Profile on main, and `bd prime` reports it: met; PR #31 merged as 6c54d08, `git show origin/main:.beads/config.yaml` has `profile: team-maintainer`, and `bd prime` on main prints "agent.profile=team-maintainer is active".
- `pm setup` sets it once, then changes nothing; `pm where` shows it: met, merged in PR #31 (57044b4): the first real `bin/pm setup` printed "set the Beads agent profile to team-maintainer", the second "already set up", and `bin/pm where` ends "agent profile team-maintainer"; `uv run tests/run.py` 326 passed; `make render` clean.
