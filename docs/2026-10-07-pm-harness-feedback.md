---
type: doc
title: pm feedback
date: 2026-10-07
project: pm-harness
---

Where pm got in the way, one entry per `pm feedback add`, newest last.

### 2026-10-06 23:10 UTC, session `<session>`

About sprint `yeeef-agents-9va.57`.

Records validation allows no undated doc, so a fixed doc that collects entries over time (the pm feedback doc) has to carry the date of its first use in its name. What would have helped: a documented way to have one long-lived doc per project.

### 2026-10-06 23:10 UTC, session `<session>`

About task `yeeef-agents-9va.57.1`.

pm task claim --help says --session applies 'when no session id is in the environment', but the code uses the flag first. An agent reading the help gets the wrong idea of which id is recorded. What would have helped: help text that matches the code (fixed for pm feedback add in this sprint, not yet for pm task claim).

### 2026-10-07 03:43 UTC, session `<session>`

About sprint `yeeef-agents-9va.51`.

The owner-request Stop hook blocked a status line ('When they report I'll merge pm-service into pm-init and move on') as asking leave for an authorized step; it was a plan for work still waiting on running subagents, not an ask. Helpful: the judge should pass a stated plan that needs no reply.

### 2026-10-07 13:25 UTC, session `<session>`

About sprint `yeeef-agents-9va.51`.

pm task add hung for hours: it reads an optional description from stdin whenever stdin is not a tty, and Claude Code's Bash gives an open socket as stdin, so it never gets EOF. Another session's pm task add hung the same way (pid 70345). Helpful: read stdin only for commands whose description is required, or only with an explicit flag such as --description -.

### 2026-10-07 19:50 UTC, session `<session>`

First install into another repo (formal-methods, 2026-10-07): PORT=8001 pm init wrote every piece, then the service crashed with 'Address already in use' because a plain python http.server held :8001; init failed at its last step. Also .pm/config.toml defaults every repo to port 8000, so a second repo on one machine always collides. Helpful: pm init checks the port is free before writing anything and, when it is not, names a free one; and init picks a free default port for a new repo's config instead of 8000.

### 2026-10-08 00:01 UTC, session `<session>`

The owner-request Stop hook flags how-to answers as owner requests. The owner asked 'do I fill in this Cloudflare form like this?' and the hook blocked the answer's instructions ('Fill it in like this', 'add an Allow policy') as asks that sprint work waits on. No sprint waited on them. It fired twice in one session on direct answers to the owner's own questions. What would help: skip sentences that answer a question the owner asked in that turn, or only flag asks tied to an open sprint or task.

### 2026-10-08 02:45 UTC, session `<session>`

The owner-request Stop hook flagged 'Still waiting on the map of pm's own bd usage.' as a request to the owner; it was a status line about my own subagent. Helpful: only flag sentences addressed to the owner (you/please/could you), not 'waiting on'.

### 2026-10-08 04:17 UTC, session `<session>`

A release bump cannot be committed: the pre-commit hook runs pm, which reads the new pin (0.1.3) in the worktree and refuses because tag pm-v0.1.3 does not exist yet (the tag follows the merge). PM_LAUNCHED=0.1.3 also refuses (the tag would build 0.1.2). Only --no-verify gets through, which agent permissions deny. Every pm command in that worktree also fails. pm needs a sanctioned path for the bump commit, e.g. a launcher that runs the worktree's own pm/ when it builds the pinned version.

### 2026-10-08 04:51 UTC, session `<session>`

Two build agents in isolated git worktrees (Agent isolation: worktree) could not raise their PR reviews: pm action need --pr refuses until the sprint's delivery report is committed, and the worktree sandbox blocks writes to the records store, so the parent session had to write the reports. Helpful: let the review need be raised before the report (the report needs the merge anyway), or say in the refusal that a sandboxed agent should hand the report back.

### 2026-10-08 20:41 UTC, session `<session>`

The owner-request Stop hook flagged a reply that only pointed at decision needs this session had already raised (yeeef-agents-9va.103.1 and .103.2, named by id). It fired twice in one session for that reason. It would help if the hook matched need ids cited in the reply, and the session's own open needs, before flagging.

### 2026-10-08 21:47 UTC, session `<session>`

The Stop hook owner-request check flagged statements of fact as owner requests three times in one session (sprint 75): 'the claim refusal takes effect once your installed pm includes 47b1fd1' and 'the main checkout is behind' after the sprint was closed. It cost a reply each time. It would help if the check ignored conditional facts with no imperative, or skipped them when the session holds no open sprint task.

### 2026-10-08 22:09 UTC, session `<session>`

pm sprint open crashed with IndexError at cli.py:1200 (cmd_sprint_open): it numbers sprints by splitting every child id of the project epic on its last dot, and a bug filed straight under the epic with a flat id (yeeef-agents-915, yeeef-agents-2se) has none. Workaround: detach those beads, open the sprint, re-attach. Would help: skip children without a dotted numeric suffix, and have pm show or pm check flag beads parented directly to a project epic, since they sit in no sprint.

### 2026-10-09 04:27 UTC, session `<session>`

pm sprint open ignored the frame given on stdin and failed with "'## Goal' is missing or empty in --text"; pm prime says the frame goes on stdin. Either read stdin or have pm prime say --text.

### 2026-10-09 16:06 UTC, session `<session>`

pm task move refuses a task that sits directly under the project (not in a sprint with a record), so an orphan task cannot be pulled into a new sprint as the rules ask; sprint 103 had to add a triage task beside them instead. A move from no sprint needs no scope-change decision and could just be allowed.

### 2026-10-09 19:23 UTC, session `<session>`

pm task close --commit only resolves commits in this repo; work done in another repo (Yeeef/pm, a sprint here) can only name its commit in the reason, and the close warns. A --commit form like Yeeef/pm@sha or a PR URL would have helped.

### 2026-10-10 01:00 UTC, session `<session>`

What happened: the owner-request Stop hook blocked three replies in a chat Q&A with no sprint or task behind it (the owner asked why the hook had raised a false alarm in another session). Each blocked sentence was a real request or an offer to the owner to paste a transcript ("I'd need the reply that was blocked and the block message"; "If you paste that blocked reply and its message, I'll name the flagged sentence"; "Without its blocked text I can't tell which."). Judged by the system prompt's taxonomy, the verdicts were consistent: an offer whose subject the current task cannot finish without counts as a request.

The gap: the rule in prime.md covers requests that sprint work waits on, but the judge cannot see whether any sprint or claimed task exists. So every request in a sprint-less chat conversation is blocked, and the only way through is to drop it. Filing a need for a chat follow-up would only add noise to the owner's inbox.

What would have helped: let the hook pass when the session holds no claimed task and has no open need (nothing in a sprint can be waiting on the reply), or give the judge that fact so it can label such asks "clarification" or "offer". Add these three sentences to tests/owner_request_cases.json once the intended verdict is decided.

### 2026-10-10 01:35 UTC, session `<session>`

pm action need --pr refuses until the sprint's delivery report is written, and a plain action need that names a PR review is refused too; a sprint whose first PR is a prerequisite tool (Yeeef/pm#6 for the pm-harness move) cannot ask for that review until the sprint ends. A mid-sprint review form, or allowing --pr with a pending report, would have helped.

### 2026-10-10 02:14 UTC, session `<session>`

About sprint `yeeef-agents-9va.117`.

Sprint 107 changed its goal (the interactive mode was dropped), but pm has no command to rename a sprint: pm task edit refuses a sprint id and pm sprint has only open and close, so the sprint keeps a stale title. A pm sprint edit --title, or pm task edit accepting a sprint, would have helped.

### 2026-10-10 03:56 UTC, session `37ac00e7-d271-54c0-863a-548f0a515c18`

About sprint `yeeef-agents-9va.121`.

After pm-harness moved to Yeeef/pm (2026-10-10), the owner-request Stop hook blocked a reply that only reported an open need: "PR #10's review is on the new site as .121.2". The hook reads this session's open needs from the clone of the session's cwd (yeeef-agents), while the need lives in Yeeef/pm's store, so `pm show yeeef-agents-9va.121.2` finds it in Yeeef/pm and errors in yeeef-agents. A session whose cwd is one repo and whose tracked work is in another always gets false blocks. What would have helped: the hook reading the session's open needs from every clone on the machine that it has written to, or from the clone that holds the session's claimed tasks.
