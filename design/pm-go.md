---
type: design
title: pm in Go
project: pm-harness
---

## Problem

> What are we solving, and why now?

The owner decided three things: pm replaces Beads with its own work store, the work store keeps its items in Dolt embedded in pm, and pm is written in Go. Dolt embeds only into Go programs, and a Go pm ships as one binary that starts faster than Python.

- pm on bd paid 3–4 s in `bd` on each of 20 commands (`pm show`, `pm task add`, `pm task claim`, `pm finding add`, …) before its own work: `bd list --all --json` takes 1.16–2.09 s and `bd export` 1.69–2.28 s (load average 70 on 8 cores).
- This page holds pm's Go design: the architecture, store access, the site, the distribution and the tests. Go is pm's one implementation; the Python pm it replaced is retired, and a pin to a Python release (below 0.2.0) fails hard (Distribution).
- [Work store](work-store.md) holds the data model, ids, ready, commands and migration; its Storage section is embedded Dolt. [pm as an installable product](pm-product.md) holds the managed pieces and the version pin; this page holds the launcher. Part of the [Project management harness](pm-harness.md) design.

| Term | Meaning |
|---|---|
| pm | The Go module `github.com/Yeeef/pm`, built as one binary; first release 0.2.0 |
| Work store | pm's store of items; the Dolt database inside Go pm, at `<main checkout>/.pm/store/work` |

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- One Go binary is all of pm: CLI, hooks, site, service and launcher. Runtime needs are `git`, plus `gh` and `claude`; pm runs no `bd`, `uv` or Python.
- A command that loads every item finishes in ≤ 300 ms in a fresh process (the work-store goal).
- Every command, refusal text, record file, help text and page is held by tests: the black-box harness and the Go golden tests (Tests).
- Each repo moves to a new pm in one step, by its pin, and back by moving the pin.
- Every merge to `main` is releasable: CI green and pm building on both targets.

**Non-goals**

- Changing the work store's data model, ids, ready rules or commands; the [Work store](work-store.md) page owns them.
- Moving a clone off the pre-package harness: `pm init` refuses such a clone and names the retired Python release that moves it (Distribution).
- Targets other than darwin/arm64 and linux/amd64.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

**Embedded Dolt in Go, measured** by the spike in [sprint 77](../sprints/pm-harness-77.md) (`dolthub/driver`, as bd uses it; 8-core M1 at load average 20–50).

::: result {title="Embedded Dolt, pm's workload, 466 items"}
| Measure | Result |
|---|---|
| Open, load all items with labels, dependencies and comments, close; fresh process | 64 ms median, 68 ms p90 (open 21, queries 2.7, close 14; n ≥ 10); 68 ms after 3,000 more commits |
| One write with `DOLT_COMMIT`, fresh process | 70 ms |
| Engine lock | Exclusive on the store while open, readers included |
| 8 processes × 20 writes | 160 of 160, 7.7 s total, p90 wait 468 ms with the driver's open retry; a flock gate bounds the max wait to 524 ms |
| A process holding the store open | Blocks every other process |
| Load through a service that holds the store, over a Unix socket | 7.8 ms in the spike; 5.6–6.2 ms median in a fresh process on 590 items, connect and version handshake included (the service-held store, measured) |
| Sync through a `git+file` remote, `refs/dolt/data` | push about 600 ms, no-op pull 180 ms |
| Merge | Different cells of one row merge; a same-cell edit lands in `dolt_conflicts_items` with base, ours and theirs |
| Build | needs cgo and `-tags gms_pure_go`; stripped darwin/arm64 spike binary 107 MB; linux/amd64 via `zig cc` 132 MB, not yet run |
:::
Reading: opening once per process meets the 300 ms goal about 5 times over, but every reader then waits on the engine's exclusive lock; a service that holds the store and serves it over a Unix socket loads in 7.8 ms (5.6 ms measured on 590 items) and makes no command wait, which is how pm runs (Store access).

- bd's own `bd list --all --json` profiles at 567 ms: about 410 ms in 9 engine opens and closes (one per transaction), 117 ms cleaning a git remote cache on close, about 105 ms more process start than a plain Go binary; the query is about 36 ms. pm opens once per process and avoids all of it.
- The released pm 0.2.2 (darwin/arm64) is 111,396,658 bytes and already links `go-mysql-server/server`, `vitess/go/mysql` and `go-sql-driver/mysql` through `dolthub/driver`, so serving the store adds little; Dolt's `commands/sqlserver` would add about 41 packages (MCP, Prometheus, gopsutil). Neither size is measured.
- Store size after about 380 commits on 466 items: 14 MB, 3.0 MB after `dolt gc`. bd's store here is 123 MB for 596 KB of exported data, 47 MB after `dolt gc` on a copy.
- bd 1.3.1 (darwin/arm64, go1.26.7) builds with `CGO_ENABLED=1` and `-tags gms_pure_go,netgo`; it links `dolthub/gozstd`, a cgo binding, through `dolthub/dolt/go`. Its binary is 144,181,568 bytes.

**Behaviour kept from the Python implementation.** pm kept its outputs when it moved to Go; these choices follow from that.

| Item | Design |
|---|---|
| Regexes | Go `regexp` is RE2: no lookarounds, no backreferences, and `\w` is ASCII only. The 9 patterns that needed them (section splits, sentence and word splitting, the id match) are scanning code, each with its own table test, not `regexp2` |
| Character counts | `hooks.CAP` and the sentence and word limits count runes (`utf8.RuneCountInString`), not bytes: `prime.md` has more bytes than characters |
| YAML | front matter keeps YAML 1.1 typing (`yes`, `on` and dates are typed), so headers quote as they always did; `internal/records/testdata/expected.json` holds the cases |
| JSON | `internal/pyjson` reads and writes JSON with Python `json`'s default separators, so stored and exported JSON keeps its form |
| Refusal texts | `prime.md` and the harness cite them word for word |

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Architecture

pm is one Go module at the root of the `Yeeef/pm` repo: `go.mod` (module `github.com/Yeeef/pm`), `cmd/pm`, `internal/…`, and `assets.go` with the files pm embeds. Release tags are `pm-v<X>`.

```mermaid
flowchart TD
  main[cmd/pm] --> launch
  main --> cli
  cli --> hooks & site & service & install & work & records & store & config
  service --> site & work & store & sync
  sync --> work & store
  site --> records & work
  hooks --> work & records
  install --> config & store & work & service
  work --> dolt[(Dolt, embedded in the pm service: dolthub/driver)]
  store --> git[(git CLI)]
```
Reading: only `work` touches Dolt: in the service it opens the engine and serves it on a Unix socket, everywhere else it is a SQL client of that socket. Only `store`, `sync` and `install` run `git`; everything else sees typed items and record structs.

| Package | Holds |
|---|---|
| `cmd/pm` | `main`: `launch.Maybe()` first, then `cli.Execute()` |
| `internal/cli` | Command tree, help texts, command bodies, `Refuse` error type and exit codes |
| `internal/config` | `.pm/config.toml` read, pin check, `PORT` |
| `internal/launch` | Pins, downloading release binaries, `exec`; a pin below 0.2.0 refused |
| `internal/work` | The `Item` type and the store interface; Dolt schema and migrations, validation, ids and minting, ready and blocked, merge rules on conflict rows, the host (the engine and its socket server) and the client (one connection per command), `--import-bd`, `export` |
| `internal/records` | Record parse, sections, blocks, templates, checks |
| `internal/store` | The `records` store: find it, flock, atomic multi-file writes, commit |
| `internal/site` | Page templates, the Markdown renderer, `style.css` (embedded) |
| `internal/service` | `service run` (the work store held open and served on its socket, the operations commands ask for, HTTP, snapshot, reply spool, merge poll, inbox push, sync and gc loops), unit files |
| `internal/sync` | Records push and work-store sync, run by the service and `pm sync` |
| `internal/hooks` | `prime` (`prime.md` from `assets.go`, chunks), `hook stop`, `hook owner-request`, git hooks |
| `internal/install` | Managed pieces, `init`, `doctor`, `upgrade`, `uninstall`, `where` |
| `internal/proc` | The subprocess helper: argv, cwd, captured text output, timeout |
| `internal/pyjson` | JSON read and written with Python `json`'s default separators |
| `internal/buildinfo` | The version, stamped by the release build |

- **Embedded files.** `//go:embed` reads only files at or below the package directory, so `assets.go`, package `pm` at the module root, embeds `prime.md`, `style.css` and the model prompts in `prompts/`. The test that checks the noun list against `prime.md` reads the cobra tree.
- **Libraries.** `BurntSushi/toml` reads `config.toml`; pm writes it from a template, as today. `go.yaml.in/yaml/v3` (the maintained yaml.v3) for front matter, with a quoting function that follows YAML 1.1, tested on every header in the corpus. The launchd plist comes from a text template; `internal/install`'s golden test holds it.
- **External tools stay external.** `git`, `gh`, `claude` and `launchctl`/`systemctl` run through `exec.LookPath` on `PATH`, never an absolute path, so the test fakes keep working. pm runs no `bd` and no `uv`.

### CLI framework

`spf13/cobra` (pflag inside). It gives nested commands, flags interspersed with positionals (`pm task close <id> --reason …`, `pm finding add --sprint ID "<text>"`), `MarkFlagRequired`, `MarkFlagsMutuallyExclusive` (`--bead`|`--project`), per-command help and hidden commands for machinery. bd 1.3.1 uses it (v1.10.2), and the binary already links about 214 modules through Dolt, so 2 more change nothing measurable.

- Each command's help text is its `Long`, in cobra's default layout; the harness asserts the text, not the layout (Open questions).

### Store access: the service holds the store

The pm service is the only process that opens the work store. It starts the Dolt engine on `<main checkout>/.pm/store/work` when it starts, keeps it open for its whole life, and serves it with go-mysql-server's MySQL-protocol server on a Unix socket. Every pm command that reads or writes items is a SQL client of that socket behind `work.Store`; the service reaches its own store through the same socket. There is one access path: no command opens the store itself, and nothing falls back to a direct open.

```mermaid
flowchart LR
  cmd[pm command] -- "one connection: SQL, CALL pm_*" --> sock[(.pm/run/work.sock)]
  svc[pm service: site, replies, loops] -- SQL --> sock
  sock --> srv[go-mysql-server]
  srv --> eng[Dolt engine, open for the service's life]
  eng --> dir[(.pm/store/work)]
  srv -- "pm_sync, pm_create, pm_setup" --> ops[service operations] -- fetch, push --> remote[(git remote, refs/pm/work)]
```
Reading: commands and the service's own loops are all clients of one server in the service process; only the service touches the store directory and the remote.

| Who | Reaches the store | Holds it |
|---|---|---|
| A `pm` command that reads or writes items | One connection to the socket, on first use | Until it exits |
| A `pm` command with no work data (`prime` without `--state`, `hook stop`, `record link`) | Never | — |
| `pm service run` | Opens the engine at start; its site, reply writer and loops use connections to its own socket | Its whole life |
| Any MySQL client (`mysql -S <main>/.pm/run/work.sock work`) | The socket | Its connection; a write outside pm skips pm's checks, so ad hoc use is for reading |

**The host** (`internal/work`, `host.go`): `work.NewHost(o HostOptions) (*Host, error)`, run only by `pm service run`.

- The engine is built as `dolthub/driver` builds it, since the driver's `Connector` keeps its engine unexported: `embedded.LoadMultiEnvFromDir(ctx, config.NewMapConfig({user.name: pm, user.email: pm@localhost}), filesys.LocalFS.WithWorkingDir(dir), dir, version, {dbfactory.DisableSingletonCacheParam: {}, dbfactory.FailOnJournalLockTimeoutParam: {}})`, then `engine.NewSqlEngine(ctx, mrEnv, &engine.SqlEngineConfig{ServerUser: "root", Autocommit: true, ...})`. No `embedded.Connector` is opened beside it: that would be a second engine on one journal.
- The server: `server.NewServer(server.Config{Listener: ln, Version: ..., MaxConnections: 0}, se.GetUnderlyingEngine(), se.ContextFactory, sb, nil)` from `github.com/dolthub/go-mysql-server/server`, with the session builder `sb = func(ctx, c, addr) { bs, err := sql.BaseSessionFromConnection(ctx, c, addr); return se.NewDoltSession(ctx, bs) }`; then `sqlserver.SetRunningServer(srv)` (`dolt/go/libraries/doltcore/sqlserver`), as Dolt's own server does, and `srv.Start()`. Dolt's `cmd/dolt/commands/sqlserver.Serve` is not used: it always opens TCP, writes `privileges.db` and creds files, starts metrics, and links about 41 more packages. The server package and `go-sql-driver/mysql` are already in the binary through the driver.
- At start, in order: load the engine (another process holding the store fails hard: "work store <dir>: another process holds it (a second pm service?): <err>"; a database the engine would open read-only, its lock taken, counts as held); remove a socket file left by a crash, which is safe once the engine is held; listen; register `pm_version()` and the operations below; serve; migrate an older schema through a connection of the host's own (only the service migrates). `pm_version()` answers only once the schema is current, so a command that connects meanwhile waits in its handshake and never sees the old schema. The server's own log stays at error level: a refusal is the command's to report.
- `Host.Close()` stops the server (Go's listener unlinks the socket), then closes the engine.

**The socket.**

| Part | Design |
|---|---|
| Path | `<main checkout>/.pm/run/work.sock`, one per clone, beside the service log. A path of 104 bytes or more (macOS's `sun_path`; 108 on Linux) fails the service's start hard: "the pm service's socket <path> is N bytes, over this system's limit of 103; move the clone to a shorter path". Worktrees do not lengthen it: the socket is under the main checkout |
| Listener | `net.Listen("unix", path)` by the host, passed as `server.Config.Listener`, so go-mysql-server opens no TCP listener and does no socket handling of its own (it neither removes a stale file nor sets a mode). No TCP port serves SQL; the site keeps its HTTP port |
| Mode | 0600: the host sets the umask to 0177 around `Listen`, so the file is never wider. The file mode is the access control |
| Auth | none in the engine: built this way, Dolt's `MySQLDb` stays disabled and accepts any user. Clients connect as `pm` with no password |

**One connection per command** (`work.Dial(main string) (*Dolt, error)`).

- The client is `go-sql-driver/mysql` through `mysql.NewConnector(&mysql.Config{User: "pm", Net: "unix", Addr: sock, ParseTime: true, Loc: time.UTC})`: a built config, never a DSN string, which a path with `@` or `)` would break. `sql.OpenDB`, `SetMaxOpenConns(1)`, one `*sql.Conn` held until the command exits (`Shutdown` closes it). A connect waits at most 2 s.
- On connect: `SELECT pm_version()` (the handshake below), then `USE work`.
- A read is one read-only transaction (`START TRANSACTION READ ONLY` … `COMMIT`), so the five queries of a load see one commit while others write. A write is one transaction ending in `CALL DOLT_COMMIT('-Am', msg)`, which commits the SQL transaction too.
- Nothing is held across slow work: an open connection blocks no one, so a command keeps it while it calls `gh`, the owner-request judge or a records push. The rule against holding the store across slow work, the gate (`.pm/run/work.lock`) and its wait log (`work-gate.log`) are gone.
- A command that writes both stores takes only the `records` store's lock; there is no second lock to order it with.
- `pm export --store DIR` reads through the service of the clone that `DIR` belongs to (`DIR/../../run/work.sock`).
- A connection that breaks mid-command (the service restarted or died) fails the command hard; it does not reconnect, since a write cut off in `DOLT_COMMIT` has an unknown outcome.

**Version handshake.** The host registers `pm_version()` on its engine's catalog (`Analyzer.Catalog.RegisterFunction`), returning the service's `buildinfo.Version`; per engine, not process-global, so a test can host another version. The command compares it with its own version right after it connects, on the connection it then uses, so no restart can come between check and use.

| Service's version | The main checkout's pin | The command |
|---|---|---|
| Equal to the command's | any | proceeds |
| Differs | equals the command's: the service is stale (a pin move it has not acted on yet, or pm replaced under it) | fails hard: "the pm service runs pm S, not pm V: run pm service restart". The service already stops itself within a second of a pin move (`pinMoved`) and its supervisor starts it on the new pin through the launcher, so this lasts only that long |
| Differs | differs from the command's: this checkout pins another version than the main checkout (a pin-moving branch) | fails hard: "this checkout pins pm V, but the clone's pm service runs pm S, which <main>/.pm/config.toml pins: run it from a checkout that pins pm S" |

A command never restarts the service: parallel commands would restart it many times over, and a restart from a mismatched checkout would bring up the wrong version.

**The write lock.** Every write transaction (an ordinary write, a pull's merge, the compare-and-swap's merge) runs under the store's write lock, taken with `CALL pm_lock(ms, pid, write)` on its connection before its first read (the wait's bound in milliseconds, rounded up, and the client's pid and write, which a timeout names) and released with `CALL pm_unlock()` after its commit or rollback. The host keeps it (`lock.go`): FIFO, so writers get it in the order they asked; a waiter that finds the holder's session gone from the server's session manager (its connection dropped) takes it over. go-mysql-server's own `GET_LOCK` is not used: it polls a compare-and-swap every 100 µs and keeps no order, so a writer could wait behind a stream of later ones. Writes one at a time are serializable and none starves; reads take no lock and never wait. A write waits for the writes queued before it, a few to tens of ms each: in the 8 × 20 benchmark, 88–90 ms on average, at most 109 ms, against the gate's exclusive wait for any reader or writer. Every write still sets `write_stamp.txn`, so one that raced a write outside the lock fails hard on Dolt's serialization failure (MySQL error 1213, SQLSTATE 40001) or `ErrMergeNeeded`, both of which land nothing; there is no retry. [Work store](work-store.md), Data model, "Concurrent writers", gives the rules and the failure texts.

**What runs in the service.** The operations that move `main` against the remote, or that a session must not race, run in the service, one at a time in its operation slot; reads and ordinary writes never wait on it. Each runs within its bound, the wait for the slot included: a sync `SyncTimeout` (120 s, the service's loop's and `pm_sync()`'s), a create `CreateTimeout` (180 s), a setup `SetupTimeout` (300 s). What the bound stops is the operation's statements that reach the remote, fetch, push and clone, each on a connection of its own: the client drops it, the operation fails and frees the slot, and the server may still finish the dropped statement, until Dolt kills its git, overlapping the next operation's; a local merge runs to its end. Setup's and the sync's `git ls-remote` run within the bound too. So an operation that outlasts its bound frees the slot; a caller still waiting at its own bound fails hard, naming the operation in the slot, with nothing written. A command asks for one with a stored procedure on its one connection; the host registers each as `sql.ExternalStoredProcedureDetails` on the engine's provider (`Catalog.DbProvider.(*sqle.DoltDatabaseProvider).Register`), and its body runs the operation through the service's own connections.

| Operation | When | A command asks with | Returns |
|---|---|---|---|
| Sync: fetch, pull with conflict resolution by the merge rules, push | every 600 s; `pm sync`; `pm push`'s work step | `CALL pm_sync()` | one row per line: what it did, then a warning per claim a merge overrode |
| Child id compare-and-swap | `work.Store.Create` of a child in a store with a remote (`pm task add`, `pm sprint open`, a need) | `CALL pm_create(?)`, the `work.New` as JSON | the created item as JSON |
| Setup: clone from the remote, or create and push, or attach a remote | `pm init` | `CALL pm_setup()`, on a connection with no database | one row per line of what it did; its refusals, `install.SetupWork`'s texts, as the error |
| Garbage collection: `CALL DOLT_GC()` | every 24 h (`GCInterval`), `GCTimeout` 10 min | none | logged; `.pm/run/gc.json` for `pm service status` |

- **Pull.** Fetch outside any transaction, once per pull. When behind, check the remote's head before merging: its schema version (refused when newer: "run pm upgrade") and every invariant, read on the revision database `work/<hash>`. Then merge in one write transaction under the write lock: `DOLT_MERGE('--no-commit', 'origin/main')`, resolve `dolt_conflicts_items` by the merge rules, keep this side's `write_stamp`, clear the holder of every closed item, check, set a fresh `write_stamp` as every write does, `DOLT_COMMIT`. A fast-forward moves `main` at once (under the lock no session commits meanwhile); a 3-way merge lands at its `DOLT_COMMIT` or rolls back. The merge runs on a background context, to its end, past the operation's bound: Dolt's SQL fast-forward is two root updates (the branch head, then the working set), and a merge cut between them would leave the working set behind the head. A failure says whether `main` had fast-forwarded to the checked head, read on a live connection, or that this is unknown. The checked head makes a failure after a fast-forward impossible, so the service never runs `DOLT_RESET --hard` on `main`, which would set the head with no check and drop other sessions' commits.
- **Push.** `DOLT_PUSH` of `main` outside any transaction, within `PushTimeout`; rejected as non-fast-forward, the sync pulls again, up to 3 attempts.
- **Compare-and-swap** on the scratch branch `pm-cas`: [Work store](work-store.md), Ids.
- **gc** runs online: Dolt's session-aware safepoints (the default; `dprocedures.UseSessionAwareSafepointController`) let open connections go on. It takes no operation slot, one collection at a time: `DOLT_GC` waits for every session in the middle of a statement, and a command in `CALL pm_sync()` or `pm_create()` is in the middle of one while it waits for the slot, so a collection holding the slot would wait for it until `GCTimeout`.
- The site's change mark is `HASHOF('main')`, read each look; the file fingerprint goes.
- Dolt skips a fetch within 1 s of the service's last read of the same remote (its git blobstore's read dedup), so a sync right after another reads nothing new; a push always fetches first, so a compare-and-swap still sees a moved remote and retries.

**Startup.** The service must run for any store command, so pm makes sure it is up.

| Who | Does |
|---|---|
| `pm init` | sets the clone and worktree up (records store, link, hooks), installs and starts the service (`service.Install`, waiting until its site answers), then asks it to attach the store (`CALL pm_setup()`). The service starts on a missing store: the engine runs on the empty directory, the socket answers, and the site serves the error "this clone has no work store yet: run pm init". The post-checkout hook sets a worktree up and leaves the store alone |
| `pm init --import-bd FILE` | through the service: on a clone with no store, makes an empty one first (`CREATE DATABASE` and the schema, on a connection with no database), then imports into it as one write |
| Session start (`pm init --session-start`) | when the service is installed, enabled and not answering, starts it (`service.Restart`, under the clone's install lock, so parallel session starts start it once); a running service, a stale one included, is left as it is and reported with `pm service restart`; a stopped one (below) is left stopped and reported with `pm service restart`. A service that answers on this version is then asked to attach the store (`CALL pm_setup()`). A start waits at most `RestartWait` (15 s) of the hook's 18 s |
| `pm service restart`, `install` | as today; the unit runs the bin-dir pm, which runs the main checkout's pin. Both enable a stopped unit first, so a typed `pm init` or `pm service restart` ends a stop |
| `pm service stop` | disables the unit and stops it: `systemctl --user disable --now`, or `launchctl disable` then `bootout`. The supervisor's disabled state is the one record of a stop (no file of pm's): the supervisor then starts it neither at login nor after a crash, and session start reads it (`systemctl --user is-enabled`, `launchctl print-disabled`). It then waits `RestartWait` until neither the socket nor the site port answers, and fails hard, naming what still answers, when one does (a `pm service run` started by hand) |
| `pm uninstall` with the service down | the unsynced-work check needs the store, and only `pm service run` may host it, so pm uninstall runs one for the check: the running pm's own binary as `pm service run` in the main checkout, on `PORT=0` (no site port taken), its output kept; it waits `RestartWait` for the socket, runs the check through `work.Dial` as every command does, then stops it (SIGTERM, as the supervisor stops the service) before it removes anything. It fails hard, with that service's output, when it does not come up (another process holds the store). A clone with no `.pm/store/work` has nothing to check and starts nothing |
| A pin move in the main checkout | the service stops itself within `ServeCheck` (1 s) and its supervisor starts it on the new pin |

**What fails hard**, with nothing written:

| Condition | Message (stderr, exit 1) |
|---|---|
| No socket, or nothing answers on it | "error: the pm service does not answer on <main>/.pm/run/work.sock; pm reaches the work store only through it: run pm service restart" |
| The service runs another version | as under Version handshake |
| No `work` database (setup never ran) | "error: this clone has no work store yet: run pm init" |
| A write waited `WriteLockWait` (60 s) for the write lock | "work store: <the write> waited 60s for the store's write lock, which a live pm process holds (connection <n>, pid <p>, <its write>, for <d>); nothing was written. If that process hangs (stopped, or in a debugger), stop it; then run the command again" (the lock has no lease: [Work store](work-store.md), Concurrent writers) |
| A create's push timed out or failed but by a non-fast-forward rejection, and its commit is not on the remote | "work store: create <id>: …: the outcome is unknown, and the push may still land as <id>. Nothing was merged here: run pm sync, then check with pm show <id> before you create it again" |
| A write raced one outside the write lock | "work store: <the write> conflicted with a write that did not take the store's write lock (a SQL client past pm?); nothing was written: run the command again" |
| The working set differs from `main`'s head at a write's start | "work store: <the write> found the store's working set differing from its head in <tables>, which no pm write leaves …; nothing was written", naming how to look and drop it |
| An operation waited for the slot to its bound, or outlasted it | "work store: the <operation> waited <bound> for the pm service's <operation>, running for <d>, and gave up; nothing was written: run the command again, and see pm service logs if it recurs"; "work store: the <operation> did not finish within <bound> and was stopped: <err>" |
| The connection broke mid-command | "error: the connection to the pm service broke (<err>); a write in flight may or may not have landed: check with pm show, then pm service status" |
| Service start: another process holds the store, or the socket path is too long | the host's texts above, in the service log; `pm service status` shows the service down |

**Tests.** A test that needs a store starts its own service.

- Go: `worktest.Serve(t testing.TB) (*work.Host, *work.Dolt)` makes a clone directory with `os.MkdirTemp("", "pm")` (short enough for the socket on macOS, where `t.TempDir()` paths may not be), starts `work.NewHost` on it, makes an empty store, dials one client, and stops both in `t.Cleanup`. Sync and compare-and-swap tests run two hosts against a bare remote: one clone creates through `CALL pm_create`, the other runs the compare-and-swap in process so the test can hold its push; a sync that must see the other clone's last push waits out Dolt's 1 s read dedup. A version test hosts `pm_version()` returning another version and checks both refusals.
- The concurrency test: 8 connections × 20 writes, 160 of 160 correct, invariants checked after; two opposite `pm dep add` at once: exactly one lands, the other refuses the cycle; a long write behind 3 tight-loop writers gets the lock in its turn; a write racing one outside the lock fails hard. The race tests (a pull and `pm_create` racing tight-loop local writers, zero failures asserted; gc racing `pm_sync` and `pm_create`) also run under `-race` in `make test-go`. The benchmarks (`bench_test.go`, with `PM_BENCH_STORE` naming a copy of a store) run 10 fresh-process loads and 8 processes × 20 writes.
- pytest: the `repo` fixture starts `pm service run` for the clone (a free port, its socket under a short `--basetemp`), waits for the socket, seeds through it (a re-import restarts it on a removed store), and stops it at teardown. This per-test service does not make a test `integration`; `integration_only` keeps naming the service lifecycle tests. A test that runs the clone's own `pm service run` stops it first and starts it after, and the fake supervisor stops it when it starts the clone's installed service.
- The access-path test: with the service stopped, every store command fails with the message naming `pm service restart` and writes nothing (pytest, 39 commands); and only `host.go` calls into `dolthub/driver` or Dolt's engine package (`LoadMultiEnvFromDir`, `NewSqlEngine`), and only `pm service run` and `worktest` call `work.NewHost`: a Go test over every non-test file's calls, resolved by import path with `go/parser`, so a new direct open fails CI.

### Site

| Part | Choice |
|---|---|
| HTTP | `net/http` on `127.0.0.1:$PORT`; GET, HEAD and POST as `cli.Handler` does today. Goroutines replace the 3 threads (snapshot, reply writer, merge poll) |
| Page assembly | `html/template`: one template per page kind, with the Markdown body passed in as `template.HTML` from goldmark's renderer. No hand-written escaper |
| Markdown | `github.com/yuin/goldmark` (CommonMark 0.31.2; bd links v1.8.5) with `extension.Table` and `WithTableCellAlignMethod(TableCellAlignStyle)` (writes `style="text-align:…"` as markdown-it does); `html.WithUnsafe()` for records, a second renderer without it for comments and replies (`COMMENT_MD`); no linkify, no typographer, no auto heading ids |
| Custom goldmark parts | front matter skip; heading ids for h2–h3 with the mdit-py-plugins `anchors` slug and duplicate suffixes; a `:::` container parser for `note`, `decision` and `result`, with the attribute parser from `records.attrs` and the required attributes (`BLOCK_ATTRS`); a `mermaid` fence renderer (`<pre class="mermaid">`); the link rewrite `.md(#…)` → `.html(#…)`; the TOC from the heading nodes |

**Golden pages.** `internal/site/golden_test.go` renders every page of `testdata/constructs` (a fixture for each construct a page renders: projects, sprints, designs, docs, postmortems, days, the index) and compares it with its copy under `testdata/constructs/pages`. After an intended change, `go test -tags gms_pure_go ./internal/site -run Golden -update` rewrites the copies, and the PR shows the diff for review.

### Distribution

| Part | Design |
|---|---|
| Machine install | `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh \| sh && pm init`. `install.sh` picks the asset for `uname -s`/`-m`, checks it against `SHA256SUMS`, installs to `${PM_BIN_DIR:-$HOME/.local/bin}/pm` |
| Launcher | the installed binary |
| A pin's binary | `$XDG_DATA_HOME/pm/pins/<pin>/pm`, downloaded once from release `pm-v<pin>`, checked against `SHA256SUMS`, written atomically; its sha256 is kept in `pins/<pin>/sha256`, and a later download that differs fails hard |
| Runtime needs | git; `gh` and `claude` |
| `pm init` | copies the running binary (`os.Executable()`) into the bin dir when it is not there, replacing a link there (such as the retired pm uv tool's) rather than following it |

**The launcher.**

| Case | What `pm` does |
|---|---|
| No readable pin, or the pin is this binary's version | Runs in process |
| Pin ≥ 0.2.0 and not this version | `syscall.Exec` of `pins/<pin>/pm`, downloading it first if missing (10 s connect timeout, 300 s download); a missing release or asset, a checksum mismatch or a timeout is a hard error that names `pm-v<pin>` |
| Pin < 0.2.0 (a retired Python release) | Fails hard, naming the fix: move the pin to a release from 0.2.0 on with `pm upgrade --to <X>` |
| `PM_LAUNCHED=<pin>` set | Runs in process, then the config check; removes `PM_LAUNCHED`/`PM_LAUNCHER` from its children's environment |
| `pm upgrade [--to X]` | Moves the pin up to the running version, or launches X to move it; `--to` a version below 0.2.0 is refused |

**A clone on the pre-package harness.** `pm init` refuses a clone that still holds the pre-package harness's pieces and names the fix: run `pm init` once with the retired Python release that moves them (`uvx --from "git+https://github.com/Yeeef/pm@pm-v<that release>" pm init`, which needs uv), then move the pin to a release from 0.2.0 on and run `pm init`.

**A release is a tag.** Releasing pm is `git tag pm-v<X> <any commit on main> && git push origin pm-v<X>`: no bump commit, no release PR and no rule on how a PR is merged. No version is written in Go source or in any file a release edits: the release workflow takes `<X>` from the tag name and stamps it with `-ldflags -X …/buildinfo.Version=<X>`; an untagged build reports `dev`, which no repo pins. Moving a repo's pin is a separate, ordinary PR after the release exists (`pm upgrade --to X`, merged any way); its hooks pass because the release's assets already exist.

**Release builds.** `.github/workflows/pm-release.yml` runs on a `pm-v*` tag push, on native runners, because the build needs cgo (`gozstd`).

| Target | Runner | Build |
|---|---|---|
| darwin/arm64 | `macos-14` | `CGO_ENABLED=1 go build -trimpath -tags gms_pure_go -ldflags "-s -w -X …/buildinfo.Version=<X>"` |
| linux/amd64 | `ubuntu-22.04` (glibc 2.35, the oldest it links against) | same |

- Assets: `pm-<X>-darwin-arm64.tar.gz`, `pm-<X>-linux-amd64.tar.gz`, `SHA256SUMS`, `install.sh`.
- Size: about 107 MB stripped on darwin/arm64, from the spike binary (the Dolt driver plus the spike's code); pm's own code adds little beside Dolt, but the release size is not measured yet.

**What `pm init` installs.**

| Scope | Piece |
|---|---|
| Machine | `pm` in the bin dir; the `pins/` cache; the service unit running `<abs path of the bin-dir pm> service run`; Codex `writable_roots` including `.pm/store/work` |
| Repo (tracked) | `.pm/config.toml`, `README.md`, `.gitignore`; pm's hook entries; `.gitignore` block; pm's git hooks in `.pm/hooks`, which `core.hooksPath` points at |
| Clone | `records` store; the work store at `<main>/.pm/store/work`, cloned from the remote, or created and pushed when the remote has none; the `records/` link |

`pm init` keeps its shape: a repo half on first install, and a clone and worktree half on every run, including `--session-start`.

### Migration from bd

`pm init --import-bd FILE` imports a `bd export` into a clone with no work store, as one write; the other clones then attach with `pm init`. The procedure and its checks are in [Work store](work-store.md), Migration from bd. The final bd export and `.beads/` stay untouched until the owner removes them.

### Tests

**The harness.** The pytest suite in `tests/` is pm's black-box harness: it drives the binary `make go-build` builds, with `fake_gh`, `fake_claude` and `fake_sched` (launchctl, systemctl, crontab) on `PATH`. `pyproject.toml` and `uv.lock` hold its environment; pm itself has no Python.

| Guarantee | Where |
|---|---|
| Each command against a temp repo: every refusal changes nothing, every happy path writes what it says | `test_pm.py`; work data read as work-store items, as `pm export` gives them (`repo.items()`) |
| Refusal texts word for word | asserted in `test_pm.py`; cited by `prime.md` |
| Hooks as the runtimes run them (JSON on stdin) | `test_hooks.py`; the noun list from `pm --help` |
| Owner-request judge | `test_owner_request_hook.py` (fake judge); the live eval, `make test-live`, on `owner_request_cases.json` |
| Service end to end | `test_service.py`; the in-process parts are Go tests in `internal/service` |
| Setup lifecycle | `test_init.py`, `test_lifecycle.py` |
| Launcher | `test_launch.py`, against a local HTTP server that serves release assets |
| Config check | `test_config.py` |
| Release build and `install.sh` | `test_release.py` |
| Pages | `tests/render-pages` prints every page as the service renders it |
| Light vs integration | the `integration` marker; `conftest.py` fails an unmarked test that starts the service, `init` or `sync` |

**Go tests** (`make test-go`: `go vet` and `go test ./...` with `-tags gms_pure_go`, the service and concurrency tests again under `-race`), for what is cheaper to check in process:

| Package | Tests |
|---|---|
| `work` | The merge rules table from the [Work store](work-store.md) page, one case per row; ids and minting; `ready`, `blocked` and cycle detection, from that page's definitions, not from the code |
| `site` | Golden pages (Site) |
| `install` | Golden pieces: the managed pieces on a table of inputs, against `testdata/pieces.json` |
| `records` | Section and block parsing; the scanning functions and YAML 1.1 typing against `testdata/expected.json`, edited by hand |
| `hooks` | Each chunk under `CAP` in runes; the chunks add up to the rules; the noun list matches the cobra tree |

A golden file is rewritten only by `go test -tags gms_pure_go ./internal/<pkg> -run Golden -update` after an intended change; the PR shows its diff for review.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| Each command opens the embedded store itself, under a gate | See "Each command opens the store" below the table |
| Byte-identical pages: string building with an `esc()` port of Python `html.escape`, no `html/template` | Byte identity binds the Go renderer to markdown-it-py's incidental output (entity forms, whitespace) and needs a custom escaper; normalised comparison checks the same structure and text and keeps contextual escaping |
| `gitlab.com/golang-commonmark/markdown`, a Go port of markdown-it with the same token stream | Last release years ago; targets an older CommonMark |
| stdlib `flag` with a hand-written tree | `flag` stops at the first positional, and pm puts positionals before flags; no `--x=v`/`-x` split, no nested help. pm would need its own parser about the size of pflag |
| `spf13/pflag` with a hand-written tree | Still needs a hand-written tree, help, required and exclusive flags, which cobra has |
| `dlclark/regexp2` for the 9 lookaround and backreference regexes | A second regex engine with backtracking for 9 sites; scanning code is short and testable against Python's results |
| A pure-Go build (`CGO_ENABLED=0`) | Dolt links `gozstd`, a cgo binding; the spike's build needs cgo |
| Go takes over command groups early; Python dispatches to it | During the port, Go would have to drive `bd` (a throwaway adapter) or Python would have to read Dolt, which it cannot embed. Two stores or a throwaway adapter for weeks, and the groups share state (`show`, needs and the site read everything) |
| Go first takes only commands without work data (`prime`, `hook stop`) | Saves nothing: these are the cheapest parts, and the dispatch layer costs more than it earns |
| `pm uninstall` with the service down opens the store itself for its unsynced-work check | Breaks the one access path, which `access_test.go` holds; a `pm service run` of its own for the check costs a second or two |
| `pm uninstall` with the service down starts the installed service through the supervisor for the check | Needs an installed unit (a removed one has none), enables a stopped one, and binds the site port, which another clone may hold by then |
| `pm uninstall` with the service down skips the check | Deletes items the remote lacks without a word: the check exists to refuse that |
| A marker file of pm's (`.pm/run/stopped`) records a stop | A second record beside the supervisor's enabled state, which decides what runs at login; the two drift when the owner enables the unit by hand |


### Each command opens the store

The alternative to the service-held store: each pm command opens the embedded store itself and closes it at exit, and the service opens it per poll. pm ran this way until the owner chose the service-held store.

| | Store opened per command | Store held by the service (chosen) |
|---|---|---|
| Full load in a fresh process | 64 ms on 466 items in the spike; 35.8 ms on 590 items, 68.3 ms with process start | 5.6 ms on 590 items, 36.4 ms with process start |
| One write with its commit | 70 ms | 17 ms |
| 8 processes x 20 writes | 7.7–10.9 s in the spike, 6.9 s on 590 items; p90 wait about 0.5 s on the exclusive lock | 2.11–2.17 s on 590 items; a write waits 88–90 ms on average for the write lock, readers never |
| Readers | wait for any open store, writers included | never wait |
| Slow work inside a command | the command must close the store first, or every other command waits | no rule needed |
| Writes at once | serialised by the gate, which readers wait on too | one at a time under the write lock, in order; reads never wait: 160 writes in 2.11–2.17 s, each waiting 88–90 ms on average |
| Site and sync | the service opens the store per poll and competes for the gate | the service holds the store; the site rereads when `HEAD` moves |
| Ad hoc SQL | through pm only | any SQL client on the socket |
| A command when the service is down | works | fails hard |
| Pinned versions | each command runs its own pin | a command must match the service's version |
| Tests | a temporary store per test | a running server per test |

**How it worked.** Opening the store starts the engine on the store's directory and takes its exclusive lock, readers included (open about 21 ms, close about 14 ms, against about 3 ms for the queries). Concurrent pm processes queued on a gate, an exclusive `flock` on `.pm/run/work.lock` taken before every open, with a timeout and a log of every wait; under 8 contending writers the max wait was 524 ms. A command closed the store before slow work that was not the store's (`gh`, the owner-request judge, a records push), and the store's own Dolt push (about 600 ms) was the one long hold.

**Why not.** It needs no running service and met the 300 ms load goal at 64 ms, but every reader waits for any writer, every command pays the open and close, the slow-work rule is easy to break, and the site's polling, several agent sessions and their hooks all queue on one lock. The service-held store removes the lock from every command at the cost of a running service and a version match between command and service.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

| Source | What we took |
|---|---|
| bd 1.3.1 | Embedded Dolt through `dolthub/driver`; cobra; the build flags (`CGO_ENABLED=1`, `-tags gms_pure_go`); Not taken: one engine open per transaction, about 410 ms of its 567 ms list, and one process per command behind a gate lock |
| markdown-it-py and mdit-py-plugins | The rendering that goldmark reproduces: tables with style alignment, h2–h3 anchors and their slug, containers, the mermaid fence |
| goldmark | The CommonMark 0.31.2 parser and its extension points for the custom parts |

## Open questions

> What is still unresolved?

| # | Question | Options | Default |
|---|---|---|---|
| 1 | cgo release builds | a. native runners per target; b. `zig cc` cross-builds from one runner (linux/amd64 built at 132 MB, not run) | a |
| 2 | Binary size per pin (107 MB measured for the spike, darwin/arm64) | a. accept; b. keep the N most recent pins and let `pm doctor` list the others; c. `upx` | a, with `-s -w`; b once a machine holds more than 3 pins |
| 3 | Release base URL for the Go launcher's tests | a. `net/http` with a documented `PM_RELEASE_URL` setting (mirrors, tests); b. `curl` on `PATH`, faked in tests; c. `gh release download` | a |
| 4 | Language of the black-box suite | a. keep pytest; b. port it to Go `testscript` | a; revisit if Python in a Go project costs CI time or confuses contributors |
| 5 | `--help` layout | a. a cobra template that mimics argparse; b. cobra's default layout, with only the text held | b. Agents read the text, and tests assert the text, not the layout |
| 6 | Codex sandbox | A command in a Codex session must connect to the service's socket `.pm/run/work.sock`; whether the sandbox allows a connect to a Unix socket inside `writable_roots` is not recorded | `pm init` keeps `.pm/store/work` and `.pm/run` in `writable_roots`; the implementation checks the connect in a Codex session |
| 7 | Go version | go.mod `go 1.26` (bd builds with go1.26.7; this Mac has go1.27.1) | 1.26, raised when Dolt requires it |
| 8 | Dolt APIs the host reaches past the driver: `embedded.LoadMultiEnvFromDir`, `engine.NewSqlEngine`, `DoltDatabaseProvider.Register`, `sqlserver.SetRunningServer` | a. use them, pinned with Dolt in `go.mod`; b. an HTTP API on a second socket for the operations | a; a Dolt bump runs the host's tests first |
| 9 | A clone whose main checkout path makes the socket path 104 bytes or more | a. fail hard, naming the limit; b. a socket under a short per-user directory | a; no clone near the limit is recorded |
| 10 | A worktree that pins another version (a pin-moving PR) cannot touch work items | a. refuse, as designed; b. allow versions with the same schema version | a, until it costs a sprint measurable time |
