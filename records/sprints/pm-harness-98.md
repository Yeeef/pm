---
type: sprint
title: A pm release passes its own hooks, so agents can release pm
bead: yeeef-agents-9va.108
---

## Goal

> What should be true when this sprint ends, and why now?

An agent releases a new pm version with a normal commit, push and PR, never skipping a git hook. Today no release since 0.1.2 (2026-10-07) went out: a release commit pins the new version, the launcher then looks for its tag `pm-v<version>`, which exists only after the merge, so every pm call in that checkout fails, the pre-commit hook included. Main holds 14 Python pm changes that no session runs, and a new user is waiting on a current release.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** how pm runs in a checkout whose pin is a version not yet tagged, in this repo (where pm's own source lives) and in other repos; the release procedure from bump to tag, written where agents read it; a test that a release commit passes the hooks; releasing 0.1.3 or the next version through the fixed path.
**Out:** the Go port's release workflow and launcher (sprint 91); changing what the version pin means for other repos.

## Done when

> What evidence will show the goal is met?

- A test commits a version bump in a scratch clone with pm's hooks installed and no matching tag, and the commit passes without `--no-verify`.
- The release procedure is written down, and the next pm release follows it with no hook skipped; its tag and the pin agree.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-08}
A pm release tags its version-bump commit before the pin moves: commit A bumps pm/pyproject.toml and uv.lock with the old pin, tag pm-v<new> goes on A, then commit B moves the pin; the PR merges with a merge commit so the tag's commit is on main; no launcher change.
The hooks run the installed pm uv tool (0.1.2), so "run the checkout's own source" in the launcher cannot help the first release without a hook skip; tag-before-pin works with every launcher since 0.1.2 and needs no code change. Done-when item 1's test therefore commits A with no tag and B after the tag, both without --no-verify.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Tag-before-pin released 0.1.3 with no hook skipped: commit A 5dbac54
  (pyproject + uv.lock) and commit B 3baf051 (pin, via pm upgrade) both passed
  the pre-commit hook without --no-verify; B's hook ran pm 0.1.3 built from
  tag pm-v0.1.3 by the 0.1.2 launcher. New integration test 1 passed in 5.4 s;
  make test 108 passed. At A alone, make test fails 9 tests (the checkout is
  0.1.3 and launches the 0.1.2 pin), so the branch is pushed only after B.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: a pm release tags its version-bump commit before the pin moves, so both release commits pass the hooks; 0.1.3 went out this way, with no launcher change.

Merged as 0a51c0f (PR #86).

- Integration test `test_a_release_tags_before_it_pins_so_both_commits_pass_the_hook` in `pm/tests/test_launch.py`.
- The "Releasing pm" procedure in `pm/AGENTS.md`: commit A, tag, commit B through `pm upgrade`, merge with a merge commit.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A test commits a version bump in a scratch clone with pm's hooks and no matching tag, passing without `--no-verify`: met, with the change the sprint decision records. Commit A (the bump) passes with no tag; commit B (the pin) is refused with no tag and passes once it exists. `make test-full ARGS="-k a_release_tags_before"`: 1 passed, 5.4 s. `make test`: 108 passed, 136 skipped.
- The release procedure is written, and the next release follows it with no hook skipped and its tag and pin agreeing: met, "Releasing pm" in `pm/AGENTS.md`; 0.1.3 released as `5dbac54` (tag `pm-v0.1.3`) and `3baf051` (pin 0.1.3).
- The sprint's PR is on main: PR #86, CI green; waits on the owner's merge.
