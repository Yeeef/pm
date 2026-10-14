---
type: sprint
title: "The Go tests do less work: macOS CI compute halves"
bead: pm-d2k5.31
---

## Goal

> What should be true when this sprint ends, and why now?

The free macOS runner's 3 cores spend about 96–98 s per PR on Go work, plus 25–35 s of checkout and cache restore, so a code PR takes a median 138 s (sprint 27). Rearranging jobs no longer helps; the work itself must shrink: what is compiled, linked and set up per test. Target: macOS Go compute about 50 s, and a code PR about 100 s.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Profile macOS work per package: compile, link and test run time, plus per-test CPU from `go test -json`, both on CI and locally. The question is where the 96–98 s goes.
- Cut link work: count how many test binaries link Dolt and fold or restructure them where that cuts links, for example by keeping store-free packages from importing it.
- Make fixtures cheaper: a store or clone template built once per package and copied per test, instead of created per test, and less git subprocess work per test.
- Remove waits: sleeps and backoffs that set test time, without weakening what a test checks.
- The same changes speed up `make test-go` locally.

**Out:**
- Running OS-independent tests on Linux only. That changes the owner's "macOS on every PR" decision; if profiling shows it is the main lever, raise it as a decision need with the numbers.
- Paid or self-hosted runners.
- Dropping any test or assertion.

## Done when

> What evidence will show the goal is met?

- macOS Go compute (the build and test steps of the two macOS jobs, summed) is ≤ ~55 s, from 96–98 s, the median of 3 PRs.
- A code PR's median wall time to the last check is ≤ ~100 s over 3 PRs, from 138 s.
- The per-OS lists of tests run are identical before and after.
- `make test-go` local wall time is reported before and after.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-14}
On macOS the 9 RACE_TESTS run only under -race; Linux keeps both runs
They are 47% of internal/work macOS CPU, about 20 s per PR; a non-race-only macOS bug is unlikely and Linux still runs them both ways
Answers `pm-d2k5.31.4`.
:::

::: decision {source=agent date=2026-10-14}
Running OS-independent tests on Linux only moves from Out to In: a PR runs on macOS the darwin build plus the OS-dependent tests; every push to main runs the full macOS suite.
The pm-quality project decision of 2026-10-14 (owner) revises macOS on every PR: most tests generalize across OSes, and a post-merge full macOS run catches what the subset misses.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Profile, macos-14 (3 cores, Apple M1 virtual), PR 81's first head; local
  runs on 32 cores under taskset -c 0-2. Baseline: the two macOS jobs' build
  and test steps summed were 148-181 s over 13 recent runs (build-test 89-113
  s, race 51-80 s); the frame's 96-98 s is the build-test step alone.
  Build-test's 113 s: the pm build beside every package's tests but
  internal/work's 55 s, work's test binary 8 s, work's 8 shards 50 s (101.6 s
  CPU: 49.6 user, 52.0 sys). Compiles come cached from main's cache;
  recompiling our own packages costs 22.6 s of compile in 50 actions. 10
  binaries per build-test run link Dolt: pm, work.test and 8 test binaries
  (cli, config, design, install, records, service, site, store). A Dolt link
  takes 4.8-7.7 s wall and 7-9 s CPU on macOS, 1.8-2.3 s locally. records,
  store, design and site link Dolt only through work's item types; config only
  through its test's import of install. Per test, each run alone: 158.7 s CPU
  on macOS against 34.2 s locally; internal/work is 125.4 s of it (61.5 user,
  63.8 sys), with 7599 git subprocesses (8148 locally). A git call costs 4.9
  ms wall and 4.5 ms CPU on macOS (homebrew git; /usr/bin/git 13 ms) against
  0.7 ms locally; the parent's heap does not change it (0, 256 or 1024 MB).
  Top tests by macOS CPU, s (git calls): TestCreateRacingALocalWriter 30.9
  (1022), TestPullRacingLocalWriters 17.2 (770), TestCreatePushTimeout 11.8
  (972), TestSyncTwoClonesMergeListFieldsPerEntry 11.6 (825),
  ...CloseBeatsClaim... 9.8 (782), ...MoveSprint... 9.1 (731),
  ...ConcurrentEdits... 6.9 (527), cli
  TestStampAndTextsSurviveGitRewritingTheRecordsMidWalk 6.8 (50),
  TestSyncFailsHardOnAnUnsettledConflict... 6.2 (560),
  TestAnOperationOutlasting... 5.7 (332), TestSyncFailsHardOnAddAddAndACycle
  5.7 (517), cli TestLoadNeverPasses... 5.2 (155),
  TestTwoClonesMigratingOnTheirOwnMerge 3.4 (201),
  TestCreateRefusesWithTheRemoteUnreachable 2.8 (244), install
  TestSetupWorkCreatesPushesAndClones 2.0 (80), TestSetupWorkRefusesAFork 1.8
  (72), site TestCheckRefusesEachBrokenRecord 1.6 (96), cli TestSyncCommand
  1.4 (58), TestCommentsKeepTheirOrderWithinASecond 1.3 (109),
  TestAWriteRacingOneOutsideTheLockFailsHard 1.2 (0). The racing tests loop
  their writers for as long as the raced operation takes, so their CPU grows
  with the machine's slowness (10x on macOS). The two-clone fixture newPair is
  241 git calls (a create through the remote about 50, a pushing sync 52, a
  clone 20), and 10 tests build one. The 9 RACE_TESTS are 58.9 s of work's
  macOS CPU and run again in the race job. Dolt's 1.1 s read-dedup waits and
  the other sleeps are wall, not CPU: with the 3 cores saturated they do not
  set macOS time.

- Levers ranked by measured saving on macos-14 (warm cache, each phase alone,
  PR 81 before and after; before/after CPU in s): (1) the 9 RACE_TESTS run
  twice on macOS: 58.9 s of internal/work's 125.4 s CPU; out of scope, raised
  as a decision need. (2) 4 fewer Dolt links: the item model moved to
  internal/item, which links no Dolt, and records, store, design and site
  import it; their test binaries stop linking Dolt (5 test binaries still
  link it: cli, config, install, service and work, beside pm). Every package's
  tests but internal/work's: 31.0 s wall, 63.3 CPU before; 26.2 s, 48.0 CPU
  after. TestTheItemReadersLinkNoDolt holds it. (3) newPair seeds its 5 items
  locally and pushes them in one sync, as reseed does: 241 to about 70 git
  calls per pair, 10 tests. internal/work's git calls 7599 to 5803 on macOS
  (8148 to 6446 locally); its 8 shards 50.0 s wall, 101.6 CPU before; 37.6 s,
  74.1 CPU after. The race shards 38.1 to 35.5 s. Rejected: test binaries
  linked with -linkmode=internal or -s save 1-2 s CPU of a 7-9 s Dolt link;
  moving config's test to install would drop one more link but needs
  config.Keys exported for a test; a git call's cost does not depend on the
  parent's heap. Locally (32 cores), make test-go ran its parts one after
  another: 35.5-35.8 s at main, 33.9-34.8 s with (2) and (3), 19.6-19.8 s with
  its parts run at once (3 runs each).

- No test lost: go test -v at origin/main (e3ebec5) and at the PR's head, both
  in one CI run per job (run 38530785456). macOS: every package but
  internal/work's 183 results at main (122 top-level, 3 skip), 184 at head
  (123 top-level); internal/work 92 results (32 top-level, 3 skip) both; race
  service 39 (31 top-level, 1 skip) and race work 16 (9 top-level) both.
  Linux: 183 against 184 (2 skip), 92 and 92, 39 and 39, 16 and 16. The sorted
  (package, test, result) lists are identical but for the added
  TestTheItemReadersLinkNoDolt, on both OSes.

- Done-when samples, from the jobs API (macOS = the build-test and race steps
  summed; wall = first job created to last check done). #81 at 280dfb8: 178 +
  70 = 248 s, wall 249 s (last check the agents-md cases job, run because
  AGENTS.md changed; every macOS test ran 1.3-1.6x slower than in other runs
  that hour). #82 at 7595b37: 108 + 58 = 166 s, wall 164 s; it restored main's
  cache from before #81, so it compiled #81's packages. #82's Go jobs rerun on
  main's new cache: 102 + 58 = 160 s, pm-go wall 145 s. Before: 148-181 s over
  13 runs, median 163 s. So macOS Go compute did not measurably fall in CI,
  against 51-55 s targeted. In those suites internal/work's shards took 37.4
  and 41.5 s against 49.7 s before, as the profile predicted. Every package's
  tests but internal/work's took 51-60 s, though, against 26 s when the same
  command ran again right after. The first go command after the cache restore
  pays for reading the restored packages, so the link work cut there is not
  what sets that phase. Two PRs and one rerun were sampled, not three PRs: no
  other session opened a PR, and the one candidate follow-up (moving config's
  test to install) was dropped.

- Classification (table below; .github/scripts/go-os-tests.txt on main holds
  it with the same reasons): of 155 top-level Go tests, 85 can behave
  differently by OS and 70 cannot. Evidence for os: process groups (Setpgid,
  kill(-pgid)); flock; unix sockets and the 104-byte sun_path limit; symlinks
  and realpath (EvalSymlinks, os.Root); file modes and Chmod 0; service units
  (PlatformKind, launchctl, systemctl); XDG data dirs; /proc against ps; exec
  of downloaded binaries; and the embedded Dolt engine writing a store on
  disk. Every package is all os (ask, store), all any (root, cli/parse,
  config, design, hooks, item) or mixed per test (cli, install, launch,
  records, service, session, site, sync, work). Checked: the test set read
  from source equals go test -list's (155 = 155). Each mode's selection equals
  its intended set, run locally with -v: Linux and main-push macOS run all
  155; macOS runs internal/work's 9 RACE_TESTS only under -race; a PR's macOS
  jobs run exactly the 85 os tests (59 outside internal/work, 19 in it, and 7
  race tests under -race; internal/service's os tests also run under -race).
  Main's push run on #83 (ad5659f) ran the full macOS suite and passed: every
  package, 23 internal/work tests, and the 9 race tests plus internal/service
  under -race (build-test 81 s, race 49 s).

- Done-when samples on the new layout. Code PRs whose CI ran with #83 (macOS =
  the build-test and race steps summed; wall = first job created to last check
  done): #83 63 + 67 = 130 s, wall 269 s (the agents-md cases job was last,
  4m25s, run because AGENTS.md changed); #84 63 + 59 = 122 s, wall 109 s; #69
  (another session's) 70 + 62 = 132 s, wall 116 s. Medians: macOS 130 s
  against 163 s before (-20%), wall 116 s against 138 s. Targets ≤55 s and
  ≤100 s not met. The race job is now as long as build-test: it links the race
  build of internal/work (about 30 s) and runs 7 Dolt racing tests under -race
  (19-28 s). #84 also cut TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers
  from 3.0 to 0.3 s by waiting out 0.1 s probes.

::: result {title="Go tests by OS dependence"}
| Package | Test | Class | Evidence |
|---|---|---|---|
| . | all | any | assets_test.go:TestAssetsBuildWithNoEntryFiles runs `go build` on copied embed files and size_test.go:TestPackagesStayUnderTheSizeCap counts lines; neither touches an OS-specific API |
| internal/ask | all | os | every Ask call goes through proc.Run with Group:true (Setpgid plus kill(-pgid)) on /bin/sh fakes; TestATimeoutKillsTheWholeCall checks the group kill with syscall.Kill(pid,0) |
| internal/cli/parse | all | any | parse_test.go:TestArgumentChecks only parses argv in memory |
| internal/config | all | any | config_test.go:TestConfigTextKeepsEveryKey round-trips TOML text and compares the two reads; the git init only makes the temp dir a repo |
| internal/design | all | any | Check and Waivers are pure; MainFiles, MergeFiles and Own run git plumbing (ls-tree, rev-list, diff --name-only, log) and compare repo-relative names only, with no absolute paths, realpath or locks |
| internal/hooks | all | any | chunking, transcript JSONL parsing, hook JSON I/O, and a /bin/sh fake Self read through proc.Run without process groups (prime_test.go:fakeSelf) |
| internal/item | all | any | item_test.go:TestTheItemReadersLinkNoDolt only checks `go list` deps |
| internal/store | all | os | every test uses store.Lock (syscall.Flock), clone() with filepath.EvalSymlinks on a temp dir, git subprocesses, or os.Chmod(p, 0) (TestApplyNeverOverwritesARecordItCannotRead) |
| internal/cli | TestEveryFuncDocStartsWithItsName | any | parses Go source docs |
| internal/cli | TestEveryCommandPmRunsIsListedInHelp | any | renders --help text from the command tree |
| internal/cli | TestAnAbbreviatedOptionIsRefused | any | the argv parser refuses before any store or repo access |
| internal/cli | TestDayActivityHoldsOnlyThatDaysEvents | os | git commits in t.TempDir read through records.Read (EvalSymlinks realpath) and git dates |
| internal/cli | TestHelpTextsEqualTheGolden | any | compares help text with the golden file |
| internal/cli | TestStampAndTextsSurviveGitRewritingTheRecordsMidWalk | os | git checkout unlinks and rewrites files while records.Texts walks them (unlink/readdir race, EvalSymlinks) |
| internal/cli | TestLoadNeverPassesARecordSetMissingAFileGitRewrote | os | git rewrite race, records lock, worktest.ServeAt (Dolt on a unix socket), EvalSymlinks on MkdirTemp |
| internal/cli | TestSessionInboxIsTheRuntimesOwn | any | env vars only; innermost is never reached |
| internal/cli | TestAnAmbiguousSessionRefusesEveryCommand | any | innermost is stubbed; the refusal comes before any I/O |
| internal/cli | TestTaskReadyListsReadyTasksInOrder | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestTaskEdit | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestTaskRelease | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestDepAddAndRm | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestSyncCommand | os | newFixture plus a bare git remote that Dolt syncs through |
| internal/cli | TestShowIDPrintsTheItemItsHolderBlockersChildrenNeedsAndComments | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestTaskAddParentAddsASubTaskUnderAnOpenTask | os | newFixture: Dolt host on a unix socket |
| internal/cli | TestWrapFillPutsALongWordOnItsOwnLine | any | text wrapping |
| internal/install | TestCodexTrustNamesPmsHooksCodexSkips | os | CodexTrust runs codex under Setpgid with a kill(-pgid) cancel |
| internal/install | TestCodexTrustSaysWhyItWasNotCheckedAndIsBounded | os | the timeout kills a sleeping codex process group |
| internal/install | TestCodexTrustAgainstRealCodex | os | real codex binary; EvalSymlinks realpath used as the trusted project key |
| internal/install | TestPiecesEqualTheGoldenResults | any | piece text Apply/Remove/Drift on plain files, compared with the JSON golden |
| internal/install | TestEachRuntimesJudgeRunsOnThatRuntime | any | hook-entry text |
| internal/install | TestClaudeAndCodexRunTheSameHooks | any | hook-entry text |
| internal/install | TestAnEarlierPmsCodexHooksAreRewritten | any | JSON piece Drift/Rewrite on plain files |
| internal/install | TestDumpJSONKeepsASettingsFileByteForByte | any | JSON serialization |
| internal/install | TestTOMLStringReadsBack | any | TOML string quoting |
| internal/install | TestPrunePinsRemovesTheBinaryOfEachPinNoCheckoutPinsAndKeepsItsSha256 | os | XDG_DATA_HOME pin dirs plus git worktree paths with newlines |
| internal/install | TestPrunePinsPrunesNoneWhenWhatAClonePinsCannotBeRead | os | same XDG and git worktree fixture |
| internal/install | TestCodexRulesAllowPmAlone | any | codex execpolicy check on a rules text (skipped without codex) |
| internal/install | TestCodexRulesAreAPieceDoctorUpgradeAndUninstallKnow | any | piece text written and read back |
| internal/install | TestSetupWorkCreatesPushesAndClones | os | work.NewHost (Dolt on a unix socket) plus a bare git remote |
| internal/install | TestSetupWorkRefusesAFork | os | work.NewHost (Dolt on a unix socket) plus a bare git remote |
| internal/install | TestInstallBinary | os | os.Executable copy, 0755 Perm check, symlink replacement |
| internal/install | TestExcludeRoundTrip | any | exclude file text round trip |
| internal/install | TestRewriteTakesOutBeadsPieces | os | AGENTS.md symlink kept (isLink), hook files' 0755 Perm checked |
| internal/install | TestHooksPathMovesOffBeads | os | git worktree add and core.hooksPath compared with absolute temp paths |
| internal/install | TestABeadsFileIsNoBeads | os | os.Chmod(0) expects ErrPermission; skipped as root |
| internal/launch | TestGuidesPrintsEachCrossedReleasesGuideOldestFirst | any | changelog text |
| internal/launch | TestGuidesFailWhenACrossedReleaseHasNoGuide | any | changelog text |
| internal/launch | TestEmbeddedNotesGiveEveryReleaseAGuide | any | embedded text |
| internal/launch | TestGuidesWithNoEntryFilesGiveTheTwoSteps | any | changelog text |
| internal/launch | TestNewerComparesVersionsAndAPreReleaseWithItsRelease | any | version comparison |
| internal/launch | TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne | os | XDG_DATA_HOME pins dir, tarball install with modes, exec of the pinned binary |
| internal/launch | TestMarkersAreThoseScrubTookAndNoneUnlaunched | os | re-execs os.Args[0] in a git repo and reads environment markers across exec |
| internal/launch | TestAGoPinIsDownloadedOnceThenExecdWithTheMarks | os | exec of the downloaded pin, "Exec format error", 0755 Perm, symlink tar entry |
| internal/records | TestMatchReadsDoublestarGlobs | any | glob matching |
| internal/records | TestCoversIsADesignPagesListOfPatterns | any | header parsing |
| internal/records | TestHeaderValuesTypeAsPyyamlDoes | any | YAML typing compared with expected.json |
| internal/records | TestReadSummariesLeavesOutASummaryGoneWhenRead | os | dangling os.Symlink treated as gone |
| internal/service | TestPmInitKeepsTheUnitsPATH | os | launchd/systemd unit files, PlatformKind, fake launchctl/systemctl |
| internal/service | TestUninstallStopsAndRemovesTheUnit | os | service units and supervisor calls |
| internal/service | TestAServiceOnMainsPinIsNotStaleFromACheckoutOffIt | os | unit and service lifecycle fixture |
| internal/service | TestServiceInstallStatusRestartAndLogsEndToEnd | os | unit install, supervisor and log paths |
| internal/service | TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall | os | systemd and launchd stop paths, socket still answering |
| internal/service | TestASupervisorThatDoesNotAnswerIsNoStop | os | systemctl is-enabled semantics |
| internal/service | TestAReplyIsQueuedIntoALiveCodexSession | os | codex queue exec under Setpgid/kill(-pgid) |
| internal/service | TestTheServiceQueuesAReplyIntoACodexSessionAndMarksItDelivered | os | serveEvery (service.Run: TCP listen, flock) plus a Setpgid codex exec |
| internal/service | TestThePublicHostNeedsACloudflareAccessTokenThatVerifies | any | access handler on httptest |
| internal/service | TestThePublicHostIsRefusedWhenTheTeamKeysCannotBeFetched | any | access handler on httptest |
| internal/service | TestWithoutTheAccessKeysEveryPublicRequestIsRefusedNamingThem | any | access handler on httptest |
| internal/service | TestLocalhostIsServedAsBeforeAndAnEdgeForwardedRequestNeedsTheTokenWhateverItsHost | any | access handler on httptest |
| internal/service | TestTheServiceChecksAccessOnEveryRequestAndARefusalNamesNoStore | os | serve(): service.Run with TCP listen and flock |
| internal/service | TestInboxMarksWithinOneBudget | any | MarkWithin on a fake store with an injected clock |
| internal/service | TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers | os | PlatformKind unit file, UnitBytes for launchd/systemd, TCP port held |
| internal/service | TestServesPagesStyleAndVersionWithTheStoreAndVersionHeaders | os | serve(): service.Run with TCP listen and flock |
| internal/service | TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt | os | serve() plus os.Symlink escapes refused by os.Root |
| internal/service | TestAReplyIsSpooledStoredOnceAndPushedIntoTheSessionsInbox | os | flocked spool plus a unix-socket inbox in /tmp |
| internal/service | TestAReplyIsRefusedWithItsReason | os | serve(): service.Run with flock and TCP |
| internal/service | TestAPartialReadIsRepeatedUnderTheLockAndServedThere | os | records flock (LOCK_SH) inside service.Run |
| internal/service | TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsRefused | os | serve() plus a flocked spool |
| internal/service | TestAPendingSpoolIsStoredAtStartOnce | os | flocked spool at start |
| internal/service | TestTheSweepDeliversWhatAFailedPushLeftAndSkipsAGoneSession | os | unix-socket inbox and a gone .sock (ENOENT/ECONNREFUSED) |
| internal/service | TestAReplyAnyCommandStoresIsPushedOnceTheStoreMoves | os | serve() plus a unix-socket inbox |
| internal/service | TestAMergeToMainIsStoredAndPushedOnce | os | serve() plus git init and a bare origin |
| internal/service | TestRunReturnsOnlyOnceTheGoroutinesItStartedEnded | os | serve(): service.Run TCP listen and the pin-file rename watch |
| internal/service | TestSyncStepsSyncTheStoreSummarizeAndPushTheRecords | any | fake work; the records step's result is not asserted |
| internal/service | TestGCRunsWhenDueAndRecordsTheSizes | any | logical file sizes (dirSize) and a JSON state file |
| internal/service | TestSweepsCoalesceAndNeverTakeAReplysPlace | any | channel logic |
| internal/service | TestSinceIsTheRepliesFromTheWaitsSecond | any | time filtering |
| internal/service | TestAPushToASessionWithNoInboxWaitsForItWhileItLives | any | SessionLive stubbed, no inbox |
| internal/session | TestSessionOfIsTheSetOneOrTheInnermostRuntimesWhenBothAre | any | injected innermost |
| internal/session | TestInnermostRuntimeIsTheNearestClaudeCodeOrCodexAncestor | any | injected procInfo |
| internal/session | TestReadProcReadsAProcesssParentAndArgv0 | os | /proc/<pid>/stat on Linux, `ps -o ppid=,comm=` on macOS |
| internal/session | TestUUID7StartIsTheMillisecondsItsFirst48BitsHold | any | bit arithmetic |
| internal/session | TestCodexRootMapsASubagentThreadToItsRootThread | any | rollout JSONL files under CODEX_HOME |
| internal/session | TestCurrentSessionOfACodexSubagentIsItsRootThread | any | env plus rollout files |
| internal/session | TestCodexRolloutsReadsOnlyTheDatesAThreadCanHaveStarted | any | dated rollout directory reads |
| internal/session | TestCodexLiveLooksUnderTheHomeItIsGiven | any | rollout mtime within LiveWindow |
| internal/session | TestCodexHooksRanReadsTheRolloutUpToTheFirstPrompt | any | rollout parsing |
| internal/site | TestMermaidLoadsOneExactVersionAtNaturalWidth | any | rendered HTML strings |
| internal/site | TestPagesEqualTheGoldenPages | os | git fast-import store in TempDir read through records.Read (EvalSymlinks) with git dates |
| internal/site | TestADesignPageSaysWhyItsCoveredCodeWasNotChecked | os | same git store fixture plus git update-ref |
| internal/site | TestCheckRefusesEachBrokenRecord | os | records.Read and ReadSummaries on edited temp-dir copies of the constructs store (realpath) |
| internal/site | TestCodeChangedLeavesOutTheMergesOfSprintsThatUpdatedOrWaivedThePage | any | in-memory commits and records |
| internal/site | TestFillRepliesAndStatus | any | HTML template fill compared with testdata |
| internal/site | TestAReplyToASessionWithNoInboxWaitsForIt | any | HTML text |
| internal/sync | TestARunRecordsEachStepAndFlagsAFailure | os | the push lock is syscall.Flock LOCK_NB; the ASecondRunSkipsWhileOneHoldsTheLock subtest depends on it |
| internal/sync | TestANeverSuccessfulPushIsOverdueFromTheInstall | any | JSON state file and an injected clock |
| internal/sync | TestPushRecordsPushesRebasesAndReports | os | git clone/push/rebase under a store flock, with process-group kill in Run |
| internal/work | TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt | any | AST scan of the module and a file walk |
| internal/work | TestBenchLoad | os | Dolt store host (skipped without PM_BENCH_STORE) |
| internal/work | TestBenchWriters | os | Dolt store host with concurrent writers (skipped without PM_BENCH_STORE) |
| internal/work | TestCheckRefusesEachImpossibleState | any | FromBD plus invariant checks in memory |
| internal/work | TestAWriteRacingOneOutsideTheLockFailsHard | os | embedded Dolt writes through the host |
| internal/work | TestDialNamesASocketTheSystemRefuses | os | unix socket dial errors and the sun_path limit |
| internal/work | TestDeliveredIsNeverLowered | os | serve(): Dolt on a unix socket |
| internal/work | TestClaimIsACompareAndSet | os | serve(): Dolt on a unix socket |
| internal/work | TestCommentsKeepTheirOrderWithinASecond | os | serve(): Dolt on a unix socket |
| internal/work | TestFromExportRefusesALineThatIsNotAnItem | os | serve(): Dolt import and commits |
| internal/work | TestImportFailsHardOnWhatItCannotMap | any | bd export parsing from testdata |
| internal/work | TestAWriteLockWaitEndsAtItsBoundOrItsStatement | any | in-memory writeLock with injected liveness |
| internal/work | TestASecondLockOnAConnectionAndAnUnlockWithoutItAreRefused | any | in-memory writeLock |
| internal/work | TestPmLockThroughTheHost | os | serve(): pm_lock over the Dolt host socket, dropped connections |
| internal/work | TestMergeOtherScalarsTheLaterUpdatedAt | any | pure three-way row merge |
| internal/work | TestAnOperationOutlastingItsBoundFreesTheSlotAndAWaiterGivesUp | os | Dolt host; the git that Dolt runs is killed at the bound |
| internal/work | TestPullRacingLocalWriters | os | two Dolt clones, a git remote, concurrency |
| internal/work | TestCreateRacingALocalWriter | os | Dolt plus a git remote, CAS |
| internal/work | TestTwoClonesMigratingOnTheirOwnMerge | os | two Dolt clones merging |
| internal/work | TestRoundTripOfTheFixture | os | serve(): Dolt import and export |
| internal/work | TestRoundTripOfARealExport | os | serve(): Dolt import (skipped without PM_BD_EXPORT) |
| internal/work | TestDoltHoldsTheJournalPruneFix | os | Dolt journal and GC on disk |
| internal/work | TestACollectionRacingItsOwnVisitsReturnsOnceEachEnds | os | Dolt GC racing visits |
| internal/work | TestSyncTwoClonesConcurrentEditsMergePerTheTable | os | two hosted Dolt clones through a bare git remote |
| internal/work | TestSyncTwoClonesMergeListFieldsPerEntry | os | two hosted Dolt clones through a bare git remote |
| internal/work | TestSyncTwoClonesCloseBeatsClaimAndACreateGivesUp | os | two hosted Dolt clones through a bare git remote |
| internal/work | TestSyncTwoClonesMoveSprintKeepsTheOtherClonesWrites | os | two hosted Dolt clones through a bare git remote |
| internal/work | TestCreatePushTimeout | os | Dolt push to the remote with a timeout kill |
| internal/work | TestSyncFailsHardOnAddAddAndACycle | os | Dolt sync through a git remote |
| internal/work | TestSyncFailsHardOnAnUnsettledConflictAndABrokenRemoteHead | os | Dolt sync through a git remote |
| internal/work | TestCreateRefusesWithTheRemoteUnreachable | os | Dolt host plus an unreachable git remote |
| internal/work | TestAServiceOnAnotherVersionIsRefused | os | NewHost/Dial over a unix socket, sun_path length refusal |

os: its behaviour can differ by OS, so a PR runs it on macOS; any: Linux on each PR, macOS after merge.
:::

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: macOS Go compute per PR fell 20% (median 163 to 130 s) and a code PR's wall fell 16% (138 to 116 s), short of the 55 s and 100 s targets; the Go tests do less work, and a PR runs on macOS only the tests whose behaviour can differ by OS.

- #81: the item model moved to internal/item, which links no Dolt, so 4 test binaries stopped linking Dolt; newPair seeds locally (241 to about 70 git calls); make test-go runs its parts at once.
- #82: internal/work's test seconds remeasured.
- #83: go-os-tests.txt classifies all 155 Go tests (85 os). A PR's macOS jobs run the darwin build and the os tests; a push to main runs every test on macOS; macOS runs RACE_TESTS only under -race (owner decisions). go-os-check holds every test to one row.
- #84: the port check test waits out 0.1 s probes (3.0 to 0.3 s).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- macOS Go compute (the build and test steps of the two macOS jobs, summed) ≤ ~55 s, median of 3 PRs: not met, 130 s. Samples on code PRs run on the new layout: #83 130 s, #84 122 s, #69 132 s (Findings). Before: 148-181 s over 13 runs, median 163 s; the frame's 96-98 s was the build-test step alone, which is now 63-70 s on a PR.
- A code PR's median wall time to the last check ≤ ~100 s over 3 PRs: not met, 116 s. #83 269 s (its agents-md job, run for its AGENTS.md change, was last), #84 109 s, #69 116 s; 138 s before.
- The per-OS lists of tests run (Linux identical; the main-push macOS run full): met. Linux runs the same targets, which select all 155 tests, as before. Main's push run on ad5659f ran every package, 23 internal/work tests, and the 9 RACE_TESTS plus internal/service under -race, and passed. A PR's macOS list equals the 85 os tests (checked locally with -v for each mode). Before #83, #81's lists at main and head were identical but for the added guard test.
- make test-go local wall time reported: 35.5-35.8 s before (3 runs); 19.6-19.8 s after (32 cores).
- make lint, make test and make test-go passed locally for each PR. CI was green on #81-#84, and each was merged by merge_ready.py's printed command after a fresh-context review.
