---
type: doc
title: pm code and test metrics
date: 2026-10-12
project: pm-quality
---

# pm code and test metrics

The reference for how healthy pm's code is: size, complexity, lint, tests, CI and the trend across the pm-quality sprints, each as a table, a one-line reading and a figure where it adds something.

- **Commit measured:** `fe05966` on origin/main, "RACE_TESTS names only top-level tests, and go_shards.py holds it (#53)", committed 2026-10-12 19:31 UTC.
- **Measured:** 2026-10-12, 19:40–19:48 UTC, in a detached worktree of that commit. Linux, 32 cores, 62 GB, go1.26.9 (the toolchain `go.mod` names), golangci-lint 2.14.0. Other sessions ran on the same machine: the load average was 2.8 to 14.6 at the start of each timed run.
- **Generator:** [`pm-quality-metrics.py`](pm-quality-metrics.py), beside this doc: one PEP 723 script that measures and draws every table and figure here. Method gives the commands.
- **Code lines:** a line that holds a Go token other than a comment (Go's own scanner); blank and comment-only lines are left out. For Python, a line that holds a token other than a comment, docstrings included.

## Headline

| Measure | Value |
|---|---|
| Go code lines, non-test / test | 22,532 / 8,007 (0.36 test lines per non-test line) |
| Python harness (`tests/`) code lines | 4,347 in 19 files |
| Go functions, non-test | 1,012: median 11 code lines, p90 42, max 204; 17 over 100 |
| gocyclo > 30 / gocognit > 40 | 26 / 33 functions (17 / 19 of them non-test); 35 distinct functions over either |
| `make lint` | clean: exit 0 |
| Ratcheted backlog, whole tree | 481 findings: errcheck 364, gocognit 33, gocyclo 26, funlen 36, reassign 19, file length 3 |
| `//nolint` directives | 20 |
| Go tests | 89 top-level (86 pass, 3 skip), 103 subtests run; 0 benchmarks, 0 fuzz tests |
| pytest tests | 151: 114 light (63 of them the live judge eval, skipped without `PM_LIVE_TESTS`), 37 integration |
| Statement coverage, combined | 72.8% of 12,465 statements (Go tests alone 55.1%, light harness alone 44.4%) |
| Local runtime, `make test` / `make test-go` | 7.7 s / 70.5 s wall |
| CI wall per PR, push to last check | median 2.9 min over the last 5 merged PRs; 4.0 min over the last 30 |
| Runs that failed, then passed on a rerun | 7 of 457 pull_request runs since 2026-10-10 (1.5%); 7 of 123 `pm go` runs (5.7%) |

## 1. Size

![Code lines per package, non-test and test](pm-quality-metrics-size.svg)

::: result {title="Code lines per Go package, and the Python files"}
| Package | Files | Non-test lines | Test files | Test lines | Test / non-test |
|---|---|---|---|---|---|
| `internal/cli` | 23 | 8,054 | 5 | 820 | 0.10 |
| `internal/work` | 15 | 3,937 | 18 | 3,224 | 0.82 |
| `internal/service` | 7 | 2,442 | 5 | 1,693 | 0.69 |
| `internal/install` | 7 | 2,294 | 2 | 783 | 0.34 |
| `internal/site` | 3 | 1,540 | 3 | 375 | 0.24 |
| `internal/records` | 5 | 1,212 | 1 | 143 | 0.12 |
| `internal/hooks` | 3 | 899 | 2 | 145 | 0.16 |
| `internal/launch` | 3 | 516 | 2 | 455 | 0.88 |
| `internal/store` | 1 | 434 | 1 | 160 | 0.37 |
| `internal/sync` | 1 | 368 | 1 | 141 | 0.38 |
| `internal/pyjson` | 1 | 382 | 1 | 10 | 0.03 |
| `internal/config` | 2 | 184 | 0 | 0 | 0 |
| `internal/proc` | 1 | 106 | 0 | 0 | 0 |
| root package `pm` | 1 | 18 | 1 | 58 | 3.22 |
| `internal/work/worktest` | 1 | 73 | 0 | 0 | 0 |
| `tests/render-pages` | 1 | 57 | 0 | 0 | 0 |
| `cmd/pm` | 1 | 14 | 0 | 0 | 0 |
| `internal/buildinfo` | 1 | 2 | 0 | 0 | 0 |
| **Go total** | 77 | **22,532** | 42 | **8,007** | 0.36 |
| Python harness `tests/` | | | 19 | 4,347 | |
| Python tools `release/`, `.github/scripts/` | 2 | 428 | | | |
:::

`internal/cli` holds 36% of the non-test Go code and the least Go test code per line (0.10); its coverage comes mostly from the harness (section 4). The size test (`size_test.go`, cap 5,000 per package) counts raw lines: `internal/cli` has 8,920, exactly its ratchet allowance, against 8,054 code lines here.

::: result {title="Largest files, by code lines"}
| File | Code lines | All lines |
|---|---|---|
| `internal/cli/writes.go` | 1,480 | 1,612 |
| `internal/site/site.go` | 1,071 | 1,213 |
| `internal/work/dolt.go` | 1,032 | 1,225 |
| `internal/cli/needs.go` | 917 | 1,003 |
| `internal/cli/install.go` | 904 | 989 |
| `internal/install/pieces.go` | 898 | 1,071 |
| `internal/service/run.go` | 794 | 936 |
| `internal/service/run_test.go` | 713 | 766 |
| `internal/work/sync_test.go` | 710 | 825 |
| `internal/cli/commands.go` | 676 | 685 |
:::

The first three are the files over revive's file-length limit (1,000 lines without comments and blanks) in section 3.

::: result {title="Largest functions, by code lines"}
| Function | File:line | Code lines |
|---|---|---|
| `TestCreatePushTimeout` | `internal/work/sync_test.go:517` | 247 |
| `TestClaimIsACompareAndSet` | `internal/work/dolt_test.go:86` | 222 |
| `TestCheckRefusesEachImpossibleState` | `internal/work/check_test.go:14` | 220 |
| `fromIssue` | `internal/work/importbd.go:225` | 204 |
| `TestSyncTwoClonesConcurrentEditsMergePerTheTable` | `internal/work/sync_test.go:244` | 193 |
| `TestImportFailsHardOnWhatItCannotMap` | `internal/work/importbd_test.go:26` | 187 |
| `cmdUninstall` | `internal/cli/install.go:680` | 182 |
| `(*command).parse` | `internal/cli/cli.go:131` | 164 |
| `cmdSprintClose` | `internal/cli/writes.go:584` | 162 |
| `TestLoadNeverPassesARecordSetMissingAFileGitRewrote` | `internal/cli/serve_test.go:113` | 156 |
:::

Six of the ten are tests; the four non-test ones are also over a complexity threshold in section 2.

## 2. Complexity

![gocyclo and gocognit distributions over all Go functions](pm-quality-metrics-complexity.svg)

::: result {title="Functions per score bin: 1,281 Go functions, 1,018 of them non-test"}
| Score | gocyclo, all | gocyclo, non-test | gocognit, all | gocognit, non-test |
|---|---|---|---|---|
| 0 | - | - | 272 | 209 |
| 1 | 271 | 208 | 227 | 176 |
| 2 | 217 | 166 | 102 | 88 |
| 3–5 | 361 | 302 | 255 | 211 |
| 6–10 | 229 | 196 | 170 | 140 |
| 11–15 | 93 | 70 | 97 | 78 |
| 16–20 | 43 | 30 | 47 | 37 |
| 21–30 | 41 | 29 | 55 | 41 |
| 31–40 | 15 | 12 | 23 | 19 |
| 41–60 | 8 | 3 | 17 | 10 |
| 61+ | 3 | 2 | 16 | 9 |
| mean / median / p90 / p99 / max | 6.21 / 3 / 14 / 37 / 91 | | 7.14 / 3 / 18 / 65 / 130 | |
:::

The median function is simple (3 on both scales); the tail is long: 110 functions have gocyclo > 15, and 33 have gocognit > 40. The linters score the 1,275 declared functions and 6 package-level `var X = func…` values (test seams), hence 1,018 non-test against the 1,012 declared in section 1.

::: result {title="The 35 functions over a lint threshold (gocyclo > 30 or gocognit > 40), by gocognit"}
| Function | File:line | gocyclo | gocognit |
|---|---|---|---|
| `results` | `internal/install/golden_test.go:191` | 36 | 130 |
| `TestCreatePushTimeout` | `internal/work/sync_test.go:517` | 59 | 109 |
| `TestClaimIsACompareAndSet` | `internal/work/dolt_test.go:86` | 91 | 105 |
| `(*command).parse` | `internal/cli/cli.go:131` | 65 | 104 |
| `(*repo).dayActivity` | `internal/cli/day.go:152` | 36 | 97 |
| `fromIssue` | `internal/work/importbd.go:225` | 81 | 89 |
| `cmdUninstall` | `internal/cli/install.go:680` | 55 | 86 |
| `showDataOf` | `internal/cli/show.go:236` | 35 | 81 |
| `TestCheckRefusesEachImpossibleState` | `internal/work/check_test.go:14` | 41 | 81 |
| `RenderIndex` | `internal/site/site.go:980` | 36 | 72 |
| `showProjectText` | `internal/cli/show.go:556` | 26 | 70 |
| `TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall` | `internal/service/lifecycle_test.go:248` | 38 | 68 |
| `cmdSprintClose` | `internal/cli/writes.go:584` | 47 | 65 |
| `TestCommentsKeepTheirOrderWithinASecond` | `internal/work/dolt_test.go:322` | 45 | 65 |
| `TestSyncTwoClonesConcurrentEditsMergePerTheTable` | `internal/work/sync_test.go:244` | 52 | 65 |
| `liveUses` | `internal/cli/clean.go:315` | 32 | 62 |
| `TestHeaderValuesTypeAsPyyamlDoes` | `internal/records/records_test.go:41` | 29 | 60 |
| `TestLoadNeverPassesARecordSetMissingAFileGitRewrote` | `internal/cli/serve_test.go:113` | 30 | 53 |
| `LegacyRepo` | `internal/install/legacy.go:71` | 26 | 53 |
| `TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt` | `internal/work/access_test.go:19` | 26 | 53 |
| `cmdInit` | `internal/cli/install.go:224` | 33 | 51 |
| `initSteps` | `internal/cli/install.go:343` | 30 | 50 |
| `TestMergeOtherScalarsTheLaterUpdatedAt` | `internal/work/merge_test.go:49` | 47 | 50 |
| `TestAWriteRacingOneOutsideTheLockFailsHard` | `internal/work/concurrent_test.go:25` | 22 | 49 |
| `plan` | `internal/install/pieces.go:798` | 26 | 48 |
| `TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne` | `internal/launch/launch_test.go:182` | 31 | 48 |
| `RenderRecord` | `internal/site/site.go:806` | 31 | 47 |
| `checkItem` | `internal/work/check.go:52` | 35 | 46 |
| `mergeItem` | `internal/work/merge.go:97` | 44 | 46 |
| `cmdCommit` | `internal/cli/writes.go:1492` | 36 | 45 |
| `resolveConflicts` | `internal/work/sync.go:391` | 31 | 45 |
| `TestAnOperationOutlastingItsBoundFreesTheSlotAndAWaiterGivesUp` | `internal/work/opslot_test.go:39` | 24 | 43 |
| `cmdSprintOpen` | `internal/cli/writes.go:423` | 32 | 41 |
| `SetupClone` | `internal/install/clone.go:446` | 32 | 39 |
| `dispatch` | `internal/cli/cli.go:494` | 37 | 31 |
:::

14 of the 35 are tests; 12 of the 21 non-test ones are in `internal/cli`.

::: result {title="funlen findings (lines > 100 or statements > 60, comments ignored)"}
| Kind | Findings | Non-test | Test |
|---|---|---|---|
| statements > 60 | 24 | 21 | 3 |
| lines > 100 | 12 | 1 | 11 |
| **functions** | **36** | 22 | 14 |
:::

No function trips both funlen limits, so the 36 findings are 36 functions. Non-test code trips the statement limit (21) and test code the line limit (11); 17 of the 22 non-test functions are in `internal/cli`.

## 3. Lint state

`make lint` exits 0 at this commit: the gating golangci-lint linters, `deadcode` and `ruff` are clean. The ratcheted linters (`RATCHET` in the Makefile) gate only the lines a branch changes, so their backlog stands on the rest of the tree:

::: result {title="Ratcheted backlog: golangci-lint --enable-only errcheck,gocyclo,gocognit,funlen,revive,reassign ./..."}
| Linter | Findings | Non-test | Test | Files |
|---|---|---|---|---|
| errcheck | 364 | 179 | 185 | 71 |
| funlen (statements > 60) | 24 | 21 | 3 | 14 |
| funlen (lines > 100) | 12 | 1 | 11 | 10 |
| gocognit (> 40) | 33 | 19 | 14 | 25 |
| gocyclo (> 30) | 26 | 17 | 9 | 19 |
| reassign | 19 | 0 | 19 | 6 |
| revive file-length-limit (> 1,000) | 3 | 3 | 0 | 3 |
| **total** | **481** | 240 | 241 | |
:::

errcheck is 76% of the backlog: 268 unchecked error returns and 96 errors assigned to `_` (`check-blank`). By package: `internal/service` 82, `internal/work` 75, `internal/cli` 45, `internal/install` 44, `internal/launch` 29, the other eight 89. Every reassign finding is a test swapping `buildinfo.Version` (15) or `time.Local` (4).

::: result {title="//nolint directives, by linter"}
| Linter | Directives |
|---|---|
| nilerr | 8 |
| usetesting | 5 |
| forbidigo | 4 |
| staticcheck | 2 |
| gocritic | 1 |
| **total** | **20** |
:::

Each directive names one linter; nolintlint holds that each also gives a reason.

## 4. Tests

::: result {title="Test counts"}
| Suite | Tests | How counted |
|---|---|---|
| Go, top-level | 89 (86 pass, 3 skip) | `func TestX(t *testing.T)` in `_test.go` files; outcomes from `go test -json` |
| Go, subtests | 103 run, all pass | `go test -json` events with a `/` in the name; 96 `t.Run(name, func…)` call sites |
| Go benchmarks, fuzz tests | 0, 0 | |
| pytest, light (`-m "not integration"`) | 114: 51 pass, 63 skipped | the 63 are the live judge eval, which runs only with `PM_LIVE_TESTS=1` |
| pytest, integration | 37 | collected only: CI runs them; this doc did not |
| pytest, live (`make test-live`) | 63 | collected only: needs a logged-in `claude` |
:::

Go tests per package: `internal/work` 24, `internal/service` 23, `internal/cli` 12, `internal/install` 8, `hooks`, `launch`, `site` 4 each, `store`, `sync` 3 each, `records` 2, `pyjson` and the root package 1 each.

![Statement coverage per package](pm-quality-metrics-coverage.svg)

::: result {title="Statement coverage per package: Go tests, light harness, and the union"}
| Package | Statements | Go tests | Light harness | Combined |
|---|---|---|---|---|
| `internal/cli` | 4,483 | 16.0% | 54.7% | 59.5% |
| `internal/work` | 2,053 | 84.2% | 46.6% | 84.9% |
| `internal/service` | 1,399 | 81.3% | 16.7% | 81.8% |
| `internal/install` | 1,320 | 67.0% | 2.5% | 67.2% |
| `internal/site` | 842 | 92.5% | 73.8% | 93.0% |
| `internal/records` | 621 | 83.9% | 58.1% | 88.6% |
| `internal/hooks` | 522 | 48.5% | 59.2% | 74.7% |
| `internal/launch` | 282 | 77.0% | 54.6% | 78.0% |
| `internal/store` | 273 | 72.2% | 74.4% | 84.2% |
| `internal/sync` | 225 | 78.2% | 13.8% | 78.2% |
| `internal/pyjson` | 220 | 66.8% | 33.6% | 70.9% |
| `internal/config` | 90 | 64.4% | 62.2% | 70.0% |
| `internal/proc` | 49 | 77.6% | 67.3% | 77.6% |
| `internal/work/worktest` | 45 | 31.1% | 0.0% | 31.1% |
| `tests/render-pages` | 31 | 0.0% | 0.0% | 0.0% |
| `cmd/pm` | 9 | 0.0% | 100.0% | 100.0% |
| root package `pm` | 1 | 100.0% | 100.0% | 100.0% |
| **module** | **12,465** | 55.1% | 44.4% | **72.8%** |
:::

`internal/cli` is the largest gap: 1,817 of its 4,483 statements run in no measured test, and the Go tests alone reach 16%. `internal/install` (67%) is covered by Go tests only, since `pm init`, `upgrade` and `doctor` are integration tests, which were not run. `tests/render-pages` runs only in integration page tests. The audit of 2026-10-11 measured 73.74% the same way at `eedbafd` ([test value audit](2026-10-11-test-value-audit.md)); `cmd/pm`'s 9 statements include the coverage build's SIGTERM patch (Method).

::: result {title="Local runtimes, go build cache warm, Go tests uncached (GOFLAGS=-count=1)"}
| Command | Wall s | CPU s | Cores busy (CPU / wall) | Load average before | Result |
|---|---|---|---|---|---|
| `make test` | 7.7 | 60.9 | 7.9 | 10.2 | 51 passed, 63 skipped; pytest `-n auto` = 32 workers |
| `make test-go` | 70.5 | 94.7 | 1.3 | 11.1 | exit 0 |
| its parts, run one by one: | | | | | |
| `make go-build` | 0.6 | 3.1 | 5.5 | 14.6 | cached |
| `make go-fmt` | 0.2 | 0.7 | 3.9 | 14.6 | |
| `make go-vet` | 0.2 | 1.3 | 5.6 | 14.6 | vet results cached |
| `make go-test-but-work` | 8.0 | 25.1 | 3.1 | 14.6 | |
| `make go-test-work` (8 shards) | 27.7 | 25.3 | 0.9 | 13.2 | |
| `make go-test-race` | 30.3 | 26.7 | 0.9 | 10.3 | |
:::

`make test-go` keeps about one core of 32 busy: 58 of its 70.5 s are the `internal/work` tests and the race tests, which wait on Dolt and timers rather than compute. A warm `make test` is 7.7 s. An earlier run at load 2.8–4.9 gave 6.7 s and 81.5 s: the spread between runs on this shared machine is about 15%.

## 5. CI

Per merged PR: the first attempt of every workflow run that the push of the PR's merged head started (runs created within 120 s of the first), from the first run's creation to the last job's completion. Jobs from the GitHub jobs API; skipped jobs left out.

![CI wall time per PR](pm-quality-metrics-ci.svg)

::: result {title="CI per PR, the last 30 merged PRs in merge order (UTC)"}
| PR | Merged | Wall min | Jobs | Slowest job (queue + run) | Its queue s | Its run s |
|---|---|---|---|---|---|---|
| #20 | 10-10 14:03 | 10.1 | 5 | pm go / build-and-test (darwin-arm64, macos-14) | 141 | 447 |
| #23 | 10-10 14:35 | 18.3 | 5 | pm go / build-and-test (darwin-arm64, macos-14) | 274 | 801 |
| #22 | 10-10 15:03 | 12.3 | 9 | pm go / build-vet-test (darwin-arm64) | 344 | 367 |
| #21 | 10-10 15:18 | 13.2 | 9 | pm go / build-vet-test (darwin-arm64) | 431 | 361 |
| #27 | 10-10 15:26 | 4.0 | 10 | pm go / work (darwin-arm64) | 33 | 204 |
| #25 | 10-10 15:28 | 4.4 | 9 | pm go / work (darwin-arm64) | 54 | 208 |
| #24 | 10-10 16:33 | 3.0 | 9 | pm go / work (darwin-arm64) | 5 | 176 |
| #28 | 10-10 16:41 | 3.2 | 9 | pm go / work (darwin-arm64) | 8 | 183 |
| #29 | 10-10 16:50 | 4.4 | 10 | pm go / work (darwin-arm64) | 10 | 255 |
| #30 | 10-10 17:00 | 4.7 | 9 | pm go / build-vet-test (darwin-arm64) | 89 | 172 |
| #32 | 10-10 17:21 | 6.4 | 11 | pm go / work (darwin-arm64) | 170 | 208 |
| #31 | 10-10 17:30 | 4.5 | 11 | pm release build / release build (darwin-arm64) | 8 | 261 |
| #33 | 10-10 17:43 | 3.5 | 9 | pm go / work (darwin-arm64) | 7 | 199 |
| #34 | 10-10 17:53 | 3.3 | 9 | pm go / work (darwin-arm64) | 8 | 192 |
| #35 | 10-10 18:01 | 3.1 | 10 | pm go / work (darwin-arm64) | 6 | 178 |
| #36 | 10-10 18:27 | 3.2 | 9 | pm go / race (darwin-arm64) | 9 | 181 |
| #37 | 10-10 18:51 | 3.7 | 9 | pm go / work (darwin-arm64) | 9 | 213 |
| #38 | 10-10 19:19 | 3.1 | 10 | pm go / work (darwin-arm64) | 7 | 180 |
| #42 | 10-11 18:47 | 4.8 | 9 | pm go / work (darwin-arm64) | 10 | 275 |
| #40 | 10-11 18:49 | 3.9 | 9 | pm go / work (darwin-arm64) | 8 | 224 |
| #46 | 10-11 19:20 | 8.8 | 9 | pm go / work (darwin-arm64) | 328 | 198 |
| #45 | 10-11 19:30 | 7.5 | 9 | pm go / build-vet-test (darwin-arm64) | 309 | 140 |
| #48 | 10-11 19:51 | 4.0 | 11 | pm release build / release build (darwin-arm64) | 8 | 235 |
| #43 | 10-11 20:01 | 4.1 | 14 | pm release build / release build (darwin-arm64) | 8 | 239 |
| #44 | 10-11 20:13 | 3.9 | 14 | pm go / build-vet-test (darwin-arm64) | 10 | 223 |
| #49 | 10-11 20:20 | 4.7 | 12 | pm go / build-vet-test (darwin-arm64) | 64 | 218 |
| #50 | 10-11 20:25 | 3.7 | 12 | pm go / race (darwin-arm64) | 103 | 120 |
| #51 | 10-11 20:32 | 2.3 | 12 | pm go / build-vet-test (darwin-arm64) | 9 | 126 |
| #52 | 10-12 18:46 | 2.2 | 13 | pm go / work (darwin-arm64) | 7 | 124 |
| #53 | 10-12 19:31 | 2.9 | 12 | pm go / work (darwin-arm64) | 34 | 139 |
:::

The macOS job is the slowest job on every PR. Wall above about 6 min is macOS runner queue (#21, #22, #45, #46: 309–431 s queued) or the one-job layout before #24 (#20, #23). After #44 (sprint 18) the median is 2.9 min over 5 PRs, against 3.9 min over the 18 PRs from #27 to #45.

::: result {title="Per job: queue over all 30 PRs, run median before and after the merge of #44"}
| Workflow / job | Jobs | Queue median s | Queue p90 s | Run median s, #27–#44 | Run median s, #49–#53 | Run max s |
|---|---|---|---|---|---|---|
| pm changelog / changelog | 35 | 3 | 3 | 5 | 5 | 7 |
| pm tests / integration | 30 | 3 | 3 | 81 | 77 | 94 |
| pm tests / light | 30 | 3 | 3 | 71 | 59 | 87 |
| pm go / build-vet-test (darwin-arm64) | 28 | 9 | 309 | 129 | 126 | 367 |
| pm go / build-vet-test (linux-amd64) | 28 | 3 | 4 | 67 | 61 | 388 |
| pm go / race (darwin-arm64) | 28 | 9 | 207 | 144 | 120 | 346 |
| pm go / race (linux-amd64) | 28 | 3 | 4 | 83 | 54 | 164 |
| pm go / work (darwin-arm64) | 28 | 9 | 170 | 199 | 124 | 303 |
| pm go / work (linux-amd64) | 28 | 3 | 4 | 129 | 67 | 250 |
| pm lint / lint-deadcode | 7 | 3 | 3 | 77 | 84 | 95 |
| pm lint / lint-go | 7 | 3 | 3 | 51 | 41 | 54 |
| pm lint / lint-py | 7 | 3 | 3 | 6 | 7 | 9 |
| pm release build / release build (darwin-arm64) | 5 | 8 | 21 | 236 | - | 261 |
| pm release build / release build (linux-amd64) | 5 | 2 | 3 | 221 | - | 226 |
| pm go / build-and-test (darwin-arm64, macos-14) | 2 | 207 | 274 | - | - | 801 |
| pm go / build-and-test (linux-amd64, ubuntu-22.04) | 2 | 3 | 3 | - | - | 574 |
:::

Sprint 18 cut `work (darwin-arm64)` from 199 s to 124 s and `work (linux-amd64)` from 129 s to 67 s (medians); queue is 3 s on Linux at p90 but 170–309 s on macOS, so the macOS queue, not run time, sets the worst PRs. `pm lint` runs from #43 on; release build runs only when a PR touches the release paths. Changelog has more jobs than PRs: an edit or a label starts it again within 120 s.

::: result {title="Reruns: every pull_request run created since 2026-10-10 (457 runs)"}
| Workflow | Runs | Re-run | Failed, then passed on a rerun | Failing job in the first attempt |
|---|---|---|---|---|
| pm go | 123 | 11 | 7 (5.7%) | race (linux-amd64) 2, build-and-test 3 (old layout), work (linux-amd64) 1, race (darwin-arm64) 1 |
| pm tests | 123 | 3 | 0 | |
| pm changelog | 135 | 2 | 0 | |
| pm release build | 28 | 2 | 0 | |
| pm lint | 19 | 1 | 0 | |
| Records guard | 29 | 0 | 0 | |
| **all** | **457** | **19** | **7 (1.5%)** | |
:::

Every failed-then-passed rerun is in `pm go`, 4 of 7 in the race or work jobs (the Dolt concurrency tests). One more `pm go` run failed and its rerun was cancelled. The 11 reruns that were success then success were deliberate, to time CI. A flaky failure followed by a new push, rather than a rerun, is not counted, so 1.5% is a lower bound.

## 6. Trend across the pm-quality sprints

At the merge commit of each pm-quality sprint PR (`git log --grep "pm-quality sprint"`, the 12 commits that name a PR), plus the measured commit, in merge order. Go tests counted statically (top-level `TestX` functions); gocyclo > 30 and staticcheck with this commit's settings on every commit, tests included.

![Metrics at each pm-quality sprint merge](pm-quality-metrics-trend.svg)

::: result {title="At each sprint PR's merge commit"}
| Sprint | PR | Commit | Merged | Non-test Go lines | Go test lines | Harness lines | Go tests | gocyclo > 30 | staticcheck |
|---|---|---|---|---|---|---|---|---|---|
| 5 | #18 | 963fb75 | 2026-10-10 | 20,772 | 7,682 | 3,606 | 205 | 16 | 7 |
| 10 | #25 | 2911a04 | 2026-10-10 | 21,360 | 7,968 | 3,899 | 213 | 17 | 7 |
| 7 | #30 | cee2d7d | 2026-10-10 | 21,866 | 8,255 | 4,037 | 221 | 18 | 7 |
| 12 | #32 | af0a631 | 2026-10-10 | 21,866 | 8,255 | 4,273 | 221 | 18 | 7 |
| 13 | #31 | da3423c | 2026-10-10 | 21,956 | 8,471 | 4,395 | 225 | 18 | 7 |
| 9 | #33 | b61f12d | 2026-10-10 | 22,213 | 8,497 | 4,512 | 226 | 19 | 7 |
| 15 | #36 | 41b6696 | 2026-10-10 | 22,245 | 8,567 | 4,512 | 227 | 19 | 7 |
| 16 | #37 | 6752e93 | 2026-10-10 | 22,320 | 8,610 | 4,585 | 228 | 19 | 7 |
| 17 | #38 | eedbafd | 2026-10-10 | 22,320 | 8,610 | 4,585 | 228 | 19 | 7 |
| 20 | #48 | d98406d | 2026-10-11 | 22,550 | 7,900 | 4,205 | 88 | 25 | 7 |
| 19 | #43 | 09f62a7 | 2026-10-11 | 22,529 | 7,993 | 4,272 | 89 | 26 | 0 |
| 18 | #44 | 5d1e05b | 2026-10-11 | 22,529 | 8,004 | 4,336 | 89 | 26 | 0 |
| (main) | #53 | fe05966 | 2026-10-12 | 22,532 | 8,007 | 4,347 | 89 | 26 | 0 |
:::

- Non-test code grew 8.5% (20,772 to 22,532 lines), 88% of that before sprint 20.
- Sprint 20 (#48) folded 228 top-level Go tests into 88 and cut 710 Go test lines and 380 harness lines; in the same PR gocyclo > 30 rose from 19 to 25 functions.
- Sprint 19 (#43) took staticcheck from 7 findings to 0 and made it gate every line.
- gocyclo > 30 rose from 16 to 26 over the project: the ratchet stops new findings on changed lines, and nothing has paid the old ones down yet.

## Method

Every number above comes from the generator, run in a detached worktree of the commit measured. The machine needs `go`, `uv`, `git`, `gh` (logged in) and golangci-lint 2.14.0 on PATH.

```sh
git worktree add --detach .claude/worktrees/metrics origin/main
S=$(pm where records)/docs/pm-quality-metrics.py
uv run "$S" measure .claude/worktrees/metrics /tmp/pm-metrics size complexity lint tests runtime coverage ci trend
uv run "$S" draw /tmp/pm-metrics /tmp/pm-metrics/fig > /tmp/pm-metrics/tables.md
git worktree remove .claude/worktrees/metrics
```

| Step | What it runs |
|---|---|
| size | a Go program on `go/scanner` and `go/parser` over every `.go` file outside dot dirs, `testdata` and `vendor`; Python's `tokenize` on `tests/`, `release/`, `.github/scripts/` |
| complexity | golangci-lint with gocyclo and gocognit at threshold 0, so it reports every function (gocognit omits a score of 0); `-tags gms_pure_go`, tests included |
| lint | `make lint`; `golangci-lint run --enable-only errcheck,gocyclo,gocognit,funlen,revive,reassign ./...` with the repo's `.golangci.yml`, without `--new-from-merge-base`; `//nolint:` directives found by regexp |
| tests | `pytest --collect-only` for all, `-m "not integration"`, `-m integration` and the live file |
| runtime | `make test`, then `GOFLAGS=-count=1 make test-go` and each of its prerequisites; CPU from `getrusage(RUSAGE_CHILDREN)` |
| coverage | as in the [test value audit](2026-10-11-test-value-audit.md): `go test -cover -coverpkg=<module>/... ./...`, and the light harness at `-n 32` on `pm`, `pm-before` and `render-pages` built with `-cover -coverpkg=<module>/...` and a SIGTERM handler that calls `os.Exit` (patched into `cmd/pm/main.go` for the build only, then restored), each test in its own `GOCOVERDIR`; the union takes a statement block as covered when either profile reached it |
| ci | `gh pr list --state merged --limit 30`; `actions/runs?head_sha=<head>&event=pull_request` and each run's attempt-1 jobs; every `pull_request` run since the oldest PR's creation for reruns |
| trend | one `git worktree add --detach` per commit, all 13 at once (`GOMAXPROCS=2` each on 32 cores), size and golangci-lint with staticcheck and gocyclo > 30, then the worktrees removed |

## Not measured

- The integration set (37 tests): neither its coverage nor its local runtime. CLAUDE.md keeps the whole set to CI. So `internal/install`, `tests/render-pages` and the service's lifecycle code read lower here than their tested state.
- The live judge eval (63 tests): needs a logged-in `claude` and model calls.
- Coverage of `release/*.py`, `install.sh` and the Makefile.
- Coverage and runtime at the trend's older commits: the trend gives size, test count, gocyclo and staticcheck only.
- CI cost in runner minutes, and macOS queue time outside these 30 PRs.
- A flaky failure that was pushed over rather than re-run.
