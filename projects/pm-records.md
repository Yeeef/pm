---
type: project
title: Next generation Record Layer
bead: pm-c72a
---

## Goal

> Why do we do it? What is it? What outcome do we expect?

The owner sees records change live, before a sprint commits them and before its code PR merges, and agents never step on each other's record edits. Today every session edits one shared working tree of the `records` branch, `pm commit` commits whole files, and the site renders that tree: a commit can sweep in another session's half-made edit, the owner sees live edits with no mark of whose or how finished they are, and a design page describes its sprint's code before the PR is on main. The record layer gives each session its own copy to write in, publishes an edit only by a commit that refuses to overwrite another session's change, shows the owner the published records with each session's or sprint's pending edits beside them as labelled drafts, and publishes a design page's edits when its sprint's PR merges.

## Progress

> Where are we now, and what's next? Generated from the work store and the
> sprint records when the page is rendered. Do not write here.

## Decisions

> What constrains every future sprint? Sprint-only choices live in the sprint
> record.

::: decision {source=owner date=2026-10-15}
The record layer builds Model A of the record-drafts design: every record stays in the records store, each code worktree writes in a private records workspace, journals publish at pm commit, and design-page edits publish when their code PR merges.
Owner choice over the agent default B. A meets all five goals for every record type, journals included, and keeps one store and the rule that no code branch tracks records.
Answers `pm-c72a.1.3.1`.
:::

::: decision {source=owner date=2026-10-15}
Design-page drafts publish at pm sprint close, not when the pm service sees a PR merge; pm sprint close --discard <slug> --reason drops a page draft instead, recorded as a sprint decision.
The closing session can resolve a conflict with a page another sprint published first, where the service could only flag it after the code merged; sprint close already checks the PRs and covers on main, so the service needs no publish step, no gh poll and no branch-to-worktree match.
:::

::: decision {source=owner date=2026-10-15}
The agent delivers pm-records end to end: it merges the project PRs itself through make merge-ready, and finishes by releasing and moving this repo pin to a pm release that carries the record layer.
The owner handed the project over in chat; their reviews stop gating each PR, so the agent runs the fresh-context reviews and the merge-ready check itself.
:::

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): two candidate models for drafts and publishing, shadow records branches (A) and design pages in the code tree (B), with their comparison and increments

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

The goal is met on this repo, which runs pm 0.15.2: each worktree writes records in its own workspace, `pm commit` refuses an edit that overlaps another's, the owner reads the published records with every draft beside them as a labelled version, and a design page goes live only at its sprint's close, once its code is on main.

**Against the goal**

| Goal | Result | Evidence |
|---|---|---|
| The owner sees records change before they are committed | Met | Every worktree's workspace and every other clone's committed draft is a version of each page it changes, at `/drafts/<draft>/…`, with a diff by section; live on this repo: a hand edit showed as its worktree's version, the published page unchanged ([sprint 6](../sprints/pm-records-6.md)) |
| Agents never step on each other's record edits | Met | A private workspace per worktree; follow by `git merge-tree` and `git merge-file`, never rebase, merge or stash; an overlap is refused naming the commit, markers only in the second workspace ([sprint 4](../sprints/pm-records-4.md)) |
| Published and draft are told apart | Met | The site renders the `records` branch, never a working tree; a broken draft fails only its own version ([sprint 2](../sprints/pm-records-2.md)) |
| A design page describes its code only once the code merges | Met | `pm sprint close` publishes the sprint's drafts in one `records` commit after its PRs and covers are on main; `--discard` drops one with its reason; sprint 6's close published its own two pages that way in 1.7 s ([sprint 5](../sprints/pm-records-5.md), [sprint 6](../sprints/pm-records-6.md)) |
| Across clones | Met | Per-clone draft refs pushed with a lease, never forced; a close from any clone publishes them ([sprint 6](../sprints/pm-records-6.md)) |

**Numbers measured:** a workspace at session start 0.11 to 0.14 s; `pm commit` from a workspace 0.06 to 0.31 s; a close publishing two drafts 0.58 s on a scratch clone, 1.7 s on this repo; a draft listed on another clone 0.25 s after its sync.

**What was learned**

- A per-file version check in `pm commit` refuses the second writer without separating the two edits; isolation needs a private copy per writer and a merge by hunk ([sprint 1](../sprints/pm-records-1.md)).
- Publishing at sprint close, the owner's choice, beat publishing when the service sees the merge: the closing session can resolve a conflict, and no service step or PR polling is needed.
- `claude -w` isolation goes by path prefix, so a workspace nested in the worktree is writable with no extra settings; Codex's sandboxed agent stays untested here, since its sandbox cannot start on this host.
- The pin move from 0.11.0 needed one published covers pattern fixed first: under the new rules a page may not name code main lacks.
- Fresh-context reviews found every data-loss risk before merge (a reflog-dependent base, `merge.autoStash`, stranded text lost to `git worktree remove`); each is fixed with a test.

**What was retired:** the store's working tree as the site's source; the Stop hook's transcript match in worktrees; the covers waiting list and the close's covers hold; `pm init`'s Claude `additionalDirectories` entry for worktrees.

**Left open** ([Record drafts and publishing](../design/record-drafts.md), Open questions; [Shared records store](../design/records-store.md)): whether a sandboxed Codex agent can commit in its workspace; two clones committing the same day's summary conflict in the records rebase.

**Delivery reports:** [sprint 1](../sprints/pm-records-1.md), [sprint 2](../sprints/pm-records-2.md), [sprint 3](../sprints/pm-records-3.md) (design and doc rename, another session), [sprint 4](../sprints/pm-records-4.md), [sprint 5](../sprints/pm-records-5.md), [sprint 6](../sprints/pm-records-6.md).
