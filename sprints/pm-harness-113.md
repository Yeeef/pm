---
type: sprint
title: A marketing website for pm on GitHub Pages
bead: yeeef-agents-9va.123
---

## Goal

> What should be true when this sprint ends, and why now?

pm has a public marketing website that sells it in one page to someone who runs coding agents: what pm is (a
human-agent project management suite), why it beats a pack of prompt files (it structurally enforces the agents'
decisions and actions through the `pm` cli and hooks), the outcomes, the site as the owner's view of a swarm of
agents, and how to install it. Why now: pm has no public face; the repo's README is its only description.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A static, single-page site in `website/` (HTML and CSS, no build step), with the site screenshots from sprint 112 and
  real commands.
- A GitHub Actions workflow that deploys `website/` to GitHub Pages on a push to main that changes it.
- The README links the website.
- Each claim the page makes checked against the code, as in sprint 112.

**Out:**
- A custom domain, analytics, a docs site, a blog.
- Changes to the pm site (the owner's interface) or the binary.

## Done when

> What evidence will show the goal is met?

- A private preview of the page is shared with the owner, and it renders at phone and desktop widths with no
  horizontal scroll (checked in a headless browser at 390 and 1440 pixels wide).
- The PR is merged to main and the Pages deploy serves the page at its public URL.

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

- The page renders with no horizontal scroll in headless Chromium at 390 and
  1440 px, light and dark: scrollWidth 390/390 and 1440/1440 in all four runs
  (first run at 390 overflowed to 500 px from the outcome grid's min-content
  width; fixed with min-width: 0). Copy buttons put the install command and pm
  init on the clipboard.

- Claims checked against the code: pm init adds no line to AGENTS.md or
  CLAUDE.md (pieces.go; it only removes a Beads block); refusals quoted
  verbatim from internal/cli writes.go and needs.go and
  prompts/owner_request_reason.txt; sync under refs/pm/work. Dropped: a claim
  that pm task claim refuses a blocked task. It does not: work.Claim checks
  status and live holder only, and 123.1 was claimed while blocked by 122.2.

- pm task close with no --reason closes the task as done with reason Done, and
  pm has no reopen: 123.2 was closed by mistake at 17:18 UTC while reading
  refusals; a comment on it records that and later the commit (5c9b2df) that
  holds the work.

- Private preview of the page published to the owner as a claude.ai artifact
  (https://claude.ai/artifact/XDBSuhWDzBNAxQU9e5oydG) from website/ at
  c946172, with style.css and the four screenshots.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm has a one-page marketing website in `website/` and a workflow that deploys it to GitHub Pages (PR #42, stacked on #41); merged as e122a0c and live at https://yeeef.github.io/pm/.

- `website/index.html`, `website/style.css`: hero with install, the three parts, "structural, not prompts" with pm's refusals quoted verbatim, five outcomes, the site gallery, an inline SVG diagram of how pm works, get started. Dark first, light by system setting; no build step, no framework.
- `.github/workflows/website.yml`: deploys `website/` to GitHub Pages on a push to main that changes it, or by hand.
- README links the website; AGENTS.md's layout names `website/` and the workflow.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Private preview shared with the owner, rendering at phone and desktop widths with no horizontal scroll: met. Preview: https://claude.ai/artifact/XDBSuhWDzBNAxQU9e5oydG (version 2, at deb2b26). Headless Chromium: scrollWidth 390/390 at 390 px and 1440/1440 at 1440 px, light and dark. The fresh-context review's overclaims on the page (outcome 02 implied a stray PLANNING.md is blocked; "the only way in" held only for the work store; "tens of agents" beyond the evidence; the `claude` CLI caveat; the 30-minute idle rule is per machine) are fixed in deb2b26. CI on deb2b26: 9 of 9 checks passed.
- The PR is merged and the Pages deploy serves the page at its public URL: met. Merged as e122a0c. The first deploy (run 38165264739) failed at `actions/configure-pages` because Pages was not yet enabled; after the owner enabled it, `gh workflow run website.yml` (run 38165338912) succeeded, and https://yeeef.github.io/pm/ answers 200 (35,837 bytes, the page's title), as do `img/overview.png` and `style.css`.
