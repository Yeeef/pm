"""The differential transcript: every `Repo.pm` call of one test, normalised so that two implementations' runs of
the test compare equal when they behave the same.

A call holds its argv (without the pm binary), stdin, stdout, stderr, exit code, the record files it changed (path to
new text, null when removed) and the store export after it. The normaliser replaces what differs between two runs:
the temp and checkout paths and random temp names, git commit ids and UUIDs, timestamps, durations, ports, and root
ids pm minted (in order of appearance; the seeded ids are kept)."""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path

CALLS: list[dict] = []
ROOTS: list[str] = []

TEMP_NAME = re.compile(r"(<systmp>/[\w.-]*?)[a-z0-9_]{8}(?![\w.-])")  # tempfile.mkdtemp's 8 random characters
UUID = re.compile(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b")
# a commit id, or a digest cut like one; with a digit, so a word like "defaced" stays (1 in 27 short ids is digits only)
SHA = re.compile(r"\b(?=[0-9a-f]*\d)[0-9a-f]{7,40}\b")
TIME = re.compile(r"\b\d{4}-\d\d-\d\d[T ]\d\d:\d\d(?::\d\d(?:\.\d+)?)?(?:Z|[+-]\d\d:?\d\d)?")
DURATION = re.compile(r"\b\d+(?:\.\d+)?(?: ?(?:ms|s|min)|[mhd])\b")
PORT = re.compile(r"(?:(?<=localhost:)|(?<=127\.0\.0\.1:)|(?<= :))\d{2,5}\b")


def start() -> None:
    CALLS.clear()
    ROOTS.clear()


def record(call: dict, roots: list[str]) -> None:
    CALLS.append(call)
    ROOTS.extend(r for r in roots if r not in ROOTS)


def normalise(value, paths: dict[str, str]):
    """`value` with, in each string, each path in `paths` (longest first) and each minted root id replaced by its
    placeholder, then random temp names, UUIDs, commit ids, timestamps, durations and ports."""
    if isinstance(value, dict):
        return {normalise(k, paths): normalise(v, paths) for k, v in value.items()}
    if isinstance(value, list):
        return [normalise(v, paths) for v in value]
    if not isinstance(value, str):
        return value
    for path in sorted(paths, key=len, reverse=True):
        value = value.replace(path, paths[path])
    for n, root in enumerate(ROOTS, 1):
        value = re.sub(rf"(?<![\w-]){re.escape(root)}(?![\w-])", f"<root-{n}>", value)
    value = TEMP_NAME.sub(r"\1<random>", value)
    value = UUID.sub("<uuid>", value)
    value = SHA.sub("<sha>", value)
    value = TIME.sub("<time>", value)
    value = DURATION.sub("<duration>", value)
    return PORT.sub("<port>", value)


def name(nodeid: str) -> str:
    """A file name for a test id: its characters outside [\\w.-] as _, a long one cut and made unique by a digest."""
    stem = re.sub(r"[^\w.-]", "_", nodeid.replace("::", "__"))
    return stem if len(stem) <= 150 else f"{stem[:130]}-{hashlib.sha1(nodeid.encode()).hexdigest()[:12]}"


def write(directory: Path, nodeid: str, paths: dict[str, str]) -> Path:
    """Write the test's transcript, normalised, as <directory>/<test file>/<test>.json."""
    file, _, test = nodeid.partition("::")
    path = directory / Path(file).stem / f"{name(test)}.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    data = normalise({"test": nodeid, "calls": CALLS}, paths)
    path.write_text(json.dumps(data, indent=1, ensure_ascii=False) + "\n")
    return path
