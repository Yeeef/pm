"""pm doctor, pm upgrade and pm uninstall in the temp repos of test_init (a local bare remote, the fake bd, and the
fake launchd/systemd in tmp/home): init and upgrade keep what is not pm's, doctor names each hand-made change,
upgrade moves the pin and rewrites pm's parts only, uninstall removes pm's parts and setup only."""

from __future__ import annotations

import json
import socket
import subprocess
import tomllib
from pathlib import Path

import pytest

from conftest import IMPL, PM
from pm import __version__
from test_init import (BD_SET, BD_WRITES, BEADS_HOOK, CLAUDE_PM, GITIGNORE_BLOCK, PM_FILES, USER_CODEX, USER_SETTINGS,  # noqa: F401
                       commands, env, existing, git, new_repo, pm, pm_free, section, snapshot)

pytestmark = pytest.mark.integration  # each test runs pm init in a fresh repo with a remote and starts its service

LOCAL_SETTINGS = '{\n  "model": "café"\n}\n'  # non-ASCII: pm rewrites the file around it, byte for byte
CODEX_USER = '# mine\nmodel = "o3"\n\n[sandbox_workspace_write]\nnetwork_access = true\n'


def uv_cache() -> str:
    res = subprocess.run(["uv", "--color", "never", "cache", "dir"], check=True, capture_output=True, text=True)
    return str(Path(res.stdout.strip()).resolve())


# Go pm needs no uv, so its roots are the clone's own five: .git, the records store and its git dir, the work store
# and .pm/run, which holds the work store's gate (the pm-go page, Open question 12); Python pm's four (.git, the store,
# its git dir, .beads) and uv's cache, which every clone shares
CLONE_ROOTS, SHARED_ROOTS = (4, 1) if IMPL == "python" else (5, 0)


def codex_after_uninstall() -> str:
    """CODEX_USER with uv's cache, the one root uninstall keeps: other clones share it. Go pm adds no uv cache, so
    uninstall leaves CODEX_USER as it was."""
    if IMPL == "go":
        return CODEX_USER
    return CODEX_USER.replace("[sandbox_workspace_write]\n",
                              f"[sandbox_workspace_write]\nwritable_roots = [{json.dumps(uv_cache())}]\n")


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


def reported(lines: list[str], wants: list[str]) -> bool:
    """Doctor gave one line per change: each line starts with exactly one change's expected start, in any order."""
    return sorted(w for l in lines for w in wants if l.startswith(w)) == sorted(wants) and len(lines) == len(wants)


def test_doctor_reports_each_changed_repo_piece_and_upgrade_restores_it(new_repo: Path):
    """Every change at once, so one init serves them all (each init costs seconds); each still gets its own line."""
    assert pm(new_repo, "init").returncode == 0
    for change, _ in REPO_CHANGES.values():
        change(new_repo)
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, [f"repo: {want}" for _, want in REPO_CHANGES.values()]), lines
    assert all(l.endswith(f"run pm upgrade --to {__version__} to rewrite it") for l in lines), lines
    res = pm(new_repo, "upgrade")
    assert res.returncode == 0 and "pin stays" in res.stdout, res.stderr
    assert doctor(new_repo)[0] == 0


def test_init_leaves_an_installed_repos_files_alone_and_doctor_names_upgrade(new_repo: Path):
    """Once .pm/config.toml exists, pm init (session start runs it) does only the clone's half: a branch that changed
    pm's hook entries keeps its change, and pm doctor names pm upgrade --to the pin as the fix."""
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    git(new_repo, "checkout", "-q", "-b", "changed")
    settings = new_repo / ".claude/settings.json"
    edit(settings, '"pm hook stop || exit 1"', '"pm hook stop"')
    (new_repo / ".github/workflows/pm-records-guard.yml").unlink()
    git(new_repo, "commit", "-qam", "change pm's pieces")
    changed = settings.read_bytes()
    res = pm(new_repo, "init")
    assert res.returncode == 0, res.stderr
    assert settings.read_bytes() == changed and not (new_repo / ".github/workflows/pm-records-guard.yml").exists()
    assert git(new_repo, "status", "--porcelain") == "" and "git add" not in res.stdout, res.stdout
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, ["repo: .claude/settings.json: pm's part differs",
                                          "repo: .github/workflows/pm-records-guard.yml: pm's part is missing"]), lines
    assert all(l.endswith(f"run pm upgrade --to {__version__} to rewrite it") for l in lines), lines
    # --site-url is the one repo write a later pm init makes, on request; a bad URL is refused before any write
    res = pm(new_repo, "init", "--site-url", "pm.example.com")
    assert res.returncode == 1 and "is not an http(s) base URL" in res.stderr and git(new_repo, "status", "--porcelain") == ""
    res = pm(new_repo, "init", "--site-url", "https://pm.example.com/")
    assert res.returncode == 0 and "git add -- .pm/config.toml && " in res.stdout, res.stdout
    assert tomllib.loads((new_repo / ".pm/config.toml").read_text())["site_url"] == "https://pm.example.com"
    assert settings.read_bytes() == changed


SETUP_CHANGES = {  # each hand-made change to the clone's setup and the start of the line doctor reports for it
    "records link": (lambda r, tmp: (r / "records").unlink(), "records link: "),
    "sparse checkout": (lambda r, tmp: git(r, "sparse-checkout", "disable"), "sparse checkout: "),
    "hooks path": (lambda r, tmp: git(r, "config", "core.hooksPath", ".git/hooks"),
                   "hooks path: core.hooksPath is .git/hooks, not .beads/hooks; pm works only with Beads' hooks path"),
    "service": (lambda r, tmp: [p.unlink() for p in sched_units(tmp)], "service: not installed"),
    "codex roots": (lambda r, tmp: (tmp / "codex/config.toml").write_text(CODEX_USER), "codex: "),
    "git exclude": (lambda r, tmp: edit(r / ".git/info/exclude", "/.pm/run/\n", ""), "git exclude: "),
}


def test_doctor_reports_each_changed_clone_setup(new_repo: Path, tmp_path: Path):
    """Every change at once, so one init serves them all; each still gets its own line."""
    (tmp_path / "codex").mkdir()
    assert pm(new_repo, "init").returncode == 0
    assert doctor(new_repo)[0] == 0
    for change, _ in SETUP_CHANGES.values():
        change(new_repo, tmp_path)
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, [want for _, want in SETUP_CHANGES.values()]), lines


def test_init_and_upgrade_keep_what_is_not_pms(existing: Path, tmp_path: Path):
    """pm init into a repo with Beads and settings of its own, then pm upgrade from an older pin: each rewrites pm's
    parts only, around the repo's own content, byte for byte."""
    before = snapshot(existing)
    res = pm(existing, "init")
    assert res.returncode == 0, res.stderr
    calls = [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    assert ["init", "--non-interactive"] not in calls, "Beads is there; bd init must not run"
    assert f"git add -- {' '.join(PM_FILES + BD_WRITES)} && " in res.stdout, "the agent profile bd set is listed"
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
    installed = snapshot(existing)
    assert all(installed[k] == v for k, v in before.items() if k not in PM_FILES + BD_SET), "files pm does not manage are kept"
    again = pm(existing, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(existing) == installed and "git add" not in again.stdout

    git(existing, "add", "-A")
    git(existing, "commit", "-qm", "Install pm")
    # the repo as an older pm left it: an older pin and an older section in a Beads hook
    edit(existing / ".pm/config.toml", f'version = "{__version__}"', 'version = "0.0.1"')
    edit(existing / ".beads/hooks/pre-commit", f"BEGIN PM v{__version__}", "BEGIN PM v0.0.1")
    git(existing, "commit", "--no-verify", "-qam", "pm 0.0.1")
    def launched(pin: str, *args: str) -> subprocess.CompletedProcess:  # as the pm uv tool launches `pin`, whose
        # release here builds this pm: test_launch.py has the launch itself, which would reach GitHub
        return subprocess.run([*PM, *args], cwd=existing, env=dict(env(existing.parent), PM_LAUNCHED=pin),
                              capture_output=True, text=True)
    assert launched("0.0.1", "show").returncode == 1, "every other command refuses the old pin"
    res = launched("9.9.9", "upgrade", "--to", "9.9.9")
    assert res.returncode == 1 and res.stderr.endswith(  # the launcher's install, never an old pm in its place
        'which launches pm 9.9.9: install the latest with uv tool install --reinstall '
        '"git+https://github.com/Yeeef/yeeef-agents#subdirectory=pm"\n'), res.stderr
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
    assert all(after[k] == v for k, v in before.items() if k not in PM_FILES + BD_SET), "files pm does not manage are kept"
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
    exclude = existing / ".git/info/exclude"
    exclude.write_text(exclude.read_text() + "*.swp\n")  # a line of the user's after git's own comments
    exclude_before = exclude.read_bytes()
    before = snapshot(existing)
    wt = install_everywhere(existing, tmp_path)
    gone = tmp_path / "gone"  # deleted without git worktree remove: still listed (prunable), and uninstall skips it
    subprocess.run(["git", "worktree", "add", "-q", "-b", "gone", str(gone)], cwd=existing, env=env(tmp_path), check=True)
    subprocess.run(["rm", "-rf", str(gone)], check=True)
    assert "prunable" in git(existing, "worktree", "list", "--porcelain")
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
    assert git(existing, "worktree", "list", "--porcelain").count("worktree ") == 3
    for tree in (existing, wt):
        assert not (tree / "records").is_symlink()
        assert git(tree, "config", "--get", "--default=", "core.sparseCheckout").strip() in ("", "false")
    assert sched_units(tmp_path) == [] and loaded(tmp_path) == []
    assert (tmp_path / "codex/config.toml").read_text() == codex_after_uninstall()
    # pm's exclude lines go, byte for byte; the fake bd's own line (its database) stays with Beads
    assert exclude.read_bytes() == exclude_before + (b"/.beads/embeddeddolt/\n" if IMPL == "python" else b"")
    assert (existing / ".claude/settings.local.json").read_text() == LOCAL_SETTINGS
    assert not (wt / ".claude/settings.local.json").exists()
    assert not (existing / ".pm").exists() and (existing / ".beads/hooks").is_dir()
    # every byte that is not pm's is as it was, but for the newline pm's block needed after the unterminated last
    # line of .gitignore
    after = {k: v for k, v in snapshot(existing).items() if not k.startswith(".beads/embeddeddolt/")}  # Beads stays
    assert after.pop(".gitignore") == before.pop(".gitignore") + b"\n"
    assert after.pop(".claude/settings.local.json") == LOCAL_SETTINGS.encode()
    profile = b"agent.profile: team-maintainer\n" if IMPL == "python" else b""  # Go pm sets no Beads profile
    assert after.pop(".beads/config.yaml") == before.pop(".beads/config.yaml") + profile
    assert after == before
    assert pm(existing, "uninstall").returncode != 0, "with .pm/ gone, pm refuses to run here"


def test_uninstall_keeps_the_shared_uv_cache_and_other_clones_roots(new_repo: Path, tmp_path: Path):
    """Two clones set up on one machine: uninstalling one removes only its own roots (CLONE_ROOTS); uv's cache, which both
    need, and the other clone's roots stay, and every other byte of config.toml with them. Roots it cannot take out
    without breaking the TOML are refused before anything changes."""
    (tmp_path / "codex").mkdir()
    config = tmp_path / "codex/config.toml"
    config.write_text(CODEX_USER)
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    git(new_repo, "push", "-q", "origin", "main")
    other = tmp_path / "other"
    git(tmp_path, "clone", "-q", str(tmp_path / "remote.git"), str(other))
    with socket.socket() as s:  # a second clone on this machine gets its own site port
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    res = subprocess.run([*PM, "init"], cwd=other, env=dict(env(tmp_path), PORT=str(port)), capture_output=True,
                         text=True)
    assert res.returncode == 0, res.stderr
    roots = tomllib.loads(config.read_text())["sandbox_workspace_write"]["writable_roots"]
    mine = [r for r in roots if r.startswith(str(new_repo.resolve()))]
    theirs = [r for r in roots if r.startswith(str(other.resolve()))]
    assert len(mine) == len(theirs) == CLONE_ROOTS and roots.count(uv_cache()) == SHARED_ROOTS, roots
    assert len(roots) == 2 * CLONE_ROOTS + SHARED_ROOTS, roots
    # writable_roots spread over lines, one root a line: taking a root out leaves its comma, which is not TOML;
    # uninstall names the file and says to remove the roots by hand, before it changes anything
    flat = config.read_text()
    spread = "writable_roots = [\n" + "".join(f"  {json.dumps(r)},\n" for r in roots) + "]\n"
    config.write_text(CODEX_USER.replace("[sandbox_workspace_write]\n", f"[sandbox_workspace_write]\n{spread}"))
    codex, before, units = config.read_text(), snapshot(new_repo), sched_units(tmp_path)
    res = pm(new_repo, "uninstall")
    assert res.returncode == 1, res
    assert f"editing {config} would leave it invalid TOML (" in res.stderr, res.stderr
    assert "remove these from its writable_roots by hand: " in res.stderr and json.dumps(mine[0]) in res.stderr
    assert config.read_text() == codex and snapshot(new_repo) == before and sched_units(tmp_path) == units != []
    assert (new_repo / ".pm/store/records").is_dir() and (new_repo / "records").is_symlink()
    config.write_text(flat)
    res = pm(new_repo, "uninstall")
    assert res.returncode == 0, res.stderr
    assert "uv's cache stays" in res.stdout if IMPL == "python" else f"removed this clone's writable_roots from {config}" \
        in res.stdout, res.stdout
    kept = [r for r in roots if r not in mine]
    assert config.read_text() == CODEX_USER.replace(
        "[sandbox_workspace_write]\n",
        f"[sandbox_workspace_write]\nwritable_roots = [{', '.join(json.dumps(r) for r in kept)}]\n")


def test_doctor_names_a_settings_file_it_cannot_read_as_a_repo_finding(new_repo: Path):
    """A settings file that is not JSON hides any pre-package harness piece in it: doctor says it could not look,
    labelled as the repo finding it is, not as a legacy piece."""
    assert pm(new_repo, "init").returncode == 0
    (new_repo / ".claude/settings.json").write_text("{not json")
    code, lines = doctor(new_repo)
    assert code == 1 and any(l.startswith("repo: cannot look for the pre-package harness's pieces: .claude/settings.json "
                                          "is not valid JSON") for l in lines), lines
    assert not any(l.startswith("legacy: ") for l in lines), lines
