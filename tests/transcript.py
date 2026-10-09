"""The differential transcript: every `Repo.pm` call of one test, normalised so that two implementations' runs of
the test compare equal when they behave the same.

A call holds its argv (without the pm binary), stdin, stdout, stderr, exit code, the record files it changed (path to
new text, null when removed) and the store export after it. The normaliser replaces what differs between two runs,
keeping its shape so a difference in format still shows: the temp and checkout paths and random temp names, git
commit ids and UUIDs (numbered by first appearance, with their length), timestamps and today's date (each digit as
0), durations and ports (the number only), and root ids pm minted (in order of appearance; the seeded ids are kept).
One line differs by design until the cut-over: pm where's work-layer line, where Python pm names Beads and Go pm its
work store; each becomes one placeholder."""

from __future__ import annotations

import hashlib
import json
import re
from datetime import date, datetime, timezone
from pathlib import Path

CALLS: list[dict] = []
ROOTS: list[str] = []
IDS: dict[str, str] = {}  # each commit id or UUID seen, by its placeholder

TEMP_NAME = re.compile(r"(<systmp>/[\w.-]*?)[a-z0-9_]{8}(?![\w.-])")  # tempfile.mkdtemp's 8 random characters
UUID = re.compile(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b")
# a commit id, or a digest cut like one; with a digit, so a word like "defaced" stays (1 in 27 short ids is digits only)
SHA = re.compile(r"\b(?=[0-9a-f]*\d)[0-9a-f]{7,40}\b")
TIME = re.compile(r"\b\d{4}-\d\d-\d\d[T ]\d\d:\d\d(?::\d\d(?:\.\d+)?)?(?:Z|[+-]\d\d:?\d\d)?")
DURATION = re.compile(r"\b\d+(?:\.\d+)?(?= ?(?:ms|s|min)\b|[mhd]\b)")
PORT = re.compile(r"(?:(?<=localhost:)|(?<=127\.0\.0\.1:)|(?<= :))\d{2,5}\b")
WORK_LAYER = re.compile(r"(?m)^(?:beads     |work      )\S.*$")  # pm where: Python's Beads line, Go's work-store line


def start() -> None:
    CALLS.clear()
    ROOTS.clear()
    IDS.clear()


def record(call: dict, roots: list[str]) -> None:
    CALLS.append(call)
    ROOTS.extend(r for r in roots if r not in ROOTS)


def numbered(kind: str):
    """A substitution giving each distinct id `<kind><length>-<n>`, n counting this kind's ids by first appearance."""
    def sub(m: re.Match) -> str:
        if m.group(0) not in IDS:
            n = 1 + sum(p.startswith(f"<{kind}") for p in IDS.values())
            IDS[m.group(0)] = f"<{kind}{len(m.group(0))}-{n}>"
        return IDS[m.group(0)]
    return sub


def zeroed(m: re.Match) -> str:
    return re.sub(r"\d", "0", m.group(0))


def normalise(value, paths: dict[str, str]):
    """`value` with, in each string, each path in `paths` (longest first) and each minted root id replaced by its
    placeholder, then random temp names, UUIDs, commit ids, timestamps, today's date, durations and ports."""
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
    value = UUID.sub(numbered("uuid"), value)
    value = SHA.sub(numbered("sha"), value)
    value = TIME.sub(zeroed, value)
    for today in {date.today().isoformat(), datetime.now(timezone.utc).date().isoformat()}:
        value = value.replace(today, "0000-00-00")
    value = DURATION.sub("0", value)
    value = WORK_LAYER.sub("<work layer: Beads or the work store>", value)
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
