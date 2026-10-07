"""The pieces the project-management harness put in a repo and a clone before pm was a package (`bin/pm` and
`skills/project-management/harness/`), which `pm init` migrates and `pm doctor` reports.

pm's ownership test (a hook command starting `pm prime` or `pm hook `, a marked section) cannot see them, so every
legacy piece is listed here exactly, and nothing off these lists is removed:

- repo (tracked): the RULES.md import and the Codex "read RULES.md" line in CLAUDE.md and AGENTS.md; the hook
  entries that run `bin/pm` or a `harness/*_hook.py` by path in .claude/settings.json and .codex/hooks.json; the
  unmarked pm code after Beads' section in .beads/hooks/post-checkout and pre-commit (pm's marked section replaces
  it); .github/workflows/records-{guard,copy}.yml (pm writes pm-records-{guard,copy}.yml); the old store's lines in
  .gitignore (pm's block replaces them);
- clone: the store at `<main>/.records` (moved to `<main>/.pm/store/records`), each worktree's `records` link and
  .claude/settings.local.json entry naming it, its Codex writable root, the scheduled push job (launchd agent,
  systemd user timer or crontab line `local.pm-push.<name>.<hash>`) and its state files in the git dir.

The repo half only reads (`repo_overlay`), so `pm init` plans it with pm's pieces and a refusal leaves the worktree as
it was; the clone half (`migrate_clone`) writes, and refuses before it moves anything while the old store holds
uncommitted or unpushed records."""

from __future__ import annotations

import fcntl
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

from .records import RecordError

# ---------------------------------------------------------------- repo: tracked files

INSTRUCTION_FILES = ("CLAUDE.md", "AGENTS.md")
INSTRUCTION_LINES = (
    "@skills/project-management/harness/RULES.md",
    "Runtimes that do not expand `@` imports (such as Codex): read `skills/project-management/harness/RULES.md` "
    "before working; its rules are binding here.",
)

# every command the harness wrote into the runtimes' settings, as git history holds them
_HOOKS = ("session_context", "owner_request", "uncommitted_records", "reply_wait")
_BIN = ("prime --hook-json", "prime --rules --hook-json", "prime --state --hook-json", "prime --subagent --hook-json",
        "hook owner-request || exit 1", "hook stop || exit 1", "hook stop",
        *(f"prime --rules {n} --hook-json" for n in range(1, 5)))  # bin/pm ran four rules chunks at the end
HOOK_COMMANDS = frozenset(
    [f'python3 "$CLAUDE_PROJECT_DIR"/skills/project-management/harness/{h}_hook.py' for h in _HOOKS]
    + [f'python3 "$CLAUDE_PROJECT_DIR/skills/project-management/harness/{h}_hook.py"' for h in _HOOKS]
    + [f'python3 "$(git rev-parse --show-toplevel)/skills/project-management/harness/{h}_hook.py"' for h in _HOOKS]
    + [f'f="$(git rev-parse --show-toplevel)/skills/project-management/harness/{h}_hook.py"; [ -f "$f" ] || exit 0; '
       f'python3 "$f"' for h in _HOOKS]
    + [f'"$CLAUDE_PROJECT_DIR"/bin/pm {c}' for c in _BIN]
    + [f'f="$(git rev-parse --show-toplevel)/bin/pm"; [ -f "$f" ] || exit 0; "$f" {c}' for c in _BIN]
    + [f'f="$(git rev-parse --show-toplevel)/bin/pm"; [ -f "$f" ] || {{ echo "pm hook: $f is missing" >&2; exit 1; }}; '
       f'"$f" {c}' for c in _BIN])
SETTINGS = (".claude/settings.json", ".codex/hooks.json")

# the unmarked code the harness put after Beads' section, by hook file
GIT_HOOK_CODE = {
    "post-checkout": (
        "\n# A new worktree or clone (previous HEAD is the null id) links its records/ to the shared store,\n"
        "# whichever tool created it (records/design/records-store.md).\n"
        'if [ "$1" = 0000000000000000000000000000000000000000 ] && [ -x bin/pm ]; then\n'
        "  bin/pm setup >&2 || exit $?\n"
        "fi\n"),
    "pre-commit": (
        "\n# Records live on the records branch (records/design/records-store.md): a code-branch commit must not edit "
        "records/.\n"
        'if [ ! -f "$(git rev-parse --git-dir)/MERGE_HEAD" ] && [ -n "$(git diff --cached --name-only -- records/)" ]; '
        "then\n"
        '  echo >&2 "error: this commit edits records/, which only the records branch may change; write records with '
        'bin/pm"\n'
        '  echo >&2 "and unstage these edits: git restore --staged records/"\n'
        "  exit 1\n"
        "fi\n"),
}

WORKFLOWS = {".github/workflows/records-guard.yml": ".github/workflows/pm-records-guard.yml",
             ".github/workflows/records-copy.yml": ".github/workflows/pm-records-copy.yml"}

# .gitignore: the old store's line marks a legacy install; only then do the other lines go (pm's block carries
# /records and /.claude/settings.local.json, and site/ was the old static build)
GITIGNORE_MARK = "/.records/"
GITIGNORE_LINES = (
    GITIGNORE_MARK, "site/", "/records", "/.claude/settings.local.json",
    "# Records live on the records branch, checked out at .records; records is a link to it (bin/pm setup)",
    "# Claude Code's per-machine settings; pm setup adds the records store to them",
)


def drop_lines(text: str, drop) -> str:
    """`text` without the lines in `drop`, and without a blank line that a dropped line leaves doubled (or trailing);
    every other byte is kept."""
    out: list[str] = []
    gap = False
    for line in text.splitlines(keepends=True):
        if line.rstrip("\r\n") in drop:
            gap = True
            continue
        if gap and not line.strip() and (not out or not out[-1].strip()):
            gap = False
            continue
        gap = gap and not line.strip()
        out.append(line)
    if gap and out and not out[-1].strip():
        out.pop()
    return "".join(out)


def settings_without_legacy(rel: str, text: str, keep_events) -> str:
    """The settings without the legacy hook commands; a group they empty goes, and so does an event, unless pm's
    entries go there (`keep_events`), so its key keeps its place. Every other byte stays."""
    from .install import dump_json, load_hooks
    data = load_hooks(rel, text, layout=True)
    events = data.get("hooks", {})
    for event in list(events):
        kept = []
        for g in events[event]:
            hooks = g.get("hooks", [])
            mine = [h for h in hooks if isinstance(h, dict) and h.get("command") in HOOK_COMMANDS]
            if mine:
                g["hooks"] = [h for h in hooks if h not in mine]
                if not g["hooks"]:
                    continue
            kept.append(g)
        if events[event] and not kept and event not in keep_events:
            del events[event]
        else:
            events[event][:] = kept
    return dump_json(data)


def read(path: Path) -> str | None:
    return path.read_text() if path.is_file() and not path.is_symlink() else None


def repo_overlay(top: Path, pm_events: dict[str, set[str]]) -> tuple[dict[str, str | None], list[str]]:
    """Each tracked file under `top` that holds a legacy piece, with its text without it (None: the file goes), and
    one line per piece saying what goes; read-only. `pm_events` names, per settings file, the events pm writes."""
    from .install import load_hooks
    overlay: dict[str, str | None] = {}
    said: list[str] = []
    for rel in INSTRUCTION_FILES:  # AGENTS.md is often a link to CLAUDE.md: a link is left alone
        text = read(top / rel)
        if text is not None and any(l.rstrip("\r\n") in INSTRUCTION_LINES for l in text.splitlines()):
            overlay[rel] = drop_lines(text, INSTRUCTION_LINES)
            said.append(f"{rel}: the RULES.md import and the Codex line naming it (pm prime gives the rules now)")
    for rel in SETTINGS:
        text = read(top / rel)
        if text is None:
            continue
        events = load_hooks(rel, text).get("hooks", {})
        found = [h["command"] for gs in events.values() for g in gs for h in g.get("hooks", [])
                 if isinstance(h, dict) and h.get("command") in HOOK_COMMANDS]
        if found:
            overlay[rel] = settings_without_legacy(rel, text, pm_events.get(rel, set()))
            said.append(f"{rel}: {len(found)} hook entries running bin/pm or harness/*_hook.py by path")
    for name, code in GIT_HOOK_CODE.items():
        rel = f".beads/hooks/{name}"
        text = read(top / rel)
        if text is not None and code in text:
            overlay[rel] = text.replace(code, "", 1)
            said.append(f"{rel}: the unmarked pm code after Beads' section (pm's marked section replaces it)")
    for rel, new in WORKFLOWS.items():
        if (top / rel).is_file():
            overlay[rel] = None
            said.append(f"{rel}: renamed to {new}")
    text = read(top / ".gitignore")
    if text is not None and GITIGNORE_MARK in text.splitlines():
        overlay[".gitignore"] = drop_lines(text, GITIGNORE_LINES)
        gone = [l for l in GITIGNORE_LINES if l in text.splitlines() and not l.startswith("#")]
        said.append(f".gitignore: {', '.join(gone)} (pm's block replaces them)")
    return overlay, said


# ---------------------------------------------------------------- clone: the store, the links, the push job

OLD_STORE = ".records"
PUSH_STATE = ("pm-push.json", "pm-push.lock", "pm-push.log")  # in the clone's git dir


def old_store(main: Path) -> Path:
    return main / OLD_STORE


def push_label(main: Path) -> str:
    """The old push job's name: one per clone, its directory name and a hash of its path."""
    return f"local.pm-push.{main.name}.{hashlib.sha1(str(main.resolve()).encode()).hexdigest()[:8]}"


def quiet(cmd: list[str], input: str | None = None) -> subprocess.CompletedProcess | None:
    """The command's result; None when it is not installed."""
    try:
        return subprocess.run(cmd, input=input, capture_output=True, text=True)
    except FileNotFoundError:
        return None


def checked(cmd: list[str], input: str | None = None) -> None:
    res = quiet(cmd, input)
    if res is None or res.returncode != 0:
        raise RecordError(f"{' '.join(cmd)} failed: {'not installed' if res is None else (res.stderr or res.stdout).strip()}")


def systemd_dir() -> Path:
    return Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config") / "systemd/user"


def cron_lines() -> list[str] | None:
    """The user's crontab lines; None when crontab is not installed. A crontab that cannot be read is refused, so
    nothing rewrites it from a wrong copy or misses a job in it."""
    res = quiet(["crontab", "-l"])
    if res is None:
        return None
    if res.returncode == 0:
        return res.stdout.splitlines()
    if "no crontab for" in res.stderr:
        return []
    raise RecordError(f"crontab -l failed: {(res.stderr or res.stdout).strip()}; pm looks there for the old push job")


def push_jobs(main: Path) -> list[str]:
    """Where the old push job is installed, as `launchd`, `systemd` or `cron`; read-only."""
    name = push_label(main)
    if sys.platform == "darwin":
        plist = Path.home() / "Library/LaunchAgents" / f"{name}.plist"
        res = quiet(["launchctl", "print", f"gui/{os.getuid()}/{name}"])
        return ["launchd"] if plist.exists() or (res is not None and res.returncode == 0) else []
    out = []
    if any((systemd_dir() / f"{name}.{s}").exists() for s in ("service", "timer")):
        out.append("systemd")
    if any(l.endswith(f"# {name}") for l in cron_lines() or []):
        out.append("cron")
    return out


def remove_push_job(main: Path) -> list[str]:
    """Remove the old push job wherever it is installed; what went. The caller holds the job's lock (job_lock), so
    no run is in progress and one the supervisor starts meanwhile skips."""
    name, out = push_label(main), []
    for kind in push_jobs(main):
        if kind == "launchd":
            plist = Path.home() / "Library/LaunchAgents" / f"{name}.plist"
            res = quiet(["launchctl", "print", f"gui/{os.getuid()}/{name}"])
            if res is not None and res.returncode == 0:
                checked(["launchctl", "bootout", f"gui/{os.getuid()}/{name}"])
            plist.unlink(missing_ok=True)
            out.append(f"stopped and removed the old push job: launchd agent {name} ({plist})")
        elif kind == "systemd":
            timer = systemd_dir() / f"{name}.timer"
            if timer.exists():
                checked(["systemctl", "--user", "disable", "--now", timer.name])
            quiet(["systemctl", "--user", "stop", f"{name}.service"])  # a run the timer started; none while locked
            for s in ("timer", "service"):
                (systemd_dir() / f"{name}.{s}").unlink(missing_ok=True)
            checked(["systemctl", "--user", "daemon-reload"])
            out.append(f"stopped and removed the old push job: systemd user timer {name}")
        else:
            lines = cron_lines()
            if lines is None:
                raise RecordError(f"crontab is gone, so pm cannot remove the old push job's entry {name}")
            checked(["crontab", "-"], input="".join(l + "\n" for l in lines if not l.endswith(f"# {name}")))
            out.append(f"removed the old push job: crontab entry {name}")
    return out


def remove_push_state(main: Path) -> list[str]:
    gitdir = main / ".git"
    gone = [f for f in PUSH_STATE if (gitdir / f).exists()]
    for f in gone:
        (gitdir / f).unlink()
    if gone:
        return [f"removed the old push job's state in {gitdir}: {', '.join(gone)} (the pm service keeps its own in "
                f"{main / '.pm/run'})"]
    return []


def flock(path: Path, flags: int) -> int:
    """An exclusive lock on `path`, waiting for its holder; the fd to close."""
    fd = os.open(path, flags)
    fcntl.flock(fd, fcntl.LOCK_EX)
    return fd


def store_unsettled(old: Path, remote: str, branch: str) -> str:
    """Why the old store cannot move yet (uncommitted records, or commits the remote lacks); empty when it can."""
    def git(*args: str) -> subprocess.CompletedProcess:
        return subprocess.run(["git", *args], cwd=old, capture_output=True, text=True)
    dirty = git("status", "--porcelain")
    if dirty.returncode != 0:
        return f"git status failed in {old}: {dirty.stderr.strip()}"
    if dirty.stdout.strip():
        return f"it holds uncommitted records; commit them with pm commit or revert them:\n{dirty.stdout.rstrip()}"
    head = git("symbolic-ref", "--quiet", "HEAD").stdout.strip()
    if head != f"refs/heads/{branch}":
        return f"it is not on branch {branch} (HEAD is {head or 'detached'}); check out {branch} there"
    for state in ("rebase-merge", "rebase-apply", "MERGE_HEAD"):
        if Path(git("rev-parse", "--path-format=absolute", "--git-path", state).stdout.strip()).exists():
            return f"a {state.split('-')[0].lower().replace('_head', '')} is in progress there; finish or abort it"
    upstream = f"refs/remotes/{remote}/{branch}"
    if git("rev-parse", "--verify", "--quiet", upstream).returncode != 0:
        return f"{remote} has no {branch} branch here ({upstream}); push it first: git -C {old} push -u {remote} {branch}"
    ahead = git("rev-list", "--count", f"{upstream}..refs/heads/{branch}").stdout.strip()
    if ahead != "0":
        return f"it holds {ahead} commit(s) {remote} lacks; push them first: git -C {old} push {remote} {branch}"
    return ""


def worktrees(main: Path) -> list[Path]:
    res = subprocess.run(["git", "worktree", "list", "--porcelain"], cwd=main, capture_output=True, text=True)
    out = []
    for block in res.stdout.split("\n\n"):
        lines = block.splitlines()
        if lines and lines[0].startswith("worktree ") and not any(l.startswith("prunable") for l in lines):
            tree = Path(lines[0].removeprefix("worktree "))
            if tree.is_dir():
                out.append(tree)
    return out


def links_old(link: Path, old: Path) -> bool:
    """Whether `link` is a link to the old store, whether or not the store is still there."""
    if not link.is_symlink():
        return False
    target = Path(os.readlink(link))
    return target == old or (link.parent / target).resolve() == old.resolve()


def names_old(entry, old: Path) -> bool:
    """Whether a path entry is the old store, spelled as given or through a link (/tmp and /private/tmp, say)."""
    return isinstance(entry, str) and (entry == str(old) or Path(entry).resolve() == old.resolve())


def local_settings_dirs(path: Path) -> list | None:
    if not path.is_file():
        return None
    try:
        data = json.loads(path.read_text())
    except ValueError:
        return None
    dirs = data.get("permissions", {}).get("additionalDirectories") if isinstance(data, dict) else None
    return dirs if isinstance(dirs, list) else None


def relink(main: Path, store: Path) -> list[str]:
    """In every worktree, point a `records` link to the old store at the store, and replace the old store's path in
    .claude/settings.local.json by the store's; every other byte of that file's data stays."""
    from .install import dump_json
    old, out = old_store(main), []
    for tree in worktrees(main):
        if tree.resolve() == store.resolve():
            continue
        link = tree / "records"
        if links_old(link, old):
            link.unlink()
            link.symlink_to(store)
            out.append(f"relinked {link} -> {store}")
        path = tree / ".claude/settings.local.json"
        dirs = local_settings_dirs(path)
        if dirs and any(names_old(d, old) for d in dirs):
            data = json.loads(path.read_text())
            new, have = [], str(store) in dirs
            for d in dirs:
                if not names_old(d, old):
                    new.append(d)
                elif not have:
                    new.append(str(store))
                    have = True
            data["permissions"]["additionalDirectories"] = new
            path.write_text(dump_json(data))
            out.append(f"replaced {old} by {store} in {path}")
    return out


def migrate_clone(main: Path, store: Path, remote: str, branch: str, codex_remove) -> list[str]:
    """Move the clone off the old layout: the push job goes, the store moves from <main>/.records to <main>/.pm/store/
    records (refused, before anything changes, while it holds uncommitted records or commits the remote lacks), and
    every worktree's link and local Claude Code settings and the Codex roots follow it. Each step runs only when its
    legacy piece is there, so a migrated clone is left as it is. `codex_remove(roots)` takes roots out of Codex's
    config."""
    old, out = old_store(main), []
    moving = old.is_dir()
    if moving:
        if store.exists():
            raise RecordError(f"both the old store {old} and the store {store} exist; keep the one holding your "
                              f"records, remove the other (git worktree remove), and run pm init again")
        why = store_unsettled(old, remote, branch)
        if why:
            raise RecordError(f"pm init moves the records store from {old} to {store}, but {why}; then run pm init again")
    # The old job's lock first (a run in progress holds it; one starting now skips), then the store's, which every old
    # pm write and the job's push take: in that order, as a run takes them, so neither waits on the other.
    fds = []
    try:
        if push_jobs(main) or moving:
            fds.append(flock(main / ".git" / PUSH_STATE[1], os.O_RDWR | os.O_CREAT))
        out += remove_push_job(main)  # before the move: the old job commits and pushes in the store
        if moving:
            fds.append(flock(old, os.O_RDONLY))
            why = store_unsettled(old, remote, branch)  # a run or a session may have written since the first check
            if why:
                raise RecordError(f"the old push job is gone, but the store {old} changed meanwhile: {why}; then run "
                                  f"pm init again")
            store.parent.mkdir(parents=True, exist_ok=True)
            res = subprocess.run(["git", "worktree", "move", str(old), str(store)], cwd=main, capture_output=True,
                                 text=True)
            if res.returncode != 0:
                raise RecordError(f"git worktree move {old} {store} failed: {(res.stderr or res.stdout).strip()}")
            out.append(f"moved the records store from {old} to {store}")
        out += remove_push_state(main)
    finally:
        for fd in fds:
            os.close(fd)
    out += relink(main, store)
    said = codex_remove([str(old.resolve())])
    if said:
        out.append(said)
    return out


def clone_pieces(main: Path, store: Path, codex_roots: list[str]) -> list[str]:
    """Each legacy piece left in the clone, one line each; read-only. `codex_roots` is Codex's writable roots."""
    old, out = old_store(main), []
    if old.exists():
        out.append(f"the records store is still at {old}; pm init moves it to {store}")
    for kind in push_jobs(main):
        out.append(f"the old push job is installed ({kind} {push_label(main)}); pm init replaces it with the pm service")
    state = [f for f in PUSH_STATE if (main / ".git" / f).exists()]
    if state:
        out.append(f"the old push job's state is in {main / '.git'}: {', '.join(state)}")
    for tree in worktrees(main):
        if links_old(tree / "records", old):
            out.append(f"{tree / 'records'} links the old store {old}")
        if any(names_old(d, old) for d in local_settings_dirs(tree / ".claude/settings.local.json") or []):
            out.append(f"{tree / '.claude/settings.local.json'} lists the old store {old}")
    if str(old.resolve()) in codex_roots:
        out.append(f"Codex's writable_roots list the old store {old.resolve()}")
    return out

