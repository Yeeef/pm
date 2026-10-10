---
type: design
title: "Work layer: pm's boundary with Beads"
project: pm-harness
---

## Problem

> What are we solving, and why now?

Sub page of [Work layer: Beads](work-layer.md). pm is meant to become one product with its own context layering and CLI. Today it is a layer over Beads (`bd`) and works around bd in several places. Before pm grows further, the owner chooses whether pm stays layered on bd, forks bd, or replaces bd with its own store. This page maps what pm uses of bd, where bd constrains pm, and what each option costs.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- One list of every bd feature pm depends on, so the cost of any option can be checked against it.
- The constraints bd puts on pm, each with its evidence.
- The three options with their costs, including migration of the existing data and keeping up with bd upstream.

**Non-goals**

- Carrying out a fork or a replacement. That is later sprints' work, if the owner chooses it.
- Packaging pm, which [pm as a product](pm-product.md) covers.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

**pm's side** (measured 2026-10-08 at main `314d701`):

| Fact | Value | Source |
|---|---|---|
| pm source | 7,730 lines of Python; `cli.py` 3,693 | `wc -l pm/src/pm/*.py` |
| bd call sites | 38, of which 31 are in `cli.py`, all subprocess calls | grep of `bd(` in `pm/src/pm` |
| Distinct bd subcommands | 14 | table under Design |
| Output pm parses | JSON only (`--json`, or the JSONL of `bd export`); bd's stderr goes into pm's errors | code read |
| Issues in the database | 450: 411 closed, 33 open, 6 in progress; 363 tasks, 82 epics, 5 bugs; 151 labelled `human`, 79 `action` | `bd count`, `bd list --all --json` |
| `pm show` | 2 bd calls (`list --all --json`, `export`), 3.18 s wall; the export alone is 577 KB in 0.66–0.73 s warm | a logging shim on `PATH` |

**bd's side** (upstream at `gastownhall/beads`, read 2026-10-08):

| Fact | Value | Source |
|---|---|---|
| Language, license | Go, MIT; the module path is still `github.com/steveyegge/beads` | `go.mod`, `LICENSE` |
| Size | about 1.01 M lines of Go, about 405 k non-test; checkout 69 MB | `wc -l` on a shallow clone at `d0cc1b1` |
| Community | 27,720 stars, about 353 contributors; created 2025-10-12 | GitHub API |
| Releases | 9 final releases from 2026-04-07 to 2026-09-30 (1.0.1 to 1.3.1); 1.2.1 was pulled | `gh release list` |
| Breaking changes | in almost every minor release: 11 "breaking" entries from 1.0.0 to 1.3.0, for example the close policy enforced on `update --status`, `update --notes` refused, a Go SDK signature change | `CHANGELOG.md` |
| Storage | embedded Dolt (needs a CGO build); SQLite was removed in 0.57.0 and later returned; Postgres and MySQL adapters were added and rolled back; an older binary refuses a newer schema | `CHANGELOG.md`, `README.md` |
| Sync | `bd dolt push/pull` to `refs/dolt/data` on the git remote; `issues.jsonl` is an export, and importing it cannot detect deletions | `docs/core-concepts/sync-concepts.md` |
| Export | `bd export` keeps issues, labels, dependencies, comments and metadata. It drops Dolt history, audit events and the kv store | `bd export --help` |
| Binary | 144 MB arm64 binary inside an npm wrapper; no separate `dolt` | `ls -la`, `otool -L` |

**Extension points bd offers:**

| Mechanism | Use for pm |
|---|---|
| Free JSON `metadata` per issue, which upstream calls the preferred extension point | pm already keeps its own fields there |
| `--json` on every command, `bd schema` for the JSON Schema, and a versioned envelope (`BD_JSON_ENVELOPE=1`, the default in v2.0) | a stable read contract |
| `.beads/PRIME.md` overrides `bd prime`'s text; memories are still appended | pm can own the session context |
| Script hooks (`on_create`, `on_update`, `on_close`) and an events journal, also over HTTP through `bd serve` | change notification without fingerprinting Dolt files |
| Go library | its signatures change between releases |

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

pm replaces bd with its own work store (option C, the owner's decision in [pm-harness](../projects/pm-harness.md)). Until sprint 79 completes, pm is layered on bd and calls it only as a subprocess. The tables below are the boundary the replacement must cover; [sprint 77](../sprints/pm-harness-77.md) designs the store.

### What pm uses of bd

R reads, W writes. Line numbers are in `pm/src/pm/cli.py` unless a file is named.

| Purpose | Invocation | Used by | R/W |
|---|---|---|---|
| Snapshot | `list --all --json` | `beads.py:27`, behind every `pm show`, the service, `pm commit` | R |
| Comment bodies | `export` (all issues), cached by id and comment count | `beads.py:58` | R |
| Comments | `comments <id> --json` | decision close, reply delivery, new replies | R |
| Comments | `comments add <id> --file= --author=` | decision close, site reply | W |
| Lookup | `show <ids> --json` | `beads.py:68`, project and sprint open, reply | R |
| Needs by session | `list --label human --metadata-field session=X --limit 0 --json` | `owner_request.py:114` (Stop hook), reply read | R |
| Create | `create --type --parent --labels --title --description --metadata --external-ref --json` | needs, PR reviews, tasks, projects, sprints | W |
| Close | `close <id> --reason=` | action done, task, sprint, project close | W |
| Holder | `update --claim --set-metadata claimed_by=… claimed_at=…` | task claim | W |
| Move | `update --parent=` | task move | W |
| Labels | `update --add-label=no-decision` | decision close | W |
| Metadata | `update --set-metadata` (`picked_up`, `merged`, `merge_reported`) | reply, service merge tracking | W |
| Owner answer | `human respond <id> --response=` | decision add, decision close | W |
| Location | `context --json` | `beads.py:74`, `pm where` | R |
| Config | `config get/set agent.profile` | `hooks.py:163`, init, `pm where` | R/W |
| Setup | `bootstrap --dry-run --json`, `bootstrap --yes`, `hooks install --beads`, `init --non-interactive` | `pm init` | W |
| Sync | `dolt push`, every 600 s | `push.py:73` (the service) | W |

Outside pm, the repo's agent hooks call `bd prime --hook-json` (`.claude/settings.json`) and four `bd codex-hook` entries (`.codex/hooks.json`). pm never calls `bd dolt pull`.

**Data pm relies on:** the fields `id`, `status`, `issue_type`, `parent`, `dependencies` (type `blocks`), `labels`, `metadata`, `assignee`, `started_at`, `close_reason`, `comment_count`, `comments`, `external_ref`; the labels `human`, `action`, `no-decision`; the metadata keys `session`, `claimed_by`, `claimed_at`, `picked_up`, `merged`, `merge_reported`; dotted child ids, from which pm derives sprint numbers; and the close reason `merged as <sha>`.

### Where bd constrains pm

| # | Constraint | What pm does about it | Evidence |
|---|---|---|---|
| 1 | `bd list` returns no comment bodies | runs a full `bd export` and caches it | `beads.py:38-63`; 0.7 s per `pm show` |
| 2 | No session or holder: `--claim` sets only the assignee, the owner's identity | keeps `claimed_by`/`claimed_at` in metadata; judges "live" from the transcript's modification time (30 min window) | `cli.py:698-733`, `beads.py:189` |
| 3 | No change notification that pm uses | the service fingerprints the Dolt chunk store by file sizes, because a bd read touches mtimes | `beads.py:83-88` |
| 4 | `bd init`, or a bootstrap from an old JSONL, forks the project's issues | `pm init` refuses any bootstrap action other than `none` or `sync`; then sets `.beads` to 0700, `beads.role` and the hooks that bootstrap leaves out | `cli.py:2248`; [sprint 10](../sprints/pm-harness-10.md) |
| 5 | No Dolt ahead/behind in embedded mode | `pm where` prints "not reported by bd" | [sprint 10](../sprints/pm-harness-10.md) |
| 6 | bd's default `conservative` profile tells agents never to commit or push | `pm init` sets `team-maintainer`; the Beads block in `CLAUDE.md` still states the conservative rules | `cli.py:2263`; `CLAUDE.md` |
| 7 | Context arrives three times: `bd prime` (6,923 B), pm's prime, and the Beads block in `CLAUDE.md` | none yet | [sprint 14](../sprints/pm-harness-14.md) |
| 8 | bd owns the git hook files | pm appends its section after `END BEADS INTEGRATION` | `install.py:292` |
| 9 | CLI edge cases: a text starting with `-` reads as a flag; `list` hides closed issues without `--all`; dolt child processes outlive a timeout | `--file` for replies, `--all`, killing the process group | `cli.py:1391`, `owner_request.py:124`, `push.py:55` |
| 10 | Breaking changes in most minor releases; an older binary refuses a newer schema | none: the repo does not pin a bd version | `CHANGELOG.md` |

Constraints 1, 3 and 7 have a bd mechanism that pm does not use yet: the export cache could narrow to `comments`, the events journal or script hooks replace the fingerprint, and `.beads/PRIME.md` replaces bd's context. Constraints 2, 4, 5 and 10 are bd's model or its storage, and only a fork or a replacement removes them.

## Alternatives considered

> What else was considered and not adopted, and why not?

The owner chose C. The cost estimates below are estimates, not measurements.

| | A. Stay layered, take the context | B. Fork bd | C. Replace bd with pm's own store |
|---|---|---|---|
| What it is | Keep bd as a dependency. Own the session context with `.beads/PRIME.md`, drop the Beads block from `CLAUDE.md`, pin the bd version in `.pm/config.toml` and check it in `pm doctor`, route every bd call through `beads.py` | Maintain a fork of bd and change its model: sessions, context, setup | Keep work items in pm's own store. One option: one file per item on the `records` branch, synced by the git push the pm service already does |
| Build cost | small: one sprint | large: Go, CGO and Dolt in a 405 k-line codebase | medium: about 14 commands' worth of behaviour, `ready`/blocked computation, ids, concurrent writers; estimated at 1.5–3 k lines of Python |
| Upkeep | follow bd releases, with a pin to absorb breaking minors | merge upstream's churn: 9 releases and 11 breaking entries in 6 months, or diverge and lose upstream fixes | pm owns all of it; no outside churn |
| Migration | none | none, if the schema stays compatible | `bd export` of 450 issues keeps issues, labels, dependencies, comments and metadata; Dolt history and audit events are lost; one cut-over across clones |
| Removes constraints | 1, 3, 6, 7, 8 (with bd's journal and PRIME.md) | all, at the price of owning them | all; also drops the 144 MB binary and the Dolt sync |
| Gives up | constraints 2, 4, 5 and 10 stay | upstream's 350 contributors' work, unless merged by hand | bd's agent ecosystem (`bd ready`, Codex hooks, memories) and its maturity |

**The agent had recommended A now, with C kept open; the owner chose C** to keep pm clean and self-contained.

The agent's reasoning for A: pm uses a narrow, JSON-only part of bd (14 commands), so a later move to C stays cheap, provided all bd calls pass through `beads.py`. B costs the most for the least: pm uses 14 of the about 120 commands `bd --help` lists, and every bd minor release would need merging.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- [Beads](https://github.com/gastownhall/beads): its `docs/core-concepts/metadata.md` names `metadata` as the extension point, and `docs/reference/events-journal.md` describes the journal that option A would use.
- The prior art under [Work layer: Beads](work-layer.md) records why Beads was chosen.

## Open questions

> What is still unresolved?

- An export-then-import round trip of `bd export` has not been tested for full fidelity.
