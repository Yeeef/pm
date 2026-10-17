---
type: sprint
title: "One CLI grammar: scope flags, need close verb, and a naming check"
bead: pm-d2k5.39
---

## Goal

> What should be true when this sprint ends, and why now?

Wave 2 of the [command audit](../docs/2026-10-16-pm-command-quality-audit.md). A scope is still named five ways (`--parent`, `--sprint`, `--project`, `--level` with `--project/--sprint`, the hidden `task add --parent`) and a need closes four ways (`decision add --need`, `decision close`, `action done`, `need dismiss`). When this sprint ends, every command follows the naming grammar on the [pm-cli](../design/pm-cli.md) design page, and a test refuses a new command that does not, so the spread cannot grow back one command at a time.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Owner decisions first: the renames reopen pm-quality 21 (the CLI grammar stays) and pm-harness 13 (close verbs sit under their noun), so each is raised as a decision need before any code changes.
- Proposal #5: one scope vocabulary, `--sprint`, `--task`, `--project`, on every command. `decision need` and `action need` drop `--parent`; `decision add` drops `--level`, which `--project` or `--sprint` already implies.
- Proposal #8: one verb, `need close <id> --reason`, closes a need that sets no rule, in place of `decision close`, `action done` and `need dismiss`. `decision add --need` stays the path that records a rule.
- Proposal #10: a Go test over `commands.go` that refuses a scope flag, create verb or positional name outside the grammar table.
- Old flags and verbs are removed, not aliased; their refusal names the new form. prime.md, `--help`, goldens, `test_pm.py`, the hook and prompt texts that name them, and a changelog entry with the migration.

**Out:**
- Merging nouns or verbs beyond the scope and need-close changes.
- The record format and the site's need cards. The site keeps its cards; only the CLI verbs change.

## Done when

> What evidence will show the goal is met?

- Each owner decision need is answered and cited by a decision in this sprint; a change the owner declines leaves the scope, recorded as a scope decision.
- `pm decision need --sprint ID …` and `pm action need --task ID …` raise needs; `--parent` and `decision add --level` are refused, and each refusal names the new flag (harness tests).
- `pm need close <id> --reason "…"` closes a decision, action or question need that sets no rule; the three old verbs are refused, naming it (harness tests).
- The grammar test fails on a deliberately added off-grammar flag and passes on main (shown in the PR).
- No prime.md, `--help`, hook or prompt text names a removed form (`test_every_flag_prime_md_and_the_help_texts_name_parses` passes); the PR's CI is green.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): Naming, with the grammar test; Owner requests; Open questions
- [Agent lifecycle](../design/agent-lifecycle.md): decision scope flags; needs close with pm need close
- [Decision need layout](../design/decision-need-layout.md): decision need --sprint
- [Owner questions](../design/owner-questions.md): a question closes with pm need close
- [Owner-request hook](../design/owner-request-hook.md): a need's --sprint or --task
- [Record layer](../design/record-layer.md): decision add --project or --sprint
- [Reply delivery](../design/reply-delivery.md): pm need close in the next lines and refusals
- [Site replies](../design/site-replies.md): pm need close
- [Sprint lifecycle](../design/sprint-lifecycle.md): a review closes with pm need close
- [Work layer and bd](../design/work-layer-bd-boundary.md): need close
- [Work layer](../design/work-layer.md): need commands
- [Work store](../design/work-store.md): task add --task, pm need close --dropped

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
The design page pm-go stays as it is for this sprint's code changes.
Only a code comment in internal/work/dolt.go and a test's command form changed; the Go port's design is unchanged.
:::

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
