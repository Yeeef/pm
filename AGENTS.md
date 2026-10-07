# Developing pm

Guidance for an agent changing pm itself: its code, tests, hooks and site. `CLAUDE.md` is a symlink to this file.
An agent that only uses pm in a repo gets its context from `pm prime`, `pm show` and `pm <noun> --help`; nothing here
is for them, and nothing here ships: the wheel holds only `src/pm/` (`[tool.hatch.build.targets.wheel]` in
`pyproject.toml`), so `prime.md` and `style.css` ship and this file does not.

## Layout

| Path | Holds |
|---|---|
| `src/pm/cli.py` | The commands and their `--help` texts; `parser()` builds the argparse tree |
| `src/pm/hooks.py` | `pm prime` (SessionStart and SubagentStart context) and `pm hook stop` (uncommitted records) |
| `src/pm/owner_request.py` | `pm hook owner-request`: the Haiku judge, its prompt and `claude -p` arguments |
| `src/pm/prime.md` | The rules `pm prime` prints; the only prose the package ships to agents |
| `src/pm/records.py`, `store.py`, `beads.py` | Record parsing and checks, the store (the `records` worktree and its lock), `bd` calls |
| `src/pm/site.py`, `style.css` | The site the pm service serves and `pm check` renders; the one stylesheet every page gets |
| `src/pm/service.py`, `push.py` | `pm service`: one supervised process per clone serves the site and runs `pm push` |
| `src/pm/install.py`, `tool.py`, `legacy.py` | `pm init`, `doctor`, `upgrade`, `uninstall`: the repo's pieces, the pm uv tool, the pre-package harness's pieces |
| `src/pm/config.py` | `.pm/config.toml`: every command fails hard without it or on another pinned version |
| `tests/` | pytest suite, fakes and the live eval (below) |
| The pm uv tool | The `pm` on PATH that hooks, agents and the service run; `pm init` installs it from git (`tool.py`). Run this checkout's code with `uv run --project pm pm …` |
| `../.claude/settings.json`, `../.codex/hooks.json` | Where the runtimes wire the hooks (below) |
| `../.pm/config.toml` | This repo's pm config; its `version` must equal `version` in `pyproject.toml` |
| `../records/design/pm-harness.md` | The harness design; one sub page per area (`pm-cli.md`, `owner-request-hook.md`, `records-store.md`, `site-replies.md`, …) |

A design change edits the sub page it touches to the new state; the trail of findings stays in the sprint record.

## Tests

| Command | Runs |
|---|---|
| `make test` (repo root) | The fast set: `pm/tests/run.py -n auto -m "not slow"` in the package environment |
| `make test-full` (repo root) | Every test but the live eval, the slow ones too (`-n auto`) |
| `uv run pytest -q -n auto tests/test_hooks.py` (in `pm/`) | One file, or `-k name` for one test |
| `make test-live` | The live eval: `PM_LIVE_TESTS=1`, `-k owner_request_prompt_live`; needs `claude` on PATH |

Run the fast set during development and the full set before raising a PR review (owner decision, 2026-10-06). A
test that starts the service, makes a clone with a remote or runs the session-start hook is marked `slow`.

What the tests are:

- `test_pm.py`: each command against a temp repo and a fake `bd`; every refusal changes nothing, every happy path
  writes what it says. `test_config.py`: the config check. `test_service.py`: the service's units in process and
  `pm service` end to end. `test_tool.py`: the pm uv tool. `test_init.py`, `test_lifecycle.py`, `test_migrate.py`:
  `pm init`, `doctor`, `upgrade` and `uninstall` on temp clones, and the move off the pre-package harness.
  `test_hooks.py`: `pm prime` and `pm hook stop`, run as the runtimes run them (JSON on stdin). `test_owner_request_hook.py`: the owner-request hook against a
  fake judge. `test_owner_request_prompt_live.py`: the judge's accuracy, with the real model.
- Fixtures (`conftest.py`): `repo` is a temp main checkout with its store at `.pm/store/records` on branch
  `records` and the `records/` link, as `pm setup` leaves a clone. Its env puts the fakes first on PATH and points
  `HOME`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME` at temp dirs. `pytest_configure` points the test process's own
  `HOME`, `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and `XDG_CONFIG_HOME` at a temp dir too, so git and the pm hooks it
  runs never touch the user's files, and an autouse check fails a test that changes the user's Codex config or
  pm service units. `test_pm.py` adds `served` (a `pm service run` on a free port), `origin` (a cut-over origin,
  for `pm setup`) and `pushed` (a bare origin plus a second clone, for `pm push`).
- Fakes: `fake_bd.py` serves issues from `$FAKE_BD_STATE` and logs calls to `$FAKE_BD_LOG`; `$FAKE_BD_FAIL`
  makes one call fail until `$FAKE_BD_HEAL` exists, `$FAKE_BD_HOLD` makes one wait. `fake_gh.py` answers
  `gh pr view` from `$FAKE_GH_STATE`. `fake_claude.py` stands in for `claude -p` and logs each call. `fake_sched.py`
  stands in for `launchctl`, `systemctl` and `crontab`.

## The judge and its live eval

The owner-request Stop hook calls Haiku through `claude -p` (`JUDGE_ARGS` in `owner_request.py`, thinking off).
Its labelled cases are `tests/owner_request_cases.json`: a final reply, the open needs Beads holds and the
verdict the rule gives. After editing the prompt, run `make test-live`: each case runs `PM_LIVE_RUNS` times
(default 3), 8 calls at once, and every run must give the case's verdict; it prints pass counts and latency.
A case's label comes from the rule, never from what the judge answers: a miss is a prompt change, not a relabel.

## Changing prime.md and the hooks

- `pm prime` prints `prime.md` then the noun list, which `hooks.commands()` reads from the parser
  (`MACHINERY` = `prime`, `hook`, `push` are left out). A new noun needs no edit to `prime.md`;
  `test_prime_lists_every_agent_command_from_the_parser` checks the list. A new command's rules go in its
  `--help`, and `prime.md` names it only where a procedure runs it.
- Claude Code passes each hook's `additionalContext` inline only up to `hooks.CAP` (10,000 characters; a longer
  one arrives as a 2 KB preview and a file path), per hook, and the hooks of one entry arrive in any order. So the
  rules run as one hook per chunk: `hooks.chunks()` cuts them at the headings in `hooks.STARTS` and puts a title
  line naming each chunk's place and sections on top (`pm prime --rules N`), and the state runs as its own hook,
  cut at a line. 2026-10-07: 4 chunks of 8,447, 5,749, 6,317 and 4,708 characters, 24,872 without titles.
  `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` fails when a chunk outgrows the cap: move a heading in
  `STARTS`, or add one plus its hook entries; `test_hook_entries_run_every_rules_chunk` checks the entries.
- `prime.md` carries only what a user's agents need (owner decision, 2026-10-07). Guidance for developing pm,
  `[TEST]` needs and this repo's checks go here, never in `prime.md` or a `--help` text.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `cli.py` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts; grep it before rewording one.
- `pm init` writes the hook entries (`claude_hooks()` and `codex_hooks()` in `install.py`) into
  `.claude/settings.json` (SessionStart: `pm prime --rules N --hook-json` for each chunk, then `--state
  --hook-json`; SubagentStart: the same chunks, then `--subagent --hook-json`; Stop: `pm hook owner-request` then
  `pm hook stop`, each `|| exit 1`) and `.codex/hooks.json`, with `hooks = true` in `.codex/config.toml`. A hook
  change edits both functions; this repo's two files are what `pm init` writes. Hooks fail open on their own
  errors (one line on stderr); a `pm` missing from PATH fails each hook with the shell's error.

## The site

- `pm check` renders every record with Beads and writes nothing (the check before a records commit); the pm
  service serves the site on the port in `.pm/config.toml` (`PORT=` overrides it). This repo's site is at the
  `site_url` in `.pm/config.toml`, a tunnel to the pm service on the owner's machine.
- One stylesheet, `src/pm/style.css`, for every page; a look it cannot express is added there, never to a record.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer. A
  generated section (Progress, Decisions await you, Actions await you, a day's Sprints) is rendered from Beads.
  `test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does` holds the render check.
- Day summaries live beside the day record as `records/days/<date>.summary.json`, written by `pm day summarize`.

## Live checks on this repo

A change to setup, the hooks, the site or replies gets a live check besides its tests, in the form the sprint's
"Done when" names.

- A need raised during a test or a live check starts its title with `[TEST]`; once the check is done, close it
  with a bare `bd human dismiss <id>`. `pm sprint close` skips a dismissed review, so a `[TEST]` review never
  blocks a close.
- A scratch worktree checks the session-start path: `git worktree add --no-checkout` then `git reset --hard`
  (Claude Code's own sequence), or a session started with `claude -w <name>`; a headless `claude -p` session
  checks a write through `records/`. Remove the worktree and its branch afterwards (`git worktree remove`,
  `git branch -D`).
- A check of `pm init`, `pm setup`, `pm push` or the service runs in a scratch clone of a scratch origin under a
  temp directory with `HOME` and `CODEX_HOME` there, as the fixtures do, never on this clone's Beads, store or service;
  never run `bd init`.
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.

## Elsewhere

- `main` still holds the pre-package harness (`bin/pm`, `skills/project-management/harness/*.py`, `SKILL.md`,
  `RULES.md`); this branch replaces it with the package, and `pm init` moves a clone off it (`legacy.py`).
