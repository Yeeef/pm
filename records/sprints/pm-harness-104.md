---
type: sprint
title: pm's work store is served by the pm service, not opened by each command
bead: yeeef-agents-9va.114
---

## Goal

> What should be true when this sprint ends, and why now?

Every pm command reaches the work store through the pm service, which holds the Dolt store open and serves it over a Unix socket, so commands never wait on the store's exclusive lock and every read is warm. The owner chose this over the per-command embedded store, which the [pm in Go](../design/pm-go.md) page kept as the alternative to switch to.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The service hosting the Dolt engine in process with its MySQL-compatible server on a Unix socket under `.pm/run/`; pm commands as SQL clients behind the same `work.Store` interface.
- One access path: no command opens the store directly, and no fallback to a direct open; a command with the service down fails hard naming the fix.
- A command and the service on different pm versions refuse, or the service restarts on the pinned version.
- Sync, the child-id compare-and-swap, gc and conflict resolution run in the service; `pm sync` asks the service.
- Removing the gate, its wait log and the rule against holding the store across slow work.
- Tests, with the service started per test where a store is needed; the pm-go and work-store design pages moved to the new final state; a release from Yeeef/pm and this repo's pin moved to it.

**Out:** a standalone `dolt sql-server` binary; other repos' upgrades; a multi-machine shared server.

## Done when

> What evidence will show the goal is met?

- A test shows no pm command opens the embedded store, and every store command fails hard with the service stopped, naming `pm service restart`.
- Loading every item in a fresh `pm export` takes at most 20 ms median over 10 runs on this repo's data (the spike measured 7.8 ms; per-command it is 64 to 70 ms).
- 8 processes × 20 writes finish with 160 of 160 correct and no lock wait, against 7.7 to 10.9 s per-command.
- A version mismatch between a command and the service is refused or resolved by a restart, with a test.
- Released from Yeeef/pm and pinned here: `pm doctor` clean, `pm export` equal before and after, the site equal after normalisation.
- The sprint's PRs are on main in Yeeef/pm and here.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Store sharing, and "The service holds the store" under Alternatives considered
- [Work store: pm's own replacement for Beads](../design/work-store.md): Storage

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-09}
Commands ask the service for sync, the child-id compare-and-swap and setup with stored procedures on the one SQL socket (CALL pm_sync(), pm_create(?), pm_setup()), and learn its version with SELECT pm_version() on the connection they then use; no second socket or HTTP API.
One connection per command and a version check bound to that connection; Dolt registers external procedures and functions per engine in about 20 lines; an HTTP socket would add a second endpoint and a gap between check and use.
:::

::: decision {source=agent date=2026-10-09}
pm writes are serializable through a write_stamp row every write sets to a fresh UUID; the loser of Dolt serialization failure (error 1213) reruns from a fresh read, at most 20 attempts; no named lock or gate.
Row-level merge of concurrent transactions lets two writes on different rows break cross-row invariants (opposite dep add makes a cycle, which then fails every read); the stamp makes them conflict while keeping the done-when no-lock-wait run.
:::

::: decision {source=agent date=2026-10-09}
The service never resets main: the pull checks the remote head before a fast-forward, and the child-id compare-and-swap mints and pushes from a scratch branch pm-cas, then merges it into main.
DOLT_RESET --hard sets the branch head with no check and would drop commits other sessions made meanwhile; a fast-forward is a compare-and-swap in Dolt and safe.
:::

::: decision {source=agent date=2026-10-09}
The socket is <main checkout>/.pm/run/work.sock, mode 0600, served from a listener pm opens (no TCP); a path of 104 bytes or more fails the service start hard, and tests use short temp roots.
It stays inside the Codex writable roots and beside the service log; a /tmp socket may be outside the sandbox; the main checkout path is short on every recorded clone.
:::

::: decision {source=agent date=2026-10-09}
A write that loses to concurrent writes runs at most 100 times (WriteAttempts), not 20.
On a copy of this repo store, 8 processes x 20 writes peaked at 22 attempts for one write over 17 runs and lost a write in 1 of 7 runs at 20; each attempt loses with odds near 7/8, so 100 makes a lost write in a run about 2.5e-4 likely.
:::

::: decision {source=agent date=2026-10-09}
A merge commit (a pull 3-way merge, the pm-cas merge into main) keeps this side write_stamp on the conflict, then sets a fresh one, as every write does.
A fast-forward over a remote commit whose stamp equals the base would let a write in flight land on rows it never read; a fresh stamp on every commit makes that write conflict and rerun.
:::

::: decision {source=agent date=2026-10-09}
pm init sets the clone up first, then installs and starts the service, then calls pm_setup(); the post-checkout hook no longer attaches the store; pm init --import-bd makes an empty store through the service when there is none.
The store is reachable only through the service, so its attach must follow the service start; the hook runs in every checkout and must not need the service; the import is how a cut-over and the tests seed a store.
:::

::: decision {source=agent date=2026-10-09}
The host migrates an older schema after it starts serving, on a connection of its own, and pm_version() answers only once the schema is current.
Migrating needs a SQL connection to the engine, which the host gets only through its server; holding every other client in its handshake keeps any command off the old schema.
:::

::: decision {source=agent date=2026-10-09}
While a test repo pins another pm, its transcript records a fixed line in place of the work-store export, for Python and Go alike.
The per-test service stops once the pin moves, as the installed one does, and only that other pm service could hold the store; the launcher tests that move the pin never touch the store.
:::

::: decision {source=agent date=2026-10-09}
The garbage collection takes no operation slot (one collection at a time on its own lock), and every operation in the slot runs within its bound, the wait included: sync 120 s, create 180 s, setup 300 s.
DOLT_GC waits for every session mid-statement, and a CALL pm_sync or pm_create waiting for the slot is one, so a collection holding the slot deadlocked until GCTimeout (review, confirmed); an unbounded fetch in the slot blocked every later create, setup and sync.
:::

::: decision {source=agent date=2026-10-09}
A write that gets Dolt dataset head is not ancestor of commit (ErrMergeNeeded) runs again, as on a serialization failure; a pull fetches once and only its merge starts again.
Dolt returns ErrMergeNeeded only from inside its root update compare-and-swap, before anything lands, so it is a lost race like 40001; a merge lost to a local write needs no new fetch.
:::

::: decision {source=agent date=2026-10-09}
pm writes run one at a time under a fair write lock the service holds (CALL pm_lock/pm_unlock, FIFO, taken over from a dropped connection), from before the first read to after the commit; the write stamp stays as the safety net and a conflict fails hard; there is no retry. This supersedes the decisions on write_stamp retries (no named lock) and on 100 write attempts.
Second review, confirmed under -race: optimistic retries starved long transactions (a pull or CAS merge over every item lost 100 times to one writer pausing 0-6 ms; 4 of 15 creates failed). Under the lock, -race -count=5 of the race tests passes with zero failures, and 8 x 20 writes take 2.11-2.17 s, each waiting 88-90 ms on average; GMS GET_LOCK polls a CAS and keeps no order, so it could starve too.
:::

::: decision {source=agent date=2026-10-09}
An operation bound stops only fetch and push, each on a connection of its own; a pull merge runs to its end on a background context; a write refuses a working set that differs from main head at its start.
A merge cut by the bound could split Dolt SQL fast-forward (head moved, working set not), which the next DOLT_COMMIT(-A) would commit as a revert; the dropped fetch or push connection leaves the operation connection and its lock usable to check the outcome.
:::

::: decision {source=agent date=2026-10-09}
A child create whose push timed out or failed but by a non-fast-forward rejection never mints again: a check that sees C1 on the remote reports it landed, anything else fails hard as unknown, naming the id it may land as. This supersedes the retry in the compare-and-swap design step 4.
Third review: the server finishes a dropped DOLT_PUSH until Dolt kills its git, so C1 can land after the check; the retry then minted the item again (the review simulation made demo-pfmw.1.3 and .1.4). Only a non-fast-forward rejection proves nothing landed.
:::

::: decision {source=agent date=2026-10-09}
The write lock has no lease; a timeout names the holder (connection id, pid and write, which pm_lock takes) and tells the caller to stop a hung process.
A lease cannot tell a stalled holder from a slow merge over every item, and a holder past its lease could still commit; a dropped connection is already taken over within 50 ms, so only a live stalled pm process blocks writes, which naming it lets the owner end.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Load through the service-held store (Done when 2): on a copy of this repo's
  store (590 items, 8-core M1, optimized go test binary, Yeeef/pm 9474374), a
  fresh process that connects, checks pm_version(), loads every item in one
  read-only transaction and disconnects takes 5.6 ms median over 10 runs (min
  5.4, max 7.2), against 35.8 ms median (min 35.1, max 54.1) for the
  per-command open on the same data (a32ce1b). End to end the pm export
  --store process takes 36.4 ms median against 68.3 ms; the 110 MB binary's
  process start is most of it.

- 8 processes x 20 writes (Done when 3), a new connection per write, comments
  on 3 shared items, same copy and machine: 160 of 160 in 2.40-2.71 s over 10
  runs with no lock wait, 190-243 retries per run, at most 22 attempts for one
  write; the per-command store with its gate took 6.95 s for 160 of 160. At
  the designed bound of 20 attempts one run in 7 lost a write; each attempt
  loses with odds near 7/8 under 8 writers, so the bound is 100 (about 2.5e-4
  chance a run loses a write).

- A pm service per test costs about 80 ms: pm service run answers a pm export
  115 ms median after its start (10 runs, the export about 36 ms of it). The
  Go shared suite ran 134 passed in 61.7 s against 52.7 s before; make
  test-go-suite took 1:09 against 1:46.

- The per-test service exposed a race the installed service has too: its site
  refresh ran git status in the records store for the design pages' dates,
  whose opportunistic index.lock failed a concurrent pm design new (Unable to
  create index.lock); git --no-optional-locks status fixes it. And Dolt skips
  a fetch within 1 s of the service's last read of a git remote, so tests that
  need a sync to see the other clone's push wait that out; a push always
  fetches, so the compare-and-swap still sees a moved remote.

- Review of the service-held store: a pull racing local writers that never
  stop can lose every one of its 100 merge attempts (fail-safe: the store
  stays as it was, the next sync lands it). The reviewer saw 1 of 12 rounds
  fail with 3 tight-loop writers, 2178 writes landing exactly once; my rerun
  saw 1 of 6 pulls starve, 1758 writes landing once each. A pull now fetches
  once and only its merge retries; the starvation itself is left, being
  fail-safe. Also from the review: GC holding the operation slot deadlocked
  against a waiting CALL pm_sync until GCTimeout; with GC outside the slot, 10
  collections racing pm_sync and pm_create loops all succeeded, the slowest in
  416 ms; a sync on a remote whose ssh never answers now ends at its 1 s test
  bound, its git killed.

- Second review of the service-held store (branch service-held-store at
  86403d2): the GC deadlock fix, the operation slot and the ErrMergeNeeded
  retry hold; optimistic writes still starve long merges: under -race 4 of 15
  creates failed against one writer pausing 0-6 ms, and a create whose CAS
  merge starves reports an error although its item is on the remote

- Write lock in place of optimistic retries, re-measured on a copy of this
  repo's store (590 items, M1, Yeeef/pm e2d021e): 8 processes x 20 writes land
  160 of 160 in 2.11-2.17 s over 10 runs (per command 6.95 s; with retries
  2.40-2.71 s), a write waiting 88-90 ms on average for the lock, at most 109
  ms, about the 7 writes queued before it, while reads never wait; a
  fresh-process load of every item takes 6.2 ms median (min 5.5, max 7.5).
  Under -race -count=5 the race tests (pull and pm_create against tight-loop
  writers, gc racing pm_sync and pm_create, 8 x 20 in process) all pass with
  zero failed writes, pulls or creates; they cost about 57 s per run under
  -race, so make test-go runs them once under -race, with internal/service (18
  s).

- Third review (86403d2..efb888c): the fair write lock held under review and
  scratch tests (six queued waiters served in order, a dropped holder taken
  over in 62 ms, every write to main under the lock); a create could be minted
  twice when a timed-out push lands after the check fetch (simulated: two
  items from one create), and a live but stalled lock holder fails every
  writer after 60 s

- Third review of the write lock: mutual exclusion, FIFO order, takeover of a
  dropped holder in 62 ms, every main write under the lock and no false
  refusals from the working-set guard held up. It found a create minted twice
  after a push timeout (the dropped push landing after the check fetch;
  simulated: items .1.3 and .1.4), now refused as an outcome unknown
  (TestCreateWhosePushLandsAfterItsCheckMakesTheItemOnce leaves one item), and
  pm_lock truncating a sub-second wait to 0 s, now milliseconds. An unfair
  handoff (LIFO, a mutation check) fails both the FIFO lock test and the
  long-write test, whose writers then starve to the 60 s bound.

- Local release dry run (0.3.0-rc.1 built from Yeeef/pm bee2d94, nothing
  published) on a scratch copy of this clone: the service migrated the store
  from schema v2 to v3; pm export byte-identical before and after (592 items,
  658,078 bytes); pm doctor clean before and after; the site identical on all
  153 pages after normalisation except a push-failure banner caused by the dry
  run's faked claude; pm export 66 ms median through the launcher vs 98 ms on
  0.2.2; binary 110,235,330 bytes vs 111,396,658. A clone still on 0.2.2
  cannot pull a v3 remote or add tasks until it upgrades, and fails cleanly
  naming pm upgrade; after its own upgrade it syncs.

- A worktree that pins 0.3.0 while main and the live service are still on
  0.2.2 gets 'the pm service does not answer on .pm/run/work.sock … run pm
  service restart' for every store command; the advice is wrong in that case
  (a restart stays on main's 0.2.2 pin, which has no socket). The message
  should name the pin difference when main's pin differs from the checkout's,
  as the version refusal does once a service answers.

- Live upgrade to 0.3.0 (pin PR merged as fc97a69): the service migrated the
  live store to schema v3; pm doctor clean; pm export keeps all 592 baseline
  items byte-equal and adds only the two needs raised after the baseline (594
  items); the site's 153 pages equal the baseline after normalisation except 4
  pages that show those writes; pm export 74 ms median through the launcher
  (51 ms direct) vs 98 ms for 0.2.2 on the dry-run copy; a worktree pinned to
  0.2.2 is refused with 'its schema is version 3, newer than this pm's 2: run
  pm upgrade' and writes nothing to the store; pm push synced the v3 store to
  GitHub (work and records 0 ahead). Backup and baselines:
  ~/pm-backups/yeeef-agents-work-20261009T202413Z-pre-0.3.0/.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: every pm command reaches the work store through the pm service, released as [pm 0.3.0](https://github.com/Yeeef/pm/releases/tag/pm-v0.3.0) and pinned here.

Merged as 3fd1e73 (PR #5). Merged as fc97a69 (PR #101).

- The pm service alone holds the Dolt work store and serves it on a Unix socket. Every command is a SQL client with a version handshake, and fails hard naming `pm service restart` when the service is down.
- Writes are serialized by a fair write lock in the service. Sync, the child-id compare-and-swap, setup and gc run in the service, with time limits on their remote steps.
- The gate, its wait log and the slow-work rule are gone.
- Three adversarial reviews and a fix check found five defects before the release, and all five were fixed: a GC deadlock, unbounded syncs, starving merges, a merge cut partway, and a duplicate create after a late push.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | State | Evidence |
|---|---|---|
| No command opens the embedded store; every store command fails hard with the service stopped, naming `pm service restart` | met | `TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt`; `test_every_store_command_fails_hard_with_the_service_stopped` over 39 commands; the dry run's stopped-service check |
| Loading every item in a fresh `pm export` at most 20 ms median over 10 runs on this repo's data | met | Connect, version check and load every item: 6.2 ms median on a copy of this repo's store (590 items), against 35.8 ms for the per-command open. A whole `pm export` process on the live clone is 51 ms direct and 74 ms through the launcher, against 98 ms for 0.2.2 |
| 8 processes × 20 writes: 160 of 160 correct, no lock wait | met, with a bounded queue | 160/160 in 2.11–2.17 s, against 6.95 s per-command. There is no store-lock wait; each write waits 88–90 ms on average (at most 109 ms) in the service's fair write lock |
| A version mismatch is refused or resolved by a restart, with a test | met | `TestAServiceOnAnotherVersionIsRefused` checks both refusal texts. Live, a worktree pinned to 0.2.2 is refused with "its schema is version 3, newer than this pm's 2: run pm upgrade" |
| Released from Yeeef/pm and pinned here: `pm doctor` clean, `pm export` equal before and after, the site equal after normalisation | met | pm-v0.3.0 at 3fd1e73; pin PR merged as fc97a69. `pm doctor` rc 0. All 592 baseline items are byte-equal, plus the 2 needs raised after the baseline. 153 pages, equal after normalisation except the 4 that show those writes |
| The sprint's PRs are on main in Yeeef/pm and here | met | [Yeeef/pm#5](https://github.com/Yeeef/pm/pull/5) merged as 3fd1e73; [yeeef-agents#101](https://github.com/Yeeef/yeeef-agents/pull/101) merged as fc97a69 |
