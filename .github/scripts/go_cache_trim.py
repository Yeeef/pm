#!/usr/bin/env python3
"""Trim a restored Go build cache to what one run used, so the cache a CI run saves does not grow with every save.

    go_cache_trim.py age GOCACHE     before the run: every file's mtime set to two hours ago
    go_cache_trim.py trim GOCACHE    after it: every file still older than an hour deleted

The go command refreshes the mtime of each cache file it uses once that mtime is over an hour old (its own trim, run
once a day, deletes what went unused for five days), and writes new files at the time they are made. So after `age`,
the run refreshes what it uses, and `trim` deletes the rest: what earlier runs compiled and this one did not. A file
`trim` deleted that a later run needs is a cache miss there, which the go command compiles again.
"""

from __future__ import annotations

import os
import sys
import time
from pathlib import Path

HOUR = 3600


def files(cache: Path):
    for root, _, names in os.walk(cache):
        for name in names:
            yield Path(root) / name


def main() -> int:
    if len(sys.argv) != 3 or sys.argv[1] not in ("age", "trim"):
        print(__doc__.split("\n\n")[1], file=sys.stderr)
        return 2
    mode, cache = sys.argv[1], Path(sys.argv[2])
    if not cache.is_dir():
        print(f"go_cache_trim: no Go cache at {cache}", file=sys.stderr)
        return 1
    now = time.time()
    kept = removed = size = 0
    for f in files(cache):
        if mode == "age":
            os.utime(f, (now - 2 * HOUR, now - 2 * HOUR))
            kept += 1
        elif f.stat().st_mtime < now - HOUR:
            size += f.stat().st_size
            f.unlink()
            removed += 1
        else:
            kept += 1
    if mode == "age":
        print(f"go_cache_trim: aged {kept} files in {cache}")
    else:
        print(f"go_cache_trim: removed {removed} files ({size / 2**20:.0f} MiB) this run did not use; kept {kept}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
