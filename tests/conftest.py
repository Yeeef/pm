"""Fixtures: a temp clone whose records store holds a small record set, and a fake `bd` serving a JSON issue list."""

from __future__ import annotations

import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
from pathlib import Path

import pytest

from pm import __version__

# This checkout's records/ link: the real records, wherever this clone keeps its store.
REAL_RECORDS = Path(__file__).resolve().parents[2] / "records"
# pm and the renderer from the package environment the tests run in (make test).
PM = [str(Path(sys.executable).with_name("pm"))]
FAKE_BD = Path(__file__).resolve().parent / "fake_bd.py"
FAKE_GH = Path(__file__).resolve().parent / "fake_gh.py"
FAKE_SCHED = Path(__file__).resolve().parent / "fake_sched.py"
FAKE_CLAUDE = Path(__file__).resolve().parent / "fake_claude.py"
RENDER_PAGES = Path(__file__).resolve().parent / "render_pages.py"


def write_config(root: Path, **settings) -> Path:
    """Write the repo's .pm/config.toml under `root`: this pm's version and the defaults, as overridden."""
    values = {"version": __version__, "remote": "origin", "main_branch": "main", "port": 8000, **settings}
    path = root / ".pm/config.toml"
    path.parent.mkdir(exist_ok=True)
    (path.parent / ".gitignore").write_text("store/\nrun/\n")
    path.write_text("".join(f"{k} = {v if isinstance(v, int) else json.dumps(v)}\n" for k, v in values.items()))
    return path


def uv_dir(*args: str) -> str:
    return subprocess.run(["uv", "--color", "never", *args], check=True, capture_output=True, text=True).stdout.strip()


# HOME is a temp dir in tests, so pm service install writes its unit there; uv keeps the real cache and Pythons.
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


# the git commit the tests' pm says it was built from: pm refuses to install or check the tool from a pm not from git
TEST_SOURCE = {"url": "https://github.com/Yeeef/yeeef-agents", "subdirectory": "pm",
               "vcs_info": {"vcs": "git", "commit_id": "c0ffee" + "0" * 34}}


def git_build(site: Path) -> None:
    """A dist-info under `site` naming TEST_SOURCE (PEP 610), which a pm with `site` first on PYTHONPATH reads as its
    own: the tests' pm, an editable install of this checkout, then counts as a build from git."""
    dist = site / f"pm-{__version__}.dist-info"
    dist.mkdir(parents=True, exist_ok=True)
    (dist / "METADATA").write_text(f"Metadata-Version: 2.1\nName: pm\nVersion: {__version__}\n")
    (dist / "direct_url.json").write_text(json.dumps(TEST_SOURCE))


def install_tool(tools: Path, bindir: Path) -> None:
    """The pm uv tool as `uv tool install` lays it out under UV_TOOL_DIR and UV_TOOL_BIN_DIR, running the pm the
    tests run as the git build git_build names, so the service's unit and the hooks' `pm` run this checkout's code
    as the build the tests' pm is."""
    env_bin, site = tools / "pm/bin", tools / "pm/site"
    env_bin.mkdir(parents=True, exist_ok=True)
    git_build(site)
    for name, target in (("python", sys.executable), ("pm", PM[0])):
        (env_bin / name).write_text(f'#!/bin/sh\nPYTHONPATH="{site}" exec "{target}" "$@"\n')
        (env_bin / name).chmod(0o755)
    bindir.mkdir(parents=True, exist_ok=True)
    if not (bindir / "pm").is_symlink():
        (bindir / "pm").symlink_to(env_bin / "pm")


def stop_services(tmp: Path) -> None:
    """Stop every process the fake supervisor started under `tmp`."""
    path = tmp / "sched.json"
    if not path.exists():
        return
    for pid in json.loads(path.read_text()).get("pids", {}).values():
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass


# What only an `integration` test may start: `make test` runs the rest while agents work, CI runs both sets.
REMOTE_GIT = {"clone", "fetch", "pull", "push", "ls-remote"}  # a clone, or a remote reached
HEAVY_PM = {"init", "setup", "upgrade", "uninstall", "doctor", "push", "service"}  # set a clone up, the pm service


def integration_only(args) -> str | None:
    """What the command `args` starts that only an integration test may, or None for a light call."""
    if isinstance(args, (str, bytes, os.PathLike)) or not args:
        return None
    if str(RENDER_PAGES) in map(str, args):
        return "the whole site rendered (Repo.pages)"
    name, rest = Path(str(args[0])).name, [str(a) for a in args[1:]]
    if rest[:2] == ["-m", "pm.cli"]:  # how pm's hooks and service run pm (hooks.SETUP, the service unit)
        name, rest = "pm", rest[2:]
    if name == "git":
        sub = next((a for i, a in enumerate(rest) if not a.startswith("-") and rest[i - 1:i] not in (["-C"], ["-c"])),
                   "")
        if sub in REMOTE_GIT:
            return f"git {sub}"
        if "--bare" in rest:
            return "a bare git repo (git --bare)"
    if name == "pm" and rest:
        if rest[0] in HEAVY_PM:
            return f"pm {rest[0]}"
        if rest[0] == "prime" and not {"--rules", "--subagent"} & set(rest):
            return "the session-start hook (pm prime runs pm setup, where and show)"
    return None


@pytest.fixture(autouse=True)
def light_unless_integration(request, monkeypatch):
    """Fail a test not marked `integration` once it starts what integration_only names. Autouse fixtures set up
    first, so this covers the test's other fixtures too; pytest.fail is a BaseException, so no `except` swallows it."""
    if request.node.get_closest_marker("integration"):
        return
    test = request.node.nodeid

    class Light(subprocess.Popen):  # subprocess.run starts its process through subprocess.Popen too
        def __init__(self, args, *rest, **kwargs):
            if what := integration_only(args):
                pytest.fail(f"{test} starts {what}: mark it @pytest.mark.integration, so `make test` leaves it to CI "
                            "and `make test-full`", pytrace=False)
            super().__init__(args, *rest, **kwargs)

    monkeypatch.setattr(subprocess, "Popen", Light)


@pytest.fixture(autouse=True)
def no_service_left(tmp_path: Path):
    yield
    stop_services(tmp_path)


# The user's files pm writes outside a repo: Codex's config (pm setup's writable roots) and the service units.
# pytest_configure points HOME, CODEX_HOME and CLAUDE_CONFIG_DIR at a temp dir for the whole test process, so
# whatever a test runs with the inherited environment (git and the pm hooks it runs, in-process calls) writes there,
# never here; each test then checks that nothing here changed.
# xdist workers start after the controller's pytest_configure, so the user's values come from PM_TESTS_REAL_*.
REAL = {k: os.environ.get(f"PM_TESTS_REAL_{k}", os.environ.get(k, "")) for k in ("HOME", "CODEX_HOME",
                                                                                 "XDG_CONFIG_HOME")}
REAL_HOME = Path(REAL["HOME"])
REAL_CODEX_CONFIG = Path(REAL["CODEX_HOME"] or REAL_HOME / ".codex") / "config.toml"
REAL_UNITS = (REAL_HOME / "Library/LaunchAgents",
              Path(REAL["XDG_CONFIG_HOME"] or REAL_HOME / ".config") / "systemd/user")


def real_state() -> tuple:
    codex = REAL_CODEX_CONFIG.read_bytes() if REAL_CODEX_CONFIG.is_file() else None
    return codex, sorted(str(p) for d in REAL_UNITS if d.is_dir() for p in d.glob("*pm.*"))


REAL_BEFORE = real_state()


@pytest.fixture(autouse=True)
def real_home_untouched():
    yield
    after = real_state()
    assert after[0] == REAL_BEFORE[0], f"a test changed the user's {REAL_CODEX_CONFIG}"
    assert after[1] == REAL_BEFORE[1], f"a test added or removed the user's pm service units: {after[1]}"


def fake_bd_env(tmp: Path, base) -> dict[str, str]:
    """`base` with the fake bd and gh first on PATH, bd serving ISSUES from tmp/bd.json and logging calls to
    tmp/bd.log, gh serving PRs from tmp/gh.json (none at first), claude a fake logging to tmp/claude.log; made on first use in `tmp`, so later calls keep
    their state and log. CODEX_HOME is tmp/codex, absent until a test makes it, so no test reads or edits the
    user's Codex config. HOME is tmp/home and launchctl, systemctl and crontab are fakes logging to tmp/sched.log,
    so no test installs a real service; UV_TOOL_DIR and UV_TOOL_BIN_DIR are under tmp/uv, which holds the pm uv
    tool (install_tool), so no test reads or installs the user's tools; PYTHONPATH makes pm the tool's git build."""
    bindir = tmp / "bin"
    if not bindir.exists():
        bindir.mkdir()
        # each runs with this interpreter, whatever python3 the PATH a unit gets holds
        for tool, script in (("bd", FAKE_BD), ("gh", FAKE_GH), ("claude", FAKE_CLAUDE), ("launchctl", FAKE_SCHED),
                             ("systemctl", FAKE_SCHED), ("crontab", FAKE_SCHED)):
            (bindir / tool).write_text(f'#!/bin/sh\nFAKE_TOOL={tool} exec "{sys.executable}" "{script}" "$@"\n')
            (bindir / tool).chmod(0o755)
        (tmp / "home").mkdir()
        (tmp / "gh.json").write_text("{}")
        (tmp / "bd.json").write_text(json.dumps(ISSUES))
        (tmp / "bd.log").write_text("")
        install_tool(tmp / "uv/tools", tmp / "uv/bin")
    # a test names its session itself, and its transcripts live under tmp/claude, not the user's
    base = {k: v for k, v in base.items() if k not in ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID",
                                                         "CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN")}
    return dict(base, PATH=f"{bindir}{os.pathsep}{base['PATH']}", FAKE_BD_STATE=str(tmp / "bd.json"),
                FAKE_BD_LOG=str(tmp / "bd.log"), FAKE_GH_STATE=str(tmp / "gh.json"), CODEX_HOME=str(tmp / "codex"),
                CLAUDE_CONFIG_DIR=str(tmp / "claude"), HOME=str(tmp / "home"), XDG_CONFIG_HOME=str(tmp / "home/.config"),
                FAKE_SCHED_LOG=str(tmp / "sched.log"), FAKE_SCHED_STATE=str(tmp / "sched.json"),
                FAKE_CLAUDE_LOG=str(tmp / "claude.log"), UV_TOOL_DIR=str(tmp / "uv/tools"),
                UV_TOOL_BIN_DIR=str(tmp / "uv/bin"), PYTHONPATH=str(tmp / "uv/tools/pm/site"), **UV_DIRS)


class Repo:
    """A main checkout on branch main, its store at .pm/store/records on branch records, and the records link to it."""

    def __init__(self, root: Path, tmp: Path):
        self.root, self.store, self.records = root, root / ".pm/store/records", root / "records"
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

    def pages(self, cwd: Path | None = None) -> dict[str, str]:
        """Every page of the site by path, as the pm service renders it from the records and Beads now."""
        res = subprocess.run([sys.executable, str(RENDER_PAGES)], cwd=cwd or self.root, env=self.env,
                             capture_output=True, text=True)
        assert res.returncode == 0, res.stderr
        return json.loads(res.stdout)

    def page(self, path: str) -> str:
        return self.pages()[path]

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
        """Make the embedded Dolt store the pm service watches: a manifest and a journal that every bd write (fake bd or
        set_issue) grows, as Dolt's chunk journal does."""
        (self.noms / "oldgen").mkdir(parents=True, exist_ok=True)  # bd bootstrap may have made it
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
    (root / ".gitignore").write_text("/records\n")
    write_config(root)
    r.git("add", ".gitignore", ".pm")
    r.git("commit", "-qm", "code")
    r.git("worktree", "add", "-q", "--detach", ".pm/store/records")
    r.git("checkout", "-q", "--orphan", "records", cwd=r.store)
    r.git("rm", "-rqf", ".", cwd=r.store)
    for rel, text in RECORDS.items():
        (r.store / rel).parent.mkdir(parents=True, exist_ok=True)
        (r.store / rel).write_text(text)
    r.commit("records")
    r.records.symlink_to(r.store)
    return r


def pytest_configure(config):
    # before any test module is imported, so module-level environments (test_init's GIT_ENV) get the temp dirs too
    os.environ.update({f"PM_TESTS_REAL_{k}": v for k, v in REAL.items()})
    home = config.pm_home = Path(tempfile.mkdtemp(prefix="pm-tests-home-"))
    os.environ.update(HOME=str(home / "home"), CODEX_HOME=str(home / "codex"), CLAUDE_CONFIG_DIR=str(home / "claude"),
                      XDG_CONFIG_HOME=str(home / "home/.config"), **UV_DIRS)
    (home / "home").mkdir()
    # the tests' git, without the user's global config: new repos and bare remotes start on main
    (home / "home/.gitconfig").write_text("[init]\n\tdefaultBranch = main\n[user]\n\tname = t\n\temail = t@example.com\n")
    config.addinivalue_line("markers", "integration: starts the pm service, renders the whole site, sets a clone up, "
                                       "reaches a git remote or runs the session-start hook; `make test` skips it, "
                                       "CI and `make test-full` run it")


def pytest_unconfigure(config):
    if hasattr(config, "pm_home"):
        shutil.rmtree(config.pm_home, ignore_errors=True)
