---
type: sprint
title: CI fails on Go code that gofmt would change
bead: pm-d2k5.17
---

## Goal

> What should be true when this sprint ends, and why now?

No unformatted Go reaches main: CI fails when `gofmt -l` lists any file. From pm feedback 2026-10-10 (pm-quality sprint 16 found `internal/cli/cli.go` on main not gofmt-clean, and no CI step checks formatting).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A `make go-fmt` target that fails and names each file `gofmt -l` lists, run by `make test-go` and by pm-go.yml's build-vet-test job.
- Format the files main has today, in the same PR.

**Out:**
- Other linters.

## Done when

> What evidence will show the goal is met?

- `make go-fmt` fails on a branch with one unformatted file (shown in the PR) and passes on the PR's head; the PR's CI passes and `make merge-ready` says ready.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- On main at 6752e93, gofmt -l over the module's 122 Go files lists one:
  internal/cli/cli.go (a trailing comment gofmt aligns with the line above).
  gofmt -l given a directory walks everything below it, so the check lists
  files through go list: on a main checkout, gofmt -l . would also walk other
  sessions' worktrees under .claude/worktrees/, which git does not ignore.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: CI fails on Go code that gofmt would change: `make go-fmt` runs in `make test-go` and in pm-go.yml's build-vet-test job on both targets, and main's one unformatted file is formatted (PR #38).

Merged as eedbafd (PR #38).

- `make go-fmt`: `gofmt -l` on the files `go list` gives for the module's packages (every build tag's and the tests'), naming each file it lists and failing; a file that does not parse fails it with gofmt's error.
- `internal/cli/cli.go` gofmt'd; the developer guide's `make test-go` row names `go-fmt`.
- Labelled `no-changelog`: users would not notice it.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| `make go-fmt` fails on a branch with one unformatted file (shown in the PR) | met | Locally, with `internal/config/zz_unformatted.go` added unformatted and cli.go's fix reverted: it prints both files and `make go-fmt: gofmt would change the files above`, rc=2 (output in PR #38's body). On origin/main's tree before the cli.go commit it lists `internal/cli/cli.go`, rc=2. |
| passes on the PR's head | met | `make go-fmt` prints nothing, rc=0; its file list covers 122 files, all 122 tracked `.go` files. |
| the PR's CI passes | met | PR #38 head 262cf39: all 9 checks pass; build-vet-test (linux-amd64, darwin-arm64) ran `make go-build go-fmt go-vet go-test-but-work`. |
| `make merge-ready` says ready | met | `make merge-ready PR=38`: "PR #38 is ready: its head 262cf39 contains origin/main 6752e93, and every check on it passed". Merge sha: PR #38. |
