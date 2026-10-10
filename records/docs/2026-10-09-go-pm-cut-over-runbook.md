---
type: doc
title: Go pm cut-over runbook for this repo
date: 2026-10-09
bead: yeeef-agents-9va.88
---

Rehearsed on a scratch clone at `pm-v0.2.0-rc.5`: `pm doctor` clean in two clones, `pm export` equal to the bd export on all 568 items, 145 of 149 site pages equal and 4 differing only in blocker order (an accepted difference, sprint 86's decision). Not rehearsed: `bd dolt push` to GitHub, the real launchd unit, the Linux server.

Preconditions: the preparation PR is on main (#97); the owner has answered decision need `yeeef-agents-9va.88.4` on how bd is retired; no session holds work in this repo (`pm show`); `gh auth token` works for the user running the steps.

| # | Step | Check | Rollback |
|---|---|---|---|
| 0 | Crawl the site for the before copy (the rehearsal's `crawl.py`) | Every page saved | None needed |
| 1 | Stop the service on every reachable clone (`launchctl bootout` on macOS, `systemctl --user stop` on Linux) | `pm where` shows the service down | `pm service restart` |
| 2 | In the main clone: `bd dolt push`, then `bd export -o ~/bd-final.jsonl` | The line count equals `bd list --all --json` | Read-only |
| 3 | Release 0.2.0: `git tag pm-v0.2.0 <main commit>` and push the tag | The release has both tarballs, `SHA256SUMS` and `install.sh` | Delete the tag and release |
| 4 | On a branch in a worktree: `pm upgrade --to 0.2.0`, commit, PR, merge | The pin on main is 0.2.0 | Revert the pin to 0.1.6 |
| 5 | Main checkout: `git pull --ff-only`; `gh release download pm-v0.2.0 -R Yeeef/yeeef-agents -p install.sh -O - \| sh`; `pm init --import-bd ~/bd-final.jsonl`; `pm init`; `pm push` | `pm where` shows the work store pushed; `pm doctor` clean; `pm export` equals the bd export on every migration check | Revert the pin, `git config core.hooksPath <main>/.beads/hooks`, `pm init`; writes made after the switch exist only in the work store |
| 6 | Retire bd as decision need `yeeef-agents-9va.88.4` decides (default: `mv .beads .beads.retired`) | `bd list` fails loudly | `mv .beads.retired .beads` |
| 7 | Crawl the site again and compare with step 0 (`compare.py`) | Equal after normalisation, except blocker order | None needed |
| 8 | Linux server, once its network is back (action `yeeef-agents-9va.88.3`): `install.sh`, `git pull`, `pm init` | `pm doctor` clean | Revert as in step 5 |
| 9 | After the soak: delete Python pm and update `pm/AGENTS.md`, the pm-product page and the work store page | Python pm gone from main | Revert the PR |

Known risks from the rehearsal: `pm sync` fails until `pm init` attaches the store; a machine whose pm uv tool is older than the 0.1.6 bridge gets HTTP 404 on Go downloads; the service under launchd may lack a token, so step 5 downloads the binary interactively first; `.pm/README.md` still gives Python's install command.
