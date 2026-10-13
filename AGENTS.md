# Developing pm

Guidance for an agent changing pm itself: its code, tests, hooks and site. `CLAUDE.md` is a symlink to this file. An
agent that only uses pm in a repo gets its context from `pm prime`, `pm show` and `pm <noun> --help`; nothing here is
for them, and nothing here ships: the release binary embeds `prime.md`, `style.css`, `prompts/` and the release
notes (`assets.go`), not this file. This repo's own sessions load `prime.md` too, so its rules are not repeated here.

## Map

pm is one Go module; the pytest suite in `tests/` is its black-box harness, which drives the built binary. Each Go
package's doc comment holds its file map and detail (`go doc ./internal/<pkg>`); each test module's docstring says what
it covers.

| Path | Holds |
|---|---|
| `cmd/pm`, `assets.go` | The entry point (the launcher runs first); the embedded files, the release notes among them |
| `prime.md`, `prompts/`, `style.css` | What `pm prime` prints; the model prompts and hook texts; the site's one stylesheet |
| `internal/cli` | Every command; `commands.go` holds the command tree and every `--help` text |
| `internal/hooks`, `internal/ask` | What the runtimes' hooks run (`pm prime`, `pm hook …`); one prompt through `claude -p` or `codex exec` |
| `internal/work` | The work store on embedded Dolt, its host (only `pm service run` starts one), client, sync and merge |
| `internal/records`, `internal/store`, `internal/design` | Record parsing and checks; the records store; design pages' hold on the code they cover |
| `internal/site`, `internal/service` | The pages; the pm service, its units and its access check |
| `internal/install`, `internal/launch` | `pm init`, `doctor`, `upgrade`, `uninstall`; the launcher that runs a repo's pinned release |
| `internal/session`, `internal/sync` | Which session runs pm and which are live; the push steps |
| `internal/config`, `buildinfo`, `proc`, `pyjson`, `textwrap` | `.pm/config.toml`; the build's version; subprocesses; JSON as Python pm wrote it; Python's `textwrap.fill` |
| `tests/` | The harness: `conftest.py` (fixtures, seeds, the integration guard), the fakes, the live evals |
| `release/`, `install.sh`, `CHANGELOG.md`, `changelog.d/` | The release build and tools (`release/README.md`); released notes; unreleased entry files |
| `.github/workflows/`, `.github/scripts/` | CI on every PR and push to main, the release, the website deploy; their helpers |
| `website/` | pm's public website, one static page; not shipped |

pm tracks its own development with the release `.pm/config.toml` pins: its projects' sprints, tasks and needs in the
work store (the remote's `refs/pm/work`), its records (plans, decisions, design pages) on the `records` branch, which
no code branch tracks. A design change edits the design page it touches to the new state; the trail of findings stays
in the sprint record. A PR that changes files a design page `covers:` updates that page or waives it
(`pm design unchanged`); `pm action need --pr` and `pm sprint close` refuse otherwise.

## Commands

| Command | Runs |
|---|---|
| `make go-build` | pm built as released (cgo, `-tags gms_pure_go`, stripped) into `.go/pm`, plus `.go/render-pages` and `.go/pm-before` (the pin-move test's older pm). `test`, `test-full`, `test-live`, `test-live-parity` and `test-go` run it first; `lint` and the single `go-*` parts do not |
| `make test` | The light harness set: `tests/run.py -n auto -m "not integration"` against `.go/pm` |
| `make test-full ARGS="-k serve"` | The harness tests `-k` selects, integration ones included; without `ARGS` it refuses (`CI=1` forces the whole set) |
| `uv run pytest -q -n auto tests/test_hooks.py` | One file, or `-k name` for one test, after `make go-build` |
| `make test-go` | `go-build`, `go-fmt` (`gofmt -l`, fails naming each file), `go-vet`, `go-test-but-work`, `go-test-work` (`internal/work` in `WORK_SHARDS` processes), `go-test-race` (`make -j2 go-test-race` runs its two halves at once). `make go-test GO_PKGS=./internal/service` runs one package. Go commands need `-tags gms_pure_go` |
| `make lint` | `lint-go` (golangci-lint, on PATH at `GOLANGCI_LINT_VERSION`: gating linters on every line, ratcheted ones on lines changed since the merge base with `origin/main`), `lint-deadcode`, `lint-py` (`ruff check`) |
| `make test-live` | The owner-request judge's live eval on each runtime (`PM_LIVE_RUNTIMES`, default `claude codex`; each CLI on PATH, logged in) |
| `make test-live-parity` | The parity live eval: Claude Code and Codex sessions get the same pm guarantees; needs both CLIs, logged in |
| `make merge-ready PR=<n>` | The check before every merge (Merging) |

## Tests

- Run `make test` while working. When a change touches what an integration test covers (the service and its site,
  `init`, `push`, the session-start hook), run just those tests with `make test-full ARGS="-k …"`.
- **Never run the whole integration set locally.** `.github/workflows/pm-tests.yml` runs it on every PR, and that run
  is the check: push, then watch it (`gh pr checks <n> --watch`); to reproduce a CI failure, run only the failing tests
  with `make test-full ARGS="-k …"`. The PR's CI must be green before `pm action need --pr`.
- **Mark a test `integration`** when it starts the pm service, renders the whole site (`Repo.pages`), sets a clone up
  (`pm init`, `upgrade`, `uninstall`, `doctor`), reaches a git remote (`clone`, `fetch`, `pull`, `push`, a bare repo),
  runs `pm push`, or runs the session-start hook (`pm prime` without `--rules` or `--subagent`). The autouse fixture
  `light_unless_integration` in `conftest.py` fails an unmarked test that starts one of these.
- **A test that pins another version and does not test the launch sets `PM_LAUNCHED=<pin>`**, as a launched pm has
  it; without it the launcher downloads that release from GitHub.
- Seed work data only through `repo.set_issue` and `repo.add_issue`, before pm's first write (a seed re-imports the
  store), and read it with `repo.items()`, `repo.changes()` or `repo.unchanged()`; `conftest.py`'s docstring has the
  forms.
- **Golden files**: `internal/site/golden_test.go` (every page of `testdata/constructs`) and
  `internal/install/golden_test.go` (`testdata/pieces.json`). After an intended change, rewrite them with
  `go test -tags gms_pure_go ./internal/<pkg> -run Golden -update` and review the diff in the PR; never hand-edit a
  golden file to make a test pass. A page construct the fixture lacks gets a record or item in
  `internal/site/testdata/constructs`. `internal/records/testdata/expected.json` is edited by hand.
- **After adding or renaming an `internal/work` test, run `make go-test-work UPDATE=1`**: it measures and rewrites
  `.github/scripts/work-test-seconds.txt`, which must name every test and no other, or `go-test-work` fails.
- A lint finding is fixed, or kept with `//nolint:<linter> // <why>` (`# noqa: <code> — <why>` in Python). A test
  waits on a condition with a deadline, never a fixed sleep. `size_test.go` caps a package's non-test Go lines at
  5000 (`internal/cli` is on its ratchet allowlist, `oversized`): split a package rather than raise the cap.

## Merging

Merge a PR only through `make merge-ready PR=<n>`, run right before the merge, then the
`gh pr merge <n> --squash --match-head-commit <sha>` it prints. Each PR's CI runs against main as main was then, so two
PRs that pass alone can break main together; the check refuses, naming why, unless the PR's head contains `origin/main`
as the remote has it now and every check on that head passed. A refused PR is rebased onto `origin/main`, pushed, and
merged once its new CI passes. `release/merge_ready.py`'s docstring has the exact conditions.

## Changelog and releases

- **A PR that changes what a release ships** (`SHIPPED` in `release/changelog.py`: Go sources but tests, embedded
  files, `go.mod`, `install.sh`, `release/build.sh`) **adds one entry file, `changelog.d/<slug>.md`**, in the form
  `CHANGELOG.md`'s preamble gives. Never edit `CHANGELOG.md`: the release writes it from the entries, so parallel PRs
  never conflict. When users would not notice the change, label the PR `no-changelog` instead. `pm-changelog.yml`
  checks both on every PR.
- Every merge to main that carries an entry releases, with no one cutting it; `release/README.md` holds the workflow,
  the version rule, release candidates, notes fixes and a failed release's recovery.
- **Never move or recreate a release tag, and never rebuild a release's assets**: launchers keep each binary's sha256
  and fail hard when a later download differs. A failed publish is finished by re-running its failed jobs.
- Moving a repo's pin, this one's included, is a separate ordinary PR (`pm upgrade --to X`).

## prime.md, --help and the hooks

- `prime.md` carries only what a user's agents need. Guidance for developing pm, `[TEST]` needs and this repo's checks
  go here, never in `prime.md` or a `--help` text.
- A new noun needs no edit to `prime.md`: `pm prime` appends the noun list built from the command tree
  (`test_prime_lists_every_agent_command_pm_help_lists`). A new command's rules go in its `--help`; `prime.md` names
  it only where a procedure runs it.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `internal/cli` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts: grep it before rewording one.
- Claude Code passes each hook's `additionalContext` inline only up to `hooks.Cap` characters, per hook, so the rules
  run as one hook per chunk, cut at the headings in `hooks.Starts`. When
  `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` or `TestChunksFitTheCapAndAddUpToTheHead` fails, move a
  heading in `Starts` and in `conftest.py`'s `RULE_STARTS`, or add one plus its hook entries
  (`test_init_bootstraps_a_brand_new_repo` checks the entries). Two PRs that each fit the cap can overflow it together.
- **A hook change edits both `claudeHooks` and `codexHooks`** (`internal/install/pieces.go`);
  `TestClaudeAndCodexRunTheSameHooks` fails on a difference `runtimeDifferences` gives no reason for.
- Hooks fail open on their own errors (one line on stderr). Stop runs no inbox hook: a Stop block ends the
  continuation with `stop_hook_active`, which would let the stop checks through.

## pm init

`pm init` is the one install command; keep it that way rather than adding a setup command. Its repo half (pm's
pieces, the records branch) runs only when the worktree has no `.pm/config.toml`; after that `pm doctor` reports and
`pm upgrade` rewrites a piece. Its clone half (the stores, the `records/` link, excludes, the Codex and Claude Code
dirs, pm in the bin dir, the service) runs every time, at every session start too (`pm prime --state` runs
`pm init --session-start`). That run starts a service that does not answer but leaves a running one, a stale one
included, to a typed `pm init` or `pm service restart`; a typed `pm init` keeps an installed unit's PATH. Never make
session start restart a running service or take a session's PATH: each runtime runs pm with a PATH of its own, so
the service would restart under every session (the parity eval fails on it).

## The owner-request judge

- The Stop hook's judge runs on the session's own runtime (`--runtime claude` or `codex`, through `internal/ask`,
  whose doc says why its children run with no hooks). One prompt serves both: `prompts/owner_request_system.txt`.
- **The rule is one rule in three texts**: the request rule in `prime.md`, the judge prompt, and the block reasons
  (`prompts/owner_request_reason.txt`, `owner_request_needless.txt`). A rule change edits all three in one commit and
  adds a labelled case to `tests/owner_request_cases.json` for each case it moves.
- **A case's label comes from the rule, never from what the judge answers**: a miss is a prompt change, not a relabel.
- After editing the prompt, run `make test-live`: every run of every case must give the case's verdict on each
  runtime. Record the pass counts and latency it prints in the sprint's Findings, not here.

## The parity live eval

`make test-live-parity` runs real Claude Code and Codex sessions in scratch clones and asserts from the work store,
the records store and each runtime's transcript, never from the agent's prose; `tests/test_agent_parity_live.py`'s
docstring lists the scenarios. A new scenario updates the Makefile's `-n` and the docstring's count
(`tests/test_ci.py` holds both to the scenarios pytest collects). Every copy of a login goes through
`agents.login_copy` (`test_login_copies.py` holds the harness to it). `PM_PARITY_CODEX_SANDBOX` picks Codex's mode:
`approve-for-me`, what a Codex user runs, or a sandbox mode (default `workspace-write`); a host where bubblewrap cannot
start passes only with `danger-full-access`.

## The site

- `pm check` renders every record with the work store and writes nothing: the check before a records commit.
- Who may reach the service is `refusal` in `internal/service/access.go`; the `site-replies` design page, Security,
  is the rule. Change the page and the code together.
- One stylesheet, `style.css`, for every page: a look it cannot express is added there, never to a record.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer.
- The pages load Mermaid at one exact version (`MermaidVersion` in `internal/site/site.go`); bump it there only.

## Live checks

A change to setup, the hooks, the site or replies gets a live check besides its tests, in the form the sprint's
"Done when" names.

- **A need raised during a test or a live check starts its title with `[TEST]`**; once the check is done, close it with
  `pm need dismiss <id> --reason "[TEST] done"`.
- **A check of `pm init`, `pm push` or the service runs in a scratch clone of a scratch origin** under a temp directory
  with `HOME` and `CODEX_HOME` there, as the fixtures do, never on a real clone's store or service.
- The session-start path is checked in a scratch worktree: `git worktree add --no-checkout` then `git reset --hard`
  (Claude Code's own sequence), or a session started with `claude -w <name>`. Remove the worktree and its branch
  afterwards (`git worktree remove`, `git branch -D`).
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.
