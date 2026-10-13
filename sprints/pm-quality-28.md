---
type: sprint
title: A sprint's close checks only the covers patterns it added, not other sessions' pending ones
bead: pm-d2k5.28
---

## Goal

> What should be true when this sprint ends, and why now?

A sprint closes on the strength of its own work. Since pm 0.8.5, `pm sprint close` refuses while any covers pattern on a page the sprint holds matches nothing on the remote's main. Pages are shared, so a sprint that touches pm-cli or sprint-lifecycle waits on another session's unmerged PR (the CLI split, #62) that it has nothing to do with. Sprint 26, which introduced the rule, would itself be refused. The rule must hold a sprint only to the patterns it added or changed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint close` checks, on each page the sprint holds, only the covers patterns its records commits added or changed since the sprint opened (the page's covers at the sprint's start compared with now).
- A pattern another session added and left pending does not hold this sprint.
- Tests; `--help`; the design pages of the areas changed.

**Out:**
- `pm check`'s report of pending patterns, which stays as it is.

## Done when

> What evidence will show the goal is met?

- A harness test: a page holds a pending pattern another sprint added, and a sprint that touches the page's code closes; the same sprint is refused when it added that pattern itself.
- In this repo, a sprint touching pm-cli's code can close while #62's patterns are pending.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): Covers hold on main at the close, which patterns are the sprint's, and the alternatives.
- [Record layer](../design/record-layer.md): what `pm sprint close` holds that `pm check` lists as waiting.
- [pm CLI](../design/pm-cli.md): `pm sprint close`'s refusals.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
A covers pattern is the sprint's when the page did not hold it at the last records commit before the sprint opened: added or edited patterns, and every pattern of a page made during the sprint; a pattern another session adds while the sprint is open counts as the sprint's.
Exact and needs no new input: records commits name no sprint or session, a subagent shares its parent session (one session runs several sprints), and the owner commits with none, so per-commit attribution would need a required sprint on every pm commit; the window rule clears the case that occurs (patterns from before the sprint, as the CLI split's) and fails closed on concurrent edits to one page, whose refusal names the pattern.
:::

::: decision {source=agent date=2026-10-13}
The design page session-claims stays as it is for this sprint's code changes.
writes.go's change is in pm sprint close's covers check; task claims and holders are unchanged.
:::

::: decision {source=agent date=2026-10-13}
The design page agent-lifecycle stays as it is for this sprint's code changes.
prime.md's sprint close line narrows one refusal; the session lifecycle is unchanged.
:::

::: decision {source=agent date=2026-10-13}
The design page pm-product stays as it is for this sprint's code changes.
prime.md's sprint close line narrows one refusal; packaging, config keys and install are unchanged.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm sprint close` holds a sprint only to the covers patterns it added or changed since it opened, so another session's pending pattern from before then no longer blocks it.

- A pattern is the sprint's when the page did not hold it at the last records commit before the sprint opened; a page made during the sprint is the sprint's whole; with no pattern of the sprint's, nothing is fetched.
- Accepted limit (sprint decision): a pattern another session adds to the page while the sprint is open counts as the sprint's; the refusal says the PR may be another sprint's.
- `--help`, `prime.md`'s close line, a changelog entry, and [Sprint lifecycle](../design/sprint-lifecycle.md), [Record layer](../design/record-layer.md) and [pm CLI](../design/pm-cli.md) at the new state.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A page holds a pending pattern another sprint added, and a sprint touching the page's code closes; refused when it added the pattern itself: met, `test_a_sprint_closes_past_a_pending_pattern_another_sprint_added_but_not_past_its_own` (refused naming only `internal/cli/own.go`, not the pending `internal/cli/sprint.go`; closes once its own pattern is gone), locally and in CI on PR #73.
- In this repo, a sprint touching pm-cli's code closes while #62's patterns are pending: met by that test, which reproduces the pm CLI page's case, and live by this sprint's own close (its PR changes `internal/cli/writes.go`, which pm CLI covers, while the page holds #62's 13 pending patterns), run on the released pin.
- Every existing design page whose area changed is at the new state and in Design pages: met for Sprint lifecycle, Record layer and pm CLI; Session claims, Agent lifecycle and pm as an installable product, which the gate names for PR #73, are waived with reasons.
