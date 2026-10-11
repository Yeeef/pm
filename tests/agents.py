"""Real Claude Code and Codex sessions in a scratch clone, for the live parity eval (test_agent_parity_live.py).

A Clone is a repo set up by `pm init` as a user's is: a local bare origin, pm's pieces committed, the work store and
the records store, and the pm service, which the fake supervisor runs. The agents run in its main checkout:

- Claude Code with the user's login (their Claude config dir, as the judge's live eval runs it), user settings left
  out (`--setting-sources project,local`), so the session gets the hooks `pm init` wrote and nothing else;
- Codex with CODEX_HOME a temp dir holding a copy of the user's auth.json and a config this module writes: the clone
  trusted, and each of pm's hooks trusted by its hash, as the Codex user's own trust review would leave it.

HOME, the pm bin dir and the supervisor's units are the scratch ones in either runtime, so a hook's
`pm init --session-start` never touches the user's pm, units or Codex config.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import socket
import subprocess
import threading
import time
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
    res = subprocess.run(["git", *args], cwd=cwd, env=env, capture_output=True, text=True)
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


def _load(path: Path) -> list[dict]:
    out = []
    if path.exists():
        for line in path.read_text().splitlines():
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError:
                pass
    return out


# ---------------------------------------------------------------- the scratch clone

class Clone:
    """A repo set up by `pm init` under `tmp`, with its origin, its pm service and the scratch homes the agents use."""

    def __init__(self, tmp: Path):
        self.tmp = tmp
        self.root = tmp / "repo"
        self.origin = tmp / "origin.git"
        self.port = free_port()
        base = {k: v for k, v in os.environ.items() if not INHERITED.match(k)}
        self.env = dict(fake_env(tmp, base), PORT=str(self.port))
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

    # -- pm as the owner and the test run it: no session

    def pm(self, *args: str, text: str | None = None, cwd: Path | None = None,
           env: dict | None = None) -> subprocess.CompletedProcess:
        argv = [*PM, *args, *([f"--text={text}"] if text is not None else [])]
        return subprocess.run(argv, cwd=cwd or self.root, env=env or self.env, capture_output=True, text=True,
                              stdin=subprocess.DEVNULL, timeout=120)

    def must(self, *args: str, text: str | None = None, text_file: str | None = None) -> str:
        if text_file is not None:
            path = self.tmp / "body.md"
            path.write_text(text_file)
            args = (*args, "--text-file", str(path))
        res = self.pm(*args, text=text)
        assert res.returncode == 0, f"pm {' '.join(args)}: {res.stdout}\n{res.stderr}"
        return res.stdout

    def items(self) -> dict[str, dict]:
        res = subprocess.run([*PM, "export", "--store", str(self.root / ".pm/store/work")], cwd=self.tmp,
                             env=self.env, capture_output=True, text=True, timeout=120)
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
        shutil.copyfile(REAL_CODEX_AUTH, self.codex_home / "auth.json")
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
        """What either runtime runs with: the fakes' supervisor and gh, the pm under test on PATH, the scratch HOME
        and CODEX_HOME, and the user's Claude config dir, where Claude Code's login and the judge's are, and where pm
        finds a live Claude session's transcript."""
        bindir = self.tmp / "agent-bin"
        if not bindir.exists():
            bindir.mkdir()
            for tool in ("gh", "launchctl", "systemctl", "crontab"):
                (bindir / tool).symlink_to(self.tmp / "bin" / tool)
        env = {k: v for k, v in self.env.items() if not INHERITED.match(k)}
        path = os.pathsep.join(p for p in self.env["PATH"].split(os.pathsep) if p != str(self.tmp / "bin"))
        return dict(env, PATH=f"{bindir}{os.pathsep}{path}", CODEX_HOME=str(self.codex_home),
                    CLAUDE_CONFIG_DIR=str(CLAUDE_DIR), FAKE_SCHED_LOG=self.env["FAKE_SCHED_LOG"],
                    FAKE_SCHED_STATE=self.env["FAKE_SCHED_STATE"], FAKE_GH_STATE=self.env["FAKE_GH_STATE"])

    def start(self, runtime: str, name: str, prompt: str) -> "Agent":
        """Start a headless session of `runtime` in the main checkout on `prompt`; its output streams to the test's
        tmp dir."""
        if runtime == "claude":
            argv = [shutil.which("claude", path=REAL_PATH) or "claude", "-p", prompt, "--model", CLAUDE_MODEL,
                    "--setting-sources", "project,local", "--output-format", "stream-json", "--verbose",
                    "--allowedTools", "Bash", "Edit", "Write", "Read", "Glob", "Grep"]
        else:
            argv = [shutil.which("codex", path=REAL_PATH) or "codex", "exec", "--json", "-s", CODEX_SANDBOX,
                    "-C", str(self.root), "-c", f'projects."{self.root}".trust_level="trusted"',
                    *(["-m", CODEX_MODEL] if CODEX_MODEL else []), prompt]
        out = self.out / f"{name}.{runtime}.jsonl"
        with out.open("w") as f:
            proc = subprocess.Popen(argv, cwd=self.root, env=self.agent_env(), stdin=subprocess.DEVNULL, stdout=f,
                                    stderr=subprocess.STDOUT, start_new_session=True)
        agent = Agent(self, runtime, name, proc, out, time.monotonic())
        threading.Thread(target=agent.watch, daemon=True).start()
        return agent

    def run(self, runtime: str, name: str, prompt: str, timeout: float) -> Run:
        return self.start(runtime, name, prompt).finish(timeout)

    def close(self) -> None:
        """Stop the clone's pm service and whatever else the fake supervisor started, and drop the Claude Code
        project dirs this clone's sessions left in the user's Claude config (their transcripts are copied)."""
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
        if found:
            shutil.copyfile(found[0], transcript)
        entries = _load(transcript)
        if self.runtime == "codex":  # pm names a Codex session by its root's id, which session_meta holds
            meta = next((e["payload"] for e in entries if e.get("type") == "session_meta"
                         and e["payload"].get("id") == session), {})
            session = meta.get("session_id") or session
        return Run(self.runtime, self.name, session, transcript, took, rc, self.out, entries)


# The user's PATH, where the real claude and codex are: the tests' own PATH puts the fakes first.
REAL_PATH = os.environ["PATH"]
