Docs render as written: TeX math is typeset, MDX-style SVG diagrams draw, links between docs resolve by slug, and a
docs index lists every doc; `pm check` refuses a link to a record that does not exist.

### Upgrade guide

1. Run `pm check`. For each link it names, point the link at an existing record's file, or at a doc's or a postmortem's
   slug, or remove the link. Then commit the fixed records with `pm commit`.

### Breaking changes

- `pm check` and `pm commit` refuse a store in which a relative `.md` link in a record names no record. The error
  names each file and its link. The page still renders on the site. Before, such a link was a 404 on the site and
  nothing reported it.

### Added

- Math: `$…$` inline and `$$…$$` display math reach the page exactly as written, and KaTeX typesets them in the
  browser. KaTeX loads from a CDN at one exact version, only on pages with math. A `$` in code, a price such as `$5`
  and an escaped `\$` stay text.
- A docs index at `docs/index.html` lists every doc, newest first, with its date, its project and its bead. Every
  page's navigation links to it.

### Fixed

- MDX-style raw HTML now renders. A block whose tag has `className`, or an `<svg>` block, is one HTML block up to the
  line that closes its tag, blank lines included. `className` becomes `class`, JSX presentation attributes such as
  `strokeWidth` get their SVG names, and `{/* … */}` comments are dropped. An inline SVG with no width of its own
  fills the column, up to 960px wide.
- A relative link to a doc or a postmortem by its slug alone, without the date (`[x](note-foo.md)`), opens that
  record's page and is no longer a 404.
