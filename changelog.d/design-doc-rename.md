A design page or a doc can be renamed in one command that keeps every record's links to it working.

### Added

- `pm design rename <slug> [--to NEW_SLUG] [--title "…"]` moves `records/design/<slug>.md` to its new slug, sets its
  title, or both, and in the same records commit rewrites every record's link to the page (anchors kept, a link whose
  text was the old title takes the new one) and every `pm design unchanged` waiver of it. It refuses while an open PR
  review need names the page with `--design`.
- `pm doc rename <doc> [--to NEW_SLUG] [--title "…"]` does the same for a doc, named by its path, `<date>-<slug>` or
  slug; its date prefix stays. The feedback doc is not renamed.
