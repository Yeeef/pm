---
type: sprint
title: "Ship 0.8.0 here: pin move, service, Codex trust, and closing sprint 2"
bead: pm-v498.4
---

## Goal

> What should be true when this sprint ends, and why now?

This repo runs pm 0.8.0, the first release with the Codex work: its pin moved, its service restarted with codex on PATH, and pm's Codex hooks trusted, so a Codex session here gets every pm guarantee. pm-codex sprint 2 is closed too, which needs pm to restamp a PR review closed without its main merge commit. Now, because 0.8.0 is published (pm-v0.8.0, 2026-10-13) and nothing uses it yet.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The pin-move PR (`pm upgrade --to 0.8.0`), with `design_sources` added to `.pm/config.toml` if the release carries the key (it does not: the gate ships later).
- `pm service install` from a shell with codex 0.149.0 or later.
- The owner's one-time Codex hook trust.
- `pm action done` restamping a closed PR review that lacks "merged as <sha>", then closing sprint 2.

**Out:**
- Other repos' pin moves.
- The release policy, which is its own sprint in pm-harness.

## Done when

> What evidence will show the goal is met?

- `.pm/config.toml` on main pins 0.8.0, and `pm version` in the main checkout prints 0.8.0.
- `pm doctor` in the main checkout prints no codex service line and no "Codex skips" line.
- `pm sprint close pm-v498.2` succeeds.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): `pm action need --pr` refuses a PR whose base is not main; `pm action done` restamps a review closed without `merged as <sha>`.
- [pm CLI](../design/pm-cli.md): the `pm action done` and `pm action need --pr` rows; `covers:` follows the word wrapper to `internal/textwrap`.
- [Work store](../design/work-store.md): `work.SetCloseReason` in "Inside pm", and how the close group merges after a restamp.
- [pm as an installable product](../design/pm-product.md): `config.toml` is pm's whole, every key it reads kept by `pm upgrade` and compared by `pm doctor`, `design_sources` included; `covers:` gains `internal/cli/install.go`.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
The design page pm-go stays as it is for this sprint's code changes.
PR 66 adds a test-only config.Keys and moves settingsOf into install as SettingsOf; the package table and dependency diagram (install imports config) stay true
:::

::: decision {source=agent date=2026-10-13}
The design page pm-versioning stays as it is for this sprint's code changes.
PR 66 only makes pm upgrade keep design_sources; the config row (unknown keys fail hard, the version key must equal the running pm) and the pin and launcher behavior are unchanged
:::

::: decision {source=agent date=2026-10-13}
The design page decision-need-layout stays as it is for this sprint's code changes.
PR #61 touched needs.go only for pm action need --pr's check that the PR targets main and the restamp path of pm action done; a decision need's layout did not change
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: the pin moved to 0.8.3, the newest release, which carries the review restamp (PR #61, pm 0.8.2) and keeps `design_sources` (PR #66, pm 0.8.3); the service runs with codex, `design_sources` is set, and sprint 2 is closed; the owner's Codex trust step remains.

- PR #60 (merged as 21dcf17): `pm upgrade --to 0.8.2`, redone from 0.8.0 once 0.8.1 and 0.8.2 were out: the pin, the hook entries for both runtimes, `.codex/rules/pm.rules`, and the post-checkout marker.
- PR #61 (merged as 83e1787, released as pm-v0.8.2): `pm action done` restamps a closed PR review that lacks "merged as <sha>"; `pm action need --pr` refuses a PR whose base is not main.
- PR #66 (merged as 1036382, released by CI as pm-v0.8.3): `pm upgrade` and `pm doctor` keep `design_sources`. 0.8.2's config writer dropped the key, so setting it read as drift in `pm doctor`, and the next `pm upgrade` would have deleted it. `TestConfigTextKeepsEveryKey` rewrites a config that sets every accepted key and reads it back equal.
- PR #67 (merged as 783b2d1): pin 0.8.3 and `design_sources = ["internal/**", "cmd/**", "prompts/**", "prime.md", "style.css", "assets.go", "install.sh", "release/build.sh"]`: the planned list plus the other shipped files. `go.mod` and `go.sum` are left out, since a dependency bump would hold its PR to the pm-go page. Covers were added for `assets.go` (pm-go), `install.sh` (pm-versioning), `internal/cli/help.go` (pm-cli) and `internal/cli/install.go` (pm-product).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| `.pm/config.toml` on main pins 0.8.0; `pm version` in the main checkout prints 0.8.0 | met, at 0.8.3 | PR #60 (21dcf17) pinned 0.8.2; PR #67 (783b2d1, CI 12/12) pins 0.8.3, the newest release; after `git pull --ff-only` and `pm service restart`, the main checkout's `pm version` prints 0.8.3 |
| `pm doctor` prints no codex service line and no "Codex skips" line | met in this repo | `pm doctor` in the main checkout at 0.8.3 (2026-10-13, after PR #67): one line, "every managed piece and the clone's setup match what pm init makes", exit 0; no codex service line and no "Codex skips" line, and `pm init` prints no Codex line either. Earlier the same day it printed two "Codex skips" lines (2 of 12 hooks untrusted, 5 of 12 changed since trusted). The owner's trust action pm-v498.1.3 names each repo Codex works in; only this one is checked here |
| `pm sprint close pm-v498.2` succeeds | met | `pm sprint close pm-v498.2`: "closed pm-v498.2: done … stamped the Outcome: Merged as 1cd0fa4 (PR #56)", once agent-lifecycle, pm-go and session-claims were brought to the new state (records fdb8216) and pm-versioning was waived. Before that, with review pm-v498.2.4 restamped as merged as 1cd0fa4 (pm 0.8.2), the design-page gate (pm 0.8.1) refused the close: PR #56 changed code those four pages cover |
| Design pages of changed areas updated and listed | met | Sprint lifecycle, pm CLI, Work store (records ad9b8e2, 6f2cdc4); pm as an installable product for PR #66 (records ad43da2), with pm-go and pm-versioning waived for it |
