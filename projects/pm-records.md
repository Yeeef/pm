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

None yet.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): two candidate models for drafts and publishing, shadow records branches (A) and design pages in the code tree (B), with their comparison and increments

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
