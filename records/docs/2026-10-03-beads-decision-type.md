---
type: doc
title: Beads decision issues versus record decision blocks
date: 2026-10-03
bead: yeeef-agents-9va.6.1
---

Can Beads' `decision` issue type (alias `adr`) replace the `::: decision`
blocks in project and sprint records, and can a decision's `date` be dropped
in favour of git history? Open question 1 on the design page. Checked against
`bd` 1.3.1 in a scratch database on 2026-10-03.

**Recommendation: keep `::: decision` blocks, and keep `date`.** Beads'
decision type is an ordinary issue with a body template; everything that makes
a record decision useful (level, source, reason, `until`, being read with the
record it governs) would have to be rebuilt as labels and metadata conventions,
and decisions would start appearing in the ready queue as work. Git dates are
commit dates, not decision dates: they differ for 4 of the 16 project
decisions on this repo.

## What Beads offers

- **The type.** `bd types` lists `decision` ("Architecture decision record
  (ADR)") among the core work types; `bd create --type=decision` (or `dec`,
  `adr`) makes one. It is an ordinary issue: id, title, description, status,
  priority, labels, parent, dependencies, comments, `created_at`,
  `closed_at`, and a free `metadata` JSON object (`--metadata`).
- **A body template, enforced only on request.** `bd create --validate`
  refuses a decision whose description lacks `## Decision`, `## Rationale`
  and `## Alternatives Considered`. Without `--validate` any body is
  accepted.
- **Superseding.** `bd supersede <old> --with <new>` closes the old issue
  and adds a `supersedes` dependency to the new one. This is the one feature
  records do not have.
- **No decision-specific commands** beyond that: no list of active
  decisions, no level, no source, no review date. `bd list --type=decision`
  filters like any type.
- **JSON.** `bd show --json` returns the usual issue object with
  `issue_type: "decision"`, the description as one Markdown string, labels,
  `parent`, and `dependencies` (with `dependency_type: "parent-child"` or
  `"supersedes"`).

Observed behaviour that matters for us:

- An open decision issue is returned by `bd ready` and by
  `bd ready --exclude-type=epic`, so a standing decision shows up as
  claimable work. Avoiding that means creating it closed
  (`bd create -s closed` works), which inverts what "open" means for a
  decision that is still in force.
- Level, source and `until` have no fields; they would be labels
  (`level:project`) or `--metadata '{"source":"agent","until":"…"}'`. Both
  are accepted; neither is checked or shown by any `bd` view.

## Comparison

| Concern | `::: decision` block in a record | Beads `decision` issue |
|---|---|---|
| Level | Where the block lives: the project or sprint record it governs | A convention: parent epic plus a `level:` label; nothing checks it |
| Source | `source=owner\|agent`, required; `pm` sets it, `--need`/`--confirmed` for owner | No field; metadata or a label by convention; `created_by` is the git user, the same for owner and agents |
| Reason | Body must state it; `pm decision add` refuses a one-line body | `## Rationale` section, enforced only with `--validate` |
| Until | `until="…"` attribute, rendered on the page | No field; metadata by convention, or `--defer`/`--due`, which mean scheduling, not review |
| Date | `date=` attribute, the day it was decided | `created_at`, the day the issue was created |
| Superseding | By hand: edit or remove the block | `bd supersede` closes the old one and links the new one |
| Rendering | Already rendered in its record; `pm show` lists the last three | New renderer and `pm show` code to fetch, filter and place decision issues |
| Read with the context | Yes: an agent reading a sprint record sees its decisions | No: a separate `bd` query per record |
| Ready queue | Not work, never in it | Open decisions appear in `bd ready` |
| Answered needs | `make render` checks a decision cites each answered need | Would need a `bd dep` link per answer, plus a new check |
| Cost of switching | None | Migrate 21 blocks (16 project, 5 sprint); rewrite `pm decision add`, `pm need respond`, the renderer's decision rendering and the answered-need check; add label and metadata conventions and their checks |

## Is `date` needed, given git history?

Yes. `git blame` gives the commit that last touched a line, not the day of
the decision:

- On this repo, 4 of the 16 project decisions are dated 2026-09-30 or
  2026-10-01, but their lines were committed on 2026-10-02, because they
  were recorded after they were made.
- Rewording, reflowing or moving a block changes its blame date; a rebase
  rewrites commit dates.
- `pm show` sorts and prints decisions by date from the record text alone;
  using git would add a `git blame` per record to every call.

The attribute costs a few tokens per decision and is set by `pm`, so it
cannot be mistyped.

## What to borrow

Superseding is real value. If replacing a decision becomes common, add an
optional `supersedes` reference to the block rather than switch types. Not
needed yet: no decision on this repo has been replaced.
