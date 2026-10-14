#!/usr/bin/env python3
"""Whether a pull request's changes can affect pm's Go jobs (pm-go.yml, pm-lint.yml's Go linters), so a PR that changes
only docs or the harness's Python skips their work while each of their checks still reports; and which Go test
packages they can affect, so a PR runs only those.

    go_changes.py           writes go=true or go=false to $GITHUB_OUTPUT and prints why
    go_changes.py select F  writes to F the test packages the changes can affect, one per line as go test takes them
                            (., ./internal/site), and prints why each changed file selects what it does

On any event but a pull request both run everything: go is true, and select writes every package that has tests. On a
pull request the checkout is the PR's merge commit fetched two deep, and its changes are the diff against its first
parent, the base. go is false only when there are changes and each one matches NO_GO and no Go file reads it: a Go
file reaches a repo file by a path from its package's directory, where go test runs it (`../../tests/fake_sched.py`,
filepath.Join("..", "..", "go.mod"), or the root package's plain "README.md"). README.md, though listed, never skips
them today: assets_test.go, in the root package, names "README.md" (changelog.d's), which reads as the root's.

select reads the module's packages from `go list -deps -test` and takes, for each changed file:
- go.mod or go.sum: every package (they change every package's build);
- a .go file in a package's directory: that package, so every test package whose test binary links it (its own, and
  each one importing it, test-only imports included; a _test.go file, its own package's tests alone), plus each
  package of SOURCE_READERS whose tests read it;
- a file a package embeds: that package, as for a .go file (a test-only embed: that package's tests alone);
- a file under a package's testdata/, or one a package's Go files name by path, a directory of it included: that
  package's tests (and, named by a non-test file, every test package linking it);
- a file of NO_GO that no Go file names: nothing;
- anything else (the Makefile, a workflow, go-os-tests.txt, a .go file in no package): every package, since the
  selector cannot place it.
It fails hard on a package whose tests read Go source (parse it, run the go command on it, or walk for .go files) and
is not in SOURCE_READERS, or one listed that no longer does. A checkout that is not a merge commit with both parents
(one fetched too shallow, or a pull_request event checked out at its head), and any git or go failure, fail the step:
nothing is guessed.
"""

from __future__ import annotations

import fnmatch
import functools
import json
import os
import posixpath
import re
import subprocess
import sys
from pathlib import Path, PurePosixPath

# The paths no Go build, vet, lint or test reads: the docs no file embeds (prime.md and CHANGELOG.md are embedded),
# the site's static page, and the harness's and the release tools' Python. README.md stays read by Go while a root
# package file names "README.md" (assets_test.go does, for changelog.d's).
NO_GO = ("README.md", "AGENTS.md", "CLAUDE.md", "website/*", "tests/*.py", "release/*.py")

# The packages whose tests read Go source files beyond their own build, each with the directory whose .go files they
# read and the evidence: a .go file changed under it selects the package, whatever the import graph says.
SOURCE_READERS = {
    ".": (".", "size_test.go counts every package's lines; access_test.go parses every non-test file's calls; "
               "assets_test.go runs go build"),
    "internal/item": (".", "item_test.go runs go list -deps on the item readers, whose imports can reach any package"),
    "internal/cli": ("internal/cli", "doc_test.go parses the func docs of internal/cli and its subpackages"),
}
# What marks a test file as reading Go source: the parser's packages, the go command, or a walk or glob keeping .go
# files.
SOURCE_MARK = re.compile(r'"go/(?:ast|parser|build|doc|types)"|"golang\.org/x/tools/go/packages"|'
                         r'exec\.Command(?:Context)?\((?:\w+, )?"go"|"\.go"|\*\.go"')
ALL = None  # select's answer when every package runs


def git(*args: str) -> str:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=True).stdout


@functools.cache
def literals(text: str) -> frozenset[str]:
    """The string literals in Go text."""
    return frozenset(re.findall(r'"((?:[^"\\\n]|\\.)*)"', text))


def named(path: str, pkg_dir: str, text: str) -> bool:
    """Whether Go text in package dir pkg_dir names repo path, as a path from pkg_dir or joined from its parts."""
    rel = posixpath.relpath(path, pkg_dir)
    parts = PurePosixPath(rel).parts
    if rel in literals(text):
        return True
    joined = '"' + '", "'.join(parts) + '"'
    return parts[-1] in literals(text) and (joined + ")" in text or joined + ", " in text)


def names_path(path: str, pkg_dir: str, text: str) -> bool:
    """Whether Go text in pkg_dir names path or a directory holding it (not the repo root, nor a bare `..`)."""
    for p in [path, *map(str, PurePosixPath(path).parents)]:
        if p == ".":
            break
        if any(part != ".." for part in PurePosixPath(posixpath.relpath(p, pkg_dir)).parts) and named(p, pkg_dir, text):
            return True
    return False


def read_by_go(path: str, go_files: dict[str, str]) -> bool:
    """Whether a Go file names path as one it reads (go_files: each tracked .go file's repo path and text)."""
    return any(names_path(path, posixpath.dirname(name) or ".", text) for name, text in go_files.items())


def go_affected(changed: list[str], go_files: dict[str, str]) -> tuple[bool, str]:
    """Whether changed (repo paths) can affect the Go jobs, and why."""
    if not changed:
        return True, "no changed file found: every job runs"
    for path in changed:
        if not any(fnmatch.fnmatchcase(path, p) for p in NO_GO):
            return True, f"{path} can affect the Go jobs"
        if read_by_go(path, go_files):
            return True, f"{path} is read by a Go file"
    return False, f"none of the {len(changed)} changed files can affect the Go jobs: {' '.join(changed)}"


class Package:
    """A module package as select reads it: its directory relative to the module root ("." for the root), each
    non-test .go file's name and text, each _test.go file's, its embed patterns and its test files', and, for a package
    with tests, the package dirs its test binary links, itself included (None for one without)."""

    def __init__(self, dir: str, files: dict[str, str] | None = None, test_files: dict[str, str] | None = None,
                 embeds: list[str] | None = None, test_embeds: list[str] | None = None,
                 links: set[str] | None = None):
        self.dir, self.files, self.test_files = dir, files or {}, test_files or {}
        self.embeds, self.test_embeds, self.links = embeds or [], test_embeds or [], links


def embedded(path: str, pkg: Package, patterns: list[str]) -> bool:
    if not path.startswith("" if pkg.dir == "." else pkg.dir + "/"):
        return False
    rel = posixpath.relpath(path, pkg.dir)
    for p in patterns:
        p = p.removeprefix("all:")
        if fnmatch.fnmatchcase(rel, p) or rel.startswith(p.rstrip("/") + "/"):
            return True
    return False


def select(changed: list[str], pkgs: dict[str, Package]) -> tuple[set[str] | None, list[str]]:
    """The test package dirs changed (repo paths) can affect, or ALL, and a line per changed file saying why."""
    tests = {d for d, p in pkgs.items() if p.links is not None}
    built: set[str] = set()  # packages whose build changed: every test binary linking one runs
    direct: set[str] = set()  # test packages whose tests read a changed file
    why = []
    if not changed:
        return ALL, ["no changed file found: every package runs"]
    for path in changed:
        if path in ("go.mod", "go.sum"):
            return ALL, why + [f"{path}: changes every package's build, so every package runs"]
        home = posixpath.dirname(path) or "."
        if path.endswith(".go") and home in pkgs:
            readers = {d for d, (scope, _) in SOURCE_READERS.items() if scope == "." or (path + "/").startswith(scope + "/")}
            if path.endswith("_test.go"):  # compiled into its own package's test binary alone
                readers.add(home)
            else:
                built.add(home)
            direct.update(readers & tests)
            why.append(f"{path}: package {home}{'' if path.endswith('_test.go') else ' and every test binary linking it'}"
                       f"; tests that read it: {' '.join(sorted(readers & tests)) or 'none'}")
            continue
        hit_built = {d for d, p in pkgs.items() if embedded(path, p, p.embeds)
                     or any(names_path(path, d, t) for t in p.files.values())}
        hit_tests = {d for d, p in pkgs.items() if embedded(path, p, p.test_embeds)
                     or path.startswith(("" if d == "." else d + "/") + "testdata/")
                     or any(names_path(path, d, t) for t in p.test_files.values())}
        if hit_built or hit_tests:
            built |= hit_built
            direct |= hit_tests & tests
            why.append(f"{path}: read by {' '.join(sorted(hit_built | hit_tests))}")
        elif not path.endswith(".go") and any(fnmatch.fnmatchcase(path, p) for p in NO_GO):
            why.append(f"{path}: read by no Go file")
        else:
            return ALL, why + [f"{path}: no package is known to read it, so every package runs"]
    chosen = direct | {t for t in tests if pkgs[t].links & built}
    return chosen, why


def source_reader_errors(pkgs: dict[str, Package]) -> list[str]:
    """A package whose tests read Go source and is not in SOURCE_READERS, or one listed whose tests do not."""
    marked = {d for d, p in pkgs.items() if any(SOURCE_MARK.search(t) for t in p.test_files.values())}
    return ([f"{d}: its tests read Go source ({SOURCE_MARK.pattern}); add it to SOURCE_READERS with the directory "
             "they read" for d in sorted(marked - set(SOURCE_READERS))]
            + [f"{d}: in SOURCE_READERS, but no test file of it reads Go source" for d in sorted(set(SOURCE_READERS) - marked)])


def go_list(root: Path) -> dict[str, Package]:
    """The module's packages, from go list -deps -test: each one's files, embeds and test binary's links."""
    fields = "ImportPath,Dir,Module,GoFiles,CgoFiles,IgnoredGoFiles,TestGoFiles,XTestGoFiles,EmbedPatterns," \
             "TestEmbedPatterns,XTestEmbedPatterns,Deps"
    out = subprocess.run(["go", "list", "-tags", "gms_pure_go", "-deps", "-test", f"-json={fields}", "./..."],
                         cwd=root, capture_output=True, text=True, check=True, env={**os.environ, "CGO_ENABLED": "1"}).stdout
    entries, dec, i = [], json.JSONDecoder(), 0
    while (i := len(out) - len(out[i:].lstrip())) < len(out):
        entry, i = dec.raw_decode(out, i)
        entries.append(entry)
    module = json.loads(subprocess.run(["go", "list", "-m", "-json"], cwd=root, capture_output=True, text=True,
                                       check=True).stdout)["Path"]
    ours = [e for e in entries if (e.get("Module") or {}).get("Path") == module]
    dirs = {e["ImportPath"]: Path(e["Dir"]).resolve().relative_to(root.resolve()).as_posix()
            for e in ours if " [" not in e["ImportPath"] and not e["ImportPath"].endswith(".test")}
    pkgs: dict[str, Package] = {}
    for e in ours:
        if e["ImportPath"] not in dirs:
            continue
        d = dirs[e["ImportPath"]]
        texts = {k: {n: (root / d / n).read_text(encoding="utf-8") for k2 in k for n in e.get(k2, [])}
                 for k in (("GoFiles", "CgoFiles", "IgnoredGoFiles"), ("TestGoFiles", "XTestGoFiles"))}
        pkgs[d] = Package(d, *texts.values(), e.get("EmbedPatterns", []),
                          e.get("TestEmbedPatterns", []) + e.get("XTestEmbedPatterns", []))
    for e in ours:
        if e["ImportPath"].endswith(".test"):
            d = dirs[e["ImportPath"].removesuffix(".test")]
            pkgs[d].links = {d} | {dirs[p] for p in (x.split(" [")[0] for x in e.get("Deps", [])) if p in dirs}
    return pkgs


def changes() -> list[str] | None:
    """A pull request's changed files, or None on another event; exits on a checkout that is not its merge commit."""
    if os.environ.get("GITHUB_EVENT_NAME") != "pull_request":
        return None
    if subprocess.run(["git", "rev-parse", "--verify", "-q", "HEAD^2"], capture_output=True, check=False).returncode:
        sys.exit("go_changes.py: HEAD is not a pull request's merge commit with its two parents fetched (checkout "
                 "with fetch-depth: 2 at the merge ref); refusing to guess the PR's changes")
    return git("diff", "--name-only", "--no-renames", "HEAD^1", "HEAD").split()


def main() -> int:
    if len(sys.argv) == 3 and sys.argv[1] == "select":
        root = Path(git("rev-parse", "--show-toplevel").strip())
        pkgs = go_list(root)
        if errors := source_reader_errors(pkgs):
            sys.exit("\n".join(f"go_changes.py: {e}" for e in errors))
        changed = changes()
        if changed is None:
            chosen, why = ALL, [f"a {os.environ.get('GITHUB_EVENT_NAME')} event runs every package"]
        else:
            chosen, why = select(changed, pkgs)
        every = {d for d, p in pkgs.items() if p.links is not None}
        chosen = every if chosen is ALL else chosen
        print("\n".join(why))
        print(f"selected {len(chosen)} of {len(every)} test packages: {' '.join(sorted(chosen)) or 'none'}")
        Path(sys.argv[2]).parent.mkdir(parents=True, exist_ok=True)
        Path(sys.argv[2]).write_text("".join(("." if d == "." else "./" + d) + "\n" for d in sorted(chosen)))
        return 0
    if len(sys.argv) != 1:
        sys.exit(__doc__.split("\n\n")[1])
    changed = changes()
    if changed is None:
        go, why = True, f"a {os.environ.get('GITHUB_EVENT_NAME')} event runs every job"
    else:
        go_files = {f: Path(f).read_text(encoding="utf-8") for f in git("ls-files", "*.go").split()}
        go, why = go_affected(changed, go_files)
    print(f"go={'true' if go else 'false'}: {why}")
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as out:
        out.write(f"go={'true' if go else 'false'}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
