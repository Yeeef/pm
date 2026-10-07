"""pm doctor, pm upgrade and pm uninstall in the temp repos of test_init (a local bare remote, the fake bd, and the
fake launchd/systemd in tmp/home): doctor is clean after init and names each hand-made change, upgrade moves the pin
and rewrites pm's parts only, uninstall removes pm's parts and setup only, and uninstall then init round-trips."""

from __future__ import annotations

import json
import subprocess
import tomllib
from pathlib import Path

import pytest

from pm import __version__
from test_init import BEADS_HOOK, PM_FILES, env, existing, git, new_repo, pm, section, snapshot  # noqa: F401 (fixtures)

LOCAL_SETTINGS = '{\n  "model": "café"\n}\n'  # non-ASCII: pm rewrites the file around it, byte for byte
CODEX_USER = '# mine\nmodel = "o3"\n\n[sandbox_workspace_write]\nnetwork_access = true\n'


def sched_units(tmp: Path) -> list[Path]:
    home = tmp / "home"
    return sorted(p for d in (home / "Library/LaunchAgents", home / ".config/systemd/user") if d.is_dir()
                  for p in d.iterdir())


def loaded(tmp: Path) -> list[str]:
    path = tmp / "sched.json"
    return json.loads(path.read_text())["loaded"] if path.exists() else []


def doctor(repo: Path) -> tuple[int, list[str]]:
    res = pm(repo, "doctor")
    return res.returncode, res.stdout.splitlines()


def test_doctor_is_clean_after_init(new_repo: Path):
    assert pm(new_repo, "init").returncode == 0
    code, lines = doctor(new_repo)
    assert code == 0 and lines == [f"pm {__version__}: every managed piece and the clone's setup match what pm init makes"]


def edit(path: Path, old: str, new: str) -> None:
    text = path.read_text()
    assert old in text, (path, old)
    path.write_text(text.replace(old, new, 1))


# each hand-made change, by the kind of piece it hits, and the start of the line doctor reports for it
REPO_CHANGES = {
    "whole file": (lambda r: edit(r / ".pm/README.md", "# pm", "# not pm"),
                   ".pm/README.md: pm's part differs"),
    "workflow deleted": (lambda r: (r / ".github/workflows/pm-records-guard.yml").unlink(),
                         ".github/workflows/pm-records-guard.yml: pm's part is missing"),
    "hook entry": (lambda r: edit(r / ".claude/settings.json", '"pm hook stop || exit 1"', '"pm hook stop"'),
                   ".claude/settings.json: pm's part differs"),
    "git hook section": (lambda r: edit(r / ".beads/hooks/pre-commit", "pm hook git-pre-commit", "pm hook git-x"),
                         ".beads/hooks/pre-commit: pm's part differs"),
    "gitignore block": (lambda r: edit(r / ".gitignore", "/records\n", "/records/\n"),
                        ".gitignore: pm's part differs"),
}


@pytest.mark.parametrize("kind", REPO_CHANGES)
def test_doctor_reports_a_changed_repo_piece_and_upgrade_restores_it(new_repo: Path, kind: str):
    assert pm(new_repo, "init").returncode == 0
    change, want = REPO_CHANGES[kind]
    change(new_repo)
    code, lines = doctor(new_repo)
    assert code == 1 and len(lines) == 1 and lines[0].startswith(f"repo: {want}"), lines
    assert lines[0].endswith("run pm upgrade to rewrite it")
    res = pm(new_repo, "upgrade")
    assert res.returncode == 0 and "pin stays" in res.stdout, res.stderr
    assert doctor(new_repo)[0] == 0


def setup_changes(tmp: Path) -> dict:
    return {
        "records link": (lambda r: (r / "records").unlink(), "records link: "),
        "sparse checkout": (lambda r: git(r, "sparse-checkout", "disable"), "sparse checkout: "),
        "hooks path": (lambda r: git(r, "config", "core.hooksPath", ".git/hooks"), "hooks path: core.hooksPath is .git/hooks"),
        "service": (lambda r: [p.unlink() for p in sched_units(tmp)], "service: not installed"),
        "codex roots": (lambda r: (tmp / "codex/config.toml").write_text(CODEX_USER), "codex: "),
    }


@pytest.mark.parametrize("kind", ["records link", "sparse checkout", "hooks path", "service", "codex roots"])
def test_doctor_reports_a_changed_clone_setup(new_repo: Path, tmp_path: Path, kind: str):
    (tmp_path / "codex").mkdir()
    assert pm(new_repo, "init").returncode == 0
    assert doctor(new_repo)[0] == 0
    change, want = setup_changes(tmp_path)[kind]
    change(new_repo)
    code, lines = doctor(new_repo)
    assert code == 1 and len(lines) == 1 and lines[0].startswith(want), lines


def test_upgrade_moves_the_pin_and_keeps_what_is_not_pms(existing: Path, tmp_path: Path):
    before = snapshot(existing)
    assert pm(existing, "init").returncode == 0
    git(existing, "add", "-A")
    git(existing, "commit", "-qm", "Install pm")
    # the repo as an older pm left it: an older pin and an older section in a Beads hook
    edit(existing / ".pm/config.toml", f'version = "{__version__}"', 'version = "0.0.1"')
    edit(existing / ".beads/hooks/pre-commit", f"BEGIN PM v{__version__}", "BEGIN PM v0.0.1")
    git(existing, "commit", "--no-verify", "-qam", "pm 0.0.1")
    assert pm(existing, "show").returncode == 1, "every other command refuses the old pin"
    res = pm(existing, "upgrade", "--to", "9.9.9")
    assert res.returncode == 1 and 'uv tool install "git+https://github.com/Yeeef/yeeef-agents@pm-v9.9.9' in res.stderr
    head = git(existing, "rev-parse", "HEAD")
    res = pm(existing, "upgrade")
    assert res.returncode == 0, res.stderr
    assert f"moved the pin from 0.0.1 to {__version__}" in res.stdout
    assert "git add -- .pm/config.toml .beads/hooks/pre-commit && " in res.stdout
    assert git(existing, "rev-parse", "HEAD") == head, "pm upgrade commits nothing"
    assert tomllib.loads((existing / ".pm/config.toml").read_text())["version"] == __version__
    assert (existing / ".beads/hooks/pre-commit").read_text() == BEADS_HOOK + section("pre-commit") + "\n# mine\necho done\n"
    assert git(existing, "status", "--porcelain").split() == ["M", ".beads/hooks/pre-commit", "M", ".pm/config.toml"]
    after = snapshot(existing)
    assert all(after[k] == v for k, v in before.items() if k not in PM_FILES), "files pm does not manage are kept"
    again = pm(existing, "upgrade")
    assert again.returncode == 0 and "nothing to commit" in again.stdout and snapshot(existing) == after


def install_everywhere(repo: Path, tmp: Path) -> Path:
    """Codex and Claude Code in use with config of their own, pm installed, and a second worktree set up by the
    post-checkout hook."""
    (tmp / "codex").mkdir()
    (tmp / "codex/config.toml").write_text(CODEX_USER)
    (tmp / "claude").mkdir()
    (repo / ".claude").mkdir(exist_ok=True)
    (repo / ".claude/settings.local.json").write_text(LOCAL_SETTINGS)
    res = pm(repo, "init")
    assert res.returncode == 0, res.stderr
    git(repo, "add", "-A")
    git(repo, "commit", "-qm", "Install pm")
    wt = tmp / "wt"
    res = subprocess.run(["git", "worktree", "add", "-q", "-b", "feature", str(wt)], cwd=repo, env=env(tmp),
                         capture_output=True, text=True)
    assert res.returncode == 0 and (wt / "records").is_symlink(), res.stderr
    assert doctor(repo)[0] == 0 and doctor(wt)[0] == 0
    return wt


def test_uninstall_removes_pms_parts_and_setup_only(existing: Path, tmp_path: Path):
    before = snapshot(existing)
    wt = install_everywhere(existing, tmp_path)
    store = existing / ".pm/store/records"
    records_head = git(existing, "rev-parse", "records")
    (store / "sprints/a-1.md").write_text("changed\n")
    res = pm(existing, "uninstall")
    assert res.returncode == 1 and "uncommitted records" in res.stderr and store.is_dir() and sched_units(tmp_path)
    git(store, "checkout", "--", "sprints/a-1.md")

    res = pm(existing, "uninstall")
    assert res.returncode == 0, res.stderr
    assert "git add -A -- " in res.stdout and 'git commit -m "Uninstall pm"' in res.stdout
    # the clone's and machine's setup is gone; the records branch and Beads stay
    assert not store.exists() and git(existing, "rev-parse", "records") == records_head
    assert git(existing, "worktree", "list", "--porcelain").count("worktree ") == 2
    for tree in (existing, wt):
        assert not (tree / "records").is_symlink()
        assert git(tree, "config", "--get", "--default=", "core.sparseCheckout").strip() in ("", "false")
    assert sched_units(tmp_path) == [] and loaded(tmp_path) == []
    assert (tmp_path / "codex/config.toml").read_text() == CODEX_USER
    assert (existing / ".claude/settings.local.json").read_text() == LOCAL_SETTINGS
    assert not (wt / ".claude/settings.local.json").exists()
    assert not (existing / ".pm").exists() and (existing / ".beads/hooks").is_dir()
    # every byte that is not pm's is as it was, but for the newline pm's block needed after the unterminated last
    # line of .gitignore
    after = snapshot(existing)
    assert after.pop(".gitignore") == before.pop(".gitignore") + b"\n"
    assert after.pop(".claude/settings.local.json") == LOCAL_SETTINGS.encode()
    assert after == before
    assert pm(existing, "uninstall").returncode != 0, "with .pm/ gone, pm refuses to run here"


def test_uninstall_then_init_round_trips(new_repo: Path, tmp_path: Path):
    install_everywhere(new_repo, tmp_path)
    installed = snapshot(new_repo)
    codex = (tmp_path / "codex/config.toml").read_text()
    res = pm(new_repo, "uninstall")
    assert res.returncode == 0, res.stderr
    assert (tmp_path / "codex/config.toml").read_text() == CODEX_USER
    res = pm(new_repo, "init")
    assert res.returncode == 0, res.stderr
    def lasting(snap: dict) -> dict:  # .pm/run is runtime state: the new service's install time differs
        return {k: v for k, v in snap.items() if not k.startswith(".pm/run/")}
    assert lasting(snapshot(new_repo)) == lasting(installed)
    assert (tmp_path / "codex/config.toml").read_text() == codex
    assert git(new_repo, "status", "--porcelain").strip() == ""
    assert doctor(new_repo)[0] == 0
