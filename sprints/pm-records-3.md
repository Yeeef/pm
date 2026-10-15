---
type: sprint
title: Rename a design page or a doc
bead: pm-c72a.3
---

## Goal

> What should be true when this sprint ends, and why now?

An agent can rename a design page or a doc, its slug, its title or both, in one command that keeps every record's links to it working. Today a slug is fixed at `pm design new` / `pm doc new`: renaming one by hand leaves `Design pages` entries, prose links and design waivers pointing at a page that no longer exists, and `pm check` does not notice.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm design rename <slug> [--to NEW] [--title "…"]`: moves `records/design/<slug>.md`, sets its title, and rewrites every `../design/<slug>.md` link and design waiver line in the records, in one commit.
- `pm doc rename <path|slug> [--to NEW] [--title "…"]`: the same for a doc; the date prefix stays.
- Refusals: an unknown or existing target, the feedback doc, a page named by an open PR review need.
- `--help` texts, golden files, harness tests, changelog entry, design page update.

**Out:**
- Redirects from old URLs.
- Rewriting review needs in the work store (an open one refuses the rename instead).

## Done when

> What evidence will show the goal is met?

- `make test` and the PR's CI pass, with harness tests that rename a design page and a doc and assert the moved file, the new title, the rewritten links and waivers, and each refusal.
- A live run in a scratch clone renames a design page that a sprint's Design pages section links, and `pm check` passes after it.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): the `pm design rename` and `pm doc rename` rows of Commands.
- [Record layer](../design/record-layer.md): The design page, how a rename keeps links whole.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
A rename rewrites every link and waiver in the records in its one commit, and refuses while an open PR review need names the design page; old URLs get no redirect.
Links in records are the only references pm checks against; rewriting them keeps pm check and the review gate correct, and refusing on an open review avoids a work-store rewrite for a transient case.
:::

::: decision {source=agent date=2026-10-15}
The design page sprint-lifecycle stays as it is for this sprint's code changes.
The design gate and its waivers work as before; the rename only rewrites a renamed page's slug in its waivers, which the pm-cli page states.
:::

::: decision {source=agent date=2026-10-15}
The design page agent-lifecycle stays as it is for this sprint's code changes.
prime.md only names the two new rename commands beside design new and doc new; the lifecycle it describes is unchanged.
:::

::: decision {source=agent date=2026-10-15}
The design page pm-product stays as it is for this sprint's code changes.
prime.md only names the two new rename commands; nothing the product page says about setup or prime changes.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check in a scratch clone (built binary, scratch origin, temp HOME): pm
  design rename parser --to grammar --title Grammar made one records commit
  (design/{parser.md => grammar.md}, projects/lang.md, sprints/lang-1.md); no
  old link or waiver text left; pm check: all 6 pages render, exit 0; site
  /design/grammar.html 200, /design/parser.html 404; rename onto an existing
  slug and --to Bad_Slug both refused with exit 1.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm design rename` and `pm doc rename` rename a design page or a doc (its slug, its title or both) and rewrite every link and waiver that names it, in one records commit.

- Link forms rewritten: `.md` and `.html`, with anchors, link titles and angle brackets kept; links in code blocks and inline code are left as written.
- Refusals: no flag given; an unknown or ambiguous target; an invalid or taken slug; an empty title; a rename that changes nothing; the feedback doc; an open PR review naming the page; uncommitted targets.
- Not covered: reference-style links, slugs in prose, and redirects from old URLs.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `make test` and the PR's CI pass, with harness tests for both renames and every refusal. Met: `make test` gave 146 passed and 169 skipped; PR #100's CI passed 52 checks, the rest skipped by design. The tests are `test_design_rename_moves_the_page_and_rewrites_every_link_and_waiver` and `test_doc_rename_keeps_the_date_prefix_and_rewrites_every_link`, plus Go unit tests in `internal/records/rename_test.go`.
- A live rename in a scratch clone, with `pm check` passing afterwards. Met (see Findings): the rename made one records commit and left no old links or waivers; `pm check` exited 0; the site gave 200 for `/design/grammar.html` and 404 for `/design/parser.html`.
