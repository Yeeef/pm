---
type: doc
title: "Record layer: concurrent edits, live view and records ahead of code"
date: 2026-10-14
project: pm-records
---

The starting analysis for the [pm-records](../projects/pm-records.md) project: what went wrong, the prior art, and a proposed design with its increments. It is a proposal, not a decision; the project's sprints decide.

## The owner's constraints

- The owner wants to see records change before they are committed to the `records` branch and before a sprint's code PR merges: live edit and live view.
- Agents must not step on each other's record edits.

## What happened (2026-10-14)

- pm-quality sprint 34 (PR #90) and pm-harness sprint 118 (PR #89) edited the same 8 design pages. Sprint 34's commit `bca443a` landed at 21:36; the sprint 118 session began editing those pages at 21:38, on top of it. Nothing was lost, but only by that two-minute gap.
- The Stop hook's uncommitted-records check blocked sprint 34's session twice for files it named: once a page its own subagent was mid-edit on, once the other session's pending edits. It attributes by the paths a session's tool calls named, not by who wrote the change.
- The design pages described pm without `pm hook inbox` while main still had it, and #89's pages described its own branch's question flow: two open PRs, one shared page set.

## The three problems

| Problem | Database name | Today |
|---|---|---|
| Two agents write one file | Dirty write, lost update | One shared working tree of `records` (`internal/store`); `pm commit` commits whole files, so one session's commit can carry another's half-made edit |
| The owner wants work in progress | Dirty read, wanted on purpose | The site renders the working tree (`records.Read` over the files), so the owner sees live edits, unlabelled by session and with no mark of done |
| Records describe code before it merges | Atomicity across two stores | A design page edit is published at `pm commit`, while its code lands at the PR's merge |

## Prior art

| Source | Idea | Fit |
|---|---|---|
| MVCC, snapshot isolation | Each transaction writes privately, publishes at commit | Per-session records workspace |
| Optimistic concurrency control (Kung and Robinson, 1981); HTTP `If-Match`/ETag (RFC 9110); Wikipedia's edit conflict | A write carries the version it was based on; a stale base is refused | `pm commit` refuses a file changed since this session's base |
| Preview deployments (Vercel, Netlify) | Each branch gets its own rendered preview; the main site shows only merged work | Drafts beside the published page |
| CMS draft and publish (WordPress revisions, Contentful environments) | Drafts visible to reviewers and marked as drafts; readers see published | The site's default view and the draft marks |
| Docs-as-code | Docs change in the code PR and merge with it | Solves records ahead of code, but breaks pm's rule that no code branch tracks `records/` |
| OT (Google Docs), CRDTs (Automerge, Yjs) | Merge concurrent keystrokes | Overkill: agents make discrete edits to sectioned Markdown, which git's line-based three-way merge handles |
| Jujutsu (jj) | The working copy is a commit; conflicts are first-class objects, not blocking errors | A model if concurrent writers should never be refused |
| Dolt branches | Already pm's work store engine | Shows the per-writer branch model pm could reuse for records |

## Proposed design

1. **A private records workspace per session**, as pm already makes code worktrees: a worktree of `records` per session. `pm commit` rebases the session's commits onto `records`: disjoint hunks merge on their own; an overlapping one is refused, naming the other session's commit. Nothing is swept in silently.
2. **The site shows committed records by default, with drafts beside them.** A page with pending edits says so, for example "Draft edits: sprint 34 (session 7980…), sprint 118", each linking a rendered preview or a diff. The owner keeps the live view, labelled by whose it is, never mixed into the published page.
3. **Two kinds of record, published at different times.**
   - Journals (sprint and project records, findings, decisions, docs) are facts as they occur: published at `pm commit`.
   - Design pages state the code's state: their edits stay on a per-sprint records branch and publish when the sprint's PR merges, through `pm sprint close`, which already checks the PR is on main. Until then the page shows them as that sprint's draft.

## Increments

| Step | Fixes | Leaves |
|---|---|---|
| Optimistic concurrency in `pm commit`: keep the file's hash at this session's last write; refuse when it moved | The silent sweep | Unlabelled live edits; records ahead of code |
| Steps 1 and 2 | Isolation and a labelled live view | Records ahead of code |
| Step 3 | Design pages publish with their code | Nothing of the three |

## Open questions

- Per-session or per-sprint workspaces: a sprint can span sessions, and a session can touch two sprints.
- What the Stop hook's uncommitted-records check becomes once each session has its own workspace.
- How generated records (day pages, `Progress`) and the service's sync fit a store with many workspaces.
- How a draft preview is served without letting a draft's broken record break the published page.
- Cost: what the service pays to render drafts per session, and how long a stale workspace lives.
