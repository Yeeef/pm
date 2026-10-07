"""What the runtimes' hooks run: `pm prime` (session and subagent context) and `pm hook <name>` (the other hooks).

Every hook fails open: it exits 0 and says on stderr why it let the event through, because a broken hook must never
stop a session from starting or an agent from stopping. `pm hook owner-request` (pm.owner_request) instead exits 1
when its check cannot run, which both runtimes show without blocking the stop. The other exception runs before any
hook: pm's check of the repo's .pm/config.toml, which fails hard, so a session never runs a pm other than the one the
repo pins. Standard library only, so importing it stays cheap."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime, timezone
from importlib.resources import files
from pathlib import Path

from pm.config import STORE
from pm.owner_request import hook_owner_request

# ---------------------------------------------------------------- pm prime

CAP = 10_000  # Claude Code's additionalContext limit, in characters
TIMEOUT = 20  # seconds; `pm show` takes about 1 s
# seconds; `pm init` in a set-up clone takes 0.8-0.9 s (tests, 2026-10-07), and one that installs or restarts the pm
# service waits up to service.RESTART_WAIT (15 s) for its site; a fresh clone's Beads bootstrap needs `pm init` by hand
INIT_TIMEOUT = 18
WHERE_TIMEOUT = 3  # seconds; `pm where` takes about 0.5 s
BUDGET = 28  # seconds for init, where and show together, under the state hook's 30 s timeout
HEADER = ("Project state from `pm show` at session start, {at} UTC: a snapshot to orient by, which other sessions "
          "may have changed since; run `pm show` again before stating project state to the owner.\n\n")
CUT = "\n… cut at the hook's 10,000-character limit; run `pm show` for the rest."
SHOW = [sys.executable, "-m", "pm.cli", "show", "--refresh-inbox"]
INIT = [sys.executable, "-m", "pm.cli", "init"]
WHERE = [sys.executable, "-m", "pm.cli", "where"]


def rules() -> str:
    """pm's rules, shipped in the package so they match the installed pm."""
    return files("pm").joinpath("prime.md").read_text(encoding="utf-8").strip()


MACHINERY = {"prime", "hook", "push"}  # what the runtimes and the scheduler call, not agents


def commands() -> str:
    """pm's agent-facing nouns, read from the argparse parser so the list never drifts from the code."""
    from pm.cli import parser  # lazy: cli imports this module
    sub = next(a for a in parser()._subparsers._group_actions if a.dest == "cmd")
    nouns = ", ".join(f"`{act.dest}`" for act in sub._choices_actions if act.dest not in MACHINERY)
    return f"# Commands\n\n`pm` nouns: {nouns}."


def head() -> str:
    """The rules and the command list, whole and in order: what `chunks()` splits."""
    return rules() + "\n\n" + commands()


# Where `chunks()` cuts `head()`: the heading line each chunk starts with. Claude Code passes each hook's
# additionalContext inline only up to CAP characters (longer reaches the model as a 2 KB preview and a file path), so
# the rules run as one hook per chunk. The hooks of one entry run in parallel and arrive in any order, so each chunk
# starts with a title naming its place and its sections. The hook entries name each chunk by number: a new chunk is a
# new hook entry in every runtime's settings.
STARTS = ("# pm rules", "# Part 2: how", "## 7. Records", "## 8. Needs and actions")


def chunks() -> list[str]:
    """`head()` cut at the lines in STARTS, each chunk under a title line such as "# pm rules (2 of 4): Part 2: how —
    5. Reading state, 6. Projects, sprints and tasks". Without the titles, the chunks joined by blank lines are
    `head()`. Raises ValueError when a heading in STARTS is missing or out of order."""
    lines = head().split("\n")
    at = [lines.index(s) for s in STARTS]
    if at[0] != 0 or at != sorted(at):
        raise ValueError(f"the chunk headings {STARTS} are not in order at the top of prime.md")
    bodies = ["\n".join(lines[a:b]).strip() for a, b in zip(at, at[1:] + [len(lines)])]
    out, part = [], ""
    for i, body in enumerate(bodies, 1):
        groups = [] if body.startswith("# ") else [[part, []]]  # a chunk that starts inside a part names it
        for line in body.split("\n"):
            if line.startswith("# "):
                part = line[2:]
                groups.append([part, []])
            elif line.startswith("## "):
                groups[-1][1].append(line[3:])
        what = "; ".join(("the introduction" if p == "pm rules" else p) + (" — " + ", ".join(s) if s else "")
                         for p, s in groups)
        out.append(f"# pm rules ({i} of {len(bodies)}): {what}\n\n{body}")
    return out


def init(cwd: str | None, cmd: list[str] | None = None) -> str:
    """What `pm init` did, ending in a blank line, or one line saying why it did not run. Session start is the one
    place init runs in a new worktree: it changes nothing in a set-up one, and readies one whatever tool created it
    (a git `post-checkout` hook would miss Claude Code's worktrees, added with --no-checkout and then reset). A
    refusal is named by its error line, the first of its output."""
    try:
        res = subprocess.run(cmd or INIT, cwd=cwd, capture_output=True, text=True, timeout=INIT_TIMEOUT)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm init did not run at session start ({type(e).__name__}: {e}); run `pm init` by hand.\n\n"
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        line = next((l for l in why if l.startswith("error: ")), why[-1]) if why else f"exit {res.returncode}"
        return f"pm init failed at session start ({line}); run `pm init` by hand.\n\n"
    return f"`pm init` at session start:\n{res.stdout.strip()}\n\n"


def where(cwd: str | None, cmd: list[str] | None = None) -> str:
    """`pm where` under a heading, ending in a blank line, or one line saying why it did not run: every location and
    its state, since `pm show` works without the records link and never says it is missing."""
    try:
        res = subprocess.run(cmd or WHERE, cwd=cwd, capture_output=True, text=True, timeout=WHERE_TIMEOUT)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm where did not run at session start ({type(e).__name__}: {e}); run `pm where` by hand.\n\n"
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        return f"pm where failed at session start ({why[-1] if why else f'exit {res.returncode}'}); run `pm where` by hand.\n\n"
    return f"Locations from `pm where` at session start:\n{res.stdout.strip()}\n\n"


def context(cwd: str | None, cmd: list[str] | None = None, session: str | None = None, cap: int = CAP,
            timeout: float = TIMEOUT) -> str:
    """`pm show` under a header, cut at a line to `cap` characters, or one line when it fails. `pm show` runs as the
    starting session, so its warning lists only tasks other live sessions hold, and with --refresh-inbox, which
    first points the session's open requests at its current inbox socket (a resumed session binds a new one) from
    the Beads read `pm show` makes anyway. It runs in a subprocess so a hang is cut off at `timeout`."""
    cmd = cmd or SHOW
    env = dict(os.environ, CLAUDE_CODE_SESSION_ID=session) if session else None
    try:
        res = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout, env=env)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm show did not run at session start ({type(e).__name__}: {e}); run `pm show` by hand."
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        return f"pm show failed at session start ({why[-1] if why else f'exit {res.returncode}'}); run `pm show` by hand."
    text = HEADER.format(at=datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M")) + res.stdout.strip()
    if len(text) > cap:
        text = text[:text.rfind("\n", 0, cap - len(CUT))] + CUT
    return text


def state(cwd: str | None, session: str | None = None) -> str:
    """`pm prime --state`: what `pm init` did and `pm where`, then `pm show`, all within CAP, so `pm show` gets what
    the init and where lines leave and loses its last part first."""
    start = time.monotonic()
    first = init(cwd) + where(cwd)
    left = min(TIMEOUT, max(1.0, BUDGET - (time.monotonic() - start)))  # pm show gets what init and where left
    return first + context(cwd, session=session, cap=CAP - len(first), timeout=left)


def prime(cwd: str | None, session: str | None = None) -> str:
    """Plain `pm prime`, for a reader by hand: the rules and the command list in order, then the state."""
    return head() + "\n\n" + state(cwd, session)


def profile(cwd: str | None, cmd: list[str] | None = None) -> str:
    """The active Beads agent profile in one line, or why it could not be read."""
    cmd = cmd or ["bd", "config", "get", "agent.profile", "--json"]
    try:
        res = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=TIMEOUT)
        if res.returncode != 0:
            why = (res.stderr or res.stdout).strip().splitlines()
            raise RuntimeError(why[-1] if why else f"exit {res.returncode}")
        value = json.loads(res.stdout)["value"]
        if not value:
            raise RuntimeError("agent.profile is not set")
    except (OSError, subprocess.SubprocessError, RuntimeError, ValueError, KeyError, TypeError) as e:
        return f"Beads agent profile: unknown (bd config failed: {type(e).__name__}: {e})."
    note = " (commit and push are routine unless your brief says otherwise)" if value == "team-maintainer" else ""
    return f"Beads agent profile: {value}{note}."


def read_event() -> dict | None:
    """The hook input JSON on stdin, or None when it is not a JSON object."""
    try:
        event = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        return None
    return event if isinstance(event, dict) else None


def cmd_prime(part: str | int | None, hook_json: bool) -> int:
    """`pm prime`: the rules, the commands and the state; `part` prints one: N (chunk N of `chunks()`), "state"
    (`pm init`, `pm where` and `pm show`) or "subagent" (the line naming the Beads agent profile). SessionStart runs
    one hook per chunk plus "state", SubagentStart one per chunk plus "subagent", each under its own CAP. With
    --hook-json it reads the hook input on stdin (cwd, session_id, hook_event_name) and prints the envelope Claude
    Code and Codex both read: {"hookSpecificOutput": {"hookEventName": ..., "additionalContext": ...}}, named for the
    event that ran it, so one chunk command serves both events."""
    event = (read_event() or {}) if hook_json else {}
    cwd, session = event.get("cwd"), event.get("session_id")
    if isinstance(part, int):
        text = chunks()[part - 1]
    else:
        text = {"subagent": lambda: profile(cwd), "state": lambda: state(cwd, session),
                None: lambda: prime(cwd, session)}[part]()
    if hook_json:
        name = event.get("hook_event_name") or ("SubagentStart" if part == "subagent" else "SessionStart")
        text = json.dumps({"hookSpecificOutput": {"hookEventName": name, "additionalContext": text}})
    print(text)
    return 0


# ---------------------------------------------------------------- pm hook stop
# An agent may not hand back with records it edited by hand left uncommitted in the store. `pm` writes commit
# themselves, but Goal, Done when, design pages and delivery reports are edited by hand and committed with
# `pm commit`; a forgotten one is invisible on the records branch and blocks the next `pm` write to that record.
#
# The store is `<main checkout>/.pm/store/records`, found from the clone's common git dir. When it has uncommitted files, the
# hook blocks the stop once with a reason naming them. A session commits only its own records, and other sessions
# write to the same store at the same time, so it blocks only on files this session touched: a dirty file counts when
# its path under the store (such as `sprints/demo-1.md`) appears in one of this session's tool calls in the transcript
# (Claude Code `tool_use` inputs; Codex `function_call` arguments and `custom_tool_call` inputs, such as apply_patch).
# A file edited without naming that path (after `cd` into a store folder, or through a glob) is missed; a file this
# session only read but another session edited is named, and the reason says to leave a file it did not edit.
#
# Prints {"decision": "block", "reason": ...} to block, nothing to let the stop through. On `stop_hook_active` it
# always lets the stop through, so it never loops. One `git status` when the store is clean.

STOP_REASON = (
    "These records in the store ({store}) have uncommitted changes, and this session's tool calls name them:\n"
    "{files}\n"
    "Commit the ones you edited with `pm commit -m \"<why>\" <path>...` (paths as listed, under records/), or "
    "revert them with `git -C {store} checkout -- <path>` (`rm` for a new file). Leave a file you did not edit: "
    "another session is writing it."
)


def git(cwd: str | Path | None, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=5, check=True).stdout


def store_of(cwd: str | None) -> Path:
    """The records store of the clone containing `cwd`: `.pm/store/records` beside the clone's common .git dir."""
    common = Path(git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir").strip())
    return common.parent / STORE


def dirty(store: Path) -> list[str]:
    """Paths, relative to the store, of tracked files with changes and of untracked files."""
    out = git(store, "status", "--porcelain", "-z", "--untracked-files=all")
    entries, paths = out.split("\0"), []
    i = 0
    while i < len(entries):
        e = entries[i]
        if len(e) > 3:
            paths.append(e[3:])
            if "R" in e[:2] or "C" in e[:2]:  # a rename or copy, staged or not, is followed by its source path
                i += 1
        i += 1
    return paths


def tool_inputs(entry: dict) -> list[str]:
    """The tool-call inputs in one transcript line, as text: Claude Code `tool_use` blocks, Codex function and custom
    tool calls."""
    if entry.get("type") == "assistant":
        content = (entry.get("message") or {}).get("content")
        return [json.dumps(b.get("input"), ensure_ascii=False) for b in content or []
                if isinstance(b, dict) and b.get("type") == "tool_use"] if isinstance(content, list) else []
    payload = entry.get("payload")
    if entry.get("type") == "response_item" and isinstance(payload, dict):
        if payload.get("type") == "function_call":
            return [str(payload.get("arguments", ""))]
        if payload.get("type") == "custom_tool_call":
            return [str(payload.get("input", ""))]
        if payload.get("type") == "local_shell_call":
            return [json.dumps(payload.get("action"), ensure_ascii=False)]
    return []


def touched(transcript: str, paths: list[str]) -> list[str]:
    """The paths among `paths` that some tool call in the transcript names. Lines that name none are not parsed."""
    found: set[str] = set()
    with open(transcript, encoding="utf-8", errors="replace") as f:
        for line in f:
            hits = [p for p in paths if p not in found and p in line]
            if not hits:
                continue
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            if not isinstance(entry, dict):
                continue
            calls = tool_inputs(entry)
            found.update(p for p in hits if any(p in c for c in calls))
    return [p for p in paths if p in found]


def stop_reason(event: dict) -> str | None:
    """The block reason for this stop, or None to let it through."""
    if event.get("stop_hook_active"):
        return None
    try:
        store = store_of(event.get("cwd"))
        if not store.is_dir():
            return None  # a clone without a store has no records to commit
        if Path(git(store, "rev-parse", "--show-toplevel").strip()).resolve() != store.resolve():
            print(f"pm hook stop: {store} is not a worktree of its own; letting the stop through", file=sys.stderr)
            return None
        paths = dirty(store)
    except (OSError, subprocess.SubprocessError) as e:
        print(f"pm hook stop: git is unavailable ({type(e).__name__}); letting the stop through", file=sys.stderr)
        return None
    if not paths:
        return None
    transcript = event.get("transcript_path")
    try:
        mine = touched(transcript, paths) if transcript else None
    except OSError:
        mine = None
    if mine is None:
        print("pm hook stop: no readable transcript, so the records this session touched are unknown; "
              f"letting the stop through with {len(paths)} uncommitted in {store}", file=sys.stderr)
        return None
    if not mine:
        return None
    return STOP_REASON.format(store=store, files="\n".join(f"- records/{p}" for p in mine))


def hook_stop() -> int:
    event = read_event()
    if event is None:
        print("pm hook stop: hook input is not JSON; letting the stop through", file=sys.stderr)
        return 0
    reason = stop_reason(event)
    if reason:
        print(json.dumps({"decision": "block", "reason": reason}))
    return 0


# ---------------------------------------------------------------- pm hook git-pre-commit
# Records live on the records branch; a code-branch commit must not edit records/ (main's copy is the copy
# workflow's). A merge is let through, since merging main brings in the copy. Unlike the runtime hooks, this one
# refuses: it is the guard.

def hook_git_pre_commit() -> int:
    if Path(git(None, "rev-parse", "--path-format=absolute", "--git-dir").strip(), "MERGE_HEAD").exists():
        return 0
    if git(None, "diff", "--cached", "--name-only", "--", "records/").strip():
        print("error: this commit edits records/, which only the records branch may change; write records with pm\n"
              "and unstage these edits: git restore --staged records/", file=sys.stderr)
        return 1
    return 0


HOOKS = {"stop": hook_stop, "owner-request": hook_owner_request, "git-pre-commit": hook_git_pre_commit}
