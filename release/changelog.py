#!/usr/bin/env python3
"""Check CHANGELOG.md and print a release's notes from it (AGENTS.md, Releasing pm). Standard library only.

  changelog.py check [--require-unreleased]   the format CHANGELOG.md's preamble states; fails naming the line
  changelog.py notes X                        the body of release X; for X-rc.N, X's section, else [Unreleased]
  changelog.py pr BASE                        fails when the changes since BASE touch a shipped path and leave
                                              [Unreleased] as it is at BASE

Every command fails hard (exit 1, the reason on stderr) on a changelog that does not pass check.
"""

from __future__ import annotations

import argparse
import datetime
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

REPO = "https://github.com/Yeeef/pm"
ROOT = Path(__file__).resolve().parents[1]
CHANGELOG = ROOT / "CHANGELOG.md"
GUIDE = "Upgrade guide"  # first in every release: the numbered steps from the previous release to this one
CATEGORIES = [GUIDE, "Breaking changes", "Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"]
STEP = re.compile(r"^(\d+)\. \S")
# What a Go release builds or serves: the binary's sources and embedded files, its build and its install script.
# Go tests and their data ship nothing; Python pm (src/pm/*.py) serves only Python pins, which have no changelog.
SHIPPED = ("cmd/", "internal/", "assets.go", "prime.md", "src/pm/style.css", "src/pm/prompts/", "go.mod", "go.sum",
           "install.sh", "release/build.sh")

VERSION = r"(\d+)\.(\d+)\.(\d+)"
RELEASE_HEADING = re.compile(rf"^## \[({VERSION})\] - (\d{{4}}-\d{{2}}-\d{{2}})$")
LINK_REF = re.compile(r"^\[([^\]]+)\]: (\S+)$")


class ChangelogError(Exception):
    pass


@dataclass
class Section:
    name: str  # "Unreleased" or the version
    line: int  # the heading's line number, from 1
    date: str = ""
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
    sections: list[Section]  # [Unreleased] first, then releases, newest first
    links: dict[str, str]

    def release(self, version: str) -> Section | None:
        return next((s for s in self.sections[1:] if s.name == version), None)


def fail(line: int, message: str):
    raise ChangelogError(f"CHANGELOG.md:{line}: {message}")


def parse(text: str) -> Changelog:
    """Parse and check the whole changelog: the rules CHANGELOG.md's preamble states."""
    sections: list[Section] = []
    links: dict[str, str] = {}
    category: list[tuple[int, str]] | None = None
    category_name = ""
    title = False
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.rstrip()
        if m := LINK_REF.match(line):
            if m[1] in links:
                fail(n, f"link reference [{m[1]}] is defined twice")
            links[m[1]] = m[2]
            continue
        if line.startswith("#"):
            if line.startswith("# "):
                if title or sections:
                    fail(n, "only the first heading may be a '# ' title")
                title = True
            elif line == "## [Unreleased]":
                if sections:
                    fail(n, "[Unreleased] must be the first '## ' section")
                sections.append(Section("Unreleased", n))
                category = None
            elif line.startswith("## "):
                if not (m := RELEASE_HEADING.match(line)):
                    fail(n, f"a release heading is '## [X.Y.Z] - YYYY-MM-DD', not {line!r}")
                if not sections:
                    fail(n, "'## [Unreleased]' must come before the first release")
                try:
                    datetime.date.fromisoformat(m[5])
                except ValueError:
                    fail(n, f"{m[5]} is not a valid date")
                sections.append(Section(m[1], n, date=m[5]))
                category = None
            elif line.startswith("### "):
                name = line[4:]
                if not sections:
                    fail(n, f"category {name!r} is outside any section")
                section = sections[-1]
                if name not in CATEGORIES:
                    fail(n, f"unknown category {name!r}; the categories are, in this order: {', '.join(CATEGORIES)}")
                if name in section.categories:
                    fail(n, f"category {name!r} appears twice in [{section.name}]")
                if section.categories and CATEGORIES.index(name) < CATEGORIES.index(list(section.categories)[-1]):
                    fail(n, f"category {name!r} comes after {list(section.categories)[-1]!r}; the order is "
                            f"{', '.join(CATEGORIES)}")
                category = section.categories[name] = []
                category_name = name
            else:
                fail(n, f"only '#' title, '## ' section and '### ' category headings are allowed, not {line!r}")
            continue
        if not sections:
            continue  # the preamble
        if category is None:
            sections[-1].summary.append(line)
            continue
        started = any(t for _, t in category)
        if category_name == GUIDE:
            if line and not (line.startswith("  ") and started):
                step = sum(1 for _, t in category if STEP.match(t)) + 1
                if not (m := STEP.match(line)) or int(m[1]) != step:
                    fail(n, f"{GUIDE!r} is a numbered list ('1. ', '2. ', …, with indented continuation lines and "
                            f"sub-bullets); step {step} should start '{step}. '")
        elif line and not (line.startswith("- ") or (line.startswith("  ") and started)):
            fail(n, "a category holds only '- ' bullets, with indented continuation lines and sub-bullets")
        category.append((n, line))
    if not sections:
        fail(1, "no '## [Unreleased]' section")
    for section in sections:
        check_section(section)
    check_order(sections)
    check_links(sections, links)
    return Changelog(sections, links)


def check_section(section: Section):
    for name, lines in section.categories.items():
        if not any(line for _, line in lines):
            fail(section.line, f"[{section.name}] has an empty {name!r} category")
    if section.name == "Unreleased":
        return
    if not "\n".join(section.summary).strip():
        fail(section.line, f"[{section.name}] needs a summary paragraph before its first category")
    if GUIDE not in section.categories:
        fail(section.line, f"[{section.name}] needs an '### {GUIDE}' first: the numbered steps that move a repo from "
                           "the previous release to this one")


def check_order(sections: list[Section]):
    for newer, older in zip(sections[1:], sections[2:]):
        if older.key >= newer.key:
            fail(older.line, f"releases go newest first: {older.name} comes after {newer.name}")
        if older.date > newer.date:
            fail(older.line, f"{older.name}'s date {older.date} is after {newer.name}'s {newer.date}")


def check_links(sections: list[Section], links: dict[str, str]):
    releases = sections[1:]
    head = f"pm-v{releases[0].name}" if releases else None
    want = {"Unreleased": f"{REPO}/compare/{head}...HEAD" if head else f"{REPO}/commits/HEAD"}
    for newer, older in zip(releases, releases[1:]):
        want[newer.name] = f"{REPO}/compare/pm-v{older.name}...pm-v{newer.name}"
    for section in sections:
        url = links.get(section.name)
        if url is None:
            fail(section.line, f"[{section.name}] has no link reference; add [{section.name}]: "
                               f"{want.get(section.name, f'{REPO}/compare/pm-v<previous>...pm-v{section.name}')}")
        if section.name in want and url != want[section.name]:
            fail(section.line, f"[{section.name}] links {url}, not {want[section.name]}")
        if section.name not in want and not (url.startswith(f"{REPO}/") and url.endswith(f"pm-v{section.name}")):
            fail(section.line, f"[{section.name}] links {url}, which is not {REPO}'s pm-v{section.name}")


def notes(log: Changelog, version: str) -> str:
    """Release X's body: its section without the heading. A pre-release X-<suffix> takes X's section when there is
    one, else [Unreleased]; either way the body must not be empty."""
    if not (m := re.fullmatch(rf"{VERSION}(-[0-9A-Za-z.-]+)?", version)):
        raise ChangelogError(f"{version!r} is not a version X.Y.Z or X.Y.Z-<pre-release>")
    base = version.split("-", 1)[0]
    section = log.release(base)
    if section is None and m[4]:
        section = log.sections[0]
    if section is None:
        raise ChangelogError(f"CHANGELOG.md has no '## [{base}] - <date>' section; write release {base}'s notes "
                             "there before tagging it (AGENTS.md, Releasing pm)")
    body = section.body()
    if not body:
        raise ChangelogError(f"CHANGELOG.md's [{section.name}] section is empty; pre-release {version} takes its notes "
                             "from it")
    if GUIDE not in section.categories:
        raise ChangelogError(f"CHANGELOG.md's [{section.name}] section has no '### {GUIDE}'; pre-release {version} "
                             "takes its notes from it")
    return body


def shipped(path: str) -> bool:
    if path.endswith("_test.go") or "/testdata/" in path:
        return False
    return any(path == p or (p.endswith("/") and path.startswith(p)) for p in SHIPPED)


def needs_entry(paths: list[str], base_text: str | None, head_text: str) -> list[str]:
    """The shipped paths among paths when head's [Unreleased] section equals base's, else none: a change that ships
    needs an [Unreleased] entry. A base with no changelog, or one that fails check, counts as a changed section."""
    if base_text is None:
        return []  # the change adds the changelog
    head = parse(head_text).sections[0].body()
    try:
        base = parse(base_text).sections[0].body()
    except ChangelogError:
        return []  # base's changelog fails check and head's passes: the change edits it
    return [] if base != head else [p for p in paths if shipped(p)]


def git(*args: str) -> str:
    return subprocess.run(["git", "-C", str(ROOT), *args], check=True, capture_output=True, text=True).stdout


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(prog="changelog.py", description=__doc__.split("\n\n")[0])
    sub = p.add_subparsers(dest="cmd", required=True)
    c = sub.add_parser("check", help="check CHANGELOG.md's format")
    c.add_argument("--require-unreleased", action="store_true", help="also fail when [Unreleased] is empty")
    n = sub.add_parser("notes", help="print release X's notes")
    n.add_argument("version")
    r = sub.add_parser("pr", help="fail when the changes since BASE touch a shipped path but not [Unreleased]")
    r.add_argument("base")
    a = p.parse_args(argv)
    try:
        log = parse(CHANGELOG.read_text())
        if a.cmd == "check":
            if a.require_unreleased and not log.sections[0].body():
                raise ChangelogError("CHANGELOG.md's [Unreleased] section is empty")
            print(f"CHANGELOG.md: ok, {len(log.sections) - 1} releases")
        elif a.cmd == "notes":
            print(notes(log, a.version))
        else:
            base = git("merge-base", a.base, "HEAD").strip()
            paths = git("diff", "--name-only", f"{base}..HEAD").split()
            try:
                base_text = git("show", f"{base}:CHANGELOG.md")
            except subprocess.CalledProcessError:
                base_text = None
            if missing := needs_entry(paths, base_text, CHANGELOG.read_text()):
                raise ChangelogError(
                    "this change touches what a release ships (" + ", ".join(missing[:5]) +
                    (", …" if len(missing) > 5 else "") + ") but not CHANGELOG.md's [Unreleased] section: add an "
                    "entry there for pm's users, or label the PR no-changelog if they would not notice the change")
            print("CHANGELOG.md: [Unreleased] is up to date with this change")
    except ChangelogError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
