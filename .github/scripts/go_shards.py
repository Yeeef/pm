#!/usr/bin/env python3
"""Run a Go test binary's top-level tests in N processes at once, balanced by their measured seconds.

    go_shards.py [--shards N] [--run REGEX] [--update] TEST_BINARY PACKAGE_DIR SECONDS_FILE

The binary is `go test -c`'s; its tests run with PACKAGE_DIR as the working directory, as `go test` runs them. The
tests are those `-test.list REGEX` names (every test without --run); each goes to the shard with the fewest predicted
seconds so far, longest first. SECONDS_FILE holds one `<test> <seconds>` line per test of the binary, every test and
no other, or the run fails before it starts, naming each test missing or stale: a new test is placed by its measured
time, never by a guess. --run is test names joined by `|`, as Go's -run reads them; each must match a top-level
test, or the run fails before it starts, naming each that matches none (a subtest's name selects nothing). --update
runs every test and rewrites SECONDS_FILE from their measured times.

It fails unless every shard exits 0 and every test assigned reports PASS or SKIP exactly once, so a test in no shard,
or one the run pattern missed, fails the run. A failing shard's whole output is printed.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

# A top-level test's result line; a test's own output left unterminated can precede it on its line, and subtests'
# (TestA/sub) are indented and named with a slash.
RESULT = re.compile(r"--- (PASS|FAIL|SKIP): ([^\s/]+) \(([\d.]+)s\)$", re.M)


def listed(binary: Path, cwd: Path, pattern: str) -> list[str]:
    out = subprocess.run([str(binary), "-test.list", pattern], cwd=cwd, capture_output=True, text=True, check=True)
    return [name for name in out.stdout.split() if name.startswith("Test")]


def read_seconds(path: Path) -> dict[str, float]:
    seconds = {}
    for line in path.read_text().splitlines():
        name, secs = line.split()
        seconds[name] = float(secs)
    return seconds


def assign(tests: list[str], seconds: dict[str, float], n: int) -> list[list[str]]:
    """Longest processing time first: each test, longest first, to the shard with the least predicted time so far."""
    load = [0.0] * n
    shards: list[list[str]] = [[] for _ in range(n)]
    for name in sorted(tests, key=lambda t: (-seconds[t], t)):
        i = load.index(min(load))
        load[i] += seconds[name]
        shards[i].append(name)
    return [s for s in shards if s]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--shards", type=int, default=8)
    ap.add_argument("--run", default=".", help="the tests to run, a Go regexp as -test.list takes it")
    ap.add_argument("--update", action="store_true", help="run every test and rewrite SECONDS_FILE")
    ap.add_argument("binary", type=Path)
    ap.add_argument("package_dir", type=Path)
    ap.add_argument("seconds_file", type=Path)
    a = ap.parse_args()
    binary = a.binary.resolve()

    every = listed(binary, a.package_dir, ".")
    if a.update:
        seconds = {name: 1.0 for name in every}
        tests = every
    else:
        seconds = read_seconds(a.seconds_file)
        missing, stale = sorted(set(every) - set(seconds)), sorted(set(seconds) - set(every))
        if missing or stale:
            for name in missing:
                print(f"go_shards: {name} has no measured seconds in {a.seconds_file}", file=sys.stderr)
            for name in stale:
                print(f"go_shards: {name} in {a.seconds_file} is no test of {a.binary}", file=sys.stderr)
            print("go_shards: rewrite it with make go-test-work UPDATE=1", file=sys.stderr)
            return 1
        if any(c in a.run for c in "()/"):
            print(f"go_shards: --run {a.run!r} must be test names joined by |", file=sys.stderr)
            return 1
        unmatched = [name for name in a.run.split("|") if not listed(binary, a.package_dir, name)]
        if unmatched:
            for name in unmatched:
                print(f"go_shards: --run name {name} matches no top-level test of {a.binary}", file=sys.stderr)
            return 1
        tests = listed(binary, a.package_dir, a.run)
    shards = assign(tests, seconds, a.shards)
    assert sorted(t for s in shards for t in s) == sorted(tests), "every test is in exactly one shard"

    start = time.monotonic()
    procs = [subprocess.Popen([str(binary), "-test.v", "-test.count=1", "-test.timeout=10m",
                               "-test.run", "^(" + "|".join(s) + ")$"],
                              cwd=a.package_dir, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
             for s in shards]

    def finish(p: subprocess.Popen) -> tuple[str, float]:
        out, _ = p.communicate()
        return out, time.monotonic() - start

    with ThreadPoolExecutor(len(procs)) as pool:
        finished = list(pool.map(finish, procs))
    measured: dict[str, float] = {}
    failed = False
    for i, (p, shard, (out, done)) in enumerate(zip(procs, shards, finished)):
        results = RESULT.findall(out)
        for _, name, secs in results:
            measured[name] = float(secs)
        reported = sorted(name for verdict, name, _ in results if verdict in ("PASS", "SKIP"))
        ok = p.returncode == 0 and reported == sorted(shard)
        print(f"shard {i + 1}/{len(shards)}: {len(shard)} tests, predicted {sum(seconds[t] for t in shard):.1f}s, "
              f"done at {done:.1f}s, {'ok' if ok else 'FAILED'}; longest: "
              + ", ".join(f"{t} {measured[t]:.1f}s" for t in sorted(shard, key=lambda t: -measured.get(t, 0))[:3]
                          if t in measured))
        if not ok:
            failed = True
            sys.stdout.write(out)
            if p.returncode == 0:
                print(f"shard {i + 1}: these tests reported no PASS or SKIP: "
                      f"{' '.join(sorted(set(shard) - set(reported)))}")
    print(f"{len(tests)} tests in {len(shards)} shards: {time.monotonic() - start:.1f}s")
    if failed:
        return 1
    if a.update:
        a.seconds_file.write_text("".join(f"{name} {measured[name]:.2f}\n" for name in sorted(measured)))
        print(f"wrote {a.seconds_file}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
