"""pm init in temp git repos with a local bare remote: a brand-new repo, the refusals, and the
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

from conftest import PM, RULE_STARTS, VERSION, fake_env, write_config

GIT_ENV = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")
pytestmark = pytest.mark.integration  # each test makes a repo with a remote; init starts the service

LAYOUT = ["days", "design", "docs", "postmortems", "projects", "sprints"]
BEADS_HOOK = ("#!/usr/bin/env sh\n# --- BEGIN BEADS INTEGRATION v1.3.1 ---\n# beads' part\n"
              "# --- END BEADS INTEGRATION v1.3.1 ---\n")
HOOKS = ".pm/hooks"  # pm's own git hook files, which core.hooksPath names
PM_FILES = [".pm/config.toml", ".pm/README.md", ".pm/.gitignore", ".claude/settings.json", ".codex/hooks.json",
            f"{HOOKS}/post-checkout", ".gitignore"]
GITIGNORE_BLOCK = ("# --- BEGIN PM ---\n# each worktree's records/ is a link to the clone's records store\n/records\n"
                   "# per-machine Claude Code settings: pm adds the store's absolute path to them\n"
                   "/.claude/settings.local.json\n# --- END PM ---\n")
RULES = [f"pm prime --rules {n} --hook-json" for n in range(1, len(RULE_STARTS) + 1)]  # one hook per rules chunk
CLAUDE_PM = {"SessionStart": [*RULES, "pm prime --state --hook-json"],
             "SubagentStart": [*RULES, "pm prime --subagent --hook-json"],
             "Stop": ["pm hook owner-request || exit 1", "pm hook stop || exit 1"]}


def section(name: str) -> str:
    return f'# --- BEGIN PM v{VERSION} ---\npm hook git-{name} "$@" || exit $?\n# --- END PM ---\n'


def git(cwd: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, check=True, capture_output=True, text=True).stdout


def env(tmp: Path) -> dict[str, str]:
    """The fakes' environment, the installed pm on PATH: the git hooks pm init writes call `pm`."""
    return dict(fake_env(tmp, GIT_ENV), PORT=str(site_port(tmp)))


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
    # the work store, created and pushed to the remote's refs/pm/work
    assert git(new_repo, "ls-remote", "origin", "refs/pm/work").strip()
    assert "created the work store at " in res.stdout, res.stdout
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
    assert cfg == {"version": VERSION, "remote": "origin", "main_branch": "main", "port": site_port(tmp_path)}
    assert (new_repo / ".pm/.gitignore").read_text() == "store/\nrun/\n"
    assert (new_repo / f"{HOOKS}/post-checkout").read_text() == "#!/usr/bin/env sh\n" + section("post-checkout")
    assert not (new_repo / f"{HOOKS}/pre-commit").exists() and not (new_repo / ".github").exists()
    assert git(new_repo, "config", "core.hooksPath").strip() == str(new_repo / HOOKS)
    claude = json.loads((new_repo / ".claude/settings.json").read_text())
    assert all(commands(claude, e) == c for e, c in CLAUDE_PM.items())
    codex = json.loads((new_repo / ".codex/hooks.json").read_text())
    assert all(commands(codex, e) == c for e, c in CLAUDE_PM.items())
    status = [h["statusMessage"] for g in codex["hooks"]["SessionStart"] for h in g["hooks"] if h["command"] in RULES]
    assert status == [f"Loading pm rules ({n} of {len(RULES)})" for n in range(1, len(RULES) + 1)]
    assert (new_repo / ".gitignore").read_text() == GITIGNORE_BLOCK
    status = git(new_repo, "status", "--porcelain", "--untracked-files=all").split("\n")
    untracked = sorted(l[3:] for l in status if l.startswith("?? ") and not l[3:].startswith(".beads/embedded"))
    assert untracked == sorted(PM_FILES)
    # the commit to make names every file the run changed
    assert f"git add -- {' '.join(PM_FILES)} && " in res.stdout, res.stdout
    assert f'git commit -m "Install pm {VERSION}"' in res.stdout
    # the pm service, under the fake supervisor in tmp/home: pm init installs it
    sched = [json.loads(l) for l in (tmp_path / "sched.log").read_text().splitlines()]
    assert [c for c in sched if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]], sched
    assert "installed the pm service: " in res.stdout

    exclude = (new_repo / ".git/info/exclude").read_text()
    assert exclude.endswith("\n/.pm/store/\n/.pm/run/\n/records\n"), exclude

    before = snapshot(new_repo)
    again = pm(new_repo, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(new_repo) == before, "a second run changes nothing"
    assert (new_repo / ".git/info/exclude").read_text() == exclude
    assert "git add" not in again.stdout and "already set up" in again.stdout

    # pm's git hooks, once committed: a new worktree gets its records link and no sparse checkout, and the link is
    # git-ignored, so a code-branch commit of everything leaves records/ out
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    wt = tmp_path / "wt"
    res = subprocess.run(["git", "worktree", "add", "-q", "-b", "feature", str(wt)], cwd=new_repo, env=env(tmp_path),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == store.resolve(), res.stderr
    for tree in (new_repo, wt):
        assert git(tree, "config", "--get", "--default=", "core.sparseCheckout").strip() == ""
    (wt / "code.txt").write_text("code\n")
    git(wt, "add", "-A")
    assert git(wt, "diff", "--cached", "--name-only").split() == ["code.txt"]
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


def test_init_refuses_before_writing_anything(new_repo: Path, tmp_path: Path):
    """A refusal comes before pm writes anything: the repo is left as it was."""
    (new_repo / ".claude").mkdir()
    (new_repo / ".claude/settings.json").write_text(json.dumps(USER_SETTINGS))
    head, before = git(new_repo, "rev-parse", "HEAD"), snapshot(new_repo)
    res = pm(new_repo, "init")
    assert res.returncode != 0 and ".claude/settings.json is not laid out as pm writes JSON" in res.stderr, res.stderr
    assert snapshot(new_repo) == before and git(new_repo, "rev-parse", "HEAD") == head


def test_init_refuses_a_held_site_port_before_writing_anything(new_repo: Path, tmp_path: Path):
    """PORT names a port a server that is not pm holds: pm init refuses before the work store, the records branch or any
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
        assert not git(new_repo, "ls-remote", "--heads", "origin", "records").strip()
        assert not (tmp_path / "sched.log").exists(), "no service"
        free = int(said.group(1))
        assert free >= 8000 and free != port
        no_port = {k: v for k, v in env(tmp_path).items() if k != "PORT"}
        res = subprocess.run([*PM, "init"], cwd=new_repo, env=no_port, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert tomllib.loads((new_repo / ".pm/config.toml").read_text())["port"] == free
    assert f"serving http://localhost:{free} " in res.stdout, res.stdout


def test_init_in_a_worktree_sets_it_up_when_mains_pin_differs(repo):
    """The main checkout pins another pm: the worktree's own setup (its records link) still runs, and
    only the installed pm and the service, which follow main's pin, are refused."""
    wt = repo.root.parent / "feature"
    repo.git("worktree", "add", "-q", "--no-checkout", "-b", "feature", str(wt))
    repo.git("reset", "-q", "--hard", cwd=wt)
    write_config(repo.root, version="9.9.9")  # main's checkout moved its pin, uncommitted
    res = repo.pm("init", cwd=wt)
    assert res.returncode == 1, res.stdout
    assert f"error: the main checkout {repo.root} pins pm 9.9.9, and the pm service and the installed pm follow it" \
        in res.stderr, res.stderr
    assert "pm init set up this worktree and left the installed pm and the service alone:" in res.stderr, res.stderr
    assert f"linked {wt / 'records'} -> {repo.store}" in res.stderr, res.stderr
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == repo.store.resolve()
    assert repo.git("config", "--get", "--default=", "core.sparseCheckout", cwd=wt).strip() == ""
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
