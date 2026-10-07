"""The pieces `pm init` puts in a repo's tracked files, one function per piece producing its expected content, so
`pm doctor`, `pm upgrade` and `pm uninstall` can reuse them; and the bootstrap of a brand-new repo's records branch.

A piece is either a whole file pm owns (`.pm/config.toml`, `.pm/README.md`, `.pm/.gitignore`, the two workflows) or
pm's part of a shared file: its hook entries in `.claude/settings.json` and `.codex/hooks.json` (a hook is pm's when
its command starts with `pm prime` or `pm hook `), its marked section in `.beads/hooks/post-checkout` and
`pre-commit` (after Beads' section), and its marked block in `.gitignore`. Each piece has `present(text)`, whether
pm's part is there, `apply(text)`, the file with pm's part as this version writes it and every other byte kept,
`part(text)`, pm's part alone (what `pm doctor` compares with `part(apply(text))`), and `remove(text)`, the file
without pm's part (None: nothing else is left, so the file goes). `pm init` applies a piece only when it is not
present, so a second run changes nothing; `pm upgrade` applies every piece; `pm uninstall` removes every piece."""

from __future__ import annotations

import json
import re
import subprocess
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

from pm import __version__, hooks


class InstallError(Exception):
    """A piece cannot be written without changing what is not pm's; nothing was written."""


@dataclass(frozen=True)
class Settings:
    """What `.pm/config.toml` holds; the pieces that name the remote or the main branch take them from here."""
    remote: str
    main_branch: str
    port: int = 8000
    site_url: str = ""


@dataclass(frozen=True)
class Piece:
    rel: str                                  # the path under the worktree root
    present: Callable[[str | None], bool]     # pm's part is in the file (None: the file is absent)
    apply: Callable[[str | None], str]        # the file with pm's part as this version writes it
    part: Callable[[str | None], object]      # pm's part alone, comparable (None: absent)
    remove: Callable[[str], str | None]       # the file without pm's part (None: delete it)
    mode: int = 0o644                         # for a file pm creates


# ---------------------------------------------------------------- whole files pm owns

def config_text(s: Settings) -> str:
    site = f"site_url = {json.dumps(s.site_url)}\n" if s.site_url else ""
    return (f"version = {json.dumps(__version__)}   # the pm version every session must run\n"
            f"remote = {json.dumps(s.remote)}\n"
            f"main_branch = {json.dumps(s.main_branch)}\n"
            f"port = {s.port}         # the site port; the PORT environment variable overrides it for one run\n"
            + site)


README = """\
# pm

This repo uses pm: Beads holds the work, Markdown records hold the context, and pm writes the records and serves
them as a site.

- Install the pinned version (`version` in `config.toml`):
  `uv tool install "git+https://github.com/Yeeef/yeeef-agents@pm-v<version>#subdirectory=pm"`, then run `pm init`
  in each clone.
- Records live on the `records` branch. Each clone checks it out once at `.pm/store/records`, and each worktree reads
  it through `records/`, a git-ignored link. `records/` on the main branch is a copy a workflow keeps.
- Agents get pm's rules and the project's state from hooks (`pm prime`, `pm hook <name>`); run `pm --help` for the
  commands.
- `store/` and `run/` here are per clone and git-ignored; `config.toml`, this file and `.gitignore` are tracked.
"""

PM_GITIGNORE = "store/\nrun/\n"


def guard_workflow(s: Settings) -> str:
    return f"""\
# pm: records live on the records branch; {s.main_branch}'s records/ is a copy the pm-records-copy workflow owns.
# A pull request that edits records/ would make that copy drift, so it fails here.
name: Records guard

on: pull_request

jobs:
  guard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - name: Fail if the pull request edits records/
        run: |
          edited=$(git diff --name-only "origin/$GITHUB_BASE_REF...HEAD" -- records/)
          if [ -n "$edited" ]; then
            echo "::error::This pull request edits records/, which only the records branch may change. Write records with pm; drop these edits:"
            echo "$edited"
            exit 1
          fi
"""


def copy_workflow(s: Settings) -> str:
    return f"""\
# pm: copy the records branch into {s.main_branch}'s records/ after every push to {s.main_branch}, so
# {s.main_branch} carries the records with their per-file history.
name: Copy records to {s.main_branch}

on:
  push:
    branches: [{s.main_branch}]
  workflow_dispatch:

concurrency: pm-records-copy

jobs:
  copy:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - name: Merge the records branch into records/
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git fetch origin records:records
          if [ -d records ]; then
            git subtree merge --prefix=records records -m "Copy records from the records branch"
          else
            git subtree add --prefix=records records -m "Copy records from the records branch"
          fi
          test "$(git rev-parse HEAD:records)" = "$(git rev-parse 'records^{{tree}}')"
          git push origin HEAD:{s.main_branch}
"""


def whole(rel: str, content: str) -> Piece:
    return Piece(rel, lambda text: text is not None, lambda text: content, lambda text: text, lambda text: None)


# ---------------------------------------------------------------- hook entries in the runtimes' settings

def is_pm_hook(hook: dict) -> bool:
    cmd = hook.get("command")
    return isinstance(cmd, str) and (cmd.startswith("pm prime") or cmd.startswith("pm hook "))


def claude_hooks() -> dict[str, dict]:
    """pm's entries in .claude/settings.json, by event: the matcher group pm adds, holding only pm's hooks."""
    def h(command: str, timeout: int) -> dict:
        return {"command": command, "type": "command", "timeout": timeout}
    def rules(timeout: int) -> list[dict]:
        return [h(f"pm prime --rules {i} --hook-json", timeout) for i in range(1, len(hooks.STARTS) + 1)]
    return {
        "SessionStart": {"hooks": [*rules(30), h("pm prime --state --hook-json", 30)], "matcher": ""},
        "SubagentStart": {"hooks": [*rules(15), h("pm prime --subagent --hook-json", 15)], "matcher": ""},
        "Stop": {"hooks": [h("pm hook owner-request || exit 1", 30), h("pm hook stop || exit 1", 10)]},
    }


def codex_hooks() -> dict[str, dict]:
    """pm's entries in .codex/hooks.json. Codex's SessionStart has no compact event (sprint 34's question)."""
    def h(command: str, status: str, timeout: int) -> dict:
        return {"command": command, "statusMessage": status, "type": "command", "timeout": timeout}
    def rules(timeout: int) -> list[dict]:
        n = len(hooks.STARTS)
        return [h(f"pm prime --rules {i} --hook-json", f"Loading pm rules ({i} of {n})", timeout)
                for i in range(1, n + 1)]
    return {
        "SessionStart": {"hooks": [*rules(30),
                                   h("pm prime --state --hook-json", "Loading pm setup, pm where and pm show", 30)],
                         "matcher": "startup|resume|clear"},
        "SubagentStart": {"hooks": [*rules(15),
                                    h("pm prime --subagent --hook-json", "Naming the Beads agent profile", 15)]},
        "Stop": {"hooks": [h("pm hook owner-request || exit 1", "Checking for owner requests asked only in chat", 30),
                           h("pm hook stop || exit 1", "Checking for uncommitted records", 10)]},
    }


def dump_json(data) -> str:
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


def load_hooks(rel: str, text: str | None, layout: bool = False) -> dict:
    """The settings file's data, refused unless it is a JSON object whose `hooks` maps events to lists of matcher
    groups; with `layout`, also unless it is laid out as pm writes JSON (two-space indent), so rewriting it keeps
    every byte that is not pm's."""
    if text is None:
        return {}
    try:
        data = json.loads(text)
    except ValueError as e:
        raise InstallError(f"{rel} is not valid JSON ({e}); fix it by hand and run pm init again")
    events = data.get("hooks", {}) if isinstance(data, dict) else None
    if not isinstance(events, dict) or not all(
            isinstance(gs, list) and all(isinstance(g, dict) and isinstance(g.get("hooks", []), list) for g in gs)
            for gs in events.values()):
        raise InstallError(f"{rel}: hooks is not a map of events to lists of matcher groups; fix it by hand")
    if layout and text != dump_json(data):
        raise InstallError(f"{rel} is not laid out as pm writes JSON (2-space indent, one key per line), so adding "
                           f"pm's hooks would change other lines; reformat it (python3 -m json.tool --indent 2) and "
                           f"run pm init again")
    return data


def hooks_present(rel: str, entries: dict[str, dict]) -> Callable[[str | None], bool]:
    def present(text: str | None) -> bool:
        events = load_hooks(rel, text).get("hooks", {})
        for event, group in entries.items():
            have = {h.get("command") for g in events.get(event, []) for h in g.get("hooks", [])}
            if any(h["command"] not in have for h in group["hooks"]):
                return False
        return True
    return present


def hooks_apply(rel: str, entries: dict[str, dict]) -> Callable[[str | None], str]:
    """The settings with pm's hooks as this version writes them: pm's old hooks removed (a group left empty goes
    too), then pm's group appended to each event. Everything else keeps its place."""
    def apply(text: str | None) -> str:
        data = load_hooks(rel, text, layout=True)
        events = data.setdefault("hooks", {})
        for gs in events.values():
            kept = []
            for g in gs:
                mine = [h for h in g.get("hooks", []) if isinstance(h, dict) and is_pm_hook(h)]
                if mine:
                    g["hooks"] = [h for h in g["hooks"] if h not in mine]
                    if not g["hooks"]:
                        continue
                kept.append(g)
            gs[:] = kept
        for event, group in entries.items():
            events.setdefault(event, []).append(json.loads(json.dumps(group)))
        return dump_json(data)
    return apply


def hooks_part(rel: str) -> Callable[[str | None], object]:
    """pm's hooks by event, in order, each as written; None when the file is absent."""
    def part(text: str | None) -> object:
        if text is None:
            return None
        events = load_hooks(rel, text).get("hooks", {})
        mine = {e: [h for g in gs for h in g.get("hooks", []) if isinstance(h, dict) and is_pm_hook(h)]
                for e, gs in events.items()}
        return {e: hs for e, hs in mine.items() if hs}
    return part


def hooks_remove(rel: str) -> Callable[[str], str | None]:
    """The settings without pm's hooks: a group or event pm's removal empties goes too, and so does `hooks` and then
    the file when nothing else is left."""
    def remove(text: str) -> str | None:
        data = load_hooks(rel, text, layout=True)
        events = data.get("hooks")
        if not events:
            return text
        for event in list(events):
            gs = events[event]
            kept = []
            for g in gs:
                mine = [h for h in g.get("hooks", []) if isinstance(h, dict) and is_pm_hook(h)]
                if mine:
                    g["hooks"] = [h for h in g["hooks"] if h not in mine]
                    if not g["hooks"]:
                        continue
                kept.append(g)
            if gs and not kept:
                del events[event]
            else:
                gs[:] = kept
        if not events:
            del data["hooks"]
        return dump_json(data) if data else None
    return remove


def hooks_piece(rel: str, entries: dict[str, dict]) -> Piece:
    return Piece(rel, hooks_present(rel, entries), hooks_apply(rel, entries), hooks_part(rel), hooks_remove(rel))


# ---------------------------------------------------------------- marked sections

GIT_HOOKS = ("post-checkout", "pre-commit")
SECTION_END = "# --- END PM ---"
SECTION = re.compile(r"^# --- BEGIN PM v[^\n]* ---\n.*?^# --- END PM ---\n?", re.M | re.S)
BEGIN_ANY = re.compile(r"^# --- BEGIN PM v[^\n]* ---$", re.M)
BEADS_END = re.compile(r"^# --- END BEADS INTEGRATION[^\n]*(\n|\Z)", re.M)
SHEBANG = "#!/usr/bin/env sh\n"


def git_hook_section(name: str) -> str:
    """pm's section in a Beads hook file: one line, so the logic ships in the package. `|| exit $?` keeps a failure
    (pm missing, or the pre-commit guard refusing) from being lost when lines follow it."""
    return f'# --- BEGIN PM v{__version__} ---\npm hook git-{name} "$@" || exit $?\n{SECTION_END}\n'


def strip_section(rel: str, text: str, pattern: re.Pattern, begin: re.Pattern) -> str:
    out = pattern.sub("", text)
    if begin.search(out):
        raise InstallError(f"{rel} has a pm begin marker without its end marker ({SECTION_END}); fix it by hand")
    return out


def git_hook_apply(rel: str, name: str) -> Callable[[str | None], str]:
    """pm's section right after Beads' end marker (or at the end when Beads has none); a new file gets a shebang."""
    def apply(text: str | None) -> str:
        if text is None:
            return SHEBANG + git_hook_section(name)
        text = strip_section(rel, text, SECTION, BEGIN_ANY)
        m = BEADS_END.search(text)
        at = m.end() if m else len(text)
        head = text[:at] if not text[:at] or text[:at].endswith("\n") else text[:at] + "\n"
        return head + git_hook_section(name) + text[at:]
    return apply


GITIGNORE_BEGIN, GITIGNORE_END = "# --- BEGIN PM ---", "# --- END PM ---"
GITIGNORE_BLOCK = (f"{GITIGNORE_BEGIN}\n"
                   "# each worktree's records/ is a link to the clone's records store\n"
                   "/records\n"
                   "# per-machine Claude Code settings: pm adds the store's absolute path to them\n"
                   "/.claude/settings.local.json\n"
                   f"{GITIGNORE_END}\n")
GITIGNORE_SECTION = re.compile(rf"^{re.escape(GITIGNORE_BEGIN)}\n.*?^{re.escape(GITIGNORE_END)}\n?", re.M | re.S)
GITIGNORE_BEGIN_RE = re.compile(rf"^{re.escape(GITIGNORE_BEGIN)}$", re.M)


def gitignore_apply(text: str | None) -> str:
    text = strip_section(".gitignore", text or "", GITIGNORE_SECTION, GITIGNORE_BEGIN_RE)
    sep = "" if not text or text.endswith("\n") else "\n"
    return text + sep + GITIGNORE_BLOCK


def marked(begin: re.Pattern) -> Callable[[str | None], bool]:
    return lambda text: text is not None and begin.search(text) is not None


def section_part(pattern: re.Pattern) -> Callable[[str | None], object]:
    """pm's marked sections in the file, as written; None when the file is absent."""
    return lambda text: None if text is None else [m.group(0) for m in pattern.finditer(text)]


def section_remove(rel: str, pattern: re.Pattern, begin: re.Pattern, bare: tuple[str, ...]) -> Callable[[str], str | None]:
    """The file without pm's section; None when only what pm writes into a new file (`bare`) is left."""
    def remove(text: str) -> str | None:
        out = strip_section(rel, text, pattern, begin)
        return None if out in bare else out
    return remove


# ---------------------------------------------------------------- the pieces

def pieces(s: Settings) -> list[Piece]:
    return [
        whole(".pm/config.toml", config_text(s)),
        whole(".pm/README.md", README),
        whole(".pm/.gitignore", PM_GITIGNORE),
        hooks_piece(".claude/settings.json", claude_hooks()),
        hooks_piece(".codex/hooks.json", codex_hooks()),
        *(Piece(f".beads/hooks/{name}", marked(BEGIN_ANY), git_hook_apply(f".beads/hooks/{name}", name),
                section_part(SECTION), section_remove(f".beads/hooks/{name}", SECTION, BEGIN_ANY, ("", SHEBANG)),
                0o755)
          for name in GIT_HOOKS),
        whole(".github/workflows/pm-records-guard.yml", guard_workflow(s)),
        whole(".github/workflows/pm-records-copy.yml", copy_workflow(s)),
        Piece(".gitignore", marked(GITIGNORE_BEGIN_RE), gitignore_apply, section_part(GITIGNORE_SECTION),
              section_remove(".gitignore", GITIGNORE_SECTION, GITIGNORE_BEGIN_RE, ("",))),
    ]


def plan(top: Path, s: Settings) -> list[tuple[Piece, Path, str | None, str]]:
    """Each piece not present under `top`, with its path, current text and new text. Planning reads only, so a
    refusal leaves the worktree as it was."""
    out = []
    for piece in pieces(s):
        path = top / piece.rel
        text = path.read_text() if path.exists() else None
        if not piece.present(text):
            out.append((piece, path, text, piece.apply(text)))
    return out


def read(path: Path) -> str | None:
    return path.read_text() if path.exists() else None


def drift(top: Path, s: Settings) -> list[str]:
    """Each piece whose pm part under `top` is not what this version writes, as one line saying how."""
    out = []
    for piece in pieces(s):
        text = read(top / piece.rel)
        try:
            want = piece.part(piece.apply(text))
            have = piece.part(text)
        except InstallError as e:
            out.append(f"{piece.rel}: {e}")
            continue
        if not have:
            out.append(f"{piece.rel}: pm's part is missing")
        elif have != want:
            out.append(f"{piece.rel}: pm's part differs from what pm {__version__} writes")
    return out


def rewrite(top: Path, s: Settings) -> list[tuple[Piece, Path, str | None, str]]:
    """Every piece whose file under `top` differs from the file with pm's part as this version writes it, with its
    path, current text and new text; read-only, so a refusal leaves the worktree as it was."""
    out = []
    for piece in pieces(s):
        path = top / piece.rel
        text = read(path)
        new = piece.apply(text)
        if new != text:
            out.append((piece, path, text, new))
    return out


def removals(top: Path, s: Settings) -> list[tuple[Path, str | None]]:
    """Each file under `top` holding a pm part, with the file without it (None: delete the file); read-only."""
    out = []
    for piece in pieces(s):
        path = top / piece.rel
        text = read(path)
        if text is not None and piece.part(text):
            out.append((path, piece.remove(text)))
    return out


def write(planned: list[tuple[Piece, Path, str | None, str]]) -> list[str]:
    """Write the planned pieces; the paths written."""
    for piece, path, text, content in planned:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        if text is None:
            path.chmod(piece.mode)
    return [piece.rel for piece, *_ in planned]


# ---------------------------------------------------------------- a brand-new repo's records branch

LAYOUT = ("projects", "sprints", "days", "design", "docs", "postmortems")
KEEP = ".gitkeep"  # git tracks no empty directory


def git(cwd: Path, *args: str, input: str | None = None) -> str:
    res = subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, input=input)
    if res.returncode != 0:
        raise InstallError(f"git {' '.join(args)} failed in {cwd}: {(res.stderr or res.stdout).strip()}")
    return res.stdout.strip()


def remote_has_branch(cwd: Path, remote: str, branch: str) -> bool:
    res = subprocess.run(["git", "ls-remote", "--exit-code", "--heads", remote, f"refs/heads/{branch}"], cwd=cwd,
                         capture_output=True, text=True)
    if res.returncode not in (0, 2):  # 2: the remote answered and has no such branch
        raise InstallError(f"git ls-remote {remote} failed: {(res.stderr or res.stdout).strip()}")
    return res.returncode == 0


def create_records_branch(cwd: Path, remote: str, branch: str) -> str:
    """Make the store's empty layout an orphan commit, publish it as <remote>/<branch>, then create the local branch
    tracking it. The push comes first, so a rejected push (another clone created it meanwhile) leaves nothing local;
    pm push fetches <remote>/<branch> and so cannot publish a branch the remote lacks."""
    blob = git(cwd, "hash-object", "-w", "--stdin", input="")
    sub = git(cwd, "mktree", input=f"100644 blob {blob}\t{KEEP}\n")
    root = git(cwd, "mktree", input="".join(f"040000 tree {sub}\t{d}\n" for d in LAYOUT))
    commit = git(cwd, "commit-tree", root, "-m", "pm: empty records store")
    git(cwd, "push", "--quiet", remote, f"{commit}:refs/heads/{branch}")
    git(cwd, "fetch", "--quiet", remote, f"refs/heads/{branch}:refs/remotes/{remote}/{branch}")
    git(cwd, "branch", "--quiet", "--track", branch, f"{remote}/{branch}")
    return f"created the {branch} branch with an empty store ({', '.join(d + '/' for d in LAYOUT)}) and pushed it to {remote}"
