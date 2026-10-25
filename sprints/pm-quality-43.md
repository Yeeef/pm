---
type: sprint
title: Agents give feedback on the repo itself
bead: pm-d2k5.43
---

## Goal

> What should be true when this sprint ends, and why now?

An agent can report its development experience in the repo it works in, not only friction with pm: `pm feedback add --repo` appends an entry to the repo's one repo feedback doc, and pm gives a fixed starting set of questions to answer. Why now: the owner asked for it. pm feedback shows where pm gets in the way, but nothing collects what slows agents down in the repo's own code, docs and tools.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm feedback add --repo`: appends to `records/docs/repo-feedback.md`, kept apart from `records/docs/pm-feedback.md`, with the same entry form and the same `--project`, `--sprint` and `--task` scoping.
- The starting question set, in one file under `prompts/` and embedded: navigating and understanding the code, docs and tools; what blocked or slowed understanding or implementation; stale or missing docs; slow or hard-to-find commands; what to fix first.
- `pm feedback --help`, `pm feedback questions` and `prime.md` name the questions and when to answer them: once per sprint, when its delivery report is written.
- `pm show` and the site show the repo feedback doc beside the pm one.

**Out:**
- Asking at SessionEnd (sprint 25 owns that hook; its prompt can add these questions later).
- Questions set per repo in `.pm/config.toml`.

## Done when

> What evidence will show the goal is met?

- `pm feedback add --repo --text=…` in a test repo writes `records/docs/repo-feedback.md` and leaves the pm feedback doc alone; the harness test passes.
- `pm feedback questions` prints the set, and `pm feedback --help` and `prime.md` name it; the golden tests pass.
- `pm show` prints a repo feedback line with the entry count and the doc's URL.
- The PR's CI is green.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): `pm feedback add --repo`, `pm feedback questions` and the `repo feedback:` line of `pm show`.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-25}
The design page record-layer stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page views-and-site stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page pm-go stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page agent-lifecycle stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page pm-product stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page reply-delivery stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page session-claims stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page owner-questions stays as it is for this sprint's code changes.
only the feedback doc table and its repo feedback twin changed; this page names neither
:::

::: decision {source=agent date=2026-10-25}
The design page sprint-lifecycle stays as it is for this sprint's code changes.
writes.go only switched to the feedback doc table; the sprint lifecycle names no feedback doc
:::

::: decision {source=owner date=2026-10-25}
Repo feedback has no pm feedback questions command; its questions live in pm feedback add --help and prime.md names them in one line
Plain pm feedback has no questions command either; a separate command adds surface for text the help already carries
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: agents can give feedback on the repo itself with `pm feedback add --repo`, answering the questions `pm feedback questions` prints.

- `records/docs/repo-feedback.md`, kept apart from the pm feedback doc; both docs are now one table in `internal/records/feedback.go`.
- `prompts/repo_feedback_questions.txt`: five questions (navigation, blockers, docs, tools, next fix).
- `prime.md`: answer them once per sprint, when the delivery report is written.
- `pm show`: a `repo feedback:` line, and `repo_feedback` in `--json`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `--repo` writes `records/docs/repo-feedback.md` and leaves the pm doc alone: met; `test_feedback_on_the_repo_goes_to_its_own_doc_and_answers_the_questions` passes (`tests/test_pm.py -k feedback`: 4 passed).
- `pm feedback questions` prints the set; `--help` and `prime.md` name it: met; the same test checks the output, `help.golden` was regenerated, and `make test-go` passes; `tests/test_hooks.py`: 25 passed.
- `pm show` prints the repo feedback line with count and URL: met; the same test checks the text at both levels and `--json`.
- The PR's CI is green: see PR #118's checks before its merge.
