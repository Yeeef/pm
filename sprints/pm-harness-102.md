---
type: sprint
title: pm moves to its own public repo, Yeeef/pm
bead: yeeef-agents-9va.112
---

## Goal

> What should be true when this sprint ends, and why now?

pm lives in its own public GitHub repo, Yeeef/pm, with its source, history, CI and releases, so any machine downloads pm without a token and pm is one product of its own. The owner chose this over a releases-only repo. It comes before the cut-over: the Go launcher builds the release URL in, so the first Go release must come from the repo pm will keep.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A history of `pm/` alone, rewritten to the repo root, with no other path of yeeef-agents; a secrets scan of that whole history before anything is pushed.
- The public repo `Yeeef/pm`: CI, the release workflow, `install.sh`, and the Go launcher's and the Python bridge's release URL pointing at it.
- A bridge release that downloads Go pm from `Yeeef/pm`, released by the Python procedure.
- yeeef-agents installing pm from `Yeeef/pm`, its docs and setup naming the new repo.

**Out:** the cut-over itself (sprint 79); deleting `pm/` from yeeef-agents, which waits for the cut-over's soak; other repos.

## Done when

> What evidence will show the goal is met?

- A secrets scan of the rewritten history finds no secret, and the history holds no path outside `pm/`; the main session reviewed both before the first public push.
- `Yeeef/pm` is public, its CI is green, and a pre-release tag there publishes both assets that an anonymous `curl` downloads.
- A scratch repo with only the new bridge runs a repo pinned to that pre-release, downloading without a token.
- The sprint's PRs are on main in both repos.

## Design pages

> Where is the detail?

- [How a repo runs the right pm version](../design/pm-versioning.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-09}
The Claude-Session lines are removed from every commit message of the pm history before the first push to Yeeef/pm; authors, dates and tags stay.
The owner chose strip: the links are private session pointers nobody else can open, and publishing them could not be undone.
Answers `yeeef-agents-9va.112.5`.
:::

::: decision {source=owner date=2026-10-09}
The agent session tags pm-v0.2.0 in Yeeef/pm itself, on the merge of the standalone PR, once its CI is green and an anonymous download of every release asset works.
The owner told the agent session in chat to tag the release for them, as with merging the PR.
:::

::: decision {source=agent date=2026-10-09}
No further Python bridge release: a machine moves to Go pm by running install.sh from Yeeef/pm once; task .112.3 narrows to pointing yeeef-agents setup and docs at Yeeef/pm.
This Mac installs Go pm with install.sh in the cut-over runbook, and the Linux server and the new user set up fresh; install.sh downloads with no token from the public repo, so a bridge 0.1.7 would only save that one command at the cost of another Python release.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm/ history rewritten locally (git filter-repo --subdirectory-filter pm on
  GitHub main e61a104): 241 commits (1803 before), 12 annotated tags
  pm-v0.1.0..pm-v0.2.0-rc.5 kept, HEAD tree identical to main:pm (dc86686);
  193 paths ever, none outside pm/. Authors: yeeef/Yeeef
  (206) and Yeeef <noreply GitHub> (35). gitleaks 8.30.1 (191 non-merge
  commits) and trufflehog 3.99.2: 0 findings. Grep: one fake token
  gho_test_token in a test, fixtures synthetic (example.com, demo ids), no
  hosts, IPs, /Users paths or site URL; 178 commit messages carry
  Claude-Session URLs. The rewrite drops the 136 pre-move commits from
  skills/project-management/ (pm/ starts at 3c8cff4, 2026-10-07).

- Tags stay named pm-v* in Yeeef/pm: existing pins, the 0.1.5 and 0.1.6
  bridges and the release workflow all use that name, so a v* rename would
  need another bridge for no gain.

- Yeeef/pm is public (https://github.com/Yeeef/pm): main (241 commits, tree
  dc86686, 0 Claude-Session lines) and the 12 annotated tags
  pm-v0.1.0..pm-v0.2.0-rc.5 pushed as reviewed. Only the tags moved: the
  GitHub releases rc.1..rc.5 and their assets stay in yeeef-agents, so a
  launcher reading Yeeef/pm cannot download a pin below rc.6.

- PR Yeeef/pm#1 (4 commits, head 82412c7) makes the repo standalone: module
  github.com/Yeeef/pm, downloads with no token first and the GitHub API with a
  token only after a failed download (Go launcher, Python bridge, install.sh),
  a Python pin's commit kept in pins/<pin>/commit-Yeeef-pm (old launchers keep
  yeeef-agents' commit in pins/<pin>/commit). Local: make test 135 passed;
  make test-go exit 0; make test-go-suite 132 passed, 134 transcripts, 0
  differ; release-build test 8 passed. CI green on 82412c7 after one rerun of
  linux build-and-parity: TestMergeCloseBeatsClaimOnACleanMerge panicked in
  Dolt's TempDir cleanup (git-remote-cache walk), a flake unrelated to the
  diff.

- Pre-release pm-v0.2.0-rc.6 on Yeeef/pm (82412c7, release workflow run
  37945639080 green): anonymous curl of all 4 assets returned 200
  (darwin-arm64 38617427 B, linux-amd64 41478352 B, SHA256SUMS, install.sh),
  shasum -c OK for both tarballs, and install.sh piped from curl with no token
  installed a pm whose pm version prints 0.2.0-rc.6.

- pm-v0.2.0 released from Yeeef/pm on d0841f3 (PR #1 merged by the agent
  session under the owner's authority, CI green on all 8 checks of 82412c7):
  the release workflow succeeded; an anonymous curl gets all 4 assets with
  HTTP 200, SHA256SUMS verifies both tarballs, /releases/latest/ resolves, and
  the darwin/arm64 binary (111,396,658 bytes) reports 0.2.0.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm lives in its own public, MIT-licensed repo, [Yeeef/pm](https://github.com/Yeeef/pm), with its history, CI and releases, and pm-v0.2.0 downloads from it with no token.

- The history of `pm/` alone (241 commits, 12 tags), scanned clean and stripped of its Claude-Session lines, pushed to Yeeef/pm.
- [Yeeef/pm#1](https://github.com/Yeeef/pm/pull/1): the standalone repo, module `github.com/Yeeef/pm`, anonymous downloads first, CI and the release workflow; [Yeeef/pm#2](https://github.com/Yeeef/pm/pull/2): the MIT license.
- pm-v0.2.0 released from Yeeef/pm; yeeef-agents installs pm from it ([#98](https://github.com/Yeeef/yeeef-agents/pull/98)).
- Merged by the agent session under the owner's decision for this sprint.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** gitleaks and trufflehog found no secret in the rewritten history, which holds no path outside `pm/`; the main session reviewed both before the first public push.
- **Met:** Yeeef/pm is public and its CI is green; pre-release `pm-v0.2.0-rc.6` and release `pm-v0.2.0` publish both tarballs, `SHA256SUMS` and `install.sh`, each fetched by an anonymous `curl` with HTTP 200, the checksums verifying.
- **Met, changed by sprint decision:** no further bridge release; instead this Mac installed pm 0.2.0 with `install.sh` from Yeeef/pm, with no token, and runs this repo pinned to it (`pm doctor` clean).
- **Met:** the sprint's PRs are on main: Yeeef/pm#1 as `d0841f3`, Yeeef/pm#2 as `e05385c`, and yeeef-agents [#98](https://github.com/Yeeef/yeeef-agents/pull/98) as `945d025`.
