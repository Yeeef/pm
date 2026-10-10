---
type: sprint
title: "Go port: the pm service (P8)"
bead: yeeef-agents-9va.99
---

## Goal

> What should be true when this sprint ends, and why now?

Go pm runs the clone's background service with the behaviour of Python's, on macOS and Linux, so install and release can wire it end to end. It needs no Dolt: it builds against the store interface from the skeleton.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `service run`: HTTP, snapshot, spool, merge poll, inbox push, the sync and gc loops, opening the store once per poll.
- `service install/status/restart/logs` on launchd and systemd.
- The gc loop carries sprint 81's Dolt garbage collection over to the Go service.

**Out:** wiring the service to the real Dolt store end to end (install sprint); the site renderer (records-and-site sprint).

## Done when

> What evidence will show the goal is met?

- Go unit tests in `internal/service`, with a fake site and store, pass.
- The launchd and systemd unit files Go writes equal Python's for the same inputs.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Architecture, Store sharing between the CLI and the service, Site

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Unit files: Go's launchd plist and systemd unit equal Python's byte for byte
  on 3 cases x 2 supervisors (plain, the uv-tool command, and one quoting %,
  quotes, backslash, &, <, > and non-ASCII); internal/service/testdata/units
  holds Python's output, and TestUnitFilesEqualPythonsForTheSameInputs and
  test_unit_files_equal_the_files_go_pm_is_held_to both compare against it.

- Live check under the real launchd (HOME and CODEX_HOME in /tmp, scratch
  label local.pm.clone.119ca937, service.Run with an in-memory store and
  site): the plist passes plutil -lint and install brought it to state =
  running; a POST /reply returned 303 and the inbox socket received the reply
  message (delivered=1 on the page); after kill -9, KeepAlive restarted it
  (pid 48733, then 48784); restart via kickstart -k, logs and uninstall worked
  (afterwards launchctl print exits 113 and nothing listens on :18731).
  systemd was checked only under the fake systemctl on Linux CI.

- Open conflict for P9: the work-store page says the site writes a reply as pm
  reply add does (a reply comment, then close with resolution answered), but
  Python's site keeps the need open for the agent's pm decision add --need or
  pm action done. Go's service follows Python (a reply comment by owner, need
  left open); the site's write must be settled before the cut-over.

- CI flake, not from this change:
  test_pm.py::test_a_reviewed_prs_merge_is_pushed_into_the_session_once failed
  once on PR 81's integration job with JSONDecodeError in repo.items()
  (conftest.py:400), which read the fake bd state while it was being written;
  it passes locally (1 passed, 10.97 s), and the PR touches no Python code it
  runs.

- Review (one fresh-context pass on PR 81) found one correctness bug: on a
  render error the service took the records lock and then opened the work
  store under the gate, the reverse of pm writes' order (gate, then records
  lock), a possible deadlock. Fixed in 601b5b1 with five minor findings (a
  mutex keeps store opens to one at a time per process, a failed open is
  retried at the next look, the spool cannot block startup, a reply over 1 MiB
  gets a 413, a sync step's panic is recorded as a failure). After the fix: go
  test -race over internal/... ok; make test-go parity 114 passed, 1 skipped;
  CI green on all 5 jobs.

- For the cut-over (P10): Go's push state names the work-store step 'work'
  where Python's push.json has 'beads', so on a clone with an old
  installed_at, pm service status flags 'work push overdue' until the first Go
  sync, at most 10 min after start. Go prints reply timestamps as whole-second
  UTC (2026-10-08T12:00:00Z) where Python printed bd's raw created_at.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm has the pm service's run loop and its launchd and systemd lifecycle, built against the store and site interfaces and tested with fakes.

Merged as 2c391b1 (PR #81).

- `internal/service`: the run loop, the unit files, install, status, restart, logs, uninstall and drift.
- `internal/sync`: the push steps and their state.
- A daily gc loop (`CALL DOLT_GC` through the store), carrying [sprint 81](pm-harness-81.md)'s collection over.
- Go CLI: `pm service status` and `pm service logs`; `run`, `install` and `restart` refuse until the install sprint wires the store and site.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** Go unit tests in `internal/service` with a fake site and store pass: `go test -race ./internal/service`, 21 tests; CI build-and-parity is green on macOS and Linux.
- **Met:** the launchd and systemd unit files Go writes equal Python's for the same inputs: 3 cases x 2 supervisors byte for byte, checked from both the Go and the Python test.
- **Met:** the PR, [#81](https://github.com/Yeeef/yeeef-agents/pull/81), is on main as `2c391b1`.
