---
type: sprint
title: The website installs the latest release
bead: yeeef-agents-9va.124
---

## Goal

> What should be true when this sprint ends, and why now?

The public website (https://yeeef.github.io/pm/) points new users at the latest pm release, and stays right after each release without an edit. Today it hardcodes pm-v0.6.0 while the latest is 0.7.1.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The two install commands and the version pill in `website/index.html` use GitHub's `releases/latest` URLs.

**Out:**
- The quoted refusal transcript's "verbatim from pm 0.6.0" label, which dates a quote.
- README and other docs.

## Done when

> What evidence will show the goal is met?

`website/index.html` names no pinned release in its install commands or pill; `curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh` serves the latest release's script (version 0.7.1 today); the PR is on main.

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

done: the website's install commands and pill point at GitHub's latest release, so they stay current after each release.

- `website/index.html`: both `curl … install.sh` commands use `releases/latest/download/install.sh`; the pill reads "Latest release" and links to `releases/latest`; step 1 says it installs the latest release.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- No pinned release in the install commands or pill: met; the only `0.6.0` left is the dated transcript label, out of scope.
- `releases/latest/download/install.sh` serves the latest script: met; it holds `version='0.7.1'`, and `releases/latest` redirects to `pm-v0.7.1`.
- Page renders at phone width: 375px and 1280px give `scrollWidth` equal to the viewport.
- PR on main: PR #52, CI green; waits on review.
