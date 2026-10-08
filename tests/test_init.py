"""pm init in temp git repos with a local bare remote and the fake bd: a brand-new repo, the refusals, and the
fixtures test_lifecycle shares (an existing repo whose settings and hook files hold other content). Expected file contents are written out here from the
design (pm-product: "Git hooks", "Context: hook-only", "The .pm/ directory"), not taken from pm's code."""

from __future__ import annotations

import json
import os
import re
import socket
import subprocess
import tomllib
from pathlib import Path

import pytest

from conftest import PM, fake_bd_env, write_config
from pm import __version__, hooks

GIT_ENV = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")
pytestmark = pytest.mark.integration  # each test makes a repo with a remote; init starts the service

LAYOUT = ["days", "design", "docs", "postmortems", "projects", "sprints"]
BEADS_HOOK = ("#!/usr/bin/env sh\n# --- BEGIN BEADS INTEGRATION v1.3.1 ---\n# beads' part\n"
              "# --- END BEADS INTEGRATION v1.3.1 ---\n")
PM_FILES = [".pm/config.toml", ".pm/README.md", ".pm/.gitignore", ".claude/settings.json", ".codex/hooks.json",
            ".beads/hooks/post-checkout", ".beads/hooks/pre-commit", ".github/workflows/pm-records-guard.yml",
            ".github/workflows/pm-records-copy.yml", ".gitignore"]
BD_SET = [".beads/config.yaml"]  # what bd changes when pm init sets the agent profile
GITIGNORE_BLOCK = ("# --- BEGIN PM ---\n# each worktree's records/ is a link to the clone's records store\n/records\n"
                   "# per-machine Claude Code settings: pm adds the store's absolute path to them\n"
                   "/.claude/settings.local.json\n# --- END PM ---\n")
RULES = [f"pm prime --rules {n} --hook-json" for n in range(1, len(hooks.STARTS) + 1)]  # one hook per rules chunk
CLAUDE_PM = {"SessionStart": [*RULES, "pm prime --state --hook-json"],
             "SubagentStart": [*RULES, "pm prime --subagent --hook-json"],
             "Stop": ["pm hook owner-request || exit 1", "pm hook stop || exit 1"],
             "PreToolUse": ["pm hook main-checkout || exit 1"]}  # Claude Code only: Codex edits with apply_patch


def section(name: str) -> str:
    return f'# --- BEGIN PM v{__version__} ---\npm hook git-{name} "$@" || exit $?\n# --- END PM ---\n'


def git(cwd: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, check=True, capture_output=True, text=True).stdout


def env(tmp: Path) -> dict[str, str]:
    """The fake bd's environment, with the pm uv tool's bin dir first on PATH, as `uv tool update-shell` puts it: the
    git hooks pm init writes call `pm`."""
    e = fake_bd_env(tmp, GIT_ENV)
    return dict(e, PATH=f"{e['UV_TOOL_BIN_DIR']}{os.pathsep}{e['PATH']}", PORT=str(site_port(tmp)))


def site_port(tmp: Path) -> int:
    """A port free when first asked for, the same for every call in `tmp`: the service pm init starts serves on it,
    as a second clone's would, so it never meets the user's own on the config's 8000."""
    path = tmp / "port"
    if not path.exists():
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            path.write_text(str(s.getsockname()[1]))
    return int(path.read_text())


def pm(cwd: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run([*PM, *args], cwd=cwd, env=env(cwd.parent), capture_output=True, text=True)


def snapshot(root: Path) -> dict[str, bytes]:
    """Every file under `root` but git's and the service's runtime state (.pm/run: its log grows with each probe)."""
    return {p.relative_to(root).as_posix(): p.read_bytes() for p in sorted(root.rglob("*"))
            if p.is_file() and not p.is_symlink() and ".git" not in p.relative_to(root).parts
            and not p.relative_to(root).as_posix().startswith(".pm/run/")}


def pm_free(data: dict) -> dict:
    """Settings data without pm's hooks (a command starting `pm prime` or `pm hook `) and the groups left empty."""
    out = json.loads(json.dumps(data))
    for event, groups in list(out.get("hooks", {}).items()):
        for g in groups:
            g["hooks"] = [h for h in g["hooks"] if not h["command"].startswith(("pm prime", "pm hook "))]
        out["hooks"][event] = [g for g in groups if g["hooks"]]
        if not out["hooks"][event]:
            del out["hooks"][event]
    return out


def commands(data: dict, event: str) -> list[str]:
    return [h["command"] for g in data["hooks"].get(event, []) for h in g["hooks"]]


@pytest.fixture
def new_repo(tmp_path: Path) -> Path:
    """A brand-new repo: one commit on main, pushed to an empty bare remote; no .beads/, no records branch."""
    git(tmp_path, "init", "-q", "--bare", "remote.git")
    repo = tmp_path / "repo"
    git(tmp_path, "init", "-q", "-b", "main", str(repo))
    (repo / "README.md").write_text("hello\n")
    git(repo, "add", "README.md")
    git(repo, "commit", "-qm", "first")
    git(repo, "remote", "add", "origin", str(tmp_path / "remote.git"))
    git(repo, "push", "-q", "-u", "origin", "main")
    return repo


def test_init_bootstraps_a_brand_new_repo(new_repo: Path, tmp_path: Path):
    head = git(new_repo, "rev-parse", "HEAD")
    res = pm(new_repo, "init")
    assert res.returncode == 0, res.stderr
    calls = [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    assert calls[0] == ["init", "--non-interactive"]
    # the records branch: an orphan holding the empty layout, on the remote and checked out as the store
    assert git(new_repo, "ls-remote", "--heads", "origin", "records").strip()
    assert git(new_repo, "ls-tree", "-r", "--name-only", "origin/records").split() == [f"{d}/.gitkeep" for d in LAYOUT]
    assert git(new_repo, "rev-list", "--count", "origin/records").strip() == "1"
    store = new_repo / ".pm/store/records"
    assert git(store, "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (new_repo / "records").is_symlink() and (new_repo / "records").resolve() == store.resolve()
    # the repo's pieces, uncommitted: pm never commits on the code branch
    assert git(new_repo, "rev-parse", "HEAD") == head
    cfg = tomllib.loads((new_repo / ".pm/config.toml").read_text())
    assert cfg == {"version": __version__, "remote": "origin", "main_branch": "main", "port": site_port(tmp_path)}
    assert (new_repo / ".pm/.gitignore").read_text() == "store/\nrun/\n"
    for name in ("post-checkout", "pre-commit"):
        assert (new_repo / f".beads/hooks/{name}").read_text() == BEADS_HOOK + section(name)
    claude = json.loads((new_repo / ".claude/settings.json").read_text())
    assert commands(claude, "SessionStart") == ["bd prime --hook-json", *CLAUDE_PM["SessionStart"]]
    assert all(commands(claude, e) == c for e, c in CLAUDE_PM.items() if e != "SessionStart")
    assert [g["matcher"] for g in claude["hooks"]["PreToolUse"]] == ["Edit|Write|MultiEdit|NotebookEdit"]
    codex = json.loads((new_repo / ".codex/hooks.json").read_text())
    assert all(commands(codex, e) == c for e, c in CLAUDE_PM.items() if e != "PreToolUse")
    assert "PreToolUse" not in codex["hooks"]
    status = [h["statusMessage"] for g in codex["hooks"]["SessionStart"] for h in g["hooks"] if h["command"] in RULES]
    assert status == [f"Loading pm rules ({n} of {len(RULES)})" for n in range(1, len(RULES) + 1)]
    assert "branches: [main]" in (new_repo / ".github/workflows/pm-records-copy.yml").read_text()
    assert (new_repo / ".gitignore").read_text() == GITIGNORE_BLOCK
    status = git(new_repo, "status", "--porcelain", "--untracked-files=all").split("\n")
    untracked = sorted(l[3:] for l in status if l.startswith("?? ") and not l[3:].startswith(".beads/embedded"))
    assert untracked == sorted(PM_FILES + [".beads/config.yaml"])
    # the commit to make names every file the run changed: pm's pieces and what bd wrote
    assert f"git add -- {' '.join(PM_FILES + ['.beads/config.yaml'])} && " in res.stdout, res.stdout
    assert f'git commit -m "Install pm {__version__}"' in res.stdout
    # the pm service, under the fake supervisor in tmp/home: pm init installs it
    sched = [json.loads(l) for l in (tmp_path / "sched.log").read_text().splitlines()]
    assert [c for c in sched if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]], sched
    assert "installed the pm service: " in res.stdout

    exclude = (new_repo / ".git/info/exclude").read_text()
    assert exclude.endswith("\n/.pm/store/\n/.pm/run/\n/records\n/.claude/worktrees/\n"), exclude

    before = snapshot(new_repo)
    again = pm(new_repo, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(new_repo) == before, "a second run changes nothing"
    assert (new_repo / ".git/info/exclude").read_text() == exclude
    assert "git add" not in again.stdout and "already set up" in again.stdout

    # pm's git hooks, once committed: a new worktree gets its records link, and a code branch cannot commit records
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    wt = tmp_path / "wt"
    res = subprocess.run(["git", "worktree", "add", "-q", "-b", "feature", str(wt)], cwd=new_repo, env=env(tmp_path),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == store.resolve(), res.stderr
    (wt / "records").unlink()
    (wt / "records").mkdir()
    (wt / "records/a.md").write_text("x\n")
    git(wt, "add", "-f", "--sparse", "records/a.md")
    res = subprocess.run(["git", "commit", "-qm", "edit records"], cwd=wt, env=env(tmp_path), capture_output=True,
                         text=True)
    assert res.returncode != 0 and "only the records branch may change" in res.stderr, res.stderr
    git(wt, "restore", "--staged", "records/a.md")
    (wt / "code.txt").write_text("code\n")
    git(wt, "add", "code.txt")
    res = subprocess.run(["git", "commit", "-qm", "code"], cwd=wt, env=env(tmp_path), capture_output=True, text=True)
    assert res.returncode == 0, res.stderr


USER_SETTINGS = {"permissions": {"allow": ["Bash(ls:*)"]},
                 "hooks": {"SessionStart": [{"hooks": [{"command": "bd prime --hook-json", "type": "command"}],
                                             "matcher": ""}],
                           "PreToolUse": [{"hooks": [{"command": "./lint.sh", "type": "command"}], "matcher": "Bash"}]},
                 "model": "x"}
USER_CODEX = {"hooks": {"SessionStart": [{"hooks": [{"command": "bd codex-hook SessionStart", "type": "command"}],
                                          "matcher": "startup|resume|clear"}]}}


@pytest.fixture
def existing(tmp_path: Path) -> Path:
    """A clone of a repo that has Beads (its hook files carry content after Beads' section), its own Claude Code and
    Codex settings and .gitignore lines, and a records branch on the remote."""
    git(tmp_path, "init", "-q", "--bare", "remote.git")
    seed = tmp_path / "seed"
    git(tmp_path, "init", "-q", "-b", "records", str(seed))
    (seed / "sprints").mkdir()
    (seed / "sprints/a-1.md").write_text("one\n")
    git(seed, "add", "-A")
    git(seed, "commit", "-qm", "records")
    git(seed, "checkout", "-q", "--orphan", "main")
    git(seed, "rm", "-rqf", ".")
    (seed / ".beads/hooks").mkdir(parents=True)
    for name in ("post-checkout", "pre-commit"):
        (seed / f".beads/hooks/{name}").write_text(BEADS_HOOK + "\n# mine\necho done\n")
        (seed / f".beads/hooks/{name}").chmod(0o755)
    (seed / ".beads/config.yaml").write_text("# beads\n")
    (seed / ".claude").mkdir()
    (seed / ".claude/settings.json").write_text(json.dumps(USER_SETTINGS, indent=2) + "\n")
    (seed / ".codex").mkdir()
    (seed / ".codex/hooks.json").write_text(json.dumps(USER_CODEX, indent=2) + "\n")
    (seed / ".gitignore").write_text("*.log\nbuild/")  # no final newline
    git(seed, "add", "-A")
    git(seed, "commit", "-qm", "code")
    git(seed, "remote", "add", "origin", str(tmp_path / "remote.git"))
    git(seed, "push", "-q", "origin", "main", "records")
    git(tmp_path, "clone", "-q", str(tmp_path / "remote.git"), "clone")
    return tmp_path / "clone"


def test_init_refuses_before_bd_init_runs(new_repo: Path, tmp_path: Path):
    """A refusal comes before bd init, which writes and commits Beads' files: the repo is left as it was."""
    (new_repo / ".claude").mkdir()
    (new_repo / ".claude/settings.json").write_text(json.dumps(USER_SETTINGS))
    head, before = git(new_repo, "rev-parse", "HEAD"), snapshot(new_repo)
    res = pm(new_repo, "init")
    assert res.returncode != 0 and ".claude/settings.json is not laid out as pm writes JSON" in res.stderr, res.stderr
    assert ["init", "--non-interactive"] not in [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    assert snapshot(new_repo) == before and git(new_repo, "rev-parse", "HEAD") == head


def test_init_refuses_a_held_site_port_before_writing_anything(new_repo: Path, tmp_path: Path):
    """PORT names a port a server that is not pm holds: pm init refuses before bd init, the records branch or any
    file, and names a free port; without PORT a new repo's config gets that first free port from 8000 up."""
    with socket.socket() as held:
        held.bind(("127.0.0.1", 0))
        held.listen()
        port = held.getsockname()[1]
        before = snapshot(new_repo)
        res = subprocess.run([*PM, "init"], cwd=new_repo, env=dict(env(tmp_path), PORT=str(port)),
                             capture_output=True, text=True)
        assert res.returncode == 1, res.stdout
        said = re.search(rf"the site port :{port} is held by a process that does not answer HTTP, so the pm service "
                         r"could not serve there; pm init wrote nothing\. Run PORT=(\d+) pm init \(a free port\)",
                         res.stderr)
        assert said, res.stderr
        assert snapshot(new_repo) == before and not (new_repo / ".beads").exists()
        assert ["init", "--non-interactive"] not in [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
        assert not git(new_repo, "ls-remote", "--heads", "origin", "records").strip()
        assert not (tmp_path / "sched.log").exists(), "no service"
        free = int(said.group(1))
        assert free >= 8000 and free != port
        no_port = {k: v for k, v in env(tmp_path).items() if k != "PORT"}
        res = subprocess.run([*PM, "init"], cwd=new_repo, env=no_port, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert tomllib.loads((new_repo / ".pm/config.toml").read_text())["port"] == free
    assert f"serving http://localhost:{free} " in res.stdout, res.stdout


def test_a_pm_from_a_local_checkout_refuses_to_install_or_check_the_tool(new_repo: Path, tmp_path: Path):
    """This pm runs from a local checkout (PYTHONPATH no longer names its git build), which cannot install the tool,
    and the tool, from git, is never its build: init, service install, doctor and where say how to run pm as the
    tool instead of naming an install that would change nothing, and init says it before it changes anything."""
    local = {k: v for k, v in env(tmp_path).items() if k != "PYTHONPATH"}  # env() makes tmp's fakes, the tool too
    said = (f"pm {__version__} here runs from file://", ", not from git, and a local checkout cannot install or check "
            f'the pm uv tool; run pm as the tool (uv tool install "git+https://github.com/Yeeef/yeeef-agents@pm-v'
            f'{__version__}#subdirectory=pm", then pm init), or once with uvx --from "git+')

    def run(*args: str) -> subprocess.CompletedProcess:
        return subprocess.run([*PM, *args], cwd=new_repo, env=local, capture_output=True, text=True)

    before = snapshot(new_repo)
    res = run("init")
    assert res.returncode == 1 and all(p in res.stderr for p in said), res.stderr
    assert snapshot(new_repo) == before and not (tmp_path / "sched.log").exists(), "no bd init, no service"
    assert pm(new_repo, "init").returncode == 0, "the same checkout as the tool's git build installs"
    units = sorted((tmp_path / "home").rglob("*.plist")) + sorted((tmp_path / "home").rglob("*.service"))
    unit_bytes = [u.read_bytes() for u in units]
    res = run("service", "install")
    assert res.returncode == 1 and all(p in res.stderr for p in said), res.stderr
    assert units and [u.read_bytes() for u in units] == unit_bytes
    res = run("doctor")
    assert res.returncode == 1 and all(p in res.stdout for p in said) and "stale" not in res.stdout, res.stdout
    res = run("where")
    assert "unchecked: " in res.stdout and all(p in res.stdout for p in said) and "stale" not in res.stdout, res.stdout


def test_init_in_a_worktree_sets_it_up_when_mains_pin_differs(repo):
    """The main checkout pins another pm: the worktree's own setup (records link, sparse checkout) still runs, and
    only the pm uv tool and the service, which follow main's pin, are refused."""
    wt = repo.root.parent / "feature"
    repo.git("worktree", "add", "-q", "--no-checkout", "-b", "feature", str(wt))
    repo.git("reset", "-q", "--hard", cwd=wt)
    write_config(repo.root, version="9.9.9")  # main's checkout moved its pin, uncommitted
    res = repo.pm("init", cwd=wt)
    assert res.returncode == 1, res.stdout
    assert f"error: the main checkout {repo.root} pins pm 9.9.9, and the pm service and the one pm uv tool follow it" \
        in res.stderr and "pm init set up this worktree and left the pm uv tool and the service alone:" in res.stderr
    assert f"linked {wt / 'records'} -> {repo.store}" in res.stderr, res.stderr
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == repo.store.resolve()
    assert "!/records/" in repo.git("sparse-checkout", "list", cwd=wt).split()
    assert not (repo.root.parent / "sched.log").exists(), "no service"


def test_init_in_a_worktree_of_a_branch_without_config_refuses(repo):
    """A linked worktree on a branch cut before pm was installed: pm init does not install the repo's files there."""
    wt = repo.worktree("old")
    repo.git("rm", "-rq", ".pm", cwd=wt)
    repo.git("commit", "-qm", "before pm", cwd=wt)
    before = snapshot(wt)
    res = repo.pm("init", cwd=wt)
    assert res.returncode == 1, res.stdout
    assert ("error: this worktree's branch has no .pm/config.toml: it was cut before pm was installed in this repo, "
            f"and pm init installs the repo's files only in the main checkout; merge the main branch into this one, "
            f"or run pm init in the main checkout {repo.root}; pm init wrote nothing") in res.stderr, res.stderr
    assert snapshot(wt) == before and not (wt / "records").exists() and not (wt / ".beads").exists()
    assert ["init", "--non-interactive"] not in [json.loads(l) for l in repo.log.read_text().splitlines()] \
        if repo.log.exists() else True
