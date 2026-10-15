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

::: decision {source=agent date=2026-10-15}
The design page pm-go stays as it is for this sprint's code changes.
only a test's expected page count changes, 2 to 3, for the docs index
:::

::: decision {source=owner date=2026-10-15}
pm does not support MDX: the MDX translation is removed, and pm check refuses JSX syntax and blank lines inside <svg> in raw HTML, naming the line and the HTML fix
Plain HTML, image files and Mermaid already cover rich viz; MDX is JSX that a partial subset cannot render, and a write-time check fixes the class for every repo
Answers `pm-bfli.4.8`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: docs typeset TeX math, links between docs resolve by slug, a docs index lists every doc, and pm check refuses dead record links and JSX in raw HTML (PR #102, merged as b242ffd).

Merged as b242ffd (PR #102).

- KaTeX 0.19.0 is pinned on the CDN like Mermaid and loads only on pages that have math.
- MDX is not supported (owner decision). `pm check` and `pm commit` refuse `className=`, `{/* */}`, `attr={…}`, camelCase SVG attributes and blank lines inside `<svg>`, naming the line and the HTML fix.
- Breaking: a store with a dead record link or JSX fails `pm check` once its pin moves. The pm store has 2 dead links (`sprints/pm-harness-50.md`). The poker store has 13 dead links and 219 JSX errors in two imported notes, which its agent rewrites.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Met: Go tests and golden files cover math, links, the docs index and the JSX check. `make test-go` exit 0, `make test` 150 passed, `make lint` 0 issues, and PR #102's CI is green at 61df4dd.
- Partly met: on a scratch pm service with headless Chromium, the unedited CFR note had 32 `.math` and 32 `.katex` elements, with 0 errors and no raw `$` text. A link between two docs returned 200, and `docs/index.html` listed the note. The SVG drew at 960×421 under the MDX translation, which was later dropped; the note now draws only after its rewrite to plain HTML. The JSX check gives 0 errors on ai-safety's 11 SVG-bearing docs, which shows plain HTML SVG passes.
