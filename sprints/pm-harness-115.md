---
type: sprint
title: Credit Beads and Dolt in the README and website
bead: yeeef-agents-9va.125
---

## Goal

> What should be true when this sprint ends, and why now?

The README and the website credit the two projects pm builds on: Beads, which inspired pm's work layer and which pm
itself used before it had one, and Dolt, the database the work store runs on. Why now: the owner asked for it; the
README and website, just published, name neither.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A Credits section in `README.md` and on `website/index.html`, linking each project.

**Out:**
- License notices or a NOTICE file; any other copy change.

## Done when

> What evidence will show the goal is met?

- The README and the live website show the Credits section with working links to Beads and Dolt.
- The PR is merged to main and the Pages deploy serves it.

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

done: the README and the website credit Beads, which inspired pm's work layer and ran pm's own tracking before the work store, and Dolt, the work store's database (PR #57), merged as 2fcc7e8 and live on the website.

- `README.md`: a Credits section after Docs.
- `website/index.html`, `website/style.css`: a Credits section after Get started, two cards that stack at phone width, linked from the footer.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The README and the live website show the Credits section with working links: met. Main's README has the section; https://yeeef.github.io/pm/ serves `id="credits"` with both links. Links answer 200 (`github.com/steveyegge/beads` redirects to `github.com/gastownhall/beads`, the link used; `github.com/dolthub/dolt`). Headless Chromium on the page: scrollWidth 390/390 at 390 px and 1440/1440 at 1440 px, light and dark. CI on e3ed2cb: 12 checks passed (one changelog run cancelled by its re-run after the `no-changelog` label).
- The PR is merged to main and the Pages deploy serves it: met. Merged as 2fcc7e8; deploy run 38306674323 succeeded.
