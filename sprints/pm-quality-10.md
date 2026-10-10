---
type: sprint
title: The Stop hooks block only what is the agent's to fix
bead: pm-d2k5.10
---

## Goal

> What should be true when this sprint ends, and why now?

The owner-request judge passes the non-requests agents reported, and does so on labelled cases that `make test-live` measures. `pm hook stop` no longer blocks a parent over a record that its running subagent is writing.

None of the reported sentences is in `tests/owner_request_cases.json` (40 cases), so the current judge's verdict on them is unmeasured:
- a stated plan (pm 2026-10-07 03:43);
- how-to instructions that answer the owner's own question (pm 2026-10-08 00:01);
- a "still waiting on" status line about the agent's own subagent (pm 2026-10-08 02:45);
- conditional facts (pm 2026-10-08 21:47);
- a line reporting a need held in another clone (pm 2026-10-10 03:56);
- a summary restating open needs raised by an earlier session (formal-methods 2026-10-10 03:31).

The stop-hook case is formal-methods 2026-10-07 20:45.

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Each reported sentence becomes a labelled case, its name citing the entry. Its label comes from the request rule in prime.md, never from the judge's answer.
- Where the rule does not settle a label, raise a decision need before labelling. Two cases need one: instructions that answer the owner's own question, and a restated need that is not in the judge's list.
- Decide whether the judge's OPEN REQUESTS list also carries the clone's other open needs, marked as another session's, so a restatement can be checked against them. Today the list holds only this session's needs (`internal/cli/ownerrequest.go`).
- Change the prompt and the block texts until every case passes. One rule lives in three texts, so change them in one commit.
- `pm hook stop` leaves out a path named only in the prompt of a subagent call that has not returned.

**Out:**
- Reading needs from other clones on the machine.
- Codex (pm-codex sprint 1).
- Checking AskUserQuestion calls.

## Done when

> What evidence will show the goal is met?

- `tests/owner_request_cases.json` holds at least 7 new cases, one per reported sentence.
- `make test-live` with `PM_LIVE_RUNS=3` passes every case 3/3, the new cases and all 40 existing ones, the chat-request block cases included. Pass counts and latency go in Findings.
- A harness test of `pm hook stop` shows no block for a path named only by a subagent call that has not returned, and a block for the same path edited by the session's own tool call.
- `make test` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-10}
The owner-request hook passes instructions that answer a question the owner asked, when no step of the agent waits on them.
The owner asked and nothing waits on the step, so the reply asks nothing; owner chose option pass.
Answers `pm-d2k5.10.4`.
:::

::: decision {source=owner date=2026-10-10}
The owner-request judge lists every open need of the clone, each marked; a request matching a need of another session passes only when the reply names its id.
It passes a summary restating open needs while an uncited restatement still blocks; owner chose option cited.
Answers `pm-d2k5.10.5`.
:::

::: decision {source=owner date=2026-10-10}
The owner-request judge lists only the open needs this session raised (option mine); a reply must not ask again for, or raise a duplicate of, a request another session holds: it drops the ask or reports the need as open. This supersedes the site reply cited on pm-d2k5.10.5.
Owner in chat, 2026-10-10: why make it more complicated; that agent should not raise the same request again.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline, before any prompt change: the 0.4.0 judge on 49 cases (40
  existing + 9 new reported ones), make test-live with PM_LIVE_RUNS=3: 43 pass
  3/3; 6 new cases fail: how-to answer 0/3, fix takes effect once installed
  0/3, review need held in another clone 0/3, formal-methods restated needs
  0/3, review-then-merge status 0/3, uncited restatement (block) 1/3. 147 hook
  runs in 47.9 s, per run median 2.07 s, max 11.13 s.

- After the change (commit 8c279aa): make test-live, PM_LIVE_RUNS=3, 52
  cases (40 existing + 12 new): every case 3/3 in two consecutive full runs;
  156 hook runs in 51.9 s and 54.3 s, per run median 2.29 s and 2.28 s, max
  11.31 s and 14.42 s. A third run in between errored on one claude -p call
  that hit the hook's 15 s judge timeout (no verdict mismatch); the max
  latency now runs close to that timeout while five sprints share the machine.

- prime.md's request rule gained one clause (another session's need
  holds a request if the reply names its id); the first rules chunk is now
  9,947 of the 10,000-character hook cap. A longer clause put it at 10,154 and
  failed TestChunksFitTheCapAndAddUpToTheHead, so the next prime.md addition
  before '# How' needs a new heading in hooks.Starts.

- pm hook stop on this coordinator session's real transcript (2026-10-10, 9
  Agent calls naming records/sprints/pm-quality-N.md): Touched now gives
  pm-quality-1 to -6 (one named by the session's own Edit, the rest by
  subagents whose task notification arrived) and leaves out pm-quality-8, -10
  and -11, named only by subagents still running; the old scan named all nine.
  A subagent that reported an interim result (it may resume) counts as
  returned.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the owner-request judge passes every reported non-request on 12 new labelled cases and keeps all 40 existing verdicts, and `pm hook stop` leaves out a record only a running subagent's prompt names; PR #25, pending merge.

- 12 new cases in `tests/owner_request_cases.json`: one per reported sentence (7), three from the coordinator's session of 2026-10-10, and two contrast cases that must still block.
- One rule in three texts, changed in one commit: the judge prompt, the block reason and `prime.md`'s request rule.
- The judge sees every open need of the clone, another session's marked; a match on another session's need counts only when the reply names its id (default of decision need pm-d2k5.10.5, still open).
- `pm hook stop` leaves out a path named only by the prompt of a subagent call that has not returned.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- At least 7 new cases, one per reported sentence: met. 12 new cases (40 to 52), each named after its feedback entry; the 2026-10-08 21:47 entry holds two sentences, so two cases.
- `make test-live` with `PM_LIVE_RUNS=3` passes every case 3/3: met. All 52 cases 3/3 in two consecutive full runs (156 hook runs each; per run median 2.29 s and 2.28 s, max 11.31 s and 14.42 s). Baseline before the change: 6 of the first 9 new cases failed. One run in between errored on a `claude -p` call that hit the 15 s judge timeout, not on a verdict.
- A harness test of `pm hook stop` shows no block for a path named only by a running subagent call and a block for the same path edited by the session's own call: met. `test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names` (4 parametrizations: running, returned, own edit, both); its running case fails on the old code. Go: `TestTouchedCountsASubagentsPathOnceItsCallReturns`.
- `make test` and the PR's CI pass: `make test` passes (118 passed, 52 skipped); CI on PR #25: see the coordinator's merge check.
