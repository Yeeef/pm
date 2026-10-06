"""Fixtures: a temp clone whose records store holds a small record set, and a fake `bd` serving a JSON issue list."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

HARNESS = Path(__file__).resolve().parents[2] / "skills/project-management/harness"  # the hook scripts
# pm and the renderer from the package environment the tests run in (make test).
PM = [str(Path(sys.executable).with_name("pm"))]
RENDER = [sys.executable, "-m", "pm.render"]
FAKE_BD = Path(__file__).resolve().parent / "fake_bd.py"
FAKE_GH = Path(__file__).resolve().parent / "fake_gh.py"
FAKE_SCHED = Path(__file__).resolve().parent / "fake_sched.py"
FAKE_CLAUDE = Path(__file__).resolve().parent / "fake_claude.py"


def uv_dir(*args: str) -> str:
    return subprocess.run(["uv", "--color", "never", *args], check=True, capture_output=True, text=True).stdout.strip()


# HOME is a temp dir in tests, so pm setup writes its schedule there; uv keeps the real cache and Pythons.
UV_DIRS = {"UV_CACHE_DIR": uv_dir("cache", "dir"), "UV_PYTHON_INSTALL_DIR": uv_dir("python", "dir")}

SPRINT_TAIL = '''## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

{findings}

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

{outcome}

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

{against}
'''


def sprint(title, bead, findings="None yet.", outcome="Not closed yet.", against="Not closed yet."):
    return (f"---\ntype: sprint\ntitle: {title}\nbead: {bead}\n---\n\n"
            "## Goal\n\n> What should be true when this sprint ends, and why now?\n\nShip it.\n\n"
            "## Scope\n\n> What's in, and what's explicitly out? Keep this high level; implementation\n"
            "> details go in a design page.\n\n**In:** a.\n\n**Out:** b.\n\n"
            "## Done when\n\n> What evidence will show the goal is met?\n\n- It works.\n\n"
            + SPRINT_TAIL.format(findings=findings, outcome=outcome, against=against))


def project(title, bead, outcome="Not closed yet."):
    return (f"---\ntype: project\ntitle: {title}\nbead: {bead}\n---\n\n"
            "## Goal\n\n> Why do we do it? What is it? What outcome do we expect?\n\nA demo project.\n\n"
            "## Progress\n\n> Where are we now, and what's next?\n\n"
            "## Decisions\n\n> What constrains every future sprint?\n\n"
            "::: decision {source=owner date=2026-10-01}\nKeep it small. Because small is cheap.\n:::\n\n"
            "## Design pages\n\n> Where is the detail?\n\nNone yet.\n\n"
            f"## Outcome\n\n> Written when the project closes.\n\n{outcome}\n")


RECORDS = {
    "projects/demo.md": project("Demo", "demo"),
    "projects/old.md": project("Old", "old", outcome="Done: retired."),
    "sprints/demo-1.md": sprint("First", "demo.1"),
    "sprints/demo-2.md": sprint("Second", "demo.2", outcome="Done: shipped.", against="- It works: met."),
    "days/2026-10-01.md": "---\ntype: day\ndate: 2026-10-01\n---\n\n## Today\n\n> What are we chasing today, and why now?\n\nStart.\n",
}

ISSUES = [
    {"id": "demo", "title": "Demo", "status": "open", "issue_type": "epic", "created_at": "2026-10-01T12:00:00Z"},
    {"id": "demo.1", "title": "Sprint 1: First", "status": "open", "issue_type": "epic", "parent": "demo",
     "created_at": "2026-10-01T12:00:00Z"},
    {"id": "demo.1.1", "title": "Done task", "status": "closed", "issue_type": "task", "parent": "demo.1",
     "created_at": "2026-10-01T12:00:00Z", "closed_at": "2026-10-01T13:00:00Z"},
    {"id": "demo.1.2", "title": "Ask the owner", "status": "open", "issue_type": "task", "parent": "demo.1",
     "labels": ["human"], "created_at": "2026-10-01T12:00:00Z",
     "dependencies": [{"depends_on_id": "demo.1.1", "type": "blocks"}]},
    {"id": "demo.2", "title": "Sprint 2: Second", "status": "open", "issue_type": "epic", "parent": "demo",
     "created_at": "2026-10-01T12:00:00Z"},
    {"id": "old", "title": "Old", "status": "closed", "issue_type": "epic", "created_at": "2026-09-01T12:00:00Z"},
]


def fake_bd_env(tmp: Path, base) -> dict[str, str]:
    """`base` with the fake bd and gh first on PATH, bd serving ISSUES from tmp/bd.json and logging calls to
    tmp/bd.log, gh serving PRs from tmp/gh.json (none at first), claude a fake logging to tmp/claude.log; made on first use in `tmp`, so later calls keep
    their state and log. CODEX_HOME is tmp/codex, absent until a test makes it, so no test reads or edits the
    user's Codex config. HOME is tmp/home and launchctl, systemctl and crontab are fakes logging to tmp/sched.log,
    so no test installs a real schedule."""
    bindir = tmp / "bin"
    if not bindir.exists():
        bindir.mkdir()
        (bindir / "bd").symlink_to(FAKE_BD)
        (bindir / "gh").symlink_to(FAKE_GH)
        (bindir / "claude").symlink_to(FAKE_CLAUDE)
        for tool in ("launchctl", "systemctl", "crontab"):
            (bindir / tool).symlink_to(FAKE_SCHED)
        (tmp / "home").mkdir()
        (tmp / "gh.json").write_text("{}")
        (tmp / "bd.json").write_text(json.dumps(ISSUES))
        (tmp / "bd.log").write_text("")
    # a test names its session itself, and its transcripts live under tmp/claude, not the user's
    base = {k: v for k, v in base.items() if k not in ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID",
                                                         "CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN")}
    return dict(base, PATH=f"{bindir}{os.pathsep}{base['PATH']}", FAKE_BD_STATE=str(tmp / "bd.json"),
                FAKE_BD_LOG=str(tmp / "bd.log"), FAKE_GH_STATE=str(tmp / "gh.json"), CODEX_HOME=str(tmp / "codex"),
                CLAUDE_CONFIG_DIR=str(tmp / "claude"), HOME=str(tmp / "home"), XDG_CONFIG_HOME=str(tmp / "home/.config"),
                FAKE_SCHED_LOG=str(tmp / "sched.log"), FAKE_SCHED_STATE=str(tmp / "sched.json"),
                FAKE_CLAUDE_LOG=str(tmp / "claude.log"), **UV_DIRS)


class Repo:
    """A main checkout on branch main, its store at .records on branch records, and the records link to it."""

    def __init__(self, root: Path, tmp: Path):
        self.root, self.store, self.records = root, root / ".records", root / "records"
        self.state, self.log = tmp / "bd.json", tmp / "bd.log"
        self.noms = root / ".beads/embeddeddolt/demo/.dolt/noms"  # the Dolt store bd context points at; see dolt()
        self.env = fake_bd_env(tmp, os.environ)

    def pm(self, *args: str, stdin: str = "", cwd: Path | None = None) -> subprocess.CompletedProcess:
        return subprocess.run([*PM, *args],
                              cwd=cwd or self.root, env=self.env, input=stdin, capture_output=True, text=True)

    def git(self, *args: str, cwd: Path | None = None) -> str:
        return subprocess.run(["git", *args], cwd=cwd or self.root, check=True, capture_output=True, text=True).stdout

    def commit(self, msg: str = "change") -> None:
        """Commit whatever changed, records in the store and code in the main checkout."""
        for where in (self.store, self.root):
            if self.git("status", "--porcelain", cwd=where):
                self.git("add", "-A", cwd=where)
                self.git("commit", "-qm", msg, cwd=where)

    def worktree(self, branch: str) -> Path:
        """A code worktree on a new branch, outside the main checkout."""
        path = self.root.parent / branch
        self.git("worktree", "add", "-q", "-b", branch, str(path))
        return path

    def store_log(self) -> list[str]:
        return self.git("log", "--format=%s", cwd=self.store).splitlines()

    def write(self, rel: str, text: str) -> None:
        (self.records / rel).write_text(text)

    def issues(self) -> dict[str, dict]:
        return {i["id"]: i for i in json.loads(self.state.read_text())}

    def set_issue(self, issue_id: str, **fields) -> None:
        issues = json.loads(self.state.read_text())
        for i in issues:
            if i["id"] == issue_id:
                i.update(fields)
        self.state.write_text(json.dumps(issues))
        if self.noms.is_dir():
            with open(self.noms / "journal", "a") as f:
                f.write(json.dumps([issue_id, fields]) + "\n")

    def dolt(self) -> None:
        """Make the embedded Dolt store pm serve watches: a manifest and a journal that every bd write (fake bd or
        set_issue) grows, as Dolt's chunk journal does."""
        (self.noms / "oldgen").mkdir(parents=True)
        (self.noms / "manifest").write_text("5:fake\n")
        (self.noms / "journal").write_text("")

    def set_pr(self, url: str, state: str, merge: str | None = None) -> None:
        """What the fake gh reports for a PR: its state and, once merged, its merge commit."""
        path = Path(self.env["FAKE_GH_STATE"])
        prs = json.loads(path.read_text())
        prs[url] = {"state": state, "mergeCommit": {"oid": merge} if merge else None}
        path.write_text(json.dumps(prs))

    def bd_calls(self) -> list[list[str]]:
        return [json.loads(l) for l in self.log.read_text().splitlines()]

    def bd_writes(self) -> list[list[str]]:
        reads = lambda c: c[:1] in (["list"], ["show"]) or c in (["context", "--json"], ["export"]) or (c[:1] == ["comments"] and c[2:] == ["--json"])
        return [c for c in self.bd_calls() if not reads(c)]

    def comments(self, issue_id: str) -> list[str]:
        return [c["text"] for c in self.issues()[issue_id].get("comments", [])]

    def snapshot(self) -> dict[str, bytes]:
        return {p.relative_to(self.root).as_posix(): p.read_bytes()
                for p in sorted(self.root.rglob("*")) if p.is_file() and ".git" not in p.parts}


@pytest.fixture
def repo(tmp_path: Path) -> Repo:
    root = tmp_path / "repo"
    root.mkdir()
    r = Repo(root, tmp_path)
    r.git("init", "-q", "-b", "main")
    r.git("config", "user.email", "t@example.com")
    r.git("config", "user.name", "t")
    (root / ".gitignore").write_text("/records\n/.records/\n")
    r.git("add", ".gitignore")
    r.git("commit", "-qm", "code")
    r.git("worktree", "add", "-q", "--detach", ".records")
    r.git("checkout", "-q", "--orphan", "records", cwd=r.store)
    r.git("rm", "-rqf", ".", cwd=r.store)
    for rel, text in RECORDS.items():
        (r.store / rel).parent.mkdir(parents=True, exist_ok=True)
        (r.store / rel).write_text(text)
    r.commit("records")
    r.records.symlink_to(r.store)
    return r
