"""pm init in temp git repos with a local bare remote and the fake bd: a brand-new repo, an existing one whose
settings and hook files hold other content, and the refusals. Expected file contents are written out here from the
design (pm-product: "Git hooks", "Context: hook-only", "The .pm/ directory"), not taken from pm's code."""

from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import tomllib
from pathlib import Path

import pytest

from conftest import PM, fake_bd_env
from pm import __version__

GIT_ENV = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")
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
CLAUDE_PM = {"SessionStart": ["pm prime --rules --hook-json", "pm prime --state --hook-json"],
             "SubagentStart": ["pm prime --subagent --hook-json"],
             "Stop": ["pm hook owner-request || exit 1", "pm hook stop || exit 1"]}


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
    return {p.relative_to(root).as_posix(): p.read_bytes() for p in sorted(root.rglob("*"))
            if p.is_file() and not p.is_symlink() and ".git" not in p.relative_to(root).parts}


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
    assert cfg == {"version": __version__, "remote": "origin", "main_branch": "main", "port": 8000}
    assert (new_repo / ".pm/.gitignore").read_text() == "store/\nrun/\n"
    for name in ("post-checkout", "pre-commit"):
        assert (new_repo / f".beads/hooks/{name}").read_text() == BEADS_HOOK + section(name)
    claude = json.loads((new_repo / ".claude/settings.json").read_text())
    assert commands(claude, "SessionStart") == ["bd prime --hook-json", *CLAUDE_PM["SessionStart"]]
    assert all(commands(claude, e) == c for e, c in CLAUDE_PM.items() if e != "SessionStart")
    codex = json.loads((new_repo / ".codex/hooks.json").read_text())
    assert all(commands(codex, e) == c for e, c in CLAUDE_PM.items())
    assert "branches: [main]" in (new_repo / ".github/workflows/pm-records-copy.yml").read_text()
    assert (new_repo / ".gitignore").read_text() == GITIGNORE_BLOCK
    status = git(new_repo, "status", "--porcelain", "--untracked-files=all").split("\n")
    untracked = sorted(l[3:] for l in status if l.startswith("?? ") and not l[3:].startswith(".beads/embedded"))
    assert untracked == sorted(PM_FILES + [".beads/config.yaml"])
    # the commit to make names every file the run changed: pm's pieces and what bd wrote
    assert f"git add -- {' '.join(PM_FILES + ['.beads/config.yaml'])} && " in res.stdout, res.stdout
    assert f'git commit -m "Install pm {__version__}"' in res.stdout
    # the pm service, under the fake supervisor in tmp/home: pm init installs it, pm setup never does
    sched = [json.loads(l) for l in (tmp_path / "sched.log").read_text().splitlines()]
    assert [c for c in sched if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]], sched
    assert "installed the pm service: " in res.stdout

    before = snapshot(new_repo)
    again = pm(new_repo, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(new_repo) == before, "a second run changes nothing"
    assert "git add" not in again.stdout and "already set up" in again.stdout


def test_init_hooks_set_up_a_new_worktree_and_guard_records(new_repo: Path, tmp_path: Path):
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    wt = tmp_path / "wt"
    res = subprocess.run(["git", "worktree", "add", "-q", "-b", "feature", str(wt)], cwd=new_repo, env=env(tmp_path),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == (new_repo / ".pm/store/records").resolve(), res.stderr
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


def test_init_site_url_is_written_to_the_config(new_repo: Path):
    res = pm(new_repo, "init", "--site-url", "https://pm.example.com/")
    assert res.returncode == 0, res.stderr
    assert tomllib.loads((new_repo / ".pm/config.toml").read_text())["site_url"] == "https://pm.example.com"
    where = pm(new_repo, "where").stdout
    assert next(l for l in where.splitlines() if l.startswith("site ")).split()[1] == "https://pm.example.com"
    res = pm(new_repo, "init", "--site-url", "")
    assert res.returncode == 0 and "site URL cleared" in res.stdout, res.stderr
    where = pm(new_repo, "where").stdout
    assert next(l for l in where.splitlines() if l.startswith("site ")).split()[1] == f"http://localhost:{site_port(new_repo.parent)}"


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


def test_init_keeps_what_is_not_pms(existing: Path, tmp_path: Path):
    before = snapshot(existing)
    res = pm(existing, "init")
    assert res.returncode == 0, res.stderr
    calls = [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    assert ["init", "--non-interactive"] not in calls, "Beads is there; bd init must not run"
    assert f"git add -- {' '.join(PM_FILES)} .beads/config.yaml && " in res.stdout, "the agent profile bd set is listed"
    for name in ("post-checkout", "pre-commit"):
        assert (existing / f".beads/hooks/{name}").read_text() == BEADS_HOOK + section(name) + "\n# mine\necho done\n"
    for rel, user in ((".claude/settings.json", USER_SETTINGS), (".codex/hooks.json", USER_CODEX)):
        text = (existing / rel).read_text()
        data = json.loads(text)
        assert pm_free(data) == user and text == json.dumps(data, indent=2) + "\n", rel
    claude = json.loads((existing / ".claude/settings.json").read_text())
    assert all(commands(claude, e)[-len(c):] == c for e, c in CLAUDE_PM.items())
    assert (existing / ".gitignore").read_text() == "*.log\nbuild/\n" + GITIGNORE_BLOCK
    assert git(existing / ".pm/store/records", "ls-files").split() == ["sprints/a-1.md"]
    after = snapshot(existing)
    assert all(after[k] == v for k, v in before.items() if k not in PM_FILES + BD_SET), "files pm does not manage are kept"
    again = pm(existing, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(existing) == after and "git add" not in again.stdout


def test_init_refuses_another_hooks_path(existing: Path):
    git(existing, "config", "core.hooksPath", ".husky")
    before = snapshot(existing)
    res = pm(existing, "init")
    assert res.returncode != 0 and "core.hooksPath is .husky" in res.stderr, res.stderr
    assert snapshot(existing) == before and not (existing / ".pm").exists()


def test_init_refuses_settings_it_would_reformat(existing: Path):
    path = existing / ".claude/settings.json"
    path.write_text(json.dumps(USER_SETTINGS))  # one line: rewriting it would change lines that are not pm's
    before = snapshot(existing)
    res = pm(existing, "init")
    assert res.returncode != 0 and ".claude/settings.json is not laid out as pm writes JSON" in res.stderr, res.stderr
    assert snapshot(existing) == before


def test_init_refuses_before_bd_init_runs(new_repo: Path, tmp_path: Path):
    """A refusal comes before bd init, which writes and commits Beads' files: the repo is left as it was."""
    (new_repo / ".claude").mkdir()
    (new_repo / ".claude/settings.json").write_text(json.dumps(USER_SETTINGS))
    head, before = git(new_repo, "rev-parse", "HEAD"), snapshot(new_repo)
    res = pm(new_repo, "init")
    assert res.returncode != 0 and ".claude/settings.json is not laid out as pm writes JSON" in res.stderr, res.stderr
    assert ["init", "--non-interactive"] not in [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    assert snapshot(new_repo) == before and git(new_repo, "rev-parse", "HEAD") == head


def test_init_refuses_without_the_pm_uv_tool_it_cannot_install(new_repo: Path, tmp_path: Path):
    """This pm runs from a local checkout, which has no source to install the tool from: init names the command,
    before it changes anything."""
    env(tmp_path)  # makes tmp's fakes, the tool among them
    shutil.rmtree(tmp_path / "uv/tools/pm")
    before = snapshot(new_repo)
    res = pm(new_repo, "init")
    assert res.returncode == 1, res
    assert 'was not installed from git; install the tool with uv tool install "git+https://github.com/Yeeef' in res.stderr
    assert snapshot(new_repo) == before and not (tmp_path / "sched.log").exists(), "no bd init, no service"
