---
type: doc
title: Bring yeeef-agents and pm work back to the Linux server
date: 2026-10-09
bead: yeeef-agents-9va.88.3
---

Moves the server's clone of yeeef-agents onto Go pm and its work store, as this Mac's clone was moved on 2026-10-09, and sets the server up to develop pm itself in Yeeef/pm. Run it on the server, in order; each step has its check. `<clone>` is the server's existing yeeef-agents checkout.

## 0. Before you start

| Check | Command | Expected |
|---|---|---|
| Network to GitHub | `git ls-remote https://github.com/Yeeef/pm.git pm-v0.2.2` | one line |
| Machine | `uname -sm` | `Linux x86_64` (Go pm ships linux-amd64 only; an arm64 server needs a build from source, section 6) |
| GitHub login, for the private yeeef-agents | `gh auth status` | logged in |
| Tools | `git --version; curl --version` | both present; `bd` and `uv` are no longer needed |
| systemd user session, for the pm service | `systemctl --user status` | answers; for the service to run while you are logged out: `loginctl enable-linger "$USER"` |

## 1. Stop what the old setup runs

The clone was last set up by the pre-package harness or by Python pm. Stop its background jobs so nothing writes during the move.

| Step | Command | Check |
|---|---|---|
| Old push job (timer or cron) | `systemctl --user list-timers \| grep pm-push`; `crontab -l \| grep pm-push` | note what exists |
| Stop and remove it | `systemctl --user disable --now local.pm-push.<name>.<hash>.timer` and remove its `.service`/`.timer` from `~/.config/systemd/user/`, then `systemctl --user daemon-reload`; or delete the line with `crontab -e` | neither command above lists it |
| A Python pm service, if any | `systemctl --user list-units 'local.pm.*'`; `systemctl --user disable --now local.pm.<name>.<hash>.service` | not listed as active |

## 2. Install Go pm

```bash
curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh | sh
pm version          # 0.2.2 or later
```

`install.sh` puts `pm` in `~/.local/bin` (make sure it is on `PATH`, ahead of any older `pm`: `which -a pm`). It replaces a `pm` link to the old uv tool; repos that still pin Python pm keep working through it.

## 3. Bring the clone to main

Beads is retired in this repo: main tracks a regular file named `.beads`, and the server's `.beads/` directory (its bd database and hooks) is in the way.

```bash
cd <clone>
git status --short                     # commit or stash your own work first
mv .beads .beads.retired               # keeps the server's Beads copy; nothing reads it any more
git fetch origin && git checkout main && git pull --ff-only origin main
cat .beads                             # "bd is retired in this repo …"
grep -m1 '^version' .pm/config.toml    # the Go pin, 0.2.2 or later
```

Check: `bd list` fails with "not a directory".

## 4. Move the old harness's leftovers by hand

Go pm refuses a clone that still holds the pre-package harness's pieces, and the fix it names (Python pm's `pm init`) cannot run in a repo pinned to Go. Move them by hand:

| Piece | Present if | Move |
|---|---|---|
| The old records store | `ls -d .records` | `mkdir -p .pm/store && git worktree move .records .pm/store/records` |
| The old push job's state | `ls .git/pm-push.*` | `rm .git/pm-push.json .git/pm-push.lock .git/pm-push.log` |
| The old `records/` link | `ls -l records` points at `.records` | `rm records` (pm init links it again) |

Check: `git worktree list` shows the records store at `<clone>/.pm/store/records` on branch `records`, or no records worktree at all (pm init then checks it out).

## 5. Set the clone up with Go pm

```bash
pm init      # clones the work store from origin's refs/pm/work, checks out records, points core.hooksPath at .pm/hooks, installs the systemd user service
pm doctor    # "every managed piece and the clone's setup match what pm init makes"
pm where     # work: 0 ahead, 0 behind origin refs/pm/work; service: running
```

Checks against this Mac's clone:

| Check | Command on both machines | Expected |
|---|---|---|
| Same items | `pm export \| wc -l` | equal |
| Same state | `pm show` | the same projects, sprints and needs |
| Site | `curl -s -o /dev/null -w '%{http_code}' localhost:<port>/` (the port `pm where` names) | 200 |

The two clones sync through the git remote: each service pushes its work store and records every 10 minutes, and a concurrent edit merges by the work store's rules. Each clone has its own service and site; the public site (pm.yeeefs.com) stays served from this Mac.

Agent setup on the server: `make setup-agent` (links `~/.claude/CLAUDE.md`, `settings.json`, `commands` and the skills). Codex: `pm init` already added the work store and `.pm/run` to `writable_roots`.

## 6. Develop pm itself on the server

pm's source lives in the public repo Yeeef/pm; `pm/` in yeeef-agents is the frozen copy, deleted after the soak.

```bash
git clone https://github.com/Yeeef/pm.git ~/workspace/pm
cd ~/workspace/pm && cat AGENTS.md          # layout, tests, the release procedure
go version                                   # go1.26.2 or later (go.mod)
gcc --version                                # cgo is required: Dolt links gozstd
make test && make test-go                    # the light set and the Go tests with -tags gms_pure_go
```

- Change code only in a worktree or branch of your own and open a PR there; CI runs `pm tests`, `pm go` and builds both release targets.
- A release is a tag on a main commit: `git tag -a pm-v<X> <commit> -m "…" && git push origin pm-v<X>`; the workflow builds and publishes both assets. Moving a repo's pin is a separate PR there: `pm upgrade --to <X>`.
- Never run a dev build as the `pm` on `PATH`; run it by path (`./pm …`) so hooks and the service keep running the pinned release.

## 7. Report back

Run `pm action done yeeef-agents-9va.88.3 --reason "<pm doctor line; pm export count on both machines>"` from the server, or tell the session that runs the cut-over. That closes the Linux part of the cut-over (sprint 79) and sprint 63.

## Rollback

Before step 5 changes anything: `git checkout <old branch>`, `mv .beads.retired .beads`, restore the old push job. After step 5: `systemctl --user disable --now local.pm.<name>.<hash>.service`, then the same; the work store's data also lives on origin, so nothing written there is lost.
