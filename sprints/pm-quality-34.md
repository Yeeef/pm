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
- Remove `pm hook inbox` and its UserPromptSubmit entries from `claudeHooks` and `codexHooks`; `pm upgrade`, which moves an installed repo to this version, drops the entry from a repo that has it, and so does `pm init` on a first install (on an installed repo neither `pm init` nor session start rewrites the repo's files), so no installed repo keeps calling a removed command.
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

- [Reply delivery](../design/reply-delivery.md): push as the only route, with the `pm show` flag and `pm reply read` as the safety net; the `pm reply wait` and `pm hook inbox` sections gone; Alternatives gives why both were dropped; `covers:` loses the removed files.
- [Replies on the site](../design/site-replies.md): the "no push" card names the `pm show` flag and `pm reply read`.
- [Agent lifecycle](../design/agent-lifecycle.md): the hook tables per runtime without UserPromptSubmit; a Codex question answered in chat closes by the service's check and sweep.
- [Views and the site](../design/views-and-site.md), [Sprint lifecycle](../design/sprint-lifecycle.md), [pm in Codex](../design/pm-in-codex.md), [Owner-request Stop hook](../design/owner-request-hook.md), [pm CLI](../design/pm-cli.md), [pm in Go](../design/pm-go.md), [Project management harness](../design/pm-harness.md), [pm as an installable product](../design/pm-product.md): mentions of the two removed routes replaced by the push.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-14}
Remove both pm reply wait and pm hook inbox; push plus the pm show flag and pm reply read are the only reply routes
Owner in chat, 2026-10-14: neither delivered a real reply in the transcripts on the owner machine; both predate Codex push and complicate the design
:::

::: decision {source=agent date=2026-10-14}
The stale pm hook inbox entry leaves an installed repo by pm upgrade, and a first install by pm init; pm init on an installed repo keeps writing no repo file
pm init plans repo files only without .pm/config.toml (internal/cli/setup/init.go:234), and making session start rewrite committed files is out of this sprint; an installed repo keeps its pinned release, which still has the command, until pm upgrade moves the pin and rewrites the hooks
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Usage before removal, from the transcripts on the owner's machine (about 180 Claude Code sessions in this repo, about 1,760 Codex sessions): `pm hook inbox` handed over 0 real replies on either runtime (all 11 Claude Code matches were agents reading `inbox.go` or its tests); `pm reply wait <id>` ran once outside its own development, with `--timeout 10`; no raise printed the no-inbox route.
- `pm init`'s hook presence check counted a hooks file that held every current hook plus a stale pm hook as present, so on a first install init never took out a hook an earlier pm wrote; only `pm upgrade` did. On an installed repo (one with `.pm/config.toml`) `pm init`, `--session-start` included, plans no repo file at all, so `pm upgrade` remains the step that takes it out there. Now a pm hook this version does not write at that event makes the piece not present. `test_init_takes_out_an_earlier_pms_inbox_hook` fails on the old check (1 failed) and passes on the new one.
- PR #89 (pm-harness sprint 118) extends `pm hook inbox` to close a Codex question answered in chat; `git merge-tree` against this branch conflicts in 9 files. Its service check before queueing and its sweep close that question too, so whichever PR merges second drops the inbox code and regenerates the goldens.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
