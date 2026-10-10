---
type: design
title: pm as an installable product
project: pm-harness
---

## Problem

> What are we solving, and why now?

pm runs in one repo, this one, and nowhere else. Another repo cannot take it up: there is no installer, and pm's behaviour and context are spread over about 20 places owned by pm, Beads, Claude Code, Codex and git, at four scopes (machine, repo, clone, worktree). Every repo-level piece (hook entries, the `CLAUDE.md` import of the rules, pm's code in the git hooks, two GitHub workflows, Makefile targets, `.gitignore` lines) was copied by hand, and none of pm's pieces is marked as pm's, so nothing can upgrade or remove them.

pm is now moving to another repo, so it has to become a product: installed with one command, upgraded deliberately, and sitting on Beads in a defined way. Part of the [Project management harness](pm-harness.md) design.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- One line installs pm into a repo, including a brand-new repo with no records branch and no Beads data.
- Every piece pm puts in a repo is known to pm, so `pm upgrade`, `pm doctor` and `pm uninstall` act on exactly those pieces.
- Every session on a repo runs the same pm version.
- An agent gets pm's rules and the project's state without pm writing into the repo's instruction files.
- Nothing about this repo (its name, remote, ids, `make` targets, site URL) is baked into pm.

**Non-goals**

- Changing Beads or what Beads tells agents; whether pm forks or replaces Beads is [sprint 36](../sprints/pm-harness-36.md).
- Hosts other than GitHub, and agent runtimes without hooks (only Claude Code and Codex).
- Other git hook managers (husky, pre-commit) in the first version.
- Codex parity beyond carrying today's Codex pieces; that is [sprint 34](../sprints/pm-harness-34.md).

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- pm is one Go binary ([pm in Go](pm-go.md)). It needs git on the machine, plus `gh` and `claude`, and systemd or launchd for the pm service.
- Hardcoded today: the hook paths (`skills/project-management/harness/…` in `bin/pm`, `.claude/settings.json`, `.codex/hooks.json` and pm's own `wait_hint`), the remote `origin` and branch `main`, `make docs` and `make render` in agent-facing messages, `yeeef-agents-` ids in the Claude Code owner-request prompt hook, and this repo's remote in `.beads/config.yaml`.
- `pm setup` requires an existing `records` branch on the remote and existing `refs/dolt/data`; it refuses to create either.
- Beads keeps its repo footprint in `.beads/` (tracked `config.yaml`, `metadata.json`, `README.md`, `hooks/`, and a `.gitignore` for its database and runtime files). Its context reaches agents two ways: a short block in `AGENTS.md` between `BEGIN/END BEADS INTEGRATION` markers carrying a version and hash, and `bd prime` from a SessionStart hook. Beads moved away from full instructions in `AGENTS.md` because they cost tokens and went stale on upgrade.
- Beads' git hooks live in `.beads/hooks/` (`core.hooksPath`) and mark Beads' part with section markers; `bd hooks install` documents that any content outside the markers is preserved across installs and upgrades (`bd hooks install --help`, bd 1.3.1). pm's code already sits there, after Beads' end marker, but without markers of its own.
- Claude Code's SessionStart fires on startup, resume, clear and compact, and SubagentStart fires for each subagent, so a hook reaches every context an agent works in. Claude Code passes each hook's text inline only up to 10,000 characters (a longer one reaches the model as a 2 KB preview and a file path); the cap is per hook, not in total. The hooks of one entry run in parallel, so their texts arrive in any order (measured on Claude Code 2.1.292). The [hooks reference](https://code.claude.com/docs/en/hooks.md) documents the cap, gives no reason for it, has no setting to raise it, and says Claude Code does not ask the model to read the saved file.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Distribution

pm is a Go program in its own public repo, `Yeeef/pm`, released as one binary per platform from a version tag `pm-v<X>`. One line installs it on a machine; `pm init` then sets a repo up:

```
curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh | sh
pm init
```

`install.sh` checks the binary against the release's `SHA256SUMS` and puts it in `~/.local/bin`. `pm init` sets up whatever is missing in the repo, the clone and the worktree (below). The installed `pm` runs each repo's pinned version (Version pin, below). Target repos hold no copy of pm's code. The rules, the site's stylesheet and the model prompts are embedded in the binary. [pm in Go](pm-go.md), Distribution, has the detail.

### Where pm's pieces live

| Scope | Piece | Written by |
|---|---|---|
| Machine | the `pm` binary in `~/.local/bin` and its pins cache; the pm service per clone (systemd user service or launchd agent); Codex sandbox `writable_roots` per clone | `pm init` |
| Repo (tracked) | `.pm/` (below); pm's hook entries in `.claude/settings.json` and `.codex/hooks.json`; pm's lines in `.gitignore`, `/records` among them; pm's marked section in each `.beads/hooks/*` file it needs | `pm init`, `pm upgrade` |
| Clone | the store checkout at `<main>/.pm/store/records` (git-ignored); Beads database and config; `core.hooksPath` (Beads' `.beads/hooks`); the Beads agent profile | `pm init` |
| Worktree | the `records/` link to the store | `pm init`, run by the `post-checkout` hook |

pm writes nothing into `AGENTS.md` or `CLAUDE.md`, and inside Beads' git hook files only its own marked section.

### The `.pm/` directory

Content and machinery stay apart: records stay at `records/` (each worktree's link to the store; no branch tracks them); everything else pm owns lives in `.pm/`.

```
.pm/
  config.toml    tracked: the pinned pm version and the repo's settings
  README.md      tracked: what pm is, how to install it, where records live
  .gitignore     tracked: ignores store/ and run/
  store/         pm's stores, one directory each (clone, main checkout only)
    records/     the records branch checked out
  run/           runtime state: service log, push state, locks
```

`.pm/README.md` is the visible sign that a repo uses pm, for people and for agents in hosts without hooks. `config.toml`:

```toml
version = "0.3.0"        # the pm version every session must run
remote = "origin"
main_branch = "main"
port = 8000              # the service's site port; the PORT environment variable overrides it for one run
# site_url = "https://pm.example.com"   # optional, written by pm init --site-url URL; without it links use http://localhost:<port>
```

The public site URL is a repo setting: every clone prints the same links, and `pm init --site-url URL` writes it, never a hand edit.

Agent-facing messages name pm's own commands (`pm service`, `pm check`), never `make` targets, so a repo needs no Makefile.

### Context: hook-only

All of pm's agent context comes from hooks; the repo's instruction files carry none of it.

- `pm prime` prints pm's rules (embedded in the binary, so they match the pinned version) and `pm show`. The rules are about 25,000 characters, so they run as one hook per chunk (owner decision, 2026-10-07): `pm prime --rules N` prints chunk N of 4, cut at fixed headings, each under the cap and under a title naming its place and sections, since the chunks arrive in any order. The SessionStart entry runs the 4 chunks and `pm prime --state` for startup, resume, clear and compact, so all the rules come back after compaction. An agent can also run plain `pm prime` by hand: the rules whole, then the state.
- The rules `pm prime` prints include one merged list for anything addressed to the owner: put the conclusion first; use 4 bullets or fewer; no ids, hashes or file names unless the owner asks; give the page link, not a file path; tell what each number measures; use ASD-STE100 approved words, each with one meaning; at most 20 words per instruction sentence and 25 per description sentence; active voice and simple tenses; one instruction per sentence.
- Subagents get the same rules: the SubagentStart entry runs the same 4 chunk hooks (each envelope names the event that ran it), then `pm prime --subagent`, one line naming the Beads agent profile. A subagent gets no `pm show`.
- Codex's SessionStart has no compact event today (`.codex/hooks.json` matches startup, resume and clear); how Codex gets the rules back after compaction is sprint 34's.
- Every other hook is a `pm hook <name>` subcommand. `pm hook <name>` does not install anything: it is the command the runtime runs when the event fires, and it holds that hook's logic (installing the entries is `pm init`'s job): `pm hook stop` (uncommitted records), `pm hook reply-wait` (PostToolUse), and `pm hook owner-request` (the Stop check for requests left only in chat, in both Claude Code and Codex). No hook needs the repo's issue-id prefix.
- pm is self-contained: the `pm` CLI orchestrates the work and gives the context about itself, and it does the progressive disclosure that a skill would otherwise do. `pm prime` gives the model and the procedures at every session start; `pm show` gives project state level by level; each command's `--help` holds that command's and its subsystem's detail (the pm service's, for one). The context sits beside the code it describes, so the two change together and do not drift. There is no `pm guide` and no installed skill; a rule found missing later goes into code, into `pm prime` or into the `--help` of the command it concerns.
- `pm prime`'s text has a "what" part (the layers; the objects, how they relate and their invariants; the interfaces: the site for the owner, the `pm` CLI and the records for agents; how to work with them), then "how" parts as ordered procedures with a balanced amount of command reference (reading state; project, sprint and task operation; records operation; needs and actions), then the owner-writing list. It also states the key rules that code, hooks or CI enforce, at the step where they fire, so agents stay clear of the guards and the guards act as a safety net. A command list generated from the parser follows it.
- `pm prime` also reports the pm service's health (below).
- pm's context sits beside Beads' context, not over it: Beads keeps its `AGENTS.md` block and its `bd prime` hook unchanged.

Hook entries call `pm prime` or `pm hook <name>`, never a file path. An entry is pm's if its command starts with `pm prime` or `pm hook `; `pm upgrade` and `pm uninstall` touch only those entries and keep the rest of each settings file byte for byte. `pm` finds its own hooks the same way: `pm reply wait` advice depends on whether a `pm hook reply-wait` entry is present, and messages name `pm init`, not `bin/pm setup`. When `pm` is not installed, the hook fails with a non-zero exit and the host shows the error; there is no silent fallback.

### The pm service

One supervised background process per clone serves the site and pushes Beads data and the `records` branch every 10 minutes. It replaces both `pm serve` started by hand and the scheduled push job.

- `pm service install|status|restart|logs`. `pm init` installs it: a systemd user service on Linux, a launchd agent with `KeepAlive` on macOS. The supervisor restarts it after a crash.
- The site is served live from the store and Beads; nothing is built to disk. `pm check` validates the records (what `make render` did as a check), and `pm commit` runs the same check.
- `pm prime` reports the service's state in each session. When it is down, the agent runs `pm service restart`; if that fails, the agent raises an action to the owner and files a bug task.
- Two clones on one machine need different ports (`port` in `config.toml`, or `PORT`). Machines without systemd or launchd have no service; `pm init` refuses there.

### Git hooks

pm uses Beads' hook files rather than a hook directory of its own: `core.hooksPath` stays `.beads/hooks`, and pm adds a section between `# --- BEGIN PM v<X> ---` and `# --- END PM ---` markers to the hooks it needs, after Beads' section. The section is one line, `pm hook git-<name> "$@"`, so the logic ships in the binary. Beads preserves content outside its markers, and pm rewrites only inside its own:

- `post-checkout`: in a new worktree or clone, `pm init`.

`pm init` refuses a repo whose `core.hooksPath` points anywhere other than `.beads/hooks`.

### Version pin

`.pm/config.toml` pins the exact pm version every session in the repo runs; a worktree reads its own branch's file, so the pin moves with the code. The installed `pm` is a launcher, so repos on different pins share one machine (owner decision, 2026-10-07; it reverses the earlier choice of one installed version and a hard failure on any other pin).

| Case | What `pm` does |
|---|---|
| No readable pin (no repo, `pm init` in a fresh one), or the pin is the installed pm's own version | Runs in process. |
| Another pin, 0.2.0 or later | Replaces itself (`exec`) with that release's binary, `$XDG_DATA_HOME/pm/pins/<pin>/pm` (default `~/.local/share`): stdin, stdout, stderr, the pid and the exit code are the pinned pm's, so hooks, git hooks and the service's unit go through it unchanged. |
| A pin below 0.2.0 (a retired Python release) | Fails hard, naming the fix: `pm upgrade --to <X>` with a release from 0.2.0 on. `pm upgrade --to` a version below 0.2.0 is refused. |
| `pm upgrade` | Runs in process and moves the pin to the running version, never down: a pin newer than the running pm is refused, naming `pm upgrade --to <pin>`; `--to X` launches pm X to make the move. |
| Launched for this pin already (`PM_LAUNCHED=<pin>`) | Runs in process; if that build is not the pinned version, the config check fails hard naming the tag. No second launch, so no loop. The launched pm takes `PM_LAUNCHED` and `PM_LAUNCHER` out of its environment first, so its children (git hooks, `claude -p`, any `pm`) reach the launcher afresh. |

- **Download once.** The first launch of a pin on a machine downloads release `pm-v<pin>`'s asset for the platform (10 s connect timeout, 300 s in all), checks it against `SHA256SUMS`, writes it atomically and keeps its sha256; a later download that differs fails hard. A missing release or asset, a checksum mismatch and a timeout are hard errors naming `pm-v<pin>`; nothing falls back to the installed pm's own version. After that a launch needs no network.
- **A launched pm leaves the bin dir alone.** `pm init` run by a launched pm does not copy itself into the bin dir, since that would replace the launcher. The service's unit runs the bin-dir pm (`<path> service run`), which launches the main checkout's pin, so the service runs the repo's pin.
- **Moving the pin.** `pm upgrade [--to X]` moves the pin and rewrites every managed piece for the new version in one change for the owner to commit. Once the main checkout's pin moves, the service exits; the supervisor starts the bin-dir pm again, which launches the new pin.
- `pm where` and `pm doctor` name the version running and why (in process, or launched by the installed pm at version Y).

### Commands

| Command | Does |
|---|---|
| `pm init` | One idempotent command for every scope, doing only what is missing. Repo, when `.pm/config.toml` is absent: write `.pm/`, the hook entries and the `.gitignore` lines; run `bd init` if the repo has no `.beads/`; create the `records` branch with an empty store if the remote has none; then the clone and worktree steps. Writes files and prints the commit to make; never commits on the code branch. Clone and worktree, always: what `pm setup` does today (Beads bootstrap, store checkout, `records/` link, hooks path, the pm service, Codex roots), reading names from `config.toml`; this half is what the `post-checkout` hook runs. |
| `pm upgrade` | Move the pin; rewrite the managed pieces. |
| `pm doctor` | Compare every managed piece with what the pinned version writes, and the clone and worktree setup with what `pm init` makes; report each difference and exit non-zero on any. |
| `pm uninstall` | Remove the managed pieces (hook entries, `.gitignore` lines, pm's sections in `.beads/hooks`, `.pm/`), and the clone and machine setup (the store checkout, the `records/` link in every worktree `git worktree list` shows, the pm service, the Codex roots); keep the `records` branch and Beads. |

### Where today's pieces end up

| Today | In the product |
|---|---|
| `skills/project-management/harness/pm.py`, `harness/harness/*.py`, `render.py`, `style.css` | the `pm` binary; `render.py`'s static build goes away, and its check becomes `pm check` |
| `harness/*_hook.py` | `pm prime` and `pm hook <name>` |
| `harness/RULES.md` | `prime.md`, embedded in the binary and printed by `pm prime` |
| `SKILL.md`, `references/*.md`, `status-site/*.md` | deleted; their essential judgment rules move into the rules `pm prime` prints; no skill is installed |
| `agents/openai.yaml` | dropped with the skill; Codex's interface is sprint 34's |
| `harness/tests/` | pm's black-box harness, `tests/` in `Yeeef/pm`, which drives the built binary |
| Makefile `render`, `docs`, `test`, `test-live` | `pm check`, `pm service`, pm's own test commands in `Yeeef/pm`; the Makefile keeps only this repo's own targets |
| push state `pm-push.{json,log,lock}` in the git dir; the scheduled push job | `.pm/run/` in the main checkout; the pm service |
| `git config beads.role maintainer`, the Beads agent profile | unchanged, set by `pm init` |
| `bin/pm` | removed; `pm` is the installed binary |
| `.github/workflows/records-{guard,copy}.yml`, the pre-commit records guard, the per-worktree sparse checkout, `records/` on main | removed: records live only on the `records` branch ([Shared records store](records-store.md)) |

### Migrating yeeef-agents

This repo is the first install and the one with legacy pieces. `pm init` here also:

- removes the `@skills/project-management/harness/RULES.md` import and the Codex "read RULES.md" line from `CLAUDE.md`;
- replaces the hook entries that call `harness/*_hook.py` by path in `.claude/settings.json` and `.codex/hooks.json` (the ownership test cannot see them, so `pm init` lists them explicitly as legacy);
- wraps the unmarked pm code after Beads' section in `.beads/hooks/post-checkout` into pm's marked section, and removes it from `pre-commit`;
- removes the records copy and guard workflows, under either name;
- replaces `/.records/` and `site/` in `.gitignore` with pm's `.pm/.gitignore`;
- in each clone, `pm init` moves the store from `<main>/.records` to `<main>/.pm/store/records` (`git worktree move`), relinks `records/` in every worktree, and replaces the push job with the pm service.

### Bootstrapping a brand-new repo

With no `records` branch on the remote, `pm init` creates it as an orphan branch holding the store's empty layout (`projects/`, `sprints/`, `days/`, `design/`, `docs/`, `postmortems/`); with no `.beads/`, it runs `bd init` (Beads then writes its own `AGENTS.md` block and hooks, which pm leaves alone, and commits them on the code branch itself). `pm init` pushes the new `records` branch at once, before it creates the local branch, because the push starts by fetching `records` from the remote and fails while the remote has none; a rejected push leaves nothing behind locally. The pm service publishes Beads data on its next run.

## Alternatives considered

> What else was considered and not adopted, and why not?

- **A Claude Code plugin.** Plugins carry hooks and skills natively, but only for Claude Code; Codex would need a second channel, and the CLI still needs installing.
- **A copy of pm in each repo, as today.** Upgrades mean copying files, and copies drift.
- **pm's rules in `AGENTS.md`**, in full or as a pointer section beside the hook (Beads' approach). Beads needs the file for hosts without hooks; pm targets only hosts with hooks, the file text goes stale on upgrade and costs tokens on every request, and `.pm/README.md` already signals that pm is in use.
- **pm as the only entry point over Beads**, wrapping `bd prime` and pinning Beads' block to its minimal profile. pm's context does not conflict with Beads', and changing what Beads says belongs to sprint 36.
- **pm's own hook directory (`.pm/hooks`) chaining to Beads' hooks.** It would change `core.hooksPath` away from what Beads installs and checks; Beads already preserves other tools' marked sections, so a section in its files is simpler.
- **One installed version and a hard failure on any other pin.** It was the first design. It made the owner reinstall pm on every switch between repos on different pins, and repos cannot all move pins at once.
- **`uvx` with the release tag.** uv fetches the remote to resolve a tag on every run (6 s), too slow for hooks; the launcher resolves the tag once and runs the commit.
- **A persistent environment per pin, managed by pm.** It starts faster than `uv tool run` (no resolution), but it is a second install mechanism beside uv's cache; `uv tool run` with a commit costs 0.2 s.
- **Floating to the latest installed pm.** Sessions on one repo could follow different rules, and one upgrade would change every repo at once.
- **Everything, records included, in `.pm/`.** Records are content for the owner; hiding them under a dot-directory on main and in every worktree costs visibility for one fewer top-level path.
- **A first version for existing repos only.** A new repo would still need setup by hand, so the one-line install would not hold.
- **`pm guide` topics on demand, or an installed skill.** Text that most sessions never load is not read; the essentials go in every session through `pm prime`, and the rest is dropped.
- **The site URL per clone.** The owner reads one site, so every clone links to it.
- **The site and the push as separate mechanisms.** The site was down whenever nobody started it, and two mechanisms had to be installed and checked.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- **Beads** (`bd init`, `bd setup`, `bd prime`): a tracked tool directory with its own README and `.gitignore`; marked, versioned, hashed blocks in shared files so setup can check, update and remove only its own part; context delivered by a SessionStart hook that re-fires after compaction. pm takes the directory and the managed-piece discipline, and goes further on hooks: no instruction-file block at all.

## Open questions

> What is still unresolved?

- Whether Codex passes each hook's text whole, as Claude Code does, to sessions and subagents; settled by sprint 34.
