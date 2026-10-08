"""pm init on a clone laid out as the project-management harness left yeeef-agents (bin/pm, the RULES.md import,
hook entries by path, unmarked git-hook code, the old workflow names and .gitignore lines, the store at .records, the
scheduled push job): it migrates each legacy piece, keeps every other byte, and pm doctor is clean after. Expected
contents are written out from the design (pm-product: "Migrating yeeef-agents"), not taken from pm's code."""

from __future__ import annotations

import hashlib
import json
import plistlib
import sys
from pathlib import Path

import pytest

from test_init import BEADS_HOOK, CLAUDE_PM, GITIGNORE_BLOCK, commands, env, git, pm, section, snapshot

pytestmark = pytest.mark.integration  # a clone with a remote; init moves the store and starts the service

CLAUDE_MD = """# CLAUDE.md

Guidance.

## Project management

@skills/project-management/harness/RULES.md

Runtimes that do not expand `@` imports (such as Codex): read `skills/project-management/harness/RULES.md` before working; its rules are binding here.

## Beads Issue Tracker

Use bd.
"""
CLAUDE_MD_AFTER = "# CLAUDE.md\n\nGuidance.\n\n## Project management\n\n## Beads Issue Tracker\n\nUse bd.\n"

BIN = '"$CLAUDE_PROJECT_DIR"/bin/pm'
CODEX_BIN = 'f="$(git rev-parse --show-toplevel)/bin/pm"; [ -f "$f" ] || { echo "pm hook: $f is missing" >&2; exit 1; }; "$f"'


def hook(command: str, **extra) -> dict:
    return {"command": command, "type": "command", **extra}


CLAUDE_SETTINGS = {
    "hooks": {
        "SessionStart": [{"hooks": [hook("bd prime --hook-json"), hook(f"{BIN} prime --rules --hook-json", timeout=30),
                                    hook(f"{BIN} prime --state --hook-json", timeout=30)], "matcher": ""}],
        "SubagentStart": [{"hooks": [*(hook(f"{BIN} prime --rules {n} --hook-json", timeout=15) for n in range(1, 5)),
                                     hook(f"{BIN} prime --subagent --hook-json", timeout=15)], "matcher": ""}],
        "PostToolUse": [{"hooks": [hook('python3 "$CLAUDE_PROJECT_DIR"/skills/project-management/harness/'
                                        'reply_wait_hook.py')], "matcher": "Bash"}],
        "PreToolUse": [{"hooks": [hook("./lint.sh")], "matcher": "Bash"}],
        "Stop": [{"hooks": [hook(f"{BIN} hook owner-request || exit 1", timeout=30),
                            hook(f"{BIN} hook stop || exit 1", timeout=10)]}],
    },
    "worktree": {"bgIsolation": "none"},
}
CODEX_HOOKS = {
    "hooks": {
        "SessionStart": [{"hooks": [hook("bd codex-hook SessionStart"),
                                    hook(f"{CODEX_BIN} prime --rules --hook-json", timeout=30)],
                          "matcher": "startup|resume|clear"}],
        "Stop": [{"hooks": [hook('f="$(git rev-parse --show-toplevel)/skills/project-management/harness/'
                                 'uncommitted_records_hook.py"; [ -f "$f" ] || exit 0; python3 "$f"')]}],
    },
}
OLD_POST_CHECKOUT = ("\n# A new worktree or clone (previous HEAD is the null id) links its records/ to the shared store,\n"
                     "# whichever tool created it (records/design/records-store.md).\n"
                     'if [ "$1" = 0000000000000000000000000000000000000000 ] && [ -x bin/pm ]; then\n'
                     "  bin/pm setup >&2 || exit $?\nfi\n")
OLD_PRE_COMMIT = ("\n# Records live on the records branch (records/design/records-store.md): a code-branch commit must "
                  "not edit records/.\n"
                  'if [ ! -f "$(git rev-parse --git-dir)/MERGE_HEAD" ] && [ -n "$(git diff --cached --name-only -- '
                  'records/)" ]; then\n'
                  '  echo >&2 "error: this commit edits records/, which only the records branch may change; write '
                  'records with bin/pm"\n'
                  '  echo >&2 "and unstage these edits: git restore --staged records/"\n  exit 1\nfi\n')
MINE = "\n# mine\necho done\n"
GITIGNORE = ("*.log\nsite/\n\n# Agent worktrees\n.claude/worktrees/\n\n"
             "# Records live on the records branch, checked out at .records; records is a link to it (bin/pm setup)\n"
             "/records\n/.records/\n\n# mine\n/pm/.venv/\n"
             "# Claude Code's per-machine settings; pm setup adds the records store to them\n"
             "/.claude/settings.local.json\n")
GITIGNORE_AFTER = "*.log\n\n# Agent worktrees\n.claude/worktrees/\n\n# mine\n/pm/.venv/\n" + GITIGNORE_BLOCK
OLD_WORKFLOWS = [".github/workflows/records-guard.yml", ".github/workflows/records-copy.yml"]


def label(main: Path) -> str:
    return f"local.pm-push.{main.name}.{hashlib.sha1(str(main.resolve()).encode()).hexdigest()[:8]}"


def old_job(tmp: Path, main: Path) -> list[Path]:
    """The scheduled push job as the old bin/pm setup installed it for this platform, held by the fake supervisor."""
    home, name = tmp / "home", label(main)
    if sys.platform == "darwin":
        files = [home / "Library/LaunchAgents" / f"{name}.plist"]
        files[0].parent.mkdir(parents=True, exist_ok=True)
        files[0].write_bytes(plistlib.dumps({"Label": name, "ProgramArguments": [str(main / "bin/pm"), "push"],
                                             "StartInterval": 600, "WorkingDirectory": str(main)}))
    else:
        d = home / ".config/systemd/user"
        d.mkdir(parents=True, exist_ok=True)
        files = [d / f"{name}.service", d / f"{name}.timer"]
        files[0].write_text(f'[Service]\nType=oneshot\nExecStart="{main / "bin/pm"}" push\n')
        files[1].write_text("[Timer]\nOnUnitActiveSec=600s\n")
    state = tmp / "sched.json"
    data = json.loads(state.read_text()) if state.exists() else {"loaded": [], "pids": {}}
    data["loaded"].append(name)
    state.write_text(json.dumps(data))
    return files


def loaded(tmp: Path) -> list[str]:
    return json.loads((tmp / "sched.json").read_text())["loaded"]


@pytest.fixture
def legacy_clone(tmp_path: Path) -> Path:
    """A clone of a repo the harness set up: its tracked legacy pieces on main, a records branch on the remote, the
    store at .records with every worktree linking it, and the old push job with its state."""
    env(tmp_path)  # the fakes and the temp HOME first: the old job lives under it
    git(tmp_path, "init", "-q", "--bare", "remote.git")
    seed = tmp_path / "seed"
    git(tmp_path, "init", "-q", "-b", "records", str(seed))
    (seed / "sprints").mkdir()
    (seed / "sprints/a-1.md").write_text("one\n")
    git(seed, "add", "-A")
    git(seed, "commit", "-qm", "records")
    git(seed, "checkout", "-q", "--orphan", "main")
    git(seed, "rm", "-rqf", ".")
    (seed / "CLAUDE.md").write_text(CLAUDE_MD)
    (seed / "AGENTS.md").symlink_to("CLAUDE.md")
    for rel, data in ((".claude/settings.json", CLAUDE_SETTINGS), (".codex/hooks.json", CODEX_HOOKS)):
        (seed / rel).parent.mkdir()
        (seed / rel).write_text(json.dumps(data, indent=2) + "\n")
    (seed / ".beads/hooks").mkdir(parents=True)
    for name, tail in (("post-checkout", OLD_POST_CHECKOUT), ("pre-commit", OLD_PRE_COMMIT + MINE)):
        (seed / f".beads/hooks/{name}").write_text(BEADS_HOOK + tail)
        (seed / f".beads/hooks/{name}").chmod(0o755)
    (seed / ".beads/config.yaml").write_text("# beads\n")
    (seed / ".github/workflows").mkdir(parents=True)
    for rel in OLD_WORKFLOWS:
        (seed / rel).write_text("name: old\n")
    (seed / ".github/workflows/ci.yml").write_text("name: ci\n")
    (seed / ".gitignore").write_text(GITIGNORE)
    git(seed, "add", "-A")
    git(seed, "commit", "-qm", "code")
    git(seed, "remote", "add", "origin", str(tmp_path / "remote.git"))
    git(seed, "push", "-q", "origin", "main", "records")
    main = tmp_path / "clone"
    git(tmp_path, "clone", "-q", str(tmp_path / "remote.git"), str(main))
    # what the old bin/pm setup made: hooks path, the store at .records, the records links, sparse checkout,
    # Claude Code's and Codex's paths, the push job and its state
    git(main, "config", "core.hooksPath", ".beads/hooks")
    git(main, "branch", "-q", "--track", "records", "origin/records")
    old = main / ".records"
    git(main, "worktree", "add", "-q", str(old), "records")
    wt = tmp_path / "wt"
    git(main, "worktree", "add", "-q", "-b", "feature", str(wt))
    (tmp_path / "claude").mkdir()
    for tree in (main, wt):
        git(tree, "sparse-checkout", "set", "--no-cone", "/*", "!/records/")
        git(tree, "config", "--worktree", "sparse.expectFilesOutsideOfPatterns", "true")
        (tree / "records").symlink_to(old)
    (wt / ".claude/settings.local.json").write_text(
        json.dumps({"permissions": {"additionalDirectories": ["/elsewhere", str(old)]}, "model": "x"}, indent=2) + "\n")
    (tmp_path / "codex").mkdir()
    (tmp_path / "codex/config.toml").write_text(
        f'model = "o3"\n\n[sandbox_workspace_write]\nwritable_roots = ["/mine", {json.dumps(str(old.resolve()))}]\n')
    for f in ("pm-push.json", "pm-push.log", "pm-push.lock"):
        (main / ".git" / f).write_text("{}\n")
    return main


def test_init_migrates_a_legacy_clone_and_doctor_is_clean(legacy_clone: Path, tmp_path: Path):
    main, wt, old, store = legacy_clone, tmp_path / "wt", legacy_clone / ".records", legacy_clone / ".pm/store/records"
    job = old_job(tmp_path, main)

    # the store holds a record not yet committed, then one not yet pushed, then sits off its branch: refused, and
    # nothing changes
    for change, said in ((lambda: (old / "sprints/a-1.md").write_text("two\n"), "holds uncommitted records"),
                         (lambda: git(old, "commit", "-qam", "two"), "holds 1 commit(s) origin lacks")):
        change()
        before, sched = snapshot(main), loaded(tmp_path)
        res = pm(main, "init")
        assert res.returncode != 0 and said in res.stderr, res.stderr
        assert snapshot(main) == before and loaded(tmp_path) == sched and all(f.exists() for f in job)
    git(old, "push", "-q", "origin", "records")
    git(old, "checkout", "-q", "--detach", "HEAD~1")  # a store off its branch may hide commits the remote lacks
    before = snapshot(main)
    res = pm(main, "init")
    assert res.returncode != 0 and "it is not on branch records (HEAD is detached)" in res.stderr, res.stderr
    assert snapshot(main) == before and all(f.exists() for f in job)
    git(old, "checkout", "-q", "records")

    before = snapshot(main)
    res = pm(main, "init")
    assert res.returncode == 0, res.stderr
    out = res.stdout
    for piece in ("CLAUDE.md: the RULES.md import", ".claude/settings.json: 10 hook entries",
                  ".codex/hooks.json: 2 hook entries", ".beads/hooks/post-checkout: the unmarked pm code",
                  ".beads/hooks/pre-commit: the unmarked pm code", ".github/workflows/records-guard.yml: renamed to "
                  ".github/workflows/pm-records-guard.yml", ".gitignore: /.records/, site/, /records, "
                  "/.claude/settings.local.json", "stopped and removed the old push job", "moved the records store "
                  f"from {old} to {store}", f"relinked {wt / 'records'} -> {store}"):
        assert f"legacy: {piece}" in out, (piece, out)

    # tracked files: each legacy piece replaced by pm's, every other byte kept
    assert (main / "CLAUDE.md").read_text() == CLAUDE_MD_AFTER and (main / "AGENTS.md").is_symlink()
    claude = json.loads((main / ".claude/settings.json").read_text())
    assert commands(claude, "SessionStart") == ["bd prime --hook-json", *CLAUDE_PM["SessionStart"]]
    assert commands(claude, "SubagentStart") == CLAUDE_PM["SubagentStart"]
    assert commands(claude, "Stop") == CLAUDE_PM["Stop"]
    assert commands(claude, "PreToolUse") == ["./lint.sh", *CLAUDE_PM["PreToolUse"]] and "PostToolUse" not in claude["hooks"]
    assert list(claude["hooks"]) == ["SessionStart", "SubagentStart", "PreToolUse", "Stop"], "events keep their place"
    assert claude["worktree"] == {"bgIsolation": "none"}
    codex = json.loads((main / ".codex/hooks.json").read_text())
    assert commands(codex, "SessionStart") == ["bd codex-hook SessionStart", *CLAUDE_PM["SessionStart"]]
    assert commands(codex, "Stop") == CLAUDE_PM["Stop"]
    assert (main / ".beads/hooks/post-checkout").read_text() == BEADS_HOOK + section("post-checkout")
    assert (main / ".beads/hooks/pre-commit").read_text() == BEADS_HOOK + section("pre-commit") + MINE
    assert not any((main / rel).exists() for rel in OLD_WORKFLOWS)
    assert (main / ".github/workflows/pm-records-copy.yml").is_file()
    assert (main / ".gitignore").read_text() == GITIGNORE_AFTER
    after = snapshot(main)
    changed = {p for p in before.keys() | after.keys() if before.get(p) != after.get(p)}
    assert changed == {"CLAUDE.md", ".claude/settings.json", ".codex/hooks.json", ".beads/hooks/post-checkout",
                       ".beads/hooks/pre-commit", *OLD_WORKFLOWS, ".github/workflows/pm-records-guard.yml",
                       ".github/workflows/pm-records-copy.yml", ".gitignore", ".pm/config.toml", ".pm/README.md",
                       ".pm/.gitignore", ".beads/config.yaml", ".claude/settings.local.json",
                       ".records/sprints/a-1.md"} | {
                           p for p in after if p.startswith((".pm/store/records/", ".beads/embeddeddolt/"))}, sorted(changed)

    # the clone: the store moved with its history, every link and path follows it, the push job is the pm service
    assert not old.exists() and git(store, "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (store / "sprints/a-1.md").read_text() == "two\n"
    for tree in (main, wt):
        assert (tree / "records").is_symlink() and (tree / "records").resolve() == store.resolve()
    local = json.loads((wt / ".claude/settings.local.json").read_text())
    assert local == {"permissions": {"additionalDirectories": ["/elsewhere", str(store)]}, "model": "x"}
    roots = (tmp_path / "codex/config.toml").read_text()
    assert str(old.resolve()) not in roots and '"/mine"' in roots and str(store.resolve()) in roots
    assert not any(f.exists() for f in job) and label(main) not in loaded(tmp_path)
    assert not any((main / ".git" / f).exists() for f in ("pm-push.json", "pm-push.log", "pm-push.lock"))
    assert "installed the pm service: " in out

    res = pm(main, "doctor")
    assert res.returncode == 0, res.stdout
    again = pm(main, "init")
    assert again.returncode == 0 and "legacy:" not in again.stdout, again.stdout
    assert snapshot(main) == after, "a second run changes nothing"

    # doctor names a legacy piece left behind, such as an old job a second install brought back
    old_job(tmp_path, main)
    res = pm(main, "doctor")
    assert res.returncode == 1 and "legacy: the old push job is installed" in res.stdout, res.stdout
