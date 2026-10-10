---
type: sprint
title: "Go port: Go skeleton with every command and its help (P2)"
bead: yeeef-agents-9va.93
---

## Goal

> What should be true when this sprint ends, and why now?

A Go pm builds on both targets with the full command tree and the shared types, so the work store, records-and-site and service sprints can start in parallel against one fixed `work.Item` type and store interface. It is on the critical path of the port.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm/go.mod`, `cmd/pm`, the cobra tree with all 40 commands and their help texts.
- `config`, `buildinfo`, `prime` and its chunks, `hook stop`.
- The `work.Item` type and the store interface, with no Dolt behind them.
- CI building Go pm with cgo on both targets (macOS and Linux), and the parity job with a list of expected failures that only shrinks.

**Out:** command bodies beyond `prime` and `hook stop`; the Dolt store; records, site, service, launcher and release.

## Done when

> What evidence will show the goal is met?

- `pm prime` in every mode is byte-identical between Python and Go.
- Every `--help` text is identical after whitespace normalisation.
- CI builds Go pm on both targets; the binary size is recorded as a finding.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Architecture, CLI framework, Distribution, Tests

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-08}
The parity job in pm-go.yml runs the Go-specific parity tests (prime, hook stop, config and argument errors, every --help); running the shared suite with PM_IMPL=go against a shrinking list of expected failures moves to sprint 83 (P1), which adds the PM_IMPL switch.
The shared suite cannot run on Go before P1 lands PM_IMPL, and P1 is changing the same harness in parallel; a list wired here would have no consumer.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Go pm binary size, stripped (-s -w), cgo, -tags gms_pure_go, no Dolt linked
  yet: darwin/arm64 3,418,626 bytes and linux/amd64 3,547,401 bytes in CI (go
  1.26 from go.mod, PR #80 run); 3,302,194 bytes on darwin/arm64 with go
  1.27.1 locally. Dolt adds about 100 MB once P3 links it (spike: 107 MB).

- go:embed reaches only files at or below the embedding package, so
  internal/hooks cannot embed src/pm/prime.md. Solved without a copy: a
  package at the module root (pm/assets.go, package pm) embeds src/pm/prime.md
  and src/pm/style.css, where Python pm ships and reads them; the move to
  internal/hooks and internal/site the pm-go page names can wait for the
  cut-over, when Python pm goes.

- --help parity: 56 of 56 parsers (root, 15 nouns, 40 commands) identical
  after whitespace normalisation, and byte-identical at COLUMNS=10000, because
  internal/cli/help.go lays help out as argparse does. The reference is Python
  3.13's argparse: 3.12 prints '-n LINES, --lines LINES' where 3.13 prints
  '-n, --lines LINES', so pm-go.yml pins UV_PYTHON=3.13 (the pm-tests jobs run
  the runner's 3.12).

- pm prime parity: byte-identical (stdout, stderr, exit code) for --rules 1
  and 2 and --subagent in 7 bd cases, each with and without --hook-json, and
  for --state and plain prime with --hook-json where init, where and show fail
  at the config check; 101 parity tests pass, 1 skipped, on darwin/arm64 and
  linux/amd64. Not comparable yet: --state when init, where and show succeed
  (Go has them in P5 and P9), and a TimeoutExpired line, which names the
  program run (Python's interpreter, Go's binary).

- cobra's pflag cannot parse a two-value option (pm decision need --option
  LABEL TEXT, --cost, --default) and does not accept argparse's abbreviations
  (--reas for --reason). Leaves therefore parse their own arguments with pflag
  after cobra routes (DisableFlagParsing), a pre-pass joining each pair; 11
  argument errors match argparse's text. Abbreviations stay unsupported: a P5
  parity test that uses one fails.

- After the review fixes (commit 7877edb) the CI binaries are darwin/arm64
  3,451,954 bytes and linux/amd64 3,576,073 bytes, stripped, cgo; parity 114
  passed, 1 skipped on both. The review found a config-check crash on a dotted
  key (version.a = 1) and argparse readings Go missed: option prefixes
  (--hook), ints as int() reads them (01, ' +2'), values starting with '-'
  that are negative numbers or hold a space; each now has a parity case. Still
  different: TOML syntax-error text (tomllib's wording vs BurntSushi's), and
  BurntSushi accepts some TOML 1.1 that tomllib refuses.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm builds with cgo on darwin/arm64 and linux/amd64 with all 40 commands and their help, `pm prime` and `pm hook stop` match Python pm, and `work.Item` with the store interface is fixed for P3, P4 and P8.

Merged as c03d436 (PR #80).

- `pm/go.mod`, `cmd/pm`, `internal/{buildinfo,config,cli,hooks,proc,pyjson,work}`; `pm/assets.go` embeds `prime.md` and `style.css` from `src/pm`, one copy.
- The 38 commands without a body fail hard, naming Python pm as the one that runs them until the cut-over.
- `make test-go` and `.github/workflows/pm-go.yml`: build as released, `go vet`, `go test`, and `pm/tests/test_go_parity.py` against Python pm.
- Moved on: the shared suite on Go with its shrinking expected-failures list, now in sprint 85 (sprint decisions).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| `pm prime` in every mode byte-identical | met, within the scope below | `test_go_parity.py`: `--rules 1`, `--rules 2`, `--subagent` (9 bd cases), each with and without `--hook-json`; `--state` and plain prime with `--hook-json` where init, where and show fail at the config check. `--state` with init, where and show succeeding waits for those commands (P5, P9) |
| Every `--help` identical after whitespace normalisation | met | `test_help`: 56 of 56 parsers (40 commands, 15 nouns, the root); byte-identical too at `COLUMNS=10000` |
| CI builds Go pm on both targets; size recorded | met | PR #80, `pm go` workflow: macos-14 and ubuntu-22.04, `CGO_ENABLED=1`, 114 parity tests passed, 1 skipped, on each; 3,451,954 and 3,576,073 bytes (Findings) |
| The sprint's PR is on main | met | [#80](https://github.com/Yeeef/yeeef-agents/pull/80) merged as `c03d436` |
