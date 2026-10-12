"""Real Claude Code and Codex sessions in a scratch clone, for the live parity eval (test_agent_parity_live.py).

A Clone is a repo set up by `pm init` as a user's is: a local bare origin, pm's pieces committed, the work store and
the records store, and the pm service, which the fake supervisor runs. The agents run in its main checkout:

- Claude Code with the user's login (their Claude config dir, as the judge's live eval runs it), user settings left
  out (`--setting-sources project,local`), so the session gets the hooks `pm init` wrote and nothing else;
- Codex with CODEX_HOME a temp dir holding a copy of the user's auth.json and a config this module writes: the clone
  trusted, and each of pm's hooks trusted by its hash, as the Codex user's own trust review would leave it.

Every copy of a login (Codex's auth.json, Claude Code's .credentials.json for a TUI) is made by login_copy: mode 600,
removed when the clone closes, which its fixture runs after a failed test and a failed setup too.

HOME, the pm bin dir and the supervisor's units are the scratch ones in either runtime, so a hook's
`pm init --session-start` never touches the user's pm, units or Codex config.
"""

from __future__ import annotations

import contextlib
import fcntl
import json
import os
import pty
import re
import select
import shutil
import signal
import socket
import struct
import subprocess
import termios
import threading
import time
from collections.abc import Iterator
from dataclasses import dataclass, field
from pathlib import Path

from conftest import PM, REAL, fake_env, stop_services

CLAUDE_DIR = Path(REAL["CLAUDE_CONFIG_DIR"] or Path(REAL["HOME"]) / ".claude")
REAL_CODEX_AUTH = Path(REAL["CODEX_HOME"] or Path(REAL["HOME"]) / ".codex") / "auth.json"
# The sandbox Codex runs its commands in. workspace-write is what a Codex user runs; on a host where Codex's
# bubblewrap sandbox cannot start, danger-full-access is the only mode whose commands run.
CODEX_SANDBOX = os.environ.get("PM_PARITY_CODEX_SANDBOX", "workspace-write")
CLAUDE_MODEL = os.environ.get("PM_PARITY_CLAUDE_MODEL", "haiku")
CODEX_MODEL = os.environ.get("PM_PARITY_CODEX_MODEL", "")  # "" is Codex's default for the login
# What the agent's environment must not inherit from the session that runs the tests: a runtime's session and its
# markers, which pm reads to name the session.
INHERITED = re.compile(r"^(CLAUDE.*|CODEX_.*|AI_AGENT|GH_TOKEN|PM_.*)$")


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def git(cwd: Path, *args: str, env: dict | None = None) -> str:
    res = subprocess.run(["git", *args], cwd=cwd, env=env, capture_output=True, text=True, check=False)
    if res.returncode:
        raise AssertionError(f"git {' '.join(args)} exited {res.returncode} in {cwd}:\n{res.stderr}")
    return res.stdout


# ---------------------------------------------------------------- Codex's hook trust

class AppServer:
    """`codex app-server` over stdio, as Codex's TUI drives it: JSON-RPC requests, one JSON object per line."""

    def __init__(self, cwd: Path, env: dict[str, str]):
        self.proc = subprocess.Popen([shutil.which("codex", path=REAL_PATH) or "codex", "app-server"], cwd=cwd,
                                     env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, text=True)
        self.next = 0
        self.request("initialize", {"clientInfo": {"name": "pm-parity-eval", "title": None, "version": "0"}})
        self.send({"method": "initialized"})

    def send(self, msg: dict) -> None:
        self.proc.stdin.write(json.dumps(msg) + "\n")
        self.proc.stdin.flush()

    def request(self, method: str, params: dict) -> dict:
        """The result of `method`; notifications and server requests on the way are skipped."""
        self.next += 1
        self.send({"id": self.next, "method": method, "params": params})
        for line in self.proc.stdout:
            msg = json.loads(line)
            if msg.get("id") == self.next and "method" not in msg:
                assert "error" not in msg, f"codex app-server {method}: {msg['error']}"
                return msg["result"]
        raise AssertionError(f"codex app-server exited before answering {method}")

    def close(self) -> None:
        self.proc.stdin.close()
        self.proc.terminate()
        self.proc.wait(timeout=30)


def trust_project_hooks(cwd: Path, env: dict[str, str]) -> list[dict]:
    """Trust the project hooks Codex finds for `cwd` as the TUI's hook review does when the user trusts them
    (codex-rs tui/src/hooks_rpc.rs: hooks/list, then config/batchWrite of hooks.state upserting each untrusted or
    modified hook's key with its currentHash), and return the project hooks as hooks/list then reports them."""
    server = AppServer(cwd, env)
    try:
        def project_hooks() -> list[dict]:
            [entry] = server.request("hooks/list", {"cwds": [str(cwd)]})["data"]
            assert not entry["errors"], f"Codex's hook errors for {cwd}: {entry['errors']}"
            return [h for h in entry["hooks"] if h["source"] == "project"]
        review = [h for h in project_hooks() if h["trustStatus"] in ("untrusted", "modified")]
        if review:
            server.request("config/batchWrite", {
                "edits": [{"keyPath": "hooks.state", "mergeStrategy": "upsert",
                           "value": {h["key"]: {"trusted_hash": h["currentHash"]} for h in review}}],
                "filePath": None, "expectedVersion": None, "reloadUserConfig": True})
        return project_hooks()
    finally:
        server.close()


# ---------------------------------------------------------------- a run and its transcript

@dataclass
class Run:
    """One agent run: its runtime, its session as pm names it, its transcript (Claude Code's session JSONL, or Codex's
    rollout) copied into the test's tmp dir, and how it went."""
    runtime: str
    name: str
    session: str
    transcript: Path
    seconds: float
    returncode: int
    stdout: Path
    entries: list[dict] = field(default_factory=list)
    pid: int = 0  # the runtime's process: Claude Code names its messaging socket after it

    def text(self) -> str:
        return self.transcript.read_text() if self.transcript.exists() else ""

    def hook_contexts(self) -> list[str]:
        """Every additionalContext a hook gave the session, as the runtime recorded it: Claude Code's
        hook_additional_context attachments, Codex's developer messages."""
        out = []
        for e in self.entries:
            if self.runtime == "claude":
                a = e.get("attachment") or {}
                if a.get("type") in ("hook_additional_context",):
                    out.extend(a.get("content") or [])
            else:
                p = e.get("payload") or {}
                if e.get("type") == "response_item" and p.get("type") == "message" and p.get("role") == "developer":
                    out.extend(c.get("text", "") for c in p.get("content", []) if isinstance(c, dict))
        return out

    def stop_blocks(self) -> list[str]:
        """The reasons a Stop hook blocked the session's end, as the runtime recorded them: Claude Code's
        hook_blocking_error attachment, Codex's <hook_prompt hook_run_id="stop:…"> user message."""
        out = []
        for e in self.entries:
            if self.runtime == "claude":
                a = e.get("attachment") or {}
                if a.get("type") == "hook_blocking_error" and a.get("hookEvent") == "Stop":
                    out.append((a.get("blockingError") or {}).get("blockingError", ""))
            else:
                p = e.get("payload") or {}
                if e.get("type") == "response_item" and p.get("type") == "message" and p.get("role") == "user":
                    out.extend(c.get("text", "") for c in p.get("content", []) if isinstance(c, dict)
                               and c.get("text", "").startswith('<hook_prompt hook_run_id="stop:'))
        return out


    def commands(self) -> list[str]:
        """The shell commands the session ran, as its transcript holds them."""
        out = []
        for e in self.entries:
            if self.runtime == "claude":
                content = (e.get("message") or {}).get("content")
                out.extend((c.get("input") or {}).get("command", "") for c in
                           (content if isinstance(content, list) else [])
                           if isinstance(c, dict) and c.get("type") == "tool_use" and c.get("name") == "Bash")
            else:
                p = e.get("payload") or {}
                if e.get("type") == "response_item" and p.get("type") in ("function_call", "custom_tool_call",
                                                                            "local_shell_call"):
                    out.append(json.dumps(p.get("arguments") or p.get("input") or p.get("action")))
        return out

    def failure(self) -> str:
        """The runtime's own error for the run, from its output, else "": Codex's turn.failed (a usage limit, say),
        Claude Code's result with is_error. A run that failed here says nothing about pm."""
        for e in _load(self.stdout):
            if self.runtime == "codex" and e.get("type") == "turn.failed":
                return (e.get("error") or {}).get("message", "turn failed")
            if self.runtime == "claude" and e.get("type") == "result" and e.get("is_error"):
                return str(e.get("result") or e.get("subtype"))
        return ""

    def prompt_contexts(self) -> list[str]:
        """What the UserPromptSubmit hooks added to the session's prompts, as the runtime recorded it: Claude Code's
        hook_additional_context attachments of that event; Codex's developer messages that follow a prompt of the
        user's (session-start context comes before the first one)."""
        out, prompted = [], False
        for e in self.entries:
            if self.runtime == "claude":
                a = e.get("attachment") or {}
                if a.get("type") == "hook_additional_context" and a.get("hookEvent") == "UserPromptSubmit":
                    out.extend(a.get("content") or [])
            else:
                p = e.get("payload") or {}
                if e.get("type") != "response_item" or p.get("type") != "message":
                    continue
                texts = [c.get("text", "") for c in p.get("content", []) if isinstance(c, dict)]
                if p.get("role") == "user" and any(t and not t.startswith("<") for t in texts):
                    prompted = True
                elif p.get("role") == "developer" and prompted:
                    out.extend(texts)
        return out

    def reached_by(self, text: str) -> list[str]:
        """How `text` reached the session: "pushed" for a message Claude Code's inbox took in, "queued" for one codex
        queue put on a Codex thread, each a turn's input; "hook" for a hook's context; else each command whose output
        held it, or "background" for a background command's notice."""
        out, calls = [], {}
        for e in self.entries:
            if self.runtime == "claude":
                if (e.get("type"), e.get("operation")) == ("queue-operation", "enqueue") and text in str(e.get("content", "")):
                    out.append("background" if "<task-notification>" in e["content"] else "pushed")
                content = (e.get("message") or {}).get("content")
                said = content if isinstance(content, str) else " ".join(
                    c.get("text", "") for c in content or [] if isinstance(c, dict) and c.get("type") == "text")
                if e.get("type") == "user" and text in said and "<task-notification>" not in said:
                    out.append("pushed")  # an idle session takes a pushed message in as its turn's prompt
                a = e.get("attachment") or {}
                if a.get("type") == "hook_additional_context" and text in json.dumps(a):
                    out.append(f"hook {a.get('hookEvent')}")
                content = (e.get("message") or {}).get("content")
                for c in content if isinstance(content, list) else []:
                    if c.get("type") == "tool_use":
                        calls[c["id"]] = (c.get("input") or {}).get("command", c.get("name"))
                    elif c.get("type") == "tool_result" and text in json.dumps(c.get("content")):
                        out.append(str(calls.get(c.get("tool_use_id"), "?")))
            else:
                p = e.get("payload") or {}
                if p.get("type") in ("function_call", "custom_tool_call", "local_shell_call"):
                    calls[p.get("call_id")] = json.dumps(p.get("arguments") or p.get("input") or p.get("action"))
                elif p.get("type") in ("function_call_output", "custom_tool_call_output") and text in json.dumps(p):
                    out.append(calls.get(p.get("call_id"), "?"))
                elif p.get("type") == "message" and p.get("role") == "developer" and text in json.dumps(p):
                    out.append("hook")
                elif p.get("type") == "message" and p.get("role") == "user" and text in json.dumps(p):
                    out.append("queued")  # a message codex queue put on the thread, its turn's input
        return out


@contextlib.contextmanager
def login_copy(src: Path, dst: Path) -> Iterator[Path]:
    """A copy of a runtime's login (Codex's auth.json, Claude Code's .credentials.json) at `dst`, readable by the user
    alone (mode 600), removed when the block exits, a failing block too: the copies hold the user's live tokens."""
    fd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(src.read_bytes())
        yield dst
    finally:
        dst.unlink(missing_ok=True)


def _load(path: Path) -> list[dict]:
    out = []
    if path.exists():
        for line in path.read_text().splitlines():
            with contextlib.suppress(json.JSONDecodeError):
                out.append(json.loads(line))
    return out


# ---------------------------------------------------------------- the scratch clone

class Clone:
    """A repo set up by `pm init` under `tmp`, with its origin, its pm service and the scratch homes the agents use.
    The logins copied for its agents are `logins`, removed by close, which a failed setup runs too."""

    def __init__(self, tmp: Path):
        self.tmp = tmp
        self.logins = contextlib.ExitStack()
        try:
            self._set_up()
        except BaseException:
            self.close()
            raise

    def _set_up(self) -> None:
        tmp = self.tmp
        self.root = tmp / "repo"
        self.origin = tmp / "origin.git"
        self.port = free_port()
        base = {k: v for k, v in os.environ.items() if not INHERITED.match(k)}
        env = fake_env(tmp, base)
        # the fakes' supervisor and gh, never their claude or codex: the owner, pm init and the agents all run with
        # this PATH, so the service unit pm init writes is the one an agent's own pm init finds current, and no
        # agent's pm init restarts the service under the others
        bindir = tmp / "agent-bin"
        bindir.mkdir()
        for tool in ("gh", "launchctl", "systemctl", "crontab"):
            (bindir / tool).symlink_to(tmp / "bin" / tool)
        path = [p for p in env["PATH"].split(os.pathsep) if p != str(tmp / "bin")]
        self.env = dict(env, PORT=str(self.port), PATH=os.pathsep.join([str(bindir), *path]))
        self.home = Path(self.env["HOME"])
        self.codex_home = Path(self.env["CODEX_HOME"])
        self.out = tmp / "runs"
        self.out.mkdir()
        (self.home / ".gitconfig").write_text(
            "[init]\n\tdefaultBranch = main\n[user]\n\tname = parity\n\temail = parity@example.com\n"
            "[maintenance]\n\tauto = false\n")
        git(tmp, "init", "-q", "--bare", str(self.origin), env=self.env)
        git(tmp, "init", "-q", "-b", "main", str(self.root), env=self.env)
        (self.root / "README.md").write_text("A scratch repo for pm's live parity eval.\n")
        git(self.root, "add", "README.md", env=self.env)
        git(self.root, "commit", "-qm", "first", env=self.env)
        git(self.root, "remote", "add", "origin", str(self.origin), env=self.env)
        git(self.root, "push", "-q", "-u", "origin", "main", env=self.env)
        res = self.pm("init")
        assert res.returncode == 0, f"pm init failed:\n{res.stdout}\n{res.stderr}"
        git(self.root, "add", "-A", env=self.env)
        git(self.root, "commit", "-qm", "Install pm", env=self.env)
        git(self.root, "push", "-q", "origin", "main", env=self.env)
        self.write_codex_config()
        self.sched_mark = len(self.supervisor_calls())

    # -- pm as the owner and the test run it: no session

    def pm(self, *args: str, text: str | None = None, cwd: Path | None = None,
           env: dict | None = None) -> subprocess.CompletedProcess:
        argv = [*PM, *args, *([f"--text={text}"] if text is not None else [])]
        return subprocess.run(argv, cwd=cwd or self.root, env=env or self.env, capture_output=True, text=True,
                              stdin=subprocess.DEVNULL, timeout=120, check=False)

    def must(self, *args: str, text: str | None = None, text_file: str | None = None) -> str:
        if text_file is not None:
            path = self.tmp / "body.md"
            path.write_text(text_file)
            args = (*args, "--text-file", str(path))
        res = self.pm(*args, text=text)
        assert res.returncode == 0, f"pm {' '.join(args)}: {res.stdout}\n{res.stderr}"
        return res.stdout

    def supervisor_calls(self) -> list[list[str]]:
        """Every call of the fake supervisor (systemctl, launchctl, crontab) in this clone, in order."""
        log = Path(self.env["FAKE_SCHED_LOG"])
        return [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []

    def service_restarts(self) -> list[list[str]]:
        """The supervisor calls since setup that reload, restart, load or unload the clone's service: none may
        happen while sessions work, since every pm command fails while the service restarts."""
        verbs = {"restart", "daemon-reload", "bootstrap", "bootout", "kickstart"}
        return [c for c in self.supervisor_calls()[self.sched_mark:] if verbs & set(c[1:])]

    def items(self) -> dict[str, dict]:
        res = subprocess.run([*PM, "export", "--store", str(self.root / ".pm/store/work")], cwd=self.tmp,
                             env=self.env, capture_output=True, text=True, timeout=120, check=False)
        assert res.returncode == 0, res.stderr
        return {i["id"]: i for i in map(json.loads, res.stdout.splitlines())}

    def store(self) -> Path:
        return self.root / ".pm/store/records"

    def uncommitted_records(self) -> str:
        return git(self.store(), "status", "--porcelain", "--untracked-files=all", env=self.env)

    def records_log(self) -> str:
        """Every commit of the records store with its patch."""
        return git(self.store(), "log", "-p", "--format=commit %H%n%s", env=self.env)

    def seed(self, tasks: int) -> dict[str, str]:
        """A project `demo`, an open sprint and `tasks` ready tasks under it; their ids by name: project, sprint,
        task1, task2, …"""
        self.must("project", "open", "demo", "--title", "Demo", text="Show that two runtimes work one project.")
        out = self.must("sprint", "open", "demo", "--title", "Parity", text_file=(
            "## Goal\n\nBoth runtimes get pm's guarantees.\n\n## Scope\n\n**In:** the eval's tasks.\n\n"
            "**Out:** anything else.\n\n## Done when\n\n- Every task is closed.\n"))
        ids = {"project": "demo", "sprint": self._id(out)}
        for n in range(1, tasks + 1):
            ids[f"task{n}"] = self._id(self.must("task", "add", "--sprint", ids["sprint"], "--title", f"Parity task {n}"))
        return ids

    @staticmethod
    def _id(out: str) -> str:
        m = re.search(r"\b([a-z0-9]+-[a-z0-9]+(?:\.\d+)+)\b", out)
        assert m, f"no id in: {out}"
        return m.group(1)

    # -- the agents

    def write_codex_config(self) -> None:
        """CODEX_HOME's auth (a copy of the user's) and config: the clone trusted as a project, appended to what
        `pm init` wrote there, and pm's project hooks trusted as the user's hook review in Codex's TUI trusts them;
        each of pm's hooks must then be listed trusted."""
        self.codex_home.mkdir(exist_ok=True)
        assert REAL_CODEX_AUTH.is_file(), f"Codex is not logged in: no {REAL_CODEX_AUTH}"
        self.logins.enter_context(login_copy(REAL_CODEX_AUTH, self.codex_home / "auth.json"))
        config = self.codex_home / "config.toml"
        text = config.read_text() if config.exists() else ""
        config.write_text(text + f'\n[projects."{self.root}"]\ntrust_level = "trusted"\n')
        hooks = trust_project_hooks(self.root, self.agent_env())
        wrote = {h["command"] for g in json.loads((self.root / ".codex/hooks.json").read_text())["hooks"].values()
                 for grp in g for h in grp["hooks"]}
        listed = {h["command"]: h["trustStatus"] for h in hooks}
        assert wrote and wrote <= listed.keys(), f"Codex does not list pm's hooks {sorted(wrote - listed.keys())}"
        assert all(listed[c] == "trusted" for c in wrote), f"pm's hooks are not all trusted in Codex: {listed}"

    def agent_env(self) -> dict[str, str]:
        """What either runtime runs with: the clone's environment (the fakes' supervisor and gh, the pm under test on
        PATH, the scratch HOME and CODEX_HOME), and the user's Claude config dir, where Claude Code's login and the
        judge's are, and where pm finds a live Claude session's transcript."""
        env = {k: v for k, v in self.env.items() if not INHERITED.match(k)}
        return dict(env, CODEX_HOME=str(self.codex_home), CLAUDE_CONFIG_DIR=str(CLAUDE_DIR))

    def start(self, runtime: str, name: str, prompt: str, resume: str = "", without: tuple[str, ...] = ()) -> Agent:
        """Start a headless session of `runtime` in the main checkout on `prompt`, or resume the session `resume`
        with it as its next prompt; its output streams to the test's tmp dir. `without` names commands the session's
        PATH must not hold: each directory holding one is left out."""
        trust = f'projects."{self.root}".trust_level="trusted"'
        if runtime == "claude":
            argv = [shutil.which("claude", path=REAL_PATH) or "claude", "-p", prompt, "--model", CLAUDE_MODEL,
                    *(["--resume", resume] if resume else []),
                    "--setting-sources", "project,local", "--output-format", "stream-json", "--verbose",
                    "--allowedTools", "Bash", "Edit", "Write", "Read", "Glob", "Grep"]
        else:  # codex exec resume takes no -s or -C: the sandbox goes in as config, and it runs in the checkout
            argv = [shutil.which("codex", path=REAL_PATH) or "codex", "exec",
                    *(["resume"] if resume else []), "--json", "-c", f'sandbox_mode="{CODEX_SANDBOX}"', "-c", trust,
                    *(["-m", CODEX_MODEL] if CODEX_MODEL else []), *([resume] if resume else []), prompt]
        env = self.agent_env()
        env["PATH"] = os.pathsep.join(d for d in env["PATH"].split(os.pathsep)
                                      if not any((Path(d) / tool).exists() for tool in without))
        assert not any(shutil.which(tool, path=env["PATH"]) for tool in without), without
        out = self.out / f"{name}.{runtime}.jsonl"
        with out.open("w") as f:
            proc = subprocess.Popen(argv, cwd=self.root, env=env, stdin=subprocess.DEVNULL, stdout=f,
                                    stderr=subprocess.STDOUT, start_new_session=True)
        agent = Agent(self, runtime, name, proc, out, time.monotonic())
        threading.Thread(target=agent.watch, daemon=True).start()
        return agent

    def run(self, runtime: str, name: str, prompt: str, timeout: float, resume: str = "",
            without: tuple[str, ...] = ()) -> Run:
        return self.start(runtime, name, prompt, resume, without).finish(timeout)

    def close(self) -> None:
        """Remove the logins copied for the agents, stop the clone's pm service and whatever else the fake supervisor
        started, and drop the Claude Code project dirs this clone's sessions left in the user's Claude config (their
        transcripts are copied)."""
        self.logins.close()
        stop_services(self.tmp)
        for d in CLAUDE_DIR.glob("projects/" + re.sub(r"[^A-Za-z0-9]", "-", str(self.tmp)) + "*"):
            shutil.rmtree(d, ignore_errors=True)


@dataclass
class Agent:
    """A running session."""
    clone: Clone
    runtime: str
    name: str
    proc: subprocess.Popen
    out: Path
    started: float
    ended: float | None = None

    def watch(self) -> None:
        self.proc.wait()
        self.ended = time.monotonic()

    def session(self) -> str:
        """The session as pm names it, once the runtime has printed it, else "": Claude Code's session id, or
        Codex's thread id, which is its root session's for a root thread."""
        for e in _load(self.out):
            if self.runtime == "claude" and e.get("session_id"):
                return e["session_id"]
            if self.runtime == "codex" and e.get("type") == "thread.started":
                return e["thread_id"]
        return ""

    def finish(self, timeout: float) -> Run:
        """Wait for the session to end, at most `timeout` seconds from its start (then kill it), and copy its
        transcript beside its output."""
        try:
            rc = self.proc.wait(timeout=max(1, timeout - (time.monotonic() - self.started)))
        except subprocess.TimeoutExpired:
            os.killpg(self.proc.pid, 9)
            self.proc.wait()
            rc = -9
        took = (self.ended or time.monotonic()) - self.started
        session = self.session()
        if self.runtime == "claude":
            found = list(CLAUDE_DIR.glob(f"projects/*/{session}.jsonl")) if session else []
        else:
            found = list(self.clone.codex_home.glob(f"sessions/*/*/*/rollout-*-{session}.jsonl")) if session else []
        transcript = self.out.with_name(f"{self.name}.{self.runtime}-transcript.jsonl")
        if found:  # the newest: a resumed session's
            shutil.copyfile(max(found, key=lambda f: f.stat().st_mtime), transcript)
        entries = _load(transcript)
        if self.runtime == "codex":  # pm names a Codex session by its root's id, which session_meta holds
            meta = next((e["payload"] for e in entries if e.get("type") == "session_meta"
                         and e["payload"].get("id") == session), {})
            session = meta.get("session_id") or session
        return Run(self.runtime, self.name, session, transcript, took, rc, self.out, entries, self.proc.pid)


# ---------------------------------------------------------------- an interactive session in a pty

class Tui:
    """A runtime's interactive TUI (Claude Code's `claude`, Codex's `codex`) in a pseudo-terminal, as a person runs it,
    on a first prompt given on its command line. A thread reads the screen into a log, answers the terminal's queries
    (cursor position, device attributes, colors) as a terminal does, and accepts Claude Code's folder-trust dialog for
    the scratch clone should it show; nothing else is ever typed. Claude Code runs on a config dir of the clone's own
    (claude_config), so its first-run state and its transcripts never touch the user's."""

    def __init__(self, clone: Clone, runtime: str, name: str, prompt: str):
        self.clone, self.runtime, self.name = clone, runtime, name
        self.started = time.time()
        self.claude_dir = claude_config(clone)
        env = dict(clone.agent_env(), CLAUDE_CONFIG_DIR=str(self.claude_dir))
        trust = f'projects."{clone.root}".trust_level="trusted"'
        if runtime == "claude":
            argv = [shutil.which("claude", path=REAL_PATH) or "claude", prompt, "--model", CLAUDE_MODEL,
                    "--setting-sources", "project,local", "--allowedTools", "Bash", "Edit", "Write", "Read", "Glob",
                    "Grep"]
        else:
            argv = [shutil.which("codex", path=REAL_PATH) or "codex", "--no-alt-screen", "-s", CODEX_SANDBOX,
                    "-a", "never", "-c", trust, *(["-m", CODEX_MODEL] if CODEX_MODEL else []), prompt]
        self.log = clone.out / f"{name}.{runtime}.tty"
        self.pid, self.fd = pty.fork()
        if self.pid == 0:  # the child: the TUI, in the checkout, with the agents' environment and a terminal's
            os.chdir(clone.root)
            os.execvpe(argv[0], argv, dict(env, TERM="xterm-256color", COLUMNS="120", LINES="40"))
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        self.screen = b""
        self.alive = True
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self) -> None:
        answers = {b"\x1b[6n": b"\x1b[1;1R", b"\x1b[c": b"\x1b[?62;c", b"\x1b[0c": b"\x1b[?62;c",
                   b"\x1b]10;?": b"\x1b]10;rgb:ffff/ffff/ffff\x1b\\", b"\x1b]11;?": b"\x1b]11;rgb:0000/0000/0000\x1b\\"}
        trusted = False
        with self.log.open("ab") as log:
            while self.alive:
                r, _, _ = select.select([self.fd], [], [], 0.2)
                if not r:
                    continue
                try:
                    data = os.read(self.fd, 65536)
                except OSError:
                    break
                log.write(data)
                self.screen = (self.screen + data)[-20000:]
                for ask, answer in answers.items():
                    if ask in data:
                        os.write(self.fd, answer)
                plain = re.sub(rb"\x1b\[[0-9;?]*[A-Za-z]", b"", self.screen)
                if self.runtime == "claude" and not trusted and b"trust the files in this folder" in plain:
                    os.write(self.fd, b"\r")  # "Yes, proceed": the scratch clone is the eval's own
                    trusted = True

    def transcript(self) -> Path | None:
        """The session's transcript, the newest one begun since the TUI started: Claude Code's session JSONL for the
        checkout, or Codex's rollout under the clone's CODEX_HOME."""
        if self.runtime == "claude":
            key = re.sub(r"[^A-Za-z0-9]", "-", str(self.clone.root))
            found = list(self.claude_dir.glob(f"projects/{key}/*.jsonl"))
        else:
            found = list(self.clone.codex_home.glob("sessions/*/*/*/rollout-*.jsonl"))
        found = [f for f in found if f.stat().st_mtime >= self.started]
        return max(found, key=lambda f: f.stat().st_mtime) if found else None

    def entries(self) -> list[dict]:
        t = self.transcript()
        return _load(t) if t else []

    def session(self) -> str:
        """The session as pm names it: Claude Code's session id; Codex's root thread id, from the rollout's
        session_meta."""
        for e in self.entries():
            if self.runtime == "claude" and e.get("sessionId"):
                return e["sessionId"]
            if self.runtime == "codex" and e.get("type") == "session_meta":
                p = e["payload"]
                return p.get("session_id") or p.get("id", "")
        return ""

    def idle(self, quiet: float) -> bool:
        """Whether the session has ended its turn and its transcript has not changed for `quiet` seconds: Claude
        Code's last entry an assistant's reply that ends its turn (or a hook's after it), Codex's a task_complete."""
        t = self.transcript()
        if not t or time.time() - t.stat().st_mtime < quiet:
            return False
        last = [e for e in self.entries() if e.get("type") in ("assistant", "user", "event_msg")]
        if self.runtime == "codex":
            ended = [e for e in last if e.get("type") == "event_msg"]
            return bool(ended) and (ended[-1].get("payload") or {}).get("type") == "task_complete"
        turns = [e for e in last if e.get("type") in ("assistant", "user")]
        return bool(turns) and turns[-1].get("type") == "assistant" and \
            (turns[-1].get("message") or {}).get("stop_reason") in ("end_turn", None)

    def run(self) -> Run:
        """The session as a Run, its transcript copied beside the TUI's log; the TUI is stopped first."""
        self.alive = False
        with contextlib.suppress(ProcessLookupError):
            os.kill(self.pid, signal.SIGTERM)
        with contextlib.suppress(ChildProcessError):
            os.waitpid(self.pid, 0)
        session, found = self.session(), self.transcript()
        transcript = self.log.with_name(f"{self.name}.{self.runtime}-transcript.jsonl")
        if found:
            shutil.copyfile(found, transcript)
        return Run(self.runtime, self.name, session, transcript, time.time() - self.started, 0, self.log,
                   _load(transcript), self.pid)


def claude_config(clone: Clone) -> Path:
    """A Claude Code config dir of the clone's own for an interactive session: a copy of the user's login (one of the
    clone's logins), first-run onboarding done and the clone trusted, as a user's would be after their first session
    there."""
    d = clone.tmp / "claude-tui"
    if not d.exists():
        d.mkdir()
        creds = CLAUDE_DIR / ".credentials.json"
        assert creds.is_file(), f"Claude Code is not logged in: no {creds}"
        clone.logins.enter_context(login_copy(creds, d / ".credentials.json"))
        (d / ".claude.json").write_text(json.dumps({
            "hasCompletedOnboarding": True, "theme": "dark",
            "projects": {str(clone.root): {"hasTrustDialogAccepted": True, "hasCompletedProjectOnboarding": True}}}))
    return d


# The user's PATH, where the real claude and codex are: the tests' own PATH puts the fakes first.
REAL_PATH = os.environ["PATH"]
