---
type: doc
title: pm feedback
date: 2026-10-07
---

Where pm got in the way, one entry per `pm feedback add`, newest last; each names the project it is about, when it is about one.

### 2026-10-06 23:10 UTC, session `<session>`

About project `pm-harness`, sprint `yeeef-agents-9va.57`.

Records validation allows no undated doc, so a fixed doc that collects entries over time (the pm feedback doc) has to carry the date of its first use in its name. What would have helped: a documented way to have one long-lived doc per project.

### 2026-10-06 23:10 UTC, session `<session>`

About project `pm-harness`, task `yeeef-agents-9va.57.1`.

pm task claim --help says --session applies 'when no session id is in the environment', but the code uses the flag first. An agent reading the help gets the wrong idea of which id is recorded. What would have helped: help text that matches the code (fixed for pm feedback add in this sprint, not yet for pm task claim).

### 2026-10-07 03:43 UTC, session `<session>`

About project `pm-harness`, sprint `yeeef-agents-9va.51`.

The owner-request Stop hook blocked a status line ('When they report I'll merge pm-service into pm-init and move on') as asking leave for an authorized step; it was a plan for work still waiting on running subagents, not an ask. Helpful: the judge should pass a stated plan that needs no reply.

### 2026-10-07 13:25 UTC, session `<session>`

About project `pm-harness`, sprint `yeeef-agents-9va.51`.

pm task add hung for hours: it reads an optional description from stdin whenever stdin is not a tty, and Claude Code's Bash gives an open socket as stdin, so it never gets EOF. Another session's pm task add hung the same way (pid 70345). Helpful: read stdin only for commands whose description is required, or only with an explicit flag such as --description -.

### 2026-10-07 19:50 UTC, session `<session>`

About project `pm-harness`.

First install into another repo (formal-methods, 2026-10-07): PORT=8001 pm init wrote every piece, then the service crashed with 'Address already in use' because a plain python http.server held :8001; init failed at its last step. Also .pm/config.toml defaults every repo to port 8000, so a second repo on one machine always collides. Helpful: pm init checks the port is free before writing anything and, when it is not, names a free one; and init picks a free default port for a new repo's config instead of 8000.

### 2026-10-08 00:01 UTC, session `<session>`

About project `pm-harness`.

The owner-request Stop hook flags how-to answers as owner requests. The owner asked 'do I fill in this Cloudflare form like this?' and the hook blocked the answer's instructions ('Fill it in like this', 'add an Allow policy') as asks that sprint work waits on. No sprint waited on them. It fired twice in one session on direct answers to the owner's own questions. What would help: skip sentences that answer a question the owner asked in that turn, or only flag asks tied to an open sprint or task.

### 2026-10-08 02:45 UTC, session `<session>`

About project `pm-harness`.

The owner-request Stop hook flagged 'Still waiting on the map of pm's own bd usage.' as a request to the owner; it was a status line about my own subagent. Helpful: only flag sentences addressed to the owner (you/please/could you), not 'waiting on'.

### 2026-10-08 04:17 UTC, session `<session>`

About project `pm-harness`.

A release bump cannot be committed: the pre-commit hook runs pm, which reads the new pin (0.1.3) in the worktree and refuses because tag pm-v0.1.3 does not exist yet (the tag follows the merge). PM_LAUNCHED=0.1.3 also refuses (the tag would build 0.1.2). Only --no-verify gets through, which agent permissions deny. Every pm command in that worktree also fails. pm needs a sanctioned path for the bump commit, e.g. a launcher that runs the worktree's own pm/ when it builds the pinned version.

### 2026-10-08 04:51 UTC, session `<session>`

About project `pm-harness`.

Two build agents in isolated git worktrees (Agent isolation: worktree) could not raise their PR reviews: pm action need --pr refuses until the sprint's delivery report is committed, and the worktree sandbox blocks writes to the records store, so the parent session had to write the reports. Helpful: let the review need be raised before the report (the report needs the merge anyway), or say in the refusal that a sandboxed agent should hand the report back.

### 2026-10-08 20:41 UTC, session `<session>`

About project `pm-harness`.

The owner-request Stop hook flagged a reply that only pointed at decision needs this session had already raised (yeeef-agents-9va.103.1 and .103.2, named by id). It fired twice in one session for that reason. It would help if the hook matched need ids cited in the reply, and the session's own open needs, before flagging.

### 2026-10-08 21:47 UTC, session `<session>`

About project `pm-harness`.

The Stop hook owner-request check flagged statements of fact as owner requests three times in one session (sprint 75): 'the claim refusal takes effect once your installed pm includes 47b1fd1' and 'the main checkout is behind' after the sprint was closed. It cost a reply each time. It would help if the check ignored conditional facts with no imperative, or skipped them when the session holds no open sprint task.

### 2026-10-08 22:09 UTC, session `<session>`

About project `pm-harness`.

pm sprint open crashed with IndexError at cli.py:1200 (cmd_sprint_open): it numbers sprints by splitting every child id of the project epic on its last dot, and a bug filed straight under the epic with a flat id (yeeef-agents-915, yeeef-agents-2se) has none. Workaround: detach those beads, open the sprint, re-attach. Would help: skip children without a dotted numeric suffix, and have pm show or pm check flag beads parented directly to a project epic, since they sit in no sprint.

### 2026-10-09 04:27 UTC, session `<session>`

About project `pm-harness`.

pm sprint open ignored the frame given on stdin and failed with "'## Goal' is missing or empty in --text"; pm prime says the frame goes on stdin. Either read stdin or have pm prime say --text.

### 2026-10-09 16:06 UTC, session `<session>`

About project `pm-harness`.

pm task move refuses a task that sits directly under the project (not in a sprint with a record), so an orphan task cannot be pulled into a new sprint as the rules ask; sprint 103 had to add a triage task beside them instead. A move from no sprint needs no scope-change decision and could just be allowed.

### 2026-10-09 19:23 UTC, session `<session>`

About project `pm-harness`.

pm task close --commit only resolves commits in this repo; work done in another repo (Yeeef/pm, a sprint here) can only name its commit in the reason, and the close warns. A --commit form like Yeeef/pm@sha or a PR URL would have helped.

### 2026-10-10 01:00 UTC, session `<session>`

About project `pm-harness`.

What happened: the owner-request Stop hook blocked three replies in a chat Q&A with no sprint or task behind it (the owner asked why the hook had raised a false alarm in another session). Each blocked sentence was a real request or an offer to the owner to paste a transcript ("I'd need the reply that was blocked and the block message"; "If you paste that blocked reply and its message, I'll name the flagged sentence"; "Without its blocked text I can't tell which."). Judged by the system prompt's taxonomy, the verdicts were consistent: an offer whose subject the current task cannot finish without counts as a request.

The gap: the rule in prime.md covers requests that sprint work waits on, but the judge cannot see whether any sprint or claimed task exists. So every request in a sprint-less chat conversation is blocked, and the only way through is to drop it. Filing a need for a chat follow-up would only add noise to the owner's inbox.

What would have helped: let the hook pass when the session holds no claimed task and has no open need (nothing in a sprint can be waiting on the reply), or give the judge that fact so it can label such asks "clarification" or "offer". Add these three sentences to tests/owner_request_cases.json once the intended verdict is decided.

### 2026-10-10 01:35 UTC, session `<session>`

About project `pm-harness`.

pm action need --pr refuses until the sprint's delivery report is written, and a plain action need that names a PR review is refused too; a sprint whose first PR is a prerequisite tool (Yeeef/pm#6 for the pm-harness move) cannot ask for that review until the sprint ends. A mid-sprint review form, or allowing --pr with a pending report, would have helped.

### 2026-10-10 02:14 UTC, session `<session>`

About project `pm-harness`, sprint `yeeef-agents-9va.117`.

Sprint 107 changed its goal (the interactive mode was dropped), but pm has no command to rename a sprint: pm task edit refuses a sprint id and pm sprint has only open and close, so the sprint keeps a stale title. A pm sprint edit --title, or pm task edit accepting a sprint, would have helped.

### 2026-10-10 03:56 UTC, session `37ac00e7-d271-54c0-863a-548f0a515c18`

About project `pm-harness`, sprint `yeeef-agents-9va.121`.

After pm-harness moved to Yeeef/pm (2026-10-10), the owner-request Stop hook blocked a reply that only reported an open need: "PR #10's review is on the new site as .121.2". The hook reads this session's open needs from the clone of the session's cwd (yeeef-agents), while the need lives in Yeeef/pm's store, so `pm show yeeef-agents-9va.121.2` finds it in Yeeef/pm and errors in yeeef-agents. A session whose cwd is one repo and whose tracked work is in another always gets false blocks. What would have helped: the hook reading the session's open needs from every clone on the machine that it has written to, or from the clone that holds the session's claimed tasks.

### 2026-10-10 03:59 UTC, session `d2440053-39b8-519d-8887-8ecd90d4282a`

About project `pm-harness`, sprint `yeeef-agents-9va.116`, task `yeeef-agents-9va.116.3`.

Triaging open sprints after the move to Go pm hit four snags.

- Dismissing a replaced PR review needs `pm need dismiss`. Neither `pm action --help` nor prime's noun list names `need`; prime only says sprint close skips a dismissed review. I found it by grepping the Go source. Listing `need` in `pm --help` and pointing to it from `pm action --help` would help.
- `pm decision close` stores its body as the owner's answer, even for a need the owner never answered and that became moot. A moot close with only a reason, or a body labelled as the agent's note, would help.
- `pm task close --commit REF` still warns that the main checkout has uncommitted changes, although REF was given. A task closed as obsolete has no commit, so I passed a records commit as a stand-in. A `--obsolete` close that needs no commit would help.
- `pm decision add` refuses while the record holds an uncommitted hand edit; the order (commit, then add) is fine once known, but the frame edit plus decision is one step in practice.

### 2026-10-10 04:00 UTC, session `d2440053-39b8-519d-8887-8ecd90d4282a`

About project `pm-harness`.

Closing a project because it moved to another repo: pm task close closed tasks held by other live sessions without a warning, while pm task claim refuses them; and there is no single command for a move, so the close-out took 14 hand-written delivery reports and 22 task closes with an unrelated --commit. A pm project move-out command, or a warning on closing a held task, would have helped.

### 2026-10-10 04:31 UTC, session `d2440053-39b8-519d-8887-8ecd90d4282a`

About project `pm-quality`.

There is no `pm sprint move`. Moving pm-harness sprints 34, 56, 105 and 110 to pm-quality and pm-codex took, per sprint, a new `pm sprint open` with the frame copied by hand, one `pm task move` per task, a hand-written Delivery report, `pm commit` and `pm sprint close`.
The move loses the sprint number: pm-harness sprint 105 is now pm-quality sprint 2, and links or chat that name the old number go stale. Findings and decisions do not follow; I copied one finding by hand.
What would have helped: `pm sprint move <id> --to <project>` that reparents the epic with its tasks, needs, findings and decisions, renames the record, and records the move as a decision in both projects.
`pm commit` also refused a path given relative to the store (sprints/x.md); it wants records/sprints/x.md. Accepting either would have saved a retry.

### 2026-10-10 04:39 UTC, session `44850292-f394-5fbf-8a0e-4d0c75e1d559`

About project `pm-quality`, sprint `pm-bfli.1`.

Moving pm-harness sprint 92 to the new pm-site project took six steps: `pm sprint open` with the frame copied out of the old record by hand, `pm task move` per task, `pm decision add` to carry each decision, a hand-written voided delivery report, `pm commit`, `pm sprint close`. The first `pm sprint open` timed out when the service restarted mid-move, leaving the move half done until checked by hand.
What would have helped: one `pm sprint move <sprint> --to <project>` that moves the frame, tasks, decisions and findings and leaves the old id pointing at the new one.

### 2026-10-10 13:04 UTC, session `ff5f58e0-8448-5f34-be91-c55611ad9ce8`

About project `pm-harness`.

In a clone that installs pm without committing .pm/config.toml (this Yeeef/pm checkout), a session whose cwd is a worktree branched from origin/main runs the Stop hooks there: `pm hook owner-request` and `pm hook stop` refuse ("this repo has no .pm/config.toml"), exit 1, and Claude Code treats that as non-blocking, so a request asked only in chat went out unchecked (sprint 79, 2026-10-10). The same refusal from pm's pre-commit hook blocked every commit on that branch. What would have helped: the hooks finding the config through the main checkout (git's common dir) rather than the cwd, or failing with exit 2 so the miss is visible.

### 2026-10-10 13:25 UTC, session `ffa2f713-c138-5cf7-8379-c247d20c2e2a`

About project `pm-quality`.

Moving a repo's pin (0.3.0 to 0.4.0, PR #14) while ten agent worktrees were mid-sprint: once the main checkout pulls the new pin and the service restarts, every worktree still on the old pin gets each pm command refused by the version handshake, until it rebases onto main. With uncommitted work, that means a WIP commit or a stash first, in every worktree.
What would have helped: `pm upgrade`'s output, or the release's upgrade guide, naming this (rebase each open worktree after the pin move), and `pm where` in a worktree on another pin saying so before the restart rather than after.

### 2026-10-10 13:31 UTC, session `ffa2f713-c138-5cf7-8379-c247d20c2e2a`

About project `pm-quality`, sprint `pm-d2k5.1`.

pm finding add --sprint pm-d2k5.1 --text="…" wrote the finding with a leading "--text=" as part of its text: finding add takes the text as a positional argument, and an unknown --text=… was taken as that positional instead of being refused. Every other body command takes --text, so the brief's form looked right. What would have helped: finding add accepting --text/--text-file like the other body commands, or refusing an argument that starts with "--".

### 2026-10-10 14:00 UTC, session `ffa2f713-c138-5cf7-8379-c247d20c2e2a`

About project `pm-harness`.

pm finding add --text="…" is accepted and stores the literal '--text=' prefix in the finding, while every other body command takes --text; it should either take --text like them or refuse it. Hit in pm-quality sprint 8.
