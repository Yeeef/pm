---
type: sprint
title: Review follow-ups from the 2026-10-10 sprint wave
bead: pm-d2k5.13
---

## Goal

> What should be true when this sprint ends, and why now?

The low-severity defects that fresh-context reviews and sprint agents found in the 2026-10-10 wave of pm-quality PRs are fixed, each with the check that catches it, so none of them stays known and unfixed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `records.Texts` and Load: a record that vanished mid-read is skipped (PR #15), which bypasses Load's retry under the records lock; restore a consistent read (retry under the lock when anything was skipped) without bringing back the error page.
- A dangling `.md` symlink in the records store is an error again for `pm check` and `pm commit` (it became silently ignored in PR #15).
- `pm uninstall` racing a parallel session start that restarts the service between its unsynced check and the unit's removal; `service.Uninstall` disabling a systemd unit only when it is active, which leaves an enabled, crashed unit's `default.target.wants` link (found in sprint 11's review; both predate it).
- `pm sprint move`'s records step can run twice across clones when a clone whose records branch has not synced reruns a move another clone finished, duplicating the rename and decisions (sprint 4).

**Out:**
- New features; anything a running sprint already covers.

## Done when

> What evidence will show the goal is met?

- Each item has a test that fails before its fix and passes after, and `make test`, `make test-go` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
Scope adds tasks .5-.8: access_test skipping agent worktrees, the pm-release-build.yml paths and GO_PKGS failure, the TestCreateRacingALocalWriter flake root cause, and the internal/service fake data race.
The coordinator filed them into this sprint during the wave; each is a review or CI follow-up of the same kind as the In list.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Load consistency: with records.Texts skipping a file a records sync unlinked
  mid-read (PR #15), servedSite.Load passed without every record in 4-15 of
  ~45 loads per run (git checking out between two commits of 100 docs); some
  missing files were never even listed by the walk, so Load now also compares
  the stamp after the read with the one before. With Texts naming what went
  plus the stamp compare, 0 of ~60 loads per run (8 runs) passed short; each
  such read becomes a service.Partial, read again under the records lock.

- TestCreateRacingALocalWriter, linux race (clone: invalid connection): git
  2.47+ (CI runs 2.55.0, this machine 2.43.0) starts a fetch's auto
  maintenance detached; the detached git takes and drops
  objects/maintenance.lock after the fetch returned, while DOLT_CLONE walks
  the new database's .dolt (CanCreateDatabaseAtPath), and Dolt's localFS.iter
  dereferences the nil FileInfo of the vanished lock. 0 failures in 224 runs
  with git 2.43; reproduced with git 2.55.0 built from source plus a
  GIT_EXEC_PATH shim starting the detached maintenance 0-19 ms late: 2 of 40
  runs failed with the CI panic stack. Fix: the host sets
  maintenance.autoDetach=false and gc.autoDetach=false via GIT_CONFIG_COUNT.

- TestCreateRacingALocalWriter, macOS race (push: unknown push error; git
  command failed (exit -1)): the full log shows 'git hash-object -w --stdin
  ... signal: segmentation fault', the child dying before exec. That is
  golang/go#79804 (darwin -race instruments rawSyscall, which the forked child
  calls pre-exec), fixed in Go 1.26.5 (backport golang/go#79806); go.mod said
  1.26.2. Fix: go 1.26.9. Not reproducible on linux; CI's macOS race job is
  the check.

- Service fake race (task .8): fakeWork.item/Items/Get copied items shallowly,
  sharing Need with the fake; the test read Need.Delivered unlocked while
  UpdateNeed wrote it. go test -race -run TestAReplyIsSpooled, 8 runs of
  -count=100 on 2 shared CPUs: 5 of 8 raced before, 0 of 8 after the deep
  copy; -count=50 clean.

- pm sprint move on a lagging clone (task .4): the records step's decisions
  were dated the clone's local day, so a rerun on a clone whose records lagged
  another clone's finished move wrote a different commit when the day
  differed, and the records sync's rebase conflicted (test: move at UTC+14,
  rerun at UTC-11: 'could not apply ... finished moving'). Dating the
  decisions with the move note's UTC date makes the step identical everywhere;
  the rebase drops the copy. A fetch-based refusal was rejected: the other
  clone's records reach the remote only at its next 10-minute sync, while its
  work-store move is pushed at once, so a fetch misses the main window.

- Dangling record link (task .2): pm check printed 'all 6 pages render' with
  docs/2026-10-07-gone.md -> nowhere.md in the store, and pm commit named on
  it took it as a deletion and committed the link. records.LinkToNothing
  (Lstat says link, Stat says gone) now tells it from a file a sync unlinked;
  both commands refuse, changing nothing.

- pm uninstall (task .3): with the clone's install lock held (as a session
  start starting the service holds it), pm uninstall finished at once before;
  now it waits, removing nothing, until the lock is free, as it holds the lock
  from its unsynced check to the store's removal. A crashed systemd unit
  (inactive, enabled) stayed in the fake supervisor's enabled list after
  uninstall; it is now disabled (disable --now when active or is-enabled says
  enabled).

- access_test and CI (tasks .5, .6): with a module copy under
  .claude/worktrees/copy the access test failed on three calls in the copy;
  the scan now stops at a nested go.mod, the go command's module boundary.
  pm-go.yml's GO_PKGS="$(go list ... | grep | tr)" with a go list that prints
  one package and fails tested that package and exited 0; make
  go-test-but-work takes the list first and fails. pm-release-build.yml's
  paths now include cmd/pm/** and internal/launch/** (pm version goes through
  both); tests/test_ci.py holds both.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: every follow-up in Scope and the four tasks added to it ship in PR #31, merged as da3423c, each with a test that failed before its fix, and the TestCreateRacingALocalWriter flake has two root causes, both fixed.

- The site no longer passes a records read that missed a file a sync was rewriting. Such a read is now a `service.Partial`, read again under the records lock.
- `pm check` and `pm commit` refuse a `.md` link to nothing.
- `pm uninstall` holds the install lock from its unsynced check to the store's removal, and it disables a crashed systemd unit.
- A `pm sprint move` rerun writes the same records step on every clone, because the decisions are dated with the move note's UTC date.
- The access test stops at nested modules.
- `pm-release-build.yml` also runs on `cmd/pm` and `internal/launch` changes. `make go-test-but-work` fails when `go list` fails.
- The flake had a Linux cause and a macOS cause:
  - Linux: since git 2.47, a fetch's auto maintenance runs detached and removes `objects/maintenance.lock` while DOLT_CLONE walks `.dolt`. The pm service host now keeps git maintenance attached.
  - macOS: the crash is golang/go#79804. The fix is Go 1.26.9.
- The service test fake hands out deep copies of its items.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Each item has a test that fails before its fix and passes after: met.** Before and after, per item (the Findings give the full numbers):

| Item | Test | Before | After |
|---|---|---|---|
| Records read missing a file | `TestLoadNeverPassesARecordSetMissingAFileGitRewrote` | 4-15 of ~45 loads passed short | 0 of ~60 |
| Records read missing a file | `TestAPartialReadIsRepeatedUnderTheLockAndServedThere` | timed out on the error page | pass |
| Link to nothing | `test_pm_check_and_commit_refuse_a_record_link_to_nothing` | pm check: "all 6 pages render" | refused |
| `pm uninstall` | `test_uninstall_waits_for_a_session_start_that_holds_the_install_lock` | did not wait | waits |
| `pm uninstall` | `test_uninstall_disables_an_enabled_unit_that_is_not_running` | unit still enabled | disabled |
| Sprint move rerun | `test_sprint_move_rerun_on_a_clone_whose_records_lag_writes_the_same_records_step` | rebase conflict | the copy is dropped |
| Access test | `TestModuleGoFilesLeaveOutNestedModulesAndWorktrees` | 3 false failures with a module copy under `.claude/worktrees` | pass |
| CI | `tests/test_ci.py` | `'cmd/pm/**'` missing; a failing go list exited 0 | pass |
| Flake, Linux | `TestTheHostsFetchLeavesNoMaintenanceRunning`, with git 2.55 | 10 of 10 maintenances detached | none |
| Flake, Linux | create race test, git 2.55, detached maintenance delayed 0-19 ms | 5 of 80 runs failed | 0 of 160 |
| Service fake | `go test -race -run TestAReplyIsSpooled`, 8 runs of `-count=100` on 2 CPUs | 5 of 8 raced | 0 of 8 |

- **`make test`, `make test-go` and the PR's CI pass: met.**
  - `make test`: 136 passed.
  - `make test-go`: every package ok, including internal/work (34.6 s) and its race tests (96.3 s).
  - The touched integration tests: 27 passed.
  - PR #31's CI: all 11 checks passed on head 329de4c, and `make merge-ready PR=31` says the PR is ready.
  - On the first run, `work (linux-amd64)` failed on a proxy.golang.org download error (a cold cache after the Go bump). It passed when rerun once.
  - The macOS half of the flake was not reproduced here (Linux machine). The evidence for it is the Go issue's mechanism and the PR's macOS race job, one green run so far.
