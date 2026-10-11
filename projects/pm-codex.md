---
type: project
title: pm in Codex
bead: pm-v498
---

## Goal

> Why do we do it? What is it? What outcome do we expect?

An agent running in Codex works with pm with the same guarantees as one in Claude Code: it gets the session context, waits for owner replies, cannot leave records uncommitted or owner requests only in chat, and commits records from inside Codex's sandbox. Codex integration grows beyond one sprint, so it is a project of its own, apart from pm-harness.

## Progress

> Where are we now, and what's next? Generated from the work store and the
> sprint records when the page is rendered. Do not write here.

## Decisions

> What constrains every future sprint? Sprint-only choices live in the sprint
> record.

::: decision {source=owner date=2026-10-11}
pm never writes Codex hook trust; pm doctor and pm init name pm hooks that Codex has not trusted or that changed since, taking the trust state from Codex itself (app-server hooks/list) where it can, and tell the user to trust them once in the Codex TUI (/hooks).
Hook trust is Codex review step against repos running commands unasked; its hooks.state hash is internal and undocumented, so writing it would be a hack that bypasses the review.
Answers `pm-v498.1.1`.
:::

## Design pages

> Where is the detail?

- [pm in Codex](../design/pm-in-codex.md): what pm installs for Codex, session identity, owner replies, hook trust, the sandbox, and the parity checks.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
