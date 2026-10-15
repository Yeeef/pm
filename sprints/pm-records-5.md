---
type: sprint
title: Design pages publish at sprint close
bead: pm-c72a.5
---

## Goal

> What should be true when this sprint ends, and why now?

A design page's edits reach the published records only when its sprint closes, with its code on main, so a page never describes code before the code merges, and two sprints' drafts of one page meet at the close where the closing session can resolve them. Since sprint 4 a design page is tagged with its sprint in the workspace's draft commit but still published at `pm commit`. This is steps 3 and 4 of [record-drafts](../design/record-drafts.md), with the owner's rule that the close publishes and `--discard` drops a page's draft.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm commit` keeps design pages in the workspace's draft commit, tagged with their sprint, and stops publishing them; the tag survives a rename on `records`, or the untagged path becomes a conflict.
- `pm sprint close` publishes every page drafted for the sprint, from every workspace and `drafts/*` branch, in one `records` commit, after its existing checks; an overlap refuses the close naming the commit.
- `pm sprint close --discard <slug> --reason "…"`, recorded as a sprint decision.
- `pm check` holds published covers patterns to main; the waiting list and the close's covers hold retire.
- `pm where` lists `drafts/` branches whose worktree is gone.

**Out:**
- Draft refs across clones (step 5).

## Done when

> What evidence will show the goal is met?

- A scratch sprint with design-page drafts in two worktrees: the published page is unchanged until the close, then the close publishes both drafts in one `records` commit (a test and a live check on a scratch clone).
- A page another sprint published first refuses the close, naming that commit; after the agent resolves and commits in its workspace, the close passes (a test).
- `--discard` drops the draft and records the sprint decision (a test).
- `pm where` lists a `drafts/` branch whose worktree is gone (a test).
- CI green on the sprint's PR, merged through `make merge-ready`.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Publish at sprint close, Covers, Increments 3 and 4
- [Sprint lifecycle](../design/sprint-lifecycle.md): Close, Design pages follow the code, Covers hold on main at the close
- [Shared records store](../design/records-store.md): pm where's drafts line; pm commit keeps design pages as drafts
- [Record layer](../design/record-layer.md): covers held to main
- [pm CLI](../design/pm-cli.md): pm sprint close, pm commit, pm where
- [Agent lifecycle](../design/agent-lifecycle.md): design pages stay drafts until the close

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-16}
The design page pm-in-codex stays as it is for this sprint's code changes.
codex.go gained CodexState, the writable-roots state pm where prints, moved from internal/cli unchanged but for the parse message; the hooks, rules and writable_roots pm writes are unchanged
:::

::: decision {source=agent date=2026-10-16}
The design page pm-product stays as it is for this sprint's code changes.
codex.go's CodexState move, uninstall's refusal naming pm sprint close and prime.md's close step change no install, pin, service or product behaviour the page describes
:::

::: decision {source=agent date=2026-10-16}
A rename on records of a drafted page carries the page sprint tag to its new path at the follow, rather than turning the untagged path into a conflict.
Carrying keeps the draft committed and attributed, so a close from any worktree publishes it; a conflict would make it an uncommitted edit that only its own worktree sees, and a close elsewhere would silently leave it out.
:::

::: decision {source=agent date=2026-10-16}
The close drops a published page from a live workspace by removing its sprint line from the draft commit, tree kept; that workspace own next follow drops the page, and only a branch with no worktree has its tree rewritten.
Rewriting a live workspace tree or index would write another worktree files, which follow never does; an untagged page is ignored by the site, pm clean and the gate until then.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check of publish at sprint close (this branch's .go/pm 0.9.0-dev,
  commit 7d32fbc) in a scratch clone of a scratch bare origin under
  /tmp/pmlc9Uyh, HOME, CODEX_HOME and CLAUDE_CONFIG_DIR there, the harness's
  fake_sched as supervisor (the service served the site): pm init 1.36 s;
  worktrees one and two made records workspaces by pm init --session-start;
  each edited a different paragraph of design/parser.md and ran pm commit
  --sprint r-vvlj.1: 'drafted records/design/parser.md for sprint r-vvlj.1 on
  drafts/one: pm sprint close r-vvlj.1 publishes it', 0.25 s and 0.24 s.
  Before the close: records:design/parser.md held neither edit (0 and 0), the
  site's design/parser.html neither edit with both versions listed (worktree
  one on one, worktree two on two), drafts/one/design/parser.html held one's
  edit. pm sprint close r-vvlj.1 from one, 0.58 s: 'published
  design/parser.md', exactly 1 records commit (065e2f3 '[SPRINT] demo sprint
  1: closed' with 'Published: design/parser.md r-vvlj.1'), holding both edits;
  the site's page showed both 2 s later; drafts/one and drafts/two then tagged
  0 pages.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: design-page edits stay drafts until `pm sprint close` publishes them in one `records` commit with the sprint's code on main, and `--discard` drops a draft with its reason recorded.

- `pm commit` keeps a design page in the workspace's draft commit, tagged with one sprint; the tag survives a rename on `records`; a follow conflict keeps a mark that holds the close.
- `pm sprint close` publishes every page drafted for the sprint, from every workspace and `drafts/*` branch, after its existing checks; an overlap or a conflict mark refuses it and changes nothing; a rerun is safe; `--discard` is durable.
- Covers: published patterns are held to main and wait only on a draft that drops the lost one; the waiting list and the close's covers hold retired.
- `pm where` lists `drafts/` branches whose worktree is gone.
- One unreadable workspace fails only its own operations; elsewhere it is a named warning.

### Against "Done when"

- Drafts in two worktrees: the published page is unchanged until the close, then both drafts publish in one `records` commit: met. Go tests in `internal/cli/sprint_publish_test.go` and `internal/store`; live check: before the close `records` and the site held neither edit, the close took 0.58 s and made exactly 1 commit holding both, the site showed them 2 s later (Findings).
- A page another sprint published first refuses the close naming that commit; after resolve and commit the close passes: met. Go test.
- `--discard` drops the draft and records the sprint decision: met. Go tests, including a rerun after a failed drop.
- `pm where` lists a `drafts/` branch whose worktree is gone: met. Go test in `internal/store`.
- CI green, merged through `make merge-ready`: see the stamp below. Three fresh-context reviews; every correctness finding fixed with a test; cross-clone drafts left to step 5 and named in the design page's Failure modes.
