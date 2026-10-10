"""pm doctor, pm upgrade and pm uninstall in the temp repos of test_init (a local bare remote and the
fake launchd/systemd in tmp/home): init and upgrade keep what is not pm's, doctor names each hand-made change,
upgrade moves the pin and rewrites pm's parts only, uninstall removes pm's parts and setup only."""

from __future__ import annotations

import json
import socket
import subprocess
import tomllib
from pathlib import Path

import pytest

from conftest import PM, VERSION
from test_init import (BEADS_HOOK, CLAUDE_PM, GITIGNORE_BLOCK, HOOKS, PM_FILES, USER_CODEX,  # noqa: F401
                       USER_SETTINGS, commands, env, existing, git, new_repo, pm, pm_free, section, snapshot)

pytestmark = pytest.mark.integration  # each test runs pm init in a fresh repo with a remote and starts its service

LOCAL_SETTINGS = '{\n  "model": "café"\n}\n'  # non-ASCII: pm rewrites the file around it, byte for byte
CODEX_USER = '# mine\nmodel = "o3"\n\n[sandbox_workspace_write]\nnetwork_access = true\n'


# pm's roots in Codex's writable_roots, per clone: .git, the records store and its git dir, the work store and
# .pm/run, which holds the work store's socket
CLONE_ROOTS = 5


MINE = "\n# mine\necho done\n"  # the existing repo's own lines after Beads' section in its hook files


def hook_file(name: str) -> str:
    """pm's git hook file in the existing repo: pm's own file holds its section alone, and Beads' stays as it was."""
    return "#!/usr/bin/env sh\n" + section(name)


def without_bd(data: dict) -> dict:
    """The user's settings data as pm leaves it: pm, which runs no bd, takes out each hook whose command starts with
    `bd ` and the groups and events that leaves empty (the work-store page, Cut-over)."""
    out = json.loads(json.dumps(data))
    for event, groups in list(out.get("hooks", {}).items()):
        for g in groups:
            g["hooks"] = [h for h in g["hooks"] if not h["command"].startswith("bd ")]
        out["hooks"][event] = [g for g in groups if g["hooks"]]
        if not out["hooks"][event]:
            del out["hooks"][event]
    return out


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


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def edit(path: Path, old: str, new: str) -> None:
    text = path.read_text()
    assert old in text, (path, old)
    path.write_text(text.replace(old, new, 1))


# each hand-made change, by the kind of piece it hits, and the start of the line doctor reports for it
REPO_CHANGES = {
    "whole file": (lambda r: edit(r / ".pm/README.md", "# pm", "# not pm"),
                   ".pm/README.md: pm's part differs"),
    "retired workflow": (lambda r: write(r / ".github/workflows/pm-records-copy.yml", "name: Copy records\n"),
                         ".github/workflows/pm-records-copy.yml: an earlier pm's file, retired with the main branch's "
                         "records/ copy"),
    "hook entry": (lambda r: edit(r / ".claude/settings.json", '"pm hook stop || exit 1"', '"pm hook stop"'),
                   ".claude/settings.json: pm's part differs"),
    "git hook section": (lambda r: edit(r / f"{HOOKS}/post-checkout", "pm hook git-post-checkout", "pm hook git-x"),
                         f"{HOOKS}/post-checkout: pm's part differs"),
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
    assert all(l.endswith(f"run pm upgrade --to {VERSION} to rewrite it") for l in lines), lines
    res = pm(new_repo, "upgrade")
    assert res.returncode == 0 and "pin stays" in res.stdout, res.stderr
    assert not (new_repo / ".github/workflows/pm-records-copy.yml").exists()
    assert doctor(new_repo)[0] == 0


def test_a_beads_file_is_no_beads_for_doctor_and_upgrade(new_repo: Path):
    """A repo that retired Beads by replacing .beads/ with a file (which blocks every bd command) holds no Beads
    pieces, as a repo without .beads/: pm doctor is clean and pm upgrade --to the pin finds every piece current."""
    assert pm(new_repo, "init").returncode == 0
    (new_repo / ".beads").write_text("Beads is retired here.\n")
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Retire Beads")
    res = pm(new_repo, "doctor")
    assert res.returncode == 0, res.stdout + res.stderr
    res = pm(new_repo, "upgrade", "--to", VERSION)
    assert res.returncode == 0, res.stderr
    assert res.stdout == f"pm {VERSION}: every managed piece is current; nothing to commit\n", res.stdout


def test_init_leaves_an_installed_repos_files_alone_and_doctor_names_upgrade(new_repo: Path):
    """Once .pm/config.toml exists, pm init (session start runs it) does only the clone's half: a branch that changed
    pm's hook entries keeps its change, and pm doctor names pm upgrade --to the pin as the fix."""
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    git(new_repo, "checkout", "-q", "-b", "changed")
    settings = new_repo / ".claude/settings.json"
    edit(settings, '"pm hook stop || exit 1"', '"pm hook stop"')
    (new_repo / f"{HOOKS}/post-checkout").unlink()
    git(new_repo, "commit", "-qam", "change pm's pieces")
    changed = settings.read_bytes()
    res = pm(new_repo, "init")
    assert res.returncode == 0, res.stderr
    assert settings.read_bytes() == changed and not (new_repo / f"{HOOKS}/post-checkout").exists()
    assert git(new_repo, "status", "--porcelain") == "" and "git add" not in res.stdout, res.stdout
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, ["repo: .claude/settings.json: pm's part differs",
                                          f"repo: {HOOKS}/post-checkout: pm's part is missing"]), lines
    assert all(l.endswith(f"run pm upgrade --to {VERSION} to rewrite it") for l in lines), lines
    # --site-url is the one repo write a later pm init makes, on request; a bad URL is refused before any write
    res = pm(new_repo, "init", "--site-url", "pm.example.com")
    assert res.returncode == 1 and "is not an http(s) base URL" in res.stderr and git(new_repo, "status", "--porcelain") == ""
    res = pm(new_repo, "init", "--site-url", "https://pm.example.com/")
    assert res.returncode == 0 and "git add -- .pm/config.toml && " in res.stdout, res.stdout
    assert tomllib.loads((new_repo / ".pm/config.toml").read_text())["site_url"] == "https://pm.example.com"
    assert settings.read_bytes() == changed


SETUP_CHANGES = {  # each hand-made change to the clone's setup and the start of the line doctor reports for it
    "records link": (lambda r, tmp: (r / "records").unlink(), "records link: "),
    "sparse checkout": (lambda r, tmp: git(r, "sparse-checkout", "set", "--no-cone", "/*", "!/records/"),
                        "sparse checkout: "),
    "hooks path": (lambda r, tmp: git(r, "config", "core.hooksPath", ".git/hooks"),
                   "hooks path: core.hooksPath is .git/hooks, not .pm/hooks; pm works only with its own hooks path"),
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
    assert f"git add -- {' '.join(PM_FILES)} && " in res.stdout, res.stdout
    assert (existing / f"{HOOKS}/post-checkout").read_text() == hook_file("post-checkout")
    for name in ("post-checkout", "pre-commit"):
        assert (existing / f".beads/hooks/{name}").read_text() == BEADS_HOOK + MINE, "Beads' hook files stay"
    for rel, user in ((".claude/settings.json", USER_SETTINGS), (".codex/hooks.json", USER_CODEX)):
        text = (existing / rel).read_text()
        data = json.loads(text)
        assert pm_free(data) == without_bd(user) and text == json.dumps(data, indent=2) + "\n", rel
    claude = json.loads((existing / ".claude/settings.json").read_text())
    assert all(commands(claude, e)[-len(c):] == c for e, c in CLAUDE_PM.items())
    assert (existing / ".gitignore").read_text() == "*.log\nbuild/\n" + GITIGNORE_BLOCK
    assert git(existing / ".pm/store/records", "ls-files").split() == ["sprints/a-1.md"]
    installed = snapshot(existing)
    assert all(installed[k] == v for k, v in before.items() if k not in PM_FILES), "files pm does not manage are kept"
    again = pm(existing, "init")
    assert again.returncode == 0, again.stderr
    assert snapshot(existing) == installed and "git add" not in again.stdout

    git(existing, "add", "-A")
    git(existing, "commit", "-qm", "Install pm")
    # the repo as an older pm left it: an older pin and an older section in a Beads hook
    edit(existing / ".pm/config.toml", f'version = "{VERSION}"', 'version = "0.2.0"')
    edit(existing / f"{HOOKS}/post-checkout", f"BEGIN PM v{VERSION}", "BEGIN PM v0.2.0")
    git(existing, "commit", "--no-verify", "-qam", "pm 0.2.0")
    def launched(pin: str, *args: str) -> subprocess.CompletedProcess:  # as the installed pm launches `pin`, whose
        # release here builds this pm: test_launch.py has the launch itself, which would reach GitHub
        return subprocess.run([*PM, *args], cwd=existing, env=dict(env(existing.parent), PM_LAUNCHED=pin),
                              capture_output=True, text=True)
    assert launched("0.2.0", "show").returncode == 1, "every other command refuses the old pin"
    res = launched("9.9.9", "upgrade", "--to", "9.9.9")
    latest = "curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh | sh"
    assert res.returncode == 1 and res.stderr.endswith(  # the launcher's install, never an old pm in its place
        f'which launches pm 9.9.9: install the latest with {latest}\n'), res.stderr
    head = git(existing, "rev-parse", "HEAD")
    res = pm(existing, "upgrade")
    assert res.returncode == 0, res.stderr
    assert f"moved the pin from 0.2.0 to {VERSION}" in res.stdout
    assert f"git add -- .pm/config.toml {HOOKS}/post-checkout && " in res.stdout
    assert git(existing, "rev-parse", "HEAD") == head, "pm upgrade commits nothing"
    assert tomllib.loads((existing / ".pm/config.toml").read_text())["version"] == VERSION
    assert (existing / f"{HOOKS}/post-checkout").read_text() == hook_file("post-checkout")
    changed = sorted([f"{HOOKS}/post-checkout", ".pm/config.toml"])
    assert git(existing, "status", "--porcelain").split() == ["M", changed[0], "M", changed[1]]
    after = snapshot(existing)
    assert all(after[k] == v for k, v in before.items() if k not in PM_FILES), "files pm does not manage are kept"
    again = pm(existing, "upgrade")
    assert again.returncode == 0 and "nothing to commit" in again.stdout and snapshot(existing) == after


OLD_PRE_COMMIT = '#!/usr/bin/env sh\n# --- BEGIN PM v0.3.0 ---\npm hook git-pre-commit "$@" || exit $?\n# --- END PM ---\n'
RETIRED_WORKFLOWS = [".github/workflows/pm-records-copy.yml", ".github/workflows/pm-records-guard.yml"]


def test_upgrade_retires_the_main_branchs_records_copy(new_repo: Path, tmp_path: Path):
    """A repo as a pm that kept the main branch's records/ copy left it: the copy and guard workflows, pm's pre-commit
    section, the copy tracked on main and the sparse checkout that kept it from the link. pm doctor names each; pm
    init leaves the sparse checkout while HEAD tracks the copy; pm upgrade removes the pieces and names the commit
    that untracks the copy; after that commit, pm init turns the sparse checkout off and the link stays."""
    assert pm(new_repo, "init").returncode == 0
    for rel in RETIRED_WORKFLOWS:
        write(new_repo / rel, "name: old\n")
    write(new_repo / f"{HOOKS}/pre-commit", OLD_PRE_COMMIT)
    (new_repo / f"{HOOKS}/pre-commit").chmod(0o755)
    (new_repo / "records").unlink()
    write(new_repo / "records/sprints/a-1.md", "copy\n")
    git(new_repo, "add", "-A")
    git(new_repo, "add", "-f", "records")
    # the old section's pm hook git-pre-commit is retired: it lets this commit, which stages records/, through
    res = subprocess.run(["git", "commit", "-qm", "pm with main's records/ copy"], cwd=new_repo, env=env(tmp_path),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    git(new_repo, "sparse-checkout", "set", "--no-cone", "/*", "!/records/")
    git(new_repo, "config", "--worktree", "sparse.expectFilesOutsideOfPatterns", "true")
    assert not (new_repo / "records").exists()
    (new_repo / "records").symlink_to(new_repo / ".pm/store/records")

    code, lines = doctor(new_repo)
    retired = "retired with the main branch's records/ copy"
    assert code == 1 and reported(lines, [
        *(f"repo: {rel}: an earlier pm's file, {retired}" for rel in RETIRED_WORKFLOWS),
        f"repo: {HOOKS}/pre-commit: an earlier pm's file, {retired}",
        f"records copy: {new_repo} tracks records/, a copy no branch keeps now; untrack it with git rm -r -q --cached "
        "--sparse records and commit, or merge the main branch once it has"]), lines
    res = pm(new_repo, "init")
    assert res.returncode == 0 and "sparse" not in res.stdout, res.stdout
    assert "!/records/" in git(new_repo, "sparse-checkout", "list").split(), "kept while HEAD tracks the copy"

    res = pm(new_repo, "upgrade")
    assert res.returncode == 0, res.stderr
    commit = (f"git add -- {' '.join(RETIRED_WORKFLOWS)} {HOOKS}/pre-commit && git rm -r -q --cached --sparse records && "
              f'git commit -m "Upgrade pm to {VERSION}"')
    assert res.stdout.splitlines() == [
        f"pin stays {VERSION}",
        *(f"removed {rel}, {retired}" for rel in RETIRED_WORKFLOWS),
        f"removed {HOOKS}/pre-commit, {retired}",
        "this branch tracks records/, a copy no branch keeps now; the commit below untracks it",
        f"pm commits nothing on main; commit pm's files there: {commit}"], res.stdout
    assert not any((new_repo / rel).exists() for rel in [*RETIRED_WORKFLOWS, f"{HOOKS}/pre-commit"])
    res = subprocess.run(["sh", "-c", commit], cwd=new_repo, env=env(tmp_path), capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert git(new_repo, "ls-tree", "HEAD", "records") == ""
    assert (new_repo / "records").is_symlink(), "untracking the copy leaves the link"

    res = pm(new_repo, "init")
    assert res.returncode == 0 and f"turned off the sparse checkout an earlier pm set in {new_repo}" in res.stdout, \
        res.stdout
    assert git(new_repo, "config", "--get", "--default=", "core.sparseCheckout").strip() in ("", "false")
    assert git(new_repo, "config", "--worktree", "--get", "--default=", "sparse.expectFilesOutsideOfPatterns") == "\n"
    assert (new_repo / "records").is_symlink() and (new_repo / "records/sprints").is_dir()
    assert git(new_repo, "status", "--porcelain") == ""
    code, lines = doctor(new_repo)
    assert code == 0, lines


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
    assert (tmp_path / "codex/config.toml").read_text() == CODEX_USER
    assert exclude.read_bytes() == exclude_before  # pm's exclude lines go, byte for byte
    assert (existing / ".claude/settings.local.json").read_text() == LOCAL_SETTINGS
    assert not (wt / ".claude/settings.local.json").exists()
    assert not (existing / ".pm").exists() and (existing / ".beads/hooks").is_dir()
    # every byte that is not pm's is as it was, but for the newline pm's block needed after the unterminated last
    # line of .gitignore
    after = snapshot(existing)
    assert after.pop(".gitignore") == before.pop(".gitignore") + b"\n"
    assert after.pop(".claude/settings.local.json") == LOCAL_SETTINGS.encode()
    # pm init took Beads' hooks out, and uninstall does not put them back: Codex's file held only those
    assert after.pop(".claude/settings.json") == (json.dumps(without_bd(USER_SETTINGS), indent=2) + "\n").encode()
    assert ".codex/hooks.json" not in after
    del before[".claude/settings.json"], before[".codex/hooks.json"]
    assert after == before
    assert pm(existing, "uninstall").returncode != 0, "with .pm/ gone, pm refuses to run here"


def test_uninstall_keeps_other_clones_roots(new_repo: Path, tmp_path: Path):
    """Two clones set up on one machine: uninstalling one removes only its own roots (CLONE_ROOTS); the other clone's
    roots stay, and every other byte of config.toml with them. Roots it cannot take out
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
    assert len(mine) == len(theirs) == CLONE_ROOTS and len(roots) == 2 * CLONE_ROOTS, roots
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
    assert f"removed this clone's writable_roots from {config}" in res.stdout, res.stdout
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


BEADS_BLOCK = ("<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->\n## Beads Issue Tracker\n\n"
               "Run `bd prime`.\n<!-- END BEADS INTEGRATION -->\n")


def as_python_left_it(repo: Path) -> None:
    """Turn a repo Go pm set up into one as Python pm 0.1.x with Beads leaves it, committed: bd's hook entries first in
    both hook files (Codex's also in an event pm has none in), the Beads block in CLAUDE.md, pm's sections in Beads'
    hook files and core.hooksPath on them, and no .pm/hooks."""
    for rel, event, cmds in ((".claude/settings.json", "SessionStart", ["bd prime --hook-json"]),
                             (".codex/hooks.json", "SessionStart", ["bd codex-hook SessionStart"])):
        data = json.loads((repo / rel).read_text())
        data["hooks"][event].insert(0, {"hooks": [{"command": c, "type": "command"} for c in cmds], "matcher": ""})
        if rel == ".codex/hooks.json":
            data["hooks"] = {"PostCompact": [{"hooks": [{"command": "bd codex-hook PostCompact", "type": "command"}]}],
                             **data["hooks"]}
        (repo / rel).write_text(json.dumps(data, indent=2) + "\n")
    (repo / "CLAUDE.md").write_text("# Repo\n\nNotes.\n\n" + BEADS_BLOCK)
    for name in ("post-checkout", "pre-commit"):
        (repo / f".beads/hooks/{name}").write_text(BEADS_HOOK + section(name) + MINE)
    git(repo, "rm", "-rq", ".pm/hooks")
    git(repo, "config", "core.hooksPath", str(repo / ".beads/hooks"))
    git(repo, "add", "-A")
    git(repo, "commit", "--no-verify", "-qm", "as Python pm 0.1.x left it")


def test_upgrade_takes_out_what_python_pm_left_of_beads(existing: Path):
    """The cut-over (the work-store page, Cut-over): a repo as Python pm 0.1.x left it. pm doctor names each Beads
    piece, pm upgrade removes them and points core.hooksPath at pm's own hook files, keeping .beads/ and everything
    else, after which pm doctor is clean, a second upgrade changes nothing, and the upgrade commits through pm's
    hooks."""
    assert pm(existing, "init").returncode == 0
    git(existing, "add", "-A")
    git(existing, "commit", "-qm", "Install pm")
    as_python_left_it(existing)
    beads = {k: v for k, v in snapshot(existing).items() if k.startswith(".beads/")}

    code, lines = doctor(existing)
    v = VERSION
    assert code == 1 and reported(lines, [
        "repo: .claude/settings.json: holds Beads' hook entries (bd prime --hook-json), which pm " + v + " removes",
        "repo: .codex/hooks.json: holds Beads' hook entries (bd codex-hook PostCompact, bd codex-hook SessionStart), "
        "which pm " + v + " removes",
        "repo: .pm/hooks/post-checkout: pm's part is missing",
        "repo: CLAUDE.md: holds the Beads block (<!-- BEGIN BEADS INTEGRATION … -->), which pm " + v + " removes",
        f"hooks path: core.hooksPath is {existing / '.beads/hooks'} (Beads' hooks), not .pm/hooks; run pm upgrade "
        f"--to {v} to move it"]), lines

    res = pm(existing, "upgrade")
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines() == [
        f"pin stays {v}",
        "removed Beads' hook entries (bd prime --hook-json) from .claude/settings.json",
        "removed Beads' hook entries (bd codex-hook PostCompact, bd codex-hook SessionStart) from .codex/hooks.json",
        "wrote .pm/hooks/post-checkout",
        "removed the Beads block (<!-- BEGIN BEADS INTEGRATION … -->) from CLAUDE.md",
        f"moved the git hooks off Beads' {existing / '.beads/hooks'}: core.hooksPath={existing / '.pm/hooks'}",
        f"pm commits nothing on {git(existing, 'rev-parse', '--abbrev-ref', 'HEAD').strip()}; commit pm's files there: "
        "git add -- .claude/settings.json .codex/hooks.json "
        f".pm/hooks/post-checkout CLAUDE.md && git commit -m \"Upgrade pm to {v}\""], res.stdout
    for rel, user in ((".claude/settings.json", USER_SETTINGS), (".codex/hooks.json", USER_CODEX)):
        text = (existing / rel).read_text()
        data = json.loads(text)
        assert pm_free(data) == without_bd(user) and text == json.dumps(data, indent=2) + "\n", rel
        assert all(commands(data, e) == c for e, c in CLAUDE_PM.items()), rel
    assert (existing / "CLAUDE.md").read_text() == "# Repo\n\nNotes.\n"
    assert (existing / ".pm/hooks/post-checkout").read_text() == hook_file("post-checkout")
    assert not (existing / ".pm/hooks/pre-commit").exists()
    assert {k: v for k, v in snapshot(existing).items() if k.startswith(".beads/")} == beads, ".beads/ stays"
    assert git(existing, "config", "core.hooksPath").strip() == str(existing / ".pm/hooks")
    assert doctor(existing)[0] == 0, doctor(existing)[1]
    after = snapshot(existing)
    again = pm(existing, "upgrade")
    assert again.returncode == 0 and again.stdout == f"pm {v}: every managed piece is current; nothing to commit\n", \
        again.stdout
    assert snapshot(existing) == after and git(existing, "config", "core.hooksPath").strip() == str(existing / ".pm/hooks")

    git(existing, "add", "-A")  # the committed upgrade, through pm's hooks in .pm/hooks
    res = subprocess.run(["git", "commit", "-qm", "Upgrade pm"], cwd=existing, env=env(existing.parent),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
