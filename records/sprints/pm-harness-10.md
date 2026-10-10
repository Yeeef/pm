---
type: sprint
title: One-command clone setup
bead: yeeef-agents-9va.10
---

## Goal

> What should be true when this sprint ends, and why now?

A fresh clone is ready for agents after one command: Beads connected, git hooks installed, and the records store checked out and linked. Today it takes `bd init` and then `bin/pm setup`, in that order, and a worktree made in between gets no records link. A background job verified that it writes records directly.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** fold `bd init` (or the matching Beads bootstrap for an existing remote) into the setup command, possibly renamed `pm init`; the docs that describe setup.

**Out:** changes to Beads itself; setup on other repos beyond what this repo needs.

## Done when

> What evidence will show the goal is met?

- On a fresh clone of main, one documented command leaves `bd ready`, `bin/pm show` and the `records` link working, and a new worktree gets its link from the hook; checked on a real clone.
- Re-running the command on a set-up clone changes nothing.
- A background session started after setup, in Claude Code and in Codex, edits a sprint Scope and a design page in `.records` and commits them with `pm commit`; checked for real.
- `CLAUDE.md`, RULES.md and the records-store design page describe the one command.

## Design pages

> Where is the detail?

- [Shared records store](../design/records-store.md): setup and sync
- [pm CLI](../design/pm-cli.md): `pm setup`

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-04}
Sprint 10 takes task 9va.12.2 from sprint 12 and adds a Done-when item: a background session, in Claude Code and in Codex, writes `.records` and commits with `pm commit`.
Letting sandboxed sessions write the store is part of what setup must leave working for each runtime, and it waits on PR #8.
:::

::: decision {source=agent date=2026-10-04}
Keep the setup command named `pm setup`; no `pm init`.
It attaches a clone to an existing project (as `bd bootstrap` does), while `init` names minting a new one (`bd init`, `git init`), which is the wrong command on a clone of this repo; keeping the name also leaves the post-checkout hook and every doc unchanged.
:::

::: decision {source=owner date=2026-10-04}
`pm setup` adds the clone's `.git`, `.records` and the uv cache to `sandbox_workspace_write.writable_roots` in the user's Codex config (`$CODEX_HOME/config.toml`, default `~/.codex`), so Codex can write and commit records.
The paths are absolute and machine-specific, so they cannot be committed to the repo, and the owner prefers no manual step over keeping the personal config untouched by repo tools.
:::

::: decision {source=agent date=2026-10-04}
`pm setup` adds five Codex writable roots, not three: the clone's `.git`, `.records`, the store's own git dir (`.git/worktrees/-records`), the main checkout's `.beads`, and the uv cache.
Measured with codex-cli 0.131.0: listing `.records` keeps its git dir read-only unless that dir is listed itself, and `bd` needs `.beads` writable from a worktree; this applies the owner's option B as built.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- A clone of this repo needs `bd bootstrap`, not `bd init`: bootstrap clones
  the database from `refs/dolt/data` on origin (3,318 chunks, about 4 s) and
  is a no-op once a database exists (`--dry-run --json` reports action none),
  while `bd init` mints a new project identity. Bootstrap installs no hooks
  and sets no `beads.role`, so `pm setup` adds `bd hooks install --beads` and
  the role; without the role and with `.beads` at 0755, every bd call printed
  two warnings.

- Real fresh clone of origin main plus 436e4aa: one `bin/pm setup` (6 s) left
  `bd ready`, `bin/pm show` and the `records` link working with a clean `git
  status`; `git worktree add` got its link from the post-checkout hook; a
  second run in the main checkout and the worktree printed "already set up"
  and changed no config, link, store, branch or Beads data.

- bd 1.3.1 reports no Dolt ahead/behind against the remote in embedded mode:
  `bd sql` is unsupported there, and `bd vc status`, `bd dolt status` and `bd
  context --json` show only the branch, data dir and remote URL, so `pm where`
  names the Beads remote and says bd reports no ahead/behind.

- Codex workspace-write sandbox (codex-cli 0.131.0, Seatbelt): run in the main
  checkout it can write .records files, run in a worktree it can only with
  sandbox_workspace_write.writable_roots naming .records, and in both it
  refuses writes under .git; bin/pm also needs ~/.cache/uv writable for uv.

- Codex can commit to the store only when the clone's .git is itself listed in
  sandbox_workspace_write.writable_roots; .git inside the workspace stays
  read-only otherwise.

- Codex (codex-cli 0.131.0) also needs the store's own git dir
  (.git/worktrees/-records) as a writable root: with .git, .records and the uv
  cache listed, a sandboxed pm finding add wrote the record but its commit
  failed on .git/worktrees/-records/index.lock (Operation not permitted),
  because a writable root's git dir stays read-only unless listed itself.
  codex sandbox reads these roots from $CODEX_HOME/config.toml: the same probe
  write under .git passed with the config and failed with a copy lacking the
  roots. With the four roots pm setup now adds, this finding was committed
  from a Codex sandbox in the main checkout with no -c override.

- From a git worktree, Codex (codex-cli 0.131.0) also needs the main
  checkout's .beads as a writable root: pm reads Beads through bd, which
  failed opening the embedded Dolt database (openat LOCK: operation not
  permitted) with only .git, .records, .git/worktrees/-records and the uv
  cache listed. With .beads added by pm setup, this finding was committed from
  a Codex workspace-write sandbox in a temporary worktree with no -c override;
  bd only warns that it cannot take .beads.gate.lock and continues ungated.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: one `bin/pm setup` makes a fresh clone ready (Beads, git hooks, the
records store and link, and Codex's sandbox roots), re-running it changes
nothing, and background jobs in Claude Code and sandboxed Codex sessions
write and commit records.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- One documented command on a fresh clone of main leaves `bd ready`,
  `bin/pm show` and the `records` link working, and a new worktree gets its
  link from the hook: met. Real clone of origin plus this branch: one
  `bin/pm setup` (6 s), then all four worked with `git status` clean
  (436e4aa, 02ae34c).
- Re-running the command changes nothing: met. Config, links, store HEAD,
  worktree list and a hash of the Beads export were identical before and
  after a second run; `test_setup_on_fresh_clone_checks_out_store_and_links_records`
  and `test_setup_adds_only_missing_codex_roots_and_reruns_change_nothing`.
- A background session in Claude Code and in Codex edits records in
  `.records` and commits with `pm`: met. Claude Code: a fresh `claude --bg`
  job ran in the main checkout and committed its edits to sprint 10's Goal
  and the pm-harness design page with `pm commit` (store `1242562`); it
  edited the Goal rather than the Scope, both hand-written sections. Codex:
  `pm` writes run under `codex sandbox macos` in workspace-write mode
  committed from the main checkout (`8aae959`) and a worktree (`29b68b7`),
  using the roots `pm setup` adds (3163fe9); not yet run from a Codex agent
  session.
- CLAUDE.md, RULES.md and the records-store design page describe the one
  command: met (436e4aa, 1d6439c, 3163fe9; design page b0b690c, 573bb7a).
