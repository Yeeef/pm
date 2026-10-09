"""Write the corpus the Go parity tests compare against: Python pm's pages, check results and the results of the
functions Go pm replaces with its own code, all from the same records and the same work data.

    uv run --project pm python pm/tests/go_parity_corpus.py OUT [--live]

Each corpus is a directory under OUT: `records` (the store, a git repo, so design pages have dates), `items.json` (the
work-store items, mapped from the bd issues by work_items.py, in bd's order), `meta.json` (site name), and Python's
`pages.json` (every page by path) or `check.json` (pm check's result: pages or the error). Corpora:

- `fixtures`: the test suite's records and issues (conftest.RECORDS, ISSUES).
- `constructs`: pm/internal/site/testdata/constructs, a fixture for each construct the other corpora lack.
- `check-<case>`: a constructs copy with one record or item broken; pm check's error.
- `live` (with --live): this clone's records store and its Beads data, read with bd; needs bd and the store.

OUT/reference.json holds Python's results for the scanning code that replaces its lookaround regexes, YAML 1.1 quoting
and header typing, on a table of inputs plus every header value of every corpus. The Go tests (internal/site,
internal/records) read OUT through $PM_PARITY.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

import yaml

from pm import cli, records, site
from pm.records import RecordError, read_records, read_summaries
from pm.store import design_dates

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conftest import ISSUES, RECORDS  # noqa: E402
from work_items import items as work_items  # noqa: E402

HERE = Path(__file__).resolve().parent
CONSTRUCTS = HERE.parent / "internal/site/testdata/constructs"
GIT_ENV = {"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t",
           "GIT_COMMITTER_EMAIL": "t@example.com", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null"}


def git(cwd: Path, *args: str) -> str:
    import os
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
                          env={**os.environ, **GIT_ENV}).stdout


def store_of(files: dict[str, str], at: Path) -> Path:
    """A records store at `at`: the files committed on branch records."""
    at.mkdir(parents=True)
    git(at, "init", "-q", "-b", "records")
    for rel, text in files.items():
        (at / rel).parent.mkdir(parents=True, exist_ok=True)
        (at / rel).write_text(text)
    git(at, "add", "-A")
    git(at, "commit", "-qm", "records")
    return at


def beads_of(issues: list[dict]) -> dict[str, dict]:
    """bd issues by id, each with its comments oldest first, as Python pm's site reads them."""
    out = {}
    for i in issues:
        i = dict(i)
        if i.get("comments"):
            i["comments"] = sorted(i["comments"], key=lambda c: c.get("created_at") or "")
        out[i["id"]] = i
    return out


def pages(store: Path, beads: dict[str, dict], name: str) -> dict:
    try:
        recs = read_records(store)
        return {"pages": site.render_pages(recs, beads, name, dates=design_dates(store, recs),
                                           summaries=read_summaries(store))}
    except RecordError as e:
        return {"error": str(e)}


def check(store: Path, beads: dict[str, dict], name: str) -> dict:
    try:
        return {"pages": cli.check_records(read_records(store), beads, name, read_summaries(store))}
    except RecordError as e:
        return {"error": str(e)}


def write(out: Path, corpus: str, store: Path, issues: list[dict], name: str, result: dict, kind: str) -> None:
    d = out / corpus
    d.mkdir(parents=True, exist_ok=True)
    if store != d / "records":
        (d / "records").symlink_to(store)
    (d / "items.json").write_text(json.dumps(list(work_items(list(beads_of(issues).values())).values())))
    (d / "meta.json").write_text(json.dumps({"site_name": name}))
    (d / f"{kind}.json").write_text(json.dumps(result))


def corpus(out: Path, name: str, files: dict[str, str], issues: list[dict]) -> Path:
    store = store_of(files, out / name / "records")
    write(out, name, store, issues, "demo", pages(store, beads_of(issues), "demo"), "pages")
    return store


def files_under(root: Path) -> dict[str, str]:
    return {p.relative_to(root).as_posix(): p.read_text() for p in sorted(root.rglob("*")) if p.is_file()}


# pm check on a constructs copy with one thing broken: (case, record edits {path: text or None}, issue edits).
def broken() -> list[tuple[str, dict[str, str | None], list[dict]]]:
    sprint = (CONSTRUCTS / "records/sprints/demo-1.md").read_text()
    design = (CONSTRUCTS / "records/design/parser.md").read_text()
    return [
        ("no-header", {"sprints/demo-1.md": sprint.split("---\n", 2)[2]}, []),
        ("unknown-type", {"sprints/demo-1.md": sprint.replace("type: sprint", "type: epic", 1)}, []),
        ("typed-type", {"sprints/demo-1.md": sprint.replace("type: sprint", "type: 5", 1)}, []),
        ("missing-field", {"sprints/demo-1.md": sprint.replace("title: First\n", "", 1)}, []),
        ("false-title", {"sprints/demo-1.md": sprint.replace("title: First", "title: no", 1)}, []),
        ("unknown-block", {"sprints/demo-1.md": sprint + "\n::: warning\nx\n:::\n"}, []),
        ("block-attr", {"sprints/demo-1.md": sprint + "\n::: result\nx\n:::\n"}, []),
        ("unknown-bead", {"sprints/demo-1.md": sprint.replace("bead: demo.1", "bead: demo.99", 1)}, []),
        ("hand-progress", {"sprints/demo-1.md": sprint.replace("> Do not write here.\n", "> Do not write here.\n\nMine.\n", 1)}, []),
        ("generated-heading", {"sprints/demo-1.md": sprint + "\n## Docs\n\nx\n"}, []),
        ("missing-section", {"design/parser.md": design.replace("## Open questions", "## Questions", 1)}, []),
        ("bad-outcome", {"sprints/demo-1.md": sprint.replace("Not closed yet.\n\n### Against", "Maybe.\n\n### Against", 1)}, []),
        ("no-outcome-part", {"sprints/demo-1.md": sprint.replace("### Outcome", "### Result", 1)}, []),
        ("doc-two-keys", {"docs/2026-10-02-notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10-02\nbead: demo.1\n"
                                                      "project: demo\n---\n\nx\n"}, []),
        ("doc-bad-date", {"docs/2026-10-02-notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10\nproject: demo\n"
                                                      "---\n\nx\n"}, []),
        ("doc-bad-path", {"docs/notes.md": "---\ntype: doc\ntitle: Notes\ndate: 2026-10-02\nproject: demo\n---\n\nx\n"}, []),
        ("postmortem-no-sprint", {"postmortems/2026-10-03-outage.md": None,
                                  "postmortems/2026-10-03-lost.md": "---\ntype: postmortem\ntitle: Lost\ndate: 2026-10-03\n"
                                                                    "sprint: demo.42\n---\n\nx\n"}, []),
        ("no-project", {"design/parser.md": design.replace("project: demo", "project: nowhere", 1)}, []),
        ("uncited-need", {}, [{"id": "demo.1.9", "title": "Pick a parser", "status": "closed", "issue_type": "task",
                               "parent": "demo.1", "labels": ["human"], "close_reason": "Responded: a",
                               "created_at": "2026-10-01T12:00:00Z", "closed_at": "2026-10-01T13:00:00Z"}]),
        ("bad-summary", {"days/2026-10-01.summary.json": json.dumps({"date": "2026-10-02", "generated_at": "x",
                                                                     "digest": "d", "text": "t"})}, []),
        ("summary-not-object", {"days/2026-10-01.summary.json": "[1]"}, []),
        ("summary-bad-time", {"days/2026-10-01.summary.json": json.dumps({"date": "2026-10-01", "generated_at": "noon",
                                                                          "digest": "d", "text": "t"})}, []),
    ]


def live(out: Path) -> None:
    # A clone of the records branch as committed now: other sessions write the store while the Go test runs, so both
    # sides read this snapshot, its history included for the design pages' dates.
    source = subprocess.run(["pm", "where", "records"], check=True, capture_output=True, text=True).stdout.strip()
    store = out / "live" / "records"
    store.parent.mkdir(parents=True)
    git(out, "clone", "-q", "--branch", "records", "--single-branch", source, str(store))
    listed = json.loads(subprocess.run(["bd", "list", "--all", "--json"], check=True, capture_output=True,
                                       text=True).stdout)
    exported = {r["id"]: r for r in map(json.loads, subprocess.run(["bd", "export"], check=True, capture_output=True,
                                                                    text=True).stdout.splitlines())}
    for i in listed:
        if exported.get(i["id"], {}).get("comments"):
            i["comments"] = exported[i["id"]]["comments"]
        # bd list gives an issue's blockers in no defined order (neither by id nor by when each was added, and not
        # the export's); the work store keeps them by id, so Python renders them in that order too
        i["dependencies"] = sorted(i.get("dependencies") or [], key=lambda d: (d["type"], d["depends_on_id"]))
    beads = beads_of(listed)
    name = "yeeef-agents"
    write(out, "live", store, listed, name, pages(store, beads, name), "pages")
    (out / "live" / "check.json").write_text(json.dumps(check(store, beads, name)))


# Inputs for the functions Go pm writes as scanning code (Python's lookaround regexes) and for YAML 1.1.
SECTION_BODIES = [
    "", "## Goal\n", "## Goal\nx\n", "## Goal\n> p\n\ntext\n\n## Scope\ny\n", "# T\n## Goal\na\n### Sub\nb\n## Next\nc",
    "## Goals\nno\n## Goal\nyes\n", "x## Goal\nno\n## Goal\n\n> p\n> q\nbody\n", "## Goal\n## Scope\n", "## Goal",
    "## Goal\r\nx\n", "## Goal\n  ## not a heading\n##no\n## Scope\n", "### Outcome\ndone, it works.\n### Against\nok",
    "## Delivery report\n\n### Outcome\n\n> p\n\nDone: shipped.\n- a\n\n### Against \"Done when\"\n\n- ok\n",
]
YAML_STRINGS = [
    "", "plain", "yes", "No", "on", "OFF", "y", "n", "true", "null", "~", "Null", "1", "-1", "+1", "0", "010", "08",
    "0x1F", "0b101", "1_000", "1:20", "1.5", "1e3", "1.0e+3", ".5", ".inf", "-.Inf", ".nan", "2026-10-08",
    "2026-1-8 10:00:00", "2026-10-08T10:00:00Z", "2026-13-45", "Sprint 1: x", "a: b", "a:b", "a #b", "a#b", "#a",
    "- a", "-a", "? a", "?a", ":a", "[a]", "{a}", "a, b", "a]", "'a'", '"a"', "a'b", 'a"b', "&a", "*a", "!a", "|a",
    ">a", "%a", "@a", "`a`", " a", "a ", "a\tb", "a\nb", "Ünïcødé", "日本語", "a · b", "x\x01", "=", "<<", "pm-v0.1.0",
    "Go port: records and site (P4)", "3.14", "1.2.3", "12:30", "12:30:00",
]
CITES = [("see demo.1.2 here", "demo.1.2"), ("demo.1.2", "demo.1.2"), ("demo.1.23", "demo.1.2"),
         ("xdemo.1.2", "demo.1.2"), ("demo.1.2.3", "demo.1.2"), ("demo.1.2.", "demo.1.2"), ("(demo.1.2)", "demo.1.2"),
         ("a-demo.1.2", "demo.1.2"), ("demo.1.2-x", "demo.1.2"), ("demo.1.2 demo.1.2x", "demo.1.2"),
         (".demo.1.2", "demo.1.2"), ("éxdemo.1.2", "demo.1.2"), ("demo.1.2é", "demo.1.2"), ("demo.1.2.é", "demo.1.2"),
         ("demo.1.2,", "demo.1.2"), ("", "demo.1.2")]


def header_values(stores: list[Path]) -> list[str]:
    """Every header value of every record in the stores, as text."""
    out = set()
    for store in stores:
        for p in sorted(store.rglob("*.md")):
            m = records.HEADER_RE.match(p.read_text())
            for line in (m.group(1).splitlines() if m else []):
                if ": " in line:
                    out.add(line.split(": ", 1)[1])
    return sorted(out)


def header_type(value: str) -> list:
    """Python's type name, str() and truth of a header value as pyyaml reads `k: <value>`, or None on an error."""
    try:
        v = yaml.safe_load(f"k: {value}")["k"]
    except Exception:
        return None
    return [type(v).__name__, str(v), bool(v)]


def yaml_str(value: str) -> str | None:
    """Python pm's yaml_str, or None where it raises (pyyaml constructs an invalid date before comparing)."""
    try:
        return cli.yaml_str(value)
    except ValueError:
        return None


def reference(out: Path, stores: list[Path]) -> None:
    values = sorted(set(YAML_STRINGS) | set(header_values(stores)))
    from pm.records import first_para, section_text, subsection_text, summary_line
    rec = lambda body: records.Record(Path("x.md"), "sprints/x", {"type": "sprint"}, body)
    outcome = []
    for body in SECTION_BODIES:
        try:
            outcome.append([body, records.outcome(rec(body)), None])
        except RecordError as e:
            outcome.append([body, None, str(e)])
    (out / "reference.json").write_text(json.dumps({
        "yaml_str": [[s, yaml_str(s)] for s in values],
        "header_type": [[s, header_type(s)] for s in values],
        "section_text": [[b, n, section_text(b, n)] for b in SECTION_BODIES for n in ("Goal", "Scope", "Delivery report")],
        "subsection_text": [[b, n, subsection_text(b, n)] for b in SECTION_BODIES for n in ("Sub", "Outcome")],
        "outcome": outcome,
        "first_para": [[b, first_para(b)] for b in SECTION_BODIES],
        "summary_line": [[t, summary_line(t)] for t in ("", "One line.", "- a\n- b\n", "* a  \n\n  - b\nc", "x\n -  y ")],
        "cites": [[t, i, site.cites(t, i)] for t, i in CITES],
        "templates": {
            "sprint": cli.sprint_text("Go port", "demo.9", {"Goal": "G.", "Scope": "**In:** a.", "Done when": "- d."}),
            "project": cli.project_text("Demo", "demo", "Goal."),
            "design": cli.design_text("The parser", "demo"),
            "postmortem": cli.postmortem_text("Outage", "2026-10-03", "sprint: demo.1"),
        },
        "insert_entry": [[t, sec, e, insert(t, sec, e)] for t, sec, e in INSERTS],
    }, ensure_ascii=False))


INSERTS = [("## Findings\n\n> p\n\nNone yet.\n\n## Next\n", "Findings", "- a"),
           ("## Findings\n\n> p\n\n- old\n\n## Next\n", "Findings", "- a\n- b\n"),
           ("## Findings\n> p\n## Next\n", "Findings", "x"), ("## Findings", "Findings", "x"),
           ("## Other\n", "Findings", "x"), ("## Findings\n\nNone yet.\nmore\n", "Findings", "x")]


def insert(text: str, section: str, entry: str) -> str | None:
    try:
        return records.insert_entry(text, section, entry)
    except RecordError as e:
        return "error: " + str(e)


def main(argv: list[str]) -> int:
    out = Path(argv[0]).resolve()
    shutil.rmtree(out, ignore_errors=True)
    out.mkdir(parents=True)
    stores = [corpus(out, "fixtures", RECORDS, ISSUES)]
    issues = json.loads((CONSTRUCTS / "issues.json").read_text())
    files = files_under(CONSTRUCTS / "records")
    stores.append(corpus(out, "constructs", files, issues))
    for case, edits, extra in broken():
        changed = {k: v for k, v in {**files, **edits}.items() if v is not None}
        store = store_of(changed, out / f"check-{case}" / "records")
        write(out, f"check-{case}", store, issues + extra, "demo", check(store, beads_of(issues + extra), "demo"),
              "check")
    if "--live" in argv[1:]:
        live(out)
        stores.append(out / "live" / "records")
    reference(out, stores)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
