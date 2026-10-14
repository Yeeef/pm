#!/usr/bin/env python3
"""Name the Go tests whose behaviour can differ by OS, from go-os-tests.txt, for the Go jobs that run only them.

    go_os_tests.py check
    go_os_tests.py packages [--os] [--but DIR]
    go_os_tests.py names [--os] [--package DIR | --but DIR] [--only REGEX] [--skip REGEX] [--anchored]

The tests are the top-level `func TestX(t *testing.T)` of every *_test.go in the module (none builds only on some
systems, which every command refuses). check fails, naming each, on a test in no row of go-os-tests.txt or in two, and on a
row that names no test or package. packages prints the package dirs, as go test takes them, that hold a test of the
set; names prints the set's test names joined by `|`, as go_shards.py's --run takes them (or `^(…)$` with --anchored,
as go test's -run takes them). --os keeps the os tests (else every test); --package keeps one package's, --but leaves
one out; --only and --skip keep or drop the names a regexp finds. Each fails rather than print a set its reader
would widen: unanchored names must select only themselves within their package as a substring match does, and
anchored names must not name a test of another listed package outside the set.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
# A top-level test as go test finds one: Test, then no lower-case letter, taking one *testing.T, its parameter named
# or not, on one line or several.
TEST = re.compile(r"^func (Test(?![a-z])\w*)\(\s*(?:\w+\s+)?\*testing\.T\s*,?\s*\)", re.M)
# A file go test builds only on some systems: a build constraint, or a GOOS or GOARCH suffix on its name.
CONSTRAINED = re.compile(r"^//go:build|^// \+build", re.M)
SUFFIX = re.compile(r"_(aix|android|darwin|dragonfly|freebsd|illumos|ios|js|linux|netbsd|openbsd|plan9|solaris|wasip1|"
                    r"windows|386|amd64|arm|arm64|loong64|mips\w*|ppc64\w*|riscv64|s390x|wasm)_test\.go$")
SKIP_DIRS = {"testdata", "vendor", "node_modules"}


def module_tests(root: Path) -> dict[str, list[str]]:
    """Each package dir (relative, "." for the root) with its top-level tests, from the source."""
    tests: dict[str, list[str]] = {}
    for f in sorted(root.rglob("*_test.go")):
        rel = f.relative_to(root)
        if any(p.startswith(".") or p in SKIP_DIRS for p in rel.parts[:-1]):
            continue
        text = f.read_text()
        if CONSTRAINED.search(text) or SUFFIX.search(f.name):
            sys.exit(f"go_os_tests: {rel} builds only on some systems, which this list cannot follow")
        pkg = str(rel.parent) if rel.parent != Path(".") else "."
        tests.setdefault(pkg, []).extend(TEST.findall(text))
    return {p: sorted(t) for p, t in tests.items() if t}


def rows(path: Path) -> list[tuple[str, str, str]]:
    out = []
    for n, line in enumerate(path.read_text().splitlines(), 1):
        if not line.strip() or line.startswith("#"):
            continue
        parts = line.split(None, 3)
        if len(parts) < 4 or parts[2] not in ("os", "any"):
            sys.exit(f"go_os_tests: {path.name}:{n}: want '<package> <test or *> <os|any> <evidence>'")
        out.append((parts[0], parts[1], parts[2]))
    return out


def classify(root: Path, listing: Path) -> tuple[dict[tuple[str, str], str], list[str]]:
    """Each (package, test) with its class, and the errors: a test in no row or in two, a row naming nothing."""
    tests = module_tests(root)
    got: dict[tuple[str, str], list[str]] = {(p, t): [] for p, ts in tests.items() for t in ts}
    errors = []
    for pkg, test, cls in rows(listing):
        if pkg not in tests:
            errors.append(f"{pkg} {test}: no package with tests")
        elif test == "*":
            for t in tests[pkg]:
                got[(pkg, t)].append(cls)
        elif (pkg, test) not in got:
            errors.append(f"{pkg} {test}: no such top-level test")
        else:
            got[(pkg, test)].append(cls)
    for (pkg, test), cls in sorted(got.items()):
        if len(cls) != 1:
            errors.append(f"{pkg} {test}: in {len(cls)} rows of {listing.name}, want 1")
    return {k: v[0] for k, v in got.items() if len(v) == 1}, errors


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--root", type=Path, default=ROOT, help=argparse.SUPPRESS)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check")
    for name in ("packages", "names"):
        p = sub.add_parser(name)
        p.add_argument("--os", action="store_true")
        p.add_argument("--but")
        if name == "names":
            p.add_argument("--package")
            p.add_argument("--only")
            p.add_argument("--skip")
            p.add_argument("--anchored", action="store_true")
    a = ap.parse_args()
    listing = a.root / ".github" / "scripts" / "go-os-tests.txt"
    classes, errors = classify(a.root, listing)
    if errors:
        for e in errors:
            print(f"go_os_tests: {e}", file=sys.stderr)
        return 1
    if a.cmd == "check":
        return 0
    chosen = sorted(k for k, cls in classes.items() if (cls == "os" or not a.os) and k[0] != a.but
                    and (a.cmd == "packages" or not a.package or k[0] == a.package)
                    and (a.cmd == "packages" or not a.only or re.search(a.only, k[1]))
                    and (a.cmd == "packages" or not a.skip or not re.search(a.skip, k[1])))
    if not chosen:
        print("go_os_tests: the set is empty", file=sys.stderr)
        return 1
    pkgs = sorted({p for p, _ in chosen})
    if a.cmd == "packages":
        print(" ".join("." if p == "." else "./" + p for p in pkgs))
        return 0
    names = sorted({t for _, t in chosen})
    if a.anchored:
        union = set(names)
        wider = sorted(k for k in classes if k[0] in pkgs and k[1] in union and k not in set(chosen))
    else:
        if len(pkgs) != 1:
            print("go_os_tests: unanchored names are for one package's binary: give --package", file=sys.stderr)
            return 1
        regex = re.compile("|".join(names))
        wider = sorted(k for k in classes if k[0] == pkgs[0] and regex.search(k[1]) and k not in set(chosen))
    if wider:
        for pkg, test in wider:
            print(f"go_os_tests: the names would also run {pkg} {test}, outside the set", file=sys.stderr)
        return 1
    print("^(" + "|".join(names) + ")$" if a.anchored else "|".join(names))
    return 0


if __name__ == "__main__":
    sys.exit(main())
