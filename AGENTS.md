# Developing pm

Guidance for an agent changing pm itself: its code, tests, hooks and site. `CLAUDE.md` is a symlink to this file.
An agent that only uses pm in a repo gets its context from `pm prime`, `pm show` and `pm <noun> --help`; nothing here
is for them, and nothing here ships: the release binary embeds `prime.md`, `style.css` and `prompts/` (`assets.go`),
and not this file.

## Layout

pm is one Go module (`go.mod`); the pytest suite in `tests/` is its black-box harness, which drives the built binary.

| Path | Holds |
|---|---|
| `cmd/pm`, `assets.go` | The binary's entry point, which runs the launcher before anything else; `assets.go` embeds `prime.md`, `style.css` and `prompts/` |
| `prime.md` | The rules `pm prime` prints, naming the work store and pm's commands for it |
| `prompts/` | The model prompts and hook texts: the owner-request judge's system prompt and block reasons, `pm day summarize`'s prompt |
| `style.css` | The one stylesheet every page of the site gets |
| `internal/cli` | Every command: `commands.go` holds the command tree and every help text, `cli.go` the parser and `main`, `agentcmds.go` the agent commands (bodies in `writes.go`, `needs.go`, `day.go`, `show*.go`, `where.go`; shared context and checks in `agent.go`), `install.go` `pm init`, `doctor`, `upgrade`, `uninstall` and the git hooks, `serve.go` `pm service run`, `clean.go` `pm clean`, `push.go` `pm push`, `ownerrequest.go` the owner-request hook's work-store read. The commands outside the command tree (`pm version`, `pm export [--store DIR]`, `pm init --import-bd FILE`, and in `store_commands.go` `task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`, and the forms `show ID` and `task add --parent TASK`) are dispatched by `goOnly` in `work.go` |
| `internal/hooks` | `pm prime` (SessionStart and SubagentStart context), `pm hook stop` (uncommitted records) and `pm hook owner-request` (the Haiku judge and its `claude -p` arguments) |
| `internal/work` | The work store on embedded Dolt: schema, invariant checks, ids, ready and blocked, the bd import, `pm export` and its import (`pm init --import`), sync through the git remote under `refs/pm/work` with the merge rules in `merge.go` and the child-id compare-and-swap on the scratch branch `pm-cas` in `sync.go`. `host.go` is the host, which only `pm service run` starts: the Dolt engine held open and served on `<main>/.pm/run/work.sock` with `pm_version()`, `pm_sync()`, `pm_create(?)` and `pm_setup()`; `dolt.go` the client every command and the service's own loops use, `work.Dial`, with the version handshake, and every write under the store's fair write lock, `lock.go`, which the host serves as `pm_lock()` and `pm_unlock()`. `sync_test.go` runs two hosted clones on a local bare repo, `access_test.go` holds the one access path (only `host.go` loads an engine, only `pm service run` starts a host), `bench_test.go` the store benchmarks, which run with `PM_BENCH_STORE=<a copy of a clone's .pm/store/work>`; `worktest` holds `Serve` (a host and a client on a short temp clone, for a Go test that needs a store) and a read-only fake store |
| `internal/records`, `internal/store` | Record parsing and checks; the records store (the `records` worktree, its lock, design dates) |
| `internal/site` | The pages, rendered on demand by the service (`serve.go`: reply forms, the status line) and by `pm check` |
| `internal/service` | The pm service: `service.Run` against the work store's client, its change mark, sync and gc, and a `Site` interface; its unit files and lifecycle. `internal/cli/serve.go` wires it to the clone: `pm service run` hosts the work store (`work.NewHost`) and reaches it through its own socket; `pm service install` and `restart` manage a unit that runs the pm at `config.BinPath()` |
| `internal/sync` | The push steps and their state, which `pm push` (`internal/cli/push.go`) runs once |
| `internal/install` | `pm init`, `doctor`, `upgrade`, `uninstall`: the repo's pieces (`pieces.go`; pm's git hook section lives in its own tracked `.pm/hooks/post-checkout`), the pieces an earlier pm wrote and this one retired with the main branch's `records/` copy (`Retired` in `pieces.go`: the copy and guard workflows and the `pre-commit` section, named by `pm doctor` and removed by `pm upgrade`; the per-worktree sparse checkout, turned off by `pm init` in `clone.go` once the worktree tracks no `records/`), Beads' pieces taken out by `pm init` and `pm upgrade` and named by `pm doctor` (bd's hook entries in `.claude/settings.json` and `.codex/hooks.json`, the Beads block in `CLAUDE.md` and a non-link `AGENTS.md`; `.beads/` itself stays), the records-branch bootstrap, the clone's setup (`clone.go`) with the work store attached to the remote's `refs/pm/work` (`workstore.go`: cloned, or created and pushed), `core.hooksPath` set to the main checkout's `.pm/hooks` (moved off Beads' `.beads/hooks`), Codex roots (`codex.go`), pm copied into the bin dir (`binary.go`), and the pre-package harness's pieces found and refused (`legacy.go`) |
| `internal/launch` | The launcher, which `cmd/pm` runs first: the installed pm runs each repo's pinned version. A Go pin (0.2.0 and up) execs `pins/<pin>/pm`, downloaded once from release `pm-v<pin>` with no token (`$PM_RELEASE_URL` replaces GitHub's release download URL), and when that fails, through the GitHub API with a token from `$GH_TOKEN` or `gh auth token` (`$PM_RELEASE_API` replaces the API URL), then checked against `SHA256SUMS` and the kept `sha256` (`go.go`); a pin below 0.2.0 (a retired Python release) or one that names no release fails hard, naming `pm upgrade --to <X>`; `How()` is `pm where`'s line; `latest.go` reads the latest release (`<api>/releases/latest`) for `pm doctor`'s line on a newer one. `pm version` prints the build's version, `dev` when untagged |
| `internal/config`, `buildinfo`, `proc`, `pyjson` | `.pm/config.toml` (every command fails hard without it or on another pinned version); the build's version; subprocess runs; JSON written as pm has always written it |
| `release/build.sh`, `install.sh` | The release build: `build.sh OUT_DIR [pm-v<X>]` builds this machine's binary with `X` from the tag (else the `pm-v*` tag on HEAD, else `dev`) and packs `pm-<X>-<os>-<arch>.tar.gz`; `.github/workflows/pm-release.yml` runs it (Releasing pm). `install.sh`, a release asset with `@VERSION@` filled in, installs that release's binary to `${PM_BIN_DIR:-$HOME/.local/bin}/pm` after checking it against `SHA256SUMS`, downloading as the launcher does (with curl or wget; the token path needs curl); `tests/test_release.py` runs both |
| `tests/` | The harness: the pytest suite, its fakes and the live eval (below); `tests/render-pages` prints every page as the service renders it, for the harness's page tests. `pyproject.toml` and `uv.lock` are its environment |
| `CHANGELOG.md`, `release/changelog.py` | Releases' notes, and their checker: `check`, `notes X`, `pr BASE` (Releasing pm) |
| `.github/workflows/` | CI (`pm-tests.yml`: the harness; `pm-go.yml`: the Go build and tests; `pm-changelog.yml`) on every PR and push to main, the release build test (`pm-release-build.yml`) when a change can alter the release build and on a `pm-v*` tag, the release (`pm-release.yml`) on a `pm-v*` tag, and a release's notes re-rendered by hand (`pm-release-notes.yml`). No branch tracks `records/`: records live only on the `records` branch |

pm tracks its own development here, with the release that `.pm/config.toml` pins: the work store (the remote's
`refs/pm/work`) holds the `pm-harness` project's sprints, tasks and needs, the `records` branch holds its records, and
session start runs `pm init --session-start` and `pm prime`. pm's plans, decisions and design pages (`pm-harness.md` and
its sub pages, `pm-go.md`, `pm-versioning.md`) live in those records; a design change edits the sub page it touches to
the new state, and the trail of findings stays in the sprint record.

## Tests

| Command | Runs |
|---|---|
| `make go-build` | pm built as released (cgo, `-tags gms_pure_go`, stripped) into `.go/pm`, and the harness's page renderer into `.go/render-pages`, both reporting `VERSION` (Makefile), the version the harness's test repos pin. Every target below runs it first |
| `make test` | The light harness set: `tests/run.py -n auto -m "not integration"` against `.go/pm` |
| `make test-full ARGS="-k serve"` | The harness tests `-k` selects, the integration ones too; without `ARGS` it refuses (`CI=1` forces the whole set) |
| `uv run pytest -q -n auto tests/test_hooks.py` | One file, or `-k name` for one test, after `make go-build` |
| `make test-live` | The live eval: `PM_LIVE_TESTS=1`, `-k owner_request_prompt_live`; needs `claude` on PATH, logged in |
| `make test-go` | `go vet` and `go test ./...` with `-tags gms_pure_go` (Dolt needs it), then `internal/service` and the work store's concurrency tests (`RACE_TESTS`) again under `-race`: its parts `go-build`, `go-vet`, `go-test` (`GO_PKGS` narrows it: `make go-test GO_PKGS=./internal/work`) and `go-test-race`. `.github/workflows/pm-go.yml` runs the parts as parallel jobs on macOS and Linux (build, vet and every package's tests but `internal/work`'s; `internal/work`'s; the race tests), with `-count=1` and the Go caches the last push to main saved. The bd import's round trip runs on `internal/work/testdata`; `PM_BD_EXPORT=<bd export > file> PM_BD_RECORDS=$(pm where records)` runs it on a real export |

Run `make test` while working. When a change touches what an integration test covers (the service and its site,
`init`, `push`, the session-start hook), run just those tests with `-k` while iterating, not the whole set.
Before `pm action need --pr`, the PR's CI run must be green (`gh pr checks <n> --watch`):
`.github/workflows/pm-tests.yml` runs the light set and the integration set as separate jobs on every PR and push to main.
Do not run the whole integration set locally: CI runs it on every PR, and that run is the check. Push, then watch
it; to reproduce a CI failure, run only the failing tests with `make test-full ARGS="-k …"`.

Frozen expectations in the Go tests: `internal/site/golden_test.go` holds every page of
`internal/site/testdata/constructs` (a fixture for each construct a page renders) to its copy under
`testdata/constructs/pages`, and `internal/install/golden_test.go` the managed pieces on a table of inputs to
`testdata/pieces.json`; after an intended change, rewrite them with
`go test -tags gms_pure_go ./internal/<pkg> -run Golden -update` and review the diff in the PR. `internal/records`
holds the scanning functions and YAML 1.1 typing to `testdata/expected.json`, edited by hand.

A harness test reads work data as work-store items, as `pm export` gives them: `repo.items()`, and `repo.changes()`,
what pm changed since the repo was set up or `repo.mark()`, without store stamps; `repo.unchanged()` is the strict
"nothing written" check (every item equal, stamps included). Seeds are bd issues, the form the work store imports,
written only through `repo.set_issue` and `repo.add_issue`: ids `<prefix>-<root>(.<n>)*` (`repo-demo.1`), stamps as bd
writes them, an `in_progress` issue with its `claimed_by`. The fixture imports them with `pm init --import-bd` and reads
items with `pm export --store`; a seed imports them anew, so it must come before pm's first write.

A test is marked `integration` when it starts the pm service, renders the whole site (`Repo.pages`), sets a clone up
(`pm init`, `upgrade`, `uninstall`, `doctor`), reaches a git remote (`clone`, `fetch`, `pull`, `push`, a
bare repo), runs `pm push` or the session-start hook (`pm prime` without `--rules` or `--subagent`). The autouse
fixture `light_unless_integration` in `conftest.py` fails an unmarked test that starts one of these (`integration_only`
names them).

What the tests are:

- `test_pm.py`: each command against a temp repo; every refusal changes nothing, every happy path writes what it says.
  `test_config.py`: the config check, and each config key named in some `--help`. `test_launch.py`: the launcher; a
  pin below 0.2.0 refused, and a pin against releases a local HTTP server serves through `PM_RELEASE_URL` (fixture
  `release`; each tarball holds a fake `pm` script that prints its argv, markers and stdin) or, when that download
  fails, through a stand-in for GitHub's API (`GitHub`, fixture `github`: the token, and none to the storage host an
  asset redirects to); fakes of `uv` and `git ls-remote` log any call, which no launch makes. A test that pins another
  version and does not test the launch sets `PM_LAUNCHED=<pin>`, as a launched pm has it, or it would reach GitHub. `test_service.py`: `pm service` end to end.
  `test_init.py`, `test_lifecycle.py`: `pm init`, `doctor`, `upgrade` and `uninstall` on temp clones.
  `test_hooks.py`: `pm prime` and `pm hook stop`, run as the runtimes run them (JSON on stdin), against the rules'
  chunks `conftest.chunks()` writes out from the design. `test_owner_request_hook.py`: the owner-request hook in a temp
  repo against a fake judge. `test_owner_request_prompt_live.py`: the judge's accuracy, with the real model.
  `test_release.py`: `install.sh` against a local server, and (in `pm-release-build.yml`, `PM_RELEASE_BUILD=1`, as it
  builds twice) the release build in a scratch clone, tagged then untagged.
  `test_clean.py`: `pm clean` on agent worktrees beside a bare origin: dirty, unpushed, merged, squash-merged,
  pushed, locked by a live or a dead process, used by a live session, and the main checkout, store and caller kept.
- Fixtures (`conftest.py`): `repo` is a temp main checkout with its store at `.pm/store/records` on branch
  `records` and the `records/` link, as `pm init` leaves a clone; it also starts `pm service run` for the clone
  (`Repo.start_service`, on a free port), which every work-store read and write goes through, and stops it at
  teardown; it is no `integration` test's service. A test that starts the clone's own `pm service run` calls
  `repo.stop_service()` first and `repo.start_service()` after; the fake supervisor stops it itself when it starts
  the clone's installed service. The socket path must fit the kernel's 104 bytes, so `pytest_configure` sets a short
  `--basetemp` under `/tmp` unless one is given. Its env puts the fakes first on PATH, links the pm under test as
  `$HOME/.local/bin/pm`, and points `HOME`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME` at temp dirs. `pytest_configure`
  puts `.go` first on the test process's own PATH, for the git hooks pm installs, and points its `HOME`,
  `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and `XDG_CONFIG_HOME` at a temp dir, so git and the pm hooks it runs never touch
  the user's files; an autouse check fails a test that changes the user's Codex config or pm service units.
  `test_pm.py` adds `served` (a `pm service run` on a free port), `origin` (a cut-over origin, for `pm init` in a
  second clone) and `pushed` (a bare origin plus a second clone, for `pm push`).
- Fakes: `fake_gh.py` answers `gh pr view` and `gh pr list --head` from `$FAKE_GH_STATE`. `fake_env` sets
  `PM_RELEASE_API` to a port that refuses, so `pm doctor` never reaches GitHub; a test that wants a release serves one
  there. `fake_claude.py` stands in for `claude -p` and logs each call. `fake_sched.py` stands in for `launchctl`,
  `systemctl` and `crontab`, with each unit's enabled state (what `pm service stop` sets).

## Releasing pm

A release is its notes and a tag: a PR that gives `X` its section in `CHANGELOG.md`, then
`git tag pm-v<X> <a commit on main> && git push origin pm-v<X>`. No bump commit, and no code file holds `X`:
`.github/workflows/pm-release.yml` takes `X` from the tag name, and `release/build.sh` stamps it into the binary
(`-ldflags -X …/buildinfo.Version=<X>`); an untagged build reports `dev`, which no repo pins. On the tag push the
workflow:

| Job | Does |
|---|---|
| `version` | Takes `X` from the tag; it must be at least 0.2.0 (below it are Python pm's retired releases) and name a commit on main, unless it is a pre-release, and its `CHANGELOG.md` must pass `release/changelog.py check` and hold `X`'s notes (`changelog.py notes X`) |
| `build` | `release/build.sh dist pm-v<X>` natively on `macos-14` (darwin-arm64) and `ubuntu-22.04` (linux-amd64), cgo needing native runners; checks the tarball holds one `pm` whose `pm version` prints `X` |
| `release` | `SHA256SUMS` of both tarballs and `install.sh` with `X` filled in; `gh release create pm-v<X> --verify-tag` with the four assets, `--prerelease` when `X` has a `-` suffix, and notes `changelog.py notes X`: `X`'s section, its upgrade guide first |

- Write the notes first. `CHANGELOG.md` (Keep a Changelog, rules in its preamble) holds every Go release's notes,
  for pm's users. Each release opens with `### Upgrade guide`, a numbered list of the exact steps from the previous
  release (install it, `pm upgrade --to X`, then whatever else, such as `pm service restart` in each clone); a
  breaking change says what breaks and why, and points at its step. A PR that changes what a release ships
  (`SHIPPED` in `release/changelog.py`: Go sources but tests, embedded files, `go.mod`, `install.sh`,
  `release/build.sh`) adds an entry under `## [Unreleased]`; when users would not notice it, label the PR
  `no-changelog` instead. `pm-changelog.yml` checks both on every PR.
- To release `X` (SemVer from `[Unreleased]`: a breaking change bumps the minor while pm is 0.x, an addition the
  minor too, else the patch), rename `## [Unreleased]` to `## [X] - <date>` in a PR, add the summary paragraph and
  the upgrade guide, an empty `## [Unreleased]` above and the link references, merge it, then tag the merge. A tag
  whose changelog has no `X` section fails before any build.
- Fix or backfill a published release's notes by changing its section on main, then
  `gh workflow run pm-release-notes.yml -f version=<X>`: it replaces the release's notes alone (`gh release edit`).

- Cut a release candidate as `pm-v<X>-rc.<n>` (a GitHub pre-release), on any commit, a PR's included, to check the
  release build before the PR merges; the launcher treats it as version `X`'s pre-release, so a repo can pin it to
  try it. Its notes are `X`'s section when the changelog has one, else `[Unreleased]`, which must have an upgrade guide.
- Never move or recreate a release tag, and never rebuild a release's assets: launchers keep each binary's sha256
  in `<data dir>/pm/pins/<X>/sha256` and fail hard when a later download differs.
- Moving a repo's pin is a separate, ordinary PR once the release exists (`pm upgrade --to X`, merged any way).
- Install it on a machine with `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh | sh`.
  The repo is public, so `install.sh` and the launchers download with no token; only when that fails, a token from
  `$GH_TOKEN` or `gh auth token` makes them try the GitHub API, as a private copy of the repo needs.
  `$PM_RELEASE_URL` names a mirror.
- `test_changelog.py` checks `changelog.py`'s rules, notes and PR check; `test_release.py` checks the rest:
  `install.sh` against a local server, and (in `pm-release-build.yml`, `PM_RELEASE_BUILD=1`, as it builds twice) the
  build in a scratch clone, tagged then untagged. `pm-release-build.yml` runs on a PR or push that changes `release/`,
  `install.sh`, `go.mod`, `go.sum`, `internal/buildinfo`, the test or itself, and on every `pm-v*` tag.

## The judge and its live eval

The owner-request Stop hook calls Haiku through `claude -p` (`judgeArgs` in `internal/hooks/ownerrequest.go`, thinking
off; its prompt is `prompts/owner_request_system.txt`).
Its labelled cases are `tests/owner_request_cases.json`: a final reply, the open needs the work store holds and the
verdict the rule gives. After editing the prompt, run `make test-live`: each case runs `PM_LIVE_RUNS` times
(default 3), 8 calls at once, and every run must give the case's verdict; it prints pass counts and latency.
A case's label comes from the rule, never from what the judge answers: a miss is a prompt change, not a relabel.
It runs the hook with the user's `HOME` and Claude config (`REAL` in `conftest.py`), where the judge's `claude` login is.
The rule is one rule in three texts: the request rule in `prime.md`, the judge prompt and the block reasons
(`owner_request_reason.txt`, `owner_request_needless.txt`). A rule change edits all of them in one commit and adds a
labelled case for each case it moves.

## Changing prime.md and the hooks

- `pm prime` prints `prime.md` then the noun list, which `hooks.Commands` builds from the command tree
  (`Machinery` = `prime`, `hook`, `push` are left out). A new noun needs no edit to `prime.md`;
  `test_prime_lists_every_agent_command_pm_help_lists` checks the list against `pm --help`. A new command's rules go in
  its `--help`, and `prime.md` names it only where a procedure runs it.
- Claude Code passes each hook's `additionalContext` inline only up to `hooks.Cap` (10,000 characters; a longer
  one arrives as a 2 KB preview and a file path), per hook, and the hooks of one entry arrive in any order. So the
  rules run as one hook per chunk: `hooks.Chunks` cuts them at the headings in `hooks.Starts` and puts a title
  line naming each chunk's place and sections on top (`pm prime --rules N`), and the state runs as its own hook,
  cut at a line. 2026-10-07: 4 chunks of 8,447, 5,749, 6,317 and 4,708 characters, 24,872 without titles.
  `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` (and `TestChunksFitTheCapAndAddUpToTheHead`) fails when a
  chunk outgrows the cap: move a heading in `Starts` and in the harness's `RULE_STARTS`, or add one plus its hook
  entries; `test_init_bootstraps_a_brand_new_repo` checks the entries.
- `prime.md` carries only what a user's agents need (owner decision, 2026-10-07). Guidance for developing pm,
  `[TEST]` needs and this repo's checks go here, never in `prime.md` or a `--help` text.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `internal/cli` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts; grep it before rewording one.
- pm reads stdin only for `--text-file -`, and only from a heredoc or pipe (`readTextFile` refuses any other
  stdin at once, since an agent's shell may hold it open as a socket or tty that never ends); hooks read their JSON
  input from stdin. Every body command takes `--text` and `--text-file`. Docs, `--help` texts and hints give a body
  with `--text-file - <<'EOF'`, or `--text="…"` for one plain line; never `--text "…"`.
- `pm init` writes the hook entries (`claudeHooks` and `codexHooks` in `internal/install/pieces.go`) into
  `.claude/settings.json` (SessionStart: `pm prime --rules N --hook-json` for each chunk, then `--state
  --hook-json`; SubagentStart: the same chunks, then `--subagent --hook-json`; Stop: `pm hook owner-request` then
  `pm hook stop`, each `|| exit 1`) and `.codex/hooks.json`, with `hooks = true` in `.codex/config.toml`. A hook
  change edits both functions. Hooks fail open on their own errors (one line on stderr); a `pm` missing from PATH fails
  each hook with the shell's error.
- `pm init` is the one install command. Its repo half (pm's pieces, the records branch) runs only when the worktree has
  no `.pm/config.toml`; after that `pm doctor` reports and `pm upgrade` rewrites a piece. Its clone half (the work
  store, the records store, the `records/` link, excludes, Codex and Claude Code dirs, pm in the bin dir, the service)
  runs every time: `pm prime --state` runs it (`pm init --session-start` without `$PORT`) at each session start,
  within its timeout; that run installs a missing service and starts an installed one that does not answer, once for
  parallel session starts, and leaves a running one, a stale one included, to a typed `pm init` or
  `pm service restart`. In a linked worktree, a branch without `.pm/config.toml` is refused, and a main checkout on
  another pin gets the worktree's setup but no binary or service.
- Agents change code only in a worktree of their own: `pm task claim` refuses in the main checkout (the worktree whose
  git dir is the common one) and prints how to make one under `.claude/worktrees/`.

## The site

- `pm check` renders every record with the work store and writes nothing (the check before a records commit); the pm
  service serves the site on the port in `.pm/config.toml` (`PORT=` overrides it).
- One stylesheet, `style.css`, for every page; a look it cannot express is added there, never to a record.
- The service also serves the records store's image files (`ImageTypes` in `internal/service/run.go`) at their store
  path, confined to the store: dot-led and empty path parts are refused, in the path asked for and in the file it
  resolves to, and `os.Root` refuses `..` and symlinks out.
  The pages load Mermaid at one exact version (`MermaidVersion` in `internal/site/site.go`), which
  `TestMermaidLoadsOneExactVersionAtNaturalWidth` holds.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer. A
  generated section (Progress, Decisions await you, Actions await you, a day's Sprints, Not in a sprint) is rendered
  from the work store. Not in a sprint, on a project's page and the overview, lists each open task or need filed
  directly under a project, or (overview only) with no parent; a need under a project is left out, since the
  await-you sections show it.
  `test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does` holds the render check.
- Day summaries live beside the day record as `records/days/<date>.summary.json`, written by `pm day summarize`.
- A page change shows in the golden pages (`internal/site/testdata/constructs/pages`); a construct the fixture lacks
  gets a record or item in `internal/site/testdata/constructs`.

## Live checks

A change to setup, the hooks, the site or replies gets a live check besides its tests, in the form the sprint's
"Done when" names.

- A need raised during a test or a live check starts its title with `[TEST]`; once the check is done, close it
  with `pm need dismiss <id> --reason "[TEST] done"`. `pm sprint close` skips a dismissed review, so a `[TEST]` review
  never blocks a close.
- A scratch worktree checks the session-start path: `git worktree add --no-checkout` then `git reset --hard`
  (Claude Code's own sequence), or a session started with `claude -w <name>`; a headless `claude -p` session
  checks a write through `records/`. Remove the worktree and its branch afterwards (`git worktree remove`,
  `git branch -D`).
- A check of `pm init`, `pm push` or the service runs in a scratch clone of a scratch origin under a
  temp directory with `HOME` and `CODEX_HOME` there, as the fixtures do, never on a real clone's store or service.
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.

## Elsewhere

- A clone set up by the old harness (a `.records` store, the old push job, path-based hook entries) is refused by
  `pm init`, which names the one-time move: the retired Python release (0.1.x) run once through `uvx`, then
  `pm upgrade --to <X>` to a release from 0.2.0 on, then `pm init`; `internal/install/legacy.go` lists what it looks
  for.
