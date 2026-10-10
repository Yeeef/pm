---
type: doc
title: Readers of the records/ link and of main's records/ copy
date: 2026-10-07
bead: yeeef-agents-9va.65
---

Inventory for sprint 56. Code evidence is at `origin/main` 314d701; paths are repo-relative, lines are as of that commit.

## Summary

- **No pm code reads records through the `records/` link.** Every command finds the store from git's common dir: `pm/src/pm/store.py:29-51` (`store_path`, `find_store`, no fallback), called for every command at `pm/src/pm/cli.py:3673`, and by the stop hook at `pm/src/pm/hooks.py:237-240`.
- **The link exists for people and agents who edit `records/…` with ordinary file tools**, and for the wording that `prime.md` and the docs teach.
- **Main's copy has no machine reader.** The site, the service, `pm push` and the server clone all read the store or the `records` branch. The copy serves GitHub browsing on `main` and one relative link in `pm/AGENTS.md`.
- **The sparse checkout exists only because of the copy.** Without it, git would replace the ignored link with main's tracked `records/` (`cli.py:2304-2309`).
- **`additionalDirectories` stays a per-worktree step whatever happens to the link.** Claude Code writes the store only when the store is listed there. A `claude -w` session cannot write the store in any layout that keeps it as a git worktree (sprint 58 finding).

## How pm finds the store

| Step | Evidence |
|---|---|
| `git rev-parse --git-common-dir`, its parent, plus `.pm/store/records` | `store.py:29-34`, `config.py:19` |
| Checks that the store is a worktree on branch `records`, and refuses otherwise | `store.py:42-51` |
| Every command except init, doctor, push and where | `cli.py:3673` |
| Stop hook | `hooks.py:237-240` |

## A. Readers and writers of the per-worktree `records/` link

| Reader | Evidence | Without the link |
|---|---|---|
| Agents that hand-edit records | `pm/src/pm/prime.md:14,25-33,38,129` ("by hand in `records/`"); `CLAUDE.md:47`; `.pm/README.md:10` | Edit the store path (`pm where records`); prompts and docs change |
| `pm commit <path>` | `cli.py:3124-3128` maps a `records/…` prefix to the store itself ("its link may not exist yet") | Works unchanged |
| `pm record link <target>` | `cli.py:2206-2211` strips `records/` and resolves absolute paths against the store | Works unchanged |
| pm messages that name a record | `cli.py:185,274,3117`, `hooks.py:227,323` print `records/<rel>` built from the store path | Work; wording only |
| Stop hook | `hooks.py:276-292` matches store-relative paths as substrings of the transcript | Works with absolute store paths too |
| Site and service | `cli.py:1494` (`find_store`); service working dir is the main checkout (`service.py:94`) | Unaffected |
| `pm push`, day summary | `cli.py:2991-2994` (`find_store`) | Unaffected |
| `pm init` / `setup_clone` (writer) | `cli.py:2310-2317` makes the link and refuses anything else at that path | Step goes away |
| Ignore rules | `.gitignore:23` block from `install.py:323-329`; `.git/info/exclude` via `PM_EXCLUDE` (`cli.py:2813`) | Drop `/records` |
| Claude Code `additionalDirectories` | `cli.py:2850-2871` writes the store into each worktree's `.claude/settings.local.json` | Still needed, to write the store from a worktree |
| Codex `writable_roots` | `cli.py:2907-2912` lists the store and its gitdir directly | Independent of the link |
| `pm where`, `pm doctor` | `cli.py:3047-3058`, `cli.py:2559-2563` | Drop those checks |
| post-checkout hook | `.beads/hooks/post-checkout` → `cli.py:3621-3635` runs `setup_clone`; Claude Code worktrees skip it (pm-harness sprint 52), so session start runs `pm init` (`hooks.py:95-110`) | Less to do |
| Tests | `pm/tests/conftest.py:289,388`; `test_init.py:119,164,298`, `test_hooks.py:140-147`, `test_lifecycle.py:122,213,295`, `test_migrate.py:164,238`, `test_pm.py:322` | Update |
| `pm/AGENTS.md` | `:65,109,136` describe the link and the live check of writes through `records/` | Doc edits |

## B. Readers and writers of main's `records/` copy

| Reader | Evidence | Without the copy |
|---|---|---|
| Copy Action (the only writer) | `.github/workflows/pm-records-copy.yml`, from `install.py:104-138`: `git subtree merge --prefix=records` on each push to `main` | Delete the workflow |
| History | `git log origin/main -- records`: 39 commits, all by `github-actions[bot]` | — |
| PR guard (exists because of the copy) | `.github/workflows/pm-records-guard.yml`, from `install.py:78-101` | Delete |
| pre-commit guard | `hooks.py:338-349`; `.beads/hooks/pre-commit` | Delete |
| Sparse checkout in each code worktree | `cli.py:2304-2309` | Not needed |
| `pm where` detection of "main's tracked copy" | `cli.py:3054-3055` | Drop |
| GitHub browsing on `main` | `pm/AGENTS.md:26` relative link `../records/design/pm-harness.md`; records-store design: "`main` carries a copy" | Browse the `records` branch (`/tree/records/…`) or the site |
| Site | served from the store, links from `pm record link` | Unaffected |
| Linux server clone | `docs/2026-10-05-server-clone-plan.md:20`: records come from the `records` branch | Unaffected |
| CI tests (`pm-tests.yml`) | the checkout brings the copy in; `conftest.py:19` `REAL_RECORDS` is defined and not used | Unaffected |

The copy holds only records already pushed to `origin/records` (records-store design), so it lags the store by up to one push cycle (10 minutes).

## Per-worktree setup today

`setup_clone` (`cli.py:2275-2323`) runs at session start and from post-checkout. Its per-worktree steps:

| Step | Caused by |
|---|---|
| Sparse checkout that hides `records/` | main's copy |
| `records` symlink to the store | the link |
| Store in `.claude/settings.local.json` `additionalDirectories` | the store lying outside the worktree, which the link does not change |

Counted on this Mac on 2026-10-08:

- `git worktree list` shows 31 worktrees: 1 store, 25 with the link and 5 without it.
- The 5 without it are older `.claude/worktrees/` entries. They have sparse checkout on and track the copy.

## Open questions

- Removing the copy needs one commit on `main` that deletes `records/`. The pre-commit hook and the PR guard refuse that commit today, so they must go in the same PR. Branches cut before that commit still track `records/` until they merge `main`.
- `additionalDirectories` is per worktree because `settings.local.json` is per worktree. Whether a user-level or project-level setting can carry the store path was not checked.
- Readers of `main`'s `records/` outside this repo, such as bookmarks or other repos, are not recorded.
