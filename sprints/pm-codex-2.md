---
type: sprint
title: The judge and the day summary run on Codex too
bead: pm-v498.2
---

## Goal

> What should be true when this sprint ends, and why now?

The owner-request Stop judge runs on the session's own runtime, Haiku for Claude Code and a small Codex model for Codex, and `pm day summarize` runs on a runtime the repo names. A machine with only Codex then keeps every pm guarantee, without `claude` installed. Now, because pm-codex sprint 1 made Codex a first-class runtime, but the judge still silently passes every stop where `claude` is missing.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Each runtime's hook entry names its judge runtime.
- A `codex exec` judge backend: bounded in time, with no recursion into project hooks and no stray state.
- The judge's labelled cases pass on both backends.
- A per-repo summary runtime key in `.pm/config.toml`, with no fallback.
- `pm doctor` names a missing runtime binary.
- Docs and changelog.

**Out:**
- Other model providers.
- Changing the owner-request rule itself.
- Moving any other `claude -p` use.

## Done when

> What evidence will show the goal is met?

- `make test-live` passes every labelled case on the Claude backend and on the Codex backend, `PM_LIVE_RUNS` times each, with latency reported per backend.
- The live parity eval gains a scenario where a Codex session on a machine without `claude` asks the owner only in chat, and the judge blocks it.
- `pm day summarize`, with the repo's summary runtime set to codex, writes a summary in a scratch clone.
- `pm doctor` names the configured runtime's missing binary.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
