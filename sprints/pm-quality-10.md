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

- With option cited (a draft the owner later replaced with mine): make test-live, PM_LIVE_RUNS=3, 52
  cases (40 existing + 12 new): every case 3/3 in two consecutive full runs;
  156 hook runs in 51.9 s and 54.3 s, per run median 2.29 s and 2.28 s, max
  11.31 s and 14.42 s. A third run in between errored on one claude -p call
  that hit the hook's 15 s judge timeout (no verdict mismatch); the max
  latency now runs close to that timeout while five sprints share the machine.

- prime.md's request rule gained one clause (if a need holds the request,
  do not ask again); the first rules chunk is now 9,947 of the
  10,000-character hook cap. A longer clause put it at 10,154 and
  failed TestChunksFitTheCapAndAddUpToTheHead, so the next prime.md addition
  before '# How' needs a new heading in hooks.Starts.

- pm hook stop on this coordinator session's real transcript (2026-10-10, 9
  Agent calls naming records/sprints/pm-quality-N.md): Touched now gives
  pm-quality-1 to -6 (one named by the session's own Edit, the rest by
  subagents whose task notification arrived) and leaves out pm-quality-8, -10
  and -11, named only by subagents still running; the old scan named all nine.
  A subagent that reported an interim result (it may resume) counts as
  returned.

- Final state (owner's choice mine on pm-d2k5.10.5, commit 973f632): make
  test-live, PM_LIVE_RUNS=3, 55 cases (40 existing + 15 new): every case 3/3 in
  two full runs; 165 hook runs in 51.6 s and 47.7 s, per run median 1.90 s and
  1.91 s, max 11.73 s and 7.31 s. A run in between errored once: the judge
  answered out of shape ({"items": [{…}, {"items": []}]}), which the hook
  refuses (exit 1, check not run); 1 call in 165, no verdict mismatch.

- The judge read any sentence that names a PR's review or merge as a future
  event as a request to the owner (five replies of the coordinator, which holds
  the merge by owner delegation). From the reply alone the judge cannot tell who
  merges; the prompt now reads a sentence as a review only when it asks the
  owner or names them as the one to act. A PR said to wait on "your review and
  merge" still blocks (contrast case).

- Fresh-context review of PR #25: the judge cannot see who merges, so clauses
  that passed the coordinator's status lines passed real requests too
  ('**Merge #20 first.**', 'PR #12 is green and waits on review and merge').
  Rule adopted: a PR review or merge pending with no actor named is a request
  to the owner; naming the agent or its subagents as the actor is a plan; a
  request cited by id as open on the site is a report, whichever session holds
  it. The five coordinator lines are labelled block as written, each paired
  with a reworded pass form. make test-live, PM_LIVE_RUNS=3: all 63 cases 3/3
  in three full runs (189 hook runs each; per run median 2.09 s, 2.05 s, 2.09
  s; max 9.80 s, 9.32 s, 7.83 s).

- main at 4c36803 was red: its first rules chunk was 10,018 characters (over the 10,000 cap) and [Unreleased] held two '### Added'. #22 (a274b65) trimmed prime.md and merged the Added lists; PR #25 cuts the rules into three chunks at '## 3. Records' (3,647, 6,417 and 6,329 characters on a274b65 with this sprint's prime.md line; the hook entries follow len(hooks.Starts), golden pieces updated), since the two-chunk cut left 368 characters of room.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the owner-request judge gives every labelled case its verdict 3/3, including the reported non-requests and all 40 existing cases, and `pm hook stop` leaves out a record only a running subagent's prompt names; PR #25, merged as 2911a04.

- 23 new cases in `tests/owner_request_cases.json` (40 to 63): one per reported sentence (7), the five status lines from the coordinator's session as written (block) each with a form that names the agent as the actor (pass), and six contrast cases.
- One rule in three texts (judge prompt, block reason, `prime.md`): a PR review or merge pending with no actor named asks the owner; naming the agent or its subagents is a plan; a request cited by its id as open on the site is a report, whichever session holds it; asking again for an open request blocks (owner: option mine), and the block says to drop the ask or name the request's id, never to raise a duplicate.
- `pm hook stop` leaves out a path named only by the prompt of a subagent call that has not returned.
- The rules run as three chunks (cut at "## 3. Records": 3,647, 6,417 and 6,568 characters on main d0e08f3), leaving room under the hook cap.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- At least 7 new cases, one per reported sentence: met. 23 new cases (40 to 63), each named after its feedback entry or review finding; the 2026-10-08 21:47 entry holds two sentences, so two cases. The formal-methods 2026-10-10 03:31 summary, which tells the owner to do another session's needs, is labelled block (option mine); a reply that reports them as open by id is labelled pass.
- `make test-live` with `PM_LIVE_RUNS=3` passes every case 3/3: met. All 63 cases 3/3 in three full runs (189 hook runs each; per run median 2.09 s, 2.05 s and 2.09 s; max 9.80 s, 9.32 s and 7.83 s). Baseline before the change: 6 of the first 9 new cases failed.
- A harness test of `pm hook stop` shows no block for a path named only by a running subagent call and a block for the same path edited by the session's own call: met. `test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names` (running, returned, own edit, both); its running case fails on the old code. Go: `TestTouchedCountsASubagentsPathOnceItsCallReturns`.
- `make test` and the PR's CI pass: `make test` passes on head 9e894a8 (rebased on main d0e08f3); CI on that head passes every check (light, integration, changelog, Go build-vet-test, work and race on linux-amd64 and darwin-arm64), the slowest work (darwin-arm64) at 3m28s.
