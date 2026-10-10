---
type: design
title: "Work store: pm's own replacement for Beads"
project: pm-harness
---

## Problem

> What are we solving, and why now?

The owner decided that pm replaces Beads (`bd`) with a store of its own. pm borrows bd's ideas and rewrites them in its favour, so pm is one clean, self-contained product ([pm-harness](../projects/pm-harness.md) decisions). [pm's boundary with Beads](work-layer-bd-boundary.md) lists what pm uses of bd and the 10 places where bd constrains it. This page designs the store that takes bd's place.

bd is also slow on pm's hot path. 20 pm commands (`pm show`, `pm task add`, `pm task claim`, `pm finding add`, …) load every item, which takes `bd list --all --json` (1.16–2.09 s over 5 runs) plus `bd export` for comment bodies (1.69–2.28 s over 3 runs). Each such command spends 3–4 s in bd before its own work (measured at load average 70 on 8 cores).

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- Loading every item with its comments takes at most 300 ms in a fresh pm process on this Mac, against 3–4 s through bd today.
- Every bd use on the boundary page has a pm equivalent, and every constraint there is removed.
- One writer, pm, with typed fields; a session holds work, not a person.
- Sync across clones and machines through the repo's git remote, with conflicts that fail hard, never silently.
- Every existing id keeps its text, so records links stay valid.

**Non-goals**

- bd features pm does not use: formulas, molecules, defer, supersede, lint, priority.
- Dolt history and audit events from Beads, which `bd export` does not carry.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

Backends measured on this Mac (8-core M1) on the 466 items of a `bd export`; Dolt 2.4.2. The first three columns ran at load average 33–191, the embedded Go column at 20–50, so p90 is noisy. Details in [sprint 77](../sprints/pm-harness-77.md).

::: result {title="Work-store backends, measured"}
| Metric | Files on a git branch | Dolt sql-server | Dolt CLI per command | Dolt embedded in Go |
|---|---|---|---|---|
| Load all items, fresh process, median | 173 ms | 179 ms | 292 ms | 64 ms (p90 68 ms; 68 ms after 3,000 more commits) |
| One write with its commit, median | 90 ms | 36 ms | 189 ms | 70 ms, fresh process |
| 8 writers x 20 writes, without a lock | 19 of 160 commits (`index.lock`) | 160 of 160, 3.0 s | 41 of 160 (`database is read only`) | 160 of 160, 7.7 s, with the driver's open retry (p90 wait 468 ms) |
| 8 writers x 20 writes, with one `flock` | 160 of 160, 19.1 s | not needed | 160 of 160, 31.5 s | max wait 524 ms (a `flock` gate) |
| Offline edits: one item, different fields | clean with a JSON merge driver | clean | clean | clean (different cells of one row merge) |
| Offline edits: one item, different metadata keys | clean with a JSON merge driver | conflict (one JSON cell) | conflict | no JSON cell: each key is its own column |
| Same field, or the same new id, on two clones | conflict | conflict | conflict | conflict, in `dolt_conflicts_items` with base, ours and theirs |
| Push | 0.1–0.7 s | 2.2–3.1 s | 2.2–3.1 s | about 600 ms; a no-op pull 180 ms |
| Store size, about 380 commits, before and after gc | 15 MB, 3.6 MB (plus 1.8 MB checked out) | 14 MB, 3.0 MB | 14 MB, 3.0 MB | Dolt's storage format, not measured separately |
| Footprint | git only | 119 MB binary and a server process | 119 MB binary | the pm binary: 107 MB stripped (darwin/arm64), built with CGO and `-tags gms_pure_go`; linux/amd64 132 MB |
:::

Embedded in a Go process, Dolt loads every item in 64 ms, under a third of the 300 ms goal and faster than files (173 ms).

- **Why bd is slow.** `bd list --all --json` on a copy profiles at 567 ms: about 410 ms in 9 Dolt engine opens and closes (one per transaction), 117 ms cleaning a git remote cache on close, and about 105 ms more process start than a plain Go binary. The query itself takes about 36 ms. pm opens the engine once per process (open 21 ms, queries 2.7 ms, close 14 ms).
- **The engine lock.** An open embedded engine holds an exclusive lock on the store, readers included. A second process waits for it; the driver retries the open, so no write is lost. A process that holds the store open blocks every other process that opens it; so one process, the pm service, holds it and serves it to the others (Storage).
- **A server in front of the engine.** Served over a Unix socket by the process that holds it open, measured on a copy of this repo's store (590 items, an 8-core M1): a load of every item takes 5.6 ms median in a fresh process, connect and version handshake included, against 35.8 ms when the process opens the store itself; 8 processes × 20 writes, a new connection per write, finish 160 of 160 in 2.11–2.17 s (10 runs) under the store's write lock, against 6.9 s per command; a write waits 88–90 ms for the lock on average, at most 109 ms, about the length of the 7 writes queued before it. Optimistic writes, which retry on a conflict, starved a long transaction: a merge over every item lost 100 times in a row to one writer pausing 0–6 ms.
- **Storage.** Dolt stores compressed, content-addressed chunks and reclaims rewritten ones only on `dolt gc`. bd's own store here is 123 MB for 596 KB of exported data (2,041 commits), 47 MB after `dolt gc` on a copy.
- **Sync.** Dolt syncs through a git remote, under `refs/dolt/data` by default, once the remote has a branch. Different cells of one row merge automatically; a cell both sides changed lands in `dolt_conflicts_items`. A pull with conflicts commits only with `@@dolt_allow_commit_conflicts=1`.
- **Ids.** Two clones that add a child to the same parent offline mint the same `P.N` id; the merge surfaces it as a conflict.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

pm is a Go program ([pm in Go](pm-go.md)). The work store is a Dolt database that the pm service embeds through Dolt's Go driver (`dolthub/driver`), as bd embeds it, holds open for its whole life, and serves to pm commands over a Unix socket; no separate binary. [pm in Go](pm-go.md), "Store access", gives the server, the socket, the version handshake and the retries.

Terms used on this page, each defined once:

| Term | Meaning |
|---|---|
| Work store | pm's own store of work items, an embedded Dolt database; it replaces Beads (`bd`) |
| Item | One unit in the work store: a project, sprint, task or need. bd calls it an issue or a bead |
| Holder | The session that holds an item it works on. It replaces bd's `assignee` |
| Blocker | An item that must close before another item can start |
| Ready | A task that an agent can claim now (defined under "Ready and blocked") |
| Clone | One git clone of the repo; it has one work store, shared by its worktrees and sessions |
| pm service | The one process per clone that opens the work store; it serves it on the socket `<main checkout>/.pm/run/work.sock` |

Sample data on this page: a `bd export` of this repo with 466 items, 245 comments, 26 `blocks` links and 457 parent links (the 450 on the boundary page is an older count).

### Storage

| Part | Design |
|---|---|
| Location | one Dolt database per clone at `<main checkout>/.pm/store/work`, beside the `records` store; no link into worktrees; the pm service opens it by path |
| Schema | typed SQL tables, below; no JSON cell, since Dolt merges a cell whole and two clones that set different keys in one JSON cell conflict |
| Open | only the pm service opens the store: at its start, for its whole life. Every pm command that reads or writes items is a SQL client of the service's socket, with one connection per command; no command opens the store itself, and there is no fallback to a direct open. With the service down, such a command fails hard naming `pm service restart` |
| Write | each pm write is one SQL transaction: load and check, apply, check the result, write the changed rows, set the write stamp, `DOLT_COMMIT`, under the store's write lock. A command that changes several items lands whole or not at all. pm's writes run one at a time, in the order they asked for the lock, so they are serializable and none starves (Concurrent writers) |
| Read | one read-only transaction, so every query of a load sees one commit. The schema enforces types, NOT NULL, enums and foreign keys; pm checks its invariants in code (the type's fields present, closed ⇒ no holder, the tree rules) on every write and on load, and fails hard naming the item |
| Queries | SQL from Go; `pm export` prints JSONL for ad hoc `jq`; any MySQL client can read the store on the service's socket |
| History | Dolt's own, per row and cell: `dolt_history_items`, `dolt_diff`, `dolt_blame_items`, `dolt_log` |
| Remote | the repo's git remote, through Dolt's git remote support, under pm's own ref `refs/pm/work` (`DOLT_REMOTE add` and `DOLT_CLONE` take `--ref`; a push writes only that ref). pm turns off Dolt's `__dolt_remote_info__` branch |
| Sync | run by the pm service, every 600 s and when `pm sync` asks it: fetch; when behind, check the remote's head, then merge it in one write transaction with `@@dolt_allow_commit_conflicts=1`, resolve conflicts (Merge), check, `DOLT_COMMIT`; when ahead, `DOLT_PUSH`. A sync that fails rolls back and leaves the branch where it was; it never resets the branch, since other sessions' writes may have landed on it meanwhile. Push about 600 ms, no-op pull 180 ms |
| Merge | Dolt merges each cell three-way. After a pull, pm resolves each row of `dolt_conflicts_items` by the rules under Data model, applies close beats claim to the merged rows, and checks invariants, foreign keys (`dolt_constraint_violations`) and `blocked_by` cycles. A conflict no rule settles fails the sync hard and names the item and column |
| Cycle check | after every merge, pm recomputes `blocked_by` cycles (ancestors included, as `pm dep add` does); a cycle that two clones made between them fails the sync hard and names the items on it |
| Change mark | the store's `HEAD` commit hash (`HASHOF('main')`); the service rereads items only when it moves |
| Ahead/behind | `dolt_log` of the local branch against the fetched remote ref, both ways |
| Child id compare-and-swap | run by the pm service: the push of the new item from a scratch branch; a push rejected as non-fast-forward is the "remote moved" signal under Ids |
| Garbage | the pm service runs `CALL DOLT_GC()` on a schedule, online: Dolt's session-aware safepoints let open sessions go on |

**Schema**

| Table | Key | Columns |
|---|---|---|
| `items` | `id` | `type`, `parent`, `title`, `description`, `status`, `resolution`, `close_reason`, `number` (sprints only), `holder_session`, `holder_host`, `holder_claimed_at`, `started_at`, `created_at`, `updated_at`, `closed_at`, `closed_by`; need fields: `need_kind`, `raised_session`, `raised_inbox`, `raised_host`, `delivered`, `review_pr`, `review_focus`, `review_merged`, `review_merge_reported` |
| `review_targets` | `item_id`, `kind`, `target` | a review's sprints (`kind=sprint`) and design pages (`kind=design`) |
| `labels` | `item_id`, `label` | |
| `blocked_by` | `item_id`, `blocker_id` | |
| `comments` | `id` | `item_id`, `pos`, `kind`, `author`, `text`, `created_at`; `pos` orders an item's comments and is not unique, since two clones may each add a comment to one item |
| `schema_version` | one row | the version pm's migrations start from |
| `write_stamp` | one row | `txn`: a fresh UUID that every write transaction sets, so a write that raced one outside the write lock conflicts (Concurrent writers); it carries no data; a merge keeps this side's value, then sets a fresh one, as every write does |

A list is a table of rows, so Dolt's row merge unions two clones' additions per entry.

### Data model

An item has a fixed set of typed fields, stored as the columns and tables under Storage. There is no free `metadata` bag: each key pm uses becomes a column, validated on every write, and a new key is a schema change.

**Fields**

| Field | Type | Set by | Notes |
|---|---|---|---|
| `id` | string | minted at create, never changes | see Ids |
| `type` | `project` \| `sprint` \| `task` \| `need` | create | replaces bd `issue_type` plus the `human`/`action` labels |
| `title` | string | create, `pm task edit` | |
| `description` | Markdown | create, `pm task edit` | bd `notes` merge into it at migration |
| `status` | `open` \| `closed` | create, close | "in progress" is derived: an open item with a holder |
| `resolution` | `done` \| `answered` \| `no-decision` \| `dismissed`, or null while open | close | replaces matching on the close reason text `Responded` / `Dismissed` and the `no-decision` label. On a task, `dismissed` is a dropped task (`pm task close --dropped`: not done), shown as dropped; no separate resolution, so no clone's store check must change |
| `close_reason` | string | close | free text; a review keeps `merged as <sha>` |
| `number` | int, only on `type=sprint` | minted at create, and again by `pm sprint move` | the sprint's number in its record name (`pm-harness-77`); see Ids |
| `parent` | id or null | create, `pm task move`, `pm sprint move` | the id does not follow a move: 11 sample items have a parent that is not their id's prefix |
| `blocked_by` | list of ids | `pm dep add/rm` | the only dependency kind; bd's `parent-child` link becomes `parent` |
| `labels` | set of strings | create | free tags for what no field covers (`bug`, `[TEST]` marks); pm's logic reads no label |
| `holder` | `{session, host, claimed_at}` or null | `pm task claim`, `release`, close | replaces `assignee` and the `claimed_by`/`claimed_at` metadata |
| `started_at` | timestamp or null | first claim | day pages count "started" from it |
| `created_at`, `updated_at`, `closed_at` | timestamp | the store | UTC, `YYYY-MM-DDTHH:MM:SSZ` |
| `closed_by` | session id or null | close | who closed it; bd has only the git user |
| `comments` | list of comment | `pm comment add`, replies | returned with the item on every read |
| `need` | object, only on `type=need` | `pm decision need`, `pm action need` | below |

**Need fields** (`need` object):

| Field | Type | Replaces in bd |
|---|---|---|
| `kind` | `decision` \| `action` \| `review` | label `human`; label `action`; `metadata.review` present |
| `raised_by` | `{session, inbox, host}`, or null when no session raised it | `metadata.session`, `inbox`, `inbox_host`; 44 of 154 sample needs have no session |
| `delivered` | int: owner replies that reached `raised_by.session` | `metadata.picked_up` |
| `review` | `{pr, sprints, designs, focus, merged, merge_reported}`, only on `kind=review` | `metadata.review`, `external_ref`, `metadata.merged`, `metadata.merge_reported` |

**Comment**

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | bd's comment ids are UUIDs already; kept |
| `kind` | `reply` \| `note` | `reply`: the owner's answer, from the site or `pm reply add`; counted against `need.delivered`. `note`: anything else |
| `author` | string | `owner`, or the writing session id |
| `text`, `created_at` | string, timestamp | |

**Types and the tree**

| Type | Parent | Example id | bd today |
|---|---|---|---|
| `project` | none | `yeeef-agents-9va` | epic at depth 1 (5 in sample) |
| `sprint` | a project | `yeeef-agents-9va.77` | epic under an epic (82) |
| `task` | a sprint, a task (sub-task), or a project | `yeeef-agents-9va.77.2`, `…77.2.1`, `9va.26` | task (221), bug (5) |
| `need` | any item | `yeeef-agents-9va.41.2` | task labelled `human` (154; 79 also `action`; 59 with a review) |

- A task directly under a project is allowed: work that no sprint has taken up yet (4 open in the sample). It orders after every sprint's tasks in `pm task ready`. `pm task move` puts it into one of the project's open sprints, with the decision in that sprint.
- Only a project has no parent. A task or need with no parent fails a write, and fails the import unless it is closed (see Migration).

Statuses: `open` and `closed` only. bd's `in_progress`, `blocked`, `deferred` and `hooked` go: in progress is "open with a holder", blocked is computed, and pm defers nothing.

Holder: a session, never a person. `pm task claim` sets it in one write transaction, as a compare-and-set: it succeeds when `holder` is null, is this session, or names a session that is not live (live = its transcript changed within `LIVE_WINDOW`, 30 min, as in [Session claims](session-claims.md)). A holder on another host cannot be judged live locally; see Open questions. Close clears `holder` and sets `closed_by`; a closed item never has a holder.

No delete. Items close; a junk or test item closes with `resolution=dismissed`. With no delete, a merge of two store copies is a union of items, and nothing is lost to a deletion that an import cannot see (bd's `issues.jsonl` problem).

**Concurrent writers.** In one clone, many sessions write through the pm service at once, each in its own transaction; Dolt merges two transactions that changed different rows and refuses the second commit of two that changed one cell. Changing different rows can still break an invariant that spans rows (two `pm dep add` in opposite directions make a cycle; two `pm task move` can make a parent cycle). So every write transaction holds the store's write lock, from before its first read to after its commit or rollback: ordinary writes, a pull's merge and the compare-and-swap's merge. The pm service serves it (`CALL pm_lock(seconds)`, `CALL pm_unlock()`): a FIFO lock, so writers get it in the order they asked and a long transaction, such as a merge over every item, never loses to a stream of short ones; a waiter takes it over from a holder whose connection is gone. Reads take no lock.

- A write waits at most `WriteLockWait` (60 s) for the lock, then fails hard, naming itself and the holder: "work store: <the write> waited 60s for the store's write lock, which a live pm process holds (connection <n>, pid <p>, <its write>, for <d>); nothing was written. If that process hangs (stopped, or in a debugger), stop it; then run the command again". The lock has no lease: a holder whose connection dropped is taken over within 50 ms, but a live one that stalls between taking the lock and its commit keeps it, and every write, the service's sync merge and `pm_create` included, fails at its bound until that process goes. A lease would let a stalled holder's commit land after the lock passed on, which the write stamp would then refuse, but a lease cannot tell a stalled holder from a slow merge; naming the holder is the cheap, safe part. Claim reads whether a holder is live (transcripts on disk) before it takes the lock. A write runs after the one before it, so it reads that write's result: a claim another session took first refuses as held, a close of an item closed first refuses as closed.
- Every write transaction also sets `write_stamp.txn` to a fresh UUID: the safety net. A write that raced one outside the lock (a SQL client past pm) gets Dolt's serialization failure at commit, or "dataset head is not ancestor of commit" when a fast-forward moved the branch under it; both land nothing, and pm fails the write hard rather than retry it: "work store: <the write> conflicted with a write that did not take the store's write lock (a SQL client past pm?); nothing was written: run the command again".
- A write refuses a working set that differs from `main`'s head at its start, under the lock: no pm write leaves one, and its `DOLT_COMMIT('-A')` would commit it as its own change, a silent revert if it were the second half of a fast-forward cut short. The refusal names the tables and how to drop the change (`CALL DOLT_RESET('--hard')` on the socket, which keeps every commit).
- A write that changed no row (a claim again by its holder within one second) sets no stamp and makes no commit.

Across clones, two copies can change the same item before they sync. The merge is three-way per field, against the base (the common ancestor). `write_stamp` conflicts on every merge and carries no data: the merge keeps this side's value, and the merge commit sets a fresh one, as every write does, so a write in flight conflicts with it.

- A field that only one side changed from the base takes that side's value: Dolt's cell merge.
- A field that both sides changed lands in `dolt_conflicts_items`; pm resolves it by the rule below. A rule that names no winner fails the merge hard and names the item and field.
- The same new id on both sides (add/add) is a primary-key conflict with no base row; it fails the merge hard.

| Field | Both sides changed it |
|---|---|
| `id`, `type` | never changed: unequal values fail the merge |
| `number` | changed only by `pm sprint move`, which runs through the compare-and-swap, so by one side at most: that side's value; both sides changed it fails the merge. A sprint's title keeps its `Sprint <n>: ` prefix at the merged number, whichever side's title won |
| `comments` | union by comment id: rows, merged by Dolt |
| `labels`, `blocked_by` | per entry, three-way: an entry is kept unless a side removed it from the base and the other side left it as in the base; an add on either side is kept. Rows, merged by Dolt |
| `status` | `closed` wins |
| `closed_at`, `closed_by`, `close_reason` | from the earlier close when both closed; else from the side that closed |
| `resolution` | the side's value that is not null; when both set it, the later `updated_at` wins. So a resolution set after the close (`pm decision close` marks a closed need `no-decision`) is kept |
| `holder` (the three `holder_*` columns as one) | a closed result has holder null (close beats claim). Else: a claim beats a release (one side null); between two claims, the later `claimed_at` wins, and `pm show` warns that a claim was overridden |
| `started_at` | min (the earliest start) |
| `updated_at` | max |
| `need.delivered` | max |
| other scalars (`title`, `description`, `parent`, `need.review.*`) | the later `updated_at` wins |

Close beats claim also applies where Dolt merged cleanly: one side closed an item while the other claimed it from a null holder. pm clears the holder of every closed item in the merge result. Then the merged item must hold the invariants (closed ⇒ holder null; the type's fields present), and the merged store must have no `blocked_by` cycle (Storage, Cycle check); else the merge fails hard.

**bd fields dropped**

| bd field | Why |
|---|---|
| `priority` | pm never reads it; 447 of 466 sample items are priority 2 |
| `owner`, `created_by`, comment authors `yeeef`/`Yeeef` | the git user's identity, the same for every session; pm records sessions |
| `assignee` | replaced by `holder` |
| `lease_expires_at`, `heartbeat_at` | bd's lease feature (6 sample items); pm judges liveness from the transcript |
| `dependency_count`, `dependent_count`, `comment_count` | derived; computed on read |
| `metadata` (free bag) | each used key became a field; `probe` (1 test item) is dropped |
| `external_ref` | the same value as `need.review.pr` on all 59 sample items |
| dependency `created_by`, `metadata` | always the git user and `{}` in the sample |
| `design`, `acceptance_criteria` | unused in the sample; a design goes in a record |

### Ids

Format, unchanged so records links stay valid:

```
<prefix>-<root>(.<n>)*      prefix  = repo name, e.g. yeeef-agents
                            root    = [0-9a-z]{3,8}, e.g. 9va
                            n       = 1, 2, 3 … per parent, e.g. 9va.41.2
```

- Every existing id keeps its text. The store indexes by the id string and never re-derives it.
- A child's id is `<parent id>.<n>` at create time, with `n` one more than the highest `n` of any item whose id starts with `<parent id>.` and has one more segment, wherever it sits now (moved-away children count, so a number is never reused). The counter is not stored; it is derived from the ids on each mint. The id does not change on a move.
- A sprint's number, which names its record (`pm-harness-77`), is the field `number`, not the id's last segment (sprint 77 is `9va.86`). It is one more than the highest of the project's sprints' numbers and the numbers moved away from the project (Moving a sprint), minted in the same compare-and-swap as the id, so two clones cannot both open `pm-harness-N`, and a moved sprint's old name never names another sprint.
- A root id is 4 random base36 characters (1,679,616 values; bd used 3), checked against the store, and longer on a hit.

**Minting without collisions across clones.** A root id collides with negligible chance at 4 random characters. A child number does not: two clones that both see `9va.88` as the last sprint both mint `9va.89`, and both open the same sprint number. pm prevents it with a compare-and-swap on the remote. The pm service runs it, one at a time, when a command creates a child in a store with a remote; other sessions keep writing to `main` meanwhile, so the swap works on a scratch branch and never resets `main`:

1. The service pulls from the remote as Sync does, notes `main`'s `HEAD` commit, `C0`, and makes the branch `pm-cas` at `C0`.
2. On `pm-cas` it mints the id (and a sprint's `number`) from `C0`'s items, writes the item in one transaction, commits `C1`, and pushes `pm-cas` to the remote's `main`. Dolt rejects the push as non-fast-forward when the remote moved since the pull.
3. On that rejection it deletes `pm-cas` and goes back to step 1, up to 3 attempts in all; then it fails hard and writes nothing.
4. On a push timeout, or any push failure but that rejection, the outcome is unknown: the client dropped the push, but the server finishes the statement until Dolt kills its git, and git can update the ref after that, so the push may land later. The service fetches and looks for `C1` in the remote's history: found means the create landed (step 5). Anything else (`C1` not there yet, the fetch failing) deletes `pm-cas` and fails hard, never minting again, which could make the item twice: "work store: create <id>: <err>; <why>: the outcome is unknown, and the push may still land as <id>. Nothing was merged here: run pm sync, then check with pm show <id> before you create it again". If the push did land, the next sync brings the item.
5. Once the push landed, the service merges `pm-cas` into `main` in one write transaction (a fast-forward when no session wrote since `C0`; else a merge that adds one row nobody else can know of), deletes `pm-cas`, and returns the item. A merge that fails here says the item is on the remote, not to create it again, and that the next sync brings it.

Steps 1 and 2 need the remote. With the remote unreachable, a create refuses. A create costs a pull and a push, about 0.8 s from the sync timings; the whole create is not measured. A project's root id, and any id in a store with no remote, is minted by an ordinary write.

**Moving a sprint.** `pm sprint move <sprint> --to <project>` moves an open sprint, with its tasks, needs, frame, decisions, findings and report, to another open project.

| Part | Design |
|---|---|
| Id | stays. The id names the sprint in chat, records, review targets, blockers and its tasks' ids, so the old id is the new place: `pm show <id>` shows the sprint under its new project. Only `parent` changes, as for `pm task move`; the tasks, needs, holders, blockers and comments under it are not written |
| Number | the next number of the new project, minted as at create. The title's `Sprint <n>:` becomes `Sprint <m>:`, and the record moves from `sprints/<old project>-<n>.md` to `sprints/<new project>-<m>.md`, its text unchanged |
| Move note | the same write adds a comment to the sprint, kind `note`, author `pm`, its first line `pm sprint move: from <old project id> sprint <n> to <new project id> sprint <m>`, then the reason. It is the pointer from the old name: `pm show <id>` lists it; the site serves the old page path, and `pm show --record` and `pm record link` take the old record path, each as the sprint's record now; and the old project's next number counts `<n>`, so `<old project>-<n>` never names another sprint |
| Compare-and-swap | a move mints a number, so in a store with a remote the service runs it as it runs a create (Minting without collisions, steps 1 to 5, CALL `pm_move_sprint(?)`): a move into a project and a sprint opened in it, on two clones, mint one after the other. A move that finds the sprint in the target project already writes nothing and returns it, so a rerun after an unknown push outcome is safe |
| Order | the work store first, then the records: the store write is one transaction; the records write is one commit on the records branch, which renames the record and adds a `source=agent` decision to both projects' records, naming the sprint, both numbers and the reason |
| Interrupted | between the two writes the store holds the sprint in the new project with its new number, and the record keeps its old name. `pm show` and the site place a sprint by its `parent` and name it by its `number`, never by its record's file name, so both show it under the new project only. A rerun of the same command finds the sprint in the target project, its record not yet named for it, and writes the records step alone, from the move note. A records step that fails is not undone in the store, for the same reason |
| Another clone | a task another clone adds under the sprint before it syncs mints its id under the sprint's id, which did not change, so it lands in the moved sprint; claims, closes and edits change other rows, or cells the merge table settles (`parent` and `title` by the later `updated_at`). The move note merges as a comment. `number` changes only in the compare-and-swap, so two clones never both change it from one base |
| Refuses | a move while the sprint's last move has no records step yet (its record keeps the old name), naming the rerun that finishes it; a closed sprint; an unknown or closed target project; the project the sprint is in, once its record carries the new name; a reason under two lines |

### Ready and blocked

Definitions, over the items in the store copy:

```
ancestors(i)  = parent(i), parent(parent(i)), …                       nearest first
open_blockers(i) = { b ∈ blocked_by(j) : j ∈ {i} ∪ ancestors(i), status(b) = open }
blocked(i)    = status(i) = open  ∧  open_blockers(i) ≠ ∅
ready(i)      = type(i) = task  ∧  status(i) = open  ∧  ¬live(holder(i))
                ∧  ¬blocked(i)  ∧  every ancestor of i is open
live(h)       = h ≠ null  ∧  h.session's transcript changed within LIVE_WINDOW
```

- A blocker on a sprint or project blocks every task under it. One `pm dep add` between two sprints orders all their tasks.
- A parent never waits for its children, and a child is not blocked by its parent being open.
- A ready task whose holder is set but not live shows as "stale holder"; `pm task claim` may take it over.
- Needs are never ready: they are the owner's. They show in `pm show` under what waits on the owner.
- A task under a closed sprint is not ready; `pm check` reports it, since the sprint close gate should have refused.
- Order: by sprint (its `number`), then by id; tasks directly under a project come after every sprint's tasks. There is no priority.
- `pm dep add` refuses a cycle over `blocked_by` (ancestors included, so a task cannot block its own sprint) and a blocker that does not exist.
- `pm show <id>` gives the reason for a blocked item: each open blocker and, when inherited, the ancestor it came from.

The whole computation is in process over one read of the store (64 ms for 466 items); no index is kept.

### Commands

**For agents.** These replace each plain `bd` command that `pm prime` tells agents to run today. Text comes through `--text` or `--text-file` (`-` reads a heredoc), as for every pm body.

| bd today | pm | Behaviour |
|---|---|---|
| `bd ready --exclude-type=epic` | `pm task ready [--sprint ID] [--json]` | ready tasks, as defined above; no flag needed to drop projects and sprints |
| `bd show <id>` | `pm show <id> [--json]` | any item: fields, holder and liveness, blockers with reasons, children, needs, comments |
| `bd create --parent <task>` | `pm task add --parent <task> --title "…"` | sub-task; `--sprint ID` stays for a sprint's task. Description through `--text` or `--text-file` |
| `bd dep add <a> <b>` | `pm dep add <a> --on <b>` | `b` blocks `a`; refuses a cycle |
| `bd dep remove <a> <b>` | `pm dep rm <a> --on <b>` | |
| `bd update --title/--description` | `pm task edit <id> [--title "…"]` | description through `--text` or `--text-file`; a scope change is still `pm task move` or a sprint decision |
| `bd update --claim` | `pm task claim <id>` (exists) | the only way to set a holder |
| `bd unclaim` | `pm task release <id>` | clears this session's holder |
| `bd comments add`, `bd comment` | `pm comment add <id>` | text through `--text` or `--text-file`; a `note` |
| `bd close` | `pm task close` (exists) | |
| `bd human dismiss` | `pm need dismiss <id> --reason "…"` | `resolution=dismissed`; for `[TEST]` needs and replaced reviews |
| `bd human respond` (owner at a shell) | `pm reply add <id>` | owner's answer through `--text` or `--text-file`; a `reply` comment, as a reply on the site writes. The need stays open: the session that raised it reads the reply, then records it (`pm decision add --need`, `pm decision close`, `pm action done`), which closes it with its `resolution` |
| `bd list`, `bd search`, `bd count` | `pm show` filters; `pm export` | `pm export` prints the store as JSONL for ad hoc `jq` |
| `bd remember`, `bd memories`, `bd forget` | none | see Session context |
| `bd prime` | `pm prime` (exists) | see Session context |
| `bd dolt push`, `bd dolt pull` | `pm service` and `pm sync` | the service syncs every 600 s; `pm sync` asks it to sync now and prints what it did: pull, resolve conflicts, push (Storage, Sync) |
| `bd init`, `bd bootstrap` | `pm init` (exists) | see constraint 4 |

**Inside pm.** One row per row of "What pm uses of bd" on the [boundary page](work-layer-bd-boundary.md). `work` is the Go package that holds the store; every call is SQL on the pm service's socket, with no subprocess and no JSON parse.

| bd use | bd invocation | Replacement |
|---|---|---|
| Snapshot | `list --all --json` | `work.Items()`: every item, closed included, comments included, in one read |
| Comment bodies | `export`, cached by id and comment count | gone: `work.Items()` carries comments; the cache and its 0.7 s go |
| Comments (R) | `comments <id> --json` | `work.Get(id).Comments` |
| Comments (W) | `comments add <id> --file= --author=` | `work.Comment(id, kind, author, text)` |
| Lookup | `show <ids> --json` | `work.Get(ids...)`; a missing id fails hard |
| Needs by session | `list --label human --metadata-field session=X --limit 0 --json` | `work.Needs(session)`: a query on `raised_session` |
| Create | `create --type --parent --labels --title --description --metadata --external-ref --json` | `work.Create(type, parent, title, description, labels, need)`; mints the id as under Ids |
| Close | `close <id> --reason=` | `work.Close(id, reason, resolution)` |
| Holder | `update --claim --set-metadata claimed_by=… claimed_at=…` | `work.Claim(id, session)`: compare-and-set in one write transaction |
| Move | `update --parent=` | `work.Move(id, parent)`; the id stays |
| Labels | `update --add-label=no-decision` | `work.SetResolution(id, "no-decision")` |
| Metadata | `update --set-metadata` (`picked_up`, `merged`, `merge_reported`) | `work.UpdateNeed(id, …)`: `delivered`, `review_merged`, `review_merge_reported` |
| Owner answer | `human respond <id> --response=` | `work.Answer(id, text)`, called by `pm decision add --need` and `pm decision close`: a `reply` comment, then close with `resolution=answered`; a reply from the site only adds the comment |
| Location | `context --json` | `pm where` reads the store's own path and sync state |
| Config | `config get/set agent.profile` | gone: pm has no agent profile (constraint 6) |
| Setup | `bootstrap --dry-run --json`, `bootstrap --yes`, `hooks install --beads`, `init --non-interactive` | `pm init`: clone the remote's work store (`DOLT_CLONE`), or create one with the schema when the remote has none; install pm's own git hooks |
| Sync | `dolt push`, every 600 s | the pm service pulls, resolves conflicts by the rules under Data model, then pushes, every 600 s (Storage, Sync) |
| Outside pm | `bd prime --hook-json` in `.claude/settings.json`; 4 `bd codex-hook` entries in `.codex/hooks.json` | removed by `pm upgrade`; `pm prime` is the only context hook |

### Session context

`pm prime` stays the only session context. `bd prime` (6,923 B) and the Beads block in `CLAUDE.md` go.

| Part of `bd prime` today | After |
|---|---|
| Persistent memories (4) | dropped; each moves to its home (table below) |
| Session close protocol, profile policy | dropped; `prime.md` states pm's git rules once: push a sprint's branch and open its PR, commit records with `pm commit` |
| Core rules ("use beads for all tracking") | already in `prime.md` §4: every change has a task in a sprint |
| Essential commands, common workflows | `prime.md` "How" names the pm commands from the Commands table; `--help` holds the rest |
| bd features pm does not use (formulas, molecules, defer, supersede, lint, stale) | dropped |

`pm prime` output after the change:

| Hook | Prints | Change |
|---|---|---|
| `pm prime --rules N` (4 chunks) | `prime.md`, then the noun list | `bd` lines (`bd ready`, `bd show`, `bd dep add`, `bd create --parent`, `bd remember`, "beside `bd prime`") become pm commands; the Beads row of "The layers" names the work store |
| `pm prime --state` | `pm where`, `pm show` | adds one line: the count of ready tasks and of items another live session holds |
| `pm prime --subagent` | one line naming the Beads agent profile | prints nothing, or is removed: there is no profile |

**Memories (`bd remember`): drop.** The four memories in this repo all have a better home, and a memory loaded every session is context that no check keeps true.

| Memory | Home |
|---|---|
| `need-descriptions-render-as-markdown` | `pm decision need --help` and `pm action need --help`; better, a check that refuses a description without a `**Question**` line |
| `never-record-an-owner-decision-from-a-comment` | `prime.md` §4 Invariants, beside "Owner decisions are closed" |
| `owner-does-not-micro-manage-git-housekeeping-branch` | the owner's global `AGENTS.global.md`; it is not about pm |
| `when-linking-records-…` | already done by code: `pm record link` uses `site_url` from `.pm/config.toml` |

`prime.md`'s line "Keep a small operational fact: `bd remember`" goes. A fact about this repo goes in the repo's `AGENTS.md`; a fact about pm goes in pm's `--help` or code.

### Migration from bd

One command, once, in one clone: `pm init --import-bd <export.jsonl>`. It refuses a store that holds any item. The other clones attach with `pm init` after the import is pushed. Input is `bd export`; the import has no other source.

**Field mapping**

| bd export | Work store | Rule |
|---|---|---|
| `id` | `id` | verbatim |
| `issue_type=epic`, no parent | `type=project` | |
| `issue_type=epic`, parent an epic | `type=sprint` | |
| `issue_type=task`, label `human` | `type=need` | `need.kind`: `review` if `metadata.review`, else `action` if label `action`, else `decision` |
| `issue_type=task`, other | `type=task` | |
| `issue_type=bug` | `type=task`, label `bug` | 5 items |
| task or bug whose parent is a project | `type=task`, `parent` the project | allowed (Types and the tree); 4 open in the sample |
| task or bug with no parent | `type=task`, `parent` null | only when closed (3 in the sample); an open one fails the import, so the open parentless bug in the sample is moved under a sprint or project with `bd` before the export |
| sprint (epic under a project) | `number` | `N` from the sprint record whose title starts `Sprint N:` and whose name is `<project>-N`; a sprint with no record, or a name and title that disagree, fails the import |
| `title`, `description` | same | `notes`, when present, appended under a `Notes` heading (2 items) |
| `status` | `status` | `in_progress` → `open` with a holder |
| `close_reason` | `close_reason` | verbatim |
| label `no-decision`; else `close_reason` `Responded…` / `Dismissed…`; else | `resolution` | `no-decision` / `answered` / `dismissed` / `done`, first match wins (2 sample needs have both the label and `Responded`: `no-decision`); only on closed items |
| `dependencies[type=parent-child]` | `parent` | `depends_on_id` |
| `dependencies[type=blocks]` | `blocked_by` | `depends_on_id` |
| `labels` | `labels` | minus `human`, `action`, `no-decision` (now fields) |
| `metadata.claimed_by`, `claimed_at` | `holder` (open items) | `host` unknown: null; on closed items only `closed_by` keeps `claimed_by` |
| `started_at`, `created_at`, `updated_at`, `closed_at` | same | |
| `metadata.session`, `inbox`, `inbox_host` | `need.raised_by` | null when `metadata.session` is missing (44 needs) |
| `metadata.picked_up` | `need.delivered` | missing → 0 |
| `metadata.review`, `merged`, `merge_reported` | `need.review.*` | |
| `external_ref` | `need.review.pr` | must equal `metadata.review.pr`, else the import fails |
| `comments[]` | `comments[]` | `author="owner (site reply)"` → `kind=reply`, `author=owner`; others `kind=note`, author verbatim |
| `_type` (top-level record kind of the export line) | none | ignored |

No child counter is imported: the next child number comes from the ids (Ids). The import fails hard on an input it cannot map: an unknown type, status, top-level key or metadata key, a dangling `parent` or blocker, an open item outside the type tree, a `blocked_by` cycle, or a non-UTC timestamp. It maps nothing by guess.

**Lost**

| What | Why |
|---|---|
| Dolt history and audit events | `bd export` does not carry them |
| bd's kv store, memories included | `bd export` does not carry it; the 4 memories move by hand (Session context) |
| `priority`, `owner`, `created_by`, `assignee`, lease and heartbeat, dependency `created_by` | dropped fields (Data model) |
| Whether a `note` comment came from the owner at a shell | bd records the git user for both the owner and agents |
| Who held a closed item, when no `claimed_by` was set | bd's assignee is the git user |

**Check.** After the import, `pm export` and the bd export agree on: item count (466 in the sample), each id's type, status, parent, blockers, title and description, comment count per item (245 in all), and every need's `raised_by` and `delivered`. A test runs this round trip on the sample. The fidelity of `bd export` itself is untested (boundary page, Open questions).

**Cut-over.** Stop the pm service in every clone; `bd dolt push` once; `bd export`; import (one transaction and one Dolt commit) and push; `pm upgrade` (removes the bd hook entries, the Beads block in `CLAUDE.md` and `.beads/hooks` from `core.hooksPath`); start the service. `.beads/` stays on disk until the owner removes it (see Open questions).

### Constraints resolved

One row per row of "Where bd constrains pm".

| # | Constraint | How the work store removes it |
|---|---|---|
| 1 | `bd list` returns no comment bodies | `work.Items()` returns comments with each item in one read; the export and its cache go |
| 2 | No session or holder | `holder` is a field holding a session, set only by a compare-and-set claim; `assignee` goes. Cross-machine liveness stays open (Open questions) |
| 3 | No change notification | pm is the only writer, so each write is a Dolt commit; the service rereads only when the store's `HEAD` commit hash moves. No file fingerprint |
| 4 | `bd init`, or a bootstrap from an old JSONL, forks the issues | `pm init` clones the remote's work store and creates one only when the remote has none, as it does for the `records` branch; the only import is `--import-bd`, which refuses a non-empty store |
| 5 | No ahead/behind in embedded mode | pm owns sync, so `pm where` reports ahead/behind of the store against the remote, counted from `dolt_log` |
| 6 | bd's `conservative` profile tells agents never to push | no profile; `prime.md` states pm's git rules once |
| 7 | Context arrives three times | `pm prime` only; `bd prime` hook and the Beads block in `CLAUDE.md` removed |
| 8 | bd owns the git hook files | pm owns its hook directory: `pm init` points `core.hooksPath` at pm's hooks; no section after `END BEADS INTEGRATION`. The work store itself needs no git hook: it syncs through Dolt |
| 9 | CLI edge cases: `-` text as a flag, `list` hides closed, orphan dolt child processes | calls are SQL on the service's socket: no argv for text, `work.Items()` returns every item, and there is no child process to time out |
| 10 | Breaking changes upstream; an older binary refuses a newer schema | the store's `schema_version` table records the schema version; pm ships its SQL migrations and is pinned per repo in `.pm/config.toml`. An older pm refuses a newer schema with "run pm upgrade". The cadence is pm's own |

## Alternatives considered

> What else was considered and not adopted, and why not?

**What Dolt gives beyond files**, the question behind the backend decision:

| Need | Dolt, embedded | Files on a git branch |
|---|---|---|
| Schema | enforced by the database: types, NOT NULL, enums, foreign keys for `parent` and `blocked_by` | pm validates every write; a hand edit bypasses it until `pm check` |
| Queries | SQL, ad hoc too | code over every item in memory (173 ms for 466); `pm export` with `jq` for ad hoc questions |
| Transactions | multi-item, atomic, isolated | one git commit under a lock: atomic, readers not isolated mid-write |
| History | per row and cell: `dolt_history_items`, `dolt_diff`, `dolt_blame_items` | per item: `git log` and `git blame` on the item's file, one field per line |
| Merge | three-way per cell, built in; conflicts in `dolt_conflicts_items` | a merge driver in pm, registered in each clone's `.git/config`; git text-merges silently where it is missing |
| pm's merge rules (close beats claim, cycle check, add/add) | pm's code, resolving Dolt's conflict rows after a pull | pm's code, in the driver |
| Operation | in process in the pm binary (107 MB), held open by the pm service; commands reach it over a Unix socket | git only |

| Alternative | Why not |
|---|---|
| Files on a git branch: one JSON file per item on a `work` branch, a git commit per write under a `flock`, a merge driver for the rules under Data model | Measured well: 173 ms load, 90 ms write, 160 of 160 writes with a `flock` (19.1 s), push 0.1–0.7 s, git only. The owner chose Dolt for its enforced schema, SQL, transactions and row-level history, and embedded in Go it loads faster (64 ms) |
| Each pm command opens the embedded store itself, under a `flock` gate | Measured: 64 ms to load every item in a fresh process (about 35 ms of it open and close), 70 ms per write, 8 processes × 20 writes in 7.7–10.9 s with a p90 wait of about 0.5 s, since the engine's lock is exclusive for readers too. It needs no running service, but every reader waits for any open store, a command must close the store before slow work, and the site and sync compete for the gate. The owner chose the service-held store; the trade-offs are in [pm in Go](pm-go.md), Alternatives considered |
| Dolt CLI per pm command | Slowest on every measure (292 ms load, 189 ms write); concurrent commands fail without a lock (41 of 160) |
| SQLite, DuckDB or LMDB, embedded | One binary file that git cannot merge across clones; pm would need its own sync (export and replay) |
| A free `metadata` bag (a JSON cell) beside typed columns | Dolt merges a cell whole: two clones that set different keys in one JSON cell conflict (measured). A typed column merges per key and the schema validates it |
| A stored `in_progress` status | Two facts for one: in progress is an open item with a holder |
| Keep `bd remember` as `pm memory` | All 4 memories have a better home, and a memory loaded every session is context no check keeps true |
| Clone-tagged child ids (`9va.77.k3x`) to mint offline | Breaks the sequential child numbers; refusing a create while the remote is unreachable is simpler |
| Renumber a moved sprint with a pointer: a new sprint id under the new project, its tasks moved there, the old sprint closed naming the new id | Every reference to the old id (review targets, blockers, records, chat) must follow the pointer; a task another clone adds under the old id before it syncs lands under a closed sprint |
| Keep a moved sprint's number | A record name `<project>-<n>` is unique per project; the old number may be taken in the new project |
| A schema column or table for sprint moves | A schema version change: every clone must upgrade before it syncs again (as 0.3.0's did). The move note carries the same facts in a comment row, which merges by union |
| A closed placeholder sprint left in the old project to keep its number | An item that every list of sprints (the site, day pages, `pm show`) would have to skip |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- [Beads](https://github.com/gastownhall/beads): hash ids with dotted children, `ready` computed from blockers, a claim, session context from the tracker, embedded Dolt synced through a git remote. pm keeps the ids, `ready` and the embedded Dolt with its git remote; it replaces the assignee with a session, and holds the engine open in one service process where bd opens it once per transaction.
- pm's own `records` store: one store per clone under `<main checkout>/.pm/store`, synced by the pm service. The work store sits beside it.

## Open questions

> What is still unresolved?

- A holder on another machine cannot be judged live locally. For now pm stores the host, shows it, and takes over an idle claim as today; a synced heartbeat follows when two machines work at once.
- The linux/amd64 pm binary (132 MB, built with zig as the C compiler) has not yet run.
