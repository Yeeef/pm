---
type: sprint
title: pm init, upgrade, doctor and uninstall
bead: yeeef-agents-9va.51
---

## Goal

> What should be true when this sprint ends, and why now?

One line installs pm into any GitHub repo, including a brand-new one, and `pm upgrade`, `pm doctor` and `pm uninstall` act on exactly the pieces pm manages. Design: [pm as an installable product](../design/pm-product.md).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm init` writing `.pm/`, hook entries in `.claude/settings.json` and `.codex/hooks.json`, pm's marked sections in `.beads/hooks`, the two workflows and `.gitignore` lines; bootstrap of a new repo (`bd init`, orphan `records` branch); `pm setup` reading `.pm/config.toml` and using `.pm/store` and `.pm/run`; `pm upgrade`, `pm doctor`, `pm uninstall`.
**Out:** migrating this repo's legacy pieces (next sprint); non-GitHub remotes; other hook managers.

## Done when

> What evidence will show the goal is met?

- In a fresh scratch GitHub repo, the one-line install leaves a working setup: a session gets `pm prime` context, `pm show` works, a record write commits on `records`, and the scheduled push publishes it.
- `pm doctor` reports clean after init and reports each hand-made change to a managed piece; `pm upgrade` and `pm uninstall` touch only managed pieces, byte for byte elsewhere, shown by tests.

## Design pages

> Where is the detail?

- [pm as an installable product](../design/pm-product.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
pm's clone setup lists `/.pm/store/` and `/.pm/run/` in the clone's `.git/info/exclude`, and `pm uninstall` removes those lines; doctor checks them like the other clone pieces.
A branch made before pm has no `.gitignore` entry for the store, so `git add -A` there would stage it as an embedded repo; only a clone-level exclude covers every branch.
:::

::: decision {source=agent date=2026-10-07}
The clone's `.git/info/exclude` also lists `/records`, beside `/.pm/store/` and `/.pm/run/`, with the same doctor and uninstall handling.
The second live check showed the untracked `records` link is not ignored on a branch made before pm either, so `git add -A` there would stage it.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm service install refuses a pm that runs from a non-main worktree's venv
  (bin/pm in a worktree): the unit would run that worktree's code and
  crash-loop once it is removed; in the product pm runs from the uv tool, so
  this only bites this repo before its migration.

- test_a_push_that_failed_is_retried_by_the_sweep is flaky on the pm-service
  base too: 3 of 6 runs failed at the base commit, unrelated to the service
  change.

- bd 1.3.1 `bd init` commits on the code branch itself ("bd init: initialize
  beads issue tracking": .beads/, AGENTS.md, CLAUDE.md, .claude/settings.json,
  .codex/hooks.json, .cursor/). Only pm's own files stay uncommitted after `pm
  init` in a brand-new repo.

- `pm push` cannot publish a records branch the remote lacks: it starts with
  `git fetch <remote> records`, which fails. So `pm init` pushes the new
  orphan branch itself, before creating the local branch; the design's line
  that the scheduled push publishes it is wrong for records (Beads data still
  goes by `bd dolt push`).

- The design still describes the Claude Code owner-request check as a prompt
  hook with the Beads id prefix, but pm-package already made it a command hook
  (`pm hook owner-request`) in both runtimes and no id prefix is used, so `pm
  init` writes the command hook.

- Once pm-init merges, this repo's pm looks for its store at .pm/store/records
  while the store is still at .records, so pm-init must land together with the
  sprint-46 move (a worktree of pm-init already gets 'no records store' from
  its own bin/pm).

- Merged pm-service into pm-init (eba7997): pm init, not pm setup, installs
  the pm service, so a session start never installs one; the service's main
  checkout now comes from the git common dir, since the store moved into
  .pm/store/records.

- pm doctor compares only pm's part of each managed piece with what the
  running version writes, so another tool reordering its own hooks does not
  trip it; pm upgrade at the same version rewrites the pieces and so repairs
  whatever doctor reports (14 new tests, full suite 488 passed, 35 skipped).

- pm uninstall keeps every byte not pm's except one: when .gitignore or a
  Beads hook file lacked a final newline, the newline pm added before its
  block stays after the block goes; pm cannot tell it from the owner's.

- Live check (yeeef-agents-9va.51.5): the one-line `uvx ... pm init` does not
  install pm as a uv tool, so `pm` is not on PATH afterwards and every hook pm
  init writes (`pm prime`, `pm hook ...`) would fail; the design says init
  installs it. Worked around with `uv tool install
  git+...@pm-init#subdirectory=pm`.

- Live check: run through uvx, pm init installs the launchd service with
  ProgramArguments and PATH pointing at uvx's ephemeral env
  (~/.cache/uv/archive-v0/<hash>/bin/python), which `uv cache clean/prune`
  deletes; afterwards the installed `pm doctor` exits 1 ('plist does not run
  this pm; run pm service install'). `uvx ... pm doctor` is clean; after `pm
  service install` with the uv tool pm, `pm doctor` exits 0.

- Live check: smaller gaps: pm init's 'commit pm's files' command omits
  .beads/config.yaml, which the same run changed (agent profile
  team-maintainer); `pm prime --state` still says 'bin/pm where' and 'bin/pm
  show' in a repo with no bin/pm; the site answers HEAD with 501 (curl -sI
  fails, GET is 200); pm init edits the user's global ~/.codex/config.toml
  writable_roots.

- Live check 2 (pm-init 5e8bd2b, fresh repo Yeeef/pm-install-scratch-2): the
  one-line install fails. pm init exits 1 with 'installed
  local.pm.pm-install-scratch-2.22eab31f, but the site does not answer for
  this store on :8017 (... stale: it runs pm older than 0.1.0, not the pinned
  0.1.0)'. Cause: tool.ensure() reinstalls the pm uv tool only when its
  version string differs; the tool already installed here is 0.1.0 built from
  a2ae084, which sends no version header, so init keeps it and its own service
  probe then rejects it. The installed pm doctor (a2ae084 code) still exits 0,
  while pm doctor via uvx at 5e8bd2b exits 1 on the stale service. ensure()
  must compare the tool's source commit (direct_url.json commit_id) or a build
  id, not only __version__.

- Live check 2, after the failed init (committed and pushed by hand to see the
  rest): the rest of the flow works on the stale tool. claude -p replied '# pm
  rules'; pm show exit 0; pm project open committed 52cf60b on records; the
  service pushed at 06:19:47 ('records ok: pushed 2 commit(s)'), 387a8ab on
  GitHub. Every gap traces to the stale a2ae084 tool: pm prime --state still
  says 'bin/pm' (2 lines), HEAD / answers 501, the installed pm service status
  says 'running' while pm 5e8bd2b says stale, and the installed pm uninstall
  left /.pm/store/ and /.pm/run/ in .git/info/exclude. launchd agent,
  LaunchAgents listing and ~/.codex/config.toml were restored.

- Live check 3 (pm-init c33fd0a, Yeeef/pm-install-scratch-3): init replaced
  the stale a2ae084 tool by itself (tool direct_url commit c33fd0a), doctor
  exit 0, claude -p got '# pm rules', project open committed 7b8373e on
  records and the service pushed it at 14:04:28 (632 s after start). Bug: pm
  uninstall removed uv's cache dir (~/.cache/uv) from
  ~/.codex/config.toml writable_roots although init did not add it (another
  clone had); the root is shared across clones, so uninstall must keep it, or
  only remove roots this init added.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: one line installs pm into any GitHub repo, including a brand-new one, and `pm upgrade`, `pm doctor` and `pm uninstall` act only on the pieces pm manages ([PR #62](https://github.com/Yeeef/yeeef-agents/pull/62), stacked on PR #52).

Merged as 3b8c583 (PR #62). Reached main as d4e64a6 ([PR #65](https://github.com/Yeeef/yeeef-agents/pull/65)), tagged pm-v0.1.0.

- `pm init`: `.pm/`, pm's hook entries, marked git-hook sections, the `pm-records-{guard,copy}.yml` workflows, a marked `.gitignore` block, clone excludes; a brand-new repo gets `bd init` and an orphan `records` branch pushed at once; installs the pm uv tool from the exact commit it runs from, then the pm service; never commits on the code branch.
- `pm service install|status|restart|logs|run` replaces `pm serve` and the scheduled push; `pm check` replaces `pm render`; the full context is in `pm service --help`.
- `pm doctor`, `pm upgrade`, `pm uninstall` compare, rewrite or remove only pm's parts.
- Two fresh-context reviews: 8 and 4 findings, all fixed or rejected with a reason; four live installs found 3 more bugs, all fixed.
- Must reach main together with sprint 46: after it, this repo's pm looks for its store at `.pm/store/records`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **One-line install into a fresh scratch GitHub repo leaves a working setup: met.** Live check on 0de0e61 (task 9va.51.16), private repo `Yeeef/pm-install-scratch-4`: `uvx --from git+…@0de0e61 pm init` exited 0 in 15 s with no manual step and replaced an older pm tool itself; a headless `claude -p` session answered with the rules' first line `# pm rules`; `pm show` exited 0; `pm project open` committed a9b1ae6 on `records`; the pm service pushed it, and GitHub's `records` head 1747ec9 contains it, about 604 s after the commit (push interval 600 s). `pm uninstall` then left `~/Library/LaunchAgents` and `~/.codex/config.toml` byte for byte as before.
- **`pm doctor` clean after init and reports each hand-made change; `pm upgrade` and `pm uninstall` touch only managed pieces, byte for byte elsewhere, shown by tests: met.** `pm doctor` as the installed tool exited 0 right after init in the live check. `pm/tests/test_lifecycle.py` (slow set): doctor reports a change to each repo piece kind and upgrade restores it; doctor reports each changed clone-setup item; init and upgrade keep every non-pm byte; uninstall removes only pm's parts (including a prunable worktree) and keeps the shared uv cache and other clones' Codex roots. Sprint 45's 4 test files hold 18 cases after sprint 59's pruning rule; `make test` 42 passed in 10.0 s; `test_doctor_reports_each_changed_clone_setup` passes on 0de0e61.
