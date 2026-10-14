"""Live parity eval: a real Claude Code session and a real Codex session get the same pm guarantees, and work one project
together. Each scenario runs headless sessions in a scratch clone that `pm init` set up (agents.Clone) and asserts from
the work store and the records store, not from the agent's prose; the hooks' evidence comes from each runtime's own
transcript (Claude Code's session JSONL, Codex's rollout).

- solo, per runtime, identical assertions: session-start context; claim, finding, a decision need answered by the
  owner (this test, through `pm reply add`) while the session waits; the decision citing it; a doc; the task closed.
- next prompt, per runtime: a session raises a need and ends its turn; the owner answers; the session, resumed on a
  prompt that does not name the need, gets the reply from the UserPromptSubmit hook and records the decision.
- wake, per runtime: an interactive session (the runtime's TUI, in a pty) raises a need and ends its turn; once it
  is idle the owner replies, and with nothing typed the session starts a turn on the reply the pm service delivered
  (Claude Code: its socket; Codex: codex queue) and records the decision; the wake latency is printed.
- stop, per runtime: a record edited by hand and left uncommitted; the Stop hook blocks, and the store ends committed.
- ask tool, per runtime: an interactive session asks a multiple-choice question with its ask tool; pm raises it as a
  question need, which the site shows; the test answers Blue in the TUI; the question closes with that chat answer, the
  Stop hook does not block, and an answer sent later from the site's card is refused and never reaches the session.
- ask on site, per runtime: the same question answered on the site's card while it waits (Claude Code: its dialog
  open; Codex: the async question pending); the session continues with that answer (Claude Code: the tool's result;
  Codex: a message codex queue put on the thread) and the question closes as answered on the site; the latency from
  the owner's answer to the session taking it in is printed.
- chat ask, Codex with no claude on PATH: the final reply asks the owner only in chat; the owner-request judge, on
  codex exec, blocks the stop, and the session raises a need.
- mixed: a Claude session and a Codex session in parallel on one sprint's two ready tasks; while both holds are live,
  the test claims each session's task as the other session, from a worktree of its own, and pm must refuse, naming
  the holder, and change nothing; and each session's pm show must name the other's hold.

The one assertion that differs by runtime, by design: a Claude session's need stores its own messaging socket as the
inbox the service pushes into, a Codex session's `codex-queue:<its CODEX_HOME>`, which the service queues the reply
under, as raised and after a resumed session's start refreshes it. Every scenario also fails when the fake
supervisor's log shows the clone's service reloaded or restarted after setup (an agent's own pm init must not restart
it under the others), and prints whether each session typed pm init. The chat-ask scenario leaves the session-start
hooks untrusted (Clone.distrust): with pm's rules in context an agent raises the need before it stops, and the Stop
hook would have nothing to judge. The ask-tool scenario answers at the TUI's keyboard (Tui.keys): Claude Code's dialog
by its pick list; Codex's question, whose pick list its TUI offers only while the turn runs, by typing the answer once
the turn ended. The ask-on-site one posts the card's form, as the owner's browser does.

Skipped unless PM_LIVE_TESTS=1 with `claude` and `codex` on PATH, both logged in; run with `make test-live-parity`:
the 14 scenarios run at once, one worker each, 95 s to 5 min in all (the solo Claude run is the long one when it polls
for the reply instead of waiting); test_ci holds that count to the scenarios pytest collects and to the Makefile's
-n. PM_PARITY_TIMEOUT sets each session's limit in seconds (default 600). Each run prints its latency, and its output
and transcript paths, kept under the test's tmp dir (-rA shows them).

PM_PARITY_CODEX_SANDBOX sets Codex's mode: approve-for-me, or a sandbox mode with no approvals (workspace-write, the
default, read-only, danger-full-access). A Codex user is expected to run approve-for-me (--approve-for-me, Codex's
counterpart of Claude Code's auto mode): the workspace-write sandbox, approval on request, and each request to run
outside the sandbox decided by Codex's automatic reviewer (a codex-auto-review model call). A host where bubblewrap
cannot start (`bwrap: loopback: Failed RTM_NEWADDR`) runs no sandboxed command: under approve-for-me the session asks
to run each failed command outside the sandbox and the reviewer allows it, but Codex's file edit tool (apply_patch)
fails there before any review, so the Stop scenario fails; such a host passes the set only with danger-full-access.
"""

from __future__ import annotations

import html
import json
import os
import re
import secrets
import shutil
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

import pytest

from agents import Clone, Run, Tui, git
from conftest import chunks

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(os.environ.get("PM_LIVE_TESTS") != "1" or not (shutil.which("claude") and shutil.which("codex")),
                       reason="live agent eval: set PM_LIVE_TESTS=1 (make test-live-parity) with claude and codex on "
                              "PATH"),
]

RUNTIMES = ["claude", "codex"]
TIMEOUT = float(os.environ.get("PM_PARITY_TIMEOUT", "600"))
OWNER_ANSWER = "Blue: it reads best on the site."
# The owner answers this long after the request is raised, so the session must wait for the answer, not find it there.
OWNER_DELAY = 30


@pytest.fixture
def clone(tmp_path: Path):
    c = Clone(tmp_path)
    yield c
    c.close()


class Checks:
    """Every assertion of one scenario, collected, so one costly run reports all that failed, with its evidence. Its
    end checks what every scenario must hold: the clone's service was never restarted while the sessions worked."""

    def __init__(self, clone: Clone, *runs: Run):
        self.clone, self.runs, self.failed = clone, list(runs), []

    def __call__(self, ok, what: str) -> None:
        if not ok:
            self.failed.append(what)

    def done(self) -> None:
        self(not self.clone.service_restarts(),
             f"the clone's pm service was reloaded or restarted after setup: {self.clone.service_restarts()}")
        for r in self.runs:
            typed = [c for c in r.commands() if re.search(r"\bpm init\b", c)]
            print(f"\n{r.name}: {r.runtime} session {r.session or '?'}, exit {r.returncode}, {r.seconds:.1f}s, "
                  f"typed pm init: {'yes' if typed else 'no'}\n  output {r.stdout}\n  transcript {r.transcript}")
        runtime = [f"{r.name}: {r.runtime} failed on its own: {r.failure()}" for r in self.runs if r.failure()]
        assert not self.failed, "\n".join(runtime + self.failed)


def context_reached(check: Checks, run: Run, sprint: str) -> None:
    """(a) Session start gave the session pm's rules, every chunk whole, and the state, which names the sprint's
    project."""
    got = [c.strip() for c in run.hook_contexts()]
    for n, chunk in enumerate(chunks(), 1):
        check(chunk.strip() in got, f"{run.name}: rules chunk {n} is not in the session's hook context")
    check(any("projects:" in c and re.search(r"^\s+demo\s", c, re.M) for c in got),
          f"{run.name}: the state (pm show at session start) is not in the session's hook context")


def cites(record: str, need: str) -> bool:
    """Whether an owner decision block in `record` cites `need`: pm adds "Answers `<need>`." unless the text already
    names it. Its level, sprint or project, is the session's choice, so callers look in both records."""
    return any(re.search(rf"(?<![\w.]){re.escape(need)}(?![\w.]|\.\d)", body)
               for body in re.findall(r"^::: decision \{source=owner[^}]*\}\n(.*?)^:::$", record, re.S | re.M))


def inbox_by_runtime(check: Checks, run: Run, need: dict, when: str) -> None:
    """The need's stored inbox, by runtime. This is the one assertion that differs by runtime, by design: a Claude
    Code session's need carries the session's own messaging socket, which the pm service pushes the reply into (Claude
    Code names it <XDG_RUNTIME_DIR>/cc-socks/<its pid>.sock, and a resumed session binds a new one, which session start
    refreshes); a Codex session has no socket, so its need carries none (pm reply wait and the UserPromptSubmit hook
    reach it). It guards pm's hook-side runtime detection: a hook whose CLAUDE_CODE_SESSION_ID differs from the
    payload's session_id is Codex's, and must not store the Claude Code socket it inherited: a Codex session's need
    carries its CODEX_HOME, under which the pm service queues the reply (codex queue)."""
    inbox = ((need.get("need") or {}).get("raised_by") or {}).get("inbox")
    if run.runtime == "claude":
        check(inbox and inbox.endswith(f"/cc-socks/{run.pid}.sock"),
              f"{run.name}: {when}, the need's inbox is {inbox!r}, not this session's socket .../cc-socks/{run.pid}.sock")
    else:
        want = f"codex-queue:{check.clone.codex_home}"
        check(inbox == want, f"{run.name}: {when}, a Codex session's need carries the inbox {inbox!r}, not {want!r}")


def owner_answers(clone: Clone, task: str, seen: dict, stop: threading.Event, agent) -> None:
    """The owner: once a decision need under `task` is open, note the task's holder and the need, and answer it
    OWNER_DELAY seconds later. An error of the owner's ends the session at once and is the test's failure."""
    try:
        while not stop.is_set():
            items = clone.items()
            need = next((i for i in items.values() if i["parent"] == task and i["type"] == "need"
                         and i["status"] == "open"), None)
            if need:
                seen.update(need=need["id"], holder=(items[task]["holder"] or {}).get("session"),
                            asked=time.monotonic(), raised=need)
                if stop.wait(OWNER_DELAY):
                    return
                res = clone.pm("reply", "add", need["id"], text=OWNER_ANSWER)
                seen["reply"] = (res.returncode, res.stdout + res.stderr)
                return
            stop.wait(2)
    except Exception as e:  # whatever stopped the owner fails the test, with the session stopped
        seen["error"] = f"{type(e).__name__}: {e}"
        agent.proc.kill()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_solo_session_gets_pm_guarantees(clone, runtime):
    ids = clone.seed(2)
    task, sprint, nonce = ids["task1"], ids["sprint"], secrets.token_hex(4)
    prompt = f"""You work in this git repo, whose project work pm tracks; pm's rules are in your context. Use pm for \
every step:
1. Make a git worktree of your own for this work, and run the steps from it.
2. Claim task {task}.
3. Add this finding to sprint {sprint}: "parity nonce {nonce}".
4. Ask the owner to decide, under task {task}: should the parity doc name the color red or blue? Red is the default, \
as the simplest.
5. Wait for the owner's answer; it arrives within a few minutes.
6. Record the owner's answer as a decision that cites the request.
7. Create a doc record with slug parity-{nonce} for project demo, whose text names the chosen color; leave no record \
uncommitted.
8. Commit a file parity-{nonce}.txt in your worktree, then close task {task} on that commit.
Then reply with one line: done."""
    seen, stop = {}, threading.Event()
    agent = clone.start(runtime, f"solo-{runtime}", prompt)
    owner = threading.Thread(target=owner_answers, args=(clone, task, seen, stop, agent), daemon=True)
    owner.start()
    run = agent.finish(TIMEOUT)
    stop.set()
    owner.join(30)
    items = clone.items()
    check = Checks(clone, run)
    print(f"\nsolo {runtime}: {run.seconds:.1f}s; owner asked at "
          f"{seen['asked'] - agent.started:.1f}s, answered {OWNER_DELAY}s later" if "asked" in seen else f"\nsolo {runtime}: {run.seconds:.1f}s; no ask")
    check(not seen.get("error"), f"the owner (this test) failed, so the session was stopped: {seen.get('error')}")
    check(run.returncode == 0, f"{run.name}: the session exited {run.returncode}")
    check(run.session, f"{run.name}: no session id in the runtime's output")

    context_reached(check, run, sprint)
    # (b) the claim, the request and the decision are the session's own
    check(seen.get("holder") == run.session, f"{run.name}: task holder {seen.get('holder')} is not the session")
    need = items.get(seen.get("need", ""), {})
    check(need, f"{run.name}: the session raised no decision need under {task}")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised == run.session, f"{run.name}: the need was raised by {raised}, not the session")
    inbox_by_runtime(check, run, seen.get("raised", {}), "as raised")
    inbox_by_runtime(check, run, need, "at the end")
    check(seen.get("reply", (0,))[0] == 0, f"{run.name}: the owner's reply failed: {seen.get('reply')}")
    check(need.get("closed_by") == run.session,
          f"{run.name}: the need was closed by {need.get('closed_by')}, not the session")
    # (c) the reply reached the session, and a decision that cites the need closed it
    check((need.get("need") or {}).get("delivered", 0) >= 1, f"{run.name}: the reply was not delivered: {need.get('need')}")
    check(OWNER_ANSWER in run.text(), f"{run.name}: the owner's answer never reached the session's transcript")
    print(f"solo {runtime}: the owner's answer reached the session by {run.reached_by(OWNER_ANSWER) or 'nothing'}")
    check((need.get("status"), need.get("resolution")) == ("closed", "answered"),
          f"{run.name}: the need is {need.get('status')}/{need.get('resolution')}, not closed/answered")
    decided = [(clone.store() / f).read_text() for f in ("sprints/demo-1.md", "projects/demo.md")]
    check(any(cites(r, seen.get("need", "?")) for r in decided),
          f"{run.name}: no owner decision in the sprint or project record cites the need")
    # (d) nothing left uncommitted; the finding and the doc are committed
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    log = clone.records_log()
    check(f"parity nonce {nonce}" in log, f"{run.name}: the finding's nonce is not in a records commit")
    docs = list(clone.store().glob(f"docs/*parity-{nonce}*.md"))
    check(docs and "blue" in docs[0].read_text().lower() and docs[0].name in log,
          f"{run.name}: no committed doc parity-{nonce} naming blue: {docs}")
    # (e) the task is closed by the session
    t = items[task]
    check((t["status"], t["resolution"], t["closed_by"]) == ("closed", "done", run.session),
          f"{run.name}: task {task} is {t['status']}/{t['resolution']} closed by {t['closed_by']}")
    check.done()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_stop_hook_blocks_uncommitted_records(clone, runtime):
    clone.seed(1)
    nonce = secrets.token_hex(4)
    prompt = (f"This run checks pm's Stop hook. Edit the file records/sprints/demo-1.md by hand with your file "
              f'editing tool: under the "## Findings" heading, replace the line "None yet." with "- stop nonce {nonce}". '
              f"Do not commit the edit and run no command after it: end your turn right after it, with one line: "
              f"edited. The Stop hook must find the edit uncommitted; when it then tells you what is left to do, do it.")
    run = clone.run(runtime, f"stop-{runtime}", prompt, TIMEOUT)
    check = Checks(clone, run)
    print(f"\nstop {runtime}: {run.seconds:.1f}s, {len(run.stop_blocks())} stop blocks")
    check(run.returncode == 0, f"{run.name}: the session exited {run.returncode}")
    check(any("records/sprints/demo-1.md" in r for r in run.stop_blocks()),
          f"{run.name}: no Stop hook block naming the record in the transcript: {run.stop_blocks()}")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check(f"stop nonce {nonce}" in clone.records_log(), f"{run.name}: the hand edit is not in a records commit")
    check.done()


def test_codex_without_claude_is_blocked_asking_only_in_chat(clone):
    """The owner-request judge runs on the session's own runtime: a Codex session on a machine with no claude, whose
    final reply asks the owner something only in chat, is blocked by the Stop hook (its judge on codex exec), and then
    raises the request as a need."""
    ids = clone.seed(1)
    sprint = ids["sprint"]
    # without pm's session-start rules (their hooks left untrusted), so the agent asks in chat as told: with them, a
    # rule-abiding agent raises the need before it ever stops, and the Stop hook has nothing to judge
    assert clone.distrust("session_start") > 0
    run = clone.run("codex", "chat-ask-codex", f"""This run checks a Stop hook. Do not use any tool now: end your turn at \
once with this question to the owner as your whole reply, word for word: "Should the parity doc name the color red or \
blue?" When a hook then tells you what is left to do, do it, under sprint {sprint}, and end with one line: asked.""", TIMEOUT, without=("claude",))
    items = clone.items()
    check = Checks(clone, run)
    blocks = run.stop_blocks()
    print(f"\nchat ask codex: {run.seconds:.1f}s, {len(blocks)} stop blocks")
    check(run.returncode == 0, f"{run.name}: the session exited {run.returncode}")
    check(any("no open request of this session covers" in b and "red or blue" in b for b in blocks),
          f"{run.name}: no owner-request Stop block quoting the ask in the transcript: {blocks}")
    needs = [i for i in items.values() if i["type"] == "need" and i["parent"] in (sprint, ids["task1"])
             and ((i.get("need") or {}).get("raised_by") or {}).get("session") == run.session]
    check(needs, f"{run.name}: the session raised no need under the sprint after the block")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check.done()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_reply_reaches_the_session_at_its_next_prompt(clone, runtime):
    """A session that raised a need and ended its turn gets the owner's reply when it is resumed, and records it: from
    the UserPromptSubmit hook, or, for a Codex session the pm service queued the reply into while its rollout was live,
    as the queued message the resumed thread takes in."""
    ids = clone.seed(1)
    sprint = ids["sprint"]
    asked = clone.run(runtime, f"next-prompt-1-{runtime}", f"""You work in this git repo, whose project work pm \
tracks; pm's rules are in your context. Use pm to ask the owner to decide, under sprint {sprint}: should the parity doc \
name the color red or blue? Red is the default, as the simplest. Do not wait for the answer: end your turn right after \
raising it, with one line: asked.""", TIMEOUT)
    items = clone.items()
    need = next((i for i in items.values() if i["parent"] == sprint and i["type"] == "need"), {})
    replied = clone.pm("reply", "add", need["id"], text=OWNER_ANSWER) if need.get("status") == "open" else None
    run = clone.run(runtime, f"next-prompt-2-{runtime}", "Go on with your work in this repo: do whatever pm says is "
                    "left for you, then reply with one line: done.", TIMEOUT, resume=asked.session)
    items = clone.items()
    check = Checks(clone, asked, run)
    print(f"\nnext prompt {runtime}: turn 1 {asked.seconds:.1f}s, turn 2 {run.seconds:.1f}s; the answer reached it by "
          f"{run.reached_by(OWNER_ANSWER) or 'nothing'}")
    check(asked.returncode == 0 and run.returncode == 0, f"exits {asked.returncode}, {run.returncode}")
    check(need, f"{asked.name}: the session raised no need under {sprint}")
    check(run.session == asked.session, f"the resumed session is {run.session}, not {asked.session}")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised == asked.session, f"{asked.name}: the need was raised by {raised}, not the session")
    inbox_by_runtime(check, asked, need, "as raised")
    check(replied is not None and replied.returncode == 0, f"the owner's reply failed: {replied}")
    reached = run.reached_by(OWNER_ANSWER)
    check(any(r.startswith("hook") or r == "queued" for r in reached),
          f"{run.name}: the reply reached the resumed session by {reached or 'nothing'}, not the UserPromptSubmit hook "
          f"or a message the pm service queued into it")
    need = items.get(need.get("id", ""), {})
    inbox_by_runtime(check, run, need, "after the resumed session's start refreshed it")
    check((need.get("need") or {}).get("delivered", 0) >= 1, f"{run.name}: the reply was not delivered: {need.get('need')}")
    check((need.get("status"), need.get("resolution"), need.get("closed_by")) == ("closed", "answered", run.session),
          f"{run.name}: the need is {need.get('status')}/{need.get('resolution')} closed by {need.get('closed_by')}")
    decided = [(clone.store() / f).read_text() for f in ("sprints/demo-1.md", "projects/demo.md")]
    check(any(cites(r, need.get("id", "?")) for r in decided),
          f"{run.name}: no owner decision in the sprint or project record cites the need")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check.done()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_an_idle_interactive_session_is_woken_by_the_owners_reply(clone, runtime):
    """An interactive session (Claude Code's or Codex's TUI, in a pty) raises a need and ends its turn without
    waiting. Once it is idle, the owner replies; with nothing typed into it, the session starts a turn on the reply,
    which the pm service delivered into it (Claude Code: its messaging socket; Codex: codex queue), and records the
    decision citing the need. Prints the wake latency, from the reply to the turn that takes it in."""
    ids = clone.seed(1)
    sprint = ids["sprint"]
    tui = Tui(clone, runtime, f"wake-{runtime}", f"""You work in this git repo, whose project work pm tracks; pm's rules are in your context. Use pm to ask the owner to decide, under sprint {sprint}: should the parity doc name the color red or blue? Red is the default, as the simplest. Do not wait for the answer: end your turn right after raising it, with one line: asked. When the owner's answer reaches you later, record it as pm says, and reply with one line: recorded.""")
    need, replied, woke, deadline = {}, None, None, time.monotonic() + TIMEOUT
    try:
        while time.monotonic() < deadline and not need:  # the need, then the session idle
            need = next((i for i in clone.items().values() if i["parent"] == sprint and i["type"] == "need"), {})
            time.sleep(2)  # noqa: TID251 — the poll interval over a live session, under its deadline
        while time.monotonic() < deadline and need and not tui.idle(quiet=8):
            time.sleep(2)  # noqa: TID251 — the poll interval over a live session, under its deadline
        replied_at = time.monotonic()
        replied = clone.pm("reply", "add", need["id"], text=OWNER_ANSWER) if need else None
        while time.monotonic() < deadline and replied and woke is None:  # the reply as a turn's input
            if any(r in ("pushed", "queued") for r in tui_reached(tui)):
                woke = time.monotonic() - replied_at
            time.sleep(0.5)  # noqa: TID251 — the poll interval the wake latency is measured at
        while time.monotonic() < deadline and woke is not None and \
                clone.items().get(need["id"], {}).get("status") != "closed":
            time.sleep(2)  # noqa: TID251 — the poll interval over a live session, under its deadline
    finally:
        run = tui.run()
    items = clone.items()
    need = items.get(need.get("id", ""), need)
    check = Checks(clone, run)
    print(f"\nwake {runtime}: woke {f'{woke:.1f}s' if woke is not None else 'never'} after the owner's reply, by "
          f"{run.reached_by(OWNER_ANSWER) or 'nothing'}")
    check(need, f"{run.name}: the session raised no need under {sprint}")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised and raised == run.session, f"{run.name}: the need was raised by {raised}, not the session {run.session}")
    inbox_by_runtime(check, run, need, "as raised")
    check(replied is not None and replied.returncode == 0, f"the owner's reply failed: {replied}")
    check(woke is not None, f"{run.name}: no turn took the reply in within {TIMEOUT:.0f}s, with nothing typed")
    want = "pushed" if runtime == "claude" else "queued"
    check(want in run.reached_by(OWNER_ANSWER),
          f"{run.name}: the reply reached the session by {run.reached_by(OWNER_ANSWER)}, not {want}")
    check((need.get("need") or {}).get("delivered", 0) >= 1, f"{run.name}: the reply was not delivered: {need.get('need')}")
    check((need.get("status"), need.get("resolution"), need.get("closed_by")) == ("closed", "answered", run.session),
          f"{run.name}: the need is {need.get('status')}/{need.get('resolution')} closed by {need.get('closed_by')}")
    decided = [(clone.store() / f).read_text() for f in ("sprints/demo-1.md", "projects/demo.md")]
    check(any(cites(r, need.get("id", "?")) for r in decided),
          f"{run.name}: no owner decision in the sprint or project record cites the need")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check.done()


ASK_TOOLS = {"claude": ("AskUserQuestion",), "codex": ("request_user_input_async", "request_user_input")}


def ask_tool_calls(entries: list[dict], runtime: str) -> list[dict]:
    """The session's ask tool calls, as its transcript records them: Claude Code's tool_use, Codex's function_call."""
    out = []
    for e in entries:
        if runtime == "claude":
            content = (e.get("message") or {}).get("content")
            out += [c for c in (content if isinstance(content, list) else [])
                    if isinstance(c, dict) and c.get("type") == "tool_use" and c.get("name") in ASK_TOOLS[runtime]]
        elif e.get("type") == "response_item" and (e.get("payload") or {}).get("type") == "function_call" and \
                e["payload"].get("name") in ASK_TOOLS[runtime]:
            out.append(e["payload"])
    return out


ASK_PROMPT = """I am here in this chat. Ask me one multiple-choice question with your ask tool for questions to the \
user (not in plain text, and not as a pm need): "Should the parity doc name the color red or blue?" with the options \
Red and Blue. After asking, do not wait or poll: end your turn. When my answer reaches you, reply with one line: chose \
<the color>."""
# how the owner answers in the TUI's chat: Claude Code's dialog shows the pick list, and the owner picks the second
# option, Blue; Codex's request_user_input_async returns at once (its PostToolUse hook raised the question), and its
# TUI offers the pick list ("shift + ← to answer") only while the turn runs, so once the turn ended the owner types
# the answer into the composer
ASK_CHAT = {"claude": [b"\x1b[B", b"\r"], "codex": [b"B", b"l", b"u", b"e", b"\r"]}


def questions(clone: Clone) -> list[dict]:
    """The question needs in the clone's work store, oldest first."""
    return [i for i in clone.items().values() if i["type"] == "need" and (i.get("need") or {}).get("kind") == "question"]


def site(clone: Clone) -> str:
    """The site's overview, as the owner's browser gets it from the clone's pm service."""
    with urllib.request.urlopen(f"http://localhost:{clone.port}/", timeout=30) as r:
        return r.read().decode()


def site_answer(clone: Clone, page: str, need: str, fields: list[tuple[str, str]]) -> tuple[int, str]:
    """The owner's answer posted from a question's card on the site page `page`, with the token it carries, as the
    browser posts the card's form; the status and the body."""
    token = re.search(r'name="token" value="([^"]+)"', page)
    form = urllib.parse.urlencode([("token", token[1] if token else ""), ("id", need), *fields]).encode()
    req = urllib.request.Request(f"http://localhost:{clone.port}/reply", data=form)

    class Stay(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *_):
            return None

    try:
        with urllib.request.build_opener(Stay).open(req, timeout=30) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def card_option(page: str, need: str, word: str) -> str | None:
    """The value the question's card posts for its first question's option naming `word`, None when the page shows no
    such card."""
    for value in re.findall(rf'name="q0" value="([^"]*)" form="reply-{re.escape(need)}"', page):
        if word.lower() in html.unescape(value).lower():
            return html.unescape(value)
    return None


def continued_with(entries: list[dict], runtime: str, word: str) -> bool:
    """Whether the session took in an answer naming `word` after its ask tool call: Claude Code's AskUserQuestion
    result whose answers name it; a Codex user message after the request_user_input_async call (the owner's pick typed
    in the chat, or the site's answer codex queue put on the thread)."""
    called = False
    for e in entries:
        if runtime == "claude":
            answers = (e.get("toolUseResult") or {}).get("answers") if isinstance(e.get("toolUseResult"), dict) else None
            if answers and any(word.lower() in str(v).lower() for v in answers.values()):
                return True
            continue
        p = e.get("payload") or {}
        if p.get("type") == "function_call" and p.get("name") in ASK_TOOLS[runtime]:
            called = True
        elif called and e.get("type") == "response_item" and p.get("type") == "message" and p.get("role") == "user":
            said = " ".join(c.get("text", "") for c in p.get("content", []) if isinstance(c, dict))
            if not said.startswith("<") and word.lower() in said.lower():
                return True
    return False


def ask_and_answer(clone: Clone, runtime: str, name: str, on: str) -> tuple[Run, dict]:
    """An interactive session (Claude Code's or Codex's TUI, in a pty) with the owner in its chat asks a multiple-choice
    question with its ask tool. Once the question is on the session's screen, raised as a question need and on the
    site's card, the owner (this test) picks Blue: `on` "chat", at the TUI's keyboard; "site", posting the card's form.
    Then it waits for the session to take the answer in and reply, and for the need to close. What it saw by name:
    need (the question as it was open), page (the site's overview then), posted (a site answer's status and body),
    took (seconds from the owner's answer to the session taking it in), closed (seconds from it to the need closed, or
    to the deadline), stale (a site answer posted from the card once
    the chat answer closed the question)."""
    clone.seed(1)
    tui = Tui(clone, runtime, name, ASK_PROMPT)
    seen: dict = {}
    deadline = time.monotonic() + TIMEOUT
    try:
        while time.monotonic() < deadline and not (seen.get("need") and (
                "Enter to select" in tui.text() if runtime == "claude" else tui.idle(quiet=3))):
            seen["need"] = next((q for q in questions(clone) if q["status"] == "open"), None)
            time.sleep(1)  # noqa: TID251 — the poll interval over a live session, under its deadline
        while time.monotonic() < deadline and seen.get("need") and seen.get("option") is None:
            seen["page"] = site(clone)
            seen["option"] = card_option(seen["page"], seen["need"]["id"], "blue")
            time.sleep(1)  # noqa: TID251 — the service's snapshot taking the need in, under its deadline
        if seen.get("option") is None:
            return tui.run(), seen
        time.sleep(2)  # noqa: TID251 — the question settles on screen before the owner answers
        answered = time.monotonic()
        if on == "chat":
            tui.keys(*ASK_CHAT[runtime])
            answered = time.monotonic()
        else:
            seen["posted"] = site_answer(clone, seen["page"], seen["need"]["id"], [("q0", seen["option"])])
        while time.monotonic() < deadline and "took" not in seen:
            if continued_with(tui.entries(), runtime, "blue"):
                seen["took"] = time.monotonic() - answered
            time.sleep(0.5)  # noqa: TID251 — the poll interval the latency is measured at
        while time.monotonic() < deadline and not \
                re.search(r"chose\W+blue", " ".join(json.dumps(e) for e in tui.entries()[-6:]), re.I):
            time.sleep(2)  # noqa: TID251 — the poll interval over a live session, under its deadline
        while time.monotonic() < deadline and \
                clone.items().get(seen["need"]["id"], {}).get("status") != "closed":
            time.sleep(1)  # noqa: TID251 — the poll interval over the work store, under its deadline
        seen["closed"] = time.monotonic() - answered
        if on == "chat":  # the owner's site tab still shows the card: an answer sent from it must reach nobody
            while time.monotonic() < deadline and card_option(site(clone), seen["need"]["id"], "red") is not None:
                time.sleep(1)  # noqa: TID251 — the service's snapshot taking the close in, under its deadline
            red = card_option(seen["page"], seen["need"]["id"], "red")
            seen["stale"] = site_answer(clone, seen["page"], seen["need"]["id"], [("q0", red or "Red")])
        quiet = time.monotonic() + 15  # time for any late delivery to show, then the session idle
        while time.monotonic() < deadline and (time.monotonic() < quiet or not tui.idle(quiet=5)):
            time.sleep(2)  # noqa: TID251 — the poll interval over a live session, under its deadline
    finally:
        run = tui.run()
    return run, seen


def asked_checks(check: Checks, run: Run, seen: dict, need: dict) -> None:
    """What both ask-tool scenarios hold of the asking: the tool was called, the Stop hook did not block, the question
    (and no other need) was raised by the session as a question need carrying the question and its options, and the
    site showed it as a card the owner can answer."""
    asked = ask_tool_calls(run.entries, run.runtime)
    check(asked, f"{run.name}: the session made no {' or '.join(ASK_TOOLS[run.runtime])} call")
    blocks = [b for b in run.stop_blocks() if "no open request of this session" in b or "authorized" in b]
    check(not blocks, f"{run.name}: the owner-request Stop hook blocked the turn: {blocks}")
    needs = [i for i in check.clone.items().values() if i["type"] == "need"]
    check([n["id"] for n in needs] == [need.get("id")],
          f"{run.name}: the needs raised are {[(n['id'], (n.get('need') or {}).get('kind')) for n in needs]}, not "
          f"the one question {need.get('id')}")
    check(need, f"{run.name}: the ask tool's question was never raised as an open question need")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised and raised == run.session, f"{run.name}: the question was raised by {raised}, not {run.session}")
    qs = ((need.get("need") or {}).get("ask") or {}).get("questions") or []
    check(len(qs) == 1 and "red or blue" in qs[0]["text"].lower() and
          sorted(o["label"].lower() for o in qs[0]["options"]) == ["blue", "red"],
          f"{run.name}: the question need does not carry the question and its options Red and Blue: {qs}")
    check(seen.get("option"), f"{run.name}: the site never showed the question's card with the option Blue")


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_an_ask_tool_question_answered_in_chat_closes_with_the_chat_answer(clone, runtime):
    """The owner is in the chat of an interactive session, which asks a multiple-choice question with its runtime's ask
    tool (AskUserQuestion; Codex's request_user_input_async in Default mode). pm raises it as a question need, which
    the site shows; the owner (this test) answers Blue in the TUI (ASK_CHAT). The question closes with that chat
    answer, the owner-request Stop hook does not block, and an answer sent later from the site's card is refused and
    never reaches the session."""
    run, seen = ask_and_answer(clone, runtime, f"ask-tool-{runtime}", on="chat")
    need = clone.items().get((seen.get("need") or {}).get("id", ""), {})
    check = Checks(clone, run)
    took = f"{seen['took']:.1f}s" if "took" in seen else "never"
    print(f"\nask tool {runtime}: {run.seconds:.1f}s; question {need.get('id')} on the site: "
          f"{'yes' if seen.get('option') else 'no'}; the chat answer took {took} to reach the session; stale site answer: "
          f"{seen.get('stale')}")
    asked_checks(check, run, seen, need)
    check(seen.get("took") is not None, f"{run.name}: the owner's pick in the TUI never reached the session")
    check(re.search(r"chose\W+blue", run.text(), re.I), f"{run.name}: the session never replied with the pick")
    replies = [c for c in need.get("comments", []) if c["kind"] == "reply"]
    print(f"ask tool {runtime}: the question closed {need.get('status')}/{need.get('resolution')} "
          f"({need.get('close_reason')}) {seen.get('closed', 0):.1f}s after the answer, with "
          f"{[c['text'] for c in replies]}")
    check((need.get("status"), need.get("resolution"), need.get("close_reason")) == ("closed", "answered", "Responded"),
          f"{run.name}: the question is {need.get('status')}/{need.get('resolution')} ({need.get('close_reason')}), "
          f"not closed/answered with the chat answer")
    check(len(replies) == 1 and re.search(r"blue\W*$", replies[0]["text"].strip(), re.I),
          f"{run.name}: the question does not hold the chat answer alone: {need.get('comments')}")
    check((need.get("need") or {}).get("delivered", 0) == 0,
          f"{run.name}: a site reply counts as delivered: {need.get('need')}")
    stale = seen.get("stale") or (0, "")
    check(stale[0] == 400 and "already closed" in stale[1],
          f"{run.name}: a site answer sent once the chat answer closed the question was not refused: {stale}")
    check(not continued_with(run.entries, runtime, "red"), f"{run.name}: an answer of Red reached the session")
    check.done()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_an_ask_tool_question_answered_on_the_site_reaches_the_session(clone, runtime):
    """The owner is away from the chat: the session asks with its ask tool, and while the question waits (Claude Code:
    its dialog open; Codex: the async question pending, its turn ended), the owner answers Blue on the site's card. The
    session continues with that answer (Claude Code: AskUserQuestion's result, from pm hook ask's PermissionRequest
    decision; Codex: a message codex queue put on the thread) and the question closes as answered on the site. Prints
    the latency from the owner's answer to the session taking it in."""
    run, seen = ask_and_answer(clone, runtime, f"ask-site-{runtime}", on="site")
    need = clone.items().get((seen.get("need") or {}).get("id", ""), {})
    replies = [c for c in need.get("comments", []) if c["kind"] == "reply"]
    body = re.sub(r"\n*<!-- pm-reply [0-9a-f-]+ -->\s*$", "", replies[0]["text"]) if replies else ""
    check = Checks(clone, run)
    took = f"{seen['took']:.1f}s" if "took" in seen else "never"
    print(f"\nask on site {runtime}: {run.seconds:.1f}s; question {need.get('id')}; posted {seen.get('posted')}; the "
          f"session took the answer in {took} after it was posted, "
          f"by {run.reached_by(body) if body else 'nothing'}")
    asked_checks(check, run, seen, need)
    check((seen.get("posted") or (0,))[0] == 303, f"{run.name}: the site refused the owner's answer: {seen.get('posted')}")
    check(len(replies) == 1 and body.lower().endswith(": blue"),
          f"{run.name}: the question does not hold the site's answer, one line ending Blue: {need.get('comments')}")
    check(seen.get("took") is not None, f"{run.name}: the site's answer never reached the session")
    if runtime == "claude":
        results = [(e["toolUseResult"]["answers"], (e.get("permissionDecision") or {}).get("source"))
                   for e in run.entries if isinstance(e.get("toolUseResult"), dict) and "answers" in e["toolUseResult"]]
        check(any(set(a.values()) == {seen.get("option")} and source == "hook" for a, source in results),
              f"{run.name}: AskUserQuestion did not return the site's answer {seen.get('option')!r} by pm hook ask's "
              f"PermissionRequest decision: {results}")
    else:
        check("queued" in run.reached_by(body),
              f"{run.name}: the site's answer reached the session by {run.reached_by(body)}, not codex queue")
    check(re.search(r"chose\W+blue", run.text(), re.I), f"{run.name}: the session never replied with the answer")
    check((need.get("status"), need.get("resolution"), need.get("close_reason")) ==
          ("closed", "answered", "answered on the site"),
          f"{run.name}: the question is {need.get('status')}/{need.get('resolution')} ({need.get('close_reason')}), "
          f"not closed/answered on the site")
    check((need.get("need") or {}).get("delivered", 0) == 1, f"{run.name}: the site's answer is not marked delivered: "
          f"{need.get('need')}")
    check.done()


def tui_reached(tui: Tui) -> list[str]:
    """How the owner's answer reached the TUI's session so far (Run.reached_by), from its live transcript."""
    t = tui.transcript()
    return Run(tui.runtime, tui.name, "", t, 0, 0, tui.log, tui.entries()).reached_by(OWNER_ANSWER) if t else []


def as_session(clone: Clone, runtime: str, session: str, *args: str, cwd: Path | None = None):
    """pm `args` as the session `session` of `runtime` runs it: the agents' environment with that runtime's session
    variables, and no other runtime's."""
    marker = {"CLAUDE_CODE_SESSION_ID": session} if runtime == "claude" else {"CODEX_SESSION_ID": session,
                                                                              "CODEX_THREAD_ID": session}
    return clone.pm(*args, cwd=cwd, env=dict(clone.agent_env(), **marker))


def test_claude_and_codex_work_one_sprint_together(clone):
    ids = clone.seed(2)
    sprint, tasks, nonce = ids["sprint"], {ids["task1"], ids["task2"]}, secrets.token_hex(4)

    def prompt(runtime: str) -> str:
        return f"""You work in this git repo, whose project work pm tracks; pm's rules are in your context. Use pm for \
every step:
1. Make a git worktree of your own for this work, and run the steps from it.
2. Claim the first ready task of sprint {sprint}. Another agent works on this sprint at the same time: if pm refuses \
your claim because another live session holds that task, claim the other ready task instead.
3. Add this finding to the sprint: "{runtime} nonce {nonce}".
4. Run `sleep 60`: the other agent needs the time.
5. Commit a file {runtime}-{nonce}.txt in your worktree, then close your task on that commit.
Then reply with one line: the id of the task you closed."""

    # a worktree of the test's own, from which it claims a task as each session: pm refuses a claim in the main checkout
    probe = clone.tmp / "probe-wt"
    git(clone.root, "worktree", "add", "-q", "-b", "probe", str(probe), env=clone.env)
    running = {rt: clone.start(rt, f"mixed-{rt}", prompt(rt)) for rt in RUNTIMES}
    views, claims, holders = {}, {}, {}
    deadline = time.monotonic() + TIMEOUT
    while time.monotonic() < deadline and any(a.proc.poll() is None for a in running.values()):
        items = clone.items()
        held = {t: (items[t]["holder"] or {}).get("session") for t in tasks if items[t]["holder"]}
        for t, s in held.items():
            holders.setdefault(t, set()).add(s)
        sessions = {rt: a.session() for rt, a in running.items()}
        if not views and len(set(held.values())) == 2 and set(held.values()) == set(sessions.values()):
            for rt, s in sessions.items():
                other = next(t for t, h in held.items() if h != s)
                shown = as_session(clone, rt, s, "show")
                views[rt] = (shown.stdout + shown.stderr, other)
                # while the other session's hold is live, this session's claim of its task is refused and changes
                # nothing
                before = clone.items()[other]
                res = as_session(clone, rt, s, "task", "claim", other, cwd=probe)
                claims[rt] = (res, other, held[other], before, clone.items()[other])
        time.sleep(2)  # noqa: TID251 — the watch loop's poll interval over two live agent sessions, under its deadline
    runs = {rt: a.finish(TIMEOUT) for rt, a in running.items()}
    items = clone.items()
    check = Checks(clone, *runs.values())
    print("\nmixed: " + ", ".join(f"{rt} {r.seconds:.1f}s" for rt, r in runs.items()))
    for r in runs.values():
        check(r.returncode == 0, f"{r.name}: the session exited {r.returncode}")
        context_reached(check, r, sprint)
    sessions = {rt: r.session for rt, r in runs.items()}
    check(len(set(sessions.values())) == 2 and all(sessions.values()), f"sessions not distinct: {sessions}")
    # no task claimed twice: each task had one holder, and the two holders are the two sessions
    check(all(len(h) == 1 for h in holders.values()) and len(holders) == 2,
          f"tasks and their holders over the run: {holders}")
    check({s for h in holders.values() for s in h} == set(sessions.values()),
          f"holders {holders} are not the two sessions {sessions}")
    closers = {items[t]["closed_by"] for t in tasks}
    check(all(items[t]["status"] == "closed" for t in tasks), f"open tasks: "
          f"{[t for t in tasks if items[t]['status'] != 'closed']}")
    check(closers == set(sessions.values()), f"tasks closed by {closers}, not by the two sessions {sessions}")
    log = clone.records_log()
    for rt in RUNTIMES:
        check(f"{rt} nonce {nonce}" in log, f"{rt}'s finding is not in a records commit")
    check(clone.uncommitted_records() == "", f"uncommitted records: {clone.uncommitted_records()}")
    # while both held a task, each session's pm show named the other's
    check(views, "the two sessions never held their tasks at once, so no session's view of the other was checked")
    check(claims, "the two sessions never held their tasks at once, so no cross claim was tried")
    for rt, (res, other, holder, before, after) in claims.items():
        check(res.returncode != 0 and f"{other} is held by live session {holder}" in res.stdout + res.stderr,
              f"{rt}'s claim of {other}, held by {holder}, was not refused naming the holder: exit {res.returncode}\n"
              f"{res.stdout}{res.stderr}")
        check(after == before, f"{rt}'s refused claim of {other} changed it:\n{before}\n{after}")
    for rt, (shown, other) in views.items():
        warned = shown.split("warning: other live sessions hold these tasks", 1)
        check(len(warned) == 2 and re.search(rf"^  {re.escape(other)}  ", warned[1], re.M),
              f"{rt}'s pm show does not list the other session's hold on {other}:\n{shown}")
    check.done()
