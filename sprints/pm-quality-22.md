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

- [Record layer](../design/record-layer.md): a design page's `covers:` header and `pm check`'s covers check.
- [Sprint lifecycle](../design/sprint-lifecycle.md): the design-page gate at review and close, and `pm design unchanged`.
- [pm CLI](../design/pm-cli.md): `pm design unchanged`, and the gate's refusals in the command table.
- [Views and the site](../design/views-and-site.md): the line a design page shows when its covered code changed after its last edit.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-12}
Build the full mechanism: covers header, the gate at pm action need --pr and pm sprint close with a recorded waiver, the coverage report, the site staleness line, and covers on every existing page.
Owner chose it in chat, 2026-10-12, over a gate-only build or a proposal doc; the design-page miss in pm-codex sprint 1 needs a structural fix, not a prose rule.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The pm-codex sprint 1 design-page pass (12 pages updated) found text wrong
  against the code that predates that sprint: pm-harness (writable roots pm
  setup adds, .records), agent-lifecycle (Start, Claim, Work: the Beads and
  Python era), pm-cli (the commands table, Agent identity), site-replies
  (metadata.*, picked_up), agent-sync (the profile line,
  session_context_hook.py), records-store (.beads and uv-cache roots). This is
  the drift this sprint's staleness line is meant to surface.

- `internal/cli` sat exactly at its size cap (8920 of 8920 non-test lines), so
  the gate lives in a new package `internal/design`, and gh's JSON call (to
  `internal/proc`), the need-text helpers and the merge stamp (to
  `internal/records`) moved out unchanged; `internal/cli` ends at 8910 and its
  cap entry drops to that.

- The pinned pm 0.7.1 refuses unknown keys in `.pm/config.toml`, so
  `design_sources` cannot go into this repo's config until the pin moves to a
  release with this change; 0.7.1 reads a `covers:` header as an unknown field
  and accepts it, so the pages carry covers already. Until then the gate holds
  no PR here: 0.7.1 has no gate.

- `git ls-files --full-name` run from a subdirectory lists only that subtree:
  a check run from `internal/design` called 3 patterns dead. Tracked now
  passes the `:/` pathspec, which lists the whole worktree from anywhere.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
