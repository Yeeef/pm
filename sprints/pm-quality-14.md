---
type: sprint
title: Release pm 0.5.0 and move this repo's pin to it
bead: pm-d2k5.14
---

## Goal

> What should be true when this sprint ends, and why now?

pm 0.5.0 is published with this wave's changes (option C, one feedback doc, pm clean, pm service stop, task close/move, sprint move, the Stop hooks, the site's diagrams and images, the Dolt fix), and this repo runs it, so the steps that wait on a release can run here: the feedback docs merged into one, and `pm clean --apply` on this clone.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The release PR: `[Unreleased]` becomes `## [0.5.0] - <date>` with its summary and upgrade guide; tag `pm-v0.5.0` on the merge; the release workflow publishes it.
- Install 0.5.0 on this machine, `pm upgrade --to 0.5.0` in a PR, then the upgrade guide's remaining steps in this clone (service restart, untrack records/ if asked).

**Out:**
- Moving formal-methods, ai-safety and yeeef-agents to 0.5.0.

## Done when

> What evidence will show the goal is met?

- `gh release view pm-v0.5.0` lists the four assets, and `pm version` on the installed binary prints 0.5.0.
- This repo's `.pm/config.toml` pins 0.5.0 on main, `pm where` shows 0.5.0 and the service answers, and `pm doctor` reports nothing to fix.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- 0.5.0 upgrade guide (PR #28): found two steps the merged PRs missed, both
  checked in code: pm service restart is required in each clone after the pull
  (service.StartIfDown leaves a running, stale service as it is), and a 0.4.0
  clone fails its sync over a sprint it changed that pm sprint move moved
  (0.4.0 mergeItem holds number fixed), so step 5 names pm sprint move beside
  pm feedback add. CI on a780ef4: all 10 checks pass.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm 0.5.0 is published and this repo runs it.

- Release notes: PR #28, merged as fe3631e and tagged `pm-v0.5.0`; release workflow run 38068685509 (version, both builds, release) succeeded.
- Pin move: PR #29, merged as a996c8a; `pm upgrade --to 0.5.0` also wrote the third rules hook entry.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `gh release view pm-v0.5.0` lists the four assets and the installed `pm version` prints 0.5.0: met. Assets: install.sh, pm-0.5.0-darwin-arm64.tar.gz, pm-0.5.0-linux-amd64.tar.gz, SHA256SUMS (not a pre-release); `pm version` outside a repo prints 0.5.0 after install.sh.
- This repo pins 0.5.0 on main, `pm where` shows 0.5.0, the service answers, and `pm doctor` reports nothing to fix: met. After `pm service restart`, `pm where` shows pm 0.5.0 and the service running with the site on :8001. `pm doctor` first named the sparse checkout an earlier pm set in the main checkout; `pm init` turned it off (`git config core.sparseCheckout` now unset), and `pm doctor` then reported every managed piece and the clone's setup matching (exit 0).
