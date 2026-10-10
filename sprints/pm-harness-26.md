---
type: sprint
title: Owner-request hook accepts short ids
bead: yeeef-agents-9va.31
---

## Goal

> What should be true when this sprint ends, and why now?

When an agent ends its turn by asking the owner for something sprint work
waits on, the owner-request Stop hook checks that the reply cites the
request's id, so the request is on the site and not only in chat. Agents
usually cite the short id that `pm show` prints; the hook must accept it.

Example. A request has the full id `yeeef-agents-9va.29.4` ("Review PR #30");
`pm show` prints it as `9va.29.4`. An agent ends its turn with "PR #30 is up for review: `9va.29.4`."

- Should pass: the reply cites the review that carries the request, in
  short form.
- On 2026-10-05 it was blocked: the hook's prompt described an id only as
  `yeeef-agents-9va.29.4`, so it did not see `9va.29.4` as a citation.
- Must still be blocked: "PR #30 is up; please merge it." with no id at all,
  since then the request lives only in chat.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the Haiku prompt in `.claude/settings.json` and the Codex pattern script `owner_request_hook.py`: treat a short id (one or more dot-separated parts after a short hash, as `pm show` prints them) as a citation.

**Out:** changing what counts as a sprint request.

## Done when

> What evidence will show the goal is met?

- A reply that cites a request only by its short id passes in Claude Code (live `claude --bg` check) and in the Codex script (test).
- A reply that asks for a merge with no id at all is still blocked.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
Keep the Haiku owner-request prompt unchanged unless a live check with a matching short id fails: first re-run it with a reply that asks for a request and cites that request's own short id.
The earlier block came from a mismatched id (a merge request citing an unrelated decision), so it did not show the prompt rejects short ids (owner chose A).
Answers `yeeef-agents-9va.31.3`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check 2026-10-05 with matching ids: a reply asking for decision
  9va.15.3 and citing exactly 9va.15.3 was blocked by the Haiku Stop hook
  (session 0e02bae0); a merge request with no id was correctly blocked
  (session aa4cd715). The prompt needs the short-id edit (action 9va.31.4).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: a reply that cites its request by short id (`9va.15.3`) passes the
owner-request hook in Claude Code and Codex, a request with no id is still
blocked, and sprint and project closes ignore other sessions' uncommitted
records.

- Haiku prompt: "the reply cites an id" is now its own early step (5bd4de4).
- Codex script: short ids count as citations (f12b0ba).
- `pm sprint close` and `pm project close` check only their own record
  (df91ec1).
- `make test-live` runs the real prompt through Haiku on seven fixed replies;
  all passed 3 of 3 in two runs (e767520).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A reply citing a request only by its short id passes: met. Claude Code
  live check from the branch: a reply asking for decision 9va.15.3 and
  citing it passed (session 0319ad94); the Codex script, by test
  (`test_passes_a_request_citing_an_open_need_by_short_id`).
- A merge request with no id is still blocked: met (session 3e5c9bc3;
  `test_blocks_a_merge_request_citing_no_id`).
