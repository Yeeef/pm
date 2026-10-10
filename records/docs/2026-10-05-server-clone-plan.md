---
type: doc
title: Server clone checklist and handoff plan
date: 2026-10-05
bead: yeeef-agents-9va.37.1
---

What a fresh clone of this repo on another server needs so the owner can continue work there, and the order to hand off from the Mac (written 2026-10-05 by task yeeef-agents-9va.37.1). The Mac clone is now at `~/workspace/yeeef-agents` (sprint 31).

## Key constraint: one writing clone at a time

The scheduled `pm push` runs `bd dolt push` and pushes the `records` branch, and it never pulls: when `origin/records` moved it rebases its own commits onto it, but Beads has no such step, so if both clones write issues a `bd dolt push` is rejected until someone runs `bd dolt pull` by hand. The plan is therefore a **handoff**: the Mac pushes everything and stops writing before the server's first write. Running both at once is out of scope (a harness change for a later sprint).

## What travels and what does not

| # | Item | Travels by | Action on the server |
|---|---|---|---|
| 1 | Code, harness, skills, `AGENTS.global.md`, settings | git (`main`) | `git clone https://github.com/Yeeef/yeeef-agents.git ~/workspace/yeeef-agents` |
| 2 | Beads issues and `bd remember` memories | Dolt data at `refs/dolt/data` on `origin` | `bin/pm setup` runs `bd bootstrap`; never `bd init` |
| 3 | Records (project, sprint, day, docs) | `records` branch | `bin/pm setup` checks it out at `.records` and links `records/` |
| 4 | `~/.claude` links (CLAUDE.md, settings.json, commands, 3 skills) | made from the clone | `make setup-agent`; it refuses an existing differing file, so move any old `~/.claude/CLAUDE.md` or `settings.json` aside first |
| 5 | Scheduled push | made from the clone | `bin/pm setup` installs a systemd user timer on Linux (cron without systemd); a timer only runs while the user is logged in unless lingering is on: `loginctl enable-linger $USER` |
| 6 | Codex writable roots | made from the clone | `bin/pm setup`, if `~/.codex` exists; so install and log in to Codex **before** `bin/pm setup`, or re-run it after |
| 7 | Telegram notifications (`notify-me`) | not in git | export `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` in the server's shell profile; copy by hand, never commit |
| 8 | `.env`, `config.json` (git-ignored) | not in git | none exist in the Mac clone today; nothing to copy |
| 9 | Claude transcripts and per-project memory (`~/.claude/projects/…`) | not in git | the Mac's project folder holds no `memory/`; transcripts stay on the Mac (old sessions are not resumable on the server) |
| 10 | Claude `~/.claude.json` (trust, MCP settings) | not in git | made on first run; accept the trust prompt once |
| 11 | GitHub auth | not in git | `bd dolt push` and the records push use the `https` remote, so the server needs working git credentials for push: `gh auth login` then `gh auth setup-git` |

## Prerequisites on the server

`git`, `uv`, `bd` (1.3.1 on the Mac; use the same or newer), `gh`, `make`, `claude`, and `codex` if used. `dolt` is not needed: `bd` embeds it. Check with `for t in git uv bd gh make claude; do command -v $t || echo "missing $t"; done`.

## Handoff plan

### On the Mac, before the server writes anything
1. End every Claude and Codex session in the clone, and stop `make docs`.
2. Let the scheduled job push, or run `bin/pm push` by hand; then check `bin/pm where` shows both pushes ok and `git -C .records status -sb` shows no ahead/behind.
3. Push code: `git status --short` clean on `main` and on any branch to continue (push those branches; worktrees under `.claude/worktrees/` are local only).
4. Stop the Mac's scheduled push so it cannot push stale data later: `launchctl bootout gui/$(id -u)/local.pm-push.yeeef-agents.34034d47` (keep the plist; `launchctl bootstrap` brings it back on handback).

### On the server
1. Install the prerequisites, `gh auth login && gh auth setup-git`, log in to Claude Code (and Codex).
2. `mkdir -p ~/workspace && git clone https://github.com/Yeeef/yeeef-agents.git ~/workspace/yeeef-agents && cd ~/workspace/yeeef-agents`.
3. `make setup-agent`.
4. `bin/pm setup` (bootstraps Beads, checks out `.records`, installs hooks and the push timer); then `loginctl enable-linger $USER` if the server should push while you are logged out.
5. Set the Telegram variables if notifications are wanted.
6. `bd ready` and `bin/pm show` list the same open work as on the Mac (e.g. sprint 32 itself).

### Checks (sprint 32 Done when)
- `bin/pm where` shows the store, links and schedule installed; re-running `make setup-agent` and `bin/pm setup` prints only "ok"/"already set up".
- After a `pm` write on the server (e.g. closing task .37.2), within 10 min `bin/pm where` shows both pushes ok, and on the Mac `git -C .records pull` and `bd dolt pull` show that write.
- `make docs` on the server serves the site; reach it from your laptop with `ssh -L 8000:localhost:8000 <server>` and open http://localhost:8000.

### Handback to the Mac (later)
Reverse the handoff: stop the server's timer (`systemctl --user stop <timer>`), push, then on the Mac `git pull`, `git -C .records pull`, `bd dolt pull`, and `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/local.pm-push.yeeef-agents.34034d47.plist`.
