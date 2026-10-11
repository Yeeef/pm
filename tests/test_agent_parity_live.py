"""Live parity eval: a real Claude Code session and a real Codex session get the same pm guarantees, and work one project
together. Each scenario runs headless sessions in a scratch clone that `pm init` set up (agents.Clone) and asserts from
the work store and the records store, not from the agent's prose; the hooks' evidence comes from each runtime's own
transcript (Claude Code's session JSONL, Codex's rollout).

- solo, per runtime, identical assertions: session-start context; claim, finding, a decision need answered by the
  owner (this test, through `pm reply add`) while the session waits; the decision citing it; a doc; the task closed.
- next prompt, per runtime: a session raises a need and ends its turn; the owner answers; the session, resumed on a
  prompt that does not name the need, gets the reply from the UserPromptSubmit hook and records the decision.
- stop, per runtime: a record edited by hand and left uncommitted; the Stop hook blocks, and the store ends committed.
- mixed: a Claude session and a Codex session in parallel on one sprint's two ready tasks; while both holds are live,
  the test claims each session's task as the other session, from a worktree of its own, and pm must refuse, naming
  the holder, and change nothing; and each session's pm show must name the other's hold.

Skipped unless PM_LIVE_TESTS=1 with `claude` and `codex` on PATH, both logged in; run with `make test-live-parity`.
PM_PARITY_CODEX_SANDBOX sets Codex's sandbox (default workspace-write; danger-full-access where bubblewrap cannot
start), PM_PARITY_TIMEOUT each session's limit in seconds (default 600). Each run prints its latency, and its output
and transcript paths, kept under the test's tmp dir.
"""

from __future__ import annotations

import os
import re
import secrets
import shutil
import threading
import time
from pathlib import Path

import pytest

from agents import Clone, Run, git
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
    """Every assertion of one scenario, collected, so one costly run reports all that failed, with its evidence."""

    def __init__(self, *runs: Run):
        self.runs, self.failed = list(runs), []

    def __call__(self, ok, what: str) -> None:
        if not ok:
            self.failed.append(what)

    def done(self) -> None:
        for r in self.runs:
            print(f"\n{r.name}: {r.runtime} session {r.session or '?'}, exit {r.returncode}, {r.seconds:.1f}s\n"
                  f"  output {r.stdout}\n  transcript {r.transcript}")
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


def owner_answers(clone: Clone, task: str, seen: dict, stop: threading.Event) -> None:
    """The owner: once a decision need under `task` is open, note the task's holder and the need, and answer it
    OWNER_DELAY seconds later."""
    while not stop.is_set():
        items = clone.items()
        need = next((i for i in items.values() if i["parent"] == task and i["type"] == "need"
                     and i["status"] == "open"), None)
        if need:
            seen.update(need=need["id"], holder=(items[task]["holder"] or {}).get("session"),
                        asked=time.monotonic())
            if stop.wait(OWNER_DELAY):
                return
            res = clone.pm("reply", "add", need["id"], text=OWNER_ANSWER)
            seen["reply"] = (res.returncode, res.stdout + res.stderr)
            return
        stop.wait(2)


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
    owner = threading.Thread(target=owner_answers, args=(clone, task, seen, stop), daemon=True)
    agent = clone.start(runtime, f"solo-{runtime}", prompt)
    owner.start()
    run = agent.finish(TIMEOUT)
    stop.set()
    owner.join(30)
    items = clone.items()
    check = Checks(run)
    print(f"\nsolo {runtime}: {run.seconds:.1f}s; owner asked at "
          f"{seen['asked'] - agent.started:.1f}s, answered {OWNER_DELAY}s later" if "asked" in seen else f"\nsolo {runtime}: {run.seconds:.1f}s; no ask")
    check(run.returncode == 0, f"{run.name}: the session exited {run.returncode}")
    check(run.session, f"{run.name}: no session id in the runtime's output")

    context_reached(check, run, sprint)
    # (b) the claim, the request and the decision are the session's own
    check(seen.get("holder") == run.session, f"{run.name}: task holder {seen.get('holder')} is not the session")
    need = items.get(seen.get("need", ""), {})
    check(need, f"{run.name}: the session raised no decision need under {task}")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised == run.session, f"{run.name}: the need was raised by {raised}, not the session")
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
    check = Checks(run)
    print(f"\nstop {runtime}: {run.seconds:.1f}s, {len(run.stop_blocks())} stop blocks")
    check(run.returncode == 0, f"{run.name}: the session exited {run.returncode}")
    check(any("records/sprints/demo-1.md" in r for r in run.stop_blocks()),
          f"{run.name}: no Stop hook block naming the record in the transcript: {run.stop_blocks()}")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check(f"stop nonce {nonce}" in clone.records_log(), f"{run.name}: the hand edit is not in a records commit")
    check.done()


@pytest.mark.parametrize("runtime", RUNTIMES)
def test_reply_reaches_the_session_at_its_next_prompt(clone, runtime):
    """A session that raised a need and ended its turn gets the owner's reply from the UserPromptSubmit hook when it is
    resumed, and records it."""
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
    check = Checks(asked, run)
    print(f"\nnext prompt {runtime}: turn 1 {asked.seconds:.1f}s, turn 2 {run.seconds:.1f}s; the answer reached it by "
          f"{run.reached_by(OWNER_ANSWER) or 'nothing'}")
    check(asked.returncode == 0 and run.returncode == 0, f"exits {asked.returncode}, {run.returncode}")
    check(need, f"{asked.name}: the session raised no need under {sprint}")
    check(run.session == asked.session, f"the resumed session is {run.session}, not {asked.session}")
    raised = ((need.get("need") or {}).get("raised_by") or {}).get("session")
    check(raised == asked.session, f"{asked.name}: the need was raised by {raised}, not the session")
    check(replied is not None and replied.returncode == 0, f"the owner's reply failed: {replied}")
    check(any(OWNER_ANSWER in c for c in run.prompt_contexts()),
          f"{run.name}: no UserPromptSubmit hook context carried the reply: {run.prompt_contexts()}")
    need = items.get(need.get("id", ""), {})
    check((need.get("need") or {}).get("delivered", 0) >= 1, f"{run.name}: the reply was not delivered: {need.get('need')}")
    check((need.get("status"), need.get("resolution"), need.get("closed_by")) == ("closed", "answered", run.session),
          f"{run.name}: the need is {need.get('status')}/{need.get('resolution')} closed by {need.get('closed_by')}")
    decided = [(clone.store() / f).read_text() for f in ("sprints/demo-1.md", "projects/demo.md")]
    check(any(cites(r, need.get("id", "?")) for r in decided),
          f"{run.name}: no owner decision in the sprint or project record cites the need")
    check(clone.uncommitted_records() == "", f"{run.name}: uncommitted records: {clone.uncommitted_records()}")
    check.done()


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
        time.sleep(2)
    runs = {rt: a.finish(TIMEOUT) for rt, a in running.items()}
    items = clone.items()
    check = Checks(*runs.values())
    print("\nmixed: " + ", ".join(f"{rt} {r.seconds:.1f}s" for rt, r in runs.items()))
    for rt, r in runs.items():
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
