---
type: project
title: pm in pi
bead: pm-puvw
---

## Goal

> Why do we do it? What is it? What outcome do we expect?

An agent running in pi (the pi coding agent, `earendil-works/pi`) works with pm with the same guarantees as one in Claude Code or Codex: it gets the session context at start, is known to pm as a live session, asks the owner through a pm ask tool and receives replies, cannot end a turn with records uncommitted or an owner request left only in chat, and the owner-request judge can run on pi itself. pi has no shell hooks; it loads in-process TypeScript extensions, so pm ships a pi extension that bridges pi's events to pm's hook commands. The integration spans several sprints, so it is a project of its own, apart from pm-harness and pm-codex.

## Progress

> Where are we now, and what's next? Generated from the work store and the
> sprint records when the page is rendered. Do not write here.

## Decisions

> What constrains every future sprint? Sprint-only choices live in the sprint
> record.

None yet.

## Design pages

> Where is the detail?

None yet.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
