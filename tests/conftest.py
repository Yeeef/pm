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

import transcript
from work_items import items as work_items

# This checkout's records/ link: the real records, wherever this clone keeps its store.
REAL_RECORDS = Path(__file__).resolve().parents[2] / "records"
# The pm the tests run: PM_IMPL=python (the default) runs pm from the package environment the tests run in (make
# test); PM_IMPL=go runs the Go binary at $PM_GO_BIN. A test for one implementation only is marked with it and the
# reason: @pytest.mark.impl("python", reason="…"). The renderer (Repo.pages) is Python's either way.
IMPL = os.environ.get("PM_IMPL", "python")
if IMPL == "python":
    PM = [str(Path(sys.executable).with_name("pm"))]
elif IMPL == "go":
    if not os.access(os.environ.get("PM_GO_BIN", ""), os.X_OK):
        raise RuntimeError(f"PM_IMPL=go runs the Go pm at $PM_GO_BIN, which is not an executable: "
                           f"{os.environ.get('PM_GO_BIN')!r}")
    PM = [os.environ["PM_GO_BIN"]]
else:
    raise RuntimeError(f"PM_IMPL={IMPL!r}: give python or go")
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
    "projects/demo.md": project("Demo", "repo-demo"),
    "projects/old.md": project("Old", "repo-old", outcome="Done: retired."),
    "sprints/demo-1.md": sprint("First", "repo-demo.1"),
    "sprints/demo-2.md": sprint("Second", "repo-demo.2", outcome="Done: shipped.", against="- It works: met."),
    "days/2026-10-01.md": "---\ntype: day\ndate: 2026-10-01\n---\n\n## Today\n\n> What are we chasing today, and why now?\n\nStart.\n",
}

# As bd exports them: every issue has its created_at and updated_at, a closed one its closed_at.
ISSUES = [
    {"id": "repo-demo", "title": "Demo", "status": "open", "issue_type": "epic", "created_at": "2026-10-01T12:00:00Z",
     "updated_at": "2026-10-01T12:00:00Z"},
    {"id": "repo-demo.1", "title": "Sprint 1: First", "status": "open", "issue_type": "epic", "parent": "repo-demo",
     "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"},
    {"id": "repo-demo.1.1", "title": "Done task", "status": "closed", "issue_type": "task", "parent": "repo-demo.1",
     "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T13:00:00Z", "closed_at": "2026-10-01T13:00:00Z"},
    {"id": "repo-demo.1.2", "title": "Ask the owner", "status": "open", "issue_type": "task", "parent": "repo-demo.1",
     "labels": ["human"], "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z",
     "dependencies": [{"issue_id": "repo-demo.1.2", "depends_on_id": "repo-demo.1.1", "type": "blocks"}]},
    {"id": "repo-demo.2", "title": "Sprint 2: Second", "status": "open", "issue_type": "epic", "parent": "repo-demo",
     "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"},
    {"id": "repo-old", "title": "Old", "status": "closed", "issue_type": "epic", "created_at": "2026-09-01T12:00:00Z",
     "updated_at": "2026-09-02T12:00:00Z", "closed_at": "2026-09-02T12:00:00Z"},
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
HEAVY_PM = {"init", "upgrade", "uninstall", "doctor", "push", "service"}  # set a clone up, the pm service


def integration_only(args) -> str | None:
    """What the command `args` starts that only an integration test may, or None for a light call."""
    if isinstance(args, (str, bytes, os.PathLike)) or not args:
        return None
    if str(RENDER_PAGES) in map(str, args):
        return "the whole site rendered (Repo.pages)"
    name, rest = Path(str(args[0])).name, [str(a) for a in args[1:]]
    if rest[:2] == ["-m", "pm.cli"]:  # how pm's hooks and service run pm (hooks.INIT, the service unit)
        name, rest = "pm", rest[2:]
    if name == "git":
        sub = next((a for i, a in enumerate(rest) if not a.startswith("-") and rest[i - 1:i] not in (["-C"], ["-c"])),
                   "")
        if sub in REMOTE_GIT:
            return f"git {sub}"
        if "--bare" in rest:
            return "a bare git repo (git --bare)"
    if name == "pm" and rest:
        if rest[:2] == ["init", "--import-bd"]:  # Go pm's import into its work store only: how Repo seeds Go pm
            return None
        if rest[0] in HEAVY_PM:
            return f"pm {rest[0]}"
        if rest[0] == "prime" and not {"--rules", "--subagent"} & set(rest):
            return "the session-start hook (pm prime runs pm init, where and show)"
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


# Each test's Repo.pm calls, written as one transcript per test under $PM_TRANSCRIPTS/<impl> (default
# pm/.transcripts/<impl>), which a run empties first: the same file from each implementation must be equal.
TRANSCRIPTS = Path(os.environ.get("PM_TRANSCRIPTS") or Path(__file__).resolve().parents[1] / ".transcripts") / IMPL


@pytest.fixture(autouse=True)
def recorded(request, tmp_path: Path):
    transcript.start()
    yield
    paths = {str(tmp_path): "<tmp>", str(tmp_path.resolve()): "<tmp>", str(request.config.pm_home): "<home>",
             str(Path(request.config.pm_home).resolve()): "<home>", str(Path(PM[0]).parent): "<pm-bin>",
             str(REAL_RECORDS.parent): "<checkout>", **{v: f"<{k}>" for k, v in UV_DIRS.items()},
             tempfile.gettempdir(): "<systmp>", str(Path(tempfile.gettempdir()).resolve()): "<systmp>"}
    transcript.write(TRANSCRIPTS, request.node.nodeid, paths)


GO_EXPECTED_FAILURES = Path(__file__).resolve().parent / "go-expected-failures.txt"


def go_expected_failures() -> set[str]:
    """The tests expected to fail on Go pm, as <test file>::<test name>: the list's lines without comments."""
    lines = GO_EXPECTED_FAILURES.read_text().splitlines()
    return {line for line in lines if line.strip() and not line.startswith("#")}


def pytest_collection_modifyitems(config, items):
    """Skip a test marked for another implementation, with the mark's reason. Under PM_IMPL=go, a test on the
    expected-failures list must fail: a listed test that passes fails the run (strict xfail), so the list only
    shrinks."""
    expected = go_expected_failures() if IMPL == "go" else set()
    for item in items:
        if (mark := item.get_closest_marker("impl")) and IMPL not in mark.args:
            item.add_marker(pytest.mark.skip(reason=f"only for {', '.join(mark.args)}: {mark.kwargs['reason']}"))
        elif f"{item.path.name}::{item.name}" in expected:
            item.add_marker(pytest.mark.xfail(strict=True, reason=f"listed in {GO_EXPECTED_FAILURES.name}: Go pm "
                                                                    "does not pass it yet; remove it once it passes"))


# The user's files pm writes outside a repo: Codex's config (pm init's writable roots) and the service units.
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
                XDG_DATA_HOME=str(tmp / "home/.local/share"),
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
        self.base: dict[str, dict] = {}  # the items changes() counts from; the repo fixture marks them once set up
        self.seeded: set[str] = set()  # ids pm did not mint: a transcript keeps them as they are
        self.bd_mark = 0  # the fake bd's calls before the mark, which unchanged() leaves out
        self.imported: dict[str, dict] = {}  # Go pm: the items its store held right after the seeds were imported

    def pm(self, *args: str, text: str = "", stdin: str | None = None,
           cwd: Path | None = None) -> subprocess.CompletedProcess:
        """Run pm; a non-empty text goes in as --text. stdin is closed unless given: only `pm hook` reads it, for the
        hook input JSON. The call goes into the test's transcript with the record files it changed and the store
        export after it."""
        feed = {"stdin": subprocess.DEVNULL} if stdin is None else {"input": stdin}
        argv = [*args, *([f"--text={text}"] if text else [])]
        before = self.record_files()
        res = subprocess.run([*PM, *argv], cwd=cwd or self.root, env=self.env, capture_output=True, text=True, **feed)
        after = self.record_files()
        try:
            export = sorted(self.items().values(), key=lambda i: i["id"])
        except subprocess.CalledProcessError as e:  # a pm whose export fails here (no config, say): that is the record
            export = f"pm export failed ({e.returncode}): {e.stderr}"
        transcript.record({
            "argv": argv, "stdin": stdin, "stdout": res.stdout, "stderr": res.stderr, "exit": res.returncode,
            "records": {p: after[p].decode(errors="replace") if p in after else None
                        for p in sorted(before.keys() | after.keys()) if before.get(p) != after.get(p)},
            "export": export}, roots=[i["id"] for i in export if isinstance(export, list) and i["parent"] is None
                                      and i["id"] not in self.seeded])
        return res

    def git(self, *args: str, cwd: Path | None = None) -> str:
        res = subprocess.run(["git", *args], cwd=cwd or self.root, capture_output=True, text=True)
        if res.returncode:  # git's stderr says why; CalledProcessError's message alone would not show it
            raise AssertionError(f"git {' '.join(args)} exited {res.returncode} in {cwd or self.root}:\n{res.stderr}")
        return res.stdout

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

    def record_files(self) -> dict[str, bytes]:
        """The records store's files by path, its .git aside."""
        return {p.relative_to(self.store).as_posix(): p.read_bytes() for p in sorted(self.store.rglob("*"))
                if p.is_file() and ".git" not in p.relative_to(self.store).parts}

    # Work data, store-neutral: tests read it as work-store items (work_items.py has the fields), never as bd JSON or
    # bd calls. Seeds are given as the issues bd exports, which the work store imports; set_issue and add_issue
    # are the one place a test writes them.

    def items(self) -> dict[str, dict]:
        """Every work-store item by id, as `pm export` gives them: for Python pm, the fake bd's issues mapped; for Go
        pm, its store read by path, so a test that broke the repo's config still reads it."""
        if IMPL == "python":
            return work_items(json.loads(self.state.read_text()))
        res = subprocess.run([*PM, "export", "--store", str(self.root / ".pm/store/work")], cwd=self.root,
                             env=self.env, capture_output=True, text=True, check=True)
        return {i["id"]: i for i in map(json.loads, res.stdout.splitlines())}

    def import_seeds(self) -> None:
        """Go pm: its work store made anew from the fake bd's issues, as `pm init --import-bd` imports a bd export."""
        issues = json.loads(self.state.read_text())
        export = self.state.with_name("bd-export.jsonl")
        export.write_text("".join(json.dumps({"_type": "issue", **i}) + "\n" for i in issues))
        shutil.rmtree(self.root / ".pm/store/work", ignore_errors=True)
        res = subprocess.run([*PM, "init", "--import-bd", str(export)], cwd=self.root, env=self.env,
                             capture_output=True, text=True)
        assert res.returncode == 0, f"the seeds do not import into Go pm's work store: {res.stderr}"
        self.imported = self.items()

    def mark(self) -> None:
        """Count changes() from now on."""
        self.base = self.items()
        self.seeded |= self.base.keys()
        self.bd_mark = len(self.bd_calls()) if IMPL == "python" else 0

    def unchanged(self) -> bool:
        """Nothing at all changed in the work store since the repo was set up or marked, seeds aside: every item
        equal, stamps included; for Python pm also no bd write, a no-op one included."""
        reads = lambda c: (c[:1] in (["list"], ["show"]) or c in (["context", "--json"], ["export"])
                           or (c[:1] == ["comments"] and c[2:] == ["--json"]))
        writes = [c for c in self.bd_calls()[self.bd_mark:] if not reads(c)] if IMPL == "python" else []
        return self.items() == self.base and writes == []

    def changes(self) -> dict[str, dict]:
        """What pm changed in the work store since the repo was set up or marked, seeds aside: for each item it made,
        its non-empty fields; for each it changed, the fields that differ. Store stamps (created_at, updated_at,
        started_at, closed_at, claimed_at) and comment ids are left out; a test that cares asserts them itself."""
        after = self.items()
        assert self.base.keys() <= after.keys(), f"items gone from the store: {sorted(self.base.keys() - after.keys())}"
        out = {}
        for iid, item in after.items():
            new = unstamped(item)
            old = unstamped(self.base[iid]) if iid in self.base else None
            diff = ({k: v for k, v in new.items() if v not in (None, "", [])} if old is None
                    else {k: v for k, v in new.items() if old[k] != v})
            if diff:
                out[iid] = diff
        return out

    def set_issue(self, issue_id: str, **fields) -> None:
        """Seed: change an issue's bd fields."""
        def update(issues):
            for i in issues:
                if i["id"] == issue_id:
                    i.update(fields)
        self.seed(update)
        if self.noms.is_dir():
            with open(self.noms / "journal", "a") as f:
                f.write(json.dumps([issue_id, fields]) + "\n")

    def add_issue(self, issue: dict) -> None:
        """Seed: add an issue as bd exports it."""
        self.seed(lambda issues: issues.append(dict(issue)))

    def seed(self, edit) -> None:
        """Apply a seed edit to the fake bd's issues, and its items to the base changes() counts from, so a seed is
        no change of pm's."""
        if IMPL == "go" and self.items() != self.imported:  # Go pm's store is imported anew from the seeds alone
            raise NotImplementedError("PM_IMPL=go: a seed after Go pm wrote its work store would undo that write; "
                                      "seed before the first pm write")
        issues = json.loads(self.state.read_text())
        before = {i["id"]: json.dumps(i, sort_keys=True) for i in issues}
        edit(issues)
        self.state.write_text(json.dumps(issues))
        if IMPL == "go":
            self.import_seeds()
        now = self.imported if IMPL == "go" else work_items(issues)
        seeded = {i["id"] for i in issues if before.get(i["id"]) != json.dumps(i, sort_keys=True)}
        self.base.update({iid: now[iid] for iid in seeded})
        self.seeded |= seeded

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
        """The calls Python pm made to the fake bd: only for an assertion about Python pm's use of bd."""
        return [json.loads(l) for l in self.log.read_text().splitlines()]

    def snapshot(self) -> dict[str, bytes]:
        """Every file of the main checkout but git's, and but Go pm's gate log, which each open of its work store
        appends to (the pm-go page, Store sharing)."""
        return {p.relative_to(self.root).as_posix(): p.read_bytes()
                for p in sorted(self.root.rglob("*")) if p.is_file() and ".git" not in p.parts
                and p.relative_to(self.root).as_posix() != ".pm/run/work-gate.log"}


STAMPS = {"created_at", "updated_at", "started_at", "closed_at", "claimed_at"}


def unstamped(item: dict) -> dict:
    """An item without the store's stamps and its comments' ids, for Repo.changes()."""
    out = {k: v for k, v in item.items() if k not in STAMPS}
    if out["holder"]:
        out["holder"] = {k: v for k, v in out["holder"].items() if k not in STAMPS}
    out["comments"] = [{k: v for k, v in c.items() if k not in STAMPS and k != "id"} for c in out["comments"]]
    return out


@pytest.fixture
def repo(tmp_path: Path) -> Repo:
    root = tmp_path / "repo"
    root.mkdir()
    r = Repo(root, tmp_path)
    r.git("init", "-q", "-b", "main")
    r.git("config", "user.email", "t@example.com")
    r.git("config", "user.name", "t")
    # git 2.55's commit starts `git maintenance run --auto --detach`, whose worktree-prune deletes a worktree that
    # `git worktree add` is still making ("could not open '.git/worktrees/records/locked'", exit 128)
    r.git("config", "maintenance.auto", "false")
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
    if IMPL == "go":
        r.import_seeds()
    r.mark()
    return r


def pytest_configure(config):
    # before any test module is imported, so module-level environments (test_init's GIT_ENV) get the temp dirs too
    os.environ.update({f"PM_TESTS_REAL_{k}": v for k, v in REAL.items()})
    home = config.pm_home = Path(tempfile.mkdtemp(prefix="pm-tests-home-"))
    os.environ.update(HOME=str(home / "home"), CODEX_HOME=str(home / "codex"), CLAUDE_CONFIG_DIR=str(home / "claude"),
                      XDG_CONFIG_HOME=str(home / "home/.config"), XDG_DATA_HOME=str(home / "home/.local/share"),
                      **UV_DIRS)
    (home / "home").mkdir()
    # the tests' git, without the user's global config: new repos and bare remotes start on main
    (home / "home/.gitconfig").write_text("[init]\n\tdefaultBranch = main\n[user]\n\tname = t\n\temail = t@example.com\n")
    config.addinivalue_line("markers", "integration: starts the pm service, renders the whole site, sets a clone up, "
                                       "reaches a git remote or runs the session-start hook; `make test` skips it, "
                                       "CI and `make test-full` run it")
    config.addinivalue_line("markers", "impl(*impls, reason): a test for these implementations only (PM_IMPL), and why")
    if not os.environ.get("PYTEST_XDIST_WORKER"):  # once per run, before any worker writes
        shutil.rmtree(TRANSCRIPTS, ignore_errors=True)


def pytest_unconfigure(config):
    if hasattr(config, "pm_home"):
        shutil.rmtree(config.pm_home, ignore_errors=True)
