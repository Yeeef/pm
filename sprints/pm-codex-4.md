---
type: sprint
title: "Ship 0.8.0 here: pin move, service, Codex trust, and closing sprint 2"
bead: pm-v498.4
---

## Goal

> What should be true when this sprint ends, and why now?

This repo runs pm 0.8.0, the first release with the Codex work: its pin moved, its service restarted with codex on PATH, and pm's Codex hooks trusted, so a Codex session here gets every pm guarantee. pm-codex sprint 2 is closed too, which needs pm to restamp a PR review closed without its main merge commit. Now, because 0.8.0 is published (pm-v0.8.0, 2026-10-13) and nothing uses it yet.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The pin-move PR (`pm upgrade --to 0.8.0`), with `design_sources` added to `.pm/config.toml` if the release carries the key (it does not: the gate ships later).
- `pm service install` from a shell with codex 0.149.0 or later.
- The owner's one-time Codex hook trust.
- `pm action done` restamping a closed PR review that lacks "merged as <sha>", then closing sprint 2.

**Out:**
- Other repos' pin moves.
- The release policy, which is its own sprint in pm-harness.

## Done when

> What evidence will show the goal is met?

- `.pm/config.toml` on main pins 0.8.0, and `pm version` in the main checkout prints 0.8.0.
- `pm doctor` in the main checkout prints no codex service line and no "Codex skips" line.
- `pm sprint close pm-v498.2` succeeds.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): `pm action need --pr` refuses a PR whose base is not main; `pm action done` restamps a review closed without `merged as <sha>`.
- [pm CLI](../design/pm-cli.md): the `pm action done` and `pm action need --pr` rows; `covers:` follows the word wrapper to `internal/textwrap`.
- [Work store](../design/work-store.md): `work.SetCloseReason` in "Inside pm", and how the close group merges after a restamp.

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

partial: the pin move to 0.8.0 (PR #60) and the review restamp (PR #61) are ready. Restarting the service with codex, the Codex trust, and closing sprint 2 follow their merges.

- PR #60: `pm upgrade --to 0.8.0`: the pin, the hook entries for both runtimes, `.codex/rules/pm.rules`, and the post-checkout marker.
- PR #61: `pm action done` restamps a closed PR review that lacks "merged as <sha>"; `pm action need --pr` refuses a PR whose base is not main.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| `.pm/config.toml` on main pins 0.8.0; `pm version` in the main checkout prints 0.8.0 | not yet | PR #60, CI 12/12; in its worktree `pm version` prints 0.8.0 |
| `pm doctor` prints no codex service line and no "Codex skips" line | not yet | after #60 merges: `pm service install` from a shell with codex 0.154.0, then the owner's trust step |
| `pm sprint close pm-v498.2` succeeds | not yet | needs #61 in a release pinned here; under the release-on-merge policy its merge releases it |
| Design pages of changed areas updated and listed | met | Sprint lifecycle, pm CLI, Work store (records ad9b8e2, 6f2cdc4) |
