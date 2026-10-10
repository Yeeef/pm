---
type: sprint
title: "Go port: install, launcher and release (P9)"
bead: yeeef-agents-9va.100
---

## Goal

> What should be true when this sprint ends, and why now?

Go pm can be installed, pinned and released, and a repo pinned to a Go version runs it on a machine that has only the Python tool. It is the last step before this repo cuts over, and the parity gate for it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `init/doctor/upgrade/uninstall/where` on the work store, including `init --import-bd`.
- The Go launcher, `install.sh`, the release workflow building both assets, the Python bridge release 0.1.N.
- The service wired end to end to the work store.
- A Go release is a tag on any main commit: the release workflow takes the version from the tag name, no version is written in Go source, an untagged build reports `dev`, and the bridge release 0.1.N keeps the Python procedure (tag before pin, merge commit).

**Out:** migrating this repo's data and moving its pin (cut-over sprint); deleting Python pm.

## Done when

> What evidence will show the goal is met?

- A test tags an arbitrary main commit, with nothing else committed, and the release build from that tag reports the tag's version; an untagged build reports `dev`.
- `test_service`, `test_init`, `test_lifecycle` and the rewritten `test_launch` pass with `PM_IMPL=go`.
- A release-candidate tag builds both assets.
- A machine with the Python tool runs a scratch repo pinned to the Go version.
- Live-data parity on a copy of this clone, as in agent commands part one.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Distribution, Port order and coexistence
- [Work store: pm's own replacement for Beads](../design/work-store.md): Storage, Migration from bd

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-09}
Scope adds the tag-only Go release: a release is a tag on any main commit, its version taken from the tag name and stamped at build, none written in source; an untagged build reports dev; moving a pin is a separate ordinary PR.
The owner asked for it, relayed by the session that fixed Python releases in sprint 98: a version written in source forces a bump commit, a release PR and a merge-commit rule.
:::

::: decision {source=agent date=2026-10-09}
Go pm init, doctor, upgrade and uninstall manage the same repo pieces as Python pm 0.1.x (config, README, hook entries, the .beads/hooks sections, the two workflows, the .gitignore block); only the clone half differs: the work store replaces bd bootstrap, the Beads role and agent profile, the Codex roots name .pm/store/work and .pm/run instead of .beads and the uv cache, and pm sets core.hooksPath to .beads/hooks itself when it is unset.
The shared suite holds both implementations to the same repo files until the cut-over, and the pm-go page gives removing the Beads pieces to the cut-over (P10, pm upgrade); changing the pieces here would change them twice.
:::

::: decision {source=agent date=2026-10-09}
Go pm init replaces the pm uv tool's link in the bin dir with the Go binary and leaves the uv tool installed, instead of running uv tool uninstall pm as the pm-go page says.
Clones still pinned to Python pm need the uv tool: a launched Python pm checks it (tool.current) and their service units run its interpreter, so uninstalling it at the first Go init would break every such clone on the machine (fresh-context review of PR 95).
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Bridge release 0.1.5 shipped by the Python procedure (pm/AGENTS.md,
  Releasing pm): launcher change 303846e (a pin >= 0.2.0, pre-releases
  included, runs pm-v<pin>'s release binary: SHA256SUMS and
  pm-<pin>-<os>-<arch>.tar.gz, checked against its line and a kept sha256,
  written atomically to <data>/pm/pins/<pin>/pm, exec'd with
  PM_LAUNCHED/PM_LAUNCHER; every failure a hard error naming the release and
  URL); commit A d7a6359 tagged pm-v0.1.5 and pushed before commit B 2f5ae9d
  moved the pin (its pre-commit hook ran the tag's build). PR #94: CI green on
  2f5ae9d (5 jobs), merged by the sprint's agent with a merge commit, f0344d9,
  as the owner authorised for the Go port's PRs; main holds d7a6359. Evidence:
  make test 122 passed; test_launch.py 27 passed (6 new Go-pin tests against a
  local HTTP release server); live, a missing GitHub release fails with HTTP
  404 and an unreachable host with 'no answer within 10 s' after 10.0 s. The
  Python launcher treats an x86_64 Python under Rosetta
  (sysctl.proc_translated=1) as darwin-arm64, since uv may install one on
  Apple silicon.

- Live-data parity on a copy of this clone (PR head a671fe4, records at
  c4a70e3, bd export of 566 issues and 286 comments, Go pm stamped 0.1.5):
  Go's import equals tests/work_items.py's mapping on 566/566 items, all 19
  fields; 22 of 24 commands byte-equal between Python pm on the export through
  the fake bd and Go pm on the import (pm check, top-level pm show, 5
  --project, 11 open --sprint, 4 --record); the other two differ only by
  accepted differences: pm where's work-layer line (and the service label, a
  hash of each scratch clone's path) and 2 tasks' assignee in pm show --json.
  PM_PARITY_LIVE=1 make test-go: exit 0, test_go_parity 116 passed 1 skipped,
  live pages 149 compared, 147 equal and 2 equal after the assignee allow-list
  entry; constructs 16/16, fixtures 7/7, pm check 23 cases. Go pm init on a
  scratch clone with a bare origin (temp HOME, fake launchctl): refused port
  8000, which this clone's real service holds, writing nothing; with a free
  PORT it set the clone up, pushed the work store to refs/pm/work, then pm
  doctor reported every piece matching and pm where every line healthy. No new
  difference.

- Release candidates: pm-v0.2.0-rc.1 (a671fe4) failed its build check on both
  runners, since the check ran pm version inside the checkout, where the
  launcher rightly ran the repo's 0.1.5 pin (fixed in dee55be: checked in
  RUNNER_TEMP); no release was published for it. pm-v0.2.0-rc.2 (dee55be) and
  pm-v0.2.0-rc.3 (5d5a434, after the review fixes) each published a GitHub
  pre-release with pm-<X>-darwin-arm64.tar.gz (38.6 MB),
  pm-<X>-linux-amd64.tar.gz (41.5 MB), SHA256SUMS and install.sh; the darwin
  binary is 111,380,098 bytes unpacked. The workflow lets a pre-release tag
  name a PR's commit (a final Go tag must be on main). Live check
  (/tmp/pm91-live/run.sh, temp HOME, fake launchctl): machine A ran rc.3's
  install.sh and Go pm init in a new repo with a bare origin (records branch,
  refs/pm/work pushed, pin 0.2.0-rc.3); machine B with only the bridge 0.1.5
  uv tool cloned it, and pm where launched rc.3 (first launch 2 s, 2 requests,
  binary and sha256 kept in pins/0.2.0-rc.3), pm init through the bridge
  attached the work store from refs/pm/work, replaced the uv tool's link with
  the Go binary and kept the uv tool, installed the service (site answering),
  then pm where all healthy and pm doctor clean, through the Go launcher. The
  repo is private: GitHub answers HTTP 404 to the anonymous release download,
  so both runs served the release's own assets (gh release download) from a
  local mirror through PM_RELEASE_URL; how real machines download is raised as
  a decision need for the cut-over.

- Shared suite on Go pm, CI on 5d5a434 (pm-go.yml, macOS and Linux): 126
  passed, 0 xfailed, 191 skipped, and 126 transcripts equal to Python's;
  go-expected-failures.txt went from 41 entries (plus the 9 Go-pin launcher
  tests the bridge PR added) to 0. Cleared: test_init 6, test_lifecycle 7,
  test_hooks 5, test_launch 17, test_service 3, and test_pm's 10 (site
  replies, inbox push, pm push, pm init on a fresh clone). One test became
  Python-only:
  test_a_pm_from_a_local_checkout_refuses_to_install_or_check_the_tool (the pm
  uv tool, which Go retires). make test-go there: 116 passed, 1 skipped; the
  release-build test (build.sh twice in a scratch clone: tagged
  pm-v0.2.0-rc.42 reports that version, untagged reports dev) 4 passed on both
  runners; pm tests light and integration green. Locally: go test ./... every
  package ok; make test 125 passed, 136 skipped. pm-go.yml now checks out full
  history, since test_launch's real-uv test builds release 0.1.0 from tag
  pm-v0.1.0 on Go too.

- Fresh-context review of PR 95: 2 correctness findings, fixed in e4eaa6f. (1)
  pm prime --state ran pm init as a child after the launcher's markers were
  scrubbed, so a pm a Go launcher launched for a repo's pin copied the pin's
  binary over the launcher at every session start; the child now gets the
  markers (launch.Markers). (2) pm init ran uv tool uninstall pm, which breaks
  every clone on the machine still pinned to Python pm (a launched Python pm
  checks the uv tool, and its service unit runs the tool's interpreter); pm
  init now replaces only the tool's link in the bin dir (sprint decision).
  Also fixed: a kept sha256 the launcher cannot read fails hard, as Python's
  does. While merging, the init branch's own version compare refused
  pre-releases, so a pm launched by a Go rc launcher would have replaced it;
  it now uses internal/launch's Key/IsGo. Left as minor, for the cut-over:
  overridden-claim warnings are lost when the push after a merge fails (the
  next sync has nothing to merge); a bare pm upgrade by an rc launcher moves a
  0.2.0 pin down to the rc (Key drops the suffix, as Python's bridge does); pm
  uninstall checks unpushed store commits against the last sync, not the
  remote, and leaves core.hooksPath set; install.sh's wget has no overall time
  limit.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm installs, pins and releases from a tag, and a machine with only the Python tool runs a repo pinned to Go pm.

- `pm init`, `doctor`, `upgrade`, `uninstall` and `where` on the work store, the git hooks and session-start init; the service and `pm push` wired end to end.
- The Go launcher, `pm version`, the release build and workflow (`pm-release.yml`), `install.sh`; a Go release is a tag, its version taken from the tag name.
- The Python bridge release 0.1.5 ([#94](https://github.com/Yeeef/yeeef-agents/pull/94)), released by the procedure in `pm/AGENTS.md` with no hook skipped.
- Merged by the agent session under the owner's project decision that it merges the Go port PRs.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** a tag on an arbitrary main commit builds a pm that reports the tag's version, and an untagged build reports `dev`: `test_release`, 4 passed on both CI runners.
- **Met:** `test_service`, `test_init`, `test_lifecycle` and the rewritten `test_launch` pass with `PM_IMPL=go`: the shared suite on Go has 126 passed and nothing left on the expected-failures list (41 before); 126 transcripts equal Python's.
- **Met:** a release-candidate tag builds both assets: pre-releases `pm-v0.2.0-rc.2` and `pm-v0.2.0-rc.3`, each with the darwin/arm64 and linux/amd64 tarballs (38.6 MB and 41.5 MB), `SHA256SUMS` and `install.sh`.
- **Met, with assets from a local mirror:** a machine with the Python tool (bridge 0.1.5) runs a scratch repo pinned to `0.2.0-rc.3` through `pm where`, `pm init` and `pm doctor`; the private repo answers anonymous release downloads with 404, raised as decision need `yeeef-agents-9va.88.2` for the cut-over.
- **Met:** live-data parity on a copy of this clone: 566 of 566 items imported equal; 22 of 24 commands byte-equal, the other 2 differing only in `pm where`'s work-layer line and service label and in the allow-listed assignee; 149 pages, 147 equal and 2 equal after the allow-list.
- **Met:** the PR, [#95](https://github.com/Yeeef/yeeef-agents/pull/95), is on main as `77908ad`, CI green on all 9 checks of `5d5a434`.
