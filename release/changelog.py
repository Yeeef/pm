#!/usr/bin/env python3
"""Check CHANGELOG.md and the entry files in changelog.d/, print a release's notes, and write a release's section
(AGENTS.md, Releasing pm). Standard library only.

  changelog.py check                         the format CHANGELOG.md's preamble states, for CHANGELOG.md and each
                                             changelog.d/<slug>.md; fails naming the file and line
  changelog.py notes X                       the body of release X; for X-rc.N, X's section, else the entry files'
  changelog.py pr BASE                       fails when the changes since BASE touch a shipped path and add or change
                                             no entry file
  changelog.py release X --summary TEXT      writes '## [X] - <date>' into CHANGELOG.md from the entry files, with its
                     [--date YYYY-MM-DD]     link reference, and deletes them

Every command fails hard (exit 1, the reason on stderr) on a changelog or an entry file that does not pass check.
"""

from __future__ import annotations

import argparse
import datetime
import re
import subprocess
import sys
import textwrap
from dataclasses import dataclass, field
from pathlib import Path

REPO = "https://github.com/Yeeef/pm"
ROOT = Path(__file__).resolve().parents[1]
CHANGELOG = ROOT / "CHANGELOG.md"
ENTRIES = "changelog.d"  # one file per unreleased change, relative to ROOT
ENTRY_NAME = re.compile(r"^[a-z0-9][a-z0-9-]*\.md$")
GUIDE = "Upgrade guide"  # first in every release: the numbered steps from the previous release to this one
CATEGORIES = [GUIDE, "Breaking changes", "Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"]
STEP = re.compile(r"^(\d+)\. \S")
WIDTH = 118  # the changelog's line width, for the summary release writes
# What a Go release builds or serves: the binary's sources and embedded files, its build and its install script.
# Go tests and their data ship nothing.
SHIPPED = ("cmd/", "internal/", "assets.go", "prime.md", "style.css", "prompts/", "go.mod", "go.sum", "install.sh",
           "release/build.sh")

VERSION = r"(\d+)\.(\d+)\.(\d+)"
RELEASE_HEADING = re.compile(rf"^## \[({VERSION})\] - (\d{{4}}-\d{{2}}-\d{{2}})$")
LINK_REF = re.compile(r"^\[([^\]]+)\]: (\S+)$")


class ChangelogError(Exception):
    pass


@dataclass
class Section:
    name: str  # the version, or an entry file's path
    line: int  # the heading's line number, from 1
    where: str = "CHANGELOG.md"
    date: str = ""
    lines: list[tuple[int, str]] = field(default_factory=list)  # the section's text below its heading
    summary: list[str] = field(default_factory=list)
    categories: dict[str, list[tuple[int, str]]] = field(default_factory=dict)  # in file order: (line number, line)

    @property
    def key(self) -> tuple[int, ...]:
        return tuple(int(p) for p in self.name.split("."))

    def body(self) -> str:
        """The section without its heading, each paragraph and bullet on one line: GitHub renders a release body's
        line breaks as breaks, so the changelog's wrapped lines are joined."""
        parts = []
        if text := "\n".join(self.summary).strip():
            parts.append(text)
        for name, lines in self.categories.items():
            parts.append(f"### {name}\n\n" + "\n".join(line for _, line in lines).strip())
        return unwrap("\n\n".join(parts))

    def markdown(self) -> str:
        """The section as CHANGELOG.md holds it, heading included, lines as written."""
        out = [f"## [{self.name}] - {self.date}", "", *self.summary, ""]
        for name, lines in self.categories.items():
            out += [f"### {name}", "", *(line for _, line in lines), ""]
        return "\n".join(out)


def unwrap(text: str) -> str:
    """Join each line that continues a paragraph or a bullet onto the line before it."""
    out: list[str] = []
    for line in text.splitlines():
        item = line.lstrip().startswith(("- ", "#")) or STEP.match(line.lstrip())
        if line and out and out[-1] and not item:
            out[-1] += " " + line.strip()
        else:
            out.append(line)
    return "\n".join(out)


@dataclass
class Changelog:
    sections: list[Section]  # the releases, newest first
    links: dict[str, tuple[int, str]]  # name: (line number, URL)

    def release(self, version: str) -> Section | None:
        return next((s for s in self.sections if s.name == version), None)


def fail(line: int, message: str, where: str = "CHANGELOG.md"):
    raise ChangelogError(f"{where}:{line}: {message}")


def parse(text: str) -> Changelog:
    """Parse and check the whole changelog: the rules CHANGELOG.md's preamble states."""
    sections: list[Section] = []
    links: dict[str, tuple[int, str]] = {}
    title = False
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.rstrip()
        if m := LINK_REF.match(line):
            if m[1] in links:
                fail(n, f"link reference [{m[1]}] is defined twice")
            links[m[1]] = (n, m[2])
        elif line.startswith("# "):
            if title or sections:
                fail(n, "only the first heading may be a '# ' title")
            title = True
        elif line == "## [Unreleased]":
            fail(n, f"CHANGELOG.md holds released sections only: an unreleased change is its own file, "
                    f"{ENTRIES}/<slug>.md (the preamble); move this section's entries there")
        elif line.startswith("## "):
            if not (m := RELEASE_HEADING.match(line)):
                fail(n, f"a release heading is '## [X.Y.Z] - YYYY-MM-DD', not {line!r}")
            try:
                datetime.date.fromisoformat(m[5])
            except ValueError:
                fail(n, f"{m[5]} is not a valid date")
            sections.append(Section(m[1], n, date=m[5]))
        elif sections:
            sections[-1].lines.append((n, line))
        elif line.startswith("#"):
            fail(n, f"heading {line!r} is outside any release section")
    for section in sections:
        read_body(section, summary=True)
        check_section(section)
    check_order(sections)
    check_links(sections, links)
    return Changelog(sections, links)


def read_body(section: Section, summary: bool):
    """Split the section's lines into its summary (the text before its first category; only a release has one) and
    its categories, checking each category's form."""
    category: list[tuple[int, str]] | None = None
    category_name = ""
    for n, line in section.lines:
        if line.startswith("#"):
            if not line.startswith("### "):
                fail(n, f"only '#' title, '## ' section and '### ' category headings are allowed, not {line!r}"
                     if summary else f"an entry holds only '### ' category headings, not {line!r}", section.where)
            name = line[4:]
            if name not in CATEGORIES:
                fail(n, f"unknown category {name!r}; the categories are, in this order: {', '.join(CATEGORIES)}",
                     section.where)
            if name in section.categories:
                fail(n, f"category {name!r} appears twice in {section_label(section)}", section.where)
            if section.categories and CATEGORIES.index(name) < CATEGORIES.index(list(section.categories)[-1]):
                fail(n, f"category {name!r} comes after {list(section.categories)[-1]!r}; the order is "
                        f"{', '.join(CATEGORIES)}", section.where)
            category = section.categories[name] = []
            category_name = name
            continue
        if category is None:
            if summary:
                section.summary.append(line)
            elif line:
                fail(n, "an entry holds only '### ' categories and their items; its first line is a category "
                        "heading such as '### Fixed'", section.where)
            continue
        started = any(t for _, t in category)
        if category_name == GUIDE:
            if line and not (line.startswith("  ") and started):
                step = sum(1 for _, t in category if STEP.match(t)) + 1
                if not (m := STEP.match(line)) or int(m[1]) != step:
                    fail(n, f"{GUIDE!r} is a numbered list ('1. ', '2. ', …, with indented continuation lines and "
                            f"sub-bullets); step {step} should start '{step}. '", section.where)
        elif line and not (line.startswith("- ") or (line.startswith("  ") and started)):
            fail(n, "a category holds only '- ' bullets, with indented continuation lines and sub-bullets",
                 section.where)
        category.append((n, line))
    for name, lines in section.categories.items():
        if not any(line for _, line in lines):
            fail(section.line, f"{section_label(section)} has an empty {name!r} category", section.where)


def section_label(section: Section) -> str:
    return f"[{section.name}]" if section.where == "CHANGELOG.md" else "the entry"


def check_section(section: Section):
    if not "\n".join(section.summary).strip():
        fail(section.line, f"[{section.name}] needs a summary paragraph before its first category")
    if GUIDE not in section.categories:
        fail(section.line, f"[{section.name}] needs an '### {GUIDE}' first: the numbered steps that move a repo from "
                           "the previous release to this one")


def check_order(sections: list[Section]):
    for newer, older in zip(sections, sections[1:]):
        if older.key >= newer.key:
            fail(older.line, f"releases go newest first: {older.name} comes after {newer.name}")
        if older.date > newer.date:
            fail(older.line, f"{older.name}'s date {older.date} is after {newer.name}'s {newer.date}")


def check_links(sections: list[Section], links: dict[str, tuple[int, str]]):
    want = {newer.name: f"{REPO}/compare/pm-v{older.name}...pm-v{newer.name}"
            for newer, older in zip(sections, sections[1:])}
    for section in sections:
        if section.name not in links:
            fail(section.line, f"[{section.name}] has no link reference; add [{section.name}]: "
                               f"{want.get(section.name, f'{REPO}/compare/pm-v<previous>...pm-v{section.name}')}")
        url = links[section.name][1]
        if section.name in want and url != want[section.name]:
            fail(section.line, f"[{section.name}] links {url}, not {want[section.name]}")
        if section.name not in want and not (url.startswith(f"{REPO}/") and url.endswith(f"pm-v{section.name}")):
            fail(section.line, f"[{section.name}] links {url}, which is not {REPO}'s pm-v{section.name}")
    names = {s.name for s in sections}
    for name, (n, _) in links.items():
        if name not in names:
            fail(n, f"link reference [{name}] names no release section")


def parse_entry(name: str, text: str) -> Section:
    """Parse and check one entry file: '### ' categories of a release, nothing else."""
    where = f"{ENTRIES}/{name}"
    if not ENTRY_NAME.match(name):
        fail(1, f"an entry file is named {ENTRIES}/<slug>.md, the slug lowercase letters, digits and '-'", where)
    entry = Section(where, 1, where=where, lines=[(n, raw.rstrip()) for n, raw in enumerate(text.splitlines(), 1)])
    read_body(entry, summary=False)
    if not entry.categories:
        fail(1, "an entry holds at least one '### ' category, such as '### Fixed', with its bullets", where)
    return entry


def read_entries(root: Path) -> list[Section]:
    """The entry files, in name order."""
    directory = root / ENTRIES
    if not directory.exists():
        return []
    return [parse_entry(p.name, p.read_text()) for p in sorted(directory.iterdir())]


def assemble(version: str, entries: list[Section], date: str = "", summary: tuple[str, ...] = ()) -> Section:
    """Release X's section from the entry files: an upgrade guide that opens with installing X and
    `pm upgrade --to X`, then the entries' own steps, renumbered; then each category's items, entries in name order."""
    section = Section(version, 0, date=date, summary=list(summary))
    section.categories[GUIDE] = [(0, line) for line in (
        "1. On each machine, install the release:",
        f"   `curl -fsSL {REPO}/releases/download/pm-v{version}/install.sh | sh`.",
        f"2. In the repo, run `pm upgrade --to {version}`, then commit what it changes and merge it, as an ordinary "
        "PR.")]
    step = 2
    for name in CATEGORIES:
        for entry in entries:
            lines = [line for _, line in entry.categories.get(name, [])]
            while lines and not lines[-1]:
                lines.pop()
            while lines and not lines[0]:
                lines.pop(0)
            if name == GUIDE:
                for i, line in enumerate(lines):
                    if m := STEP.match(line):
                        step += 1
                        lines[i] = f"{step}{line[len(m[1]):]}"
            if lines:
                section.categories.setdefault(name, []).extend((0, line) for line in lines)
    return section


def notes(log: Changelog, entries: list[Section], version: str) -> str:
    """Release X's body: its section without the heading. A pre-release X-<suffix> takes X's section when there is
    one, else the section the entry files assemble into."""
    if not (m := re.fullmatch(rf"{VERSION}(-[0-9A-Za-z.-]+)?", version)):
        raise ChangelogError(f"{version!r} is not a version X.Y.Z or X.Y.Z-<pre-release>")
    base = version.split("-", 1)[0]
    section = log.release(base)
    if section is None and m[4]:
        if not entries:
            raise ChangelogError(f"{ENTRIES}/ holds no entries; pre-release {version} takes its notes from them")
        section = assemble(version, entries)
    if section is None:
        raise ChangelogError(f"CHANGELOG.md has no '## [{base}] - <date>' section; write release {base}'s notes there "
                             f"(changelog.py release {base}) before tagging it (AGENTS.md, Releasing pm)")
    return section.body()


def wrap(text: str) -> list[str]:
    """text wrapped at WIDTH or less, so that no line but the first starts as a list item or a heading would: the
    notes would keep it as one."""
    for width in range(WIDTH, WIDTH // 2, -1):
        lines = textwrap.wrap(text, width, break_long_words=False, break_on_hyphens=False)
        if not any(line.startswith(("- ", "#")) or STEP.match(line) for line in lines[1:]):
            return lines
    raise ChangelogError("the summary breaks into a list item or heading at every width; reword it")


def release(text: str, entries: list[Section], version: str, date: str, summary: str) -> str:
    """CHANGELOG.md with release X's section, assembled from the entry files, above the newest release, and its link
    reference above the others."""
    if not re.fullmatch(VERSION, version):
        raise ChangelogError(f"{version!r} is not a version X.Y.Z; a pre-release takes its notes from the entry files")
    log = parse(text)
    if log.release(version):
        raise ChangelogError(f"CHANGELOG.md already has [{version}]")
    if not entries:
        raise ChangelogError(f"{ENTRIES}/ holds no entries: release {version} would have nothing to say")
    if not summary.strip():
        raise ChangelogError("a release opens with a summary paragraph: give it with --summary")
    section = assemble(version, entries, date, tuple(wrap(summary)))
    newest = log.sections[0].name if log.sections else None
    link = f"[{version}]: " + (f"{REPO}/compare/pm-v{newest}...pm-v{version}" if newest
                               else f"{REPO}/releases/tag/pm-v{version}")
    lines = text.splitlines()
    at = next((i for i, line in enumerate(lines) if RELEASE_HEADING.match(line) or LINK_REF.match(line)), len(lines))
    lines[at:at] = section.markdown().splitlines() + [""]
    at = next((i for i, line in enumerate(lines) if LINK_REF.match(line)), len(lines))
    lines[at:at] = [link]
    out = "\n".join(lines) + "\n"
    parse(out)
    return out


def shipped(path: str) -> bool:
    if path.endswith("_test.go") or "/testdata/" in path:
        return False
    return any(path == p or (p.endswith("/") and path.startswith(p)) for p in SHIPPED)


def needs_entry(paths: list[str], written: list[str]) -> list[str]:
    """The shipped paths among paths when written (the paths the change adds or modifies) holds no entry file, else
    none: a change that ships needs an entry."""
    return [] if any(p.startswith(f"{ENTRIES}/") for p in written) else [p for p in paths if shipped(p)]


def git(*args: str) -> str:
    return subprocess.run(["git", "-C", str(ROOT), *args], check=True, capture_output=True, text=True).stdout


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(prog="changelog.py", description=__doc__.split("\n\n")[0])
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check", help="check CHANGELOG.md's format and each entry file's")
    n = sub.add_parser("notes", help="print release X's notes")
    n.add_argument("version")
    r = sub.add_parser("pr", help="fail when the changes since BASE touch a shipped path but add no entry file")
    r.add_argument("base")
    w = sub.add_parser("release", help="write release X's section from the entry files and delete them")
    w.add_argument("version")
    w.add_argument("--summary", required=True, help="the section's summary paragraph")
    w.add_argument("--date", default=datetime.datetime.now(datetime.timezone.utc).date().isoformat(),
                   help="the release's date, YYYY-MM-DD (default: today, UTC)")
    a = p.parse_args(argv)
    try:
        text = CHANGELOG.read_text()
        log = parse(text)
        entries = read_entries(ROOT)
        if a.cmd == "check":
            print(f"CHANGELOG.md: ok, {len(log.sections)} releases; {ENTRIES}/: ok, {len(entries)} entries")
        elif a.cmd == "notes":
            print(notes(log, entries, a.version))
        elif a.cmd == "release":
            CHANGELOG.write_text(release(text, entries, a.version, a.date, a.summary))
            for entry in entries:
                (ROOT / entry.where).unlink()
            print(f"CHANGELOG.md: [{a.version}] written from {len(entries)} entries, which are deleted; edit its "
                  "summary and upgrade guide, then commit it with the deletions")
        else:
            base = git("merge-base", a.base, "HEAD").strip()
            paths = git("diff", "--name-only", "--no-renames", f"{base}..HEAD").split()  # a rename: both names
            written = git("diff", "--name-only", "--no-renames", "--diff-filter=AM", f"{base}..HEAD").split()
            if missing := needs_entry(paths, written):
                raise ChangelogError(
                    "this change touches what a release ships (" + ", ".join(missing[:5]) +
                    (", …" if len(missing) > 5 else "") + f") but adds no entry file: add {ENTRIES}/<slug>.md for "
                    "pm's users (CHANGELOG.md's preamble), or label the PR no-changelog if they would not notice "
                    "the change")
            print(f"{ENTRIES}/: this change has its entry")
    except ChangelogError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
