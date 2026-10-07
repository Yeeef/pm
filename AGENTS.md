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
| `src/pm/site.py`, `render.py`, `style.css` | `pm serve` and `pm render`; the one stylesheet every page gets |
| `src/pm/push.py` | `pm push`, the scheduled job `pm setup` installs |
| `src/pm/config.py` | `.pm/config.toml`: every command fails hard without it or on another pinned version |
| `tests/` | pytest suite, fakes and the live eval (below) |
| `../bin/pm` | Resolves the repo root and runs `uv run --quiet --project pm pm`; hooks and agents call pm through it |
| `../.claude/settings.json`, `../.codex/hooks.json` | Where the runtimes wire the hooks (below) |
| `../.pm/config.toml` | This repo's pm config; its `version` must equal `version` in `pyproject.toml` |
| `../records/design/pm-harness.md` | The harness design; one sub page per area (`pm-cli.md`, `owner-request-hook.md`, `records-store.md`, `site-replies.md`, …) |

A design change edits the sub page it touches to the new state; the trail of findings stays in the sprint record.

## Tests

| Command | Runs |
|---|---|
| `make test` (repo root) | `pm/tests/run.py` in the package environment: every test except the live eval |
| `uv run pytest -q tests/test_hooks.py` (in `pm/`) | One file, or `-k name` for one test |
| `make test-live` | The live eval: `PM_LIVE_TESTS=1`, `-k owner_request_prompt_live`; needs `claude` on PATH |

`main` splits the suite into a fast set (`make test`, `-n auto -m "not slow"`) and the full set (`make test-full`),
by the owner's decision of 2026-10-06: run the fast set during development and the full set before raising a PR
review. On this branch (`prime-rewrite-b`) the Makefile has only `make test`, which runs everything serially; the
`slow` marker and `pytest-xdist` arrive when `main` is merged.

What the tests are:

- `test_pm.py`: each command against a temp repo and a fake `bd`; every refusal changes nothing, every happy path
  writes what it says. `test_config.py`: the config check. `test_push.py`: the scheduled push in process, with
  git and the schedulers replaced. `test_hooks.py`: `pm prime`, `pm hook stop` and the generated-section check,
  run as the runtimes run them (JSON on stdin). `test_owner_request_hook.py`: the owner-request hook against a
  fake judge. `test_owner_request_prompt_live.py`: the judge's accuracy, with the real model.
- Fixtures (`conftest.py`): `repo` is a temp main checkout with its store at `.records` on branch `records` and
  the `records/` link, as `pm setup` leaves a clone. Its env puts the fakes first on PATH and points `HOME`,
  `CLAUDE_CONFIG_DIR` and `CODEX_HOME` at temp dirs, so no test touches the user's schedule, transcripts or
  Codex config. `test_pm.py` adds `served` (a `pm serve` on a free port), `origin` and `clone` (a cut-over
  origin and a fresh clone, for `pm setup`), `pushed` (a bare origin plus a second clone, for `pm push`) and
  `public` (a site URL in the config).
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

- `pm prime --rules` prints `prime.md` then the noun list, which `hooks.commands()` reads from the parser
  (`MACHINERY` = `prime`, `hook`, `push` are left out). A new noun needs no edit to `prime.md`;
  `test_prime_lists_every_agent_command_from_the_parser` checks the list. A new command's rules go in its
  `--help`, and `prime.md` names it only where a procedure runs it.
- Claude Code caps each hook's `additionalContext` at `hooks.CAP` (10,000 characters). The rules and the state run
  as two SessionStart hooks, so `pm show` keeps its own cap; the rules hook is never cut, the state cuts `pm show`
  last, at a line. `test_session_start_keeps_a_busy_days_pm_show_whole` and `test_subagent_start_envelope` hold
  the rules under the cap; both fail while `prime.md` is over it (24,873 characters with the noun list, 2026-10-07).
- `test_prime_md_sentences_are_at_most_20_words` splits every bullet, paragraph and table cell into sentences; a
  code span counts as one word and an arrow as none. Headings and table rules are skipped.
- `prime.md` carries only what a user's agents need (owner decision, 2026-10-07). Guidance for developing pm,
  `[TEST]` needs and this repo's checks go here, never in `prime.md` or a `--help` text.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `cli.py` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts; grep it before rewording one.
- The hooks are wired in `.claude/settings.json` (SessionStart: `bin/pm prime --rules --hook-json` and
  `--state --hook-json`; SubagentStart: `--subagent --hook-json`; Stop: `pm hook owner-request` then
  `pm hook stop`, each `|| exit 1`) and in `.codex/hooks.json` with `hooks = true` in `.codex/config.toml`. A hook
  change edits both. Hooks fail open on their own errors (one line on stderr) and fail loudly when `bin/pm` is
  missing.

## The site

- `make render` runs `pm render` (writes `site/`, git-ignored; the check before a records commit); `make docs`
  runs `pm serve` on the port in `.pm/config.toml` (`PORT=` overrides it). This repo's site is at the `site_url`
  in `.pm/config.toml`, a tunnel to `pm serve` on the owner's machine.
- One stylesheet, `src/pm/style.css`, for every page; a look it cannot express is added there, never to a record.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer. A
  generated section (Progress, Decisions await you, Actions await you, a day's Sprints) is rendered from Beads;
  `test_render_refuses_text_in_progress` and its neighbours hold the check.
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
- A check of `pm setup`, `pm push` or the scheduler runs in a scratch clone of a scratch origin under a temp
  directory, as the `origin` and `pushed` fixtures do, never on this clone's Beads, store or schedule; never run
  `bd init`.
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.

## Elsewhere

- The `pm-init` branch (not merged) adds `pm init`, `doctor`, `upgrade` and `uninstall`, and has `pm service` (one
  background process per clone that serves the site and pushes replies) and `pm check` in place of `pm serve` and
  `pm render`; this branch has none of them.
- `main` still holds the pre-package harness (`skills/project-management/harness/*.py`, `SKILL.md`, `RULES.md`);
  this branch replaces them with the package and `pm prime`.
