---
type: sprint
title: A change to covered code updates or waives its design page
bead: pm-d2k5.22
---

## Goal

> What should be true when this sprint ends, and why now?

A PR cannot go to the owner for review, and its sprint cannot close, while a design page covering code the PR changed was neither updated in the sprint nor waived with a recorded reason. The check is structural, not a rule in prose. Now, because pm-codex sprint 1 changed reply delivery, session claims and the owner-request hook without editing their pages, although prime.md already had the rule; the owner asked for a mechanism (2026-10-12).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A `covers:` list of path patterns in a design page's header, checked by `pm check` (a pattern that matches no tracked file fails).
- A gate in `pm action need --pr` and `pm sprint close` (`--merged` too): every page covering a file the sprint's PR changed is edited on the records branch since the sprint opened and listed in its Design pages section, or waived by `pm design unchanged <slug> --sprint ID --reason "…"`, which records a sprint decision.
- The PR review request lists the covering pages with links.
- A coverage report of tracked source files no page covers, within source patterns a config key names.
- A design page on the site says when its covered code changed after its last edit.
- `covers:` on every existing design page in this repo's records.
- Docs, `--help` and prime.md where a procedure runs the new command; a changelog entry.

**Out:**
- A model judging whether a page's content matches the code.
- Pages other than design pages.

## Done when

> What evidence will show the goal is met?

- A harness test: a PR that changes a covered file is refused at `pm action need --pr` and at `pm sprint close`, naming the page. The same PR passes once the page is edited and listed, or once it is waived; the waiver shows as a sprint decision.
- Replayed against pm-codex sprint 1's PR #47 file list, the gate names `reply-delivery.md`, `owner-request-hook.md` and `session-claims.md`.
- `pm check` fails on a `covers` pattern that matches nothing, and reports uncovered source files; this repo's records pass with every existing design page carrying `covers:`.
- The site's page for a design page whose covered code changed after its last edit says so; a golden page shows it.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
