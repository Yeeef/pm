---
type: sprint
title: A PR runs only the Go tests its change can affect
bead: pm-d2k5.32
---

## Goal

> What should be true when this sprint ends, and why now?

A PR runs only the Go test packages that its changed files can affect, and the full suite still runs on every push to main. Most per-PR Go cost is internal/work's tests, and today a change to site, records or install code runs them anyway. Selection must be exact (the import graph plus each package's non-Go inputs), never a guess, and must fail toward running more. Starts after sprint 31 merges, since both edit pm-go.yml and the Makefile.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A selector built on `go list -deps -test`: the reverse import graph from the changed `.go` files to the test packages that import them, test-only imports included.
- Non-Go inputs mapped to their readers: testdata, embedded files (`prime.md`, `style.css`, `prompts/`, `CHANGELOG.md`, `changelog.d/`), `go.mod`/`go.sum`, and helpers such as `tests/fake_sched.py`. Reuse and extend sprint 27's `go_changes.py`.
- A changed file the selector cannot place runs everything.
- The pm go and race jobs on both OSes run the selection.
- Sprint 31's macOS OS-dependent subset composes with it: the selection intersected with that subset.
- The full suite on every push to main stays.

**Out:**
- The Python harness: it drives the built binary, so any Go change runs it whole.
- `go test`'s result cache: it is not safe, because tests read git and other external state.

## Done when

> What evidence will show the goal is met?

- On 3 real PRs that touch only non-store packages, internal/work's tests do not run, and the jobs' compute and wall time are reported against the same jobs on main.
- A PR that changes internal/work runs the full set.
- A test case shows that a testdata or embedded-file change selects the package that reads it.
- A test case shows that an unplaceable file runs everything.
- `merge_ready.py`'s per-PR workflow rule still holds.
- One main-push full run is green.

## Design pages

> Where is the detail?

- [pm-go](../design/pm-go.md): the access-path test moved to the module root package (covers `access_test.go`).

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Selector design, from evidence in the code: go list -deps -test gives each
  test binary's linked module packages (test-only imports included) in 0.3 s
  locally and 2-5 s as a CI step. Three packages' tests read Go source beyond
  their build and run on any .go change under what they read (SOURCE_READERS,
  checked against their test files' go/ast, go/parser, go/build, go/doc,
  go/types, exec go and ".go" marks): the root package (size_test.go,
  assets_test.go), internal/item (go list -deps of the item readers) and
  internal/cli (its own and subpackages' func docs). internal/work's
  access-path test parsed every module file, so it would have selected
  internal/work for any Go change; it moved to the root package. Non-Go
  inputs: only the root package embeds (prime.md, style.css, prompts,
  CHANGELOG.md, changelog.d), propagated like a .go change to the 7 test
  binaries linking it; testdata maps to its package; paths named from a
  package (../../tests/fake_sched.py from internal/service) to that package.
  internal/work's test binary links buildinfo, config, item, launch, proc and
  worktest besides itself, so a change to site, records, store, design, hooks,
  install, service, cli, ask, session or sync never runs it.

- Done-when samples (pm go jobs from the jobs API; compute = the 5 jobs'
  runner seconds; suite = the make step; wall = pm go's first job created to
  last done). Main's latest full run (8ad2720, push): 380 s compute, suites
  macOS 59 + 62 = 121 s and Linux 26 + 30 + 24 = 80 s, wall 139 s (5b48d14:
  438 s, 136 s, 92 s, 150 s; #85, a full PR run without cache saves: 374 s,
  134 s, 80 s, 138 s). #86 (internal/records, 9 of 17 packages selected): 210
  s compute (-45%), macOS suites 35 + 2 s, Linux 27 + 0 + 0 s, wall 80 s. #87
  (internal/hooks, 6 of 17): 218 s (-43%), 44 + 2 s, 24 + 0 + 0 s, wall 103 s.
  #88 (internal/site tests, 3 of 17): 190 s (-50%), 22 + 2 s, 16 + 0 + 0 s,
  wall 74 s. In all three the work job and both race jobs logged internal/work
  is not selected and no internal/work test ran on either OS. #85 changed
  internal/work and the CI files: 17 of 17 selected, the full set ran. Main's
  push runs at fedc348, 5b48d14, d318ee3 and 8ad2720 ran the full suite and
  passed.

- What is left per PR: a job with nothing selected still costs 18-24 s on
  Linux and 41-47 s on macOS (checkout, Go, cache restore and the select step,
  which needs the module cache for go list -deps), 110-130 s of the 190-218 s.
  A selection job ahead of the test jobs could skip them whole but would add
  its own setup to every PR's wall. Adding a Go test edits go-os-tests.txt
  (and an internal/work test, work-test-seconds.txt), which the selector
  cannot place, so such a PR runs every package; mapping each row to its
  package would place them. Review's weak points, none failing today:
  SOURCE_MARK misses a test reading source through filepath.Glob("*.go") (a
  glob mark flagged internal/records' covers patterns, so it was not added),
  and a NO_GO file read through a computed path selects nothing, as
  go_changes.py's skip already allowed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a PR runs only the Go test packages its changed files can affect, and on three non-store PRs internal/work's tests did not run and pm go's compute fell 43-50% against main's latest full run.

- `go_changes.py select` (PR #85): the import graph from `go list -deps -test`, embeds, testdata and paths a package's Go files name, the source-reading tests (SOURCE_READERS, checked), and every package for go.mod, go.sum or a file it cannot place.
- pm-go.yml's five jobs take the selection as `SELECTED`; macOS intersects it with the os tests; a target with nothing selected says so and passes; a push to main runs everything.
- The access-path test moved to the module root package.
- Three measured follow-ups clearing errcheck backlog: #86 (internal/records), #87 (internal/hooks), #88 (internal/site tests).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- 3 non-store PRs, internal/work not run, compute and wall reported: met. #86, #87, #88 selected 9, 6 and 3 of 17 packages; the work and race jobs logged "internal/work is not selected" on both OSes. pm go compute 210, 218 and 190 s against 380 s on main's latest full run (8ad2720); wall 80, 103 and 74 s against 139 s (Findings).
- A PR that changes internal/work runs the full set: met. #85 changed internal/work and the CI files: 17 of 17 selected, internal/work's 8 shards and the race tests ran. A change to internal/work alone selects internal/work whole and every test package linking it (cli, config, install, service), held by `test_go_changes_select_on_this_repo_follows_its_import_graph_and_its_readers`.
- A testdata or embedded-file change selects its reader: met. `test_go_changes_select_runs_the_test_packages_a_change_can_affect_and_everything_for_one_it_cannot_place` (testdata, test embed, embed, a named path) and the real-repo test (prime.md selects internal/hooks; internal/site/testdata selects internal/site).
- An unplaceable file runs everything: met. The same tests: Makefile, a workflow, an unembedded new file, a .go file in no package, go.mod and go.sum select every package; end to end in `test_go_changes_select_writes_a_pull_requests_packages_and_every_package_on_a_push`.
- merge_ready.py's per-PR workflow rule holds: met. Every pm go job reported and passed on #86-#88 with nothing to test; `release/merge_ready.py` accepted each and its exact command merged it.
- One main-push full run is green: met. pm go at fedc348 (run 38563081331), 5b48d14, d318ee3 and 8ad2720 ran every test and passed.
