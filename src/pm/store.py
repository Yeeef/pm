"""The records store: the `records` branch, checked out once per clone at `<main checkout>/.records`.

Every worktree finds it through the clone's common git dir, the way `bd` finds its database,
so a write is visible from every branch and worktree at once.
"""

from __future__ import annotations

import io
import subprocess
import tarfile
from datetime import date
from pathlib import Path, PurePosixPath

from .records import Record, RecordError, parse_record

BRANCH = "records"
SETUP = "bin/pm setup"


def git(cwd: Path, *args: str) -> str:
    res = subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True)
    if res.returncode != 0:
        raise RecordError(f"git {' '.join(args)} failed in {cwd}: {(res.stderr or res.stdout).strip()}")
    return res.stdout.strip()


def store_path(cwd: Path) -> Path:
    """Where the store of the clone containing `cwd` lives, whether or not it exists yet."""
    common = Path(git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir"))
    if common.name != ".git":
        raise RecordError(f"the clone's git dir {common} is not a .git directory inside a main checkout")
    return common.parent / ".records"


def find_store(cwd: Path) -> Path:
    """The store, checked: a worktree of this clone on the records branch. Never a fallback."""
    store = store_path(cwd)
    if not store.is_dir():
        raise RecordError(f"no records store at {store}; set it up with {SETUP}")
    top = Path(git(store, "rev-parse", "--show-toplevel")).resolve()
    branch = git(store, "rev-parse", "--abbrev-ref", "HEAD")
    if top != store.resolve() or branch != BRANCH:
        raise RecordError(f"{store} is not a worktree on branch {BRANCH}; move it away and run {SETUP}")
    return store


def code_root(cwd: Path, store: Path) -> Path:
    """The worktree a command acts on for code commits and the site: cwd's, or the main checkout from inside the store."""
    top = Path(git(cwd, "rev-parse", "--show-toplevel"))
    return store.parent if top.resolve() == store.resolve() else top


def commit(store: Path, message: str, paths: list[Path]) -> str:
    """Commit exactly `paths` on the records branch and return the short hash; any other change in the store, such
    as another session's, stays out. Records are validated by pm before they get here, and code-branch hooks do
    not apply to the store, so hooks are skipped."""
    if not paths:
        raise RecordError("no records named to commit")
    names = [p.resolve().relative_to(store.resolve()).as_posix() for p in paths]
    git(store, "add", "-A", "--", *names)
    git(store, "commit", "--quiet", "--no-verify", "-m", message, "--", *names)
    return git(store, "rev-parse", "--short", "HEAD")


def committed_records(store: Path, changes: dict[Path, str | None] | None = None) -> list[Record]:
    """The record set the records branch holds once `changes` (a path's new text, or None to remove it) are
    committed on top of HEAD. Writes and pm commit check this, not the working store, so another session's
    uncommitted file neither blocks them nor lets them depend on it."""
    root = store.resolve()
    res = subprocess.run(["git", "archive", "--format=tar", "HEAD"], cwd=store, capture_output=True)
    if res.returncode != 0:
        raise RecordError(f"git archive HEAD failed in {store}: {res.stderr.decode().strip()}")
    with tarfile.open(fileobj=io.BytesIO(res.stdout)) as tar:
        texts = {root / m.name: tar.extractfile(m).read().decode()
                 for m in tar if m.isfile() and m.name.endswith(".md")}
    for path, text in (changes or {}).items():
        if path.suffix != ".md":
            continue
        if text is None:
            texts.pop(path.resolve(), None)
        else:
            texts[path.resolve()] = text
    return [parse_record(p, p.relative_to(root).with_suffix("").as_posix(), texts[p]) for p in sorted(texts)]


def design_dates(store: Path, recs: list[Record]) -> dict[str, tuple[str, str]]:
    """(created, last updated) of each design record in `recs`, by its rel: the dates of its first and last commits
    on the records branch, following renames within its directory as `git log --follow` does. A page with
    uncommitted changes counts as updated today, and one never committed as created today too. One git log over the
    design pages' directories, parsed once, plus one git status."""
    rels = [r.rel for r in recs if r.type == "design"]
    if not rels:
        return {}
    dirs = sorted({str(PurePosixPath(rel).parent) for rel in rels})
    log = git(store, "log", "-M", "--name-status", "--format=%x00%cs", "--", *dirs)
    created, updated, renamed = {}, {}, {}
    for chunk in log.split("\0")[1:]:  # newest commit first
        day, *lines = chunk.strip("\n").split("\n")
        for line in filter(None, lines):
            status, *paths = line.split("\t")
            name = renamed.get(paths[-1], paths[-1])
            updated.setdefault(name, day)
            created[name] = day
            if status.startswith("R"):
                renamed[paths[0]] = name
    today = date.today().isoformat()
    dirty = set(uncommitted(store, [store / f"{rel}.md" for rel in rels]))
    out = {}
    for rel in rels:
        path = f"{rel}.md"
        if path in dirty:
            out[rel] = (created.get(path, today), today)
        elif path in updated:
            out[rel] = (created[path], updated[path])
        else:
            raise RecordError(f"{rel}: committed and unchanged, yet git log over {', '.join(dirs)} never names it")
    return out


def head_files(store: Path) -> list[Path]:
    """The files whose bytes change whenever the store's HEAD commit does: the worktree's HEAD, the branch's loose
    ref and packed-refs. Reading them is how pm serve notices a commit without running git."""
    gitdir, common = (Path(p) for p in git(store, "rev-parse", "--path-format=absolute", "--git-dir",
                                            "--git-common-dir").split("\n"))
    if not (common / "refs/heads").is_dir():
        raise RecordError(f"{common} keeps refs in a format other than files (reftable?); pm serve reads refs as files")
    return [gitdir / "HEAD", common / "refs/heads" / BRANCH, common / "packed-refs"]


def read_files(paths: list[Path]) -> tuple[bytes | None, ...]:
    """The bytes of each of `paths`, None for one that does not exist."""
    return tuple(p.read_bytes() if p.exists() else None for p in paths)


def uncommitted(store: Path, paths: list[Path]) -> list[str]:
    """Those of `paths` that differ from HEAD in the store (modified, staged, removed or untracked), as store paths."""
    if not paths:
        return []  # with no pathspec, git status would list every change in the store
    names = [p.resolve().relative_to(store.resolve()).as_posix() for p in paths]
    out = subprocess.run(["git", "status", "--porcelain", "--untracked-files=all", "-z", "--", *names], cwd=store,
                         capture_output=True, text=True, check=True).stdout
    return [entry[3:] for entry in out.split("\0") if entry[2:3] == " "]
