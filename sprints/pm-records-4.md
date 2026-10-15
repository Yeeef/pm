---
type: sprint
title: Per-worktree records workspaces and journal publish
bead: pm-c72a.4
---

## Goal

> What should be true when this sprint ends, and why now?

Each code worktree edits records in its own workspace, so no session's commit can carry another's edit and the owner sees each worktree's edits as its own labelled version. Today every session edits the one shared store and `pm commit` commits whole files, which is how two sprints edited the same 8 design pages on 2026-10-14. This is step 2 of [record-drafts](../design/record-drafts.md): workspaces at `<worktree>/records`, follow, and journals published at `pm commit` with overlaps refused; the site's versions (sprint 2) gain one version per workspace.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A workspace per code worktree: a git worktree of `drafts/<name>` at `<worktree>/records`, in place of the link; `pm init` makes it, and turns an existing worktree's link into one at that worktree's own session start.
- Follow, by `git merge-tree` and `git merge-file`, never rebase, merge or stash; no workspace left mid-operation.
- `pm commit` and one-shot writes from a worktree: follow, commit journals on `records`, refuse a file with conflict markers naming the commits that touched it.
- Each workspace as a draft on the site, labelled with worktree, branch, sprint and live session.
- Design pages committed from a workspace go to its draft commit tagged with their sprint. Until step 4 builds publish at sprint close, they publish at `pm commit` as journals do.

**Out:**
- Publish at sprint close and `--discard` (step 4); the Stop hook and `pm clean` guards (step 3); draft refs across clones (step 5).
- Codex's sandboxed agent writing a workspace (untestable on this host).

## Done when

> What evidence will show the goal is met?

- In a scratch clone with two worktrees: disjoint edits to one sprint record from both reach `records`; an overlapping edit is refused at the second `pm commit`, naming the first commit, with markers only in the second workspace (a test).
- A one-shot write merges into another workspace's uncommitted edit of the same file and leaves no workspace mid-operation (a test).
- The site lists each workspace as a version of the pages it changes, labelled with its worktree (a test and a live check on a scratch clone).
- A `claude -w` session writes and commits a record from its own worktree with no extra settings (a live check in a scratch worktree).
- CI green on the sprint's PR.

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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
