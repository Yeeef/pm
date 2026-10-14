---
type: sprint
title: "Remove pm reply wait and pm hook inbox: owner replies reach a session by push alone"
bead: pm-d2k5.34
---

## Goal

> What should be true when this sprint ends, and why now?

An owner's reply reaches the session that asked by one route: the service's push (Claude Code socket, `codex queue`), with the `pm show` flag and `pm reply read` as the safety net for a session that did not get it. The two fallback routes, `pm reply wait` and the UserPromptSubmit hook `pm hook inbox`, go. In the transcripts on the owner's machine (about 180 Claude Code and 1,760 Codex sessions) neither ever delivered a real reply; push delivered every one. They date from before Codex had push, and they make the reply-delivery design page and every raise's output harder to follow.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Remove the `pm reply wait` command, the site/service endpoint only it uses, and its mentions in raise outputs, `--help` texts, hints and prime.md.
- Remove `pm hook inbox` and its UserPromptSubmit entries from `claudeHooks` and `codexHooks`; `pm init` and `pm upgrade` drop the entry from a repo that has it, so no installed repo keeps calling a removed command.
- A Codex question need answered in chat closes by the service's check before queueing and its sweep alone.
- Update the reply-delivery design page to the new state (routes, diagram, questions, duplicates, alternatives, open questions), CLAUDE.md's hook rule, tests, goldens, the parity eval's docstring, and a changelog entry.

**Out:**
- Any change to push, the sweep, `pm reply read`, `pm hook ask` or the `pm show` flag.

## Done when

> What evidence will show the goal is met?

- `pm reply wait` and `pm hook inbox` are unknown commands, and `grep -rn "reply wait\|hook inbox"` finds them only in CHANGELOG.md's released notes and the changelog entry.
- `pm init` on a scratch clone whose `.claude/settings.json` and Codex hooks carry the old `pm hook inbox` entry removes it (a test asserts it).
- `make test`, `make test-go` and `make lint` pass, and the PR's CI is green.
- The reply-delivery design page names push and the `pm show` flag as the only routes, and the PR lists it under the sprint's Design pages.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-14}
Remove both pm reply wait and pm hook inbox; push plus the pm show flag and pm reply read are the only reply routes
Owner in chat, 2026-10-14: neither delivered a real reply in the transcripts on the owner machine; both predate Codex push and complicate the design
:::

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
