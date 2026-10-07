"""`pm hook owner-request`, the Stop hook wired in Claude Code and Codex: an agent may not end its turn asking the
owner for something sprint work waits on only in chat. Every such request in the final reply must match an open need
or action (a Beads issue labelled `human`) that this session raised; the reply needs no id.

It reads this session's open requests from Beads (`pm decision need` and `pm action need` store the raising session
as metadata `session`; other sessions' requests do not count), then asks Claude Haiku through `claude -p` to list
the owner requests in the reply and the open request each one matches. A request that matches none blocks the stop
once, with a reason naming it and the `pm` command that raises it. Asking leave for, or offering, a step agents are
authorized to take without asking (working the sprint, pushing its branch, opening its PR) blocks even when an open
request matches, with a reason telling the agent to take the step.

Reads the hook input JSON on stdin (`session_id`, `stop_hook_active`, `last_assistant_message`); prints
{"decision": "block", "reason": ...} to block, nothing to let the stop through. On `stop_hook_active` (the agent
already continued once for a Stop hook) it lets the stop through without reading anything, so it never loops. It fails
hard, never silently: when the input is unusable, or `bd` or `claude` fails, hangs or answers out of shape, it exits 1
with the cause on stderr, which both runtimes show without blocking the stop. This is the one pm hook that does not
fail open: a check that did not run is shown, never hidden. Standard library only, so importing it stays cheap."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile

MODEL = "claude-haiku-4-5-20251001"
BD_TIMEOUT = 10  # seconds; `bd list` takes about 0.6 s
JUDGE_TIMEOUT = 15  # seconds; the judge takes about 2 s. With BD_TIMEOUT it stays under the 30 s hook timeout
# Extended thinking off: with it, `claude -p` Haiku took 5 to 36 s per verdict instead of about 2 s. No
# nonessential traffic: it also skips a second side request.
JUDGE_ENV = {"MAX_THINKING_TOKENS": "0", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
# No settings (so none of this repo's hooks run in the child), no MCP servers, no tools, no saved session.
JUDGE_ARGS = ["claude", "-p", "--model", MODEL, "--setting-sources", "", "--strict-mcp-config", "--tools", "",
              "--no-session-persistence", "--output-format", "text"]

SYSTEM = """\
You check the final reply an AI coding agent writes to its owner at the end of a turn. Rule: when the reply asks \
the owner for something that planned work waits on, that request must already be raised as an open request on the \
owner's site, because a request asked only in chat never reaches the owner.

Step 1. List every sentence of the reply that addresses the owner with a question, an instruction, or a statement \
that something waits on the owner. Include every question mark sentence aimed at the owner.

Step 2. Give each one a kind:
- "decision": asks the owner to decide or choose something about the work: its scope or design, an approach or \
option, whether, when or how to do, ship, merge or drop something, or leave to go ahead. "Should I merge the PR now \
or wait for review?", "Option A or B?", "Which approach do you prefer?" and "OK to \
proceed?" are decisions. A question that lays out options for the work and asks the owner to pick is always \
a decision, however politely it is put.
- "review": asks the owner to review, approve or merge a PR or change.
- "action": asks the owner to do something only they can: run a command, apply or change a setting, provide a key, \
access or file, restart or install something.
- "clarification": asks what the owner just said or meant ("By 'the hook', do you mean the Stop hook or the \
SessionStart hook?").
- "offer": an optional offer the owner may take or leave, that nothing waits on: extra work the task or sprint is \
complete without ("If you want a live check too, tell me", "I can also add a test if you'd like").
- "suggestion": a possible next step that nothing waits on.
- "authorized": asks leave for, or offers, a step the agent is authorized to take without asking: doing the \
sprint's or task's own work, committing, pushing a branch, opening a PR. Asking about it is needless; the agent \
should just do it. Merging a PR, or anything else the owner reviews, is never authorized: a sentence that asks \
or offers to merge, or asks the owner to review or merge, is "decision" or "review", not "authorized".
An offer or suggestion is judged by its subject, not its wording. When it offers to do something the task or \
sprint cannot finish or close without, it is a request, however politely or optionally it is put ("if you want", \
"I can", "happy to"): "authorized" when the step is one of the authorized ones above; else "decision" (merging \
or shipping a PR, a step the close requires, picking one of several designs), or "review" for a PR review.
- "not asked": the sentence quotes, lists or describes a request, question or example without asking it of the \
owner now (test cases, examples, what a need asks, "I raised a need on X"), it is text inside a code block, \
command output or a log, or it is the agent's own plan ("I'll check its work before I tell you it's done").

Step 3. For each decision, review or action, find the OPEN REQUESTS entry that asks the owner for the same thing \
(the same decision, the same PR, the same action), even in other words. An open review of a PR covers every \
request about reviewing, approving or merging that PR, including whether or when to merge it; when only one PR \
review is open, a request about "the PR" means that PR. An id written in the reply is not a match \
by itself: only an entry in the list can match. When the list is "(none)", nothing matches.

Answer with one JSON object and nothing else:
{"items": [{"quote": "<the sentence, at most 150 characters>", "kind": "<kind>", "match": "<id of the matching \
open request, or null>"}]}
Answer {"items": []} when no sentence addresses the owner."""

ASKS = {"decision", "review", "action"}  # kinds that block unless an open request matches
AUTHORIZED = "authorized"  # a needless ask: blocks even when an open request matches

REASON = (
    "Your reply asks the owner for something sprint work waits on that no open request of this session covers: "
    "{asks}. Chat requests never reach the owner's site, so the owner may never see them. Raise each one, then end "
    "your reply (it needs no id):\n"
    "- a decision: bin/pm decision need --title \"...\" --parent <sprint or task id>, stdin with one part per line: "
    "Question:, Fact:, Option <label>: each with a Cost: line, and Default: <label>;\n"
    "- an action (run, apply, configure, ...): bin/pm action need --title \"...\" --parent <id>, what to do and why "
    "on stdin;\n"
    "- a PR review or merge: bin/pm action need --pr URL --sprint ID --focus \"...\".\n"
    "If no sprint work waits on it, end without the request or make it a plain offer."
)


NEEDLESS = (
    "Your reply asks the owner's leave for, or offers, a step you are authorized to take without asking: {asks}. "
    "The owner authorizes working a sprint's tasks, pushing its branch and opening its PR for every sprint. Do the "
    "step now instead of asking; only the PR review and merge wait on the owner (bin/pm action need --pr URL "
    "--sprint ID --focus \"...\")."
)


class HookError(Exception):
    """The check could not run; hook_owner_request reports it on stderr and exits 1."""


def open_requests(session: str, cwd: str | None) -> list[dict]:
    """This session's open needs and actions: issues labelled `human`, not closed, whose metadata `session` is
    `session`."""
    cmd = ["bd", "list", "--label", "human", "--metadata-field", f"session={session}", "--limit", "0", "--json"]
    try:
        res = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=BD_TIMEOUT)
    except FileNotFoundError:
        raise HookError("bd is not installed or not on PATH")
    except subprocess.TimeoutExpired:
        raise HookError(f"bd list timed out after {BD_TIMEOUT}s")
    if res.returncode != 0:
        raise HookError(f"bd list failed (exit {res.returncode}): {' '.join(res.stderr.split())[:300]}")
    try:
        return json.loads(res.stdout or "[]")  # bd lists closed issues only with --all
    except ValueError:
        raise HookError(f"bd list printed no JSON: {res.stdout[:200]!r}")


def judge_prompt(reply: str, requests: list[dict]) -> str:
    listed = "\n".join(f"- {r['id']}: {r.get('title', '')}" + (f"\n  {' '.join(r['description'].split())[:400]}"
                                                                 if r.get("description") else "")
                       for r in requests) or "(none)"
    return f"OPEN REQUESTS:\n{listed}\n\nREPLY:\n<<<\n{reply}\n>>>"


def judge(reply: str, requests: list[dict]) -> list[dict]:
    """The sentences of `reply` that address the owner, each {"quote": str, "kind": str, "match": id or None}, as
    the model sees them."""
    with tempfile.TemporaryDirectory() as cwd:  # no project CLAUDE.md or settings apply
        try:
            res = subprocess.run([*JUDGE_ARGS, "--system-prompt", SYSTEM], input=judge_prompt(reply, requests),
                                 capture_output=True, text=True, cwd=cwd, timeout=JUDGE_TIMEOUT,
                                 env={**os.environ, **JUDGE_ENV})
        except FileNotFoundError:
            raise HookError("claude is not installed or not on PATH")
        except subprocess.TimeoutExpired:
            raise HookError(f"claude -p timed out after {JUDGE_TIMEOUT}s")
    if res.returncode != 0:
        said = " ".join((res.stderr or res.stdout).split())[:300] or "no output"
        raise HookError(f"claude -p failed (exit {res.returncode}): {said}")
    match = re.search(r"\{.*\}", res.stdout, re.S)
    try:
        found = json.loads(match.group(0))["items"] if match else None
    except (ValueError, KeyError, TypeError):
        found = None
    if not isinstance(found, list) or not all(isinstance(r, dict) and isinstance(r.get("quote"), str)
                                              and isinstance(r.get("kind"), str)
                                              and isinstance(r.get("match"), (str, type(None))) for r in found):
        raise HookError(f"claude -p answered out of shape: {res.stdout[:300]!r}")
    return found


def decide(event: dict) -> str | None:
    """The block reason for this stop, or None to let it through. Raises HookError when the check cannot run."""
    if event.get("stop_hook_active"):
        return None
    session = event.get("session_id")
    if not isinstance(session, str) or not session:
        raise HookError("the hook input has no session_id")
    reply = event.get("last_assistant_message")
    if not isinstance(reply, str):
        raise HookError("the hook input has no last_assistant_message")
    if not reply.strip():
        return None
    requests = open_requests(session, event.get("cwd"))
    ids = {r["id"] for r in requests}
    items = judge(reply, requests)
    quoted = lambda found: "; ".join(f'"{q[:150]}"' for q in found)[:600]
    needless = [r["quote"] for r in items if r["kind"] == AUTHORIZED]
    unmatched = [r["quote"] for r in items if r["kind"] in ASKS and r.get("match") not in ids]
    reasons = ([NEEDLESS.format(asks=quoted(needless))] if needless else []) + \
              ([REASON.format(asks=quoted(unmatched))] if unmatched else [])
    return "\n\n".join(reasons) or None


def hook_owner_request() -> int:
    """`pm hook owner-request`: the hook input JSON on stdin; a block verdict on stdout, or nothing."""
    try:
        event = json.loads(sys.stdin.read() or "{}")
        if not isinstance(event, dict):
            raise ValueError
    except ValueError:
        print("pm hook owner-request: the hook input is not a JSON object; the owner-request check did not run",
              file=sys.stderr)
        return 1
    try:
        reason = decide(event)
    except HookError as e:
        print(f"pm hook owner-request: {e}; the owner-request check did not run", file=sys.stderr)
        return 1
    if reason:
        print(json.dumps({"decision": "block", "reason": reason}))
    return 0
