Docs render as written: TeX math is typeset, links between docs resolve by slug, and a docs index lists every doc;
`pm check` refuses a link to a record that does not exist, and JSX in raw HTML. pm does not support MDX.

### Upgrade guide

1. Run `pm check`. For each link it names, point the link at an existing record's file, or at a doc's or a postmortem's
   slug, or remove the link. Then commit the fixed records with `pm commit`.
2. For each JSX line it names in raw HTML, apply the fix the error gives: `class=` for `className=`, `<!-- -->` for
   `{/* … */}`, the computed value in quotes for `attr={…}`, the SVG name such as `stroke-width` for `strokeWidth`, and
   no blank line between `<svg` and `</svg>`. Then commit the fixed records with `pm commit`.

### Breaking changes

- `pm check` and `pm commit` refuse a store in which a relative `.md` link in a record names no record. The error
  names each file and its link. The page still renders on the site. Before, such a link was a 404 on the site and
  nothing reported it.
- `pm check` and `pm commit` refuse a record whose raw HTML, outside code, holds JSX: `className=`, a `{/* … */}`
  comment, an `attr={…}` value, or a camelCase SVG attribute such as `strokeWidth`, `textAnchor` or `fontSize`; or a
  blank line between `<svg` and its `</svg>`, which ends the HTML block and turns the rest of the drawing into text.
  Each error names the file, the line and the HTML fix. Records are CommonMark: pm does not support MDX. Before, such
  HTML rendered broken and nothing reported it.

### Added

- Math: `$…$` inline and `$$…$$` display math reach the page exactly as written, and KaTeX typesets them in the
  browser. KaTeX loads from a CDN at one exact version, only on pages with math. A `$` in code, a price such as `$5`
  and an escaped `\$` stay text.
- A docs index at `docs/index.html` lists every doc, newest first, with its date, its project and its bead. Every
  page's navigation links to it.

### Fixed

- An inline SVG with no width of its own fills the column, up to 960px wide.
- A relative link to a doc or a postmortem by its slug alone, without the date (`[x](note-foo.md)`), opens that
  record's page and is no longer a 404.
