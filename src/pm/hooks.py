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
from datetime import datetime, timezone
from importlib.resources import files
from pathlib import Path

from pm.owner_request import hook_owner_request

# ---------------------------------------------------------------- pm prime

CAP = 10_000  # Claude Code's additionalContext limit, in characters
TIMEOUT = 20  # seconds; `pm show` takes about 1 s
SETUP_TIMEOUT = 6  # seconds; `pm setup` takes 0.4-1.6 s; a fresh clone's Beads bootstrap needs `bin/pm setup` by hand
WHERE_TIMEOUT = 3  # seconds; `pm where` takes about 0.5 s. With the others, under the hooks' 30 s timeout
HEADER = ("Project state from `bin/pm show` at session start, {at} UTC: a snapshot to orient by, which other sessions "
          "may have changed since; run `bin/pm show` again before stating project state to the owner.\n\n")
CUT = "\n… cut at the hook's 10,000-character limit; run `bin/pm show` for the rest."
SHOW = [sys.executable, "-m", "pm.cli", "show", "--refresh-inbox"]
SETUP = [sys.executable, "-m", "pm.cli", "setup"]
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
    return f"## Commands\n\n`pm` nouns: {nouns}."


def head() -> str:
    """The rules and the command list: `pm prime --rules`, never cut. Claude Code caps each hook's additionalContext
    at CAP, so the rules run as their own SessionStart hook and leave `pm show` its own cap."""
    return rules() + "\n\n" + commands()


def setup(cwd: str | None, cmd: list[str] | None = None) -> str:
    """What `pm setup` did, ending in a blank line, or one line saying why it did not run. Session start is the one
    place setup runs in a new worktree: it changes nothing in a set-up one, and readies one whatever tool created it
    (a git `post-checkout` hook would miss Claude Code's worktrees, added with --no-checkout and then reset)."""
    try:
        res = subprocess.run(cmd or SETUP, cwd=cwd, capture_output=True, text=True, timeout=SETUP_TIMEOUT)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm setup did not run at session start ({type(e).__name__}: {e}); run `bin/pm setup` by hand.\n\n"
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        return f"pm setup failed at session start ({why[-1] if why else f'exit {res.returncode}'}); run `bin/pm setup` by hand.\n\n"
    return f"`bin/pm setup` at session start:\n{res.stdout.strip()}\n\n"


def where(cwd: str | None, cmd: list[str] | None = None) -> str:
    """`pm where` under a heading, ending in a blank line, or one line saying why it did not run: every location and
    its state, since `pm show` works without the records link and never says it is missing."""
    try:
        res = subprocess.run(cmd or WHERE, cwd=cwd, capture_output=True, text=True, timeout=WHERE_TIMEOUT)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm where did not run at session start ({type(e).__name__}: {e}); run `bin/pm where` by hand.\n\n"
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        return f"pm where failed at session start ({why[-1] if why else f'exit {res.returncode}'}); run `bin/pm where` by hand.\n\n"
    return f"Locations from `bin/pm where` at session start:\n{res.stdout.strip()}\n\n"


def context(cwd: str | None, cmd: list[str] | None = None, session: str | None = None, cap: int = CAP) -> str:
    """`pm show` under a header, cut at a line to `cap` characters, or one line when it fails. `pm show` runs as the
    starting session, so its warning lists only tasks other live sessions hold, and with --refresh-inbox, which
    first points the session's open requests at its current inbox socket (a resumed session binds a new one) from
    the Beads read `pm show` makes anyway. It runs in a subprocess so a hang is cut off at TIMEOUT."""
    cmd = cmd or SHOW
    env = dict(os.environ, CLAUDE_CODE_SESSION_ID=session) if session else None
    try:
        res = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=TIMEOUT, env=env)
    except (OSError, subprocess.SubprocessError) as e:
        return f"pm show did not run at session start ({type(e).__name__}: {e}); run `bin/pm show` by hand."
    if res.returncode != 0:
        why = (res.stderr or res.stdout).strip().splitlines()
        return f"pm show failed at session start ({why[-1] if why else f'exit {res.returncode}'}); run `bin/pm show` by hand."
    text = HEADER.format(at=datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M")) + res.stdout.strip()
    if len(text) > cap:
        text = text[:text.rfind("\n", 0, cap - len(CUT))] + CUT
    return text


def state(cwd: str | None, session: str | None = None) -> str:
    """`pm prime --state`: what `pm setup` did and `pm where`, then `pm show`, all within CAP, so `pm show` gets what
    the setup and where lines leave and loses its last part first."""
    first = setup(cwd) + where(cwd)
    return first + context(cwd, session=session, cap=CAP - len(first))


def prime(cwd: str | None, session: str | None = None) -> str:
    """Plain `pm prime`, for a reader by hand: the rules and the command list, then the state."""
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


def subagent_context(cwd: str | None) -> str:
    """The subagent context: the Beads profile line, the rules and the command list; no `pm show`."""
    return profile(cwd) + "\n\n" + head()


def read_event() -> dict | None:
    """The hook input JSON on stdin, or None when it is not a JSON object."""
    try:
        event = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        return None
    return event if isinstance(event, dict) else None


def cmd_prime(part: str | None, hook_json: bool) -> int:
    """`pm prime`: the rules, the commands and the state; `part` prints one: "rules" (the rules and the commands),
    "state" (`pm setup`, `pm where` and `pm show`) or "subagent" (the profile line, the rules and the commands). The
    SessionStart hooks run "rules" and "state" as two hooks, each under its own CAP. With --hook-json it reads the
    SessionStart or SubagentStart input on stdin (cwd, session_id) and prints the envelope Claude Code and Codex both
    read: {"hookSpecificOutput": {"hookEventName": ..., "additionalContext": ...}}."""
    event = (read_event() or {}) if hook_json else {}
    cwd, session = event.get("cwd"), event.get("session_id")
    text = {"subagent": lambda: subagent_context(cwd), "rules": head, "state": lambda: state(cwd, session),
            None: lambda: prime(cwd, session)}[part]()
    if hook_json:
        name = "SubagentStart" if part == "subagent" else "SessionStart"
        text = json.dumps({"hookSpecificOutput": {"hookEventName": name, "additionalContext": text}})
    print(text)
    return 0


# ---------------------------------------------------------------- pm hook stop
# An agent may not hand back with records it edited by hand left uncommitted in the store. `pm` writes commit
# themselves, but Goal, Done when, design pages and delivery reports are edited by hand and committed with
# `pm commit`; a forgotten one is invisible on the records branch and blocks the next `pm` write to that record.
#
# The store is `<main checkout>/.records`, found from the clone's common git dir. When it has uncommitted files, the
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
    "Commit the ones you edited with `bin/pm commit -m \"<why>\" <path>...` (paths as listed, under records/), or "
    "revert them with `git -C {store} checkout -- <path>` (`rm` for a new file). Leave a file you did not edit: "
    "another session is writing it."
)


def git(cwd: str | Path | None, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=5, check=True).stdout


def store_of(cwd: str | None) -> Path:
    """The records store of the clone containing `cwd`: `.records` beside the clone's common .git dir."""
    common = Path(git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir").strip())
    return common.parent / ".records"


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


HOOKS = {"stop": hook_stop, "owner-request": hook_owner_request}
