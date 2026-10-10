---
type: doc
title: Clone move inventory and plan
date: 2026-10-05
bead: yeeef-agents-9va.36.1
---

Every place on this Mac that names the clone's path `~/Desktop/workspace/yeeef-agents` (inventory taken 2026-10-05 by task yeeef-agents-9va.36.1, read-only), and the plan to move it to `~/workspace/yeeef-agents`. No reason against that target was found: it is outside the macOS-protected folders (Desktop, Documents, Downloads) and nothing else lives there yet.

## Inventory

| # | Item | Where | What names the old path | How it is updated | Who |
|---|---|---|---|---|---|
| 1 | `CLAUDE.md`, `settings.json`, `commands` links | `~/.claude/` | 3 absolute symlinks | Delete the 3 links, re-run `make setup-agent` from the new path (it refuses to overwrite an existing link that differs) | agent |
| 2 | Skill links `notify-me`, `playwright-cli`, `project-management` | `~/.claude/skills/` | 3 absolute symlinks | Same as 1 | agent |
| 3 | Main checkout and 16 linked worktrees (15 under `.claude/worktrees/`, plus the `.records` store) | `git worktree list` | Each worktree's `.git` file (`gitdir: …/.git/worktrees/<name>`) and each `.git/worktrees/*/gitdir` back-pointer (16) | `git worktree repair` from the new main checkout; the worktrees move with it because all live inside it | agent |
| 4 | `core.hooksPath` | `.git/config` | `…/.beads/hooks` | `git config core.hooksPath <new>/.beads/hooks` (or `bin/pm setup` if it rewrites a stale value; verify) | agent |
| 5 | `records/` link | main checkout and each worktree | absolute symlink to `…/.records` | `bin/pm setup` in each worktree (or the `post-checkout` hook); verify with `bin/pm where` | agent |
| 6 | Codex project trust and `writable_roots` | `~/.codex/config.toml` | `[projects."…/yeeef-agents"]` and 4 of 5 writable roots (`.beads`, `.git`, `.git/worktrees/-records`, `.records`) | `bin/pm setup` adds the new roots; the old entries are stale and are removed by hand (edit) | agent |
| 7 | Codex session rollouts | `~/.codex/sessions` | 4 files name the path (history only) | Nothing | — |
| 8 | launchd push job | `~/Library/LaunchAgents/local.pm-push.yeeef-agents.8d702f4c.plist` (loaded, last exit 126) | ProgramArguments `…/bin/pm`, WorkingDirectory, StandardErrorPath `…/.git/pm-push.log` | `launchctl bootout gui/$UID/<label>`, delete the plist, reinstall from the new path with the scheduler's install command (PR #34; the label suffix may be a hash of the path, so the new one can differ) | agent |
| 9 | Claude Code project folders | `~/.claude/projects/-Users-yeeef-Desktop-workspace-yeeef-agents*` (6: main plus 5 worktree folders) | Folder names are the encoded path; they hold transcripts, tool results and memory | Leave in place (see below) or copy the main folder to the new name | owner choice, default leave |
| 10 | Claude Code global state | `~/.claude.json` | 1 project key (per-project trust, allowed tools, MCP settings) | Nothing; Claude creates a fresh entry for the new path on first run (trust prompt once) | owner, on first run |
| 11 | uv script environments | `~/.cache/uv/environments-v2/pm-*` (42) | Keyed by script content and path; no `pyvenv.cfg` names the clone | Nothing; uv makes a new env on first `pm` run; old ones age out with `uv cache prune` | — |
| 12 | VS Code recent list | `~/Library/Application Support/Code/User/globalStorage/storage.json` | 5 mentions (recent folders/windows) | Open the new folder once; old entries are harmless | owner |
| 13 | Cursor, JetBrains, tmux, iTerm, ghostty, crontab, shell rc files, PATH | — | 0 mentions; no crontab | Nothing | — |
| 14 | Tracked files | `projects/hn-filter/config.example.json`, `projects/telegram-bridge/{README.md,codex-telegram-bridge.py}`, `codex-rules/default.rules`, `commands/global-knowledge.md`, `skills/notify-me/SKILL.md` | All name `~/Desktop/workspace/yeeef-agents` (the Linux machine), not this Mac's path | Nothing for the move (out of scope: harness changes); `.claude/settings.json` and `.codex/hooks.json` have 0 | — |
| 15 | Beads | `.beads/` | 0 absolute paths outside `issues.jsonl` | Nothing | — |
| 16 | Running processes | see below | cwd or argv in the old path | Stop before the move | owner |

Counts: 6 `~/.claude` links, 17 worktrees (16 gitdir back-pointers + 16 `.git` files), 1 git config value, 1 Codex config section plus 4 writable roots, 1 launchd plist (3 path fields), 6 Claude project folders, 1 `~/.claude.json` key, 1 editor recent list (5 entries); 0 in shell, PATH, crontab, terminal, Beads.

Running now (2026-10-05 20:00): 17 `claude` processes, 9 with cwd in the clone (including this session, 1bdea93a); a `claude daemon` and pty host spawned from it; `pm.py serve` (pid 99826/99830); several `pm reply wait` and `reply_wait_hook.py` loops (for 9va.31.3, 9va.30.9).

### Claude transcripts and memory
Claude Code maps a project to `~/.claude/projects/<cwd with / replaced by ->`. After the move, sessions started in `~/workspace/yeeef-agents` write to `-Users-yeeef-workspace-yeeef-agents`; `claude --resume` and `--continue` from the new path will not list the old sessions, and per-project memory under the old folder is not loaded. Sprint 22's liveness check (`pm.py` session transcripts) globs `projects/*/<session id>.jsonl` across every project folder, so it keeps working whatever the folder is called. Options: (a) leave the old folders: history stays readable by path, resume of old sessions needs `cd` to an old path that no longer exists, so effectively lost; (b) copy `-Users-yeeef-Desktop-workspace-yeeef-agents` to the new name before first use: resume and memory carry over, but transcripts still contain old absolute paths (harmless text). Default: (b) copy, keep the original until the move is checked, then delete the original. All live sessions end before the move anyway.

## Move plan

### Pre-move checklist
1. Owner approves a move window and ends every Claude and Codex session whose cwd is in the clone (9 now), including the one doing the move, which must not run from the old path: run the move from a new session started in `~` (or a plain terminal).
2. Stop `pm serve` (pid 99826), the `pm reply wait` loops and `claude daemon` children; close VS Code windows on the clone.
3. `launchctl bootout gui/$(id -u)/local.pm-push.yeeef-agents.8d702f4c` so it cannot fire mid-move.
4. Push records and Beads: `git -C .records status -sb` in sync, `bd dolt push` (or the harness's sync), `git status --short` recorded per worktree (uncommitted work moves with the tree; nothing is lost by `mv`).
5. Check nothing still holds the tree: `ps -axo pid,command | grep -F Desktop/workspace/yeeef-agents` is empty.

### Steps (target `~/workspace/yeeef-agents`)
1. `mkdir -p ~/workspace && mv ~/Desktop/workspace/yeeef-agents ~/workspace/` (same volume: a rename, instant).
2. `cd ~/workspace/yeeef-agents && git worktree repair` then `git worktree list` (17, all at the new path; `.records` included).
3. `git config core.hooksPath ~/workspace/yeeef-agents/.beads/hooks`.
4. Remove the 6 old `~/.claude` links (they now dangle), then `make setup-agent`.
5. `bin/pm setup` in the main checkout, then in each worktree (or `git worktree list --porcelain` loop) to relink `records/`; it adds the new Codex roots.
6. Edit `~/.codex/config.toml`: rename the `[projects."…"]` key and drop the 4 old writable roots.
7. Delete the old plist and install the push job from the new path.
8. Copy `~/.claude/projects/-Users-yeeef-Desktop-workspace-yeeef-agents` to `-Users-yeeef-workspace-yeeef-agents` (option b).
9. Start a Claude session in the new path (accept trust), open the folder in VS Code.

### Post-move checks (sprint 31 Done when)
- `bin/pm where` shows store, links, worktrees and schedule at the new path.
- Re-running `make setup-agent` and `bin/pm setup` changes nothing.
- `find ~/.claude -maxdepth 2 -type l -exec readlink {} + | grep -c Desktop` is 0; `git worktree list | grep -c Desktop` is 0; `grep -c Desktop/workspace/yeeef-agents ~/.codex/config.toml` is 0.
- The launchd job pushes a fresh records commit by itself: `launchctl list | grep pm-push` exit 0 and `git -C .records status -sb` in sync (task 9va.36.3).

### Rollback
Before step 4 nothing outside the tree has changed: `mv ~/workspace/yeeef-agents ~/Desktop/workspace/` and `git worktree repair` restore it. After: also restore `core.hooksPath`, re-run `make setup-agent` and `bin/pm setup` from the old path (after deleting the new links), revert `~/.codex/config.toml` (copy it to `config.toml.bak` before step 6), and reinstall the plist from the old path (it will still exit 126, the current state).
