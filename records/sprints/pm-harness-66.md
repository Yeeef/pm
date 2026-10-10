---
type: sprint
title: A 7-page pitch deck that sells pm
bead: yeeef-agents-9va.75
---

## Goal

> What should be true when this sprint ends, and why now?

A pitch deck of at most 7 pages hooks a new reader on pm within 10 minutes and ends with the one command that bootstraps their repo. Why now: pm became an installable product on 2026-10-07 (pm-v0.1.1), and the owner wants more people to use it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** one pm doc page on the site, built mostly from raw HTML: the problem, the idea (work layer, record layer, pm on top, the site), what an agent and the owner each get, why it holds together, and a closing bootstrap page; visuals, HTML/SVG memes and jokes.
**Out:** a separate slide file format; images copied from elsewhere; new pm features.

## Done when

> What evidence will show the goal is met?

- The page renders on the site with at most 7 slides, each readable at phone width, and `pm check` passes. Expected: 7 slides.
- The last slide gives the exact bootstrap command for the released version, and it runs: the same command installed pm into fresh repos in sprint 45's live checks.

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

done: a 7-slide pitch deck for pm is on the site ([the deck](../docs/2026-10-07-pm-pitch-deck.md)), built from raw HTML and inline SVG, ending with the one bootstrap command for pm 0.1.2.

- Slides: the pain (a "this is fine" meme); the idea, with the motivation as four pain-to-remedy cards and a layer diagram; what an agent gets; what the owner gets; why it holds; proof that pm runs itself; the bootstrap command.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **The page renders with at most 7 slides, readable at phone width, and `pm check` passes: met.** The served page has 7 `<section>` slides and no escaped HTML; screenshots at 1280 px (light and dark) and 390 px show no horizontal overflow; `pm check` rendered all 110 pages.
- **The last slide gives the exact bootstrap command for the released version, and it runs: met in part.** It names `pm-v0.1.2`, tagged on origin; the same `uvx --from …@pm-v<version>… pm init` form installed pm into fresh repos in sprint 45's live checks, and the 0.1.2 tool ran three repos here, but a fresh-repo `pm init` at 0.1.2 was not run.
