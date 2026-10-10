---
type: sprint
title: A clone's pm service can be stopped, uninstalled and linked locally
bead: pm-d2k5.11
---

## Goal

> What should be true when this sprint ends, and why now?

The owner or an agent can:
- stop a clone's service and keep it stopped;
- uninstall pm while the service is down;
- get a record's localhost link;
- learn that a newer release exists.

Today:
- Stopping the service needs `launchctl` or `systemctl` by hand, which agents may not run, and the owner's own `!` runs did not take (formal-methods 2026-10-09 15:43).
- Session start reinstalls or restarts a stopped service, and `pm uninstall` refuses until the service answers (formal-methods 2026-10-10 00:54).
- `pm record link` prints the public URL behind Cloudflare Access, which a headless check cannot open (formal-methods 2026-10-07 20:58).
- Nothing says that a release newer than the pin exists (formal-methods 2026-10-10 00:29).

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm service stop`: stops and disables the unit, then checks that the socket no longer answers and says so.
- Session start leaves a stopped service alone, and the state it injects names `pm service restart`. A typed `pm init` or `pm service restart` starts it again.
- `pm uninstall` with the service down checks for unsynced work without breaking the one access path (`internal/work/access_test.go`), for example by starting the service for the check. Record the choice in a design page section.
- `pm record link --local` prints `http://127.0.0.1:<port>/…` from the clone's unit port.
- `pm doctor` names a release newer than the pin, with its notes link. When the release list cannot be read, it says so; it never guesses.
- `pm service --help` and a CHANGELOG entry.

**Out:**
- Cloudflare Access (pm-harness sprint 68).
- Moving a clone between machines.

## Done when

> What evidence will show the goal is met?

- Harness tests with `fake_sched`:
  - stop disables the unit and the socket stops answering;
  - `pm init --session-start` then leaves the service stopped, and `pm service restart` brings it back;
  - `pm uninstall` with the service stopped succeeds on a synced store and refuses with an unsynced one;
  - `pm record link --local` prints the localhost URL while `site_url` is set;
  - `pm doctor` against a local release server that holds a newer release prints the line.
- These integration tests pass under `make test-full ARGS="-k …"` and in CI.
- A live check in a scratch clone under a temp `HOME` runs stop, then session start, then restart, and its output goes in Findings.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Store access, the Startup table (session start, `pm service stop`, `pm uninstall` with the service down) and its alternatives
- [pm versioning](../design/pm-versioning.md): What the user sees today, the newer-release row

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check, real systemd user instance, scratch clone of a scratch
  origin under /tmp (temp HOME and CODEX_HOME; XDG_CONFIG_HOME real so systemd
  sees the scratch unit local.pm.clone.13e3e6dc): pm service stop exit 0 in
  0.23 s, after it systemctl is-enabled=disabled, is-active=inactive; pm init
  --session-start exit 0, printed 'left the pm service stopped: it was stopped
  by pm service stop ... run pm service restart to start it', unit still
  disabled/inactive; pm prime --state carried the service line 'stopped by pm
  service stop ... run pm service restart' and pm show refused naming pm
  service restart; pm service restart exit 0 in 0.46 s, is-enabled=enabled,
  is-active=active, pm show exit 0.

- Live check, same scratch clone: with the service stopped and one
  unsynced work-store commit (pm project open), pm uninstall refused in 0.14 s
  ('holds 1 commit(s) origin's refs/pm/work lacks') through its own pm service
  run, leaving no process behind; after pm service restart, pm sync (pushed 1
  commit) and pm service stop, pm uninstall succeeded in 0.29 s and removed
  the disabled unit (systemctl: unit could not be found). pm record link
  --local demo printed http://127.0.0.1:58259/projects/demo.html, which
  answered 200. pm doctor on a scratch clone pinned 0.3.0 against the real
  GitHub API printed 'release: pm 0.4.0 is out, newer than this repo's pin
  0.3.0; what changed: https://github.com/Yeeef/pm/releases/tag/pm-v0.4.0 ...'
  with exit 0.

- Tests: make test 112 passed, 40 skipped; make test-go all ok (race
  runs included); the new and touched integration tests (pytest -k 'stop or
  uninstall or doctor or end_to_end' over test_lifecycle.py and
  test_service.py) 12 passed in 13.5 s. The fake supervisor (fake_sched.py)
  now models systemd's enabled units and launchd's disabled labels, and
  systemctl restart starts an inactive unit as systemd does; conftest points
  PM_RELEASE_API at a refusing port so no test reaches GitHub.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a clone's pm service can be stopped and stays stopped through session start, pm uninstall works with it down, pm record link --local gives the localhost URL, and pm doctor names a newer release (PR #22, merged as a274b65).

- `pm service stop`: it disables and stops the unit, and checks that the socket and the site no longer answer. The supervisor's disabled state is the record. Session start leaves the service stopped and names `pm service restart`; `pm service restart` and `pm init` enable it again.
- `pm uninstall` with the service down runs its own `pm service run` (on `PORT=0`) for the unsynced-work check, which keeps the one access path.
- `pm record link --local`.
- `pm doctor`'s release line: it names a newer release, or says that the release list cannot be read. The exit code is unchanged.
- Help texts, `prime.md`, CLAUDE.md, CHANGELOG `[Unreleased]` and the pm-go and pm-versioning design pages are updated.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Harness tests with `fake_sched`: met.
  - Stop disables the unit and the socket stops answering. A typed `pm init --session-start` then leaves the service stopped with no enable, restart or bootstrap call, and `pm service restart` brings it back: `test_stop_keeps_the_service_stopped_at_session_start_until_restart`.
  - `pm uninstall` with the service stopped refuses with one unsynced commit, and succeeds once the store is synced: `test_uninstall_with_the_service_stopped_checks_for_unsynced_work`.
  - `pm record link --local` prints `http://127.0.0.1:<port>/…` while `site_url` is set: `test_service_install_status_restart_and_logs_end_to_end`.
  - `pm doctor` against a local release API that holds `pm-v99.0.0` prints the line; when the API holds the pin it prints nothing; when nothing answers it prints the "cannot read" line: `test_doctor_names_a_release_newer_than_the_pin`.
  - Go tests cover stop, restart and install for both systemd and launchd, a stop while the socket still answers, a failed `is-enabled`, and `Newer`.
- These integration tests pass locally and in CI: met. `uv run pytest -n 8 tests/test_lifecycle.py tests/test_service.py -k "stop or uninstall or doctor or end_to_end"` gave 12 passed. `make test` gave 112 passed and 40 skipped, and `make test-go` passed. CI on PR #22 is in its Findings and the PR's checks.
- A live check in a scratch clone under a temp `HOME` runs stop, then session start, then restart, and its output goes in Findings: met.
  - It used the real systemd user instance; only `XDG_CONFIG_HOME` was real, so that systemd could see the scratch unit.
  - Stop took 0.23 s and left the unit disabled and inactive. Session start left it so. Restart took 0.46 s and left it enabled and active.
  - Uninstall with the service down refused in 0.14 s with one unsynced commit, and succeeded in 0.29 s once synced.
  - Doctor read the real GitHub API and named 0.4.0 to a clone pinned at 0.3.0.
