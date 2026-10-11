---
type: doc
title: "Test value audit: keep, fold or drop each test"
date: 2026-10-11
project: pm-quality
---

# pm test audit: per-test table

origin/main eedbafd, 2026-10-11. Unique = statements no other measured test covers, of the combined Go + light-harness union (9042 of 12262 module statements). Rank = position in the greedy set cover of that union. Runtime: Go from `go test -json -count=1 ./...` (per test), harness setup+call+teardown under `-n 32` with coverage-built binaries (sub-5 ms entries hidden by pytest show as 0.0). `-` = not measured: integration tests (CLAUDE.md: never run the whole integration set locally) and tests of release/*.py, install.sh and the Makefile.

- **Combined**: 345 measured tests, union 9042 stmts (73.74% of module). Fraction of tests -> % of union: 5% -> 74.56%, 10% -> 86.03%, 15% -> 91.95%, 20% -> 94.63%, 30% -> 97.53%, 50% -> 99.54%. Tests needed for: 80% = 24 (7.0%), 90% = 45 (13.0%), 95% = 73 (21.2%), 99% = 144 (41.7%), 100% = 214 (62.0%)
- **Go only**: 228 measured tests, union 7068 stmts (57.64% of module). Fraction of tests -> % of union: 5% -> 63.54%, 10% -> 76.61%, 15% -> 83.56%, 20% -> 88.95%, 30% -> 94.44%, 50% -> 98.83%. Tests needed for: 80% = 28 (12.3%), 90% = 49 (21.5%), 95% = 72 (31.6%), 99% = 119 (52.2%), 100% = 163 (71.5%)
- **Light harness only**: 117 measured tests, union 5519 stmts (45.01% of module). Fraction of tests -> % of union: 5% -> 79.92%, 10% -> 90.92%, 15% -> 96.14%, 20% -> 97.72%, 30% -> 99.17%, 50% -> 99.78%. Tests needed for: 80% = 7 (6.0%), 90% = 12 (10.3%), 95% = 16 (13.7%), 99% = 33 (28.2%), 100% = 70 (59.8%)

## Method and limits

- Harness: pm, pm-before and render-pages built with `go build -cover -coverpkg=<module>/...` (Makefile flags), with a SIGTERM handler that calls os.Exit added for the build only, so the per-test `pm service run` writes its counters; a pytest plugin (`-p`, outside tests/) set GOCOVERDIR per test. 145 light tests passed, 63 live skipped.
- Go: each package's test binary built once with `-cover -coverpkg=<module>/...`, each top-level test run alone with its own coverprofile and gocoverdir, 24 at a time. 4 launch tests fail under this harness only (their re-exec'd child loses GOCOVERDIR and prints a warning the test asserts against); their coverage is understated.
- Coverage counts statements reached, not assertions: a merge-rule test that drives the same code with different data has 0 unique statements yet checks a distinct rule. Such tests are MERGE (into a table test), not DROP.
- Bug guard = the test was added by a commit that fixed a defect (by subject/review): 102ff18, 10918ea, 15d5b8d, 18dcd01, 2848f0c, 2911a04, 2fe560f, 3a30121, 41b6696, 585ea6e, 6752e93, 6ae6bdd, 7c5df11, 7de6a11, 8d68f2f, 9b204b7, 9bb12c6, a43cd1c, a5d7580, b21bfcb, c661956, da3423c, dabffb6, e3bc836, ef54788, feb1728.
- Rules: KEEP = in the greedy prefix reaching 95% of the union, or >= 10 unique stmts, or a bug guard with unique stmts, or a judged contract; MERGE = contract / bug guard / race / parametrized / CLAUDE.md-named test with little unique coverage, folded into the named test as a table row; DROP = the rest. Integration and script tests are judged by hand (reason column).


## Groups (count, summed runtime s)

| suite | KEEP | MERGE | DROP |
|---|---|---|---|
| go | 69 (23.7) | 79 (86.2) | 80 (7.8) |
| light | 31 (38.5) | 42 (36.6) | 44 (45.3) |
| integration | 34 (0.0) | 22 (0.0) | 3 (0.0) |
| script | 11 (3.4) | 17 (2.5) | 0 (0.0) |
| live | 63 (0.0) | 0 (0.0) | 0 (0.0) |

## The strict 10% suite (43 of 432 non-live tests, greedy order)

1. `test_pm.py::test_sprint_move_takes_the_sprint_whole_and_a_rerun_finishes_a_move_cut_short`
2. `install.TestPiecesEqualTheGoldenResults`
3. `work.TestSyncTwoClonesConcurrentEditsMergePerTheTable`
4. `site.TestPagesEqualTheGoldenPages`
5. `test_pm.py::test_need_edit_rewrites_an_open_need_s_body_until_the_owner_replies`
6. `service.TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall`
7. `service.TestAReplyIsSpooledStoredOnceAndPushedIntoTheSessionsInbox`
8. `test_pm.py::test_new_records_feedback_projects_and_moves_write_what_they_say`
9. `test_hooks.py::test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[True-True]`
10. `test_config.py::test_every_config_key_is_named_in_some_commands_help`
11. `install.TestHooksPathMovesOffBeads`
12. `test_pm.py::test_reply_read_prints_each_undelivered_reply_and_merge_once`
13. `test_pm.py::test_sprint_edit_renames_the_sprint_in_both_stores_and_records_why`
14. `launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne`
15. `install.TestSetupWorkRefusesAFork`
16. `test_owner_request_hook.py::test_blocks_a_needless_ask_even_when_an_open_need_matches`
17. `service.TestServiceInstallStatusRestartAndLogsEndToEnd`
18. `test_pm.py::test_every_store_command_fails_hard_with_the_service_stopped`
19. `records.TestHeaderValuesTypeAsPyyamlDoes`
20. `cli.TestTaskReadyListsReadyTasksInOrder`
21. `cli.TestDayActivityHoldsOnlyThatDaysEvents`
22. `test_pm.py::test_feedback_about_any_project_goes_to_the_repo_s_one_feedback_doc`
23. `test_launch.py::test_a_repo_pinned_to_this_version_runs_in_process_with_no_uv_call`
24. `test_pm.py::test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does`
25. `test_hooks.py::test_prime_lists_every_agent_command_pm_help_lists`
26. `test_pm.py::test_task_close_resolves_another_repos_commit_or_a_pr_with_gh`
27. `hooks.TestFailuresNameOneLine`
28. `sync.TestARunRecordsEachStepAndFlagsAFailure`
29. `work.TestImportFailsHardOnWhatItCannotMap`
30. `sync.TestPushRecordsPushesRebasesAndReports`
31. `work.TestMergeOtherScalarsTheLaterUpdatedAt`
32. `service.TestGCRunsWhenDueAndRecordsTheSizes`
33. `work.TestPmLockThroughTheHost`
34. `work.TestCheckRefusesEachImpossibleState`
35. `site.TestCheckRefusesEachBrokenRecord`
36. `service.TestAMergeToMainIsStoredAndPushedOnce`
37. `test_pm.py::test_a_decision_recorded_for_a_no_decision_need_marks_it_answered`
38. `install.TestABeadsFileIsNoBeads`
39. `cli.TestSyncCommand`
40. `install.TestExcludeRoundTrip`
41. `cli.TestDepAddAndRm`
42. `install.TestInstallBinary`
43. `service.TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt`

## Every test

| action | suite | test | uniq | cov | rank | s | commit | reason |
|---|---|---|---|---|---|---|---|---|
| KEEP | go | `cli.TestDayActivityHoldsOnlyThatDaysEvents` | 72 | 322 | 21 | 0.0 | bba21e4 | greedy rank 21: +72 stmts |
| KEEP | go | `cli.TestDepAddAndRm` | 15 | 727 | 41 | 0.4 | fad11ab | greedy rank 41: +30 stmts |
| KEEP | go | `cli.TestLoadNeverPassesARecordSetMissingAFileGitRewrote` | 2 | 437 | 61 | 0.3 | da3423c | greedy rank 61: +14 stmts **BUG GUARD** |
| KEEP | go | `cli.TestShowIDPrintsTheItemItsHolderBlockersChildrenNeedsAndComments` | 8 | 804 | 58 | 0.5 | 7052c96 | greedy rank 58: +17 stmts |
| KEEP | go | `cli.TestStampAndTextsSurviveGitRewritingTheRecordsMidWalk` | 2 | 136 | 149 | 0.6 | 7c5df11 | bug guard: site change check crashed on a vanished file **BUG GUARD** |
| KEEP | go | `cli.TestSyncCommand` | 21 | 865 | 39 | 0.7 | fad11ab | greedy rank 39: +33 stmts |
| KEEP | go | `cli.TestTaskAddParentAddsASubTaskUnderAnOpenTask` | 15 | 732 | 59 | 0.5 | 7052c96 | greedy rank 59: +15 stmts |
| KEEP | go | `cli.TestTaskEdit` | 22 | 746 | 44 | 0.5 | fad11ab | greedy rank 44: +28 stmts |
| KEEP | go | `cli.TestTaskReadyListsReadyTasksInOrder` | 13 | 779 | 20 | 0.5 | fad11ab | greedy rank 20: +76 stmts |
| KEEP | go | `cli.TestTaskRelease` | 9 | 710 | 68 | 0.5 | fad11ab | greedy rank 68: +10 stmts |
| KEEP | go | `cli.TestWrapFillPutsALongWordOnItsOwnLine` | 3 | 94 | 128 | 0.0 | b21bfcb | bug guard: frame heading/first sentence split on ASCII not Python \s **BUG GUARD** |
| KEEP | go | `hooks.TestContextCutsAtALineToTheCapacity` | 10 | 47 | 69 | 0.0 | 6463ce2 | greedy rank 69: +10 stmts |
| KEEP | go | `hooks.TestFailuresNameOneLine` | 47 | 89 | 27 | 0.0 | 6463ce2 | greedy rank 27: +59 stmts |
| KEEP | go | `hooks.TestTouchedCountsASubagentsPathOnceItsCallReturns` | 3 | 143 | 131 | 0.0 | 2911a04 | bug guard: stop hooks blocked what was not the agent's to fix **BUG GUARD** |
| KEEP | go | `install.TestABeadsFileIsNoBeads` | 25 | 413 | 38 | 0.0 | a5d7580 | greedy rank 38: +36 stmts **BUG GUARD** |
| KEEP | go | `install.TestExcludeRoundTrip` | 33 | 106 | 40 | 0.0 | 7719888 | greedy rank 40: +33 stmts |
| KEEP | go | `install.TestHooksPathMovesOffBeads` | 113 | 286 | 11 | 0.0 | b7cbb0e | greedy rank 11: +144 stmts |
| KEEP | go | `install.TestInstallBinary` | 1 | 55 | 42 | 0.2 | 7719888 | greedy rank 42: +30 stmts |
| KEEP | go | `install.TestPiecesEqualTheGoldenResults` | 142 | 641 | 2 | 0.0 | 2d1a4db | greedy rank 2: +615 stmts |
| KEEP | go | `install.TestRewriteTakesOutBeadsPieces` | 6 | 461 | 46 | 0.0 | b7cbb0e | greedy rank 46: +27 stmts |
| KEEP | go | `install.TestSetupWorkCreatesPushesAndClones` | 10 | 447 | 70 | 0.8 | 7719888 | greedy rank 70: +10 stmts |
| KEEP | go | `install.TestSetupWorkRefusesAFork` | 20 | 454 | 15 | 0.8 | 18dcd01 | greedy rank 15: +101 stmts **BUG GUARD** |
| KEEP | go | `launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne` | 6 | 115 | 14 | 0.0 | 833d3e9 | greedy rank 14: +115 stmts |
| KEEP | go | `launch.TestAGoPinIsDownloadedOnceThenExecdWithTheMarks` | 0 | 2 |  | 0.0 | 78c7880 | launcher download+sha+exec core; coverage under-measured (re-exec drops GOCOVERDIR) (0 unique stmts) |
| KEEP | go | `launch.TestMarkersAreThoseScrubTookAndNoneUnlaunched` | 1 | 9 | 189 | 0.0 | 7de6a11 | bug guard: launcher lost from bin dir **BUG GUARD** |
| KEEP | go | `pyjson.TestStrReprQuotesAsPython` | 11 | 20 | 66 | 0.0 | 6463ce2 | greedy rank 66: +11 stmts |
| KEEP | go | `records.TestHeaderValuesTypeAsPyyamlDoes` | 20 | 130 | 19 | 0.0 | bc13f4d | greedy rank 19: +78 stmts |
| KEEP | go | `records.TestReadSummariesLeavesOutASummaryGoneWhenRead` | 3 | 71 | 130 | 0.0 | 7c5df11 | bug guard: site change check crashed on a vanished file **BUG GUARD** |
| KEEP | go | `service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsRefused` | 2 | 222 | 159 | 0.0 | 10918ea | bug guard: service review findings **BUG GUARD** |
| KEEP | go | `service.TestAMergeToMainIsStoredAndPushedOnce` | 14 | 259 | 36 | 0.3 | 2cfae09 | greedy rank 36: +37 stmts |
| KEEP | go | `service.TestAPartialReadIsRepeatedUnderTheLockAndServedThere` | 4 | 239 | 53 | 0.0 | da3423c | greedy rank 53: +19 stmts **BUG GUARD** |
| KEEP | go | `service.TestAPendingSpoolIsStoredAtStartOnce` | 10 | 266 | 71 | 0.1 | 2cfae09 | greedy rank 71: +10 stmts |
| KEEP | go | `service.TestAReplyIsRefusedWithItsReason` | 14 | 413 | 47 | 0.1 | 2cfae09 | greedy rank 47: +26 stmts |
| KEEP | go | `service.TestAReplyIsSpooledStoredOnceAndPushedIntoTheSessionsInbox` | 7 | 432 | 7 | 0.1 | 2cfae09 | greedy rank 7: +268 stmts |
| KEEP | go | `service.TestAServiceOnMainsPinIsNotStaleFromACheckoutOffIt` | 2 | 266 | 151 | 0.0 | 6752e93 | bug guard: a pin move stranded worktrees silently **BUG GUARD** |
| KEEP | go | `service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers` | 10 | 75 | 57 | 3.0 | 2cfae09 | greedy rank 57: +17 stmts |
| KEEP | go | `service.TestGCRunsWhenDueAndRecordsTheSizes` | 47 | 58 | 32 | 0.0 | 4db4259 | greedy rank 32: +47 stmts |
| KEEP | go | `service.TestRunReturnsOnlyOnceTheGoroutinesItStartedEnded` | 1 | 389 | 191 | 0.3 | 41b6696 | bug guard: service.Run returned before its goroutines ended **BUG GUARD** |
| KEEP | go | `service.TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt` | 27 | 196 | 43 | 0.0 | 214d486 | greedy rank 43: +30 stmts |
| KEEP | go | `service.TestServesPagesStyleAndVersionWithTheStoreAndVersionHeaders` | 13 | 253 | 62 | 0.0 | 2cfae09 | greedy rank 62: +13 stmts |
| KEEP | go | `service.TestServiceInstallStatusRestartAndLogsEndToEnd` | 20 | 330 | 17 | 0.5 | 2cfae09 | greedy rank 17: +85 stmts |
| KEEP | go | `service.TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall` | 27 | 325 | 6 | 0.1 | a274b65 | greedy rank 6: +305 stmts |
| KEEP | go | `service.TestSyncStepsSyncTheStoreSummarizeAndPushTheRecords` | 10 | 24 | 73 | 0.0 | 4db4259 | greedy rank 73: +10 stmts |
| KEEP | go | `service.TestTheSweepDeliversWhatAFailedPushLeftAndSkipsAGoneSession` | 24 | 241 | 51 | 0.0 | 2cfae09 | greedy rank 51: +24 stmts |
| KEEP | go | `service.TestUninstallStopsAndRemovesTheUnit` | 12 | 135 | 63 | 0.0 | 2cfae09 | greedy rank 63: +12 stmts |
| KEEP | go | `site.TestCheckRefusesEachBrokenRecord` | 33 | 1062 | 35 | 0.3 | 2d1a4db | greedy rank 35: +37 stmts |
| KEEP | go | `site.TestFillRepliesAndStatus` | 25 | 40 | 50 | 0.0 | 3c7d9ee | greedy rank 50: +25 stmts |
| KEEP | go | `site.TestPagesEqualTheGoldenPages` | 9 | 1202 | 4 | 0.0 | 2d1a4db | greedy rank 4: +381 stmts |
| KEEP | go | `store.TestApplyCommitsOrRestores` | 22 | 110 | 52 | 0.0 | bc13f4d | greedy rank 52: +22 stmts |
| KEEP | go | `store.TestApplyNeverOverwritesARecordItCannotRead` | 1 | 15 | 185 | 0.0 | 9b204b7 | bug guard: records/store/site review findings **BUG GUARD** |
| KEEP | go | `store.TestCommitCommitsOnlyThePathsNamed` | 1 | 44 | 173 | 0.0 | bc13f4d | pm commit commits only named paths (prime.md) (1 unique stmts) |
| KEEP | go | `sync.TestARunRecordsEachStepAndFlagsAFailure` | 13 | 111 | 28 | 0.0 | 2f6d3d8 | greedy rank 28: +59 stmts |
| KEEP | go | `sync.TestPushRecordsPushesRebasesAndReports` | 39 | 50 | 30 | 0.1 | 2f6d3d8 | greedy rank 30: +50 stmts |
| KEEP | go | `work.TestASecondLockOnAConnectionAndAnUnlockWithoutItAreRefused` | 2 | 19 | 155 | 0.0 | 585ea6e | bug guard: write lock holder/wait **BUG GUARD** |
| KEEP | go | `work.TestAServiceOnAnotherVersionIsRefused` | 5 | 230 | 45 | 0.0 | 4db4259 | greedy rank 45: +28 stmts |
| KEEP | go | `work.TestAWriteLockWaitEndsAtItsBoundOrItsStatement` | 1 | 40 | 188 | 0.1 | 585ea6e | bug guard: write lock holder/wait **BUG GUARD** |
| KEEP | go | `work.TestAWriteRacingOneOutsideTheLockFailsHard` | 5 | 481 | 72 | 0.3 | feb1728 | greedy rank 72: +10 stmts **BUG GUARD** |
| KEEP | go | `work.TestAnOperationOutlastingItsBoundFreesTheSlotAndAWaiterGivesUp` | 9 | 672 | 64 | 2.5 | 2848f0c | greedy rank 64: +12 stmts **BUG GUARD** |
| KEEP | go | `work.TestCheckRefusesEachImpossibleState` | 25 | 184 | 34 | 0.0 | fb42de7 | greedy rank 34: +38 stmts |
| KEEP | go | `work.TestClaimIsACompareAndSet` | 6 | 614 | 92 | 0.4 | 55e396c | one-holder invariant (prime.md) (6 unique stmts) |
| KEEP | go | `work.TestCommentsKeepTheirOrderWithinASecond` | 1 | 600 | 180 | 0.5 | 9bb12c6 | bug guard: work store review findings **BUG GUARD** |
| KEEP | go | `work.TestCreatePushTimeout` | 0 | 796 | 60 | 3.3 | 4860f5c | greedy rank 60: +14 stmts |
| KEEP | go | `work.TestDoltHoldsTheJournalPruneFix` | 0 | 0 |  | 0.0 | 4c36803 | guards an upstream Dolt fix pm depends on (0 unique stmts) |
| KEEP | go | `work.TestFromExportRefusesALineThatIsNotAnItem` | 6 | 113 | 55 | 0.0 | 5d5f7a2 | greedy rank 55: +18 stmts |
| KEEP | go | `work.TestImportFailsHardOnWhatItCannotMap` | 29 | 350 | 29 | 0.0 | 6fcbaa5 | greedy rank 29: +54 stmts |
| KEEP | go | `work.TestMergeOtherScalarsTheLaterUpdatedAt` | 12 | 96 | 31 | 0.0 | 4860f5c | greedy rank 31: +48 stmts |
| KEEP | go | `work.TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt` | 0 | 0 |  | 0.0 | 4db4259 | the one access path (CLAUDE.md) (0 unique stmts) |
| KEEP | go | `work.TestPmLockThroughTheHost` | 9 | 494 | 33 | 0.6 | 585ea6e | greedy rank 33: +45 stmts **BUG GUARD** |
| KEEP | go | `work.TestSyncTwoClonesConcurrentEditsMergePerTheTable` | 1 | 959 | 3 | 4.5 | 4860f5c | greedy rank 3: +428 stmts |
| KEEP | integration | `test_clean.py::test_clean_keeps_dirty_unpushed_and_not_agent_worktrees_and_apply_removes_merged_squashed_and_pushed` | - | - |  | 0.0 | 0f65958 | pm clean deletes worktrees: the core keep/remove table [coverage not measured] |
| KEEP | integration | `test_clean.py::test_clean_keeps_what_a_removal_would_delete_with_it` | - | - |  | 0.0 | 0f65958 | data-loss guard of pm clean [coverage not measured] |
| KEEP | integration | `test_config.py::test_a_pin_move_names_the_worktrees_it_strands` | - | - |  | 0.0 | 6752e93 | bug guard (sprint 16) [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_hooks.py::test_session_start_fails_open_with_one_line` | - | - |  | 0.0 | b24b04e | hooks fail open: contract [coverage not measured] |
| KEEP | integration | `test_hooks.py::test_session_start_injects_rules_then_init_where_and_pm_show` | - | - |  | 0.0 | 7f7503c | session-start contract [coverage not measured] |
| KEEP | integration | `test_hooks.py::test_session_start_starts_a_down_service_once` | - | - |  | 0.0 | 89a81c0 | parallel session starts start one service [coverage not measured] |
| KEEP | integration | `test_init.py::test_init_bootstraps_a_brand_new_repo` | - | - |  | 0.0 | 35f940f | install contract; checks hook entries [coverage not measured] |
| KEEP | integration | `test_init.py::test_init_in_a_worktree_of_a_branch_without_config_refuses` | - | - |  | 0.0 | 2fe560f | bug guard 2fe560f [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_init.py::test_init_in_a_worktree_sets_it_up_when_mains_pin_differs` | - | - |  | 0.0 | 2fe560f | bug guard 2fe560f [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_init.py::test_init_refuses_a_held_site_port_before_writing_anything` | - | - |  | 0.0 | 3a30121 | bug guard 3a30121 [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_launch.py::test_upgrade_moves_a_python_pin_and_refuses_to_move_one_to_python` | - | - |  | 0.0 | b0c8166 | upgrade contract [coverage not measured] |
| KEEP | integration | `test_launch.py::test_upgrade_without_to_never_moves_a_newer_pin_down` | - | - |  | 0.0 | 15d5b8d | bug guard 15d5b8d [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_lifecycle.py::test_doctor_names_a_settings_file_it_cannot_read_as_a_repo_finding` | - | - |  | 0.0 | 2fe560f | bug guard 2fe560f [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_lifecycle.py::test_doctor_reports_each_changed_repo_piece_and_upgrade_restores_it` | - | - |  | 0.0 | cb0c3cd | doctor/upgrade contract [coverage not measured] |
| KEEP | integration | `test_lifecycle.py::test_init_and_upgrade_keep_what_is_not_pms` | - | - |  | 0.0 | cb0c3cd | never clobber user files [coverage not measured] |
| KEEP | integration | `test_lifecycle.py::test_stop_keeps_the_service_stopped_at_session_start_until_restart` | - | - |  | 0.0 | a274b65 | service stop contract [coverage not measured] |
| KEEP | integration | `test_lifecycle.py::test_uninstall_removes_pms_parts_and_setup_only` | - | - |  | 0.0 | 34d22ac | uninstall contract [coverage not measured] |
| KEEP | integration | `test_lifecycle.py::test_uninstall_waits_for_a_session_start_that_holds_the_install_lock` | - | - |  | 0.0 | da3423c | bug guard da3423c [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_lifecycle.py::test_uninstall_with_the_service_stopped_checks_for_unsynced_work` | - | - |  | 0.0 | a274b65 | data-loss guard [coverage not measured] |
| KEEP | integration | `test_lifecycle.py::test_upgrade_retires_the_main_branchs_records_copy` | - | - |  | 0.0 | 6a0bb49 | migration still shipped (Retired pieces) [coverage not measured] |
| KEEP | integration | `test_merge_ready.py::test_a_check_that_ran_twice_on_the_head_is_judged_by_its_latest_run` | - | - |  | 0.0 | af0a631 | merge-ready rule [coverage not measured] |
| KEEP | integration | `test_merge_ready.py::test_a_pr_cut_before_main_moved_is_refused_until_it_is_rebased` | - | - |  | 0.0 | af0a631 | the reason merge-ready exists [coverage not measured] |
| KEEP | integration | `test_pm.py::test_every_reply_is_pushed_into_the_session_that_asked` | - | - |  | 0.0 | b24b04e | owner reply delivery end to end [coverage not measured] |
| KEEP | integration | `test_pm.py::test_init_on_a_fresh_clone_checks_out_store_links_records_and_starts_the_service` | - | - |  | 0.0 | 7f7503c | second-clone init [coverage not measured] |
| KEEP | integration | `test_pm.py::test_push_pushes_beads_and_new_records_commits` | - | - |  | 0.0 | b24b04e | pm push end to end [coverage not measured] |
| KEEP | integration | `test_pm.py::test_reply_is_refused_without_the_token_or_from_another_host[forged-form0-headers0-403-no valid token]` | - | - |  | 0.0 | f16edb8 | security: forged reply refused [coverage not measured] |
| KEEP | integration | `test_pm.py::test_reply_is_refused_without_the_token_or_from_another_host[page-form1-headers1-403-is neither this machine]` | - | - |  | 0.0 | f16edb8 | security: forged reply refused [coverage not measured] |
| KEEP | integration | `test_pm.py::test_serve_shows_each_change_within_its_stated_age` | - | - |  | 0.0 | b24b04e | site freshness contract [coverage not measured] |
| KEEP | integration | `test_pm.py::test_sprint_closes_after_its_pr_merges` | - | - |  | 0.0 | b24b04e | sprint-close rule [coverage not measured] |
| KEEP | integration | `test_pm.py::test_write_on_one_branch_is_visible_on_another_without_merge` | - | - |  | 0.0 | b24b04e | records-link contract [coverage not measured] |
| KEEP | integration | `test_release.py::test_the_release_build_takes_its_version_from_the_tag_alone` | - | - |  | 0.0 | 279bdbc | release contract [coverage not measured] |
| KEEP | integration | `test_service.py::test_install_fails_when_the_service_does_not_come_up` | - | - |  | 0.0 | ef54788 | bug guard ef54788 [coverage not measured] **BUG GUARD** |
| KEEP | integration | `test_service.py::test_service_install_status_restart_and_logs_end_to_end` | - | - |  | 0.0 | 5edd1f4 | service end to end [coverage not measured] |
| KEEP | integration | `test_service.py::test_service_stops_once_the_pin_moves` | - | - |  | 0.0 | a43cd1c | bug guard a43cd1c [coverage not measured] **BUG GUARD** |
| KEEP | light | `test_config.py::test_every_config_key_is_named_in_some_commands_help` | 7 | 344 | 10 | 2.3 | f8c76ad | greedy rank 10: +149 stmts |
| KEEP | light | `test_hooks.py::test_pm_init_is_the_one_install_command` | 10 | 244 | 74 | 0.1 | 7f7503c | 10 stmts no other test runs |
| KEEP | light | `test_hooks.py::test_prime_lists_every_agent_command_pm_help_lists` | 21 | 1323 | 25 | 0.7 | 5da0808 | greedy rank 25: +64 stmts |
| KEEP | light | `test_hooks.py::test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[True-True]` | 0 | 1393 | 9 | 0.6 | 2911a04 | greedy rank 9: +157 stmts **BUG GUARD** |
| KEEP | light | `test_launch.py::test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_that_download` | 0 | 1282 | 54 | 1.6 | 833d3e9 | greedy rank 54: +19 stmts |
| KEEP | light | `test_launch.py::test_a_repo_pinned_to_this_version_runs_in_process_with_no_uv_call` | 56 | 1342 | 23 | 0.8 | 1fe146d | greedy rank 23: +70 stmts |
| KEEP | light | `test_owner_request_hook.py::test_blocks_a_needless_ask_even_when_an_open_need_matches` | 0 | 1379 | 16 | 2.5 | 3b2e7d7 | greedy rank 16: +101 stmts |
| KEEP | light | `test_owner_request_hook.py::test_fails_when_claude_fails` | 2 | 1316 | 67 | 2.9 | 3b2e7d7 | greedy rank 67: +11 stmts |
| KEEP | light | `test_pm.py::test_a_decision_recorded_for_a_no_decision_need_marks_it_answered` | 9 | 2329 | 37 | 0.8 | 7052c96 | greedy rank 37: +37 stmts |
| KEEP | light | `test_pm.py::test_every_store_command_fails_hard_with_the_service_stopped` | 41 | 1874 | 18 | 1.9 | 89a81c0 | greedy rank 18: +80 stmts |
| KEEP | light | `test_pm.py::test_feedback_about_any_project_goes_to_the_repo_s_one_feedback_doc` | 36 | 2719 | 22 | 1.4 | 963fb75 | greedy rank 22: +71 stmts |
| KEEP | light | `test_pm.py::test_need_edit_rewrites_an_open_need_s_body_until_the_owner_replies` | 73 | 2059 | 5 | 2.0 | b61f12d | greedy rank 5: +338 stmts |
| KEEP | light | `test_pm.py::test_new_records_feedback_projects_and_moves_write_what_they_say` | 19 | 2700 | 8 | 1.3 | 760b3bd | greedy rank 8: +173 stmts |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[action need-no-text]` | 1 | 1245 | 198 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[action need-text]` | 3 | 1496 | 134 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[doc new-no-text]` | 1 | 1244 | 195 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[feedback add-no-text]` | 1 | 1223 | 200 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[project open-no-text]` | 1 | 1243 | 199 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[sprint open-no-text]` | 1 | 1250 | 197 | 0.5 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[task move-no-text]` | 2 | 1440 | 158 | 0.5 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_pm_check_and_commit_refuse_a_record_link_to_nothing` | 10 | 1341 | 75 | 0.7 | da3423c | 10 stmts no other test runs **BUG GUARD** |
| KEEP | light | `test_pm.py::test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does` | 6 | 2075 | 24 | 0.8 | f16edb8 | greedy rank 24: +65 stmts |
| KEEP | light | `test_pm.py::test_reply_read_prints_each_undelivered_reply_and_merge_once` | 42 | 2318 | 12 | 1.9 | d170201 | greedy rank 12: +128 stmts |
| KEEP | light | `test_pm.py::test_sprint_and_project_close_once_their_reports_are_written_and_their_work_closed` | 26 | 2104 | 49 | 1.7 | 760b3bd | greedy rank 49: +26 stmts |
| KEEP | light | `test_pm.py::test_sprint_edit_renames_the_sprint_in_both_stores_and_records_why` | 40 | 2655 | 13 | 1.2 | b61f12d | greedy rank 13: +122 stmts |
| KEEP | light | `test_pm.py::test_sprint_move_takes_the_sprint_whole_and_a_rerun_finishes_a_move_cut_short` | 63 | 3132 | 1 | 3.0 | fcae7a7 | greedy rank 1: +3132 stmts |
| KEEP | light | `test_pm.py::test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_worktree` | 4 | 1485 | 56 | 1.2 | f4d39e3 | greedy rank 56: +18 stmts |
| KEEP | light | `test_pm.py::test_task_close_refuses_a_task_another_live_session_holds` | 1 | 1997 | 65 | 1.1 | d0e08f3 | greedy rank 65: +12 stmts |
| KEEP | light | `test_pm.py::test_task_close_resolves_another_repos_commit_or_a_pr_with_gh` | 26 | 2012 | 26 | 2.8 | d0e08f3 | greedy rank 26: +61 stmts |
| KEEP | light | `test_pm.py::test_text_file_dash_refuses_a_socket_at_once` | 1 | 1205 | 194 | 0.6 | c661956 | bug guard: pm hung reading an open stdin **BUG GUARD** |
| KEEP | light | `test_pm.py::test_text_file_reads_a_file_and_refuses_with_text` | 12 | 2024 | 48 | 0.8 | c661956 | greedy rank 48: +26 stmts **BUG GUARD** |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[2026-10-10 chat: cannot tell without the blocked text]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[2026-10-10 chat: needs the blocked reply to diagnose]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[2026-10-10 chat: offer conditioned on the owner pasting the reply]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[PR #25 review: a PR waits on review and merge, no actor named, plan for after it]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[a PR waits on the owner's merge, plan for after it, no review open]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[asks again for another session's open need without citing it]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[asks leave to push and open the PR]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[asks to review and merge the open PR, no action]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[asks to review and merge the open PR, review action open]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only PR merge request]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only action at the end of a long report]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only apply a setting]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only approval as a statement]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only clarifying question]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only design choice]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only merge question beside an unrelated need]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only merge question]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only run a command]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[chat-only scope decision]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[cites by id a review need open on the site, not held by this session]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[describes an answered need]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[describes an open need without asking]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[formal-methods feedback 2026-10-10 03:31: asks again for another session's open needs, citing their ids]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[id cited but no open need]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matched PR review, no id]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matched action, no id]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matched decision, cites its id]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matched decision, paraphrased, no id]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matched merge question about the reviewed PR]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matching need belongs to another session]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[matching review belongs to another session]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[next-step suggestion nothing waits on]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[offer to do more]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[offer to merge a PR]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[offer to open the PR the sprint close waits on]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[offer to open the PR, matching an open action]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[optional live check offer after done work]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[optional offer]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10 reworded: a PR held until the agent merges another]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10 reworded: sprints that start after the agent merges PRs]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10 reworded: still waiting on reviews and rebases its own subagents do]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10 reworded: the agent names itself as the merger in the heading]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10 reworded: the agent reviews and merges the PR itself]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10: a PR held until another merges, no actor named]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10: merge order as a terse imperative heading, no actor named]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10: review-then-merge of a PR, no actor named]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10: sprints that start after PRs merge, no actor named]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm 2026-10-10: still waiting on reviews and rebases, no actor named]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-07 03:43: a plan that waits on the agent's own subagents]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-08 00:01: how-to answer to the owner's own question]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-08 02:45: still waiting on the agent's own subagent]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-08 21:47: a fix takes effect once the installed pm has it]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-08 21:47: the main checkout is behind]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[pm feedback 2026-10-10 03:56: reports a review need held in another clone]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[question inside a code block]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[reports a question it asked with AskUserQuestion]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[reports a result it found itself, no question]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[reports another session's open needs without asking them again]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[same review left pending without citing a need]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[status and plan quoting example requests]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[status report]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[summary of done work]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | live | `test_owner_request_prompt_live.py::test_owner_request_judge[two requests, one matched]` | - | - |  | 0.0 | 3b2e7d7 | live eval of the judge (skipped in CI; runs only with PM_LIVE_TESTS) [coverage not measured] |
| KEEP | script | `test_changelog.py::test_a_change_that_ships_needs_an_entry_file` | - | - |  | 0.0 | af0a631 | PR check rule [coverage not measured] |
| KEEP | script | `test_changelog.py::test_release_wraps_the_summary_so_no_line_starts_a_list_item` | - | - |  | 0.0 | af0a631 | a real rendering bug class [coverage not measured] |
| KEEP | script | `test_changelog.py::test_release_writes_x_s_section_above_the_newest_with_its_link` | - | - |  | 0.0 | af0a631 | release section writer [coverage not measured] |
| KEEP | script | `test_changelog.py::test_the_real_changelog_and_entries_pass` | - | - |  | 0.0 | af0a631 | guards CHANGELOG.md itself [coverage not measured] |
| KEEP | script | `test_changelog.py::test_two_branches_that_each_add_an_entry_merge_without_conflict_and_both_reach_the_release` | - | - |  | 0.4 | af0a631 | the design goal of changelog.d [coverage not measured] |
| KEEP | script | `test_ci.py::test_go_test_but_work_fails_when_go_list_fails_and_tests_nothing` | - | - |  | 0.0 | da3423c | bug guard da3423c [coverage not measured] **BUG GUARD** |
| KEEP | script | `test_ci.py::test_the_release_build_runs_on_every_change_to_what_pm_version_runs` | - | - |  | 0.0 | da3423c | bug guard da3423c [coverage not measured] **BUG GUARD** |
| KEEP | script | `test_release.py::test_install_sh_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone[GH_TOKEN]` | - | - |  | 1.0 | e1b0585 | token never sent to storage host [coverage not measured] |
| KEEP | script | `test_release.py::test_install_sh_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone[gh auth token]` | - | - |  | 1.0 | e1b0585 | token never sent to storage host [coverage not measured] |
| KEEP | script | `test_release.py::test_install_sh_installs_the_checked_binary_into_the_bin_dir` | - | - |  | 0.5 | 279bdbc | install.sh happy path [coverage not measured] |
| KEEP | script | `test_release.py::test_install_sh_refuses_a_checksum_mismatch_and_installs_nothing` | - | - |  | 0.5 | 279bdbc | sha contract [coverage not measured] |
| MERGE | go | `cli.TestCheckFailsWithoutTheServiceOrAWorkStore` | 0 | 381 |  | 0.0 | 4db4259 | work-store invariants; 0 unique stmts -> into work.TestAServiceOnAnotherVersionIsRefused (J=0.44) |
| MERGE | go | `cli.TestCheckReadsTheWorkStoreTheServiceHolds` | 0 | 938 |  | 0.3 | 4db4259 | work-store invariants; 0 unique stmts -> into work.TestAWriteRacingOneOutsideTheLockFailsHard (J=0.38) |
| MERGE | go | `cli.TestCheckRendersEveryPageWithTheWorkStoresItems` | 1 | 520 | 187 | 0.0 | aadc066 | work-store invariants; 1 unique stmts -> into cli.TestLoadNeverPassesARecordSetMissingAFileGitRewrote (J=0.44) |
| MERGE | go | `cli.TestFirstSentenceEndsAtAUnicodeSpace` | 0 | 59 |  | 0.0 | b21bfcb | bug guard: frame heading/first sentence split on ASCII not Python \s -> into cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.43) **BUG GUARD** |
| MERGE | go | `cli.TestParseSectionsTakesUnicodeSpaceAfterAHeading` | 0 | 70 |  | 0.0 | b21bfcb | bug guard: frame heading/first sentence split on ASCII not Python \s -> into cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.39) **BUG GUARD** |
| MERGE | go | `cli.TestRecordSectionCutsAtTheNextHeadingOfItsLevelAndRefusesAnUnknownName` | 6 | 101 | 100 | 0.0 | 607b7dd | refusal; 6 unique stmts -> into cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.33) |
| MERGE | go | `hooks.TestAFailedCheckExitsOneWithItsCause` | 5 | 69 | 108 | 0.0 | be62033 | work-store invariants; 5 unique stmts -> into records.TestReadSummariesLeavesOutASummaryGoneWhenRead (J=0.20) |
| MERGE | go | `hooks.TestChunksFitTheCapAndAddUpToTheHead` | 0 | 48 |  | 0.0 | 6463ce2 | named in CLAUDE.md as a guard; 0 unique stmts -> into hooks.TestContextCutsAtALineToTheCapacity (J=0.09) |
| MERGE | go | `hooks.TestParseJudgedRefusesAnAnswerOutOfShape` | 8 | 58 | 83 | 0.0 | be62033 | launcher sha check; 8 unique stmts -> into records.TestReadSummariesLeavesOutASummaryGoneWhenRead (J=0.22) |
| MERGE | go | `launch.TestAFailedCheckIsNotTriedThroughTheAPI` | 0 | 48 |  | 0.0 | 833d3e9 | work-store invariants; 0 unique stmts -> into launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.34) |
| MERGE | go | `launch.TestAGoPinThatCannotBeDownloadedFailsHard` | 0 | 2 |  | 0.0 | 78c7880 | fail-hard row of the launcher table; under-measured (0 unique stmts) -> into launch.TestAGoPinIsDownloadedOnceThenExecdWithTheMarks (J=1.00) |
| MERGE | go | `launch.TestAKeptBinaryThatCannotRunFailsHard` | 0 | 0 |  | 0.0 | c281f70 | fail-hard row of the launcher table; under-measured (0 unique stmts) -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) |
| MERGE | go | `launch.TestAPythonPinFailsHardNamingTheFix` | 0 | 0 |  | 0.0 | b0c8166 | refusal row of the launcher table; under-measured (0 unique stmts) -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) |
| MERGE | go | `launch.TestDownloadFailsHardAndInstallsNothing` | 2 | 103 | 89 | 0.0 | 78c7880 | fail-hard row of the launcher table (2 unique stmts) -> into launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.41) |
| MERGE | go | `launch.TestDownloadKeepsTheCheckedBinaryAndItsSha` | 0 | 81 |  | 0.0 | 78c7880 | work-store invariants; 0 unique stmts -> into launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.66) |
| MERGE | go | `service.TestAMergeNotOnMainYetIsNotReported` | 5 | 170 | 109 | 0.0 | 2cfae09 | sync/merge rules; 5 unique stmts -> into service.TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt (J=0.59) |
| MERGE | go | `service.TestInstallHoldsTheClonesInstallLock` | 0 | 121 |  | 0.0 | 2cfae09 | write lock / concurrency; 0 unique stmts -> into service.TestUninstallStopsAndRemovesTheUnit (J=0.88) |
| MERGE | go | `service.TestTheServiceStopsOnceThePinMoves` | 0 | 131 |  | 0.0 | 2cfae09 | bug guard a43cd1c; harness test_service_stops_once_the_pin_moves also holds it (0 unique stmts) -> into service.TestServesImageFilesFromTheRecordsStoreAndNothingOutsideIt (J=0.69) |
| MERGE | go | `service.TestUnitFilesEqualTheFrozenOnes` | 0 | 58 |  | 0.0 | 2d1a4db | frozen unit files: fold into the unit-file golden (0 unique stmts) -> into service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.20) |
| MERGE | go | `site.TestMermaidLoadsOneExactVersionAtNaturalWidth` | 0 | 6 |  | 0.0 | 214d486 | named in CLAUDE.md as a guard; 0 unique stmts -> into site.TestFillRepliesAndStatus (J=0.19) |
| MERGE | go | `store.TestFindChecksTheStore` | 1 | 43 | 166 | 0.0 | bc13f4d | work-store invariants; 1 unique stmts -> into store.TestCommitCommitsOnlyThePathsNamed (J=0.27) |
| MERGE | go | `store.TestLockIsExclusive` | 0 | 5 |  | 0.1 | bc13f4d | write lock / concurrency; 0 unique stmts -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) |
| MERGE | go | `sync.TestASecondRunSkipsWhileOneHoldsTheLock` | 1 | 60 | 176 | 0.0 | 2f6d3d8 | write lock / concurrency; 1 unique stmts -> into sync.TestARunRecordsEachStepAndFlagsAFailure (J=0.47) |
| MERGE | go | `work.TestAHungFetchEndsAtTheSyncsBound` | 0 | 269 | 170 | 2.2 | 2848f0c | bug guard: gc held the operation slot, unbounded ops -> into install.TestSetupWorkCreatesPushesAndClones (J=0.45) **BUG GUARD** |
| MERGE | go | `work.TestALongWriteIsNotStarvedByShortOnes` | 0 | 489 |  | 0.8 | feb1728 | bug guard: unfair write lock; merges cut short -> into work.TestPmLockThroughTheHost (J=0.89) **BUG GUARD** |
| MERGE | go | `work.TestASecondHostOnAStoreFailsHard` | 6 | 205 | 93 | 0.4 | 4db4259 | into the access-path test (6 unique stmts) -> into install.TestSetupWorkCreatesPushesAndClones (J=0.41) |
| MERGE | go | `work.TestAWriteLockHolderWhoseConnectionIsGoneIsTakenOver` | 0 | 25 |  | 0.1 | 585ea6e | bug guard: write lock holder/wait -> into work.TestAWriteLockWaitEndsAtItsBoundOrItsStatement (J=0.39) **BUG GUARD** |
| MERGE | go | `work.TestAWriteThatBreaksAnInvariantLandsNothing` | 5 | 627 | 111 | 0.3 | 55e396c | work-store invariants; 5 unique stmts -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.83) |
| MERGE | go | `work.TestBenchLoad` | 0 | 0 |  | 0.0 | dabffb6 | bug guard: write retries bound from benchmark -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) **BUG GUARD** |
| MERGE | go | `work.TestBenchWriters` | 0 | 0 |  | 0.0 | dabffb6 | bug guard: write retries bound from benchmark -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) **BUG GUARD** |
| MERGE | go | `work.TestBlockedAndItsReasons` | 0 | 27 |  | 0.0 | fb42de7 | into one ready/blocked table test (0 unique stmts) -> into work.TestFromExportRefusesALineThatIsNotAnItem (J=0.06) |
| MERGE | go | `work.TestCheckIDTakesThePagesFormat` | 3 | 19 | 124 | 0.0 | fb42de7 | work-store invariants; 3 unique stmts -> into work.TestFromExportRefusesALineThatIsNotAnItem (J=0.09) |
| MERGE | go | `work.TestClosingTheBlockerFreesTheTasks` | 0 | 33 |  | 0.0 | fb42de7 | into one ready/blocked table test (0 unique stmts) -> into work.TestFromExportRefusesALineThatIsNotAnItem (J=0.06) |
| MERGE | go | `work.TestCreateConcurrentChildMintsGetDistinctIDs` | 0 | 792 |  | 3.8 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestCreatePushTimeout (J=0.97) |
| MERGE | go | `work.TestCreateFailsAfterThreeRejectedPushes` | 1 | 790 | 177 | 3.3 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestCreatePushTimeout (J=0.98) |
| MERGE | go | `work.TestCreateMintsIdsAndSprintNumbers` | 1 | 587 | 171 | 0.3 | 55e396c | sync/merge rules; 1 unique stmts -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.89) |
| MERGE | go | `work.TestCreateRacingALocalWriter` | 0 | 795 |  | 3.8 | 102ff18 | bug guard: a fast-forward raced a commit -> into work.TestCreatePushTimeout (J=0.88) **BUG GUARD** |
| MERGE | go | `work.TestCreateRefusesWithTheRemoteUnreachable` | 1 | 756 | 183 | 9.7 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestCreatePushTimeout (J=0.93) |
| MERGE | go | `work.TestCreateWhosePushLandsAfterItsCheckMakesTheItemOnce` | 0 | 789 |  | 3.1 | e3bc836 | bug guard: a create minted twice after an unknown push -> into work.TestCreatePushTimeout (J=0.99) **BUG GUARD** |
| MERGE | go | `work.TestCreateWritesThroughToALaterOpen` | 0 | 514 |  | 0.3 | 55e396c | sync/merge rules; 0 unique stmts -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.83) |
| MERGE | go | `work.TestCyclesIncludeAncestors` | 0 | 136 |  | 0.0 | fb42de7 | into one ready/blocked table test (0 unique stmts) -> into work.TestCheckRefusesEachImpossibleState (J=0.64) |
| MERGE | go | `work.TestEightWritersTwentyWritesEachAllLand` | 0 | 555 |  | 2.0 | 4db4259 | race test (RACE_TESTS, runs under -race; coverage cannot see races); 0 unique -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.84) |
| MERGE | go | `work.TestGCRacingSyncsAndCreatesFinishesPromptly` | 0 | 681 |  | 2.4 | 2848f0c | bug guard: gc held the operation slot, unbounded ops -> into work.TestAnOperationOutlastingItsBoundFreesTheSlotAndAWaiterGivesUp (J=0.92) **BUG GUARD** |
| MERGE | go | `work.TestImportGoesIntoAnEmptyStoreOnly` | 0 | 494 |  | 0.6 | 55e396c | refusal row into TestImportFailsHardOnWhatItCannotMap (0 unique stmts) -> into work.TestClaimIsACompareAndSet (J=0.71) |
| MERGE | go | `work.TestMergeAConflictNoRuleSettlesFailsHard` | 1 | 851 | 182 | 2.9 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestCreatePushTimeout (J=0.87) |
| MERGE | go | `work.TestMergeAFieldOneSideChangedTakesThatSide` | 0 | 46 |  | 0.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.48) |
| MERGE | go | `work.TestMergeASprintMovedOnOneSideKeepsTheMove` | 1 | 51 | 184 | 0.0 | fcae7a7 | sync/merge rules; 1 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.53) |
| MERGE | go | `work.TestMergeAddAddFailsHard` | 3 | 798 | 76 | 2.5 | 4860f5c | sync/merge rules; 3 unique stmts -> into work.TestCreatePushTimeout (J=0.93) |
| MERGE | go | `work.TestMergeCloseBeatsClaimOnACleanMerge` | 0 | 839 |  | 4.7 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestCreatePushTimeout (J=0.90) |
| MERGE | go | `work.TestMergeCloseFieldsFromTheEarlierClose` | 6 | 46 | 88 | 0.0 | 4860f5c | sync/merge rules; 6 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.41) |
| MERGE | go | `work.TestMergeCommentsUnionByID` | 0 | 875 |  | 5.7 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestSyncTwoClonesConcurrentEditsMergePerTheTable (J=0.89) |
| MERGE | go | `work.TestMergeCycleAcrossClonesFailsHard` | 1 | 823 | 174 | 2.6 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestCreatePushTimeout (J=0.91) |
| MERGE | go | `work.TestMergeDeliveredTheMax` | 5 | 80 | 104 | 0.0 | 4860f5c | sync/merge rules; 5 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.68) |
| MERGE | go | `work.TestMergeHolderAClaimBeatsARelease` | 1 | 49 | 172 | 0.0 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.45) |
| MERGE | go | `work.TestMergeHolderCloseBeatsClaim` | 0 | 42 |  | 0.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.42) |
| MERGE | go | `work.TestMergeHolderTheLaterClaimWinsAndIsReported` | 1 | 54 | 186 | 0.0 | 4860f5c | sync/merge rules; 1 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.45) |
| MERGE | go | `work.TestMergeLabelsAndBlockersPerEntry` | 0 | 889 |  | 6.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestSyncTwoClonesConcurrentEditsMergePerTheTable (J=0.90) |
| MERGE | go | `work.TestMergeResolutionNotNullThenLaterUpdatedAt` | 6 | 53 | 97 | 0.0 | 4860f5c | sync/merge rules; 6 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.44) |
| MERGE | go | `work.TestMergeStartedAtTheEarliest` | 0 | 46 |  | 0.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.47) |
| MERGE | go | `work.TestMergeStatusClosedWins` | 0 | 42 |  | 0.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.42) |
| MERGE | go | `work.TestMergeTypeNeverChangesAndNumberChangedOnBothSidesFails` | 2 | 16 | 154 | 0.0 | fcae7a7 | sync/merge rules; 2 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.12) |
| MERGE | go | `work.TestMergeUpdatedAtTheLatest` | 0 | 44 |  | 0.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestMergeOtherScalarsTheLaterUpdatedAt (J=0.47) |
| MERGE | go | `work.TestModuleGoFilesLeaveOutNestedModulesAndWorktrees` | 0 | 0 |  | 0.0 | da3423c | bug guard: review follow-ups (CI, uninstall, lock) -> into service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) **BUG GUARD** |
| MERGE | go | `work.TestMoveSprintAcrossClonesKeepsTheOtherClonesWrites` | 2 | 958 | 132 | 6.8 | fcae7a7 | sync/merge rules; 2 unique stmts -> into work.TestSyncTwoClonesConcurrentEditsMergePerTheTable (J=0.91) |
| MERGE | go | `work.TestMoveSprintRefuses` | 7 | 592 | 87 | 0.2 | fcae7a7 | refusal; 7 unique stmts -> into work.TestClaimIsACompareAndSet (J=0.85) |
| MERGE | go | `work.TestNewRootMintsFourBase36CharactersAndALongerOneOnAHit` | 2 | 24 | 153 | 0.0 | fb42de7 | sync/merge rules; 2 unique stmts -> into work.TestFromExportRefusesALineThatIsNotAnItem (J=0.09) |
| MERGE | go | `work.TestNextChildIsOneAboveTheHighestChildNumberEverMinted` | 0 | 11 |  | 0.0 | fb42de7 | sync/merge rules; 0 unique stmts -> into cli.TestTaskRelease (J=0.02) |
| MERGE | go | `work.TestOppositeDepAddsAtOnceExactlyOneLands` | 0 | 557 |  | 0.4 | 4db4259 | race test (RACE_TESTS, runs under -race; coverage cannot see races); 0 unique -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.78) |
| MERGE | go | `work.TestPullOntoARemoteHeadThatFailsItsCheckMovesNothing` | 2 | 771 | 148 | 3.3 | 4db4259 | sync/merge rules; 2 unique stmts -> into work.TestCreatePushTimeout (J=0.92) |
| MERGE | go | `work.TestPullRacingLocalWriters` | 0 | 846 |  | 9.4 | 102ff18 | bug guard: a fast-forward raced a commit -> into work.TestCreatePushTimeout (J=0.91) **BUG GUARD** |
| MERGE | go | `work.TestReadsFailHardOnARowNoWriteMakes` | 2 | 600 | 127 | 0.3 | 55e396c | fail-hard read into TestCheckRefusesEachImpossibleState (2 unique stmts) -> into work.TestClaimIsACompareAndSet (J=0.93) |
| MERGE | go | `work.TestReadyTasksInTheirOrder` | 1 | 72 | 140 | 0.0 | fb42de7 | into one ready/blocked table test (1 unique stmts) -> into work.TestCheckRefusesEachImpossibleState (J=0.12) |
| MERGE | go | `work.TestSyncContextStopsOnceItsContextIsDone` | 0 | 622 |  | 0.5 | 8d68f2f | bug guard: unbounded service sync; remote attached over a store -> into work.TestCommentsKeepTheirOrderWithinASecond (J=0.80) **BUG GUARD** |
| MERGE | go | `work.TestSyncPushesUnderPmsOwnRefOnly` | 0 | 743 |  | 1.0 | 4860f5c | sync/merge rules; 0 unique stmts -> into work.TestCreatePushTimeout (J=0.93) |
| MERGE | go | `work.TestSyncWithoutARemoteFails` | 2 | 219 | 141 | 0.2 | 4860f5c | sync/merge rules; 2 unique stmts -> into install.TestSetupWorkCreatesPushesAndClones (J=0.43) |
| MERGE | go | `work.TestTheHostRefusesANewerSchemaAndAClientAMissingStore` | 4 | 211 | 118 | 0.3 | 4db4259 | refusal; 4 unique stmts -> into install.TestSetupWorkCreatesPushesAndClones (J=0.40) |
| MERGE | go | `work.TestTheHostsFetchLeavesNoMaintenanceRunning` | 0 | 737 |  | 1.4 | da3423c | bug guard: review follow-ups (CI, uninstall, lock) -> into work.TestCreatePushTimeout (J=0.92) **BUG GUARD** |
| MERGE | go | `work.TestTheWriteLockIsFIFO` | 0 | 28 |  | 0.0 | 585ea6e | bug guard: write lock holder/wait -> into work.TestASecondLockOnAConnectionAndAnUnlockWithoutItAreRefused (J=0.57) **BUG GUARD** |
| MERGE | go | `work.TestTwoClonesMigratingOnTheirOwnMerge` | 0 | 645 |  | 4.3 | 102ff18 | bug guard: a fast-forward raced a commit -> into work.TestCreatePushTimeout (J=0.69) **BUG GUARD** |
| MERGE | integration | `test_clean.py::test_clean_keeps_a_branch_not_on_main_when_gh_cannot_check_a_squash_merge` | - | - |  | 0.0 | 0f65958 | into the clean table [coverage not measured] |
| MERGE | integration | `test_clean.py::test_clean_keeps_a_worktree_a_live_process_locks_and_removes_one_whose_lock_is_stale` | - | - |  | 0.0 | 0f65958 | into the clean keep/remove table as rows [coverage not measured] |
| MERGE | integration | `test_clean.py::test_clean_keeps_a_worktree_a_live_session_used_and_the_worktree_it_runs_in` | - | - |  | 0.0 | 0f65958 | into the clean table [coverage not measured] |
| MERGE | integration | `test_hooks.py::test_session_start_sets_up_a_worktree_post_checkout_skipped[branch-tracks-a-copy]` | - | - |  | 0.0 | f23cf6c | two params into one [coverage not measured] |
| MERGE | integration | `test_hooks.py::test_session_start_sets_up_a_worktree_post_checkout_skipped[no-records]` | - | - |  | 0.0 | f23cf6c | two params into one [coverage not measured] |
| MERGE | integration | `test_init.py::test_init_refuses_before_writing_anything` | - | - |  | 0.0 | 5da0808 | into the held-port refusal (both: refuse, write nothing) [coverage not measured] |
| MERGE | integration | `test_launch.py::test_upgrade_to_a_go_version_launches_its_release_binary` | - | - |  | 0.0 | 93cd48e | into the Python-pin upgrade test [coverage not measured] |
| MERGE | integration | `test_lifecycle.py::test_a_beads_file_is_no_beads_for_doctor_and_upgrade` | - | - |  | 0.0 | a5d7580 | Go install.TestABeadsFileIsNoBeads covers it [coverage not measured] **BUG GUARD** |
| MERGE | integration | `test_lifecycle.py::test_doctor_reports_each_changed_clone_setup` | - | - |  | 0.0 | cb0c3cd | into doctor/upgrade restores test [coverage not measured] |
| MERGE | integration | `test_lifecycle.py::test_init_leaves_an_installed_repos_files_alone_and_doctor_names_upgrade` | - | - |  | 0.0 | 7f7503c | into keep_what_is_not_pms [coverage not measured] |
| MERGE | integration | `test_lifecycle.py::test_uninstall_disables_an_enabled_unit_that_is_not_running` | - | - |  | 0.0 | da3423c | into uninstall_removes_pms_parts [coverage not measured] **BUG GUARD** |
| MERGE | integration | `test_lifecycle.py::test_uninstall_keeps_other_clones_roots` | - | - |  | 0.0 | 5da0808 | into uninstall_removes_pms_parts [coverage not measured] |
| MERGE | integration | `test_merge_ready.py::test_a_closed_pr_or_one_whose_head_moved_is_refused` | - | - |  | 0.0 | af0a631 | merge-ready refusals into one table [coverage not measured] |
| MERGE | integration | `test_merge_ready.py::test_a_pr_whose_checks_did_not_all_pass_is_refused_naming_each` | - | - |  | 0.0 | af0a631 | merge-ready refusals into one table [coverage not measured] |
| MERGE | integration | `test_merge_ready.py::test_a_pr_whose_head_contains_main_and_whose_checks_passed_is_ready` | - | - |  | 0.0 | af0a631 | happy row of the same table [coverage not measured] |
| MERGE | integration | `test_merge_ready.py::test_the_workflows_every_pr_runs_must_each_have_a_check_on_the_head` | - | - |  | 0.0 | af0a631 | merge-ready refusals table [coverage not measured] |
| MERGE | integration | `test_pm.py::test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read` | - | - |  | 0.0 | b24b04e | into every_reply_is_pushed [coverage not measured] |
| MERGE | integration | `test_pm.py::test_a_reviewed_prs_merge_is_pushed_into_the_session_once` | - | - |  | 0.0 | b24b04e | Go service.TestAMergeToMainIsStoredAndPushedOnce [coverage not measured] |
| MERGE | integration | `test_pm.py::test_push_rebases_onto_a_moved_remote` | - | - |  | 0.0 | b24b04e | Go sync.TestPushRecordsPushesRebasesAndReports [coverage not measured] |
| MERGE | integration | `test_pm.py::test_reply_on_a_card_is_stored_on_its_issue_as_one_comment` | - | - |  | 0.0 | b24b04e | Go service.TestAReplyIsSpooledStoredOnce... [coverage not measured] |
| MERGE | integration | `test_pm.py::test_sprint_close_merged_stamps_an_agent_s_merge_on_main` | - | - |  | 0.0 | b61f12d | into sprint_closes_after_its_pr_merges [coverage not measured] |
| MERGE | integration | `test_pm.py::test_sprints_list_in_natural_id_order_on_the_overview_the_project_page_and_pm_show` | - | - |  | 0.0 | 6ae6bdd | bug guard 6ae6bdd; fold into site golden fixture [coverage not measured] **BUG GUARD** |
| MERGE | light | `test_config.py::test_every_command_fails_on_another_pinned_version[args0]` | 0 | 1190 | 202 | 0.6 | f0eb60e | one param kept for the parametrized contract (4 params) -> into test_text_file_dash_refuses_a_socket_at_once (J=0.98) |
| MERGE | light | `test_config.py::test_every_command_fails_without_a_config[args0]` | 0 | 1190 |  | 0.6 | f0eb60e | one param kept for the parametrized contract (4 params) -> into test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| MERGE | light | `test_hooks.py::test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[False-False]` | 0 | 1372 |  | 0.6 | 2911a04 | bug guard: stop hooks blocked what was not the agent's to fix -> into test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[Tr (J=0.98) **BUG GUARD** |
| MERGE | light | `test_hooks.py::test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[False-True]` | 0 | 1391 |  | 0.6 | 2911a04 | bug guard: stop hooks blocked what was not the agent's to fix -> into test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[Tr (J=1.00) **BUG GUARD** |
| MERGE | light | `test_hooks.py::test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[True-False]` | 0 | 1389 |  | 0.6 | 2911a04 | bug guard: stop hooks blocked what was not the agent's to fix -> into test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[Tr (J=1.00) **BUG GUARD** |
| MERGE | light | `test_launch.py::test_a_go_pin_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone[GH_TOKEN]` | 0 | 1210 |  | 1.6 | e1b0585 | one param kept for the parametrized contract (2 params) -> into test_text_file_dash_refuses_a_socket_at_once (J=0.96) |
| MERGE | light | `test_launch.py::test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing[unlisted]` | 0 | 1249 |  | 1.0 | 93cd48e | one param kept for the parametrized contract (4 params) -> into test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.96) |
| MERGE | light | `test_launch.py::test_a_go_release_that_differs_from_the_kept_sha256_fails_hard` | 0 | 1256 |  | 1.1 | 93cd48e | install.sh sha check; 0 unique stmts -> into test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.95) |
| MERGE | light | `test_launch.py::test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing[no-release]` | 0 | 1291 |  | 1.6 | e1b0585 | one param kept for the parametrized contract (3 params) -> into test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.99) |
| MERGE | light | `test_owner_request_hook.py::test_blocks_a_request_matching_no_open_need_of_this_session[repo-demo.1.5]` | 0 | 1380 |  | 2.5 | 3b2e7d7 | one param kept for the parametrized contract (2 params) -> into test_blocks_a_needless_ask_even_when_an_open_need_matches (J=0.99) |
| MERGE | light | `test_owner_request_hook.py::test_passes_without_reading_anything[event1]` | 0 | 1251 |  | 2.9 | 3b2e7d7 | one param kept for the parametrized contract (2 params) -> into test_fails_when_claude_fails (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_add_refuses_a_malformed_part_and_writes_nothing[flags0-the following arguments are required: --reason]` | 0 | 1258 | 163 | 0.6 | 6c2b184 | refusal text; 0 unique stmts -> into test_text_file_dash_refuses_a_socket_at_once (J=0.92) |
| MERGE | light | `test_pm.py::test_decision_add_refuses_a_malformed_part_and_writes_nothing[flags1---decision has more than one line]` | 2 | 1420 | 162 | 0.6 | 6c2b184 | refusal text; 2 unique stmts -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.93) |
| MERGE | light | `test_pm.py::test_decision_add_refuses_a_malformed_part_and_writes_nothing[flags2---reason is empty]` | 1 | 1423 | 161 | 0.6 | 6c2b184 | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.93) |
| MERGE | light | `test_pm.py::test_decision_add_refuses_a_malformed_part_and_writes_nothing[flags3---reason starts with ':::']` | 1 | 1423 | 206 | 0.6 |  | one param kept for the parametrized contract (1 params) -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.93) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags0-the following arguments are required: --question]` | 0 | 1253 |  | 0.6 | bc7d31d | refusal text; 0 unique stmts -> into test_text_file_dash_refuses_a_socket_at_once (J=0.92) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags1-give at least two --option flags]` | 1 | 1294 | 210 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags10-in fact 3 has 26 words; the limit is 25]` | 1 | 1350 | 212 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.90) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags2-option a has no cost; add --cost a]` | 1 | 1293 | 211 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags3---cost c names no option; the labels are a, b]` | 1 | 1291 | 213 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags4-option a has two --cost flags]` | 1 | 1291 | 208 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags5-two options have the label a]` | 1 | 1284 | 207 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.94) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags6---default z names no option; the labels are a, b]` | 1 | 1297 | 214 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.93) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags7---fact has more than one line]` | 2 | 1278 | 135 | 0.6 | bc7d31d | refusal text; 2 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.95) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags8-give exactly one --default]` | 0 | 1256 |  | 0.6 | bc7d31d | refusal text; 0 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.96) |
| MERGE | light | `test_pm.py::test_decision_need_refuses_a_malformed_part_and_writes_nothing[flags9-raise it with the review form]` | 1 | 1254 | 209 | 0.6 | bc7d31d | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[feedback add-no-text] (J=0.97) |
| MERGE | light | `test_pm.py::test_feedback_add_refuses_to_overwrite_another_record_at_the_feedback_doc_s_path` | 1 | 1426 | 204 | 0.7 | 963fb75 | refusal text; 1 unique stmts -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.92) |
| MERGE | light | `test_pm.py::test_finding_add_refuses_a_text_that_starts_with_two_dashes_and_two_texts` | 9 | 1261 | 81 | 0.7 | cee2d7d | refusal text; 9 unique stmts -> into test_no_command_waits_on_an_open_stdin[project open-no-text] (J=0.97) |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[decision close-no-text]` | 0 | 1413 | 203 | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.95) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[decision close-text]` | 0 | 1925 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_task_close_refuses_a_task_another_live_session_holds (J=0.89) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[doc new-text]` | 0 | 1678 |  | 0.5 | c661956 | bug guard: pm hung reading an open stdin -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.81) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[feedback add-text]` | 0 | 1686 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.79) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[project open-text]` | 0 | 1821 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_task_close_refuses_a_task_another_live_session_holds (J=0.77) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[sprint open-text]` | 0 | 2054 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_text_file_reads_a_file_and_refuses_with_text (J=0.84) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[task add-no-text]` | 0 | 1932 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_text_file_reads_a_file_and_refuses_with_text (J=0.95) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[task add-text]` | 0 | 1932 |  | 0.5 | c661956 | bug guard: pm hung reading an open stdin -> into test_text_file_reads_a_file_and_refuses_with_text (J=0.95) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_no_command_waits_on_an_open_stdin[task move-text]` | 0 | 2056 |  | 0.6 | c661956 | bug guard: pm hung reading an open stdin -> into test_task_close_refuses_a_task_another_live_session_holds (J=0.88) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_pm_show_project_prints_one_project_and_refuses_what_it_cannot` | 9 | 1729 | 80 | 1.0 | e6b0bf3 | refusal text; 9 unique stmts -> into test_no_command_waits_on_an_open_stdin[task move-no-text] (J=0.74) |
| MERGE | light | `test_pm.py::test_sprint_move_in_one_go_and_its_refusals` | 8 | 2332 | 79 | 2.6 | fcae7a7 | refusal text; 8 unique stmts -> into test_sprint_and_project_close_once_their_reports_are_written_and_their (J=0.87) |
| MERGE | light | `test_pm.py::test_sprint_move_rerun_on_a_clone_whose_records_lag_writes_the_same_records_step` | 0 | 2151 |  | 2.0 | da3423c | bug guard: review follow-ups (CI, uninstall, lock) -> into test_task_close_refuses_a_task_another_live_session_holds (J=0.84) **BUG GUARD** |
| MERGE | light | `test_pm.py::test_task_claim_refuses_a_task_another_live_session_holds` | 2 | 1507 | 157 | 1.2 | b24b04e | refusal text; 2 unique stmts -> into test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_wo (J=0.97) |
| MERGE | light | `test_pm.py::test_text_file_dash_reads_a_heredoc` | 0 | 1945 |  | 0.5 | c661956 | bug guard: pm hung reading an open stdin -> into test_text_file_reads_a_file_and_refuses_with_text (J=0.96) **BUG GUARD** |
| MERGE | script | `test_changelog.py::test_a_pre_release_takes_its_version_s_section_when_there_is_one` | - | - |  | 0.0 | af0a631 | into the notes test [coverage not measured] |
| MERGE | script | `test_changelog.py::test_a_release_with_no_section_has_no_notes` | - | - |  | 0.0 | 34e996b | into the notes test [coverage not measured] |
| MERGE | script | `test_changelog.py::test_a_release_without_an_upgrade_guide_is_refused` | - | - |  | 0.0 | 19697ff | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_a_valid_changelog_passes_and_gives_each_release_its_notes_unwrapped` | - | - |  | 0.0 | 34e996b | into the notes test [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_empty_category_or_release_section_is_refused` | - | - |  | 0.0 | 34e996b | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_entry_file_holds_only_categories_and_their_items` | - | - |  | 0.0 | af0a631 | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_unknown_category_is_refused` | - | - |  | 0.0 | 34e996b | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_unreleased_section_in_changelog_md_is_refused_naming_the_entry_files` | - | - |  | 0.0 | af0a631 | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_upgrade_guide_after_another_category_is_refused` | - | - |  | 0.0 | 19697ff | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_an_upgrade_guide_that_is_not_a_numbered_list_is_refused` | - | - |  | 0.0 | 19697ff | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_categories_out_of_order_are_refused` | - | - |  | 0.0 | 34e996b | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_changelog.py::test_entries_assemble_by_category_in_name_order_with_the_guide_s_steps_after_the_two_every_release_has` | - | - |  | 0.0 | af0a631 | into release_writes_x_s_section [coverage not measured] |
| MERGE | script | `test_changelog.py::test_versions_dates_headings_and_links_are_checked` | - | - |  | 0.0 | 34e996b | changelog.py refusals into one parametrized table [coverage not measured] |
| MERGE | script | `test_ci.py::test_go_test_but_work_tests_every_package_but_the_work_store` | - | - |  | 0.0 | da3423c | into the go list failure test [coverage not measured] **BUG GUARD** |
| MERGE | script | `test_release.py::test_install_sh_refuses_a_release_with_no_line_for_this_platform` | - | - |  | 0.5 | 279bdbc | install.sh refusals table [coverage not measured] |
| MERGE | script | `test_release.py::test_install_sh_refuses_a_release_without_this_platforms_asset` | - | - |  | 1.0 | e1b0585 | install.sh refusals table [coverage not measured] |
| MERGE | script | `test_release.py::test_install_sh_that_fails_with_no_token_and_has_none_fails_hard_naming_that_download` | - | - |  | 1.0 | 833d3e9 | install.sh refusals table [coverage not measured] |
| DROP | go | `cli.TestAsksReviewAsPython` | 1 | 59 | 165 | 0.0 | d170201 | 1 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.43) |
| DROP | go | `cli.TestCommentAdd` | 5 | 753 | 102 | 0.5 | fad11ab | 5 unique stmts; covered by others; nearest kept cli.TestTaskAddParentAddsASubTaskUnderAnOpenTask (J=0.91) |
| DROP | go | `cli.TestEveryCommandPmRunsIsListedInHelp` | 0 | 207 |  | 0.0 | cee2d7d | 0 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.19) |
| DROP | go | `cli.TestLive` | 2 | 684 | 136 | 0.4 | fad11ab | 2 unique stmts; covered by others; nearest kept cli.TestTaskRelease (J=0.91) |
| DROP | go | `cli.TestNeedDismiss` | 2 | 709 | 137 | 0.5 | fad11ab | 2 unique stmts; covered by others; nearest kept cli.TestTaskRelease (J=0.94) |
| DROP | go | `cli.TestPlainAsPython` | 5 | 63 | 103 | 0.0 | d170201 | 5 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.41) |
| DROP | go | `cli.TestReplyAdd` | 2 | 760 | 114 | 0.5 | fad11ab | 2 unique stmts; covered by others; nearest kept cli.TestTaskAddParentAddsASubTaskUnderAnOpenTask (J=0.91) |
| DROP | go | `cli.TestSentencesAndWordsAsPython` | 3 | 99 | 123 | 0.0 | d170201 | 3 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.31) |
| DROP | go | `cli.TestShowDumpsIsPythonsIndentOne` | 2 | 90 | 143 | 0.0 | 607b7dd | 2 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.34) |
| DROP | go | `cli.TestShowHeadingsMatchMarkdownIt` | 9 | 75 | 77 | 0.0 | 607b7dd | 9 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.41) |
| DROP | go | `cli.TestStoreCommandHelpAndUsage` | 1 | 685 | 167 | 0.5 | fad11ab | 1 unique stmts; covered by others; nearest kept cli.TestTaskRelease (J=0.94) |
| DROP | go | `cli.TestTaskReadyNone` | 2 | 733 | 138 | 0.5 | fad11ab | 2 unique stmts; covered by others; nearest kept cli.TestTaskReadyListsReadyTasksInOrder (J=0.93) |
| DROP | go | `cli.TestTheCommandsOnceOutsideTheTreeAreInIt` | 0 | 258 |  | 0.0 | cee2d7d | 0 unique stmts; covered by others; nearest kept cli.TestWrapFillPutsALongWordOnItsOwnLine (J=0.16) |
| DROP | go | `hooks.TestABlockNamesTheNeedlessAskThenTheUncoveredRequest` | 0 | 186 | 150 | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept hooks.TestTouchedCountsASubagentsPathOnceItsCallReturns (J=0.16) |
| DROP | go | `hooks.TestAJudgeThatFailsIsNamedWithWhatItSaid` | 0 | 122 |  | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept hooks.TestContextCutsAtALineToTheCapacity (J=0.15) |
| DROP | go | `hooks.TestATimeoutIsNamedAsPythonNamesIt` | 6 | 44 | 95 | 1.1 | 6463ce2 | 6 unique stmts; covered by others; nearest kept hooks.TestContextCutsAtALineToTheCapacity (J=0.31) |
| DROP | go | `hooks.TestCommandsLeaveOutTheMachinery` | 0 | 6 |  | 0.0 | 6463ce2 | 0 unique stmts; covered by others; nearest kept site.TestFillRepliesAndStatus (J=0.03) |
| DROP | go | `hooks.TestDecideReadsNothingAfterABlockOrForABlankReply` | 0 | 56 | 147 | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept records.TestReadSummariesLeavesOutASummaryGoneWhenRead (J=0.22) |
| DROP | go | `hooks.TestJudgePromptListsEachRequestWithItsDescriptionOnOneLine` | 0 | 16 | 156 | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept hooks.TestContextCutsAtALineToTheCapacity (J=0.11) |
| DROP | go | `hooks.TestParseJudgedTakesTheSpanFromTheFirstToTheLastBrace` | 0 | 52 |  | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept records.TestReadSummariesLeavesOutASummaryGoneWhenRead (J=0.24) |
| DROP | go | `hooks.TestQuotedCutsEachQuoteAndTheWhole` | 0 | 8 |  | 0.0 | be62033 | 0 unique stmts; covered by others; nearest kept site.TestFillRepliesAndStatus (J=0.03) |
| DROP | go | `install.TestBeadsBlockClean` | 3 | 45 | 120 | 0.0 | b7cbb0e | 3 unique stmts; covered by others; nearest kept install.TestInstallBinary (J=0.24) |
| DROP | go | `install.TestInstallBinaryReplacesALinkAndLeavesItsTarget` | 0 | 54 |  | 0.1 | b0c8166 | 0 unique stmts; covered by others; nearest kept install.TestInstallBinary (J=0.97) |
| DROP | go | `install.TestPlanInANewRepoWritesNoBeadsPiece` | 0 | 267 |  | 0.0 | b7cbb0e | 0 unique stmts; covered by others; nearest kept install.TestABeadsFileIsNoBeads (J=0.57) |
| DROP | go | `install.TestRetiredPiecesAreReportedAndRemoved` | 5 | 481 | 110 | 0.0 | 6a0bb49 | 5 unique stmts; covered by others; nearest kept install.TestRewriteTakesOutBeadsPieces (J=0.77) |
| DROP | go | `install.TestSetupWorkAttachesAnExistingStore` | 3 | 404 | 129 | 0.5 | 7719888 | 3 unique stmts; covered by others; nearest kept install.TestSetupWorkRefusesAFork (J=0.85) |
| DROP | go | `install.TestSetupWorkWithoutTheRemote` | 3 | 238 | 125 | 0.0 | 7719888 | 3 unique stmts; covered by others; nearest kept work.TestAServiceOnAnotherVersionIsRefused (J=0.48) |
| DROP | go | `launch.TestADownloadThatFailsBothWaysReportsTheAPIsFailure` | 0 | 60 | 192 | 0.0 | 833d3e9 | 0 unique stmts; covered by others; nearest kept launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.42) |
| DROP | go | `launch.TestDownloadGivesUpAtItsDeadline` | 3 | 46 | 126 | 0.1 | 78c7880 | 3 unique stmts; covered by others; nearest kept launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.26) |
| DROP | go | `launch.TestIsGo` | 0 | 12 |  | 0.0 | 78c7880 | 0 unique stmts; covered by others; nearest kept work.TestAServiceOnAnotherVersionIsRefused (J=0.06) |
| DROP | go | `launch.TestNewerComparesVersionsAndAPreReleaseWithItsRelease` | 0 | 17 |  | 0.0 | a274b65 | 0 unique stmts; covered by others; nearest kept work.TestAServiceOnAnotherVersionIsRefused (J=0.09) |
| DROP | go | `launch.TestReleaseURL` | 1 | 3 | 164 | 0.0 | 78c7880 | 1 unique stmts; covered by others; nearest kept launch.TestADownloadThatFailsWithNoTokenIsTriedThroughTheAPIWithOne (J=0.03) |
| DROP | go | `launch.TestScrubTakesTheMarks` | 3 | 14 | 105 | 0.0 | 78c7880 | 3 unique stmts; covered by others; nearest kept launch.TestMarkersAreThoseScrubTookAndNoneUnlaunched (J=0.31) |
| DROP | go | `launch.TestTargetRunsInProcessForItsOwnPinTheLaunchedPinAndUpgrade` | 8 | 72 | 85 | 0.0 | 78c7880 | 8 unique stmts; covered by others; nearest kept work.TestAServiceOnAnotherVersionIsRefused (J=0.13) |
| DROP | go | `pyjson.TestDumpsKeepsKeyOrderAndPythonSeparators` | 9 | 76 | 78 | 0.0 | 6463ce2 | 9 unique stmts; covered by others; nearest kept hooks.TestTouchedCountsASubagentsPathOnceItsCallReturns (J=0.25) |
| DROP | go | `pyjson.TestFloatRepr` | 4 | 12 | 115 | 0.0 | 6463ce2 | 4 unique stmts; covered by others; nearest kept records.TestHeaderValuesTypeAsPyyamlDoes (J=0.04) |
| DROP | go | `pyjson.TestStringEscapesAsJSONDumps` | 3 | 17 | 107 | 0.0 | 6463ce2 | 3 unique stmts; covered by others; nearest kept hooks.TestTouchedCountsASubagentsPathOnceItsCallReturns (J=0.05) |
| DROP | go | `records.TestParseReadsTheHeaderWithYAML11` | 1 | 88 | 175 | 0.0 | bc13f4d | 1 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.24) |
| DROP | go | `records.TestParseSortsByPathParts` | 0 | 6 |  | 0.0 | bc13f4d | 0 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.02) |
| DROP | go | `records.TestSections` | 3 | 64 | 119 | 0.0 | bc13f4d | 3 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.14) |
| DROP | go | `records.TestSplitLinesIsPythons` | 4 | 14 | 116 | 0.0 | bc13f4d | 4 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.03) |
| DROP | go | `records.TestTemplatesAndInsertEntry` | 5 | 60 | 106 | 0.0 | bc13f4d | 5 unique stmts; covered by others; nearest kept store.TestCommitCommitsOnlyThePathsNamed (J=0.07) |
| DROP | go | `records.TestYAMLStrQuotes` | 2 | 117 | 139 | 0.0 | bc13f4d | 2 unique stmts; covered by others; nearest kept records.TestHeaderValuesTypeAsPyyamlDoes (J=0.57) |
| DROP | go | `service.TestALabelNamesTheCloneAndHashesItsPath` | 0 | 6 |  | 0.0 | 2cfae09 | 0 unique stmts; covered by others; nearest kept service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.06) |
| DROP | go | `service.TestAPortInTimeWaitIsFree` | 0 | 4 |  | 0.0 | 2cfae09 | 0 unique stmts; covered by others; nearest kept service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.04) |
| DROP | go | `service.TestARenderErrorIsServedAsTheErrorAndTakesNoReply` | 2 | 251 | 133 | 0.0 | 2cfae09 | 2 unique stmts; covered by others; nearest kept service.TestAPartialReadIsRepeatedUnderTheLockAndServedThere (J=0.91) |
| DROP | go | `service.TestASupervisorThatDoesNotAnswerIsNoStop` | 0 | 134 |  | 0.0 | a274b65 | 0 unique stmts; covered by others; nearest kept service.TestUninstallStopsAndRemovesTheUnit (J=0.84) |
| DROP | go | `service.TestAUnitRunningAnotherPmIsStaleUntilPmServiceInstall` | 3 | 233 | 98 | 0.0 | 2cfae09 | 3 unique stmts; covered by others; nearest kept service.TestServiceInstallStatusRestartAndLogsEndToEnd (J=0.65) |
| DROP | go | `service.TestDriftNamesWhatDiffersFromWhatPmInitInstalls` | 2 | 209 | 117 | 0.0 | 2cfae09 | 2 unique stmts; covered by others; nearest kept service.TestAServiceOnMainsPinIsNotStaleFromACheckoutOffIt (J=0.69) |
| DROP | go | `service.TestFreePortSkipsHeldPortsAndOtherClonesUnits` | 1 | 50 | 190 | 0.0 | 2cfae09 | 1 unique stmts; covered by others; nearest kept service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.63) |
| DROP | go | `service.TestInstallRestartsACurrentUnitThatAnswersOnAnotherBuild` | 0 | 147 |  | 0.0 | 2cfae09 | 0 unique stmts; covered by others; nearest kept service.TestUninstallStopsAndRemovesTheUnit (J=0.74) |
| DROP | go | `service.TestLaunchdAgentKeepsTheServiceAlive` | 1 | 185 | 178 | 0.0 | 2cfae09 | 1 unique stmts; covered by others; nearest kept service.TestStopDisablesTheUnitAndHoldsUntilRestartOrInstall (J=0.50) |
| DROP | go | `service.TestLogsIsTheLogsLastLines` | 2 | 13 | 142 | 0.0 | 2cfae09 | 2 unique stmts; covered by others; nearest kept service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.03) |
| DROP | go | `service.TestRefusalsWithoutASupervisorOrTheTools` | 8 | 76 | 84 | 0.0 | 2cfae09 | 8 unique stmts; covered by others; nearest kept service.TestUninstallStopsAndRemovesTheUnit (J=0.36) |
| DROP | go | `service.TestStopFailsWhileTheSocketStillAnswers` | 6 | 158 | 96 | 0.4 | a274b65 | 6 unique stmts; covered by others; nearest kept service.TestUninstallStopsAndRemovesTheUnit (J=0.70) |
| DROP | go | `service.TestSystemdUnitRunsThisPmRestartsItAndQuotesPaths` | 0 | 156 |  | 0.0 | 2cfae09 | 0 unique stmts; covered by others; nearest kept service.TestUninstallStopsAndRemovesTheUnit (J=0.71) |
| DROP | go | `service.TestTheStoreIsOpenedOnlyWhenItMovedAndNeverHeld` | 0 | 189 |  | 0.2 | 2cfae09 | 0 unique stmts; covered by others; nearest kept service.TestAPartialReadIsRepeatedUnderTheLockAndServedThere (J=0.77) |
| DROP | go | `service.TestUnitPortAndCommandReadBackWhatInstallWrote` | 2 | 102 | 146 | 0.0 | 2cfae09 | 2 unique stmts; covered by others; nearest kept service.TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers (J=0.28) |
| DROP | go | `site.TestAMovedSprintsOldPageIsItsPage` | 6 | 763 | 94 | 0.0 | fcae7a7 | 6 unique stmts; covered by others; nearest kept site.TestCheckRefusesEachBrokenRecord (J=0.62) |
| DROP | go | `site.TestCitesMatchesTheIDAlone` | 3 | 30 | 121 | 0.0 | 2d1a4db | 3 unique stmts; covered by others; nearest kept site.TestFillRepliesAndStatus (J=0.12) |
| DROP | go | `site.TestIndexListsEverySprintNotDoneAndOnlyTheLatestClosedDoneOnes` | 0 | 223 |  | 0.0 | 2d1a4db | 0 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.21) |
| DROP | go | `store.TestCommittedIsHeadWithTheChanges` | 0 | 103 |  | 0.0 | bc13f4d | 0 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.21) |
| DROP | go | `store.TestDesignDatesFollowRenamesAndCountChangesAsToday` | 6 | 178 | 90 | 0.0 | bc13f4d | 6 unique stmts; covered by others; nearest kept cli.TestDayActivityHoldsOnlyThatDaysEvents (J=0.27) |
| DROP | go | `store.TestWriteAtomicKeepsTheMode` | 0 | 20 |  | 0.0 | bc13f4d | 0 unique stmts; covered by others; nearest kept install.TestExcludeRoundTrip (J=0.17) |
| DROP | go | `sync.TestANeverSuccessfulPushIsOverdueFromTheInstall` | 0 | 56 |  | 0.0 | 2f6d3d8 | 0 unique stmts; covered by others; nearest kept sync.TestARunRecordsEachStepAndFlagsAFailure (J=0.38) |
| DROP | go | `work.TestDoltRemoteURL` | 3 | 11 | 82 | 0.0 | 4860f5c | 3 unique stmts; covered by others; nearest kept install.TestSetupWorkCreatesPushesAndClones (J=0.01) |
| DROP | go | `work.TestEachWriteIsOneDoltCommit` | 1 | 639 | 168 | 0.4 | 55e396c | 1 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.91) |
| DROP | go | `work.TestEditKeepsAnAnsweredNeedsDescription` | 1 | 621 | 179 | 0.4 | b61f12d | 1 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.92) |
| DROP | go | `work.TestFromExportRoundTripsAStoreIntoAnother` | 0 | 737 | 145 | 0.4 | 5d5f7a2 | 0 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.55) |
| DROP | go | `work.TestGCKeepsEveryItem` | 0 | 548 | 86 | 0.3 | 3c7d9ee | 0 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.90) |
| DROP | go | `work.TestIDsOrderByTheirNumbers` | 3 | 22 | 122 | 0.0 | fb42de7 | 3 unique stmts; covered by others; nearest kept work.TestCheckRefusesEachImpossibleState (J=0.10) |
| DROP | go | `work.TestImportMapsEachRowOfTheTable` | 0 | 308 |  | 0.0 | 6fcbaa5 | 0 unique stmts; covered by others; nearest kept work.TestImportFailsHardOnWhatItCannotMap (J=0.85) |
| DROP | go | `work.TestItemJSONUsesThePagesFieldNames` | 0 | 0 |  | 0.0 | 364724e | 0 unique stmts; covered by others; nearest kept service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) |
| DROP | go | `work.TestMoveSprintKeepsTheIdRenumbersAndNotes` | 2 | 643 | 144 | 0.3 | fcae7a7 | 2 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.86) |
| DROP | go | `work.TestRoundTripOfARealExport` | 0 | 0 |  | 0.0 | 6fcbaa5 | opt-in (PM_BD_EXPORT), skipped in CI; keep as a manual script if wanted (0 unique stmts); nearest kept service.TestAFailedOpenIsTriedAgainAtTheNextLookAndAnOversizedReplyIsR (J=0.00) |
| DROP | go | `work.TestRoundTripOfTheFixture` | 0 | 712 |  | 0.2 | 6fcbaa5 | 0 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.57) |
| DROP | go | `work.TestSprintMovesReadsOnlyPmsNotes` | 1 | 12 | 169 | 0.0 | fcae7a7 | 1 unique stmts; covered by others; nearest kept cli.TestTaskRelease (J=0.00) |
| DROP | go | `work.TestTheMarkMovesOnAWriteAndNotOnARead` | 0 | 545 |  | 0.2 | 4db4259 | 0 unique stmts; covered by others; nearest kept work.TestCommentsKeepTheirOrderWithinASecond (J=0.90) |
| DROP | go | `work.TestTheSocketIsPrivateAndItsPathBounded` | 1 | 195 | 181 | 0.2 | 4db4259 | 1 unique stmts; covered by others; nearest kept install.TestSetupWorkCreatesPushesAndClones (J=0.40) |
| DROP | go | `work.TestTheTreeIsValid` | 0 | 130 |  | 0.0 | fb42de7 | 0 unique stmts; covered by others; nearest kept work.TestCheckRefusesEachImpossibleState (J=0.66) |
| DROP | integration | `test_lifecycle.py::test_doctor_names_a_release_newer_than_the_pin` | - | - |  | 0.0 | a274b65 | a doctor info line; launch Go tests cover latest.go [coverage not measured] |
| DROP | integration | `test_lifecycle.py::test_upgrade_takes_out_what_python_pm_left_of_beads` | - | - |  | 0.0 | b7cbb0e | Python-pm migration; Go install tests cover the rewrite [coverage not measured] |
| DROP | integration | `test_pm.py::test_day_summarize_skips_unchanged_activity_and_regenerates_on_change` | - | - |  | 0.0 | b24b04e | cache detail of day summarize [coverage not measured] |
| DROP | light | `test_config.py::test_every_command_fails_on_another_pinned_version[args1]` | 0 | 1201 |  | 0.6 | f0eb60e | param of test_every_command_fails_on_another_pinned_version: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| DROP | light | `test_config.py::test_every_command_fails_on_another_pinned_version[args2]` | 0 | 1191 |  | 0.6 | f0eb60e | param of test_every_command_fails_on_another_pinned_version: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| DROP | light | `test_config.py::test_every_command_fails_on_another_pinned_version[args3]` | 0 | 1190 |  | 0.6 | f0eb60e | param of test_every_command_fails_on_another_pinned_version: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.98) |
| DROP | light | `test_config.py::test_every_command_fails_without_a_config[args1]` | 0 | 1201 |  | 0.6 | f0eb60e | param of test_every_command_fails_without_a_config: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| DROP | light | `test_config.py::test_every_command_fails_without_a_config[args2]` | 0 | 1191 | 99 | 0.6 | f0eb60e | param of test_every_command_fails_without_a_config: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| DROP | light | `test_config.py::test_every_command_fails_without_a_config[args3]` | 0 | 1190 |  | 0.6 | f0eb60e | param of test_every_command_fails_without_a_config: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.97) |
| DROP | light | `test_hooks.py::test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` | 0 | 232 |  | 0.1 | 9b6113e | 0 unique stmts; covered by others; nearest kept test_pm_init_is_the_one_install_command (J=0.94) |
| DROP | light | `test_hooks.py::test_stop_blocks_on_a_record_this_session_edited` | 0 | 1373 |  | 0.6 | b24b04e | 0 unique stmts; covered by others; nearest kept test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[Tr (J=0.98) |
| DROP | light | `test_hooks.py::test_stop_passes_another_sessions_edit` | 0 | 1317 | 193 | 0.6 | b24b04e | 0 unique stmts; covered by others; nearest kept test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names[Tr (J=0.94) |
| DROP | light | `test_hooks.py::test_stop_passes_when_stop_hook_active` | 1 | 1226 | 152 | 0.6 | b24b04e | 1 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.95) |
| DROP | light | `test_hooks.py::test_subagent_start_envelope` | 0 | 232 |  | 0.2 | b24b04e | 0 unique stmts; covered by others; nearest kept test_pm_init_is_the_one_install_command (J=0.94) |
| DROP | light | `test_launch.py::test_a_go_pin_downloaded_with_no_token_never_asks_for_one` | 0 | 1183 |  | 1.0 | 833d3e9 | 0 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.98) |
| DROP | light | `test_launch.py::test_a_go_pin_downloads_checks_and_keeps_its_release_binary_then_execs_it` | 0 | 1205 |  | 1.2 | 93cd48e | 0 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.96) |
| DROP | light | `test_launch.py::test_a_go_pin_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone[gh auth token]` | 0 | 1210 |  | 1.5 | e1b0585 | param of test_a_go_pin_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone: same path as its siblings; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.96) |
| DROP | light | `test_launch.py::test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing[mismatch]` | 0 | 1252 |  | 1.1 | 93cd48e | param of test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing: same path as its siblings; nearest kept test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.96) |
| DROP | light | `test_launch.py::test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing[missing]` | 0 | 1286 |  | 1.1 | 93cd48e | param of test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing: same path as its siblings; nearest kept test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.99) |
| DROP | light | `test_launch.py::test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing[not-one-pm]` | 0 | 1292 |  | 1.2 | 93cd48e | param of test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing: same path as its siblings; nearest kept test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.96) |
| DROP | light | `test_launch.py::test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing[bad-token]` | 0 | 1291 |  | 1.6 | e1b0585 | param of test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing: same path as its siblings; nearest kept test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.99) |
| DROP | light | `test_launch.py::test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing[no-asset]` | 1 | 1300 | 205 | 1.5 | e1b0585 | param of test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing: same path as its siblings; nearest kept test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_ (J=0.98) |
| DROP | light | `test_launch.py::test_a_kept_go_binary_runs_with_no_request` | 0 | 1183 |  | 1.1 | 93cd48e | 0 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.98) |
| DROP | light | `test_launch.py::test_a_pm_launched_for_its_pin_that_runs_another_version_fails_instead_of_launching_again` | 0 | 1190 |  | 0.6 | 1fe146d | 0 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.98) |
| DROP | light | `test_launch.py::test_a_pre_release_pin_is_a_go_pin` | 0 | 1205 |  | 1.2 | 93cd48e | 0 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.96) |
| DROP | light | `test_launch.py::test_a_python_pin_fails_hard_naming_the_fix` | 5 | 1202 | 112 | 0.7 | b0c8166 | 5 unique stmts; covered by others; nearest kept test_text_file_dash_refuses_a_socket_at_once (J=0.96) |
| DROP | light | `test_owner_request_hook.py::test_blocks_a_clarification_asked_only_in_chat` | 0 | 1379 |  | 3.0 | 2b35026 | 0 unique stmts; covered by others; nearest kept test_blocks_a_needless_ask_even_when_an_open_need_matches (J=0.99) |
| DROP | light | `test_owner_request_hook.py::test_blocks_a_request_matching_no_open_need_of_this_session[None]` | 0 | 1379 |  | 3.0 | 3b2e7d7 | param of test_blocks_a_request_matching_no_open_need_of_this_session: same path as its siblings; nearest kept test_blocks_a_needless_ask_even_when_an_open_need_matches (J=0.99) |
| DROP | light | `test_owner_request_hook.py::test_passes_a_request_matching_this_sessions_need` | 0 | 1350 |  | 2.7 | 3b2e7d7 | 0 unique stmts; covered by others; nearest kept test_blocks_a_needless_ask_even_when_an_open_need_matches (J=0.98) |
| DROP | light | `test_owner_request_hook.py::test_passes_without_reading_anything[event0]` | 0 | 1246 |  | 2.9 | 3b2e7d7 | param of test_passes_without_reading_anything: same path as its siblings; nearest kept test_fails_when_claude_fails (J=0.94) |
| DROP | light | `test_pm.py::test_commit_commits_only_its_callers_records_beside_another_sessions_edit` | 5 | 2067 | 101 | 0.9 | b24b04e | 5 unique stmts; covered by others; nearest kept test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_on (J=0.99) |
| DROP | light | `test_pm.py::test_commit_takes_a_path_relative_to_the_store_from_anywhere` | 2 | 2104 | 160 | 0.7 | b61f12d | 2 unique stmts; covered by others; nearest kept test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_on (J=0.97) |
| DROP | light | `test_pm.py::test_concurrent_writes_from_two_worktrees_land_as_separate_commits` | 0 | 1885 |  | 0.6 | b24b04e | 0 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.79) |
| DROP | light | `test_pm.py::test_decision_add_need_closes_need_and_records_decision` | 0 | 2277 |  | 0.7 | b24b04e | 0 unique stmts; covered by others; nearest kept test_a_decision_recorded_for_a_no_decision_need_marks_it_answered (J=0.97) |
| DROP | light | `test_pm.py::test_decision_close_closes_small_answer_without_record` | 0 | 2135 |  | 0.7 | b24b04e | 0 unique stmts; covered by others; nearest kept test_a_decision_recorded_for_a_no_decision_need_marks_it_answered (J=0.92) |
| DROP | light | `test_pm.py::test_decision_close_without_an_answer_names_need_dismiss_for_a_moot_need` | 0 | 1598 |  | 0.8 | cee2d7d | 0 unique stmts; covered by others; nearest kept test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_wo (J=0.85) |
| DROP | light | `test_pm.py::test_decision_need_writes_its_flags_in_the_one_layout` | 0 | 1612 |  | 0.7 | bc7d31d | 0 unique stmts; covered by others; nearest kept test_no_command_waits_on_an_open_stdin[action need-text] (J=0.90) |
| DROP | light | `test_pm.py::test_finding_add_takes_the_text_as_an_argument_with_text_or_with_text_file` | 0 | 1900 |  | 0.7 | cee2d7d | 0 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.79) |
| DROP | light | `test_pm.py::test_pm_check_and_commit_take_image_files_beside_a_record` | 1 | 2136 | 196 | 0.7 | 214d486 | 1 unique stmts; covered by others; nearest kept test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_on (J=0.95) |
| DROP | light | `test_pm.py::test_pm_help_lists_the_commands_that_ran_outside_the_command_tree` | 0 | 233 |  | 0.2 | cee2d7d | 0 unique stmts; covered by others; nearest kept test_pm_init_is_the_one_install_command (J=0.94) |
| DROP | light | `test_pm.py::test_sprint_open_creates_epic_and_record` | 0 | 2058 |  | 0.8 | b24b04e | 0 unique stmts; covered by others; nearest kept test_text_file_reads_a_file_and_refuses_with_text (J=0.84) |
| DROP | light | `test_pm.py::test_task_add_creates_task_in_sprint` | 0 | 1935 |  | 0.8 | b24b04e | 0 unique stmts; covered by others; nearest kept test_text_file_reads_a_file_and_refuses_with_text (J=0.95) |
| DROP | light | `test_pm.py::test_task_claim_records_the_session_given_with_session_even_with_one_in_the_environment` | 0 | 1546 |  | 1.1 | cee2d7d | 0 unique stmts; covered by others; nearest kept test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_wo (J=0.94) |
| DROP | light | `test_pm.py::test_task_close_dropped_records_why_and_needs_no_commit` | 7 | 2262 | 91 | 1.3 | d0e08f3 | 7 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.86) |
| DROP | light | `test_pm.py::test_task_close_names_head` | 0 | 1980 |  | 1.1 | b24b04e | 0 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.99) |
| DROP | light | `test_pm.py::test_task_close_with_commit_warns_nothing_about_a_dirty_tree` | 1 | 1987 | 201 | 1.5 | d0e08f3 | 1 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.98) |
| DROP | light | `test_pm.py::test_task_move_takes_a_task_from_directly_under_a_project_into_its_sprint` | 5 | 2062 | 113 | 1.2 | d0e08f3 | 5 unique stmts; covered by others; nearest kept test_task_close_refuses_a_task_another_live_session_holds (J=0.88) |
