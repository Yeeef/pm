"""pm doctor, pm upgrade and pm uninstall in the temp repos of test_init (a local bare remote and the
fake launchd/systemd in tmp/home): init and upgrade keep what is not pm's, doctor names each hand-made change,
upgrade moves the pin and rewrites pm's parts only, uninstall removes pm's parts and setup only."""

from __future__ import annotations

import fcntl
import json
import os
import signal
import socket
import subprocess
import tomllib
from pathlib import Path

import pytest

from conftest import PM, VERSION, stop_services
from test_init import (BEADS_HOOK, CLAUDE_PM, GITIGNORE_BLOCK, HOOKS, PM_FILES, USER_CODEX,  # noqa: F401
                       USER_SETTINGS, brand_new_repo, commands, env, existing, git, new_repo, pm, pm_free, snapshot)

pytestmark = pytest.mark.integration  # each test runs pm init in a fresh repo with a remote and starts its service

LOCAL_SETTINGS = '{\n  "model": "café"\n}\n'  # non-ASCII: pm rewrites the file around it, byte for byte
CODEX_USER = '# mine\nmodel = "o3"\n\n[sandbox_workspace_write]\nnetwork_access = true\n'


# pm's roots in Codex's writable_roots, per clone: .git, the records store and its git dir, the work store and
# .pm/run, which holds the work store's socket
CLONE_ROOTS = 5


MINE = "\n# mine\necho done\n"  # the existing repo's own lines after Beads' section in its hook files


def old_hook_file(name: str, version: str) -> str:
    """An earlier pm's git hook file, which held its section alone."""
    return f'#!/usr/bin/env sh\n# --- BEGIN PM v{version} ---\npm hook git-{name} "$@" || exit $?\n# --- END PM ---\n'


HOOKS_RETIRED = "retired with pm's git hooks, as session start sets each new worktree up"


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


def notes(lines: list[str]) -> list[str]:
    """lines without the note that Codex's trust was not checked: CODEX_HOME (tmp/codex) is absent in these tests."""
    return [line for line in lines if not (line.startswith("codex: ") and line.endswith("was not checked"))]


def doctor(repo: Path) -> tuple[int, list[str]]:
    """pm doctor's exit code and its lines on the clone, without its notes, which leave the exit code alone: its line
    on the latest release, which no test here can read (conftest's NO_RELEASE_API), and that Codex's trust was not
    checked."""
    res = pm(repo, "doctor")
    lines = res.stdout.splitlines()
    assert lines[-1].startswith("release: cannot read pm's latest release"), res.stdout
    return res.returncode, notes(lines[:-1])


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
    "retired git hook": (lambda r: write(r / f"{HOOKS}/post-checkout", old_hook_file("post-checkout", "0.9.1")),
                         f"{HOOKS}/post-checkout: an earlier pm's file, {HOOKS_RETIRED}"),
    "gitignore block": (lambda r: edit(r / ".gitignore", "/records\n", "/records/\n"),
                        ".gitignore: pm's part differs"),
}


def reported(lines: list[str], wants: list[str]) -> bool:
    """Doctor gave one line per change: each line starts with exactly one change's expected start, in any order."""
    return (sorted(w for line in lines for w in wants if line.startswith(w)) == sorted(wants)
            and len(lines) == len(wants))


def test_doctor_reports_each_changed_repo_piece_and_upgrade_restores_it(new_repo: Path, tmp_path: Path):
    """Every change at once, so one init serves them all (each init costs seconds); each still gets its own line."""
    (tmp_path / "codex").mkdir()  # Codex in use, for the clone setup's codex roots below
    assert pm(new_repo, "init").returncode == 0
    assert doctor(new_repo)[0] == 0
    for change, _ in REPO_CHANGES.values():
        change(new_repo)
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, [f"repo: {want}" for _, want in REPO_CHANGES.values()]), lines
    assert all(line.endswith(f"run pm upgrade --to {VERSION} to rewrite it") for line in lines), lines
    res = pm(new_repo, "upgrade")
    assert res.returncode == 0 and "pin stays" in res.stdout, res.stderr
    assert not (new_repo / ".github/workflows/pm-records-copy.yml").exists()
    assert not (new_repo / HOOKS).exists(), "the retired hook file goes, and its directory with it"
    assert doctor(new_repo)[0] == 0

    # folded: test_a_beads_file_is_no_beads_for_doctor_and_upgrade
    # a repo that retired Beads by replacing .beads/ with a file (which blocks every bd command) holds no Beads
    # pieces, as a repo without .beads/: pm doctor is clean and pm upgrade --to the pin finds every piece current
    (new_repo / ".beads").write_text("Beads is retired here.\n")
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Retire Beads")
    res = pm(new_repo, "doctor")
    assert res.returncode == 0, res.stdout + res.stderr
    res = pm(new_repo, "upgrade", "--to", VERSION)
    assert res.returncode == 0, res.stderr
    assert res.stdout == f"pm {VERSION}: every managed piece is current; nothing to commit\n", res.stdout

    # folded: test_doctor_reports_each_changed_clone_setup
    # every change at once, so one init serves them all; each still gets its own line
    for change, _ in SETUP_CHANGES.values():
        change(new_repo, tmp_path)
    code, lines = doctor(new_repo)
    assert code == 1 and reported(lines, [want for _, want in SETUP_CHANGES.values()]), lines


SETUP_CHANGES = {  # each hand-made change to the clone's setup and the start of the line doctor reports for it
    "records link": (lambda r, tmp: (r / "records").unlink(), "records link: "),
    "sparse checkout": (lambda r, tmp: git(r, "sparse-checkout", "set", "--no-cone", "/*", "!/records/"),
                        "sparse checkout: "),
    "hooks path": (lambda r, tmp: git(r, "config", "core.hooksPath", str(r / HOOKS)),
                   "hooks path: core.hooksPath is "),  # its line names the path: TestRetireHooksPath has the text
    "service": (lambda r, tmp: [p.unlink() for p in sched_units(tmp)], "service: not installed"),
    "codex roots": (lambda r, tmp: (tmp / "codex/config.toml").write_text(CODEX_USER), "codex: "),
    "git exclude": (lambda r, tmp: edit(r / ".git/info/exclude", "/.pm/run/\n", ""), "git exclude: "),
}


def test_init_and_upgrade_keep_what_is_not_pms(existing: Path, tmp_path: Path):
    """pm init into a repo with Beads and settings of its own, then pm upgrade from an older pin: each rewrites pm's
    parts only, around the repo's own content, byte for byte."""
    before = snapshot(existing)
    res = pm(existing, "init")
    assert res.returncode == 0, res.stderr
    assert f"git add -- {' '.join(PM_FILES)} && " in res.stdout, res.stdout
    assert not (existing / HOOKS).exists(), "pm installs no git hook"
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
    # the repo as an older pm left it: an older pin, its post-checkout hook file and core.hooksPath naming it
    edit(existing / ".pm/config.toml", f'version = "{VERSION}"', 'version = "0.2.0"')
    write(existing / f"{HOOKS}/post-checkout", old_hook_file("post-checkout", "0.2.0"))
    (existing / f"{HOOKS}/post-checkout").chmod(0o755)
    git(existing, "add", "-A")
    git(existing, "commit", "--no-verify", "-qm", "pm 0.2.0")
    git(existing, "config", "core.hooksPath", str(existing / HOOKS))
    def launched(pin: str, *args: str) -> subprocess.CompletedProcess:  # as the installed pm launches `pin`, whose
        # release here builds this pm: test_launch.py has the launch itself, which would reach GitHub
        return subprocess.run([*PM, *args], cwd=existing, env=dict(env(existing.parent), PM_LAUNCHED=pin),
                              capture_output=True, text=True, check=False)
    assert launched("0.2.0", "show").returncode == 1, "every other command refuses the old pin"
    res = launched("9.9.9", "upgrade", "--to", "9.9.9")
    latest = "curl -fsSL https://github.com/Yeeef/pm/releases/latest/download/install.sh | sh"
    assert res.returncode == 1 and res.stderr.endswith(  # the launcher's install, never an old pm in its place
        f'which launches pm 9.9.9: install the latest with {latest}\n'), res.stderr
    head = git(existing, "rev-parse", "HEAD")
    res = pm(existing, "upgrade")
    assert res.returncode == 0, res.stderr
    assert f"moved the pin from 0.2.0 to {VERSION}" in res.stdout
    assert f"removed {HOOKS}/post-checkout, {HOOKS_RETIRED}" in res.stdout
    assert f"unset core.hooksPath={existing / HOOKS}, {HOOKS_RETIRED}" in res.stdout
    assert f"git add -- .pm/config.toml {HOOKS}/post-checkout && " in res.stdout
    assert git(existing, "rev-parse", "HEAD") == head, "pm upgrade commits nothing"
    assert tomllib.loads((existing / ".pm/config.toml").read_text())["version"] == VERSION
    assert not (existing / HOOKS).exists()
    assert git(existing, "config", "--get", "--default=", "core.hooksPath") == "\n"
    assert git(existing, "status", "--porcelain").split() == ["M", ".pm/config.toml", "D", f"{HOOKS}/post-checkout"]
    after = snapshot(existing)
    assert all(after[k] == v for k, v in before.items() if k not in PM_FILES), "files pm does not manage are kept"
    again = pm(existing, "upgrade")
    assert again.returncode == 0 and "nothing to commit" in again.stdout and snapshot(existing) == after

    # folded: test_init_leaves_an_installed_repos_files_alone_and_doctor_names_upgrade
    # once .pm/config.toml exists, pm init (session start runs it) does only the clone's half: a branch that changed
    # pm's hook entries keeps its change, and pm doctor names pm upgrade --to the pin as the fix
    git(existing, "add", "-A")
    git(existing, "commit", "-qm", "Install pm")
    git(existing, "checkout", "-q", "-b", "changed")
    settings = existing / ".claude/settings.json"
    edit(settings, '"pm hook stop || exit 1"', '"pm hook stop"')
    git(existing, "commit", "-qam", "change pm's pieces")
    changed = settings.read_bytes()
    res = pm(existing, "init")
    assert res.returncode == 0, res.stderr
    assert settings.read_bytes() == changed
    assert git(existing, "status", "--porcelain") == "" and "git add" not in res.stdout, res.stdout
    code, lines = doctor(existing)
    assert code == 1 and reported(lines, ["repo: .claude/settings.json: pm's part differs"]), lines
    assert all(line.endswith(f"run pm upgrade --to {VERSION} to rewrite it") for line in lines), lines
    # --site-url is the one repo write a later pm init makes, on request; a bad URL is refused before any write
    res = pm(existing, "init", "--site-url", "pm.example.com")
    assert res.returncode == 1 and "is not an http(s) base URL" in res.stderr and git(existing, "status", "--porcelain") == ""
    res = pm(existing, "init", "--site-url", "https://pm.example.com/")
    assert res.returncode == 0 and "git add -- .pm/config.toml && " in res.stdout, res.stdout
    assert tomllib.loads((existing / ".pm/config.toml").read_text())["site_url"] == "https://pm.example.com"
    assert settings.read_bytes() == changed
    # a site_url needs the Access keys: doctor names the missing ones, and the config holding them is pm's own
    code, lines = doctor(existing)
    assert code == 1 and "config: site_url is set, but .pm/config.toml lacks access_team, access_aud, so the site " \
                         "refuses every request to its host; set what is missing there (pm init --help)" in lines, lines
    with (existing / ".pm/config.toml").open("a") as f:
        f.write('access_team = "team.cloudflareaccess.com"\naccess_aud = "aud-tag"\n')
    code, lines = doctor(existing)
    assert code == 1 and reported(lines, ["repo: .claude/settings.json: pm's part differs"]), lines


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
                         capture_output=True, text=True, check=False)
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
        "--sparse --ignore-unmatch records and commit, or merge the main branch once it has"]), lines
    res = pm(new_repo, "init")
    assert res.returncode == 0 and "sparse" not in res.stdout, res.stdout
    assert "!/records/" in git(new_repo, "sparse-checkout", "list").split(), "kept while HEAD tracks the copy"

    res = pm(new_repo, "upgrade")
    assert res.returncode == 0, res.stderr
    commit = (f"git add -- {' '.join(RETIRED_WORKFLOWS)} {HOOKS}/pre-commit && git rm -r -q --cached --sparse "
              f'--ignore-unmatch records && git commit -m "Upgrade pm to {VERSION}"')
    assert res.stdout.splitlines() == [
        f"pin stays {VERSION}",
        *(f"removed {rel}, {retired}" for rel in RETIRED_WORKFLOWS),
        f"removed {HOOKS}/pre-commit, {retired}",
        "this branch tracks records/, a copy no branch keeps now; the commit below untracks it",
        f"pm commits nothing on main; commit pm's files there: {commit}"], res.stdout
    assert not any((new_repo / rel).exists() for rel in [*RETIRED_WORKFLOWS, f"{HOOKS}/pre-commit"])
    git(new_repo, "rm", "-r", "-q", "--cached", "--sparse", "records")  # an interrupted first try: the commit still runs
    res = subprocess.run(["sh", "-c", commit], cwd=new_repo, env=env(tmp_path), capture_output=True, text=True,
                         check=False)
    assert res.returncode == 0, res.stderr
    assert git(new_repo, "ls-tree", "HEAD", "records") == ""
    assert (new_repo / "records").is_symlink(), "untracking the copy leaves the link"

    res = pm(new_repo, "init")
    assert res.returncode == 0 and f"turned off the sparse checkout an earlier pm set in {new_repo}" in res.stdout, \
        res.stdout
    for key in ("core.sparseCheckout", "core.sparseCheckoutCone", "sparse.expectFilesOutsideOfPatterns"):
        assert git(new_repo, "config", "--get", "--default=", key) == "\n", key
    assert not Path(git(new_repo, "rev-parse", "--path-format=absolute", "--git-path", "info/sparse-checkout").strip()).exists()
    assert (new_repo / "records").is_symlink() and (new_repo / "records/sprints").is_dir()
    assert git(new_repo, "status", "--porcelain") == ""
    code, lines = doctor(new_repo)
    assert code == 0, lines


def install_everywhere(repo: Path, tmp: Path) -> Path:
    """Codex and Claude Code in use with config of their own, pm installed, and a second worktree set up by session
    start's pm init."""
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
    git(repo, "worktree", "add", "-q", "-b", "feature", str(wt))
    res = pm(wt, "init", "--session-start")
    assert res.returncode == 0 and (wt / "records").is_symlink(), res.stdout + res.stderr
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

    # folded: test_uninstall_keeps_other_clones_roots
    # two clones set up on one machine of their own: uninstalling one removes only its own roots (CLONE_ROOTS); the
    # other clone's roots stay, and every other byte of config.toml with them. Roots it cannot take out without
    # breaking the TOML are refused before anything changes
    tmp = tmp_path / "two"
    tmp.mkdir()
    repo = brand_new_repo(tmp)
    try:
        (tmp / "codex").mkdir()
        config = tmp / "codex/config.toml"
        config.write_text(CODEX_USER)
        assert pm(repo, "init").returncode == 0
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "Install pm")
        git(repo, "push", "-q", "origin", "main")
        other = tmp / "other"
        git(tmp, "clone", "-q", str(tmp / "remote.git"), str(other))
        with socket.socket() as s:  # a second clone on this machine gets its own site port
            s.bind(("127.0.0.1", 0))
            port = s.getsockname()[1]
        res = subprocess.run([*PM, "init"], cwd=other, env=dict(env(tmp), PORT=str(port)), capture_output=True,
                             text=True, check=False)
        assert res.returncode == 0, res.stderr
        roots = tomllib.loads(config.read_text())["sandbox_workspace_write"]["writable_roots"]
        mine = [r for r in roots if r.startswith(str(repo.resolve()))]
        theirs = [r for r in roots if r.startswith(str(other.resolve()))]
        assert len(mine) == len(theirs) == CLONE_ROOTS and len(roots) == 2 * CLONE_ROOTS, roots
        # writable_roots spread over lines, one root a line: taking a root out leaves its comma, which is not TOML;
        # uninstall names the file and says to remove the roots by hand, before it changes anything
        flat = config.read_text()
        spread = "writable_roots = [\n" + "".join(f"  {json.dumps(r)},\n" for r in roots) + "]\n"
        config.write_text(CODEX_USER.replace("[sandbox_workspace_write]\n", f"[sandbox_workspace_write]\n{spread}"))
        codex, before, units = config.read_text(), snapshot(repo), sched_units(tmp)
        res = pm(repo, "uninstall")
        assert res.returncode == 1, res
        assert f"editing {config} would leave it invalid TOML (" in res.stderr, res.stderr
        assert "remove these from its writable_roots by hand: " in res.stderr and json.dumps(mine[0]) in res.stderr
        assert config.read_text() == codex and snapshot(repo) == before and sched_units(tmp) == units != []
        assert (repo / ".pm/store/records").is_dir() and (repo / "records").is_symlink()
        config.write_text(flat)
        res = pm(repo, "uninstall")
        assert res.returncode == 0, res.stderr
        assert f"removed this clone's writable_roots from {config}" in res.stdout, res.stdout
        kept = [r for r in roots if r not in mine]
        assert config.read_text() == CODEX_USER.replace(
            "[sandbox_workspace_write]\n",
            f"[sandbox_workspace_write]\nwritable_roots = [{', '.join(json.dumps(r) for r in kept)}]\n")
    finally:
        stop_services(tmp)

    # folded: test_uninstall_disables_an_enabled_unit_that_is_not_running
    # a unit the supervisor still starts at login but that is not running now (it crashed) is disabled by pm uninstall
    # with the rest, so no wants link outlives its unit file; on a clone and machine of its own
    tmp = tmp_path / "crash"
    tmp.mkdir()
    repo = brand_new_repo(tmp)
    try:
        assert pm(repo, "init").returncode == 0
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "Install pm")
        state_path = tmp / "sched.json"
        state = json.loads(state_path.read_text())
        [name] = state["loaded"]
        assert name in state["enabled"]
        os.kill(state["pids"].pop(name), signal.SIGKILL)  # the service crashes: inactive, still enabled
        state["loaded"].remove(name)
        state_path.write_text(json.dumps(state))
        res = pm(repo, "uninstall")
        assert res.returncode == 0, res.stderr
        assert "removed the pm service" in res.stdout, res.stdout
        assert json.loads(state_path.read_text())["enabled"] == [] and sched_units(tmp) == []
    finally:
        stop_services(tmp)


def test_doctor_names_a_settings_file_it_cannot_read_as_a_repo_finding(new_repo: Path):
    """A settings file that is not JSON hides any pre-package harness piece in it: doctor says it could not look,
    labelled as the repo finding it is, not as a legacy piece."""
    assert pm(new_repo, "init").returncode == 0
    (new_repo / ".claude/settings.json").write_text("{not json")
    code, lines = doctor(new_repo)
    assert code == 1 and any(line.startswith("repo: cannot look for the pre-package harness's pieces: "
                                             ".claude/settings.json is not valid JSON") for line in lines), lines
    assert not any(line.startswith("legacy: ") for line in lines), lines




def answers(sock: Path) -> bool:
    """Whether a process accepts connections on the work store's socket."""
    with socket.socket(socket.AF_UNIX) as s:
        try:
            s.connect(str(sock))
            return True
        except OSError:
            return False


def supervisor_calls(tmp: Path) -> list[list[str]]:
    path = tmp / "sched.log"
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []


def test_stop_keeps_the_service_stopped_at_session_start_until_restart(new_repo: Path, tmp_path: Path):
    """pm service stop disables the unit and stops it, and says the socket no longer answers; session start leaves it
    stopped and names pm service restart; pm service restart brings it back."""
    assert pm(new_repo, "init").returncode == 0
    sock = new_repo / ".pm/run/work.sock"
    assert loaded(tmp_path) and answers(sock)
    res = pm(new_repo, "service", "stop")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("stopped the pm service: ") and f"socket {sock}" in res.stdout, res.stdout
    assert loaded(tmp_path) == [] and not answers(sock)
    status = pm(new_repo, "service", "status")
    assert status.returncode == 1 and "stopped by pm service stop" in status.stdout.splitlines()[0], status.stdout
    n = len(supervisor_calls(tmp_path))
    res = pm(new_repo, "init", "--session-start")
    assert res.returncode == 0, res.stderr
    assert "left the pm service stopped: it was stopped by pm service stop" in res.stdout, res.stdout
    assert "run pm service restart to start it" in res.stdout
    started = [c for c in supervisor_calls(tmp_path)[n:] if {"enable", "restart", "bootstrap", "kickstart"} & set(c)]
    assert started == [] and loaded(tmp_path) == [] and not answers(sock), started
    show = pm(new_repo, "show")
    assert show.returncode == 1 and "run pm service restart" in show.stderr, show
    res = pm(new_repo, "service", "restart")
    assert res.returncode == 0, res.stderr
    assert loaded(tmp_path) and answers(sock)
    assert pm(new_repo, "show").returncode == 0
    assert "stopped" not in pm(new_repo, "service", "status").stdout.splitlines()[0]


def test_uninstall_with_the_service_stopped_checks_for_unsynced_work(new_repo: Path, tmp_path: Path):
    """With the clone's service stopped, pm uninstall runs a pm service run of its own for the unsynced-work check:
    refused while the store holds a commit the remote lacks, done once it is synced; nothing is left running."""
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    res = pm(new_repo, "project", "open", "demo", "--title", "Demo", "--text=A goal.")
    assert res.returncode == 0, res.stderr
    assert pm(new_repo, "service", "stop").returncode == 0
    sock = new_repo / ".pm/run/work.sock"
    res = pm(new_repo, "uninstall")
    assert res.returncode == 1 and "holds 1 commit(s) origin's refs/pm/work lacks" in res.stderr, res
    assert (new_repo / ".pm/store/work").is_dir() and sched_units(tmp_path) and not answers(sock)
    assert pm(new_repo, "service", "restart").returncode == 0
    assert pm(new_repo, "sync").returncode == 0
    assert pm(new_repo, "service", "stop").returncode == 0
    res = pm(new_repo, "uninstall")
    assert res.returncode == 0, res.stderr
    assert "removed the pm service" in res.stdout, res.stdout
    assert not (new_repo / ".pm").exists() and sched_units(tmp_path) == [] and not answers(sock)


def test_uninstall_waits_for_a_session_start_that_holds_the_install_lock(new_repo: Path, tmp_path: Path):
    """A session start that (re)starts the service holds the clone's install lock; pm uninstall takes it too, from its
    unsynced-work check to the unit's and the store's removal, so no session start brings the service back in between."""
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    with open(new_repo / ".pm/run/install.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        proc = subprocess.Popen([*PM, "uninstall"], cwd=new_repo, env=env(tmp_path), stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        with pytest.raises(subprocess.TimeoutExpired):
            proc.wait(timeout=3)
        assert sched_units(tmp_path) and (new_repo / ".pm/store/work").is_dir()
    out, err = proc.communicate(timeout=60)
    assert proc.returncode == 0, err
    assert "removed the pm service" in out and sched_units(tmp_path) == [] and not (new_repo / ".pm").exists()


def test_doctor_and_init_name_the_pm_hooks_codex_skips(new_repo: Path, tmp_path: Path):
    """pm doctor asks Codex (here the fake codex app-server) which of pm's hooks it would skip, and names each with
    the fix, failing; pm init (not at session start) names them too. Without codex on PATH, doctor says the trust was
    not checked and does not fail on it. It fails on a missing CLI only for summary_runtime's runtime, which the pm
    service summarizes with: a machine with Codex alone passes once the repo summarizes on codex."""
    (tmp_path / "codex").mkdir()  # Codex ran on this machine: its home exists, so doctor asks it
    assert pm(new_repo, "init").returncode == 0
    git(new_repo, "add", "-A")
    git(new_repo, "commit", "-qm", "Install pm")
    hooks = new_repo / ".codex/hooks.json"
    fix = f"; open Codex in {new_repo} and trust pm's hooks in /hooks"
    trust = tmp_path / "trust.json"

    def run(*args, **extra):
        return subprocess.run([*PM, *args], cwd=new_repo, env=dict(env(tmp_path), **extra), capture_output=True,
                              text=True, check=False)

    trust.write_text(json.dumps({"stop:0:1": "untrusted", "subagent_stop:0:0": "modified"}))
    res = run("doctor", FAKE_CODEX_TRUST=str(trust))
    assert res.returncode == 1, res.stdout + res.stderr
    lines = res.stdout.splitlines()
    assert lines[:2] == [
        f"codex: Codex skips 1 of pm's 13 hooks in {hooks}, which you have not trusted: stop `pm hook stop || exit 1`"
        + fix,
        f"codex: Codex skips 1 of pm's 13 hooks in {hooks}, which changed since you trusted them: subagentStop "
        f"`pm hook stop || exit 1`" + fix], res.stdout
    assert lines[-1].startswith("release: cannot read pm's latest release"), res.stdout
    res = run("init", FAKE_CODEX_TRUST=str(trust))
    assert res.returncode == 0 and res.stdout.splitlines()[-2:] == lines[:2], res.stdout
    trust.write_text(json.dumps({"project": "untrusted"}))
    res = run("doctor", FAKE_CODEX_TRUST=str(trust))
    assert res.returncode == 1 and res.stdout.splitlines()[0] == (
        f"codex: Codex runs none of pm's hooks in {hooks}: it lists none of them, as it does until you trust the "
        f"project; open Codex in {new_repo}, trust the project, then trust pm's hooks in /hooks"), res.stdout
    assert run("doctor").returncode == 0  # the fake trusts every hook
    # neither codex nor claude on PATH: the other fakes in a bin dir of their own, no dir that holds either
    fakes = tmp_path / "fakes-but-agents"
    fakes.mkdir()
    for tool in (tmp_path / "bin").iterdir():
        if tool.name not in ("codex", "claude"):
            (fakes / tool.name).symlink_to(tool)
    path = os.pathsep.join([str(fakes)] + [d for d in env(tmp_path)["PATH"].split(os.pathsep)
                                           if not (Path(d) / "codex").exists() and not (Path(d) / "claude").exists()])
    res = run("doctor", PATH=path)
    lines = res.stdout.splitlines()
    assert res.returncode == 1 and lines[0] == (
        "claude: not on PATH; summary_runtime in .pm/config.toml is claude, so pm day summarize, which the pm service "
        "runs every 10 minutes, fails; install Claude Code and log in, or set summary_runtime to the runtime this "
        "machine has"), res.stdout
    assert lines[1] == "codex: codex is not on PATH, so whether Codex runs pm's hooks was not checked", res.stdout
    assert len(lines) == 3 and lines[2].startswith("release: "), res.stdout
    # the repo summarizes on codex: a machine without claude passes, one without codex fails naming it
    config = new_repo / ".pm/config.toml"
    config.write_text(config.read_text() + 'summary_runtime = "codex"\n')
    codex_only = tmp_path / "codex-only"
    codex_only.mkdir()
    (codex_only / "codex").symlink_to(tmp_path / "bin/codex")
    res = run("doctor", PATH=os.pathsep.join([str(codex_only), path]))
    assert res.returncode == 0, res.stdout
    res = run("doctor", PATH=path)
    assert res.returncode == 1 and res.stdout.splitlines()[0].startswith(
        "codex: not on PATH; summary_runtime in .pm/config.toml is codex, so pm day summarize"), res.stdout
    # a pm service installed from that shell has no codex to queue owner replies into Codex sessions with
    assert run("service", "install", PATH=path).returncode == 0
    res = run("doctor")
    assert res.returncode == 1 and ("service: codex is not on the pm service's PATH, so owner replies cannot be "
                                    "queued into Codex sessions (pm show flags them instead); install codex "
                                    "0.149.0 or later and run pm service install from a shell where it is on PATH"
                                    ) in res.stdout.splitlines(), res.stdout


def test_init_with_a_path_of_its_own_leaves_the_running_service_alone(new_repo: Path, tmp_path: Path):
    """Codex runs every command with its own directory first on PATH: a typed pm init from there must not rewrite the
    unit or restart the service under every live session. A real difference (the port) still restarts it, and
    pm service install takes the PATH it runs with."""
    assert pm(new_repo, "init").returncode == 0
    units = sched_units(tmp_path)
    before = {u: u.read_bytes() for u in units}
    log = tmp_path / "sched.log"
    calls = len(log.read_text().splitlines())
    codex = tmp_path / "codex-path"
    codex.mkdir()
    base = env(tmp_path)
    res = subprocess.run([*PM, "init"], cwd=new_repo, env=dict(base, PATH=f"{codex}{os.pathsep}{base['PATH']}"),
                         capture_output=True, text=True, check=False)
    assert res.returncode == 0, res.stderr
    new = [json.loads(line) for line in log.read_text().splitlines()[calls:]]
    assert not [c for c in new if set(c) & {"restart", "daemon-reload", "bootstrap", "bootout", "kickstart"}], new
    assert {u: u.read_bytes() for u in units} == before
    res = subprocess.run([*PM, "service", "install"], cwd=new_repo,
                         env=dict(base, PATH=f"{codex}{os.pathsep}{base['PATH']}"), capture_output=True, text=True,
                         check=False)
    assert res.returncode == 0 and res.stdout.startswith("updated the pm service"), res.stdout + res.stderr
    assert str(codex).encode() in b"".join(u.read_bytes() for u in units)
