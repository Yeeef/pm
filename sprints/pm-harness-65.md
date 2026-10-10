---
type: sprint
title: pm runs each repo's pinned version
bead: yeeef-agents-9va.74
---

## Goal

> What should be true when this sprint ends, and why now?

Repos pinned to different pm versions work side by side on one machine: the installed `pm` reads the repo's pin and runs that exact version, so each repo upgrades when it chooses. Why now: the owner chose the launcher on 2026-10-07, and pm 0.1.1 would otherwise stop formal-methods and ai-safety, which pin 0.1.0.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the installed `pm` as a launcher that runs the pinned version through uvx when it differs from its own, cached per version, with no loop; the pm service unit, the hook entries and the git hooks running the pinned version the same way; `pm doctor`, `pm where` and errors naming the version in use; the pm-product design page's Version pin section; shipping in 0.1.1.
**Out:** versions before the launcher learning to launch (0.1.0 is run, not changed); publishing to PyPI.

## Done when

> What evidence will show the goal is met?

- In a repo pinned to 0.1.0, `pm show` run by a 0.1.1 tool runs 0.1.0 and exits 0. Expected: shown by a test with two built versions, and live in formal-methods.
- A repo pinned to the tool's own version runs it with no uvx call (a test).
- A pin the launcher cannot fetch fails hard, naming the version and the command that fixes it (a test).

## Design pages

> Where is the detail?

None yet.

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

done: the installed `pm` runs each repo's pinned version, so repos on different pins share one machine ([PR #69](https://github.com/Yeeef/yeeef-agents/pull/69), pm 0.1.2); live on this Mac after the merge, three repos on two pins share one machine.

Merged as 211c423 (PR #69).

- `pm/src/pm/launch.py` runs first in `cli.main`: in process when there is no pin or the pin is the running version; otherwise it resolves tag `pm-v<pin>` to its commit once (10 s timeout, no prompt), warm-builds it, keeps the commit (written atomically) and execs `uv tool run --from git+…@<sha>#subdirectory=pm pm …`, 0.23 s when cached, with no network; a missing tag or failed build fails hard naming the tag.
- The launch marker reaches only the launched process, not its children; pins below 0.1.2 run in their own tool dirs and get no marker.
- `pm upgrade` never moves a pin down without `--to`; a service unit an old pin wrote is reported stale with `pm service install`.
- Two fresh-context reviews: 5 findings fixed with tests; 1 only in part (no pm hint when the exec'd uv itself fails).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **In a repo pinned to 0.1.0, `pm show` run by the new tool runs 0.1.0 and exits 0: met in a test and live.** Live on 2026-10-07 after tagging `pm-v0.1.2` and installing it: in formal-methods and ai-safety (both pinned 0.1.0) `pm where` and `pm show` exit 0, the launcher kept commit d4e64a6 for 0.1.0 (the `pm-v0.1.0` tag), their `pm init` put the 0.1.0 tool under `~/.local/share/pm/pins/0.1.0/` and changed no repo file, and `pm doctor` printed "pm 0.1.0: every managed piece and the clone's setup match" in both; this repo runs 0.1.2 in process with `pm doctor` clean and its service answering on :8000. `test_launch.py`'s integration test builds 0.1.0 from this clone's tag with real uv and runs `pm show` (exit 0); it passes in CI on PR #69.
- **A repo pinned to the tool's own version runs it with no uvx call: met.** `test_launch.py` (same pin, no uv call).
- **A pin the launcher cannot fetch fails hard, naming the version and the fixing command: met.** `test_launch.py`: fetch failure, missing tag and both timeouts give the `pm-v<v>` message and keep nothing.
- Evidence on 28b9748: `make test` 66 passed, 35 skipped (live-model eval); CI on PR #69 passed guard, light (23 s) and integration (48 s).
