# Developing pm

Guidance for an agent changing pm itself: its code, tests, hooks and site. `CLAUDE.md` is a symlink to this file.
An agent that only uses pm in a repo gets its context from `pm prime`, `pm show` and `pm <noun> --help`; nothing here
is for them, and nothing here ships: the wheel holds only `src/pm/` (`[tool.hatch.build.targets.wheel]` in
`pyproject.toml`), so `prime.md`, `style.css` and `prompts/` ship and this file does not.

## Layout

| Path | Holds |
|---|---|
| `src/pm/cli.py` | The commands and their `--help` texts; `parser()` builds the argparse tree |
| `src/pm/hooks.py` | `pm prime` (SessionStart and SubagentStart context) and `pm hook stop` (uncommitted records) |
| `src/pm/owner_request.py` | `pm hook owner-request`: the Haiku judge and its `claude -p` arguments |
| `src/pm/prompts/` | The model prompts and hook texts both implementations read (`pm.prompt()`, `assets.go`): the owner-request judge's system prompt and block reasons, `pm day summarize`'s prompt |
| `src/pm/prime.md` | The rules `pm prime` prints; the only prose the package ships to agents |
| `src/pm/records.py`, `store.py`, `beads.py` | Record parsing and checks, the store (the `records` worktree and its lock), `bd` calls |
| `src/pm/site.py`, `style.css` | The site the pm service serves and `pm check` renders; the one stylesheet every page gets |
| `src/pm/service.py`, `push.py` | `pm service`: one supervised process per clone serves the site and runs `pm push` |
| `src/pm/install.py`, `tool.py`, `legacy.py` | `pm init`, `doctor`, `upgrade`, `uninstall`: the repo's pieces, the pm uv tool, the pre-package harness's pieces |
| `src/pm/config.py` | `.pm/config.toml`: every command fails hard without it or on another pinned version |
| `src/pm/launch.py` | The launcher: `main()` first runs the repo's pinned version through `uv tool run` when it is not this one |
| `tests/` | pytest suite, fakes and the live eval (below) |
| `go.mod`, `cmd/pm`, `internal/…`, `assets.go` | Go pm, the port the `pm-go` design page plans: built and tested on main, run by no repo until the cut-over. `internal/cli/commands.go` holds every command and help text, `internal/cli/agentcmds.go` runs the agent commands Go pm has ported (`show`, `record link`, `where`, `commit`, `day summarize`, the task, record, project and sprint writes; bodies in `writes.go`, `day.go`, `show*.go`, `where.go`, shared context and checks in `agent.go`), `internal/hooks` `pm prime`, `pm hook stop` and `pm hook owner-request` (its work-store read in `internal/cli/ownerrequest.go`), `internal/work` the work store on embedded Dolt (schema, invariant checks, ids, ready and blocked, the gate, the bd import and `pm export`, sync through the git remote under `refs/pm/work` with the merge rules in `merge.go` and the child-id compare-and-swap in `sync.go`), whose Go-only commands, `pm export [--store DIR]`, `pm init --import-bd FILE` (the import only, so far) and the work-store commands in `internal/cli/store_commands.go` (`task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`), stay out of the argparse tree, dispatched by `goOnly` in `internal/cli/work.go`; `internal/work/sync_test.go` runs two clones on a local bare repo; `internal/work/worktest` a read-only fake store, `internal/records` record parsing and checks, `internal/store` the records store, `internal/site` the pages; `internal/service` the pm service (`service.Run` against a `Store` and a `Site` interface, its unit files and lifecycle; Go pm's `pm service status` and `logs` run, while `run`, `install` and `restart` refuse until the work store and site are wired), `internal/sync` the push steps and their state; `internal/service/testdata/units` holds Python's unit files, which Go's and Python's tests both compare against; `assets.go` embeds `src/pm/prime.md`, `style.css` and `prompts/`, so both implementations read one copy |
| The pm uv tool | The `pm` on PATH that hooks, agents and the service run; `pm init` installs it from git (`tool.py`). It runs each repo's pinned version (`launch.py`). Run this checkout's code with `uv run --project pm pm …`; this checkout's pin is its own version, so it runs in process |
| `../.claude/settings.json`, `../.codex/hooks.json` | Where the runtimes wire the hooks (below) |
| `../.pm/config.toml` | This repo's pm config; its `version` must equal `version` in `pyproject.toml`, except at a release's first commit (Releasing pm) |
| `../records/design/pm-harness.md` | The harness design; one sub page per area (`pm-cli.md`, `owner-request-hook.md`, `records-store.md`, `site-replies.md`, …) |

A design change edits the sub page it touches to the new state; the trail of findings stays in the sprint record.

## Tests

| Command | Runs |
|---|---|
| `make test` (repo root) | The light set: `pm/tests/run.py -n auto -m "not integration"` in the package environment |
| `make test-full ARGS="-k serve"` (repo root) | The tests `-k` selects, the integration ones too; without `ARGS` it refuses (`CI=1` forces the whole set) |
| `uv run pytest -q -n auto tests/test_hooks.py` (in `pm/`) | One file, or `-k name` for one test |
| `make test-live` | The live eval: `PM_LIVE_TESTS=1`, `-k owner_request_prompt_live`; needs `claude` on PATH |
| `make test-go` (repo root) | Go pm built as released (cgo, stripped) into `pm/.go/pm`; `tests/go_parity_corpus.py` writes Python pm's pages, `pm check` results and reference outputs into `pm/.go/parity` (`PM_PARITY_LIVE=1` adds this clone's records and Beads data, which needs `bd`); `go vet` and `go test ./...` with `-tags gms_pure_go` (Dolt needs it) against that corpus (`PM_PARITY`; the corpus tests skip without it), then `tests/test_go_parity.py` against Python pm (`PM_GO`; skipped without it). `.github/workflows/pm-go.yml` runs it on macOS and Linux. The bd import's round trip and its agreement with `tests/work_items.py` run on `internal/work/testdata`; `PM_BD_EXPORT=<bd export > file> PM_BD_RECORDS=$(pm where records)` runs both on a real export |
| `make test-go-suite` (repo root) | The whole shared suite on Go pm (`PM_IMPL=go`), each test on `tests/go-expected-failures.txt` a strict xfail, so a listed test that passes fails the run until it leaves the list (it only shrinks); then `tests/compare_transcripts.py` runs the tests that pass on Go on Python pm and diffs their transcripts. `pm-go.yml` runs it after `make test-go` |

Run `make test` while working. When a change touches what an integration test covers (the service and its site,
`init`, `push`, the session-start hook), run just those tests with `-k` while iterating, not the whole set.
Before `pm action need --pr`, the PR's CI run must be green (`gh pr checks <n> --watch`):
`.github/workflows/pm-tests.yml` runs the light set and the integration set as separate jobs on every PR and push to main.
Do not run the whole integration set locally: CI runs it on every PR, and that run is the check. Push, then watch
it; to reproduce a CI failure, run only the failing tests with `make test-full ARGS="-k …"`.

The suite runs against either implementation (records/design/pm-go.md, Tests): `PM_IMPL=python` (the default) runs
this checkout's pm, `PM_IMPL=go` the Go binary at `$PM_GO_BIN`. A test for one implementation only is marked
`@pytest.mark.impl("python", reason="…")` and skipped on the other; today that is a test of Python code in process
(`pm.hooks`, `pm.service`, `pm.launch`, the parser) or of what Go retires (`tool.py`, `legacy.py`).

A test reads work data as work-store items, never as bd JSON or bd calls: `repo.items()` (for Python, the fake bd's
issues through `tests/work_items.py`, the work store's bd import mapping), and `repo.changes()`, what pm changed since
the repo was set up or `repo.mark()`, without store stamps; `repo.unchanged()` is the strict "nothing written" check
(every item equal, stamps included, and for Python no bd write). Seeds are bd issues, the form the work store imports,
written only through `repo.set_issue` and `repo.add_issue`: ids `<prefix>-<root>(.<n>)*` (`repo-demo.1`), stamps as bd
writes them, an `in_progress` issue with its `claimed_by`. For Go, the fixture imports them with `pm init --import-bd`
and reads items with `pm export --store`; a seed imports them anew, so it must come before Go pm's first write.
`repo.bd_calls()` stays for an assertion about Python pm's use of bd, under `if IMPL == "python"`.

Each test writes one transcript, `pm/.transcripts/<impl>/<test file>/<test>.json` (or under `$PM_TRANSCRIPTS`; a run
empties it first): every `repo.pm` call's argv, stdin, stdout, stderr, exit code, changed record files and store
export, normalised by `tests/transcript.py` keeping each value's shape (temp and checkout paths, random temp names,
commit ids and UUIDs numbered with their length, timestamps and today's date with digits as 0, durations, ports,
minted root ids, and `pm where`'s work-layer line, Beads in Python and the work store in Go). The same test's transcript
from Python and Go must be equal. Calls that bypass `repo.pm` (a direct `PM` subprocess) are not recorded.
`test_go_refusals.py` is the static check that each refusal text of a command Go pm runs (`PORTED`) appears, its
constant parts, in Go pm's source; a command ported to Go joins `PORTED`.

A test is marked `integration` when it starts the pm service, renders the whole site (`Repo.pages`), sets a clone up
(`pm init`, `upgrade`, `uninstall`, `doctor`), reaches a git remote (`clone`, `fetch`, `pull`, `push`, a
bare repo), runs `pm push` or the session-start hook (`pm prime` without `--rules` or `--subagent`, also run as
`python -m pm.cli`). The autouse fixture `light_unless_integration` in `conftest.py` fails an unmarked test that
starts one of these (`integration_only` names them).

What the tests are:

- `test_pm.py`: each command against a temp repo and a fake `bd`; every refusal changes nothing, every happy path
  writes what it says. `test_config.py`: the config check. `test_launch.py`: the
  launcher against a fake `uv` (logs argv, stdin and the `PM_LAUNCHED` markers) and a fake `git ls-remote`; its
  integration test builds release 0.1.0 with real uv from this clone's tag `pm-v0.1.0` (the release URL rewritten
  to this clone, so nothing reaches GitHub) and runs `pm show` in a repo pinned to it; another runs the release
  procedure (Releasing pm) through real git hooks and real uv against a scratch origin. A test that pins another
  version and does not test the launch sets `PM_LAUNCHED=<pin>`, as a launched pm has it, or it would reach GitHub. `test_service.py`: the service's units in process and
  `pm service` end to end. `test_tool.py`: the pm uv tool. `test_init.py`, `test_lifecycle.py`, `test_migrate.py`:
  `pm init`, `doctor`, `upgrade` and `uninstall` on temp clones, and the move off the pre-package harness.
  `test_hooks.py`: `pm prime` and `pm hook stop`, run as the runtimes run them (JSON on stdin). `test_owner_request_hook.py`: the owner-request hook in a temp repo against a
  fake judge. `test_owner_request_prompt_live.py`: the judge's accuracy, with the real model.
- Fixtures (`conftest.py`): `repo` is a temp main checkout with its store at `.pm/store/records` on branch
  `records` and the `records/` link, as `pm init` leaves a clone. Its env puts the fakes first on PATH and points
  `HOME`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME` at temp dirs. `pytest_configure` points the test process's own
  `HOME`, `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and `XDG_CONFIG_HOME` at a temp dir too, so git and the pm hooks it
  runs never touch the user's files, and an autouse check fails a test that changes the user's Codex config or
  pm service units. `test_pm.py` adds `served` (a `pm service run` on a free port), `origin` (a cut-over origin,
  for `pm init` in a second clone) and `pushed` (a bare origin plus a second clone, for `pm push`).
- Fakes: `fake_bd.py` serves issues from `$FAKE_BD_STATE` and logs calls to `$FAKE_BD_LOG`; `$FAKE_BD_FAIL`
  makes one call fail until `$FAKE_BD_HEAL` exists, `$FAKE_BD_HOLD` makes one wait. `fake_gh.py` answers
  `gh pr view` from `$FAKE_GH_STATE`. `fake_claude.py` stands in for `claude -p` and logs each call. `fake_sched.py`
  stands in for `launchctl`, `systemctl` and `crontab`.

## Releasing pm

A release is tag `pm-v<X>` on the commit whose `pyproject.toml` says `X`. The pre-commit hook runs the pm uv tool,
which launches the repo's pin and resolves its tag with `git ls-remote` (`launch.commit()`): a commit that moves the
pin to a version with no tag yet is refused. So the tag comes before the pin, in two commits on one branch from main:

| Step | Does | The pre-commit hook runs |
|---|---|---|
| 1. Commit A | `version` in `pm/pyproject.toml` to `X`, `uv lock --project pm`; the pin stays at the old version | the old release, in process or from its tag |
| 2. Tag A | `git tag -a pm-v<X> -m "pm <X>: …" <A>`, `git push origin pm-v<X>` (the push sends A too) | |
| 3. Commit B | `uv run --project pm pm upgrade` (never launched, so this checkout's pm `X` moves the pin and rewrites the Beads hook markers); commit the files it names | pm `X`, built from A |
| 4. PR | push the branch, open the PR, CI green; the owner merges it with a merge commit | |

- Never commit with `--no-verify`: the hook at B is the check that the tag builds the pinned pm.
- Never move or recreate a release tag: launchers keep its commit in `<data dir>/pm/pins/<X>/commit` and never
  resolve it again.
- Merge with a merge commit, never squash or rebase (GitHub allows all three here): either rewrites A, and main
  would then hold no commit the tag names.
- At A, the tests that run pm in this repo, such as the live eval (`test_owner_request_prompt_live.py`), fail: this checkout's
  pm is `X` and launches the old pin. Test at B, and push the branch only after B, so CI runs on B.
- `test_a_release_tags_before_it_pins_so_both_commits_pass_the_hook` (`test_launch.py`) runs these steps.

## The judge and its live eval

The owner-request Stop hook calls Haiku through `claude -p` (`JUDGE_ARGS` in `owner_request.py`, thinking off; its
prompt is `prompts/owner_request_system.txt`).
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
- Until the cut-over a change to a command's arguments or help, `pm prime` or `pm hook stop` lands in both
  implementations: `cli.py` and `internal/cli/commands.go`, `hooks.py` and `internal/hooks`. `make test-go` fails
  on any difference.
- `prime.md` carries only what a user's agents need (owner decision, 2026-10-07). Guidance for developing pm,
  `[TEST]` needs and this repo's checks go here, never in `prime.md` or a `--help` text.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `cli.py` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts; grep it before rewording one.
- pm reads stdin only for `--text-file -`, and only from a heredoc or pipe (`read_text_file()` refuses any other
  stdin at once, since an agent's shell may hold it open as a socket or tty that never ends); hooks read their JSON
  input from stdin. Every body command takes `--text` and `--text-file` through `add_text()`. Docs, `--help` texts
  and hints give a body with `--text-file - <<'EOF'`, or `--text="…"` for one plain line; never `--text "…"`.
- `pm init` writes the hook entries (`claude_hooks()` and `codex_hooks()` in `install.py`) into
  `.claude/settings.json` (SessionStart: `pm prime --rules N --hook-json` for each chunk, then `--state
  --hook-json`; SubagentStart: the same chunks, then `--subagent --hook-json`; Stop: `pm hook owner-request` then
  `pm hook stop`, each `|| exit 1`) and `.codex/hooks.json`, with `hooks = true` in `.codex/config.toml`. A hook
  change edits both functions; this repo's two files are what `pm init` writes. Hooks fail open on their own
  errors (one line on stderr); a `pm` missing from PATH fails each hook with the shell's error.
- `pm init` is the one install command. Its repo half (pm's pieces, `bd init`, the records branch) runs only when
  the worktree has no `.pm/config.toml`; after that `pm doctor` reports and `pm upgrade` rewrites a piece. Its clone
  half (Beads, the store, the `records/` link, excludes, Codex and Claude Code dirs, the pm uv tool, the service)
  runs every time: `pm prime --state` runs it (`hooks.INIT`, `pm init --session-start` without `$PORT`) at each
  session start, within `hooks.INIT_TIMEOUT`; that run installs only a missing service and reports a stale or
  down one, which a typed `pm init` or `pm service restart` restarts. In a linked worktree, a branch without
  `.pm/config.toml` is refused, and a main checkout on another pin gets the worktree's setup but no tool or service.
- Agents change code only in a worktree of their own: `pm task claim` refuses in the main checkout (the worktree whose
  git dir is the common one) and prints how to make one under `.claude/worktrees/`.
  Nothing else enforces it: this repo keeps Claude Code's `bgIsolation` at `"none"`, since its worktree isolation refuses
  record edits from a background session.

## The site

- `pm check` renders every record with Beads and writes nothing (the check before a records commit); the pm
  service serves the site on the port in `.pm/config.toml` (`PORT=` overrides it). This repo's site is at the
  `site_url` in `.pm/config.toml`, a tunnel to the pm service on the owner's machine.
- One stylesheet, `src/pm/style.css`, for every page; a look it cannot express is added there, never to a record.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer. A
  generated section (Progress, Decisions await you, Actions await you, a day's Sprints, Not in a sprint) is rendered
  from Beads. Not in a sprint, on a project's page and the overview, lists each open non-epic item filed directly
  under a project epic, or (overview only) with no parent; a need under a project is left out, since the await-you
  sections show it.
  `test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does` holds the render check.
- Day summaries live beside the day record as `records/days/<date>.summary.json`, written by `pm day summarize`.
- Until the cut-over the site lands in both implementations: `site.py` and `internal/site`. Go's pages must equal Python's after `site.Normalise` (entities decoded, attributes sorted, whitespace HTML does not render dropped) on every page of the corpus; a difference no renderer change can remove goes on `internal/site/testdata/parity-allow.txt` with its reason, reviewed in the PR that adds it. A construct the corpus lacks gets a fixture in `internal/site/testdata/constructs`.

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
- A check of `pm init`, `pm push` or the service runs in a scratch clone of a scratch origin under a
  temp directory with `HOME` and `CODEX_HOME` there, as the fixtures do, never on this clone's Beads, store or service;
  never run `bd init`.
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.

## Elsewhere

- A clone set up by the old harness (a `.records` store, the old push job, path-based hook entries) is moved onto installed pm by `pm init`; the list of what it removes is `legacy.py`.
