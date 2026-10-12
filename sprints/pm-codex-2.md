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
- Every existing design page whose area this sprint changes is brought to the new state, and the sprint's Design pages section lists it.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): the judge on each session's runtime, the `codex exec` call, its no-recursion and timeouts, the live eval on both runtimes.
- [Views and the site](../design/views-and-site.md): the Today summary on the runtime `summary_runtime` names.
- [pm in Codex](../design/pm-in-codex.md): gap 8 closed; gap 1's warning to a session whose hooks did not run.
- [pm as an installable product](../design/pm-product.md): `summary_runtime` in `.pm/config.toml`, `pm doctor`'s runtime check, the Stop entries' `--runtime`.
- [pm CLI](../design/pm-cli.md): `pm day summarize`'s row.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-12}
Done when gains: every existing design page whose area the sprint changes (the owner-request hook, the day summary, config) is brought to the new state and listed in Design pages.
Owner rule for every pm-codex sprint, 2026-10-12.
:::

::: decision {source=agent date=2026-10-12}
Scope gains: a pm command run in a Codex session whose rollout holds no pm session-start context warns, once per session, that Codex skipped pm hooks and names the fix (trust them in /hooks).
Gap 1 detail on pm-in-codex: a Codex session in a repo whose pm hooks are untrusted gets no signal at all, since its hooks are what does not run; 0 of 43 sessions in yeeef-agents had pm context and none said so.
:::

::: decision {source=agent date=2026-10-12}
Both hook entries name their judge: pm hook owner-request --runtime claude in .claude/settings.json and --runtime codex in .codex/hooks.json; a flagless entry runs the Claude judge, its old meaning.
The owner decision says each runtime entry names it, never guessed; naming both keeps the entries symmetric, and pm upgrade rewrites both files anyway since the Codex entry changes.
:::

::: decision {source=agent date=2026-10-12}
pm doctor fails on a missing CLI only for the runtime summary_runtime names, and never for a judge.
The pm service runs the summary unattended every 10 minutes; each judge runs on the runtime of the session that stops, which runs only where that runtime is installed, so a Codex-only machine must not fail for lacking claude.
:::

::: decision {source=agent date=2026-10-12}
The Codex judge is gpt-5.6-luna at medium reasoning effort, 40 s timeout inside a 45 s Codex hook entry; the Claude judge keeps 15 s inside 30 s.
At low effort the live eval missed 3 or 4 of 189 runs; at medium 189 of 189, median 5.5 s, p90 14 s; 8 concurrent calls pushed the tail past 25 s.
:::

::: decision {source=agent date=2026-10-12}
The Codex judge keeps the user own CODEX_HOME AGENTS.md as context; pm does not make a temp CODEX_HOME for it.
Codex loads that file with no flag to turn it off, and a temp home would need the user auth.json copied or linked, which a token refresh could break; the judge instructions replace Codex base prompt and the live eval ran with the file and passed.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Judge live eval, 63 labelled cases x 3 runs, 8 calls at a time: claude
  189/189 (median 2.01 s, p90 3.59 s, max 11.64 s); codex gpt-5.6-luna medium
  189/189 (median 5.48 s, p90 14.10 s, max 19.69 s). Before the prompt fix
  codex missed 4 cases (Step 1 skipped PR merge/review states not addressed to
  the owner; how-to steps read as actions); at low effort it missed 3 to 4 of
  189 runs.

- No recursion, live: with a user Stop hook in CODEX_HOME/hooks.json that
  touches a marker, a plain codex exec ran it (control, 7.0 s); three real pm
  hook owner-request --runtime codex calls (4.3 to 6.6 s, each a block) did
  not. Codex still loads CODEX_HOME/AGENTS.md: no flag turns it off (codex-rs
  0.154 codex-home/src/instructions).

- pm day summarize on codex, live in a scratch clone with no claude on PATH:
  wrote days/2026-10-12.summary.json (model gpt-5.6-luna) in 6.5 s, no rollout
  left in CODEX_HOME. First try failed: on a quiet day luna answered nothing
  (Haiku wrote a bullet); the prompt now asks for at least one bullet.

- Parity live eval with the new Codex-without-claude chat-ask scenario,
  PM_PARITY_CODEX_SANDBOX=danger-full-access: 8 passed in 96.5 s; the new
  scenario 27.0 s, one Stop block quoting the ask, then a need raised.

- Codex hooks warning, live: codex exec in a scratch clone with the project
  trusted and pm hooks not; the agent ran pm show and the rollout holds the
  warning (12.9 s run, no pm rules in the rollout, marker warned).

- Partial trust, live: a scratch clone whose two Stop entries' Codex trust was
  stale (as after a pm upgrade rewrote them); a codex exec session's start
  context opened with the line naming both Stop hooks as changed since trusted
  and the /hooks fix (7.5 s run); rules chunks 3 of 3 present, so the rollout
  check alone would not have warned. The cache key must be the [hooks] table,
  not config.toml: pm init rewrites writable_roots at each start.

- Full live parity set on the sprint 2 head 730eec3, after merging PR #47's
  codex queue delivery (PM_PARITY_CODEX_SANDBOX=danger-full-access make
  test-live-parity): 10 passed in 92.22 s, including Codex with no claude
  blocked asking only in chat, and the idle interactive session woken by the
  reply (Codex 2.5 s after the reply, through codex queue).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the owner-request judge runs on the session's own runtime and the day summary on the repo's `summary_runtime`, so a machine with only Codex keeps every pm guarantee; delivered in [PR #56](https://github.com/Yeeef/pm/pull/56), stacked on PR #47.

- `pm hook owner-request --runtime claude|codex`, named by each hook entry: Claude Haiku through `claude -p` (15 s in a 30 s entry), `gpt-5.6-luna` at medium effort through `codex exec` (40 s in a 45 s entry), run from an empty temp dir with Codex's hooks, rules and user config off and no saved session; the same exit-1 failure on both; a timeout or a stopped hook kills the judge's whole process group.
- `summary_runtime` in `.pm/config.toml` (`claude` by default, or `codex`) for `pm day summarize`, with no fallback; `pm doctor` fails only when that runtime's CLI is missing.
- A Codex session is told when Codex skips pm's hooks: a pm command warns once when none ran, and the session-start state names the hooks Codex skips when some did (as after a `pm upgrade` rewrote a trusted entry).
- One judge prompt for both runtimes, with two wording fixes; the live eval and the parity eval cover both runtimes.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| `make test-live` passes every labelled case on both backends, `PM_LIVE_RUNS` times each, latency per backend | met | 63 cases x 3 runs, 8 at a time: Claude 189/189, median 2.01 s, p90 3.59 s, max 11.64 s; Codex 189/189, median 5.48 s, p90 14.10 s, max 19.69 s (Findings) |
| The parity eval gains a Codex-without-`claude` chat-ask scenario that the judge blocks | met | `make test-live-parity` (`danger-full-access`): 8 of 8 passed in 96.5 s, before the merge with PR #47's new scenarios; the new scenario 27.0 s, one Stop block quoting the ask, then a need raised |
| `pm day summarize` on codex writes a summary in a scratch clone | met | Scratch clone, no `claude` on PATH: `days/2026-10-12.summary.json` written in 6.5 s, model `gpt-5.6-luna`, no rollout left in `CODEX_HOME` |
| `pm doctor` names the configured runtime's missing binary | met | `test_doctor_and_init_name_the_pm_hooks_codex_skips`: with neither CLI, it fails naming `claude` (the default); with `summary_runtime = "codex"` it passes with only `codex`, and fails naming `codex` without it |
| Every design page this sprint changes is brought to the new state and listed | met | Design pages above: Owner-request Stop hook, Views and the site, pm in Codex (gap 8 closed, gap 1 updated), pm as an installable product, pm CLI |
