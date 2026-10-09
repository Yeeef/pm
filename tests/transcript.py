"""The differential transcript: every `Repo.pm` call of one test, normalised so that two implementations' runs of
the test compare equal when they behave the same.

A call holds its argv (without the pm binary), stdin, stdout, stderr, exit code, the record files it changed (path to
new text, null when removed) and the store export after it. The normaliser replaces what differs between two runs,
keeping its shape so a difference in format still shows: the temp and checkout paths and random temp names, git
commit ids and UUIDs (numbered by first appearance, with their length), timestamps and today's date (each digit as
0), durations and ports (the number only), and root ids pm minted (in order of appearance; the seeded ids are kept).
Some things differ by design until the cut-over, and each becomes one form: pm where's work-layer line, where Python pm
names Beads and Go pm its work store; the lines pm init prints as it connects the clone to its work layer,
Python pm's bd bootstrap, .beads mode, beads.role and agent profile, Go pm's work store cloned, created, pointed
at or pushed to the remote (both dropped); pm push's work-layer step, Python pm's bd dolt push and Go pm's
work-store sync; the pm service's log, whose timing lines name each one's store, so only its Serving lines are
kept; pm service status's gc line, which only Go pm's service has (it collects the work store); the git hooks
directory core.hooksPath names, Beads' .beads/hooks in Python and pm's own .pm/hooks in Go; the installed pm that
pm init's refusal names, Python's pm uv tool and Go's release binary; the rules pm prime prints, each
implementation's own prime.md (Python's names Beads and bd, Go's the work store and pm's commands); the launcher's
name for itself when it runs a Python pin, "the pm uv tool (pm X)" in Python and "pm X" in Go; the command a need's
line in pm show names, `bd show` in Python and `pm show` in Go; pm decision close's undo, bd's label removal in
Python and pm decision add --need in Go; and the comment that
holds the answer a need closes with (pm decision add --need, pm decision close), which Python pm's `bd human respond`
writes as a note "Response: <text>" by the git user and Go pm's work store as a reply by the owner (work.Answer)."""

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
# pm init: Python pm's Beads setup lines, Go pm's work-store setup lines (install.SetupWork)
WORK_SETUP = re.compile(r"(?m)^(?:set up the Beads database \(.*|made .* private \(0700\), as bd asks"
                        r"|set beads\.role to maintainer|set the Beads agent profile to team-maintainer"
                        r"|cloned the work store from .*|created the work store at .*|pointed the work store .*"
                        r"|pushed the work store to .*)\n")
WORK_PUSH = re.compile(r"(?m)^(\S+) (?:beads|work) (ok|error): .*$")  # pm push: bd dolt push, or the work store's sync
SERVED_LAYER = re.compile(r"; (?:Beads reread when \S+ changes|Beads reread every look \(Dolt server\)|"
                          r"the work store reread when it changes);")  # the service's Serving line
GC_LINE = re.compile(r"(?m)^gc        .*\n")  # pm service status: Go pm's collection of its work store
# core.hooksPath as pm init sets it and pm where shows it: Beads' .beads/hooks in Python, pm's own .pm/hooks in Go
HOOKS_DIR = re.compile(r"(?<=/)(?:\.beads|\.pm)/hooks\b")
# the pm on PATH, in pm init's refusal: Python's pm uv tool, Go's installed release binary
INSTALLED_PM = re.compile(r"\bthe (?:one pm uv tool|pm uv tool|installed pm)\b")
_PM = Path(__file__).resolve().parents[1]
# pm prime's rules, each implementation's own prime.md, whole
RULES = [(_PM / "src/pm/prime.md").read_text(encoding="utf-8").strip(), (_PM / "prime.md").read_text(encoding="utf-8").strip()]
# the launcher running a Python pin: Python's pm uv tool, Go's installed pm (INSTALLED_PM has run first)
PIN_RUNNER = re.compile(r"which (?:<the installed pm> \(pm (\S+)\)|pm (\S+)) runs through uv")
SHOW_ITEM = re.compile(r"-> (?:bd|pm) show ")  # pm show's need lines: the command that shows the need
UNDO = re.compile(r"; (?:undo with: bd update (\S+) --remove-label=no-decision|if it sets a rule after all, record it "
                  r"with pm decision add --need (\S+), which marks the need answered)")


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
    value = WORK_SETUP.sub("", value)
    value = WORK_PUSH.sub(r"\1 <work layer: bd dolt push or the work store's sync> \2", value)
    value = SERVED_LAYER.sub("; <work layer> reread when it changes;", value)
    value = GC_LINE.sub("", value)
    value = HOOKS_DIR.sub("<pm's git hooks dir>", value)
    value = INSTALLED_PM.sub("<the installed pm>", value)
    for rules in RULES:
        value = value.replace(rules, "<prime.md>")
    value = PIN_RUNNER.sub(lambda m: f"which <the launcher, pm {m.group(1) or m.group(2)}> runs through uv", value)
    value = SHOW_ITEM.sub("-> <show the item> ", value)
    value = UNDO.sub(lambda m: f"; <undo no-decision on {m.group(1) or m.group(2)}>", value)
    return PORT.sub("<port>", value)


def answer(comment: dict) -> dict:
    """A need's answer comment in one form, whichever store wrote it: Python pm's note "Response: <text>", or Go pm's
    reply by the owner that no site wrote (a site reply ends with its pm-reply mark)."""
    text = comment.get("text") or ""
    if comment.get("kind") == "note" and text.startswith("Response: "):
        return {**comment, "kind": "<answer>", "author": "<answer>", "text": text.removeprefix("Response: ")}
    if comment.get("kind") == "reply" and comment.get("author") == "owner" and "<!-- pm-reply" not in text:
        return {**comment, "kind": "<answer>", "author": "<answer>"}
    return comment


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
    for call in data["calls"]:  # by normalised id: a minted root id sorts apart from the seeds by its placeholder
        if call["argv"][:2] == ["service", "logs"]:
            call["stdout"] = "".join(l for l in call["stdout"].splitlines(True) if l.startswith("Serving "))
        if isinstance(call["export"], list):
            call["export"].sort(key=lambda i: i["id"])
            for item in call["export"]:
                item["comments"] = [answer(c) for c in item["comments"]]
    path.write_text(json.dumps(data, indent=1, ensure_ascii=False) + "\n")
    return path
