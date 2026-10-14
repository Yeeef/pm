# Developing pm

Guidance for an agent changing pm itself: its code, tests, hooks and site. `CLAUDE.md` is a symlink to this file. An
agent that only uses pm in a repo gets its context from `pm prime`, `pm show` and `pm <noun> --help`; nothing here is
for them, and nothing here ships (the release binary embeds only `assets.go`'s files). This repo's own sessions load
`prime.md` too, so its rules are not repeated here.

## Map

pm is one Go module; the pytest suite in `tests/` is its black-box harness, which drives the built binary. Each Go
package's doc comment holds its file map and detail (`go doc ./internal/<pkg>`); each test module's docstring says what
it covers.

| Path | Holds |
|---|---|
| `cmd/pm`, `assets.go` | The entry point (the launcher runs first); the embedded files, the release notes among them |
| `prime.md`, `prompts/`, `style.css` | What `pm prime` prints; the model prompts and hook texts; the site's one stylesheet |
| `internal/cli` | Every command; `commands.go` holds the command tree |
| `internal/hooks`, `internal/ask` | What the runtimes' hooks run (`pm prime`, `pm hook …`); one prompt through `claude -p` or `codex exec` |
| `internal/work`, `internal/item` | The work store on embedded Dolt, its host (only `pm service run` starts one), client, sync and merge; the item model, which links no Dolt (work aliases it) |
| `internal/records`, `internal/store`, `internal/design` | Record parsing and checks; the records store; design pages' hold on the code they cover |
| `internal/site`, `internal/service` | The pages; the pm service, its units and its access check |
| `internal/install`, `internal/launch` | `pm init`, `doctor`, `upgrade`, `uninstall`; the launcher that runs a repo's pinned release |
| `internal/session`, `internal/sync` | Which session runs pm and which are live; the push steps |
| `internal/config`, `buildinfo`, `proc`, `textwrap` | `.pm/config.toml`; the build's version; subprocesses; Python's `textwrap.fill` |
| `tests/` | The harness: `conftest.py` (fixtures, seeds, the integration guard), the fakes, the live evals |
| `release/`, `install.sh`, `CHANGELOG.md`, `changelog.d/` | The release build and tools (`release/README.md`); released notes; unreleased entry files |
| `.github/workflows/`, `.github/scripts/` | CI on every PR and push to main, the release, the website deploy; their helpers |
| `website/` | pm's public website, one static page; not shipped |

pm tracks its own development with the release `.pm/config.toml` pins: its projects' sprints, tasks and needs in the
work store (the remote's `refs/pm/work`), its records (plans, decisions, design pages) on the `records` branch.
A new file that `design_sources` (`.pm/config.toml`) matches goes into the `covers:` of its area's design page in
the sprint that adds it, before its PR merges (`go doc ./internal/design` says how `pm check` and `pm sprint close`
hold it).

## Commands

| Command | Runs |
|---|---|
| `make go-build` | pm built as released into `.go/pm`, with the harness's other builds |
| `make test` | The light harness set, while working |
| `make test-full ARGS="-k serve"` | The harness tests `-k` selects, integration ones included |
| `uv run pytest -q -n auto tests/test_hooks.py` | One file, or `-k name` for one test, after `make go-build` |
| `make test-go` | The Go checks: gofmt, vet, the tests, the race tests; `make go-test GO_PKGS=./internal/service` runs one package. Go commands need `-tags gms_pure_go` |
| `make lint` | The linters as CI runs them; needs golangci-lint at `GOLANGCI_LINT_VERSION` on PATH |
| `make test-live` | The owner-request judge's live eval (below) |
| `make test-live-parity` | The parity live eval (below) |
| `make agents-md-check`, `make test-live-agents-md` | This file graded against `tests/agents_md/rubric.md` (CI runs it on every PR); that grader's labelled cases |
| `make merge-ready PR=<n>` | The check before every merge (Merging) |

## Tests

- Run `make test` while working. When a change touches what an integration test covers (the service and its site,
  `init`, `push`, the session-start hook), run just those tests with `make test-full ARGS="-k …"`.
- **Never run the whole integration set locally.** `.github/workflows/pm-tests.yml` runs it on every PR, and that run
  is the check: push, then watch it (`gh pr checks <n> --watch`); to reproduce a CI failure, run only the failing tests
  with `make test-full ARGS="-k …"`. The PR's CI must be green before `pm action need --pr`.
- `.github/scripts/go_changes.py` decides which PRs skip the Go jobs' work and which Go test packages a PR runs:
  read its docstring before a Go file reads a repo file outside its package, or a test reads Go source.
- **Mark a test `integration`** when it starts the pm service, renders the whole site (`Repo.pages`), sets a clone up
  (`pm init`, `upgrade`, `uninstall`, `doctor`), reaches a git remote (`clone`, `fetch`, `pull`, `push`, a bare repo),
  runs `pm push`, or runs the session-start hook (`pm prime` without `--rules` or `--subagent`). The autouse fixture
  `light_unless_integration` in `conftest.py` fails an unmarked test that starts one of these.
- **A test that pins another version and does not test the launch sets `PM_LAUNCHED=<pin>`**, as a launched pm has
  it; without it the launcher downloads that release from GitHub.
- A test seeds and reads work data only through the `repo` helpers that `conftest.py`'s docstring names.
- **Golden files** (`internal/site`, `internal/install`, and every `--help` text in `internal/cli`): after an intended
  change, rewrite them with `go test -tags gms_pure_go ./internal/<pkg> -run Golden -update` and review the diff in the
  PR; never hand-edit one to make a test pass.
- **After adding or renaming an `internal/work` test, run `make go-test-work UPDATE=1`**: it measures and rewrites
  `.github/scripts/work-test-seconds.txt`, which must name every test and no other, or `go-test-work` fails.
- **After adding or renaming a Go test, classify it `os` or `any` in `.github/scripts/go-os-tests.txt`**, whose header
  says how; `make go-os-check` fails until you do.
- A function or file past a cap (`.golangci.yml`, `size_test.go`) is split along a seam, never kept with `//nolint`.

## Merging

The merge waits on the owner (`prime.md`). Whoever merges, the owner or an agent the owner asked to, merges a PR only
through `make merge-ready PR=<n>`, run right before the merge, then the `gh pr merge` command it prints: each PR's CI
ran against main as main was then, so two PRs that pass alone can break main together. A refused PR is rebased onto
`origin/main`, pushed, and merged once its new CI passes. `release/merge_ready.py`'s docstring has the conditions.

## Changelog and releases

- **A PR that changes what a release ships** (the `SHIPPED` paths in `release/changelog.py`) **adds one entry file,
  `changelog.d/<slug>.md`**, in the form `CHANGELOG.md`'s preamble gives, and never edits `CHANGELOG.md`. When users
  would not notice the change, label the PR `no-changelog` instead. `pm-changelog.yml` checks both on every PR.
- `release/README.md` holds the release workflow, the version rule, release candidates, notes fixes and a failed
  release's recovery.
- **Never move or recreate a release tag, and never rebuild a release's assets**: launchers keep each binary's sha256
  and fail hard when a later download differs. A failed publish is finished by re-running its failed jobs.
- Moving a repo's pin, this one's included, is a separate ordinary PR (`pm upgrade --to X`).

## prime.md, --help and the hooks

- `prime.md` carries only what a user's agents need. Guidance for developing pm, `[TEST]` needs and this repo's checks
  go here, never in `prime.md` or a `--help` text.
- A new command's rules go in its `--help`; `prime.md` names it only where a procedure runs it.
- Keep `prime.md` and `--help` in step with the code: a refusal `prime.md` names must exist in `internal/cli` with that
  wording, and a flag named in either must parse. `test_pm.py` asserts refusal texts: grep it before rewording one.
- A body shown in pm's docs, `--help` texts and hints takes one of the two forms `prime.md`'s Bodies paragraph gives,
  never `--text "…"`.
- The rules run as one hook per chunk under a size cap; `tests/test_hooks.py`'s docstring says how to re-cut them
  when a chunk outgrows it. Two PRs that each fit the cap can overflow it together.
- **A hook change edits both `claudeHooks` and `codexHooks`** (`internal/install/pieces.go`);
  `TestClaudeAndCodexRunTheSameHooks` fails on a difference `runtimeDifferences` gives no reason for.
- Hooks fail open on their own errors, the owner-request judge's exit 1 included (`internal/hooks`' package doc).
- **Read a session's transcript (Claude Code's JSONL, Codex's rollout) only when nothing else gives the fact**: first
  look for a hook event that fires on it, a field of a hook's input, or a runtime command or API, and check in a live
  session that it fires. A transcript is an undocumented format read by polling; an event is neither. Say in the code
  why no event serves where a transcript is read.

## pm init

`pm init` is the one install command; keep it that way rather than adding a setup command. Every session start runs
it (`pm init --session-start`): never make that run restart a running service or take a session's PATH, since each
runtime runs pm with a PATH of its own and the service would restart under every session (the parity eval fails on
it); leave a stale service to a typed `pm init` or `pm service restart`.

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

`tests/test_agent_parity_live.py`'s docstring lists the scenarios and what they assert from; `tests/test_ci.py` and
`tests/test_login_copies.py` hold a new scenario to the harness's counts and login copies.

## The site

- Who may reach the service is `refusal` in `internal/service/access.go`; the `site-replies` design page, Security,
  is the rule. Change the page and the code together.
- A record that does not validate shows as the error instead of its page: fix the record, not the renderer.
- The pages load Mermaid at one exact version (`MermaidVersion` in `internal/site/site.go`); bump it there only.

## Live checks

A change to setup, the hooks, the site or replies gets a live check besides its tests, in the form the sprint's
"Done when" names (`pm show --sprint ID`).

- **A need raised during a test or a live check starts its title with `[TEST]`**; once the check is done, close it with
  `pm need dismiss <id> --reason "[TEST] done"`.
- **A check of `pm init`, `pm push` or the service runs in a scratch clone of a scratch origin** under a temp directory
  with `HOME` and `CODEX_HOME` there, as the fixtures do, never on a real clone's store or service.
- The session-start path is checked in a scratch worktree: `git worktree add --no-checkout` then `git reset --hard`
  (Claude Code's own sequence), or a session started with `claude -w <name>`. Remove the worktree and its branch
  afterwards (`git worktree remove`, `git branch -D`).
- The live check's command, output and numbers go in the sprint's Findings and its delivery report.
