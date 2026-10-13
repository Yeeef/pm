---
type: sprint
title: pm releases by a written policy, automatically
bead: yeeef-agents-9va.126
---

## Goal

> What should be true when this sprint ends, and why now?

pm releases by a written policy, not by courtesy. The policy says when a release is cut (every merged PR, or a stated other rule) and how its version is chosen. Automation enforces it: nobody has to remember to cut a release, and nothing reaches users that the policy did not mean to ship. Now, because pm 0.8.0 was cut only when the owner asked, after the Codex work had sat on main unreleased; and because the order of merges decided what 0.8.0 contained.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The policy as a design page: the trigger (per merge, per batch, a schedule or other), version selection from the changelog entry files (SemVer while 0.x), what blocks a release, pre-releases, and how repos move their pins.
- Prior art: release-please, changesets, semantic-release, continuous delivery per merge, release trains.
- The owner's decision on the policy.
- The automation, in CI on main: the notes PR, or no notes PR, the tag and the release, under the existing rules (never move a tag, notes from changelog.d).
- Its tests, docs (CLAUDE.md "Releasing pm", the pm-versioning page) and a live check on a real release.

**Out:**
- Moving other repos' pins automatically (the policy may say how they learn of a release).
- Changing the build itself (release/build.sh, pm-release.yml's build jobs).

## Done when

> What evidence will show the goal is met?

- The policy page exists and the owner's decision on it is recorded.
- A merge to main that the policy says releases produces a published GitHub release with notes from changelog.d, with no manual step: shown once live.
- A merge the policy says does not release produces none, and is shown in a test.
- `make test` covers the policy's version selection and trigger rules.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [When and how pm releases](../design/release-policy.md): the policy, the version rule, the hold, the release workflow and its failure handling
- [How a repo runs the right pm version](../design/pm-versioning.md): its Releasing pm row, now a release per merge

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
