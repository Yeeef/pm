---
type: sprint
title: Docs render math, MDX diagrams and links, with a docs index
bead: pm-bfli.4
---

## Goal

> What should be true when this sprint ends, and why now?

A doc imported from a research notes repo renders on the site as written: TeX math typeset, inline SVG diagrams drawn, links between docs resolved, and a docs index lists every doc. Now because the poker repo's CFR note (`docs/2026-10-15-note-cfr-zinkevich-2007`) shows raw `$…$` with its TeX escapes stripped, broken diagrams, and 404 links.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Math: `$…$` and `$$…$$` pass through the Markdown renderer untouched (no escape stripping) and are typeset in the browser by KaTeX, pinned on the CDN as Mermaid is.
- MDX-style raw HTML/SVG: `className=` and `{/* … */}` comments render as HTML; the diagram draws.
- Links: a relative link to a doc by its slug (without the date prefix) or by its file name resolves to the doc's page; `pm check` reports a link to a record that does not exist.
- A docs index page at `docs/index.html`, linked from the site's navigation.

**Out:**
- Full MDX (imports, JSX components, expressions).
- Server-side math rendering.
- Editing the poker repo's records.

## Done when

> What evidence will show the goal is met?

- Go tests and golden files cover each of the four cases and pass; `make test` and the PR's CI are green.
- Live check: the CFR note rendered by a scratch pm service shows typeset math (KaTeX elements, no raw `$`), the information-set SVG drawn, a resolved doc-to-doc link, and the docs index listing it.

## Design pages

> Where is the detail?

- [views-and-site](../design/views-and-site.md): math, MDX blocks, links by slug, the broken-link check and the docs index.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
The design page site-replies stays as it is for this sprint's code changes.
serve.go only serves one more page path, docs/index.html; access, replies and the service are untouched
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: docs render TeX math, MDX-style SVG diagrams and links between docs by slug, and a docs index lists every doc (PR #102).

- KaTeX 0.19.0 is pinned on the CDN like Mermaid and loads only on pages that have math.
- An MDX block opened by `<svg` or a `className=` tag runs to its closing tag; JSX attributes and comments are translated. JSX expressions are not.
- `pm check` and `pm commit` refuse a dead record link. This is breaking: this store's `sprints/pm-harness-50.md` has 2 dead links and the poker store has 13.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: Go tests and golden files cover math, MDX, links and the docs index. `make test-go` exit 0, `make test` 150 passed, `make lint` 0 issues, and PR #102's CI is green.
- Met: on a scratch pm service with headless Chromium, the unedited CFR note had 32 `.math` elements, 32 `.katex` and 0 `.katex-error`, with no raw `$` text. The information-set SVG drew at 960×421. A link between two docs resolved by slug and returned 200. `docs/index.html` listed the note.
