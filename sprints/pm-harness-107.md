---
type: sprint
title: "Owner-request hook: autopilot and interactive modes"
bead: yeeef-agents-9va.117
---

## Goal

> What should be true when this sprint ends, and why now?

pm's rule on requests to the owner, as prime.md states it, is the rule the owner-request hook enforces, and the hook's reprompt tells the agent how to meet it: do the step itself first; raise a need, under a sprint or task it opens first when none holds the request, for what only the owner can do; ask with AskUserQuestion, never in plain text, when there is a clear reason not to raise a need. Why now: on 2026-10-10 the hook blocked three replies in a chat with the owner, because it judges every request while prime.md and the block text claim a "sprint work waits on it" condition that nothing checks.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- One rule in three texts: prime.md's request rules, the judge prompt and the reprompt; the "sprint work waits on it" condition goes from all three.
- The reprompt (owner_request_reason.txt): do it yourself first; a need under a sprint or task, opened first when none holds the request; AskUserQuestion over a plain-text question.
- The owner-request-hook design page brought to the new state; labelled cases in tests/owner_request_cases.json for the three blocked replies of 2026-10-10.

**Out:**
- A per-session interactive mode that turns the hook off (owner decision 2026-10-10: AskUserQuestion covers the live conversation).
- Codex's hook, beyond running the same judge and reprompt.

## Done when

> What evidence will show the goal is met?

- prime.md, the judge prompt and the reprompt state the same rule: a reviewer reading the three finds no case one allows and another blocks.
- In a live session, a reply that asks the owner for a file only in chat is blocked and the reprompt names doing it yourself first; after a need is raised for it, or the question moves to AskUserQuestion, the reply passes.
- make test-live passes with the new cases.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): one rule across prime.md, the judge and the reprompt; the judge without the sprint condition; the reprompt.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
The session mode is read from the transcript: the last owner prompt that is exactly pm interactive or pm autopilot sets it, autopilot when there is none; nothing is stored
only the owner can add a prompt entry, while an agent Bash call is indistinguishable from an owner command; it needs no new hook entry (Codex would need owner approval), no state to clean up and no work-store migration
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The hook never checks for sprint work: decide() in
  internal/hooks/ownerrequest.go reads the session's open needs, asks Haiku to
  label each sentence, and blocks every unmatched decision, review or action.
  The judge gets only the reply and the needs list, so the 'sprint work waits
  on it' condition in the prompt's first line and in the block text cannot be
  applied. On 2026-10-10 it blocked 3 of 4 replies in a chat Q&A with no
  sprint or claimed task.

- The hook contradicts its design page: owner-request-hook.md says
  conversational questions, clarifications and offers pass, and records that
  judging every request was too rigid in live conversation. The page also
  still describes the earlier one-question prompt hook that checks for a cited
  id, not today's per-sentence judge matched against open needs.

- AskUserQuestion is never judged: the hook reads last_assistant_message at
  Stop, and AskUserQuestion runs mid-turn, so its question is not in the final
  reply. In a session nobody watches it waits for an answer that never comes,
  and the question is not on the site, so it fits interactive mode only.

- No sprint was opened for the 2026-10-10 investigation of the hook's false
  alarm. The agent read 'every change has a task in a sprint' as covering only
  changes, so its findings lived only in chat and a feedback entry until this
  sprint.

- Live eval of the new judge prompt (make test-live, PM_LIVE_RUNS=3, Yeeef/pm
  branch owner-request-one-rule): 40 of 40 cases 3/3 on the first try, 120
  hook runs in 35.9 s at 8 in parallel, median 2.05 s, max 11.69 s per run.
  One case moved: a chat-only clarifying question now blocks. Added: the three
  replies blocked on 2026-10-10 (block) and two report-only replies (pass).

- Live session, headless claude -p with the branch's pm on PATH (session
  215d580a): a plain-text ask for the owner's laptop log file was blocked with
  the new reprompt (do it yourself first, open a sprint or task,
  AskUserQuestion); the agent raised [TEST] action 117.6 under the sprint and
  ended. The hook re-run on the same request with stop_hook_active false and
  the need open passed in 1.4 s, so the need, not stop_hook_active, let it
  through. Need dismissed afterwards.

- The live eval could not run on main: tests point HOME and CLAUDE_CONFIG_DIR
  at temp dirs, so every judge call failed with 'Not logged in'. Fixed on the
  branch by passing the real HOME and CLAUDE_CONFIG_DIR to the live test's
  judge only.

- Side effect of the live session check: running claude -p with the branch's
  pm first on PATH also ran its session-start pm init, which copied the dev
  build (stamped 0.3.0) into ~/.local/bin/pm at 04:28 local. Every pm hook on
  the machine ran the unreleased build for about 30 minutes, until the
  released 0.3.0 was reinstalled with its install.sh (checked against
  SHA256SUMS); pm doctor then reported the clone's setup matching. The pm
  service had started at 02:55 and kept the released binary. A live session
  check of an unreleased pm must not run session start against the real bin
  dir: give it PM_BIN_DIR or HOME in a scratch dir, as the dev guide asks for
  pm init checks.

- Judge accuracy at 10 runs per case after review fixes: two of the 2026-10-10
  cases blocked only 4/10 (offer conditioned on the owner pasting the reply)
  and 8/10 (cannot tell without the blocked text), though both had passed 3/3;
  the other 38 were 10/10. Adding to the action kind that a condition or
  statement making the next step depend on the owner is an action took all 40
  cases to 10/10 (400 calls, 105 s). 3 runs per case does not detect a 40%
  miss rate.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: prime.md, the owner-request judge prompt and its reprompt state one four-item rule (do it yourself first; a need under a task or sprint, opened first; AskUserQuestion over a plain-text question; no leave asked for authorized steps), shipped in Yeeef/pm PR #7.

Merged as c44a0ff (PR #7).

- prime.md (both copies): the rule in one invariant line; the reprompt carries the detail, which the agent gets when the hook blocks.
- Judge: no sprint-work condition; a clarification blocks like a decision, review or action; a condition or statement that makes the next step depend on the owner is an action.
- Reprompt: do it yourself first, open a sprint or task, AskUserQuestion; the needless-ask text lists committing.
- Live eval runs again (it could not log in on main); cases: one relabelled, five added.
- Dropped: the interactive mode (owner decision 2026-10-10) and needs under a project.
- Takes effect in a repo once a pm release holds it and the repo's pin moves to it; neither is part of this sprint.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| prime.md, the judge prompt and the reprompt state the same rule | met | fresh-context review of PR #7 read the three side by side: no case one allows and another blocks; the two minor gaps it found (committing missing from the needless text, an ambiguous last line) fixed in c2a6e18 |
| A live session's chat-only ask for a file is blocked with the do-it-yourself reprompt, and passes once a need is raised or the question moves to AskUserQuestion | met, AskUserQuestion part by eval only | headless session 215d580a with the branch's pm: blocked with the new reprompt, raised [TEST] action 117.6, ended; the hook re-run on the same request with the need open and stop_hook_active false passed in 1.4 s. A headless session has no AskUserQuestion; the eval case for a question put through it passes 10/10 |
| make test-live passes with the new cases | met | PM_LIVE_RUNS=10: 40 of 40 cases 10/10, 400 judge calls in 105 s. CI on PR #7: light 22 s, integration 38 s, build-and-parity linux 11m29s and macOS 14m45s, all pass; merged as c44a0ff, whose CI on main passed all four checks |
