---
type: sprint
title: Release pm 0.3.1
bead: yeeef-agents-9va.121
---

## Goal

> What should be true when this sprint ends, and why now?

pm 0.3.1 is published, carrying PR #7's one request rule (prime.md, the owner-request judge and its reprompt), so a repo can move its pin to it. Why now: the owner asked for a patch release on 2026-10-10, after PR #7 merged.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A PR to Yeeef/pm that turns CHANGELOG.md's [Unreleased] into the [0.3.1] section, with PR #7's entry, a summary and the upgrade guide.
- The tag pm-v0.3.1 on that PR's merge commit, and the release workflow's run.

**Out:**
- Moving any repo's pin to 0.3.1 (a separate PR per repo).

## Done when

> What evidence will show the goal is met?

- gh release view pm-v0.3.1 shows a release (not a pre-release) with both tarballs, SHA256SUMS and install.sh, and the 0.3.1 changelog section as its notes.
- install.sh from the release installs a binary whose pm version prints 0.3.1, in a scratch bin dir.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-10}
Release 0.4.0, not 0.3.1: rename the changelog section to 0.4.0 and name the breaking change in the summary, then tag pm-v0.4.0 on that change on main
main carries the retire-Python-pm breaking change, and the repo rule bumps the minor for a breaking change while pm is 0.x
Answers `yeeef-agents-9va.121.3`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- PR #10 merged as ef8c15c after PR #9 (retire Python pm, another session's
  sprint 109). The merge put #9's Breaking changes entry (a repo pinned below
  0.2.0 fails every pm command until pm upgrade) inside the 0.3.1 section; the
  summary written for #7 alone does not mention it. AGENTS.md's rule bumps the
  minor for a breaking change while pm is 0.x, so the tag waits on the owner's
  choice of 0.4.0 or 0.3.1 (decision need 121.3). PR #10's checks ran on its
  own head, before #9 was on main; the changelog check on ef8c15c passed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm 0.4.0 is published (tag pm-v0.4.0 on 6960e62), as 0.4.0 rather than 0.3.1 because main carried a breaking change by release time (owner decision 2026-10-10).

Merged as ef8c15c (PR #10). Merged as 6960e62 (PR #12).

- Ships PR #7 (one request rule in prime.md, the owner-request judge and its reprompt), PR #8 (changelog-based release notes), PR #9 (Python pm retired; breaking for a repo pinned below 0.2.0) and PR #6 (`pm init --import`).
- Release notes: PR #10 wrote the section as 0.3.1; PR #12 renamed it to 0.4.0, named the breaking change in the summary, and moved PR #6's Added entry from [Unreleased] into 0.4.0, since the tag ships its code.
- No repo's pin moved; each repo moves with `pm upgrade --to 0.4.0` in its own PR.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| gh release view shows the release (not a pre-release) with both tarballs, SHA256SUMS and install.sh, and its changelog section as notes | met, for 0.4.0 | `gh release view pm-v0.4.0`: pre=false, draft=false; assets install.sh, pm-0.4.0-darwin-arm64.tar.gz, pm-0.4.0-linux-amd64.tar.gz, SHA256SUMS; notes open with the 0.4.0 summary. Release workflow run 38051154221: version, build on macOS and Linux, and release all succeeded. The tag went on 6960e62 after main's CI there passed all six checks |
| install.sh from the release installs a binary whose pm version prints the release | met | `PM_BIN_DIR=<scratch>/bin sh install.sh`: "installed pm 0.4.0 … sha256 97baaf43…"; `pm version` outside any repo prints 0.4.0 (inside yeeef-agents the launcher runs that repo's 0.3.0 pin). ~/.local/bin/pm unchanged |
