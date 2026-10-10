"""Fixtures: a temp clone whose records store holds a small record set and whose work store holds a few seeded items,
served by a per-test pm service; the pm under test is the Go binary `make go-build` builds at .go/pm."""

from __future__ import annotations

import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pytest

# subprocess.Popen as it is before light_unless_integration replaces it: the repo fixture's per-test pm service is no
# integration test's (the pm-go page, Tests), so it starts through this one.
POPEN = subprocess.Popen

# This checkout: pm's repo.
CHECKOUT = Path(__file__).resolve().parents[1]
# The pm the tests run, and the page renderer Repo.pages runs: both built by `make go-build`, with one version stamped.
BIN = CHECKOUT / ".go"
for _b in ("pm", "render-pages"):
    if not os.access(BIN / _b, os.X_OK):
        raise RuntimeError(f"{BIN / _b} is not an executable: build it with make go-build (make test does)")
PM = [str(BIN / "pm")]
RENDER_PAGES = BIN / "render-pages"
# The version the build reports, which every test repo pins.
VERSION = subprocess.run([*PM, "version"], cwd="/", check=True, capture_output=True, text=True).stdout.strip()
SUBAGENT_RULE = ("Git: commit and push are routine for agents unless your brief says otherwise; only the PR review and "
                 "the merge wait on the owner.")
FAKE_GH = Path(__file__).resolve().parent / "fake_gh.py"
FAKE_SCHED = Path(__file__).resolve().parent / "fake_sched.py"
NO_RELEASE_API = "http://127.0.0.1:9/no-release-api"
FAKE_CLAUDE = Path(__file__).resolve().parent / "fake_claude.py"

# What pm prime prints, written out from the design (the pm rules' hook chunks), not taken from pm's code: prime.md and
# the noun list, cut at RULE_STARTS into chunks of at most CAP characters (Claude Code passes a hook's
# additionalContext inline only up to 10,000 characters), each under a title naming its place and its sections.
RULES = CHECKOUT / "prime.md"
CAP = 10_000
RULE_STARTS = ("# pm rules", "# How")
MACHINERY = {"prime", "hook", "push"}  # what the runtimes and the scheduler call, not agents


def nouns() -> list[str]:
    """pm's commands as pm --help lists them."""
    usage = subprocess.run([*PM, "--help"], cwd="/", check=True, capture_output=True, text=True).stdout
    return re.search(r"\{([a-z,-]+)\}", usage).group(1).split(",")


def commands() -> str:
    return "# Commands\n\n`pm` nouns: " + ", ".join(f"`{n}`" for n in nouns() if n not in MACHINERY) + "."


def rules() -> str:
    return RULES.read_text(encoding="utf-8").strip()


def head() -> str:
    """The rules and the command list, whole and in order: what chunks() cuts."""
    return rules() + "\n\n" + commands()


def chunks() -> list[str]:
    """head() cut at the lines in RULE_STARTS, each chunk under a title line such as "# pm rules (1 of 2): the
    introduction; What — 1. The layers, …"."""
    lines = head().split("\n")
    at = [lines.index(s) for s in RULE_STARTS]
    assert at[0] == 0 and at == sorted(at), f"the chunk headings {RULE_STARTS} are not in order at the top of prime.md"
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


def write_config(root: Path, **settings) -> Path:
    """Write the repo's .pm/config.toml under `root`: this pm's version and the defaults, as overridden."""
    values = {"version": VERSION, "remote": "origin", "main_branch": "main", "port": 8000, **settings}
    path = root / ".pm/config.toml"
    path.parent.mkdir(exist_ok=True)
    (path.parent / ".gitignore").write_text("store/\nrun/\n")
    path.write_text("".join(f"{k} = {v if isinstance(v, int) else json.dumps(v)}\n" for k, v in values.items()))
    return path


def uv_dir(*args: str) -> str:
    return subprocess.run(["uv", "--color", "never", *args], check=True, capture_output=True, text=True).stdout.strip()


# HOME is a temp dir in tests, so pm service install writes its unit there; uv, which the launcher runs for a Python
# pin (test_launch.py), keeps the real cache and Pythons.
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


def stop_services(tmp: Path) -> None:
    """Stop every process the fake supervisor started under `tmp`, and mark them stopped: pm reaches the work store
    only through the clone's service, so from here Repo.items reads it through a service started anew."""
    (tmp / "services-stopped").touch()
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
    if name == "git":
        sub = next((a for i, a in enumerate(rest) if not a.startswith("-") and rest[i - 1:i] not in (["-C"], ["-c"])),
                   "")
        if sub in REMOTE_GIT:
            return f"git {sub}"
        if "--bare" in rest:
            return "a bare git repo (git --bare)"
    if name == "pm" and rest and "--help" not in rest:  # a command's --help starts nothing
        if rest[:2] == ["init", "--import-bd"]:  # the import into the work store alone: how Repo seeds it
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


# The user's files pm writes outside a repo: Codex's config (pm init's writable roots) and the service units.
# pytest_configure points HOME, CODEX_HOME and CLAUDE_CONFIG_DIR at a temp dir for the whole test process, so
# whatever a test runs with the inherited environment (git and the pm hooks it runs, in-process calls) writes there,
# never here; each test then checks that nothing here changed.
# xdist workers start after the controller's pytest_configure, so the user's values come from PM_TESTS_REAL_*.
REAL = {k: os.environ.get(f"PM_TESTS_REAL_{k}", os.environ.get(k, "")) for k in ("HOME", "CODEX_HOME",
                                                                                 "XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR")}
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


def fake_env(tmp: Path, base) -> dict[str, str]:
    """`base` with the fakes first on PATH: gh serving PRs from tmp/gh.json (none at first), claude logging to
    tmp/claude.log, and launchctl, systemctl and crontab logging to tmp/sched.log, so no test installs a real service;
    made on first use in `tmp`, so later calls keep their state and log. HOME is tmp/home, whose .local/bin/pm, second
    on PATH, links the pm under test, as install.sh and pm init put it. CODEX_HOME is tmp/codex, absent until a test
    makes it, so no test reads or edits the user's Codex config."""
    bindir = tmp / "bin"
    if not bindir.exists():
        bindir.mkdir()
        # each runs with this interpreter, whatever python3 the PATH a unit gets holds
        for tool, script in (("gh", FAKE_GH), ("claude", FAKE_CLAUDE), ("launchctl", FAKE_SCHED),
                             ("systemctl", FAKE_SCHED), ("crontab", FAKE_SCHED)):
            (bindir / tool).write_text(f'#!/bin/sh\nFAKE_TOOL={tool} exec "{sys.executable}" "{script}" "$@"\n')
            (bindir / tool).chmod(0o755)
        (tmp / "home/.local/bin").mkdir(parents=True)
        (tmp / "home/.local/bin/pm").symlink_to(PM[0])
        (tmp / "gh.json").write_text("{}")
        (tmp / "issues.json").write_text(json.dumps(ISSUES))
    # a test names its session itself, and its transcripts live under tmp/claude, not the user's
    base = {k: v for k, v in base.items() if k not in ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID",
                                                         "CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN",
                                                         "GH_TOKEN")}
    return dict(base, PATH=f"{bindir}{os.pathsep}{tmp / 'home/.local/bin'}{os.pathsep}{base['PATH']}",
                FAKE_GH_STATE=str(tmp / "gh.json"), CODEX_HOME=str(tmp / "codex"),
                CLAUDE_CONFIG_DIR=str(tmp / "claude"), HOME=str(tmp / "home"), XDG_CONFIG_HOME=str(tmp / "home/.config"),
                XDG_DATA_HOME=str(tmp / "home/.local/share"),
                FAKE_SCHED_LOG=str(tmp / "sched.log"), FAKE_SCHED_STATE=str(tmp / "sched.json"),
                FAKE_CLAUDE_LOG=str(tmp / "claude.log"),
                # pm doctor reads the latest release from GitHub's API: no test reaches it (port 9 refuses at once)
                PM_RELEASE_API=NO_RELEASE_API, **UV_DIRS)


class Repo:
    """A main checkout on branch main, its store at .pm/store/records on branch records, and the records link to it."""

    def __init__(self, root: Path, tmp: Path):
        self.root, self.store, self.records = root, root / ".pm/store/records", root / "records"
        self.issues, self.sched = tmp / "issues.json", tmp / "sched.json"
        self.env = fake_env(tmp, os.environ)
        self.base: dict[str, dict] = {}  # the items changes() counts from; the repo fixture marks them once set up
        self.imported: dict[str, dict] = {}  # the items the store held right after the seeds were imported
        self.tmp = tmp
        self.service: subprocess.Popen | None = None  # the per-test pm service that holds the work store

    def pm(self, *args: str, text: str = "", stdin: str | None = None,
           cwd: Path | None = None) -> subprocess.CompletedProcess:
        """Run pm; a non-empty text goes in as --text. stdin is closed unless given: only `pm hook` reads it, for the
        hook input JSON."""
        feed = {"stdin": subprocess.DEVNULL} if stdin is None else {"input": stdin}
        argv = [*args, *([f"--text={text}"] if text else [])]
        return subprocess.run([*PM, *argv], cwd=cwd or self.root, env=self.env, capture_output=True, text=True, **feed)

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
        """Every page of the site by path, as the pm service renders it from the records and the work store now."""
        res = subprocess.run([str(RENDER_PAGES)], cwd=cwd or self.root, env=self.env, capture_output=True, text=True)
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

    # Work data: tests read it as work-store items, as `pm export` gives them. Seeds are given as the issues bd
    # exports, which `pm init --import-bd` imports; set_issue and add_issue are the one place a test writes them.

    def items(self) -> dict[str, dict]:
        """Every work-store item by id, read by path from outside the repo, so a test that broke the repo's config
        or pins another version still reads it, and the launcher never runs the pin for this read. With the clone's
        service stopped (stop_services), the per-test one is started anew to read it."""
        if (self.tmp / "services-stopped").exists():
            self.stop_service()
            self.start_service()
        res = subprocess.run([*PM, "export", "--store", str(self.root / ".pm/store/work")], cwd=self.root.parent,
                             env=self.env, capture_output=True, text=True, check=True)
        return {i["id"]: i for i in map(json.loads, res.stdout.splitlines())}

    def start_service(self) -> None:
        """Start `pm service run` for this clone on a free port, as the supervisor would, and wait for its work-store
        socket: every pm command reaches the work store only through the service. The fake supervisor stops it when
        a test starts the clone's installed service (fake_sched.py)."""
        if self.service is not None and self.service.poll() is None:
            return
        (self.tmp / "services-stopped").unlink(missing_ok=True)
        sock = self.root / ".pm/run/work.sock"
        sock.unlink(missing_ok=True)  # one a killed service left: the new one removes it too, but only once it starts
        with open(self.tmp / "fixture-service.log", "ab") as log:
            self.service = POPEN([*PM, "service", "run"], cwd=self.root, env=dict(self.env, PORT="0"),
                                 stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
        state = json.loads(self.sched.read_text()) if self.sched.exists() else {"loaded": []}
        state.setdefault("pids", {})["fixture"] = self.service.pid
        self.sched.write_text(json.dumps(state))
        deadline = time.monotonic() + 30
        while not sock.exists():
            if self.service.poll() is not None or time.monotonic() > deadline:
                raise AssertionError("the per-test pm service did not start:\n" +
                                     (self.tmp / "fixture-service.log").read_text())
            time.sleep(0.01)

    def stop_service(self) -> None:
        """Stop the per-test pm service, before a test starts its own `pm service run` for the clone."""
        if self.service is None:
            return
        self.service.terminate()
        self.service.wait(timeout=30)
        self.service = None

    def import_seeds(self) -> None:
        """The work store made anew from the seed issues, as `pm init --import-bd` imports a bd export, through the
        per-test service, restarted on the removed store."""
        issues = json.loads(self.issues.read_text())
        export = self.issues.with_name("bd-export.jsonl")
        export.write_text("".join(json.dumps({"_type": "issue", **i}) + "\n" for i in issues))
        if (self.root / ".pm/store/work").exists():
            self.stop_service()
            shutil.rmtree(self.root / ".pm/store/work")
        self.start_service()
        res = subprocess.run([*PM, "init", "--import-bd", str(export)], cwd=self.root, env=self.env,
                             capture_output=True, text=True)
        assert res.returncode == 0, f"the seeds do not import into the work store: {res.stderr}"
        self.imported = self.items()

    def mark(self) -> None:
        """Count changes() from now on."""
        self.base = self.items()

    def unchanged(self) -> bool:
        """Nothing at all changed in the work store since the repo was set up or marked, seeds aside: every item
        equal, stamps included."""
        return self.items() == self.base

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

    def add_issue(self, issue: dict) -> None:
        """Seed: add an issue as bd exports it."""
        self.seed(lambda issues: issues.append(dict(issue)))

    def seed(self, edit) -> None:
        """Apply a seed edit to the seed issues and import them anew, and take the seeded items into the base
        changes() counts from, so a seed is no change of pm's."""
        if self.items() != self.imported:  # the store is imported anew from the seeds alone
            raise NotImplementedError("a seed after pm wrote its work store would undo that write; seed before the "
                                      "first pm write")
        issues = json.loads(self.issues.read_text())
        before = {i["id"]: json.dumps(i, sort_keys=True) for i in issues}
        edit(issues)
        self.issues.write_text(json.dumps(issues))
        self.import_seeds()
        seeded = {i["id"] for i in issues if before.get(i["id"]) != json.dumps(i, sort_keys=True)}
        self.base.update({iid: self.imported[iid] for iid in seeded})

    def set_pr(self, url: str, state: str, merge: str | None = None) -> None:
        """What the fake gh reports for a PR: its state and, once merged, its merge commit."""
        path = Path(self.env["FAKE_GH_STATE"])
        prs = json.loads(path.read_text())
        prs[url] = {"state": state, "mergeCommit": {"oid": merge} if merge else None}
        path.write_text(json.dumps(prs))

    def set_commit(self, slug: str, sha: str) -> None:
        """A commit the fake gh's `gh api repos/<slug>/commits/<ref>` finds, under its full sha and its 7-character
        prefix."""
        path = Path(self.env["FAKE_GH_STATE"])
        state = json.loads(path.read_text())
        for ref in (sha, sha[:7]):
            state[f"repos/{slug}/commits/{ref}"] = {"sha": sha}
        path.write_text(json.dumps(state))

    def snapshot(self) -> dict[str, bytes]:
        """Every file of the main checkout but git's, and but the index of the Dolt chunk journal, a cache of the
        journal that a read (the pm service's too) may write; items() holds the store's content."""
        return {p.relative_to(self.root).as_posix(): p.read_bytes()
                for p in sorted(self.root.rglob("*")) if p.is_file() and ".git" not in p.parts
                and p.name != "journal.idx"}


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
    r = make_repo(tmp_path)
    yield r
    r.stop_service()


def make_repo(tmp: Path, issues: list[dict] = ISSUES) -> Repo:
    """A clone as pm init leaves it, under tmp: its records store holds RECORDS and its work store the seed `issues`,
    held by the per-test pm service, which the caller stops."""
    root = tmp / "repo"
    root.mkdir()
    r = Repo(root, tmp)
    r.issues.write_text(json.dumps(issues))
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
    r.import_seeds()
    r.mark()
    return r


@pytest.hookimpl(tryfirst=True)
def pytest_configure(config):
    # a short temp root: the pm service's socket, <tmp>/<test>/repo/.pm/run/work.sock, must fit the kernel's 104 bytes,
    # which pytest's default under $TMPDIR passes on macOS; the xdist workers take theirs under the controller's
    if not config.option.basetemp and not os.environ.get("PYTEST_XDIST_WORKER"):
        config.option.basetemp = tempfile.mkdtemp(prefix="pmt", dir="/tmp")
    # before any test module is imported, so module-level environments (test_init's GIT_ENV) get the temp dirs too
    os.environ.update({f"PM_TESTS_REAL_{k}": v for k, v in REAL.items()})
    home = config.pm_home = Path(tempfile.mkdtemp(prefix="pm-tests-home-"))
    # the pm under test first on PATH, for what runs `pm` with the inherited environment: the git hooks pm installs
    os.environ["PATH"] = f"{BIN}{os.pathsep}{os.environ['PATH']}"
    os.environ.update(HOME=str(home / "home"), CODEX_HOME=str(home / "codex"), CLAUDE_CONFIG_DIR=str(home / "claude"),
                      XDG_CONFIG_HOME=str(home / "home/.config"), XDG_DATA_HOME=str(home / "home/.local/share"),
                      **UV_DIRS)
    (home / "home").mkdir()
    # the tests' git, without the user's global config: new repos and bare remotes start on main
    (home / "home/.gitconfig").write_text("[init]\n\tdefaultBranch = main\n[user]\n\tname = t\n\temail = t@example.com\n")
    config.addinivalue_line("markers", "integration: starts the pm service, renders the whole site, sets a clone up, "
                                       "reaches a git remote or runs the session-start hook; `make test` skips it, "
                                       "CI and `make test-full` run it")


def pytest_unconfigure(config):
    if hasattr(config, "pm_home"):
        shutil.rmtree(config.pm_home, ignore_errors=True)
