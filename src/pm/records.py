"""Parse and validate records: Markdown files with a YAML header under records/.

A record's `type` decides its required header fields and sections. Fenced-div
blocks are limited to the names in BLOCKS. Sections carry line ranges so the
CLI can insert into a named section and leave the rest of the file untouched.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

import yaml
from markdown_it import MarkdownIt

REQUIRED = {
    "project": ["title", "bead"],
    "sprint": ["title", "bead"],
    "day": ["date"],
    "design": ["title", "project"],
    "doc": ["title", "date"],      # plus exactly one of bead or project
    "postmortem": ["title", "date"],  # plus exactly one of sprint or project
}
SECTIONS = {
    "project": ["## Goal", "## Progress", "## Decisions", "## Design pages", "## Outcome"],
    "sprint": ["## Goal", "## Scope", "## Done when", "## Design pages", "## Progress", "## Decisions", "## Findings",
               "## Delivery report"],
    "day": [],  # day records are no longer written; older ones keep their hand-written ## Today
    # Prior art is the template's one optional section, so it is not listed.
    "design": ["## Problem", "## Goals and non-goals", "## Constraints and key facts", "## Design",
               "## Alternatives considered", "## Open questions"],
    "postmortem": ["## Summary", "## Timeline", "## Cost", "## Root cause", "## What changed",
                   "## What would have caught it earlier"],
}
# Sections the renderer generates (harness/site.py). A record keeps an empty Progress section, prompt lines only, and
# never holds a heading named like one the renderer adds: its text would sit beside or under the generated content.
GENERATED_BODY = {"project": ["Progress"], "sprint": ["Progress"]}
GENERATED_HEADINGS = {"project": ["Docs", "Postmortems"],
                      "sprint": ["Decisions await you", "Actions await you", "Docs", "Postmortems"],
                      "day": ["Decisions await you", "Actions await you", "Sprints", "Docs"]}
NOT_CLOSED = "Not closed yet."  # placeholder body of a close-time section
NONE_YET = "None yet."          # placeholder body of an empty list section
REPORT_PARTS = ["Outcome", 'Against "Done when"']
BLOCKS = {"note", "decision", "result"}
BLOCK_ATTRS = {"decision": ["source", "date"], "result": ["title"]}
ATTR_RE = re.compile(r'(\w+)=("([^"]*)"|[^\s}]+)')
HEADER_RE = re.compile(r"---\n(.*?)\n---\n", re.S)


class RecordError(Exception):
    pass


def attrs(info: str) -> dict[str, str]:
    return {m.group(1): m.group(3) if m.group(3) is not None else m.group(2) for m in ATTR_RE.finditer(info)}


@dataclass
class Record:
    path: Path
    rel: str                     # path under records/, without suffix
    meta: dict
    body: str
    text: str = ""               # the whole file, header included; empty for a day page generated without a file
    summary: dict | None = None  # a day's generated summary (read_summaries), set by site.with_days
    out: str = field(init=False)

    def __post_init__(self):
        self.out = self.rel + ".html"

    @property
    def type(self) -> str:
        return self.meta["type"]

    @property
    def title(self) -> str:
        return str(self.meta.get("title") or self.meta.get("date") or self.rel)

    @property
    def name(self) -> str:
        """File name without suffix: the project name of a project record."""
        return self.rel.split("/")[-1]


def decisions(text: str) -> list[tuple[dict[str, str], str]]:
    """(attributes, body) of each ::: decision block in a record's text."""
    return [(attrs(m.group(1)), m.group(2))
            for m in re.finditer(r"(?ms)^::: decision(.*?)\n(.*?)^:::\s*$", text)]


def check_blocks(text: str, rel: str) -> None:
    for line in text.splitlines():
        m = re.match(r"^:::\s*(\w+)(.*)$", line)
        if not m:
            continue
        if m.group(1) not in BLOCKS:
            raise RecordError(f"{rel}: unknown block '::: {m.group(1)}' (allowed: {', '.join(sorted(BLOCKS))})")
        a = attrs(m.group(2))
        for req in BLOCK_ATTRS.get(m.group(1), []):
            if req not in a:
                raise RecordError(f"{rel}: ::: {m.group(1)} needs '{req}'")


DATED = {"doc": ("docs", "bead"), "postmortem": ("postmortems", "sprint")}  # folder, and the key besides project


def check_dated_header(meta: dict, rel: str) -> None:
    """A doc or postmortem names exactly one of its key (bead or sprint) or project, and lives at
    <folder>/<date>-<slug>."""
    kind = meta["type"]
    folder, key = DATED[kind]
    if (key in meta) == ("project" in meta):
        raise RecordError(f"{rel}: a {kind} names exactly one of '{key}' or 'project' in its header")
    day = str(meta["date"])
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", day):
        raise RecordError(f"{rel}: date must be YYYY-MM-DD, got {day!r}")
    if not re.fullmatch(rf"{folder}/{day}-[a-z0-9]+(-[a-z0-9]+)*", rel):
        raise RecordError(f"{rel}: a {kind} dated {day} lives at {folder}/{day}-<slug>.md "
                          "(slug: lowercase words joined by '-')")


def parse_record(path: Path, rel: str, text: str) -> Record:
    m = HEADER_RE.match(text)
    if not m:
        raise RecordError(f"{rel}: missing YAML header")
    meta = yaml.safe_load(m.group(1)) or {}
    kind = meta.get("type")
    if kind not in REQUIRED:
        raise RecordError(f"{rel}: type must be one of {', '.join(REQUIRED)}, got {kind!r}")
    missing = [k for k in REQUIRED[kind] if not meta.get(k)]
    if missing:
        raise RecordError(f"{rel}: missing header fields: {', '.join(missing)}")
    if kind in DATED:
        check_dated_header(meta, rel)
    check_blocks(text, rel)
    return Record(path, rel, meta, text[m.end():], text)


def record_texts(root: Path) -> dict[Path, str]:
    """The text of every record file under root, by resolved path."""
    return {p.resolve(): p.open(newline="").read() for p in root.resolve().rglob("*.md")}


def parse_records(root: Path, texts: dict[Path, str]) -> list[Record]:
    """The records of `texts` (resolved path to text, as record_texts gives), in path order."""
    root = root.resolve()
    return [parse_record(p, p.relative_to(root).with_suffix("").as_posix(), texts[p]) for p in sorted(texts)]


def read_records(root: Path, overrides: dict[Path, str] | None = None) -> list[Record]:
    """All records under root. `overrides` maps a path to planned text, existing or new."""
    return parse_records(root, record_texts(root) | {p.resolve(): t for p, t in (overrides or {}).items()})


def summary_path(root: Path, day: str) -> Path:
    """Where pm day summarize stores a day's generated summary."""
    return root / "days" / f"{day}.summary.json"


def read_summaries(root: Path) -> dict[str, dict]:
    """Every generated day summary under root, by date: {"date", "generated_at", "digest", "text"}."""
    return {p.name.split(".")[0]: read_summary(p) for p in sorted((root / "days").glob("*.summary.json"))}


def read_summary(path: Path) -> dict:
    """One generated day summary; a RecordError naming the file when it is not one, since every page reads them."""
    name = f"days/{path.name}"
    try:
        data = json.loads(path.read_text())
    except (json.JSONDecodeError, UnicodeDecodeError) as e:
        raise RecordError(f"{name}: not valid JSON ({e}); fix it or delete it and run pm day summarize")
    if not isinstance(data, dict) or not all(isinstance(data.get(k), str) for k in ("date", "generated_at", "digest",
                                                                                      "text")):
        raise RecordError(f"{name}: a summary is a JSON object with string fields date, generated_at, digest and "
                          "text; fix it or delete it and run pm day summarize")
    if data["date"] != path.name.split(".")[0]:
        raise RecordError(f"{name}: its date is {data['date']!r}, not the date in its name")
    try:
        datetime.fromisoformat(data["generated_at"])
    except ValueError:
        raise RecordError(f"{name}: generated_at {data['generated_at']!r} is not an ISO timestamp")
    return data


# ---------------------------------------------------------------- sections

def section_ranges(text: str, level: int = 2) -> list[tuple[str, int, int]]:
    """(name, heading line, end line exclusive) of each heading of `level`, skipping code fences.
    A section ends at the next heading of the same or a higher level."""
    lines = text.split("\n")
    heads, fence = [], None
    for n, line in enumerate(lines):
        f = re.match(r"^(```|~~~)", line)
        if f:
            fence = None if fence == f.group(1) else (fence or f.group(1))
            continue
        h = re.match(r"^(#{1,6}) (.*)$", line)
        if fence is None and h:
            heads.append((len(h.group(1)), h.group(2).strip(), n))
    out = []
    for i, (lvl, name, start) in enumerate(heads):
        if lvl != level:
            continue
        end = next((s for l2, _, s in heads[i + 1:] if l2 <= level), len(lines))
        out.append((name, start, end))
    return out


def headings(body: str) -> list[tuple[int, str, int]]:
    """(level, text, line) of each heading in a record body, as the renderer parses it: a '#' line inside a code
    fence or a raw HTML block is not a heading."""
    tokens = MarkdownIt("commonmark", {"html": True}).enable("table").parse(body)
    return [(int(t.tag[1]), tokens[i + 1].content, t.map[0]) for i, t in enumerate(tokens) if t.type == "heading_open"]


def section_range(text: str, name: str) -> tuple[int, int] | None:
    return next(((s, e) for n, s, e in section_ranges(text) if n == name), None)


def strip_prompts(text: str) -> str:
    return "\n".join(l for l in text.strip().splitlines() if not l.startswith(">")).strip()


def section_text(body: str, name: str) -> str:
    """Text of a '## name' section, without its prompt lines."""
    m = re.search(rf"^## {re.escape(name)}\n(.*?)(?=^## |\Z)", body, re.S | re.M)
    if not m:
        return ""
    return strip_prompts(m.group(1))


def subsection_text(text: str, name: str) -> str:
    """Text of a '### name' subsection within a section's text, without prompt lines."""
    m = re.search(rf"^### {re.escape(name)}\n(.*?)(?=^### |\Z)", text, re.S | re.M)
    if not m:
        return ""
    return strip_prompts(m.group(1))


def first_para(text: str) -> str:
    return re.sub(r"\s+", " ", text.split("\n\n")[0]).strip()


def summary_line(text: str) -> str:
    """A day summary on one line for lists: its bullets ("- " or "* ") joined with " · ", else its first paragraph."""
    items = re.findall(r"(?m)^\s*[-*]\s+(.+?)\s*$", text)
    return " · ".join(items) if items else first_para(text)


def outcome(rec: Record) -> str:
    """The delivery report's outcome, or '' while the sprint is not closed."""
    m = re.search(r"^## Delivery report\n(.*?)(?=^## |\Z)", rec.body, re.S | re.M)
    report = m.group(1) if m else ""
    for part in REPORT_PARTS:
        if not re.search(rf"^### {re.escape(part)}$", report, re.M):
            raise RecordError(f"{rec.rel}: Delivery report needs a '### {part}' subsection")
    line = first_para(subsection_text(report, "Outcome"))
    if line == NOT_CLOSED:
        return ""
    if not re.match(r"(done|partial|voided)\b", line, re.I):
        raise RecordError(f"{rec.rel}: Delivery report Outcome must start with done, partial or voided")
    return line


def report_part(rec: Record, part: str) -> str:
    m = re.search(r"^## Delivery report\n(.*?)(?=^## |\Z)", rec.body, re.S | re.M)
    return subsection_text(m.group(1) if m else "", part)


def insert_entry(text: str, section: str, entry: str) -> str:
    """Insert `entry` (one or more lines) at the end of a section, replacing its 'None yet.'
    placeholder if that is all the section holds. The rest of the file is unchanged."""
    rng = section_range(text, section)
    if rng is None:
        raise RecordError(f"no '## {section}' section")
    start, end = rng
    lines = text.split("\n")
    body = [n for n in range(start + 1, end) if lines[n].strip() and not lines[n].startswith(">")]
    new = entry.rstrip("\n").split("\n")
    if [lines[n].strip() for n in body] == [NONE_YET]:
        lines[body[0]:body[0] + 1] = new
    else:
        last = body[-1] if body else max(n for n in range(start, end) if lines[n].strip())
        lines[last + 1:last + 1] = [""] + new
    return "\n".join(lines)


# ---------------------------------------------------------------- relations and validation

def project_of(rec: Record, recs: list[Record], beads: dict[str, dict]) -> Record | None:
    """The project record a record belongs to: by Beads parent for sprints, by `project:` otherwise."""
    projects = [r for r in recs if r.type == "project"]
    if rec.type == "project":
        return rec
    if rec.type == "sprint":
        parent = beads.get(rec.meta["bead"], {}).get("parent")
        return next((p for p in projects if p.meta["bead"] == parent), None)
    if rec.type == "doc" and "bead" in rec.meta:
        from .beads import ancestors
        chain = [rec.meta["bead"], *ancestors(beads, rec.meta["bead"])]
        return next((p for b in chain for p in projects if p.meta["bead"] == b), None)
    if rec.type == "postmortem" and "sprint" in rec.meta:
        sprint = next((r for r in recs if r.type == "sprint" and r.meta["bead"] == rec.meta["sprint"]), None)
        return project_of(sprint, recs, beads) if sprint else None
    return next((p for p in projects if p.name == rec.meta.get("project")), None)


def check_generated(rec: Record) -> None:
    """Fail on hand-written text in a generated section: a non-prompt line in Progress, or a heading the renderer
    adds itself (a day's or sprint's Decisions await you and Actions await you; a day's Sprints and Docs; Docs and
    Postmortems lists)."""
    lines = rec.body.split("\n")
    line0 = rec.text.count("\n") - rec.body.count("\n")  # file line of the body's first line, 0-based
    for name, start, end in section_ranges(rec.body):
        if name in GENERATED_BODY.get(rec.type, []):
            text = next((n for n in range(start + 1, end) if lines[n].strip() and not lines[n].startswith(">")), None)
            if text is not None:
                raise RecordError(f"{rec.rel}.md:{line0 + text + 1}: hand-written text in '## {name}', which is "
                                  "generated from Beads when the page is rendered; move it to Findings or Decisions, "
                                  "or remove it")
        if name in GENERATED_HEADINGS.get(rec.type, []):
            raise RecordError(f"{rec.rel}.md:{line0 + start + 1}: '## {name}' is a section the page generates; "
                              "remove the heading and move its text to a section written by hand")


def validate_record(rec: Record, recs: list[Record], beads: dict[str, dict]) -> None:
    if rec.type in ("project", "sprint", "doc") and "bead" in rec.meta and rec.meta["bead"] not in beads:
        raise RecordError(f"{rec.rel}: bead {rec.meta['bead']} not found in Beads")
    if rec.type == "postmortem" and "sprint" in rec.meta and not any(
            r.type == "sprint" and r.meta["bead"] == rec.meta["sprint"] for r in recs):
        raise RecordError(f"{rec.rel}: no sprint record has bead {rec.meta['sprint']}")
    if rec.type != "day" and project_of(rec, recs, beads) is None:  # days are repo-wide
        raise RecordError(f"{rec.rel}: no project record found")
    for sec in SECTIONS.get(rec.type, []):
        if not re.search(rf"^{re.escape(sec)}$", rec.body, re.M):
            raise RecordError(f"{rec.rel}: {rec.type} record needs a '{sec}' section")
    check_generated(rec)
