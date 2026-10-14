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

- The AGENTS.md check (agents md workflow) failed on both CI runs of PR #90 on
  lines the PR does not change, and passed on rerun: run 38625376046
  non_inferable (Map rows 24-25, 4 agreed), run 38630236956 accurate (Map row
  23, 2 agreed of 33 calls; the earlier run had 0 findings of 33). Sprint 33
  owns this flake.

- A third AGENTS.md check run (38630236956, attempt 2) failed non_inferable on
  lines 73-75 (the go-os-tests bullet), also unchanged. Because the PR edits
  CLAUDE.md the check runs in full mode over the whole file, and each run
  flagged different unchanged lines (24-25, 23, 73-75): evidence for sprint 33
  that the full-mode verdict is not stable.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: an owner's reply reaches the session that asked by the pm service's push alone, with the `pm show` flag and `pm reply read` as the safety net; `pm reply wait` and `pm hook inbox` are removed, delivered as [PR #90](https://github.com/Yeeef/pm/pull/90).

- `pm reply wait <id>` and its service code (`WaitReply`, `ReplyPoll`, `MarkWithin`, `Mark` and the rest) removed; raise output and the site's "no push" card name the `pm show` flag and `pm reply read <id>`.
- `pm hook inbox` and its UserPromptSubmit entries removed from both `claudeHooks` and `codexHooks`.
- `pm init`'s hook presence check now counts a pm hook this version does not write as drift, so a first install takes an earlier pm's hook out; `pm upgrade` takes it out of an installed repo; `pm doctor` reports it.
- 11 design pages updated to the new state; changelog entry `changelog.d/reply-delivery-by-push-alone.md` (Removed: next release 0.10.0).
- PR diff: 32 files, +244 / -666 before the review fixes.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| `pm reply wait` and `pm hook inbox` are unknown commands; grep finds them only in the released notes and the entry | Met, with named exceptions | `.go/pm reply wait x` and `.go/pm hook inbox`: "invalid choice", exit 2. `grep -rn "reply wait\|hook inbox"` outside records also finds the tests and one comment for taking the stale hook out (`internal/install/hooks_test.go`, `pieces.go`, `tests/test_init.py`), and this repo's own `.claude/settings.json` and `.codex/hooks.json`, which the pinned 0.9.1 wrote and which still has the command; the pin move rewrites them |
| `pm init` on a scratch clone with the old entry removes it, asserted by a test | Met for a first install; an installed repo by `pm upgrade` (sprint decision) | `test_init_takes_out_an_earlier_pms_inbox_hook` runs `pm init` on a scratch repo seeded with every current hook plus the stale one: 1 failed on the old presence check, passes on the new. `TestAnEarlierPmsInboxHookIsTakenOut` covers Plan (init), Rewrite (upgrade) and Drift (doctor) for both files |
| `make test`, `make test-go`, `make lint` pass; the PR's CI is green | Met | `make test`: 132 passed, 161 skipped; `make test-go` exit 0; `make lint` 0 issues. CI on `fa51b8a`: 13 of 13 checks pass, after the AGENTS.md check was rerun (it failed on three runs on lines the PR does not change; Findings) |
| The reply-delivery page names push and the `pm show` flag as the only routes; listed under Design pages | Met | [Reply delivery](../design/reply-delivery.md), records commits `bca443a` and `75faa6e`; listed above |

Not run: `make test-live-parity`, whose next-prompt check now accepts a push; no live check beyond the integration test that runs `pm init` on a scratch repo.
