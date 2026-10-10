---
type: sprint
title: No pm command hangs on an open stdin
bead: yeeef-agents-9va.69
---

## Goal

> What should be true when this sprint ends, and why now?

No `pm` command waits forever on a stdin that stays open, so agents in Claude Code and Codex never lose hours to a hung write. Why now: on 2026-10-07 `pm task add` hung for 7 h in one session and for 10 h in another, holding up sprint 45.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** finding why stdin sometimes stays open without EOF in agent shells; making `pm` read stdin only where a command needs it, and never block on it otherwise; the fix in both `skills/project-management/harness/pm.py` (live today) and the `pm/` package (`pm/src/pm/cli.py`, PR #52), with tests.
**Out:** changes to Claude Code or Codex; other tools that read stdin (`bd`, `gh`).

## Done when

> What evidence will show the goal is met?

- A test runs every command in `READS_STDIN` with a stdin pipe held open and never closed; each one returns (output or a refusal naming the fix) within 5 s. Expected: all pass after the fix; today `task add` and `feedback add` without `--text` hang.
- Commands whose stdin is required still work with a heredoc and a pipe, shown by the existing tests passing.
- The cause of the open stdin in agent shells is written in the sprint's Findings, or marked not found with what was tried.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Symptom, 2026-10-07: `bin/pm task add --sprint … --title …` (no description)
  ran in Claude Code's Bash and never returned; Bash moved it to the
  background at its 120 s timeout, and it was still waiting 7 h later (pid
  59187, killed). Another session's `pm task add` in a compound command had
  waited 9 h 53 min (pid 70345, left running: not this session's).

- Mechanism: `main()` reads stdin for every command in `READS_STDIN` (decision
  add, decision need, decision close, action need, doc new, project open,
  sprint open, task add, task move, feedback add without --text) through
  `stdin_text()`, which returns '' only when stdin is a tty, else
  `sys.stdin.read()` to EOF (pm.py:271 and pm/src/pm/cli.py:268). The hung
  process's stdin was a unix socket (lsof fd 0u unix), with no child process
  and no file lock held.

- Only optional-stdin commands are exposed in practice: `task add`
  (description optional) and `feedback add` without `--text`. The others are
  always called with a heredoc, which gives EOF. The read happens before the
  store lock by design, so a hung command blocks only itself, never another
  session's write.

- Intermittent: the same session ran `bin/pm task add` without stdin
  redirection successfully several times earlier (e.g. 9va.50.10, 51.8,
  51.10), then hung. Why the Bash tool's stdin socket sometimes delivers EOF
  and sometimes stays open is not found yet. Workaround that worked at once:
  `</dev/null`.

- Fix directions, not chosen: read stdin only for commands whose stdin is
  required (others take `--description -` or similar to opt in); or poll stdin
  with a short timeout (select) and treat no data as empty; or refuse when
  stdin is not a tty and has no data within N s, naming `</dev/null`. A
  timeout changes what a slow pipe means, so the explicit opt-in is the most
  direct.

- Cause, found 2026-10-07: in an interactive Claude Code session (2.1.290, run
  under --bg-pty-host) every Bash command's stdin is a unix socket whose peer
  is the claude process itself (pid 61141, its fd 23); claude never writes to
  it or closes it, so a read to EOF never returns. Seen on the hung pid 70345:
  zsh, uv and python3 all hold fd 0 on that socket. A background-job session
  (claude bg-spare) gives Bash /dev/null instead, which gives EOF at once. Not
  found: why earlier un-redirected pm task add calls in session b7282abb
  returned (04:08 and 05:03 UTC). With the fix below it no longer matters.

- Fix chosen, on the owner's call for a structural fix: pm never reads stdin.
  Every command that read it takes its text from a --text flag (feedback add
  already had one); multi-line text goes as --text "$(cat <<'EOF' … EOF)". A
  select timeout was rejected: it only shortens the hang and changes what a
  slow pipe means.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm reads stdin only for `--text-file -`, and only from a heredoc or pipe, so no command can wait on an open stdin.

Merged as d8b084a (PR #60).

- Every body command takes `--text` (one plain line) or `--text-file PATH`, where `-` reads stdin only when it is a pipe or a regular file. A tty, socket or other stdin is refused at once with the heredoc form: `pm … --text-file - <<'EOF'`.
- [PR #60](https://github.com/Yeeef/yeeef-agents/pull/60) into main: `pm/src/pm/cli.py` (`add_text()`, `read_text_file()`), `owner_request.py`, `prime.md` (a Bodies paragraph in section 3, Records; no body "on stdin" left), `pm/AGENTS.md` and the package tests. `pm hook …` also reads its hook JSON from stdin, as the hook contract requires.
- [PR #59](https://github.com/Yeeef/yeeef-agents/pull/59) made the same change in `harness/pm.py`. It was closed as superseded when main (d4e64a6, pm-v0.1.0) removed the harness; its review .69.3 was dismissed.
- Design pages pm-cli, decision-need-layout and sprint-lifecycle now describe `--text` and `--text-file`.
- A peer review found that `--text="$(cat <<'EOF' …)"` breaks on macOS bash 3.2, and that double-quoted `--text` runs backtick code spans. Both led to `--text-file -`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Open-stdin test, met. `test_no_command_waits_on_an_open_stdin` runs all 10 former stdin commands, each with and without a body, with stdin a pipe held open and never closed; each must return within 5 s. On the old pm.py all 10 no-body cases fail ("still waits on its open stdin after 5 s"); now 20/20 pass. A second test passes an open socketpair end as stdin to `--text-file -`: it is refused within 5 s. In the package: 19/20 failed before the port, 20/20 pass after.
- Required bodies still work with a heredoc and a pipe, met. `--text-file -` reads a quoted heredoc or a pipe; tests feed it a pipe with backticks, `$`, quotes and a lone `)`. Every existing test that piped a body now passes it with `--text` or `--text-file -`. Rebased on main as one commit (9075ff8, merged as d8b084a), adapted to PR #71: `decision need` now takes its parts as flags, so it is no longer a body command and left the open-stdin test's list. Light `make test` 100 passed, 35 skipped (live model); targeted 35 passed; `make test-full` 136 passed, 35 skipped. CI on 9075ff8 green: guard, light (23 s), integration (54 s). By hand: `--text-file -` with this shell's /dev/null stdin is refused at once; with a quoted heredoc holding an apostrophe, a backtick span, `$HOME` and `)` the body is read whole.
- Cause, met. Findings: interactive Claude Code (2.1.290) gives each Bash command a unix-socket stdin whose peer is the claude process, which never writes to it or closes it; background-job sessions get /dev/null. Not found: why earlier un-redirected calls in one session returned; with the fix it no longer matters.
