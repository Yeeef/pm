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

::: decision {source=agent date=2026-10-11 until="a Codex release adds a reachable app-server, an ask tool, or worktree-local config"}
Three Codex gaps stay open: an idle Codex TUI is not woken by a reply (it gets it at its next prompt, or waits with pm reply wait); a Codex agent has no AskUserQuestion, so its only ask is a need; Codex reads hooks and rules from the main checkout, so a branch change to them applies once main has it.
Codex 0.154 offers no outside path into a TUI (embedded app-server), no ask-the-user tool outside plan mode, and no worktree-local .codex; each is Codex behaviour pm cannot change, and each degrades no guarantee: the need path and pm show cover them.
:::

::: decision {source=owner date=2026-10-12}
The owner-request judge runs on the session runtime: Haiku through claude -p for a Claude Code session, a small model through codex exec for a Codex session, named by each runtime hook entry, never guessed. pm day summarize runs on a per-repo runtime set in .pm/config.toml, with no automatic fallback. The work lands in a new sprint after PR #47.
Owner, in chat 2026-10-12: a Codex user should not need claude installed; without it the judge fails and Codex treats the failure as a silent pass, so the owner-request check is lost.
:::

::: decision {source=owner date=2026-10-12}
Every pm-codex sprint brings the existing design page of each area it changes to the new state (reply delivery, session claims, the owner-request hook, setup and the service), and lists those pages in its Design pages section; pm-in-codex stays the overview that links them.
Owner, in chat 2026-10-12: sprint 1 left reply-delivery.md and the other area pages describing the old design, with Codex as a non-goal; the design lived only on the new pm-in-codex page.
:::

::: decision {source=owner date=2026-10-12}
Gap 4 is not accepted: an idle Codex session waiting on the owner must be woken by the reply without the user typing, by whatever mechanism Codex supports, not necessarily the Claude Code push. This replaces gap 4 in the decision that left three gaps open; gaps 5 and 9 stay left.
Owner, in chat 2026-10-12: a reply that waits for the next prompt is not delivery for a session waiting on the owner; look for other ways if the Claude Code mechanism does not carry over to Codex.
:::

::: decision {source=owner date=2026-10-13}
Gap 9 is closed, not left: pm rule names each runtime ask tool (Claude Code AskUserQuestion; Codex request_user_input_async in Default mode and request_user_input in Plan mode), and the owner-request hook treats a call to any of them as an ask made with the tool. Lands in PR #56 (pm-codex sprint 2). This replaces gap 9 in the decision that left gaps open; gap 5 stays left.
Owner, in chat 2026-10-13, after research refuted the premise that Codex has no ask tool: Codex 0.154 offers request_user_input_async to gpt-6 models in Default mode and a blocking request_user_input in Plan mode, both measured in the TUI.
:::

## Design pages

> Where is the detail?

- [pm in Codex](../design/pm-in-codex.md): what pm installs for Codex, session identity, owner replies, hook trust, the sandbox, and the parity checks.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
