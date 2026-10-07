"""pm commands against a temp repo and a fake bd: each refusal changes nothing; happy paths write what they say."""

from __future__ import annotations

import contextlib
import json
import os
import re
import shlex
import shutil
import socket
import subprocess
import sys
import time
import tomllib
import urllib.request
from pathlib import Path

import pytest

from conftest import write_config, HARNESS, REAL_RECORDS, PM, RENDER, fake_bd_env, project, sprint

sys.path.insert(0, str(HARNESS))
from pm.beads import REPLY_MARK, reply_body, reply_in_beads  # noqa: E402
from pm.site import thread  # noqa: E402

FRAME = "## Goal\n\nShip the thing.\n\n## Scope\n\n**In:** the thing.\n\n**Out:** other things.\n\n## Done when\n\n- It ships.\n"


def committed(repo, messages):
    """The latest store commits carry `messages`, newest first, and nothing is left uncommitted anywhere."""
    return (repo.store_log()[:len(messages)] == messages and repo.git("status", "--porcelain", cwd=repo.store) == ""
            and repo.git("status", "--porcelain") == "")


def refused(repo, *args, stdin="", match):
    """Run a command that must refuse: non-zero exit, the message, no file and no Beads change."""
    heads = lambda: [repo.git("rev-parse", "HEAD", cwd=d) for d in (repo.root, repo.store)]
    before, head = repo.snapshot(), heads()
    res = repo.pm(*args, stdin=stdin)
    assert res.returncode != 0, res.stdout
    assert re.search(match, res.stderr), res.stderr
    assert repo.snapshot() == before
    assert heads() == head
    assert repo.bd_writes() == []
    return res


# ---------------------------------------------------------------- refusals

def test_show_refuses_invalid_records(repo):
    repo.write("sprints/demo-1.md", (repo.records / "sprints/demo-1.md").read_text().replace("## Findings", "## Finds"))
    refused(repo, "show", "--sprint", "demo.1", match=r"error: sprints/demo-1: sprint record needs a '## Findings' section")


def test_finding_add_refuses_missing_sprint(repo):
    refused(repo, "finding", "add", "A finding.", match=r"the following arguments are required: --sprint")


def test_finding_add_refuses_unknown_sprint(repo):
    refused(repo, "finding", "add", "--sprint", "demo.9", "A finding.", match=r"error: no sprint record has bead demo.9")


def test_finding_add_refuses_closed_sprint(repo):
    repo.set_issue("demo.2", status="closed")
    refused(repo, "finding", "add", "--sprint", "demo.2", "A finding.", match=r"error: sprint demo.2 is closed")


@pytest.mark.parametrize("frame, match", [
    (FRAME.replace("## Goal\n\nShip the thing.\n\n", ""), r"'## Goal' is missing or empty"),
    (FRAME.replace("Ship the thing.", ""), r"'## Goal' is missing or empty"),
    (FRAME.replace("\n\n**Out:** other things.", ""), r"Scope needs a non-empty \*\*In:\*\* list followed by a non-empty \*\*Out:\*\*"),
    (FRAME.replace("**In:** the thing.", "**In:**"), r"Scope needs a non-empty \*\*In:\*\*"),
    (FRAME.replace("- It ships.", ""), r"'## Done when' is missing or empty"),
    (FRAME + "\n## Notes\n\nx\n", r"unknown section '## Notes'"),
])
def test_sprint_open_refuses_incomplete_frame(repo, frame, match):
    refused(repo, "sprint", "open", "demo", "--title", "Third", stdin=frame, match=r"error: " + match)


def test_sprint_open_refuses_closed_project(repo):
    refused(repo, "sprint", "open", "old", "--title", "Late", stdin=FRAME, match=r"error: project old \(old\) is closed")


def test_sprint_open_refuses_unknown_project(repo):
    refused(repo, "sprint", "open", "nope", "--title", "X", stdin=FRAME, match=r"error: no project record named 'nope'")


PR = "https://github.com/o/r/pull/12"


def report(repo, outcome, against="- It ships: met, see the log.", name="demo-1"):
    """Write a sprint's delivery report over whatever it holds and commit it."""
    text = (repo.records / f"sprints/{name}.md").read_text()
    head, _, _ = text.partition("### Outcome\n")
    text = (head + "### Outcome\n\n> Done, partial or voided, plus one sentence; then, optionally, bullets of what "
            f"shipped.\n\n{outcome}\n\n### Against \"Done when\"\n\n> Each item, met or not, with its evidence.\n\n"
            f"{against}\n")
    repo.write(f"sprints/{name}.md", text)
    repo.commit("delivery report")


SHA = "8f5c6180a1b2c3d4e5f60718293a4b5c6d7e8f90"


def add_review(repo, sprints=("demo.1",), status="closed", pr=PR, issue_id="demo.1.3", reason=f"merged as {SHA}"):
    """A PR review raised for `sprints`, as pm action need --pr stores it; a closed one closed with `reason`."""
    issues = json.loads(repo.state.read_text()) + [
        {"id": issue_id, "title": "Review PR #12", "status": status, "issue_type": "task", "parent": sprints[0],
         "labels": ["human", "action"], "created_at": "2026-10-01T12:00:00Z",
         "metadata": {"review": {"pr": pr, "sprints": list(sprints), "focus": "F", "designs": []}},
         **({"close_reason": reason} if status == "closed" else {})}]
    repo.state.write_text(json.dumps(issues))


def close_ready(repo, outcome="Done: shipped the thing.", against="- It ships: met, see the log.", review=True):
    """Write sprint 1's delivery report, close its open task and, with `review`, add its PR review closed as merged;
    commit."""
    report(repo, outcome, against)
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    if review:
        add_review(repo)


def store_head(repo):
    return repo.git("rev-parse", "--short", "HEAD", cwd=repo.store).strip()


def test_sprint_close_refuses_its_uncommitted_record(repo):
    close_ready(repo)
    rec = repo.records / "sprints/demo-1.md"
    repo.write("sprints/demo-1.md", rec.read_text() + "\nAn edit in progress.\n")
    refused(repo, "sprint", "close", "demo.1", match=r"error: sprints/demo-1.md has uncommitted changes")


def test_sprint_close_ignores_an_unrelated_uncommitted_file(repo):
    """Another session's edit in progress elsewhere in the store does not block the close."""
    close_ready(repo)
    repo.write("days/2026-10-02.md", "---\ntype: day\ndate: 2026-10-02\n---\n\n## Today\n\nx\n")
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Done: shipped the thing. (records commit {store_head(repo)})"]]
    assert repo.git("status", "--porcelain", cwd=repo.store) == "?? days/2026-10-02.md\n"


def test_sprint_close_refuses_unwritten_outcome(repo):
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    refused(repo, "sprint", "close", "demo.1", match=r"error: records/sprints/demo-1.md: the committed Delivery report Outcome is still 'Not closed yet.'")


def test_sprint_close_refuses_unwritten_against_done_when(repo):
    close_ready(repo, against="Not closed yet.")
    refused(repo, "sprint", "close", "demo.1", match=r"the committed Delivery report 'Against \"Done when\"' is still")


def test_sprint_close_refuses_empty_against_done_when(repo):
    close_ready(repo, against="")
    refused(repo, "sprint", "close", "demo.1", match=r"the committed Delivery report 'Against \"Done when\"' is still")


def test_sprint_close_refuses_invalid_outcome(repo):
    close_ready(repo, outcome="Shipped it.")
    refused(repo, "sprint", "close", "demo.1", match=r"error: sprints/demo-1: Delivery report Outcome must start with done, partial or voided")


def test_sprint_close_refuses_open_task(repo):
    close_ready(repo)
    repo.set_issue("demo.1.2", status="in_progress")
    refused(repo, "sprint", "close", "demo.1", match=r"error: open tasks in the sprint: demo.1.2 \(in_progress\)")


def test_project_open_refuses_existing_name(repo):
    refused(repo, "project", "open", "demo", "--title", "Again", stdin="A goal.", match=r"error: project 'demo' already exists")


def test_project_open_refuses_empty_goal(repo):
    refused(repo, "project", "open", "fresh", "--title", "Fresh", stdin="", match=r"error: Goal is empty")


def test_project_close_refuses_open_sprint(repo):
    text = (repo.records / "projects/demo.md").read_text().replace("Not closed yet.", "Done: all shipped.")
    repo.write("projects/demo.md", text)
    repo.commit()
    refused(repo, "project", "close", "demo", match=r"error: open sprints in the project: demo.1 \(Sprint 1: First\), demo.2")


def test_project_close_refuses_its_uncommitted_record(repo):
    text = (repo.records / "projects/demo.md").read_text().replace("Not closed yet.", "Done: all shipped.")
    repo.write("projects/demo.md", text)
    refused(repo, "project", "close", "demo", match=r"error: projects/demo.md has uncommitted changes")


def test_project_close_refuses_unwritten_outcome(repo):
    refused(repo, "project", "close", "demo", match=r"the committed ## Outcome is still 'Not closed yet.'")


# ---------------------------------------------------------------- happy paths

def test_finding_add_replaces_placeholder_then_appends(repo):
    path = repo.records / "sprints/demo-1.md"
    before = path.read_text()
    res = repo.pm("finding", "add", "--sprint", "demo.1", "Arm B loss 0.037 vs 0.041.")
    assert res.returncode == 0, res.stderr
    assert path.read_text() == before.replace("results with their numbers.\n\nNone yet.",
                                              "results with their numbers.\n\n- Arm B loss 0.037 vs 0.041.")
    long = "word " * 30
    assert repo.pm("finding", "add", "--sprint", "demo.1", long).returncode == 0
    findings = path.read_text().split("## Findings")[1].split("## Delivery report")[0]
    assert findings.strip().splitlines()[-4:] == [
        "- Arm B loss 0.037 vs 0.041.", "",
        "- " + " ".join(["word"] * 15), "  " + " ".join(["word"] * 15)]
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"] * 2)
    assert repo.bd_writes() == []


def test_finding_add_ignores_unicode_line_separators(repo):
    path = repo.records / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("Ship it.", "form\x0cfeed and \u2028 sep"))
    repo.commit()
    assert repo.pm("finding", "add", "--sprint", "demo.1", "New finding.").returncode == 0
    assert "results with their numbers.\n\n- New finding.\n\n## Delivery report" in path.read_text()


def test_sprint_open_title_with_emoji_renders(repo):
    res = repo.pm("sprint", "open", "demo", "--title", "Ship \U0001F680: now", stdin=FRAME)
    assert res.returncode == 0, res.stderr
    assert 'title: "Ship \U0001F680: now"' in (repo.records / "sprints/demo-3.md").read_text()
    assert repo.pm("render").returncode == 0


def test_sprint_open_creates_epic_and_record(repo):
    res = repo.pm("sprint", "open", "demo", "--title", "Third: the end", stdin=FRAME)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=epic", "--parent=demo", "--title=Sprint 3: Third: the end", "--json"]]
    assert repo.issues()["demo.3"]["parent"] == "demo"
    # After the create it reads only the new epic, not every issue again.
    assert [c for c in repo.bd_calls() if c[:1] in (["list"], ["show"])] == [["list", "--all", "--json"], ["show", "demo.3", "--json"]]
    text = (repo.records / "sprints/demo-3.md").read_text()
    assert text.startswith('---\ntype: sprint\ntitle: "Third: the end"\nbead: demo.3\n---\n')
    assert "Ship the thing." in text and "**Out:** other things." in text and "- It ships." in text
    # Every section and prompt line matches an existing sprint record (the fixture mirrors the real ones).
    skeleton = lambda t: [l for l in t.splitlines() if l.startswith(("#", ">"))]
    assert skeleton(text) == skeleton((repo.records / "sprints/demo-1.md").read_text())
    assert text.count("None yet.") == 3 and text.count("\n\nNot closed yet.\n") == 2


def test_sprint_open_matches_real_sprint_records():
    """The generated skeleton equals the one in this repo's own sprint records."""
    import sys
    sys.path.insert(0, str(HARNESS))
    from pm import cli as pm
    store = REAL_RECORDS.resolve()
    skeleton = lambda t: [l for l in t.splitlines() if l.startswith(("#", ">"))]
    frame = {"Goal": "g", "Scope": "**In:** a\n\n**Out:** b", "Done when": "- d"}
    ours = skeleton(pm.sprint_text("t", "x.1", frame))
    real = list((store / "sprints").glob("*.md"))
    assert real and all(skeleton(p.read_text()) == ours for p in real)


def test_sprint_open_prints_undo_when_write_fails(repo):
    sprints = repo.records / "sprints"
    sprints.chmod(0o555)
    try:
        res = repo.pm("sprint", "open", "demo", "--title", "Third", stdin=FRAME)
    finally:
        sprints.chmod(0o755)
    assert res.returncode != 0
    assert "undo the Beads step with: bd delete demo.3 --force" in res.stderr
    assert not (sprints / "demo-3.md").exists()


def test_sprint_close_stamps_the_merge_commits_and_names_the_stamp_commit(repo):
    """Each PR review naming the sprint, closed as merged, is stamped into the Outcome after the verdict and committed
    on the records branch; the epic's close reason names that commit."""
    close_ready(repo)
    add_review(repo, sprints=("demo.2", "demo.1"), pr="https://github.com/o/r/pull/13", issue_id="demo.2.1",
               reason="merged as 1234567abcdef")
    before = (repo.records / "sprints/demo-1.md").read_text()
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["[SPRINT] demo sprint 1: closed, merged as 8f5c618, 1234567"])
    stamp = "Merged as 8f5c618 (PR #12). Merged as 1234567 (PR #13)."
    assert (repo.records / "sprints/demo-1.md").read_text() == before.replace(
        "Done: shipped the thing.\n", f"Done: shipped the thing.\n\n{stamp}\n")
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Done: shipped the thing. (records commit {store_head(repo)})"]]
    assert repo.pm("render").returncode == 0


def test_sprint_close_without_a_review_closes_with_no_stamp(repo):
    """A sprint with no PR (or voided) closes as before: no stamp, no records commit, the close names HEAD."""
    close_ready(repo, outcome="Voided: the parser was not needed.", review=False)
    head, log = store_head(repo), repo.store_log()
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert repo.store_log() == log
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Voided: the parser was not needed. (records commit {head})"]]


def test_sprint_close_keeps_changelog_bullets_out_of_the_close_reason(repo):
    """An Outcome may follow its verdict sentence with bullets of what shipped; only the verdict closes the epic, and
    the stamp goes between the two."""
    close_ready(repo, outcome="Done: shipped the thing.\n\n- `pm thing` added\n- the old flag removed")
    assert repo.pm("render").returncode == 0
    assert "<li><code>pm thing</code> added</li>" in (repo.root / "site/sprints/demo-1.html").read_text()
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert ("Done: shipped the thing.\n\nMerged as 8f5c618 (PR #12).\n\n- `pm thing` added"
            in (repo.records / "sprints/demo-1.md").read_text())
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Done: shipped the thing. (records commit {store_head(repo)})"]]


@pytest.mark.parametrize("status, reason, match", [
    ("open", None, r"error: open tasks in the sprint: demo.1.3 \(open\); .*pm action done <id> --reason \"merged as <sha>\""),
    ("closed", "PR #12 is on main.", r"error: PR review demo.1.3 \(https://github.com/o/r/pull/12\) is closed as "
                                     r"'PR #12 is on main.', not 'merged as <sha>'"),
])
def test_sprint_close_refuses_a_review_not_closed_as_merged(repo, status, reason, match):
    close_ready(repo, review=False)
    add_review(repo, status=status, reason=reason)
    refused(repo, "sprint", "close", "demo.1", match=match)


def test_sprint_close_skips_a_dismissed_review(repo):
    """A review closed with bd human dismiss (a replaced PR, a [TEST] review) delivers nothing: it neither blocks the
    close nor is stamped."""
    close_ready(repo)
    add_review(repo, pr="https://github.com/o/r/pull/11", issue_id="demo.1.4", reason="Dismissed: replaced by PR #12")
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert "Merged as 8f5c618 (PR #12).\n" in (repo.records / "sprints/demo-1.md").read_text()
    assert "PR #11" not in (repo.records / "sprints/demo-1.md").read_text()


def test_merge_stamp_stays_in_the_outcome_when_a_heading_follows_the_verdict():
    sys.path.insert(0, str(HARNESS))
    from pm import cli as pm
    text = ("## Delivery report\n\n### Outcome\n\n> prompt\n\nDone: it ships.\n### Against \"Done when\"\n\n- met\n")
    assert pm.merge_stamp(text, "Merged as abc1234 (PR #1).") == text.replace(
        "Done: it ships.\n", "Done: it ships.\n\nMerged as abc1234 (PR #1).\n\n")


def test_sprint_close_refuses_an_open_review_of_another_sprint_naming_it(repo):
    """A PR delivering two sprints sits under the first; the second waits for it too."""
    close_ready(repo, review=False)
    add_review(repo, sprints=("demo.2", "demo.1"), status="open", issue_id="demo.2.1")
    refused(repo, "sprint", "close", "demo.1", match=r"error: open tasks in the sprint: demo.2.1 \(open\)")


def test_sprint_close_renders_the_closed_sprint(repo):
    """Close validates the sprint record against the closed state, not only its report: a committed record that
    does not render stops the close."""
    close_ready(repo)
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("## Findings", "## Finds"))
    repo.commit("Break sprint 1 by hand")
    refused(repo, "sprint", "close", "demo.1", match=r"sprints/demo-1: sprint record needs a '## Findings'")


def test_sprint_closes_after_its_pr_merges(repo):
    """The whole loop: a written report, a review under the sprint, a refusal while the review is open, the review
    closed as merged once the PR is on main, then a close that stamps the merge and names its records commit."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    report(repo, "Done: shipped the thing.")
    res = repo.pm("action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "F")
    assert res.returncode == 0, res.stderr
    assert repo.issues()["demo.1.3"]["parent"] == "demo.1" and "under demo.1;" in res.stdout
    repo.log.write_text("")
    refused(repo, "sprint", "close", "demo.1", match=r"open tasks in the sprint: demo.1.3 \(open\)")
    assert repo.pm("action", "done", "demo.1.3", "--reason", f"merged as {SHA}").returncode == 0
    repo.log.write_text("")
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["[SPRINT] demo sprint 1: closed, merged as 8f5c618"])
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Done: shipped the thing. (records commit {store_head(repo)})"]]
    assert repo.pm("render").returncode == 0
    page = (repo.root / "site/sprints/demo-1.html").read_text()
    assert "<p>Done: shipped the thing.</p>\n<p>Merged as 8f5c618 (PR #12).</p>" in page


def test_an_old_report_with_a_draft_until_paragraph_still_renders(repo):
    """Records written under the old flow keep the paragraph as ordinary text, open or closed."""
    report(repo, "Done: shipped the thing.\n\nDraft until its PR merges.")
    repo.set_issue("demo.1", status="closed")
    res = repo.pm("render")
    assert res.returncode == 0, res.stderr
    assert "<p>Draft until its PR merges.</p>" in (repo.root / "site/sprints/demo-1.html").read_text()


def test_project_open_creates_epic_and_record(repo):
    res = repo.pm("project", "open", "fresh", "--title", "Fresh start", stdin="Why we do it.\n")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=epic", "--title=Fresh start", "--json"]]
    assert [c for c in repo.bd_calls() if c[:1] in (["list"], ["show"])] == [["list", "--all", "--json"], ["show", "new-7", "--json"]]
    text = (repo.records / "projects/fresh.md").read_text()
    assert text.startswith("---\ntype: project\ntitle: Fresh start\nbead: new-7\n---\n")
    assert [l for l in text.splitlines() if l.startswith("## ")] == [
        "## Goal", "## Progress", "## Decisions", "## Design pages", "## Outcome"]


def test_show_text_and_json(repo):
    res = repo.pm("show")
    assert res.returncode == 0, res.stderr
    assert "Sprint 1: First  .1  running  1/2 done" in res.stdout
    assert "  ready        .1.2  Ask the owner  [human decision]" in res.stdout
    assert ("decisions await you (1):\n  .1.2  Ask the owner  (sprint 1)  -> bd show demo.1.2\nactions await you (0):"
            in res.stdout)
    out = res.stdout  # what matters most first, as `pm prime` cuts the end: site, needs, then sprints, then decisions
    assert out.index("site: ") < out.index("actions await you") < out.index("Sprint 1: First") < out.index("decisions (last")
    data = json.loads(repo.pm("show", "--json").stdout)
    assert [p["name"] for p in data["projects"]] == ["demo"]
    assert data["projects"][0]["needs"] == [{"id": "demo.1.2", "title": "Ask the owner", "kind": "decision",
                                                  "session": None, "replied": False, "sprint": "demo.1", "task": None}]


FENCED = "- Fast.\n\n```\n## Not a heading\n```\n\n<div><pre>\n## Nor this\n</pre></div>"
FINDINGS = "> What did we learn that changes the design, the plan, or how we work? Add\n> results with their numbers."


@pytest.mark.parametrize("target, section, expected", [
    # a '##' section ends at the next '##'; a '#' line in a code fence or a raw HTML block is not a heading
    ("records/sprints/demo-3.md", "Findings", f"## Findings\n\n{FINDINGS}\n\n{FENCED}"),
    # a '##' section keeps its '###' subsections
    ("demo.3", "Delivery report", "## Delivery report\n\n> Written at close. Each part holds \"Not closed yet.\" "
     "until then.\n\n### Outcome\n\n> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.\n\nNot closed yet.\n\n"
     "### Against \"Done when\"\n\n> Each item, met or not, with its evidence (a page, a command, a number).\n\n"
     "Not closed yet."),
    # a '###' subsection ends at the next '###', and at the end of the file
    ("sprints/demo-3", "Outcome", "### Outcome\n\n> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.\n\nNot closed yet."),
    ("demo.3", 'Against "Done when"', "### Against \"Done when\"\n\n> Each item, met or not, with its evidence "
     "(a page, a command, a number).\n\nNot closed yet."),
])
def test_show_record_section(repo, target, section, expected):
    repo.write("sprints/demo-3.md", sprint("Third", "demo.3", findings=FENCED))
    res = repo.pm("show", "--record", target, "--section", section)
    assert res.returncode == 0, res.stderr
    assert res.stdout == expected + "\n"


@pytest.mark.parametrize("args, match", [
    (["--record", "sprints/demo-1", "--section", "Findngs"],
     r"records/sprints/demo-1.md has no section named 'Findngs'; its sections:\n  Goal\n  Scope\n  Done when\n"
     r"  Design pages\n  Progress\n  Decisions\n  Findings\n  Delivery report\n    Outcome\n"
     r"    Against \"Done when\"\n"),
    (["--record", "records/sprints/demo-9.md", "--section", "Goal"], r"no record matches 'records/sprints/demo-9.md'"),
    (["--section", "Goal"], r"--record and --section go together"),
    (["--record", "demo.1"], r"--record and --section go together"),
    (["--record", "demo.1", "--section", "Goal", "--json"], r"--record and --section go together, without --sprint or --json"),
])
def test_show_record_section_refuses(repo, args, match):
    refused(repo, "show", *args, match=r"error: " + match)


# ---------------------------------------------------------------- pm decision add

BODY = "Use the small parser.\nIt is enough for the record set and adds no dependency.\n"
TODAY = __import__("datetime").date.today().isoformat()


@pytest.mark.parametrize("args, stdin, match", [
    ([], BODY, r"--level is required \(project\|sprint\): project if a later sprint must follow it"),
    (["--level", "project"], BODY, r"--level project needs --project; nothing is inferred"),
    (["--level", "sprint"], BODY, r"--level sprint needs --sprint"),
    (["--level", "project", "--sprint", "demo.1"], BODY, r"--sprint does not match --level project"),
    (["--level", "sprint", "--project", "demo"], BODY, r"--project does not match --level sprint"),
    (["--level", "project", "--project", "nope"], BODY, r"no project record named 'nope'"),
    (["--level", "sprint", "--sprint", "demo.9"], BODY, r"no sprint record has bead demo.9"),
    (["--level", "project", "--project", "old"], BODY, r"project old \(old\) is closed"),
    (["--level", "sprint", "--sprint", "demo.1"], "", r"the decision body is empty"),
    (["--level", "sprint", "--sprint", "demo.1"], "Use the small parser.\n\n", r"the decision body is a single line"),
    (["--level", "sprint", "--sprint", "demo.1"], BODY + ":::\n", r"the decision body has a line starting with ':::'"),
    (["--level", "sprint", "--sprint", "demo.1", "--until", 'a "b"'], BODY, r"--until must be one non-empty line"),
    (["--level", "sprint", "--sprint", "demo.1", "--need", "demo.9"], BODY, r"demo.9 is not a Beads issue"),
    (["--level", "sprint", "--sprint", "demo.1", "--need", "demo.1.1"], BODY, r"demo.1.1 is not labelled human"),
])
def test_decision_add_refuses(repo, args, stdin, match):
    refused(repo, "decision", "add", *args, stdin=stdin, match=r"error: " + match)


def test_decision_add_refuses_closed_sprint(repo):
    repo.set_issue("demo.2", status="closed")
    refused(repo, "decision", "add", "--level", "sprint", "--sprint", "demo.2", stdin=BODY,
            match=r"error: sprint demo.2 \(demo.2\) is closed")


def test_decision_add_refuses_source_flag(repo):
    refused(repo, "decision", "add", "--level", "sprint", "--sprint", "demo.1", "--source", "owner", stdin=BODY,
            match=r"unrecognized arguments: --source owner")


def test_decision_add_help_prints_level_rule(repo):
    res = repo.pm("decision", "add", "--help")
    assert res.returncode == 0
    text = " ".join(res.stdout.split())
    assert "project if a later sprint must follow it; sprint if it is about this sprint's own work; " \
           "skip choices cheap to reverse" in text


def test_decision_add_sprint_replaces_placeholder(repo):
    path = repo.records / "sprints/demo-1.md"
    before = path.read_text()
    res = repo.pm("decision", "add", "--level", "sprint", "--sprint", "demo.1", stdin=BODY)
    assert res.returncode == 0, res.stderr
    block = f"::: decision {{source=agent date={TODAY}}}\n{BODY}:::"
    assert path.read_text() == before.replace("inside this sprint, and why?\n\nNone yet.",
                                              f"inside this sprint, and why?\n\n{block}")
    assert repo.bd_writes() == []
    assert repo.pm("render").returncode == 0


def test_decision_add_project_appends_owner_decisions(repo):
    path = repo.records / "projects/demo.md"
    res = repo.pm("decision", "add", "--level", "project", "--project", "demo", "--confirmed",
                  "--until", "the parser grows past 500 lines", stdin=BODY)
    assert res.returncode == 0, res.stderr
    res = repo.pm("decision", "add", "--level", "project", "--project", "demo", "--need", "demo.1.2", stdin=BODY)
    assert res.returncode == 0, res.stderr
    assert "closed need demo.1.2 and added a source=owner project decision" in res.stdout
    decisions = path.read_text().split("## Decisions")[1].split("## Design pages")[0]
    assert decisions.strip().split("\n\n")[1:] == [
        "::: decision {source=owner date=2026-10-01}\nKeep it small. Because small is cheap.\n:::",
        f'::: decision {{source=owner date={TODAY} until="the parser grows past 500 lines"}}\n{BODY}:::',
        f"::: decision {{source=owner date={TODAY}}}\n{BODY}Answers `demo.1.2`.\n:::",
    ]
    assert repo.bd_writes() == [["human", "respond", "demo.1.2", f"--response={BODY}Answers `demo.1.2`."]]


# ---------------------------------------------------------------- pm decision need, pm action need, answered-need check

NEED = ("Question: Which parser should we use?\nFact: Records hold no tables yet.\n"
        "Option small: The small parser.\nCost: no tables.\nOption full: The full parser.\nCost: a new dependency.\n"
        "Default: small. It is cheap.\n")


@pytest.mark.parametrize("args, stdin, match", [
    (["decision", "--title", "Q"], NEED, r"the following arguments are required: --parent"),
    (["decision", "--title", " ", "--parent", "demo.1"], NEED, r"error: --title is empty"),
    (["decision", "--title", "Q", "--parent", "demo.1"], "", r"error: the description is empty; pipe it on stdin: "
     r"a decision need's stdin is one part per line"),
    (["decision", "--title", "Q", "--parent", "demo.9"], NEED, r"error: --parent demo.9 is not a Beads issue"),
    (["action", "--title", "Q", "--parent", "demo.1"], "", r"error: the description is empty; pipe it on stdin: "
     r"an action's description says what the owner should do and why"),
])
def test_need_refuses(repo, args, stdin, match):
    refused(repo, args[0], "need", *args[1:], stdin=stdin, match=match)


SHAPE = "; a decision need's stdin is one part per line"
LONG = " ".join(["word"] * 26)


@pytest.mark.parametrize("stdin, message", [
    (NEED.replace("Fact:", "Note:"), "line 2 starts with no known key; each line starts with Question:, Fact:, "
     "Option <label>:, Cost: or Default:" + SHAPE),
    (NEED.replace("Option full: The full parser.", "Option full:  "), "line 5: Option full: has no text" + SHAPE),
    (NEED.replace("Question: Which parser should we use?\n", ""), "give exactly one Question: line" + SHAPE),
    (NEED + "Question: Again?\n", "give exactly one Question: line" + SHAPE),
    (NEED.replace("Fact: Records hold no tables yet.\n", ""),
     "give at least one Fact: line, so the owner can decide without other context" + SHAPE),
    (NEED.replace("Option full: The full parser.\nCost: a new dependency.\n", ""),
     "give at least two Option lines; a decision needs a choice" + SHAPE),
    (NEED.replace("Option full:", "Option small:"), "two options have the label small; give each option its own label"),
    ("Cost: free.\n" + NEED, "line 1: Cost: follows no option without a cost; put one Cost: line under each Option line"),
    (NEED.replace("Cost: no tables.\n", "Cost: no tables.\nCost: slow.\n"),
     "line 5: Cost: follows no option without a cost; put one Cost: line under each Option line"),
    (NEED.replace("Cost: no tables.\n", ""), "option small has no Cost: line; put its cost on the line under it"),
    (NEED.replace("Default: small. It is cheap.\n", ""),
     "give exactly one Default: line, the label of the option taken if the owner does not answer" + SHAPE),
    (NEED + "Default: full.\n",
     "give exactly one Default: line, the label of the option taken if the owner does not answer" + SHAPE),
    (NEED.replace("Default: small.", "Default: tiny."), "Default: tiny names no option; the labels are small, full"),
    (NEED.replace("Records hold no tables yet.", f"Short one. {LONG}."),
     'the sentence "word word word word word word …" in fact 1 has 26 words; the limit is 25 (ASD-STE100); '
     "split it"),
    (NEED.replace("Records hold no tables yet.", "Need yeeef-agents-9va.38.18 and 1.7 here " + " ".join(["w"] * 21)),
     'the sentence "Need yeeef-agents-9va.38.18 and 1.7 here w …" in fact 1 has 26 words'),  # ids end no sentence
    (NEED.replace("Cost: a new", f"Cost: {LONG} a new"),
     'the sentence "word word word word word word …" in the cost of option full has 29 words'),
    (NEED.replace("It is cheap.", LONG), 'the sentence "word word word word word word …" in the default has 26 words'),
])
def test_decision_need_refuses_a_malformed_stdin(repo, stdin, message):
    refused(repo, "decision", "need", "--title", "Q", "--parent", "demo.1", stdin=stdin,
            match="error: " + re.escape(message))


@pytest.mark.parametrize("text", [
    "Run `pm decision need --title x --parent y` with " + " ".join(["w"] * 22) + ".",  # a code span is one word
    "Split e.g. " + " ".join(["w"] * 24) + " - w.",  # "e.g. " ends a sentence; a lone dash is no word
])
def test_decision_need_counts_words_by_the_design_rules(repo, text):
    res = repo.pm("decision", "need", "--title", "Q", "--parent", "demo.1",
                  stdin=NEED.replace("Records hold no tables yet.", text))
    assert res.returncode == 0, res.stderr


def test_body_format_refusals_print_the_expected_shape(repo):
    """A refusal about a body's format shows the shape it expects, with an example, so the next try can succeed."""
    res = repo.pm("decision", "add", "--level", "sprint", "--sprint", "demo.1", stdin="Use the small parser.\n")
    assert res.returncode == 1
    assert ("error: the decision body is a single line; a decision body is the decision on its first line, then its "
            "reason on the next line, e.g.:\n  ") in res.stderr
    res = repo.pm("decision", "need", "--title", "Q", "--parent", "demo.1", stdin="Which parser?\nDefault: small.\n")
    assert "line 1 starts with no known key" in res.stderr and "a decision need's stdin is one part per line" in res.stderr
    assert "\n  Question: " in res.stderr and "\n  Option a: " in res.stderr and "\n  Default: " in res.stderr
    res = repo.pm("sprint", "open", "demo", "--title", "Third", stdin="## Goal\n\nShip it.\n")
    assert "'## Scope' is missing or empty on stdin; stdin is the sprint's frame, e.g.:\n  ## Goal" in res.stderr


def test_old_need_commands_are_gone(repo):
    for args in (["need", "add", "--kind", "decision"], ["need", "respond", "demo.1.2"], ["need", "done", "demo.1.2"]):
        res = repo.pm(*args)
        assert res.returncode == 2 and "invalid choice: 'need'" in res.stderr, res.stderr


def test_need_refuses_closed_parent(repo):
    repo.set_issue("demo.2", status="closed")
    refused(repo, "decision", "need", "--title", "Q", "--parent", "demo.2", stdin=NEED, match=r"error: --parent demo.2 is closed")


def test_need_refuses_parent_outside_projects(repo):
    issues = json.loads(repo.state.read_text()) + [
        {"id": "stray", "title": "Stray", "status": "open", "issue_type": "task", "created_at": "2026-10-01T12:00:00Z"}]
    repo.state.write_text(json.dumps(issues))
    refused(repo, "decision", "need", "--title", "Q", "--parent", "stray", stdin=NEED,
            match=r"error: --parent stray is not inside a project with a record")


NEED_LAYOUT = """**Question:** Which parser should we use?

**Facts:**

- Records hold no tables yet.

**Options:**

- **(small) The small parser.** *Cost:* no tables.
- **(full) The full parser.** *Cost:* a new dependency.

**Default:** (small). It is cheap."""


def test_decision_need_creates_human_task(repo):
    res = repo.pm("decision", "need", "--title", "Pick a parser", "--parent", "demo.1", stdin=NEED)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=task", "--parent=demo.1", "--labels=human",
                                 "--title=Pick a parser", f"--description={NEED_LAYOUT}", "--json"]]
    assert "raised decision need demo.1.3 under demo.1" in res.stdout
    assert "pm decision add --need demo.1.3" in res.stdout and "pm decision close demo.1.3 --reason" in res.stdout
    assert repo.issues()["demo.1.3"]["labels"] == ["human"]
    assert repo.git("status", "--porcelain") == ""


def test_decision_need_writes_the_design_layout(repo):
    """The design's example input gives the design's Markdown: blank lines and indentation in stdin are ignored, an
    option's first sentence is bold, and code spans stay."""
    stdin = """
  Question: Where does pm keep the public site URL?

Fact: Today, each clone keeps the URL in its git config.
Fact: You asked why the URL is not in `.pm/config.toml`.

Option a: In `.pm/config.toml`. A pm command still writes it.
Cost: the repo has one URL.
Option b: In each clone, as today.
Cost: you must give the URL to each new clone.
Default: a. All clones then give the same link.
"""
    assert repo.pm("decision", "need", "--title", "Site URL", "--parent", "demo.1", stdin=stdin).returncode == 0
    assert repo.issues()["demo.1.3"]["description"] == """**Question:** Where does pm keep the public site URL?

**Facts:**

- Today, each clone keeps the URL in its git config.
- You asked why the URL is not in `.pm/config.toml`.

**Options:**

- **(a) In `.pm/config.toml`.** A pm command still writes it. *Cost:* the repo has one URL.
- **(b) In each clone, as today.** *Cost:* you must give the URL to each new clone.

**Default:** (a). All clones then give the same link."""


ACTION = "Restart the site on port 8767, which the new proxy expects.\n"


def test_action_need_creates_action_without_options(repo):
    res = repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=task", "--parent=demo.1", "--labels=human,action",
                                 "--title=Restart the site", f"--description={ACTION.strip()}", "--json"]]
    assert "raised action demo.1.3 under demo.1" in res.stdout and "pm action done demo.1.3" in res.stdout
    assert repo.issues()["demo.1.3"]["labels"] == ["human", "action"]


MERGE_31 = "Merge PR #31 (https://github.com/o/r/pull/31)"


@pytest.mark.parametrize("cmd, title, stdin", [
    ("action", MERGE_31, ACTION),
    ("action", "Look at the parser", "Please review https://github.com/o/r/pull/31 before Friday.\n"),
    ("decision", "Approve PR #31?", NEED),
])
def test_a_pr_review_or_merge_request_without_pr_is_refused_with_the_review_form(repo, cmd, title, stdin):
    refused(repo, cmd, "need", "--title", title, "--parent", "demo.1", stdin=stdin,
            match=r'review or merge a PR; raise it with the review form.*'
                  r'pm action need --pr URL --sprint ID --focus "…" \[--design SLUG\]')


def test_a_pr_merge_request_with_pr_sprint_and_focus_is_accepted(repo):
    report(repo, "Done: shipped the thing.")
    res = repo.pm("action", "need", "--pr", "https://github.com/o/r/pull/31", "--sprint", "demo.1",
                  "--focus", "The refusal regex", "--title", MERGE_31)
    assert res.returncode == 0, res.stderr
    issue = repo.issues()["demo.1.3"]
    assert (issue["title"], issue["labels"]) == (MERGE_31, ["human", "action"])
    assert issue["metadata"]["review"]["pr"] == "https://github.com/o/r/pull/31"


def test_a_pr_named_in_passing_is_allowed(repo):
    """Naming a PR without asking to review, merge or approve it is not a review request."""
    res = repo.pm("decision", "need", "--title", "Should we split PR #31?", "--parent", "demo.1", stdin=NEED)
    assert res.returncode == 0, res.stderr
    assert repo.issues()["demo.1.3"]["title"] == "Should we split PR #31?"


@pytest.mark.parametrize("args, stdin, match", [
    (["--need", "demo.1.2"], BODY, r"--level is required"),
    (["--need", "demo.1.2", "--level", "sprint"], BODY, r"--level sprint needs --sprint"),
    (["--need", "demo.1.2", "--level", "project", "--project", "old"], BODY, r"project old \(old\) is closed"),
    (["--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1"], "", r"the decision body is empty"),
    (["--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1"], "Small.", r"the decision body is a single line"),
    (["--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1", "--confirmed"], BODY,
     r"argument --confirmed: not allowed with argument --need"),
])
def test_decision_add_need_refuses(repo, args, stdin, match):
    refused(repo, "decision", "add", *args, stdin=stdin, match=match if "argument" in match else r"error: " + match)


def test_decision_add_need_refuses_dismissed_need(repo):
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    refused(repo, "decision", "add", "--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1", stdin=BODY,
            match=r"error: need demo.1.2 was dismissed, so it has no answer to record")


def test_decision_add_need_closes_need_and_records_decision(repo):
    path = repo.records / "projects/demo.md"
    res = repo.pm("decision", "add", "--need", "demo.1.2", "--level", "project", "--project", "demo", stdin=BODY)
    assert res.returncode == 0, res.stderr
    text = BODY + "Answers `demo.1.2`."
    assert repo.bd_writes() == [["human", "respond", "demo.1.2", f"--response={text}"]]
    assert repo.issues()["demo.1.2"]["status"] == "closed"
    assert path.read_text().split("## Design pages")[0].rstrip().endswith(
        f"::: decision {{source=owner date={TODAY}}}\n{text}\n:::")
    assert repo.pm("render").returncode == 0


def test_decision_add_need_prints_undo_when_write_fails(repo):
    path = repo.records / "sprints/demo-1.md"
    path.parent.chmod(0o555)
    try:
        res = repo.pm("decision", "add", "--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1", stdin=BODY)
    finally:
        path.parent.chmod(0o755)
    assert res.returncode != 0
    assert "undo the Beads step with: bd reopen demo.1.2" in res.stderr
    assert "Answers" not in path.read_text()


def test_render_fails_on_answered_need_without_decision(repo):
    repo.set_issue("demo.1.2", status="closed", close_reason="Responded")
    res = repo.pm("render")
    assert res.returncode != 0
    assert "error: need demo.1.2 (Ask the owner) is closed but no decision cites it" in res.stderr


def test_render_accepts_dismissed_need_and_cited_need(repo):
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    assert repo.pm("render").returncode == 0, "a dismissed need has no answer to record"
    repo.set_issue("demo.1.2", close_reason="Responded")
    path = repo.records / "projects/demo.md"
    # Cited as free text, as this repo's older decisions do ("Answered in `…`"); a longer id does not count.
    path.write_text(path.read_text().replace("Because small is cheap.", "Because small is cheap. See demo.1.20."))
    assert repo.pm("render").returncode != 0
    path.write_text(path.read_text().replace("See demo.1.20.", "Answered in `demo.1.2`."))
    assert repo.pm("render").returncode == 0


def test_need_dismissed_with_a_note_has_no_answer_to_record(repo):
    """bd 1.3.1's `bd human dismiss <id> "<note>"` closes with reason "Dismissed: <note>": still a dismissal."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed: No longer applicable")
    assert repo.pm("render").returncode == 0, "a dismissed need has no answer to record"
    refused(repo, "decision", "close", "demo.1.2", "--reason", "Only a port.", match=r"was dismissed")


def add_action(repo, status="open"):
    issues = json.loads(repo.state.read_text()) + [
        {"id": "demo.1.3", "title": "Merge PR #12", "status": status, "issue_type": "task", "parent": "demo.1",
         "labels": ["human", "action"], "description": ACTION, "created_at": "2026-10-01T12:00:00Z"}]
    repo.state.write_text(json.dumps(issues))


ANSWER = "Port 8767."


@pytest.mark.parametrize("args, stdin, match", [
    (["close", "demo.1.2", "--reason", "Only a port.", "--level", "sprint"], ANSWER,
     r"unrecognized arguments: --level sprint"),
    (["close", "demo.1.2"], ANSWER, r"the following arguments are required: --reason"),
    (["close", "demo.1.2", "--reason", " "], ANSWER, r"error: --reason is empty"),
    (["close", "demo.1.2", "--reason", "Only a port."], "", r"error: the answer is empty"),
    (["close", "demo.1.3", "--reason", "Only a port."], ANSWER,
     r"error: demo.1.3 is an action; close it with pm action done demo.1.3"),
    (["add", "--need", "demo.1.3", "--level", "sprint", "--sprint", "demo.1"], BODY,
     r"error: demo.1.3 is an action; close it with pm action done demo.1.3"),
])
def test_decision_close_refuses(repo, args, stdin, match):
    add_action(repo)
    refused(repo, "decision", *args, stdin=stdin, match=match)


def test_decision_close_refuses_dismissed_or_marked_need(repo):
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    refused(repo, "decision", "close", "demo.1.2", "--reason", "Only a port.", match=r"was dismissed")
    repo.set_issue("demo.1.2", close_reason="Responded", labels=["human", "no-decision"])
    refused(repo, "decision", "close", "demo.1.2", "--reason", "Only a port.", match=r"already marked no-decision")


def test_decision_close_closes_small_answer_without_record(repo):
    """A small answer closes the need with the answer and the reason, labelled no-decision, and writes no record;
    the render accepts it although no decision cites it."""
    heads = repo.store_log()
    res = repo.pm("decision", "close", "demo.1.2", "--reason", "It picks a port and sets no rule.", stdin=ANSWER)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [
        ["human", "respond", "demo.1.2", f"--response={ANSWER}\n\nNo decision record: It picks a port and sets no rule."],
        ["update", "demo.1.2", "--add-label=no-decision"]]
    need = repo.issues()["demo.1.2"]
    assert (need["status"], need["labels"]) == ("closed", ["human", "no-decision"])
    assert repo.store_log() == heads and repo.git("status", "--porcelain", cwd=repo.store) == ""
    assert repo.pm("render").returncode == 0


def test_decision_close_marks_need_the_owner_closed(repo):
    """The owner answered with bd human respond; the agent marks the answer as setting no rule."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Responded")
    assert repo.pm("render").returncode != 0
    # Only render and commit check the whole store; a write that does not touch the need goes through.
    assert repo.pm("finding", "add", "--sprint", "demo.1", "Blocked.").returncode == 0
    res = repo.pm("decision", "close", "demo.1.2", "--reason", "It picks a port and sets no rule.")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["comments", "add", "demo.1.2", "No decision record: It picks a port and sets no rule."],
                                ["update", "demo.1.2", "--add-label=no-decision"]]
    assert repo.pm("render").returncode == 0


WHY = "It picks a port and sets no rule."


def failing(repo, *prefix: str) -> dict:
    """The env for a pm run whose bd calls starting with `prefix` fail."""
    return dict(repo.env, FAKE_BD_FAIL=json.dumps(list(prefix)))


def test_decision_close_labels_after_responding_and_retries(repo):
    """The answer goes in first; a failed label leaves a closed need the render check still catches, and running
    the same command again labels it without repeating the note."""
    env, repo.env = repo.env, failing(repo, "update", "demo.1.2")
    res = repo.pm("decision", "close", "demo.1.2", "--reason", WHY, stdin=ANSWER)
    repo.env = env
    assert res.returncode != 0
    assert (f'run the same command again to label it: pm decision close demo.1.2 --reason "{WHY}"'
            in res.stderr), res.stderr
    need = repo.issues()["demo.1.2"]
    assert (need["status"], need["labels"]) == ("closed", ["human"])
    assert repo.pm("render").returncode != 0
    res = repo.pm("decision", "close", "demo.1.2", "--reason", WHY)
    assert res.returncode == 0, res.stderr
    assert repo.comments("demo.1.2") == [f"Response: {ANSWER}\n\nNo decision record: {WHY}"]
    assert repo.issues()["demo.1.2"]["labels"] == ["human", "no-decision"]
    assert repo.pm("render").returncode == 0


def test_decision_close_on_closed_need_is_safe_to_retry(repo):
    """On a need the owner closed, a failed label leaves the note; the retry adds the label and no second note."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Responded")
    env, repo.env = repo.env, failing(repo, "update", "demo.1.2")
    res = repo.pm("decision", "close", "demo.1.2", "--reason", WHY)
    repo.env = env
    assert res.returncode != 0
    assert "run the same command again to label it" in res.stderr and "it does not add the note twice" in res.stderr
    assert repo.comments("demo.1.2") == [f"No decision record: {WHY}"]
    res = repo.pm("decision", "close", "demo.1.2", "--reason", WHY)
    assert res.returncode == 0, res.stderr
    assert "undo with: bd update demo.1.2 --remove-label=no-decision" in res.stdout
    assert repo.comments("demo.1.2") == [f"No decision record: {WHY}"]
    assert repo.issues()["demo.1.2"]["labels"] == ["human", "no-decision"]
    assert repo.pm("render").returncode == 0


def test_decision_add_records_answer_of_need_the_owner_closed(repo):
    """The answered-need check that blocks every other write lets through the write that records the answer."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Responded")
    res = repo.pm("decision", "add", "--level", "sprint", "--sprint", "demo.1", "--need", "demo.1.2", stdin=BODY)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == []
    assert repo.pm("render").returncode == 0


def test_render_still_fails_on_rule_answer_that_skipped_its_decision(repo):
    """Closing a decision need any other way (bd human respond, bd close) leaves it to the check."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Use the small parser everywhere")
    res = repo.pm("render")
    assert res.returncode != 0
    assert ("need demo.1.2 (Ask the owner) is closed but no decision cites it; record the owner's answer with "
            "pm decision add --need demo.1.2, or, if the answer sets no rule, close it with pm decision close "
            "demo.1.2 --reason") in res.stderr


def test_action_done_closes_action_and_render_needs_no_decision(repo):
    add_action(repo)
    refused(repo, "action", "done", "demo.1.2", "--reason", "Seen.",
            match=r"error: demo.1.2 is a decision need; answer it with pm decision add --need demo.1.2, or pm "
                  r"decision close demo.1.2 if the answer sets no rule")
    refused(repo, "action", "done", "demo.1.3", "--reason", " ", match=r"error: --reason is empty")
    res = repo.pm("action", "done", "demo.1.3", "--reason", "PR #12 is merged.")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["close", "demo.1.3", "--reason=PR #12 is merged."]]
    assert repo.pm("render").returncode == 0, "a done action has no answer to record"
    repo.log.write_text("")
    refused(repo, "action", "done", "demo.1.3", "--reason", "Again.", match=r"error: action demo.1.3 is already closed")


@pytest.mark.parametrize("args, match", [
    (["--pr", PR, "--focus", "F"], r"error: --sprint is required with --pr"),
    (["--pr", PR, "--sprint", "demo.1"], r"error: --focus is required with --pr"),
    (["--pr", PR, "--sprint", "demo.1", "--focus", "F", "--parent", "demo.1"], r"error: --parent is not allowed with --pr"),
    (["--title", "T", "--parent", "demo.1", "--sprint", "demo.1"], r"error: --sprint is only for a PR review"),
    (["--title", "T", "--parent", "demo.1", "--focus", "F"], r"error: --focus is only for a PR review"),
    (["--title", "T", "--parent", "demo.1", "--design", "arch"], r"error: --design is only for a PR review"),
    (["--title", "T"], r"error: --parent is required"),
    (["--parent", "demo.1"], r"error: --title is required"),
    (["--pr", "PR 12", "--sprint", "demo.1", "--focus", "F"], r"error: --pr 'PR 12' is not a URL"),
    (["--pr", PR, "--sprint", "demo.1", "--focus", " "], r"error: --focus is empty"),
    (["--pr", PR, "--sprint", "demo.9", "--focus", "F"], r"error: no sprint record has bead demo.9"),
    (["--pr", PR, "--sprint", "demo.1", "--focus", "F", "--design", "nope"], r"error: --design nope has no design page"),
])
def test_action_need_review_flags_refuse(repo, args, match):
    report(repo, "Done: shipped the thing.")
    refused(repo, "action", "need", *args, match=match)


def test_action_need_pr_refuses_an_unwritten_report(repo):
    """The owner reads the report while reviewing, so a review needs each sprint's report written and committed."""
    refused(repo, "action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "F",
            match=r"error: records/sprints/demo-1.md: the committed Delivery report 'Outcome' and 'Against \"Done "
                  r"when\"' are still 'Not closed yet.'; write the Outcome .* and commit them with pm commit")
    report(repo, "Done: shipped.", against="Not closed yet.")
    refused(repo, "action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "F",
            match=r"error: records/sprints/demo-1.md: the committed Delivery report 'Against \"Done when\"' is still")
    report(repo, "Done: shipped.")
    report(repo, "Not closed yet.", name="demo-2")
    refused(repo, "action", "need", "--pr", PR, "--sprint", "demo.1", "--sprint", "demo.2", "--focus", "F",
            match=r"error: records/sprints/demo-2.md: the committed Delivery report 'Outcome' is still")
    path = repo.store / "sprints/demo-1.md"  # an uncommitted report does not count: the owner reads the committed one
    report(repo, "Not closed yet.")
    path.write_text(path.read_text().replace("Not closed yet.\n\n### Against", "Done: shipped.\n\n### Against"))
    refused(repo, "action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "F",
            match=r"error: records/sprints/demo-1.md: the committed Delivery report 'Outcome' is still")


def test_action_need_pr_refuses_a_closed_sprint(repo):
    """A sprint closes only after its PR merges, so a PR under review delivers no closed sprint."""
    report(repo, "Done: shipped.")
    repo.set_issue("demo.2", status="closed")
    for sprints in (["demo.2"], ["demo.1", "demo.2"]):
        refused(repo, "action", "need", "--pr", PR, *[a for sid in sprints for a in ("--sprint", sid)], "--focus", "F",
                match=r"error: sprint demo.2 is closed, but a sprint closes only after its PR merges")


def test_action_review_command_is_gone(repo):
    res = repo.pm("action", "review", "--pr", PR, "--sprint", "demo.1", "--focus", "F")
    assert res.returncode == 2 and "invalid choice: 'review'" in res.stderr, res.stderr
    assert repo.bd_writes() == []


def test_action_need_pr_card_links_pr_sprints_design_pages_and_focus(repo):
    """A review request carries its context; the action card links the PR, each sprint, the design pages named
    and those the sprint record lists, and shows the focus."""
    for slug, title in (("arch", "Architecture"), ("store", "Store")):
        assert repo.pm("design", "new", slug, "--title", title, "--project", "demo").returncode == 0
    path = repo.store / "sprints/demo-2.md"
    path.write_text(path.read_text().replace("Where is the detail?\n\nNone yet.",
                                             "Where is the detail?\n\n- [Store](../design/store.md): where it lives"))
    repo.commit()
    report(repo, "Done: shipped.", name="demo-2")
    report(repo, "Done: shipped the thing.")
    res = repo.pm("action", "need", "--pr", PR, "--sprint", "demo.1", "--sprint", "demo.2",
                  "--focus", "The lock in `commit`: is it held across the bd step?", "--design", "arch")
    assert res.returncode == 0, res.stderr
    issue = repo.issues()["demo.1.3"]
    assert (issue["title"], issue["labels"], issue["parent"]) == ("Review PR #12", ["human", "action"], "demo.1")
    assert repo.pm("render").returncode == 0
    for page, up in (("index.html", ""), ("days/2026-10-01.html", "../")):
        html = (repo.root / "site" / page).read_text()
        card = html.split('id="actions-await-you"')[1].split('<div class="card need"')[1].split("</div>")[0]
        assert f'<a href="{PR}">{PR}</a>' in card
        assert (f'<a href="{up}sprints/demo-1.html">Sprint 1: First</a>, <a href="{up}sprints/demo-2.html">Sprint 2: '
                f'Second</a> · <a href="{up}sprints/demo-1.html#delivery-report">delivery report: First</a>, '
                f'<a href="{up}sprints/demo-2.html#delivery-report">delivery report: Second</a>') in card
        assert f'<a href="{up}design/arch.html">Architecture</a>, <a href="{up}design/store.html">Store</a>' in card
        assert "The lock in <code>commit</code>: is it held across the bd step?" in card


def test_sprint_page_lists_its_own_requests_the_pr_review_included(repo):
    """A sprint page shows the open requests under the sprint and its tasks, the PR review's card with its PR link,
    focus and reply slot, between Progress and Decisions; another sprint's requests stay off it."""
    report(repo, "Done: shipped the thing.")
    assert repo.pm("action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "The lock").returncode == 0
    assert repo.pm("render").returncode == 0
    page = (repo.root / "site/sprints/demo-1.html").read_text()
    assert page.index('id="progress"') < page.index('id="decisions-await-you"') < page.index('id="actions-await-you"') \
        < page.index('<h2 id="decisions">')
    decisions = page.split('id="decisions-await-you"')[1].split('id="actions-await-you"')[0]
    assert 'id="need-demo.1.2"' in decisions and "<!--pm-reply demo.1.2 decision-->" in decisions
    actions = page.split('id="actions-await-you"')[1].split('<h2 id="decisions">')[0]
    card = actions.split('<div class="card need" id="need-demo.1.3">')[1]
    assert f'<a href="{PR}">{PR}</a>' in card and "<p>The lock</p>" in card and "<!--pm-reply demo.1.3 action-->" in card
    assert "Draft" not in page
    other = (repo.root / "site/sprints/demo-2.html").read_text()
    assert ('<h2 id="decisions-await-you">Decisions await you</h2>' in other
            and '<p class="empty">No decisions waiting on the owner.</p>' in other
            and '<p class="empty">No actions waiting on the owner.</p>' in other and "need-demo" not in other)


@pytest.mark.parametrize("heading", ["Decisions await you", "Actions await you"])
def test_render_refuses_a_hand_written_await_you_section_in_a_sprint(repo, heading):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("## Decisions\n", f"## {heading}\n\nHand-written.\n\n## Decisions\n"))
    res = repo.pm("render")
    assert res.returncode == 1 and f"'## {heading}' is a section the page generates" in res.stderr, res.stderr


@pytest.mark.parametrize("review", [
    "see PR 12",
    {"pr": PR, "sprints": ["demo.1"]},
    {"pr": PR, "sprints": "demo.1", "focus": "F"},
])
def test_malformed_review_metadata_is_a_clear_error(repo, review):
    """A review set by hand with bd, not by pm action need --pr, fails render with the issue named, not a
    traceback; it is not skipped, so the owner's card never silently loses its context."""
    add_action(repo)
    repo.set_issue("demo.1.3", metadata={"review": review})
    for args in (["render"],):
        res = repo.pm(*args)
        assert res.returncode == 1 and "Traceback" not in res.stderr, res.stderr
        assert re.match(r"error: demo.1.3 \(Merge PR #12\): metadata\.review ", res.stderr), res.stderr


def test_request_cards_and_show_lines_name_the_sprint_and_task_a_request_sits_under(repo):
    """A request under a sprint links that sprint beside the project; under a sprint's task it also names the task;
    directly under the project it shows only the project."""
    issues = json.loads(repo.state.read_text()) + [
        {"id": "demo.1.4", "title": "Build it", "status": "open", "issue_type": "task", "parent": "demo.1",
         "created_at": "2026-10-01T12:00:00Z"},
        {"id": "demo.1.4.1", "title": "Under a task", "status": "open", "issue_type": "task", "parent": "demo.1.4",
         "labels": ["human"], "description": "Which one?", "created_at": "2026-10-01T12:00:00Z"},
        {"id": "demo.3", "title": "Under the project", "status": "open", "issue_type": "task", "parent": "demo",
         "labels": ["human"], "description": "Which one?", "created_at": "2026-10-01T12:00:00Z"}]
    repo.state.write_text(json.dumps(issues))
    assert repo.pm("render").returncode == 0
    for page, up in (("index.html", ""), ("days/2026-10-01.html", "../")):
        html = (repo.root / "site" / page).read_text()
        where = {i: re.search(rf'id="need-{re.escape(i)}">.*?<p class="k">(.*?) · {re.escape(i)}</p>', html).group(1)
                 for i in ("demo.1.2", "demo.1.4.1", "demo.3")}
        project = f'<a href="{up}projects/demo.html">Demo</a>'
        sprint = f'<a href="{up}sprints/demo-1.html">Sprint 1: First</a>'
        assert where == {"demo.1.2": f"{project} · {sprint}", "demo.1.4.1": f"{project} · {sprint} · task Build it",
                         "demo.3": project}, page
    out = repo.pm("show").stdout
    assert "  .1.2  Ask the owner  (sprint 1)  -> bd show demo.1.2" in out
    assert "  .1.4.1  Under a task  (sprint 1, task .1.4)  -> bd show demo.1.4.1" in out
    assert "  .3  Under the project  -> bd show demo.3" in out


def test_a_decision_need_card_renders_the_layout_and_an_old_free_text_need_still_renders(repo):
    """A need raised in the new form shows each block as its own paragraph or list; an old free-text need, raised
    before the layout, keeps rendering as text."""
    assert repo.pm("decision", "need", "--title", "Pick a parser", "--parent", "demo.1", stdin=NEED).returncode == 0
    repo.set_issue("demo.1.2", description="Which parser?\nOptions: small (cheap); full (slow)\nDefault: small")
    assert repo.pm("render").returncode == 0
    html = (repo.root / "site/index.html").read_text()
    card = lambda i: html.split(f'<div class="card need" id="need-{i}">')[1].split('<div class="card')[0]
    assert ("<p><strong>Question:</strong> Which parser should we use?</p>\n<p><strong>Facts:</strong></p>\n<ul>\n"
            "<li>Records hold no tables yet.</li>\n</ul>\n<p><strong>Options:</strong></p>\n<ul>\n"
            "<li><strong>(small) The small parser.</strong> <em>Cost:</em> no tables.</li>\n") in card("demo.1.3")
    assert "<p><strong>Default:</strong> (small). It is cheap.</p>" in card("demo.1.3")
    assert "<p>Which parser?\nOptions: small (cheap); full (slow)\nDefault: small</p>" in card("demo.1.2")


@pytest.mark.parametrize("fact, item", [
    ("2026. The year we ship.", "2026. The year we ship."), ("3) Three.", "3) Three."), ("# x.", "# x."),
    ("> q.", "&gt; q."), ("- dash.", "- dash."), ("+ plus.", "+ plus."), ("* star.", "* star."),
    ("*Note* the tables.", "<em>Note</em> the tables."), ("-flag stays.", "-flag stays."),
])
def test_a_fact_that_starts_with_a_block_marker_renders_as_one_plain_item(repo, fact, item):
    stdin = NEED.replace("Records hold no tables yet.", fact)
    assert repo.pm("decision", "need", "--title", "Pick a parser", "--parent", "demo.1", stdin=stdin).returncode == 0
    assert repo.pm("render").returncode == 0
    html = (repo.root / "site/index.html").read_text()
    card = html.split('<div class="card need" id="need-demo.1.3">')[1].split('<div class="card')[0]
    assert f"<p><strong>Facts:</strong></p>\n<ul>\n<li>{item}</li>\n</ul>" in card


def test_site_shows_decisions_and_actions_under_their_own_headings(repo):
    add_action(repo)
    assert repo.pm("render").returncode == 0
    for page in ("index.html", "days/2026-10-01.html"):
        html = (repo.root / "site" / page).read_text()
        decisions = html.split('<h2 id="decisions-await-you">Decisions await you</h2>')[1].split('id="actions-await-you"')[0]
        actions = html.split('<h2 id="actions-await-you">Actions await you</h2>')[1].split('id="sprints"')[0]
        assert "Ask the owner" in decisions and "bd human respond demo.1.2" in decisions and "Merge PR #12" not in decisions
        assert "Merge PR #12" in actions and "the agent checks it and closes this." in actions
        assert "Ask the owner" not in actions
    out = repo.pm("show").stdout
    assert "  ready        .1.3  Merge PR #12  [human action]" in out
    assert "actions await you (1):\n  .1.3  Merge PR #12  (sprint 1)  -> bd show demo.1.3" in out


# ---------------------------------------------------------------- doc record and pm doc new

DOC = "## Result\n\nArm B wins: 0.037 vs 0.041.\n"


@pytest.mark.parametrize("args, stdin, match", [
    (["note", "--title", "T"], DOC, r"one of the arguments --bead --project is required"),
    (["note", "--title", "T", "--bead", "demo.1", "--project", "demo"], DOC, r"not allowed with argument"),
    (["Bad_Slug", "--title", "T", "--bead", "demo.1"], DOC, r"error: doc slug 'Bad_Slug' must be lowercase"),
    (["note", "--title", " ", "--bead", "demo.1"], DOC, r"error: --title is empty"),
    (["note", "--title", "T", "--bead", "demo.1"], "", r"error: the doc body is empty"),
    (["note", "--title", "T", "--bead", "demo.9"], DOC, r"error: --bead demo.9 is not a Beads issue"),
    (["note", "--title", "T", "--project", "nope"], DOC, r"error: no project record named 'nope'"),
    (["note", "--title", "T", "--bead", "demo.1"], DOC + "\n::: verdict\nx\n:::\n", r"error: the change would not render: .*unknown block '::: verdict'"),
])
def test_doc_new_refuses(repo, args, stdin, match):
    refused(repo, "doc", "new", *args, stdin=stdin, match=match)


def test_doc_new_refuses_existing_file(repo):
    (repo.records / "docs").mkdir()
    repo.write(f"docs/{TODAY}-note.md", f"---\ntype: doc\ntitle: Old\ndate: {TODAY}\nproject: demo\n---\n\nx\n")
    repo.commit()
    refused(repo, "doc", "new", "note", "--title", "T", "--project", "demo", stdin=DOC,
            match=rf"error: records/docs/{TODAY}-note.md already exists")


@pytest.mark.parametrize("header, match", [
    ("date: 2026-10-01\n", r"a doc names exactly one of 'bead' or 'project'"),
    ("date: 2026-10-01\nbead: demo.1\nproject: demo\n", r"a doc names exactly one of 'bead' or 'project'"),
    ("date: 2026-10-02\nbead: demo.1\n", r"a doc dated 2026-10-02 lives at docs/2026-10-02-<slug>.md"),
    ("date: 2026-10-01\nbead: demo.9\n", r"bead demo.9 not found in Beads"),
    ("date: 2026-10-01\nproject: nope\n", r"no project record found"),
    ("bead: demo.1\n", r"missing header fields: date"),
])
def test_render_refuses_bad_doc_header(repo, header, match):
    (repo.records / "docs").mkdir()
    repo.write("docs/2026-10-01-note.md", f"---\ntype: doc\ntitle: Note\n{header}---\n\nx\n")
    res = repo.pm("render")
    assert res.returncode != 0 and re.search(match, res.stderr), res.stderr


def test_doc_new_writes_and_pages_list_it(repo):
    res = repo.pm("doc", "new", "arm-b", "--title", "Arm B: the result", "--bead", "demo.1.1", stdin=DOC)
    assert res.returncode == 0, res.stderr
    path = repo.records / f"docs/{TODAY}-arm-b.md"
    assert path.read_text() == f'---\ntype: doc\ntitle: "Arm B: the result"\ndate: {TODAY}\nbead: demo.1.1\n---\n\n{DOC}'
    assert repo.pm("doc", "new", "overview", "--title", "Overview", "--project", "demo", stdin=DOC).returncode == 0
    assert repo.bd_writes() == []
    assert repo.pm("render").returncode == 0
    site = repo.root / "site"
    doc_a, doc_o = f"docs/{TODAY}-arm-b.html", f"docs/{TODAY}-overview.html"
    listed = lambda page: re.findall(r'href="(?:\.\./)?(docs/[^"]+)"', (site / page).read_text())
    assert listed("sprints/demo-1.html") == [doc_a]          # its task's doc, not the project doc
    assert listed("sprints/demo-2.html") == []
    assert sorted(listed("projects/demo.html")) == sorted([doc_a, doc_o])
    assert sorted(listed(f"days/{TODAY}.html")) == sorted([doc_a, doc_o])
    assert listed("days/2026-10-01.html") == []
    assert sorted(listed("index.html")) == sorted([doc_a, doc_o])
    page = (site / doc_a).read_text()
    assert "Arm B wins: 0.037 vs 0.041." in page and 'href="../projects/demo.html"' in page


# ---------------------------------------------------------------- pm feedback add

def feedback_env(repo, sid="sess-1"):
    env = {k: v for k, v in repo.env.items() if k not in ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID")}
    repo.env = dict(env, CLAUDE_CODE_SESSION_ID=sid) if sid else env


@pytest.mark.parametrize("args, stdin, match", [
    (["--project", "demo"], "", r"error: the feedback text is empty"),
    (["--project", "demo", "--text", "  "], "", r"error: the feedback text is empty"),
    (["--text", "x"], "", r"the following arguments are required: --project"),
    (["--project", "nope", "--text", "x"], "", r"error: no project record named 'nope'"),
    (["--project", "demo", "--sprint", "demo.9", "--text", "x"], "", r"error: --sprint demo.9 is not a Beads issue"),
    (["--project", "demo", "--text", "x\n::: verdict\ny\n:::"], "", r"error: the change would not render"),
])
def test_feedback_add_refuses(repo, args, stdin, match):
    feedback_env(repo)
    refused(repo, "feedback", "add", *args, stdin=stdin, match=match)


def test_feedback_add_refuses_without_session(repo):
    feedback_env(repo, sid=None)
    refused(repo, "feedback", "add", "--project", "demo", "--text", "x", match=r"error: no agent session")


def test_feedback_add_creates_then_appends_one_doc(repo):
    feedback_env(repo)
    res = repo.pm("feedback", "add", "--project", "demo", "--sprint", "demo.1", "--task", "demo.1.2",
                  "--text", "The refusal named no fix.")
    assert res.returncode == 0, res.stderr
    feedback_env(repo, "sess-2")
    assert repo.pm("feedback", "add", "--project", "demo", stdin="No command to list tasks.\n").returncode == 0
    docs = list((repo.records / "docs").glob("*-feedback.md"))
    assert [p.name for p in docs] == [f"{TODAY}-demo-feedback.md"]
    text = docs[0].read_text()
    assert text.startswith(f"---\ntype: doc\ntitle: pm feedback\ndate: {TODAY}\nproject: demo\n---\n\n")
    first, second = re.findall(r"(?m)^### \d{4}-\d\d-\d\d \d\d:\d\d UTC, session `(.+?)`$", text), text.split("### ")
    assert first == ["sess-1", "sess-2"]                                       # newest last
    assert "About sprint `demo.1`, task `demo.1.2`.\n\nThe refusal named no fix.\n" in second[1]
    assert second[2].endswith("`sess-2`\n\nNo command to list tasks.\n")      # no About line without --sprint/--task
    assert repo.store_log()[:2] == [f"pm: added feedback to records/docs/{TODAY}-demo-feedback.md"] * 2
    assert repo.bd_writes() == []
    assert repo.pm("render").returncode == 0
    page = (repo.root / "site" / f"docs/{TODAY}-demo-feedback.html").read_text()
    assert "The refusal named no fix." in page and "No command to list tasks." in page


def test_feedback_add_refuses_two_feedback_docs(repo):
    feedback_env(repo)
    (repo.records / "docs").mkdir()
    for day in ("2026-10-01", "2026-10-02"):
        repo.write(f"docs/{day}-demo-feedback.md", f"---\ntype: doc\ntitle: pm feedback\ndate: {day}\nproject: demo\n---\n\nx\n")
    repo.commit()
    refused(repo, "feedback", "add", "--project", "demo", "--text", "x", match=r"error: project demo has 2 feedback docs")


# ---------------------------------------------------------------- design page template and pm design new

@pytest.mark.parametrize("args, match", [
    (["arch", "--title", "T"], r"the following arguments are required: --project"),
    (["Bad_Slug", "--title", "T", "--project", "demo"], r"error: design slug 'Bad_Slug' must be lowercase"),
    (["arch", "--title", " ", "--project", "demo"], r"error: --title is empty"),
    (["arch", "--title", "T", "--project", "nope"], r"error: no project record named 'nope'"),
])
def test_design_new_refuses(repo, args, match):
    refused(repo, "design", "new", *args, match=match)


REQUIRED_DESIGN = ["Problem", "Goals and non-goals", "Constraints and key facts", "Design",
                   "Alternatives considered", "Open questions"]


def design(title="Arch", skip=None):
    body = "".join(f"## {h}\n\nx\n\n" for h in REQUIRED_DESIGN if h != skip)
    return f"---\ntype: design\ntitle: {title}\nproject: demo\n---\n\n{body}"


def test_design_new_refuses_existing_file(repo):
    (repo.records / "design").mkdir()
    repo.write("design/arch.md", design("Old"))
    refused(repo, "design", "new", "arch", "--title", "T", "--project", "demo",
            match=r"error: records/design/arch.md already exists")


def test_design_new_writes_every_section_and_renders(repo):
    res = repo.pm("design", "new", "arch", "--title", "Arch: the layout", "--project", "demo")
    assert res.returncode == 0, res.stderr
    text = (repo.records / "design/arch.md").read_text()
    assert text.startswith('---\ntype: design\ntitle: "Arch: the layout"\nproject: demo\n---\n\n## Problem\n\n> ')
    heads = [l[3:] for l in text.splitlines() if l.startswith("## ")]
    assert heads == REQUIRED_DESIGN[:5] + ["Prior art", "Open questions"]
    assert text.count("\n\nNone yet.\n") == 7 and text.endswith("None yet.\n")
    assert all(re.search(rf"^## {re.escape(h)}\n\n> ", text, re.M) for h in heads)  # each opens with a prompt
    assert repo.bd_writes() == []
    assert repo.pm("render").returncode == 0
    page = (repo.root / "site/design/arch.html").read_text()
    assert '<nav class="toc">' in page and 'href="../projects/demo.html"' in page


@pytest.mark.parametrize("missing", REQUIRED_DESIGN)
def test_render_refuses_design_missing_required_section(repo, missing):
    (repo.records / "design").mkdir()
    repo.write("design/arch.md", design(skip=missing))
    res = repo.pm("render")
    assert res.returncode != 0
    assert f"design/arch: design record needs a '## {missing}' section" in res.stderr


def test_render_accepts_design_without_prior_art(repo):
    (repo.records / "design").mkdir()
    repo.write("design/arch.md", design())
    assert repo.pm("render").returncode == 0


def store_commit(repo, day: str, *args: str) -> None:
    """Commit everything changed in the store with author and committer date `day`, after running git `args` there."""
    env = dict(os.environ, GIT_AUTHOR_DATE=f"{day}T12:00:00", GIT_COMMITTER_DATE=f"{day}T12:00:00")
    for cmd in ([*args] if args else [], ["add", "-A"], ["commit", "-qm", day]):
        if cmd:
            subprocess.run(["git", *cmd], cwd=repo.store, env=env, check=True, capture_output=True)


def own_design(title: str) -> str:
    """A design page whose body is its own, so git sees no copy or rename between two such pages."""
    return design(title).replace("\nx\n", "".join(f"\n{title} {i}" for i in range(9)) + "\n")


def dated_designs(repo) -> None:
    """Arch: made 2026-01-02, edited 2026-03-04. Store: made 2026-02-01 as old-store, renamed 2026-05-06. Edited:
    made 2026-01-10, edited since without a commit. New: never committed. Bodies differ, so git sees no copies."""
    (repo.store / "design").mkdir()
    (repo.store / "design/arch.md").write_text(own_design("Arch"))
    store_commit(repo, "2026-01-02")
    (repo.store / "design/edited.md").write_text(own_design("Edited"))
    store_commit(repo, "2026-01-10")
    (repo.store / "design/old-store.md").write_text(own_design("Store"))
    store_commit(repo, "2026-02-01")
    (repo.store / "design/arch.md").write_text(own_design("Arch") + "More.\n")
    store_commit(repo, "2026-03-04")
    store_commit(repo, "2026-05-06", "mv", "design/old-store.md", "design/store.md")
    (repo.store / "design/edited.md").write_text(own_design("Edited") + "More.\n")
    (repo.store / "design/new.md").write_text(own_design("New"))


def test_design_pages_show_dates_from_the_stores_git_history(repo):
    """Each design page and its overview entry show its first and last commit dates, as git log --follow gives
    them; a page with uncommitted changes counts as updated today. The overview lists the newest update first."""
    from datetime import date
    dated_designs(repo)
    today = date.today().isoformat()
    follow = lambda slug: repo.git("log", "--follow", "--format=%cs", "--", f"design/{slug}.md", cwd=repo.store).split()
    assert follow("arch") == ["2026-03-04", "2026-01-02"] and follow("store") == ["2026-05-06", "2026-02-01"]
    expected = {"Edited": ("2026-01-10", today), "New": (today, today), "Store": ("2026-02-01", "2026-05-06"),
                "Arch": ("2026-01-02", "2026-03-04")}
    res = repo.pm("render")
    assert res.returncode == 0, res.stderr
    index = (repo.root / "site/index.html").read_text()
    listed = re.findall(r'<li><a href="design/[a-z-]+\.html">(\w+)</a><span class="k"><span>created ([\d-]+)</span>'
                        r'<span>updated ([\d-]+)</span></span></li>', index)
    assert listed == [(title, *expected[title]) for title in ("Edited", "New", "Store", "Arch")]
    for slug, title in (("arch", "Arch"), ("store", "Store"), ("new", "New"), ("edited", "Edited")):
        created, updated = expected[title]
        page = (repo.root / f"site/design/{slug}.html").read_text()
        assert f'<p class="meta"><span>created {created}</span><span>updated {updated}</span></p>' in page


def test_serve_shows_design_dates_after_a_commit_that_changes_no_file(repo, served):
    """pm serve recomputes the dates when the store's HEAD moves, though no record file changed."""
    from datetime import date
    dated_designs(repo)
    today = date.today().isoformat()
    get = lambda: load(f"{served}/design/new.html")
    until_shown(get, lambda p: f"<span>created {today}</span><span>updated {today}</span>" in p, time.time())
    store_commit(repo, "2026-07-08")
    until_shown(get, lambda p: "<span>created 2026-07-08</span><span>updated 2026-07-08</span>" in p, time.time())


def test_design_prompts_match_real_design_pages():
    """Each template section in this repo's design pages carries the prompt lines pm design new writes."""
    import sys
    sys.path.insert(0, str(HARNESS))
    from pm import cli as pm
    store = REAL_RECORDS.resolve()
    real = list((store / "design").glob("*.md"))
    assert real
    for p in real:
        prompts = dict(re.findall(r"(?m)^## (.+)\n\n((?:>.*\n)+)", p.read_text()))
        for name, prompt in pm.DESIGN_PROMPTS.items():
            if name in prompts:
                assert prompts[name].rstrip("\n") == prompt, (p.name, name)


# ---------------------------------------------------------------- postmortem record and pm postmortem new

POSTMORTEM_SECTIONS = ["Summary", "Timeline", "Cost", "Root cause", "What changed",
                       "What would have caught it earlier"]


def postmortem(header="sprint: demo.1\n", skip=None):
    body = "".join(f"## {h}\n\n> x\n\nx\n\n" for h in POSTMORTEM_SECTIONS if h != skip)
    return f"---\ntype: postmortem\ntitle: Outage\ndate: 2026-10-01\n{header}---\n\n{body}"


@pytest.mark.parametrize("args, match", [
    (["outage", "--title", "T"], r"one of the arguments --sprint --project is required"),
    (["outage", "--title", "T", "--sprint", "demo.1", "--project", "demo"], r"not allowed with argument"),
    (["Bad_Slug", "--title", "T", "--sprint", "demo.1"], r"error: postmortem slug 'Bad_Slug' must be lowercase"),
    (["outage", "--title", " ", "--sprint", "demo.1"], r"error: --title is empty"),
    (["outage", "--title", "T", "--sprint", "demo.9"], r"error: no sprint record has bead demo.9"),
    (["outage", "--title", "T", "--sprint", "demo.1.1"], r"error: no sprint record has bead demo.1.1"),
    (["outage", "--title", "T", "--project", "nope"], r"error: no project record named 'nope'"),
])
def test_postmortem_new_refuses(repo, args, match):
    refused(repo, "postmortem", "new", *args, match=match)


def test_postmortem_new_refuses_existing_file(repo):
    (repo.records / "postmortems").mkdir()
    repo.write(f"postmortems/{TODAY}-outage.md", postmortem().replace("2026-10-01", TODAY))
    repo.commit()
    refused(repo, "postmortem", "new", "outage", "--title", "T", "--project", "demo",
            match=rf"error: records/postmortems/{TODAY}-outage.md already exists")


def test_postmortem_new_writes_every_section_and_pages_list_it(repo):
    res = repo.pm("postmortem", "new", "outage", "--title", "Outage: pm refused", "--sprint", "demo.1")
    assert res.returncode == 0, res.stderr
    text = (repo.records / f"postmortems/{TODAY}-outage.md").read_text()
    assert text.startswith(f'---\ntype: postmortem\ntitle: "Outage: pm refused"\ndate: {TODAY}\nsprint: demo.1\n'
                           '---\n\n## Summary\n\n> ')
    heads = [l[3:] for l in text.splitlines() if l.startswith("## ")]
    assert heads == POSTMORTEM_SECTIONS
    assert text.count("\n\nNone yet.\n") == 6 and text.endswith("None yet.\n")
    assert all(re.search(rf"^## {re.escape(h)}\n\n> ", text, re.M) for h in heads)  # each opens with a prompt
    assert repo.pm("postmortem", "new", "drift", "--title", "Drift", "--project", "demo").returncode == 0
    assert repo.bd_writes() == []
    assert committed(repo, [f"pm: created records/postmortems/{TODAY}-drift.md; fill in its sections by hand in the "
                            f"store, then pm commit -m \"…\" records/postmortems/{TODAY}-drift.md"])
    assert repo.pm("render").returncode == 0
    site = repo.root / "site"
    pm_s, pm_p = f"postmortems/{TODAY}-outage.html", f"postmortems/{TODAY}-drift.html"
    listed = lambda page: re.findall(r'href="(?:\.\./)?(postmortems/[^"]+)"', (site / page).read_text())
    assert listed("sprints/demo-1.html") == [pm_s]           # its sprint's, not the project-level one
    assert listed("sprints/demo-2.html") == []
    assert sorted(listed("projects/demo.html")) == sorted([pm_s, pm_p])
    assert sorted(listed("index.html")) == sorted([pm_s, pm_p])
    assert 'id="postmortems"' in (site / "sprints/demo-1.html").read_text()
    page = (site / pm_s).read_text()
    assert 'href="../projects/demo.html"' in page and 'href="../sprints/demo-1.html">Sprint 1: First</a>' in page


@pytest.mark.parametrize("header, path, match", [
    ("", "2026-10-01-outage", r"a postmortem names exactly one of 'sprint' or 'project'"),
    ("sprint: demo.1\nproject: demo\n", "2026-10-01-outage", r"a postmortem names exactly one of 'sprint' or 'project'"),
    ("sprint: demo.1\n", "2026-10-02-outage", r"a postmortem dated 2026-10-01 lives at postmortems/2026-10-01-<slug>.md"),
    ("sprint: demo.1\n", "outage", r"a postmortem dated 2026-10-01 lives at postmortems/2026-10-01-<slug>.md"),
    ("sprint: demo.9\n", "2026-10-01-outage", r"no sprint record has bead demo.9"),
    ("project: nope\n", "2026-10-01-outage", r"no project record found"),
])
def test_render_refuses_bad_postmortem_header(repo, header, path, match):
    (repo.records / "postmortems").mkdir()
    repo.write(f"postmortems/{path}.md", postmortem(header))
    res = repo.pm("render")
    assert res.returncode != 0 and re.search(match, res.stderr), res.stderr


@pytest.mark.parametrize("missing", POSTMORTEM_SECTIONS)
def test_render_refuses_postmortem_missing_section(repo, missing):
    (repo.records / "postmortems").mkdir()
    repo.write("postmortems/2026-10-01-outage.md", postmortem(skip=missing))
    res = repo.pm("render")
    assert res.returncode != 0
    assert f"postmortems/2026-10-01-outage: postmortem record needs a '## {missing}' section" in res.stderr


def test_postmortem_prompts_match_real_postmortems():
    """Each section of this repo's postmortems carries the prompt line pm postmortem new writes."""
    sys.path.insert(0, str(HARNESS))
    from pm import cli as pm
    real = list((REAL_RECORDS.resolve() / "postmortems").glob("*.md"))
    if not real:  # the first one lands in the shared store only once pm on main reads the type
        pytest.skip("no postmortem in the store yet")
    for p in real:
        prompts = dict(re.findall(r"(?m)^## (.+)\n\n((?:>.*\n)+)", p.read_text()))
        assert {n: v.rstrip("\n") for n, v in prompts.items()} == pm.POSTMORTEM_PROMPTS, p.name


# ---------------------------------------------------------------- pm task add

@pytest.mark.parametrize("args, match", [
    (["--title", "T"], r"the following arguments are required: --sprint"),
    (["--sprint", "demo.1", "--title", " "], r"error: --title is empty"),
    (["--sprint", "demo.9", "--title", "T"], r"error: no sprint record has bead demo.9"),
    (["--sprint", "demo.1.1", "--title", "T"], r"error: no sprint record has bead demo.1.1"),
])
def test_task_add_refuses(repo, args, match):
    refused(repo, "task", "add", *args, stdin="Why.", match=match)


def test_task_add_refuses_closed_sprint(repo):
    repo.set_issue("demo.2", status="closed")
    refused(repo, "task", "add", "--sprint", "demo.2", "--title", "T", match=r"error: sprint demo.2 is closed")


def test_task_add_refuses_sprint_outside_projects(repo):
    repo.set_issue("demo.1", parent=None)
    refused(repo, "task", "add", "--sprint", "demo.1", "--title", "T",
            match=r"error: .*sprints/demo-1: no project record found")


def test_task_add_refuses_non_epic_sprint(repo):
    repo.set_issue("demo.1", issue_type="task")
    refused(repo, "task", "add", "--sprint", "demo.1", "--title", "T", match=r"error: sprint demo.1 is not a Beads epic")


def test_task_add_creates_task_in_sprint(repo):
    res = repo.pm("task", "add", "--sprint", "demo.1", "--title", "Write the parser", stdin="Small and fast.\n")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=task", "--parent=demo.1", "--title=Write the parser",
                                 "--description=Small and fast.", "--json"]]
    assert "created task demo.1.3 in sprint demo.1" in res.stdout
    assert repo.issues()["demo.1.3"]["parent"] == "demo.1"
    assert repo.git("status", "--porcelain") == ""
    res = repo.pm("task", "add", "--sprint", "demo.2", "--title", "No description")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes()[-1] == ["create", "--type=task", "--parent=demo.2", "--title=No description", "--json"]


# ---------------------------------------------------------------- pm task close

def add_task(repo, issue_id="demo.1.3", **fields):
    issues = json.loads(repo.state.read_text()) + [dict(
        {"id": issue_id, "title": "Write the parser", "status": "in_progress", "issue_type": "task",
         "parent": "demo.1", "created_at": "2026-10-01T12:00:00Z", "started_at": "2026-10-01T12:00:00Z"}, **fields)]
    repo.state.write_text(json.dumps(issues))


@pytest.mark.parametrize("task_id, match", [
    ("demo.9", r"error: demo.9 is not a Beads issue"),
    ("demo.1", r"error: demo.1 is an epic; close a sprint with pm sprint close"),
    ("demo.1.1", r"error: task demo.1.1 is already closed"),
    ("demo.1.2", r"error: demo.1.2 is labelled human, so it is a need; answer a decision with pm decision add "
                 r"--need demo.1.2 \(or pm decision close if the answer sets no rule\), close an action with "
                 r"pm action done demo.1.2"),
])
def test_task_close_refuses(repo, task_id, match):
    refused(repo, "task", "close", task_id, "--reason", "Done.", match=match)


def test_task_close_refuses_unknown_commit(repo):
    add_task(repo)
    refused(repo, "task", "close", "demo.1.3", "--commit", "nope", match=r"error: --commit nope is not a commit")


def test_task_close_names_head(repo):
    add_task(repo)
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    res = repo.pm("task", "close", "demo.1.3", "--reason", "Parser written.")
    assert res.returncode == 0, res.stderr
    assert res.stderr == ""
    assert repo.bd_writes() == [["close", "demo.1.3", f"--reason=Parser written. (commit {head})"]]
    assert repo.issues()["demo.1.3"]["status"] == "closed"


def test_task_close_names_given_commit_and_warns_on_dirty_tree(repo):
    add_task(repo)
    first = repo.git("rev-parse", "--short", "HEAD").strip()
    (repo.root / "notes.txt").write_text("later work")
    repo.commit()
    (repo.root / "scratch.txt").write_text("uncommitted")
    res = repo.pm("task", "close", "demo.1.3", "--commit", first)
    assert res.returncode == 0, res.stderr
    assert "warning: the working tree has uncommitted changes" in res.stderr
    assert repo.bd_writes() == [["close", "demo.1.3", f"--reason=Done (commit {first})"]]


def test_task_close_warns_without_commit_since_start(repo):
    add_task(repo, started_at="2099-01-01T00:00:00Z")
    res = repo.pm("task", "close", "demo.1.3", "--reason", "Nothing to commit.")
    assert res.returncode == 0, res.stderr
    assert "warning: no commit since demo.1.3 started, so the reason names none" in res.stderr
    assert repo.bd_writes() == [["close", "demo.1.3", "--reason=Nothing to commit."]]


# ---------------------------------------------------------------- pm task move

MOVE = "The parser waits for the second sprint.\nSprint 1 ships without it; nothing there depends on it.\n"


@pytest.mark.parametrize("args, stdin, match", [
    (["demo.1.3"], MOVE, r"the following arguments are required: --to"),
    (["demo.9", "--to", "demo.2"], MOVE, r"error: demo.9 is not a Beads issue"),
    (["demo.1", "--to", "demo.2"], MOVE, r"error: demo.1 is an epic"),
    (["demo.1.1", "--to", "demo.2"], MOVE, r"error: task demo.1.1 is already closed"),
    (["demo.1.3", "--to", "demo.1"], MOVE, r"error: demo.1.3 is already in sprint demo.1"),
    (["demo.1.3", "--to", "demo.9"], MOVE, r"error: no sprint record has bead demo.9"),
    (["demo.1.3", "--to", "demo.2"], "", r"error: the decision body is empty"),
    (["demo.1.3", "--to", "demo.2"], "Later.\n", r"error: the decision body is a single line"),
])
def test_task_move_refuses(repo, args, stdin, match):
    add_task(repo)
    refused(repo, "task", "move", *args, stdin=stdin, match=match)


def test_task_move_refuses_closed_target(repo):
    add_task(repo)
    repo.set_issue("demo.2", status="closed")
    refused(repo, "task", "move", "demo.1.3", "--to", "demo.2", stdin=MOVE, match=r"error: sprint demo.2 is closed")


def test_task_move_refuses_task_outside_sprints(repo):
    add_task(repo, issue_id="loose", parent="demo")
    refused(repo, "task", "move", "loose", "--to", "demo.2", stdin=MOVE,
            match=r"error: loose is not in a sprint with a record")


def test_task_move_reparents_and_records_decision(repo):
    add_task(repo)
    path = repo.records / "sprints/demo-1.md"
    before = path.read_text()
    res = repo.pm("task", "move", "demo.1.3", "--to", "demo.2", stdin=MOVE)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["update", "demo.1.3", "--parent=demo.2"]]
    assert repo.issues()["demo.1.3"]["parent"] == "demo.2"
    block = f"::: decision {{source=agent date={TODAY}}}\nMoved demo.1.3 to demo.2: {MOVE}:::"
    assert path.read_text() == before.replace("inside this sprint, and why?\n\nNone yet.",
                                              f"inside this sprint, and why?\n\n{block}")
    assert committed(repo, ["pm: moved demo.1.3 from demo.1 to demo.2 and added a sprint decision to "
                            "records/sprints/demo-1.md"])
    assert repo.pm("render").returncode == 0


def test_task_move_prints_undo_when_write_fails(repo):
    add_task(repo)
    path = repo.records / "sprints/demo-1.md"
    path.parent.chmod(0o555)
    try:
        res = repo.pm("task", "move", "demo.1.3", "--to", "demo.2", stdin=MOVE)
    finally:
        path.parent.chmod(0o755)
    assert res.returncode != 0
    assert "undo the Beads step with: bd update demo.1.3 --parent=demo.1" in res.stderr
    assert "Moved" not in path.read_text()


def test_task_move_out_of_closed_sprint(repo):
    add_task(repo)
    repo.set_issue("demo.1", status="closed")
    res = repo.pm("task", "move", "demo.1.3", "--to", "demo.2", stdin=MOVE)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["update", "demo.1.3", "--parent=demo.2"]]
    assert "Moved demo.1.3 to demo.2" in (repo.records / "sprints/demo-1.md").read_text()


def test_day_page_links_sprints_to_their_records(repo):
    assert repo.pm("render").returncode == 0
    page = (repo.root / "site/days/2026-10-01.html").read_text()
    sprints = page.split('id="sprints"')[1]
    assert '<a href="../sprints/demo-1.html">' in sprints
    needs = page.split('id="decisions-await-you"')[1].split('id="actions-await-you"')[0]
    assert '<a href="../sprints/demo-1.html">' in needs


# ---------------------------------------------------------------- the store: pm setup

GIT_ENV = dict(__import__("os").environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")


def git_in(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, check=True, capture_output=True, text=True).stdout


def setup_in(cwd, *args):
    """pm setup with the fake bd, whose calls land in <tmp>/bd.log next to the clone."""
    return subprocess.run([*PM, "setup", *args], cwd=cwd,
                          env=fake_bd_env(cwd.parent, GIT_ENV),
                          capture_output=True, text=True)


def schedule_line(clone: Path, home: Path, state: str) -> str:
    """pm where's schedule line for a clone, on this platform's scheduler as the fakes present it."""
    name = f"local.pm-push.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.timer")
    return f"schedule  {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


def sched_calls(tmp_path) -> list[list[str]]:
    log = tmp_path / "sched.log"
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def bd_writes_in(tmp_path):
    """The bd calls that change something: not the --json reads (the bootstrap dry run, bd context)."""
    calls = [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    return [c for c in calls if "--json" not in c]


@pytest.fixture
def origin(tmp_path):
    """A clone's origin as this repo was cut over: records split onto the records branch, then untracked on main."""
    o = tmp_path / "origin"
    (o / "records/sprints").mkdir(parents=True)
    (o / "records/sprints/demo-1.md").write_text("one\n")
    git_in(tmp_path, "init", "-q", "-b", "main", str(o))
    git_in(o, "add", "-A")
    git_in(o, "commit", "-qm", "records")
    git_in(o, "subtree", "split", "--prefix=records", "-b", "records")
    (o / ".gitignore").write_text("/records\n")
    write_config(o)
    git_in(o, "rm", "-rq", "records")
    git_in(o, "add", ".gitignore", ".pm")
    git_in(o, "commit", "-qm", "records live on the records branch")
    return o


def test_setup_on_fresh_clone_checks_out_store_and_links_records(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert bd_writes_in(tmp_path) == [["bootstrap", "--yes"], ["config", "set", "agent.profile", "team-maintainer"],
                                     ["hooks", "install", "--beads"]]
    assert git_in(clone, "config", "core.hooksPath").strip() == str(clone / ".beads/hooks")
    assert git_in(clone, "config", "beads.role").strip() == "maintainer"
    assert (clone / ".beads").stat().st_mode & 0o777 == 0o700
    assert git_in(clone / ".pm/store/records", "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (clone / "records").is_symlink() and (clone / "records/sprints/demo-1.md").read_text() == "one\n"
    assert git_in(clone, "status", "--porcelain") == ""
    config = (clone / ".git/config").read_text()
    where = subprocess.run([*PM, "where"], cwd=clone,
                           env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True).stdout.splitlines()
    assert where[0] == f"store     {clone}/.pm/store/records  branch records, 0 ahead, 0 behind origin/records (as of the last fetch)"
    assert where[1] == f"checkout  {clone}  branch main, records link set up"
    assert schedule_line(clone, tmp_path / "home", "installed") in where
    assert f"push      {clone}/.pm/run/push.log  no push recorded yet" in where
    installs = [c for c in sched_calls(tmp_path) if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]]
    assert len(installs) == 1, sched_calls(tmp_path)
    if sys.platform == "darwin":
        plist = __import__("plistlib").loads(Path(installs[0][3]).read_bytes())
        assert plist["ProgramArguments"] == [str(clone / "bin/pm"), "push"] and plist["WorkingDirectory"] == str(clone)
        assert plist["StartInterval"] == 600 and plist["StandardErrorPath"] == str(clone / ".pm/run/push.log")
        assert plist["StandardOutPath"] == "/dev/null", "pm push writes its own log lines"
    assert f"installed the push schedule: " in res.stdout
    again = setup_in(clone)
    assert again.returncode == 0 and again.stdout.startswith("already set up"), again.stderr
    assert bd_writes_in(tmp_path) == [["bootstrap", "--yes"], ["config", "set", "agent.profile", "team-maintainer"],
                                     ["hooks", "install", "--beads"]], "no bd change"
    assert len([c for c in sched_calls(tmp_path) if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]]) == 1
    assert (clone / ".git/config").read_text() == config


def test_where_before_setup_names_what_is_missing(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    res = subprocess.run([*PM, "where"], cwd=clone,
                         env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines() == [
        f"store     {clone}/.pm/store/records  missing; run bin/pm setup",
        f"checkout  {clone}  branch main, no records link; run bin/pm setup",
        f"beads     {clone}/.beads  no database; run bin/pm setup",
        "hooks     core.hooksPath unset; run bin/pm setup",
        f"codex     {tmp_path}/codex  missing, so Codex is not used here",
        schedule_line(clone, tmp_path / "home", "missing; run bin/pm setup"),
        f"push      {clone}/.pm/run/push.log  no push recorded yet",
        f"site      http://localhost:8000 (served by pm serve); pm render writes {clone}/site",
    ]
    res = subprocess.run([*PM, "where", "records"], cwd=clone,
                         env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True)
    assert res.returncode != 0 and "error: no records store at" in res.stderr


def test_setup_site_url_sets_keeps_replaces_and_clears_site_url_in_config(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    before = (clone / ".pm/config.toml").read_text()

    def stored():
        return tomllib.loads((clone / ".pm/config.toml").read_text()).get("site_url", "")

    def site_line():
        out = subprocess.run([*PM, "where"], cwd=clone,
                             env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True).stdout
        return next(l for l in out.splitlines() if l.startswith("site "))

    res = setup_in(clone, "--site-url", "https://pm.example.com/")
    assert res.returncode == 0, res.stderr
    assert "site URL set to https://pm.example.com" in res.stdout
    assert stored() == "https://pm.example.com"
    assert site_line().split()[1] == "https://pm.example.com"
    again = setup_in(clone, "--site-url", "https://pm.example.com")
    assert again.returncode == 0 and "site URL" not in again.stdout
    assert "site URL" not in setup_in(clone).stdout and stored() == "https://pm.example.com"
    for bad in ("pm.example.com", "ftp://pm.example.com", "https://pm.example.com/sub", "https://"):
        res = setup_in(clone, "--site-url", bad)
        assert res.returncode != 0 and "not an http(s) base URL" in res.stderr + res.stdout, bad
    assert stored() == "https://pm.example.com"
    res = setup_in(clone, "--site-url", "http://other.example.com:8080")
    assert "set to http://other.example.com:8080 (was https://pm.example.com)" in res.stdout
    res = setup_in(clone, "--site-url", "")
    assert res.returncode == 0 and "site URL cleared" in res.stdout
    assert stored() == "" and site_line().split()[1] == "http://localhost:8000"
    assert (clone / ".pm/config.toml").read_text() == before  # every other line kept as it was
    assert subprocess.run(["git", "config", "--get-regexp", "^pm\\."], cwd=clone, capture_output=True, text=True).stdout == ""


def test_setup_excludes_mains_tracked_copy(tmp_path, origin):
    """Once main carries the copy (subtree add, as the Action does), setup hides it so records/ can be the link."""
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    git_in(clone, "subtree", "add", "--prefix=records", "origin/records", "-m", "copy records")
    where = subprocess.run([*PM, "where"], cwd=clone,
                           env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True).stdout.splitlines()
    assert where[1] == f"checkout  {clone}  branch main, records/ is main's tracked copy, not the link; run bin/pm setup"
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert "sparse checkout" in res.stdout
    assert (clone / "records").is_symlink()
    assert git_in(clone, "status", "--porcelain") == ""
    assert git_in(clone / ".pm/store/records", "status", "--porcelain") == ""


def test_link_survives_main_starting_to_track_its_copy(tmp_path, origin):
    """Set up before main has records/; the first copy then arrives by pull and must not replace the link."""
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    assert setup_in(clone).returncode == 0
    git_in(origin, "subtree", "add", "--prefix=records", "records", "-m", "copy records")
    git_in(clone, "pull", "-q", "--no-rebase")
    assert git_in(clone, "ls-files", "records") == "records/sprints/demo-1.md\n"
    assert (clone / "records").is_symlink()
    assert git_in(clone, "status", "--porcelain") == ""


def test_setup_refuses_a_bootstrap_that_does_not_clone_the_remote(tmp_path, origin):
    """No refs/dolt/data reachable: bd would import a stale JSONL or mint a database; setup refuses instead."""
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    res = subprocess.run([*PM, "setup"], cwd=clone,
                         env=dict(fake_bd_env(tmp_path, GIT_ENV), FAKE_BD_BOOTSTRAP="init"), capture_output=True, text=True)
    assert res.returncode != 0
    assert "error: bd bootstrap would init, not clone the remote's refs/dolt/data: fake" in res.stderr
    assert bd_writes_in(tmp_path) == [] and not (clone / ".pm/store/records").exists()


def test_setup_refuses_without_records_branch(tmp_path):
    git_in(tmp_path, "init", "-q", "-b", "main", "empty")
    write_config(tmp_path / "empty")
    res = setup_in(tmp_path / "empty")
    assert res.returncode != 0
    assert "error: no records branch here or on origin" in res.stderr
    assert not (tmp_path / "empty/.pm/store/records").exists()


# ---------------------------------------------------------------- pm setup: Codex's sandbox roots

CODEX_CONFIG = """# my settings
model = "gpt-5.5"

[projects."/somewhere"]
trust_level = "trusted"  # keep this comment
"""


@pytest.fixture
def clone(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    return tmp_path / "clone"


def codex_roots(clone):
    """The five paths pm needs, worked out apart from pm: the clone's .git, its store, the store's own git dir
    (git names it after the directory), the Beads dir and uv's cache."""
    uv = subprocess.run(["uv", "--color", "never", "cache", "dir"], check=True, capture_output=True, text=True)
    paths = (clone / ".git", clone / ".pm/store/records", clone / ".git/worktrees/records", clone / ".beads",
             Path(uv.stdout.strip()))
    return ", ".join(json.dumps(str(p.resolve())) for p in paths)


def where_in(clone):
    return subprocess.run([*PM, "where"], cwd=clone,
                          env=fake_bd_env(clone.parent, GIT_ENV), capture_output=True, text=True).stdout


@pytest.mark.parametrize("before", [None, CODEX_CONFIG, CODEX_CONFIG.rstrip("\n")],
                         ids=["no-file", "file", "no-final-newline"])
def test_setup_adds_codex_roots_in_a_new_table(clone, before):
    config = clone.parent / "codex/config.toml"
    config.parent.mkdir()
    if before is not None:
        config.write_text(before)
        config.chmod(0o600)
    assert f"codex     {config}  writable_roots not checked without the store; run bin/pm setup" in where_in(clone)
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert f"added to sandbox_workspace_write.writable_roots in {config}" in res.stdout
    table = f"[sandbox_workspace_write]\nwritable_roots = [{codex_roots(clone)}]\n"
    assert config.read_text() == (table if before is None else CODEX_CONFIG + "\n" + table)
    if before is not None:
        assert config.stat().st_mode & 0o777 == 0o600
    assert f"codex     {config}  writable_roots set" in where_in(clone)


def test_setup_adds_codex_roots_to_an_existing_table(clone):
    config = clone.parent / "codex/config.toml"
    config.parent.mkdir()
    table = ('[sandbox_workspace_write]  # mine\nnetwork_access = true\nwritable_roots = [\n  "/opt/data",  # data\n]\n'
             '\n[tui]\nstatus_line = ["model"]\n')
    config.write_text(CODEX_CONFIG + table)
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert config.read_text() == CODEX_CONFIG + table.replace("writable_roots = [", f"writable_roots = [{codex_roots(clone)}, ")
    assert len(tomllib.loads(config.read_text())["sandbox_workspace_write"]["writable_roots"]) == 6


def test_setup_adds_codex_roots_under_a_table_without_them(clone):
    config = clone.parent / "codex/config.toml"
    config.parent.mkdir()
    config.write_text(CODEX_CONFIG + "[sandbox_workspace_write]\nnetwork_access = true\n")
    assert setup_in(clone).returncode == 0
    assert config.read_text() == (CODEX_CONFIG + f"[sandbox_workspace_write]\nwritable_roots = [{codex_roots(clone)}]\n"
                                   "network_access = true\n")


def test_setup_adds_only_missing_codex_roots_and_reruns_change_nothing(clone):
    config = clone.parent / "codex/config.toml"
    config.parent.mkdir()
    git_dir = json.dumps(str((clone / ".git").resolve()))
    config.write_text(f"[sandbox_workspace_write]\nwritable_roots = [{git_dir}]\n")
    assert setup_in(clone).returncode == 0
    roots = codex_roots(clone)
    assert config.read_text() == f"[sandbox_workspace_write]\nwritable_roots = [{roots.split(', ', 1)[1]}, {git_dir}]\n"
    after = config.read_bytes()
    again = setup_in(clone)
    assert again.returncode == 0, again.stderr
    assert again.stdout.startswith("already set up") and "codex" not in again.stdout.lower(), again.stdout
    assert config.read_bytes() == after


def test_setup_without_codex_home_leaves_codex_alone(clone):
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines()[-1] == f"Codex: no {clone.parent}/codex, so Codex is not used here; left its sandbox config alone"
    assert not (clone.parent / "codex").exists()


def test_setup_adds_the_store_to_claude_codes_additional_directories(clone):
    """Keeps the file's other settings; a re-run changes nothing."""
    (clone.parent / "claude").mkdir()
    local = clone / ".claude/settings.local.json"
    local.parent.mkdir()
    local.write_text(json.dumps({"permissions": {"allow": ["Bash(ls:*)"]}, "model": "x"}))
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    store = clone / ".pm/store/records"
    assert (f"added {store} to permissions.additionalDirectories in {local}, so Claude Code writes records through "
            "records/ without asking") in res.stdout.splitlines()
    assert json.loads(local.read_text()) == {"permissions": {"allow": ["Bash(ls:*)"], "additionalDirectories": [str(store)]},
                                             "model": "x"}
    again = setup_in(clone)
    assert again.returncode == 0 and "additionalDirectories" not in again.stdout
    assert json.loads(local.read_text())["permissions"]["additionalDirectories"] == [str(store)]


def test_setup_without_claude_config_dir_leaves_claude_code_alone(clone):
    assert setup_in(clone).returncode == 0
    assert not (clone / ".claude").exists()


@pytest.mark.parametrize("text", ["{not json", '{"permissions": {"additionalDirectories": "/x"}}'],
                         ids=["not-json", "dirs-not-a-list"])
def test_setup_refuses_claude_settings_it_cannot_read(clone, text):
    (clone.parent / "claude").mkdir()
    local = clone / ".claude/settings.local.json"
    local.parent.mkdir()
    local.write_text(text)
    res = setup_in(clone)
    assert res.returncode != 0 and str(local) in res.stderr
    assert local.read_text() == text


@pytest.mark.parametrize("text", ['model = "unterminated\n', '[sandbox_workspace_write]\nwritable_roots = "/x"\n'],
                         ids=["not-toml", "roots-not-a-list"])
def test_setup_refuses_codex_config_it_cannot_read(clone, text):
    config = clone.parent / "codex/config.toml"
    config.parent.mkdir()
    config.write_text(text)
    res = setup_in(clone)
    assert res.returncode != 0
    assert re.search(rf"error: (cannot parse )?{re.escape(str(config))}", res.stderr), res.stderr
    assert config.read_text() == text
    assert f"codex     {config}  unreadable: " in where_in(clone)


# ---------------------------------------------------------------- the store: shared across worktrees

def test_write_on_one_branch_is_visible_on_another_without_merge(repo):
    wt = repo.worktree("feature-x")
    res = repo.pm("finding", "add", "--sprint", "demo.1", "Seen from every branch.", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"])
    assert repo.git("status", "--porcelain", cwd=wt) == ""
    # The main checkout is on main, which merged nothing.
    assert repo.git("rev-parse", "--abbrev-ref", "HEAD").strip() == "main"
    assert repo.git("log", "--format=%s").splitlines() == ["code"]
    assert "- Seen from every branch." in repo.pm("show", "--sprint", "demo.1").stdout
    assert repo.pm("render").returncode == 0
    assert "Seen from every branch." in (repo.root / "site/sprints/demo-1.html").read_text()


def test_concurrent_writes_from_two_worktrees_land_as_separate_commits(repo):
    wts = [repo.worktree("feature-a"), repo.worktree("feature-b")]
    procs = [subprocess.Popen([*PM, "finding", "add", "--sprint", "demo.1",
                               f"From {wt.name}."], cwd=wt, env=repo.env, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, text=True) for wt in wts]
    for p in procs:
        assert p.wait() == 0, p.stderr.read()
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"] * 2)
    assert repo.store_log()[2] == "records"
    text = (repo.store / "sprints/demo-1.md").read_text()
    assert "- From feature-a." in text and "- From feature-b." in text
    for rev in ("HEAD", "HEAD~1"):
        assert repo.git("show", "--name-only", "--format=", rev, cwd=repo.store).split() == ["sprints/demo-1.md"]


def test_a_write_with_open_stdin_does_not_block_other_writes(repo):
    """Sprint 23 finding: pm action need, run with stdin left open, read stdin while holding the store lock and
    blocked every other session's write for minutes. pm reads stdin before it takes the lock."""
    held = subprocess.Popen([*PM, "decision", "need", "--title", "Q", "--parent", "demo.1"],
                            cwd=repo.root, env=repo.env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True)
    try:
        res = subprocess.run([*PM, "finding", "add", "--sprint", "demo.1", "Not blocked."],
                             cwd=repo.root, env=repo.env, capture_output=True, text=True, timeout=60)
        assert res.returncode == 0, res.stderr
        assert held.poll() is None, "the first write still waits for its stdin"
        out, err = held.communicate(input=NEED, timeout=60)
    finally:
        held.kill()
    assert held.returncode == 0, err
    assert "raised decision need demo.1.3" in out


def test_need_answered_on_another_branch_does_not_block_writes(repo):
    """Sprint 9 finding: a need closed in Beads whose citing decision was committed on another branch made pm
    and render refuse everywhere else. With one store, the decision is where every branch reads."""
    wt = repo.worktree("feature-x")
    res = repo.pm("decision", "add", "--need", "demo.1.2", "--level", "sprint", "--sprint", "demo.1", stdin=BODY,
                  cwd=wt)
    assert res.returncode == 0, res.stderr
    assert repo.issues()["demo.1.2"]["status"] == "closed"
    assert repo.git("ls-files", cwd=wt) == ".gitignore\n.pm/.gitignore\n.pm/config.toml\n", "the decision is not on the code branch"
    res = repo.pm("task", "add", "--sprint", "demo.1", "--title", "Next step")
    assert res.returncode == 0, res.stderr
    assert repo.pm("render").returncode == 0


def test_hand_edit_is_committed_with_pm_commit(repo):
    assert repo.pm("where", "records").stdout == f"{repo.store}\n"
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("Ship it.", "Ship it soon."))
    res = repo.pm("commit", "-m", "Sharpen the sprint 1 goal", "records/sprints/demo-1.md")
    assert res.returncode == 0, res.stderr
    assert "committed records/sprints/demo-1.md as " in res.stdout
    assert committed(repo, ["Sharpen the sprint 1 goal"])
    refused(repo, "commit", "-m", "Nothing", "records/sprints/demo-1.md", match=r"error: nothing to commit in .*\.pm/store/records")


def test_commit_refuses_hand_edit_that_does_not_render(repo):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("## Findings", "## Finds"))
    refused(repo, "commit", "-m", "Break it", "records/sprints/demo-1.md",
            match=r"error: sprints/demo-1: sprint record needs a '## Findings'")
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n"


def test_only_render_and_commit_check_records_a_command_does_not_touch(repo):
    """A broken record on the records branch stops render and commit, and show of that sprint, but not writes to other
    records or show of another sprint: those validate only what they touch."""
    path = repo.store / "sprints/demo-2.md"
    path.write_text(path.read_text().replace("## Findings", "## Finds"))
    repo.commit("Break sprint 2 by hand")
    broken = r"sprints/demo-2: sprint record needs a '## Findings'"
    for args, stdin in ((["finding", "add", "--sprint", "demo.1", "Seen."], ""),
                        (["task", "add", "--sprint", "demo.1", "--title", "T"], "Small.\n"),
                        (["show", "--sprint", "demo.1"], ""), (["show"], "")):
        res = repo.pm(*args, stdin=stdin)
        assert res.returncode == 0, (args, res.stderr)
    goal = repo.store / "sprints/demo-1.md"
    goal.write_text(goal.read_text().replace("Ship it.", "Ship it soon."))
    for args in (["show", "--sprint", "demo.2"], ["render"], ["commit", "-m", "Goal", "records/sprints/demo-1.md"]):
        res = repo.pm(*args)
        assert res.returncode == 1 and re.search(broken, res.stderr), (args, res.stderr)
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n"


def test_commit_commits_only_its_callers_records_beside_another_sessions_edit(repo):
    """Two sessions share the store: A has an edit in progress, B commits its own. B's commit holds B's file only,
    and pm commit with no path lists both instead of sweeping A's edit in."""
    a = repo.store / "sprints/demo-1.md"
    a.write_text(a.read_text().replace("Ship it.", "Ship it, session A."))
    (repo.store / "docs").mkdir()
    new = repo.store / "docs/scratch.md"
    new.write_text("session A, not a record yet")
    b = repo.store / "projects/demo.md"
    b.write_text(b.read_text().replace("A demo project.", "A demo project, session B."))
    wt = repo.worktree("session-b")
    res = refused(repo, "commit", "-m", "Session B", match=r"error: name the records you edited: pm commit")
    for line in (" M records/projects/demo.md", " M records/sprints/demo-1.md", "?? records/docs/scratch.md"):
        assert line in res.stderr
    refused(repo, "commit", "-m", "Session B", "records/sprints/demo-2.md", match=r"error: records/sprints/demo-2.md has no uncommitted change")
    refused(repo, "commit", "-m", "Session B", str(repo.root / ".gitignore"), match=r"is not in the records store")
    # A's half-written file does not render, but B's commit is checked as the records branch will hold it.
    res = repo.pm("commit", "-m", "Session B", "records/projects/demo.md", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert repo.git("show", "--name-only", "--format=%s", "HEAD", cwd=repo.store).split("\n")[:3] == [
        "Session B", "", "projects/demo.md"]
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n?? docs/\n"
    assert "session A" in a.read_text()


@pytest.mark.parametrize("name", ["records", "records/", "STORE"])
def test_commit_refuses_the_store_itself(repo, name):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("Ship it.", "Ship it soon."))
    res = refused(repo, "commit", "-m", "Everything", str(repo.store) if name == "STORE" else name,
                  match=r"error: .* is the records store itself; name the records you edited")
    assert "Traceback" not in res.stderr


def design_page(project: str) -> str:
    sections = ["Problem", "Goals and non-goals", "Constraints and key facts", "Design", "Alternatives considered",
                "Open questions"]
    return (f"---\ntype: design\ntitle: X\nproject: {project}\n---\n\n"
            + "".join(f"## {name}\n\nNone yet.\n\n" for name in sections))


def test_commit_and_write_refuse_what_depends_on_another_sessions_uncommitted_record(repo):
    """What is checked is the records branch after the commit: a record naming a project that exists only as
    another session's uncommitted file is refused until that file is committed too."""
    issues = json.loads(repo.state.read_text()) + [
        {"id": "other", "title": "Other", "status": "open", "issue_type": "epic", "created_at": "2026-10-01T12:00:00Z"}]
    repo.state.write_text(json.dumps(issues))
    (repo.store / "projects/other.md").write_text(project("Other", "other"))
    (repo.store / "design").mkdir()
    (repo.store / "design/x.md").write_text(design_page("other"))
    assert repo.pm("render").returncode == 0, "the working store renders"
    refused(repo, "commit", "-m", "Design", "records/design/x.md", match=r"error: design/x: no project record found")
    refused(repo, "design", "new", "y", "--title", "Y", "--project", "other",
            match=r"error: no project record named 'other'")
    res = repo.pm("commit", "-m", "Other and its design", "records/projects/other.md", "records/design/x.md")
    assert res.returncode == 0, res.stderr
    assert repo.pm("design", "new", "y", "--title", "Y", "--project", "other").returncode == 0


def test_write_refuses_a_target_with_uncommitted_changes(repo):
    """Another session's edit in progress to the record a pm write targets would ride along in that write's
    commit; the write refuses before any Beads step and says how to settle the edit."""
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("Ship it.", "Ship it, session A."))
    res = refused(repo, "finding", "add", "--sprint", "demo.1", "Mine.",
                  match=r"error: records/sprints/demo-1.md has uncommitted changes")
    assert 'pm commit -m "…" records/sprints/demo-1.md' in res.stderr and "revert" in res.stderr
    refused(repo, "task", "move", "demo.1.2", "--to", "demo.2", stdin="Moved on.\nIt fits sprint 2.",
            match=r"error: records/sprints/demo-1.md has uncommitted changes")
    assert "session A" in path.read_text()


def test_write_ignores_another_sessions_broken_uncommitted_records(repo):
    """A pm write is checked against the records branch plus its own change, so another session's broken edit in
    progress neither blocks it nor lands in its commit."""
    a = repo.store / "sprints/demo-1.md"
    a.write_text(a.read_text().replace("## Findings", "## Finds"))
    (repo.store / "docs").mkdir()
    (repo.store / "docs/scratch.md").write_text("session A, not a record yet")
    assert repo.pm("render").returncode != 0, "the working store does not render"
    res = repo.pm("finding", "add", "--sprint", "demo.2", "Mine.")
    assert res.returncode == 0, res.stderr
    assert repo.git("show", "--name-only", "--format=", "HEAD", cwd=repo.store).split() == ["sprints/demo-2.md"]
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n?? docs/\n"
    res = repo.pm("task", "add", "--sprint", "demo.1", "--title", "Next step")  # a write that changes no record
    assert res.returncode == 0, res.stderr


def store_gitdir(repo) -> Path:
    return Path(repo.git("rev-parse", "--absolute-git-dir", cwd=repo.store).strip())


def test_failed_commit_restores_the_write_and_leaves_other_edits(repo):
    """A pm write whose commit fails puts its file back, so no later pm commit sweeps it in; another session's
    edit in progress is untouched."""
    a = repo.store / "projects/demo.md"
    a.write_text(a.read_text().replace("A demo project.", "A demo project, session A."))
    path = repo.store / "sprints/demo-1.md"
    before = path.read_text()
    lock = store_gitdir(repo) / "index.lock"
    lock.write_text("")
    try:
        res = repo.pm("finding", "add", "--sprint", "demo.1", "Lost on purpose.")
    finally:
        lock.unlink()
    assert res.returncode != 0
    assert "error: committing failed" in res.stderr
    assert "restored records/sprints/demo-1.md to the state before this write" in res.stderr
    assert path.read_text() == before
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M projects/demo.md\n"
    assert repo.store_log()[0] == "records"


def test_failed_commit_removes_a_new_record(repo):
    lock = store_gitdir(repo) / "index.lock"
    lock.write_text("")
    try:
        res = repo.pm("doc", "new", "probe", "--title", "Probe", "--project", "demo", stdin="Body.")
    finally:
        lock.unlink()
    assert res.returncode != 0 and "restored records/docs/" in res.stderr
    assert repo.git("status", "--porcelain", cwd=repo.store) == ""


def test_missing_store_fails_hard_without_falling_back(repo):
    repo.git("worktree", "remove", "--force", ".pm/store/records")
    repo.records.unlink()
    shutil.copytree(Path(__file__).parent, repo.records)  # a plain records/ in the worktree is never read
    for args in (["show"], ["where", "records"], ["render"]):
        res = repo.pm(*args)
        assert res.returncode != 0
        assert re.search(r"error: no records store at .*/repo/\.pm/store/records; set it up with bin/pm setup", res.stderr)
    res = subprocess.run([*RENDER, "site"], cwd=repo.root, env=repo.env,
                         capture_output=True, text=True)
    assert res.returncode != 0 and "error: no records store at" in res.stderr


def test_where_lists_every_location_with_its_state(repo):
    """Set up: store with its sync state, this checkout and its link, Beads, the hooks and the site."""
    (repo.root / ".beads/embeddeddolt").mkdir(parents=True)
    hooks = repo.root / ".beads/hooks"
    hooks.mkdir()
    for h in ("post-checkout", "pre-commit"):
        (hooks / h).write_text("#!/bin/sh\n")
        (hooks / h).chmod(0o755)
    repo.git("config", "core.hooksPath", ".beads/hooks")
    wt = repo.worktree("feature-x")
    res = repo.pm("where", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines() == [
        f"store     {repo.store}  branch records, no origin/records",
        f"checkout  {wt}  branch feature-x, no records link; run bin/pm setup",
        f"beads     {repo.root}/.beads  database demo, remote git+https://example.com/demo.git; ahead/behind not reported by bd; agent profile conservative; run bin/pm setup",
        f"hooks     {hooks}  post-checkout installed, pre-commit installed",
        f"codex     {repo.root.parent}/codex  missing, so Codex is not used here",
        schedule_line(repo.root, repo.root.parent / "home", "missing; run bin/pm setup"),
        f"push      {repo.root}/.pm/run/push.log  no push recorded yet",
        f"site      http://localhost:8000 (served by pm serve); pm render writes {wt}/site",
    ]
    (hooks / "pre-commit").unlink()
    assert repo.pm("where").stdout.splitlines()[1:4:2] == [
        f"checkout  {repo.root}  branch main, records link set up",
        f"hooks     {hooks}  post-checkout installed, pre-commit missing"]


def test_store_on_wrong_branch_is_refused(repo):
    repo.git("checkout", "-q", "-b", "other", cwd=repo.store)
    res = repo.pm("show")
    assert res.returncode != 0
    assert re.search(r"error: .*\.pm/store/records is not a worktree on branch records", res.stderr)


SERVE_BEHIND = 10  # seconds a served page may be behind, as pm/site.py has it


def asof(page: str) -> float:
    """The time, in epoch seconds, a page served by pm serve states its data is current as of."""
    m = re.search(r'<p class="asof[^"]*" data-asof="([\d.]+)"', page)
    assert m, "the page states no age"
    return float(m.group(1))


def until_shown(get, shown, written: float | None) -> str:
    """Load get() until shown(page), which must happen within SERVE_BEHIND s of `written`, the epoch time a change
    was done; a page that does not show it yet must state data from before the change. None for a change the server
    makes in the background, whose time the test does not know: then only the bound from now applies."""
    deadline = (written or time.time()) + SERVE_BEHIND
    while True:
        page = get()
        if shown(page):
            return page
        assert written is None or asof(page) < written, "the page states data as of after a change it does not show"
        assert time.time() < deadline, "the change did not show within the stated bound"
        time.sleep(0.05)


def load(url: str) -> str:
    """The page at `url`, whatever its status."""
    import urllib.error
    try:
        with urllib.request.urlopen(url) as r:
            return r.read().decode()
    except urllib.error.HTTPError as e:
        return e.read().decode()


def without_status(page: str) -> str:
    return re.sub(r'<p class="asof.*?</script>', "", page, flags=re.S)


def test_serve_shows_each_change_within_its_stated_age(repo):
    """pm serve shows a pm write and a Beads change within SERVE_BEHIND s, every page stating data from before a
    change it does not show yet, and a failing render as the error."""
    import urllib.error
    import urllib.request

    repo.dolt()
    srv = subprocess.Popen([*PM, "serve"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        url = re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0)

        def get(path):
            try:
                with urllib.request.urlopen(f"{url}/{path}") as r:
                    return r.status, r.headers["Cache-Control"], r.read().decode()
            except urllib.error.HTTPError as e:
                return e.code, e.headers["Cache-Control"], e.read().decode()

        assert "Served fresh." not in get("sprints/demo-1.html")[2]
        assert repo.pm("finding", "add", "Served fresh.", "--sprint", "demo.1").returncode == 0
        until_shown(lambda: get("sprints/demo-1.html")[2], lambda p: "Served fresh." in p, time.time())
        assert get("sprints/demo-1.html")[:2] == (200, "no-store")

        assert "1 of 2 tasks done" in get("")[2]
        repo.set_issue("demo.1.1", status="open")
        until_shown(lambda: get("")[2], lambda p: "0 of 2 tasks done" in p, time.time())
        assert get("style.css")[0] == 200 and get("nope.html")[0] == 404

        repo.set_issue("demo.1.2", status="closed")  # an answered need no decision cites: the render fails
        until_shown(lambda: get("")[2], lambda p: "no decision cites it" in p, time.time())
        code, _, body = get("")
        assert code == 500 and "need demo.1.2 (Ask the owner) is closed but no decision cites it" in body
    finally:
        srv.terminate()
        srv.wait()



def test_serve_reuses_a_page_only_while_records_and_beads_are_unchanged(repo):
    """Nothing changed, no bd runs and a reload serves the same page; a records edit (even a second edit of an
    already uncommitted file) shows without a bd call, and a Beads write, through bd or not, with one."""
    import urllib.request

    repo.dolt()
    srv = subprocess.Popen([*PM, "serve"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        url = re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0)

        def get(path):
            with urllib.request.urlopen(f"{url}/{path}") as r:
                return r.read().decode()

        def lists():
            return sum(1 for c in repo.bd_calls() if c[:1] == ["list"])

        first, n = get("sprints/demo-1.html"), lists()
        time.sleep(1.2)  # the server's looks in the meantime find nothing changed
        assert without_status(get("sprints/demo-1.html")) == without_status(first) and lists() == n
        assert "1 of 2 tasks done" in get("")

        sprint = repo.records / "sprints/demo-1.md"
        sprint.write_text(sprint.read_text().replace("Ship it.", "Ship it soon."))
        until_shown(lambda: get("sprints/demo-1.html"), lambda p: "Ship it soon." in p, time.time())
        sprint.write_text(sprint.read_text().replace("Ship it soon.", "Ship it later."))
        until_shown(lambda: get("sprints/demo-1.html"), lambda p: "Ship it later." in p, time.time())
        assert lists() == n

        repo.set_issue("demo.1.1", status="open")
        until_shown(lambda: get(""), lambda p: "0 of 2 tasks done" in p, time.time())
        assert lists() == n + 1
        subprocess.run(["bd", "create", "--title=Third", "--parent=demo.1"], cwd=repo.root, env=repo.env, check=True,
                       capture_output=True)
        until_shown(lambda: get(""), lambda p: "0 of 3 tasks done" in p, time.time())
        assert "Third" in get("sprints/demo-1.html")
    finally:
        srv.terminate()
        srv.wait()

# ---------------------------------------------------------------- pm record link

def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture
def served(repo):
    """pm serve for the repo's store on a free port; the repo's env carries that PORT from here on."""
    repo.dolt()
    srv = subprocess.Popen([*PM, "serve"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        port = re.search(r"http://localhost:(\d+)", srv.stdout.readline()).group(1)
        repo.env = dict(repo.env, PORT=port)
        yield f"http://localhost:{port}"
    finally:
        srv.terminate()
        srv.wait()


def test_record_link_prints_the_served_page_for_every_target_kind(repo, served):
    assert repo.pm("design", "new", "parser", "--title", "Parser", "--project", "demo").returncode == 0
    for target, page in [("demo.1", "sprints/demo-1"), ("demo", "projects/demo"), ("parser", "design/parser"),
                         ("records/sprints/demo-2.md", "sprints/demo-2"), ("days/2026-10-01", "days/2026-10-01"),
                         (str(repo.store / "projects/old.md"), "projects/old")]:
        res = repo.pm("record", "link", target)
        assert res.returncode == 0, res.stderr
        assert res.stdout == f"{served}/{page}.html\n"
        until_shown(lambda: load(res.stdout.strip()), lambda p: "<h1>No page" not in p, time.time())


def test_record_link_refuses_an_unknown_or_ambiguous_target(repo, served):
    refused(repo, "record", "link", "demo.9", match=r"no record matches 'demo.9'; give a sprint id")
    refused(repo, "record", "link", "records/sprints/nope.md", match=r"no record matches")
    assert repo.pm("design", "new", "demo", "--title", "Demo design", "--project", "demo").returncode == 0
    refused(repo, "record", "link", "demo",
            match=r"'demo' names records/design/demo.md and records/projects/demo.md; give the record path")
    assert repo.pm("record", "link", "design/demo").stdout == f"{served}/design/demo.html\n"


def test_record_link_refuses_without_a_server(repo):
    port = free_port()
    repo.env = dict(repo.env, PORT=str(port))
    refused(repo, "record", "link", "demo.1",
            match=rf"no site is served on :{port}; start it with PORT={port} pm serve, then run this again")


def test_record_link_refuses_a_foreign_server(repo, tmp_path):
    """An old static server answers on the port: its pages may be stale, so pm names the fix instead of a link."""
    port = free_port()
    srv = subprocess.Popen([sys.executable, "-m", "http.server", str(port), "--bind", "127.0.0.1"], cwd=tmp_path,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(200):
            try:
                urllib.request.urlopen(f"http://127.0.0.1:{port}/", timeout=1).close()
                break
            except OSError:
                time.sleep(0.05)
        repo.env = dict(repo.env, PORT=str(port))
        refused(repo, "record", "link", "demo.1",
                match=rf"the server on :{port} is not pm serve.*stop the old server on :{port}, then "
                      rf"PORT={port} pm serve")
    finally:
        srv.terminate()
        srv.wait()


def test_show_site_line_has_constant_size(repo):
    """pm show adds one line naming the site and the record-to-page rule; more history does not grow it."""
    def show():
        res = repo.pm("show")
        assert res.returncode == 0, res.stderr
        return res.stdout.splitlines()

    before = show()
    site = [l for l in before if l.startswith("site: ")]
    assert site == ["site: http://localhost:8000 (pm serve); a record's page is <site>/<its path under records/, "
                    "without .md>.html; pm record link <target> prints one"]
    issues = json.loads(repo.state.read_text())
    for n in range(3, 9):
        issues.append({"id": f"demo.{n}", "title": f"Sprint {n}: Past", "status": "closed", "issue_type": "epic",
                       "parent": "demo", "created_at": "2026-10-01T12:00:00Z"})
        repo.write(f"sprints/demo-{n}.md", sprint("Past", f"demo.{n}", outcome="Done: shipped.",
                                                  against="- It works: met."))
        repo.write(f"days/2026-09-0{n}.md", f"---\ntype: day\ndate: 2026-09-0{n}\n---\n\n## Today\n\n"
                                            "> What are we chasing today, and why now?\n\nWork.\n")
    repo.state.write_text(json.dumps(issues))
    repo.commit("history")
    after = show()
    assert len(after) == len(before) and [l for l in after if l.startswith("site: ")] == site
    data = json.loads(repo.pm("show", "--json").stdout)
    assert data["site"] == "http://localhost:8000"
    assert data["projects"][0]["url"] == "http://localhost:8000/projects/demo.html"
    assert data["projects"][0]["sprints"][0]["url"] == "http://localhost:8000/sprints/demo-1.html"


@pytest.mark.parametrize("fields", [
    {"close_reason": "Dismissed: no longer applicable"},
    {"close_reason": "Responded", "labels": ["human", "action"]},
    {"close_reason": "Responded", "labels": ["human", "no-decision"]},
])
def test_render_accepts_needs_closed_by_newer_pm(repo, fields):
    """Beads is shared with newer pm code: its actions, small answers and dismiss-with-note closes need no decision."""
    repo.set_issue("demo.1.2", status="closed", **fields)
    assert repo.pm("render").returncode == 0


# ---------------------------------------------------------------- replies on the site

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def post_reply(url: str, form: dict[str, str], headers: dict[str, str] | None = None) -> tuple[int, str, str]:
    """POST a card's form as the browser does; the status, the Location header and the body."""
    import urllib.error
    import urllib.parse

    req = urllib.request.Request(f"{url}/reply", data=urllib.parse.urlencode(form).encode(), headers=headers or {})
    try:
        with urllib.request.build_opener(NoRedirect).open(req) as r:
            return r.status, r.headers.get("Location", ""), r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("Location", ""), e.read().decode()


def page_token(url: str, page: str = "") -> str:
    with urllib.request.urlopen(f"{url}/{page}") as r:
        m = re.search(r'name="token" value="([^"]+)"', r.read().decode())
    assert m, "the served page has no reply form"
    return m.group(1)


def test_reply_on_a_card_is_stored_on_its_issue_as_one_comment(repo, served):
    """A decision's answer and an action's evidence each land on their own issue, which stays open for the agent;
    each reply is one Beads write, the comment, and no label."""
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION).returncode == 0
    day = until_shown(lambda: urllib.request.urlopen(f"{served}/days/2026-10-01.html").read().decode(),
                      lambda p: p.count('<form class="reply" method="post" action="/reply"') == 2, time.time())
    assert '<input type="hidden" name="id" value="demo.1.2">' in day
    token = page_token(served, "days/2026-10-01.html")
    repo.log.write_text("")

    for issue_id, text in (("demo.1.2", "-Pick small.\r\nBecause tables can wait."), ("demo.1.3", "Merged as abc123.")):
        code, where, _ = post_reply(served, {"token": token, "id": issue_id, "text": text},
                                    {"Referer": f"{served}/days/2026-10-01.html"})
        assert (code, where) == (303, f"/days/2026-10-01.html#need-{issue_id}")
    deadline = time.monotonic() + 10  # the comments are written in the background
    while len(repo.bd_writes()) < 2 and time.monotonic() < deadline:
        time.sleep(0.05)
    issues = repo.issues()
    for issue_id, text in (("demo.1.2", "-Pick small.\nBecause tables can wait."), ("demo.1.3", "Merged as abc123.")):
        assert [(c["author"], reply_body(c["text"])) for c in issues[issue_id]["comments"]] == [("owner (site reply)",
                                                                                                text)]
        assert re.fullmatch(r"(?s).*\n\n<!-- pm-reply [0-9a-f-]{36} -->", issues[issue_id]["comments"][0]["text"])
        assert issues[issue_id]["labels"] == ["human"] + (["action"] if issue_id == "demo.1.3" else [])
        assert issues[issue_id]["status"] == "open"
    assert [c[:3] for c in repo.bd_writes()] == [["comments", "add", "demo.1.2"], ["comments", "add", "demo.1.3"]]
    until_shown(lambda: urllib.request.urlopen(f"{served}/").read().decode(),
                lambda p: p.count('<span class="replied">not delivered') == 2 and "Merged as abc123." in p, None)

    assert repo.pm("render").returncode == 0  # the static site keeps the slot and shows no form, but the replies
    static = (repo.root / "site" / "index.html").read_text()
    assert "<!--pm-reply demo.1.2 decision-->" in static and "<form" not in static
    assert "<p>Merged as abc123.</p>" in static and "<!-- pm-reply" not in static


@contextlib.contextmanager
def serving(repo, **env):
    """pm serve for the repo's store on a free port with `env` added; its URL, then its stderr once it stopped."""
    log = repo.root.parent / "serve.log"
    with log.open("w") as err:
        srv = subprocess.Popen([*PM, "serve"], cwd=repo.root,
                               env=dict(repo.env, PORT="0", **env), stdout=subprocess.PIPE, stderr=err, text=True)
    try:
        yield re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0), log
    finally:
        srv.terminate()
        srv.wait()


def test_a_sent_reply_stays_on_its_card_until_the_snapshot_holds_its_comment():
    """A refresh that finds Dolt unchanged keeps the old Beads read under a newer as_of: the sent note must stay
    until that data holds the reply's comment, or the reply vanishes from the card."""
    stale = {"x.1": {"status": "open", "comment_count": 0}}
    assert not reply_in_beads(stale, "x.1", "r1")  # a newer as_of, but Beads read before the write: keep the note
    other = {"x.1": {"status": "open", "comments": [{"text": "Old.\n" + REPLY_MARK.format("r0")}]}}
    assert not reply_in_beads(other, "x.1", "r1")
    fresh = {"x.1": {"status": "open", "comments": [{"text": "Small.\n" + REPLY_MARK.format("r1")}]}}
    assert reply_in_beads(fresh, "x.1", "r1")
    assert reply_in_beads({"x.1": {"status": "closed"}}, "x.1", "r1") and reply_in_beads({}, "x.1", "r1")


def test_a_reply_returns_before_its_beads_write_and_its_card_shows_saving_then_saved(repo, tmp_path):
    """The POST returns while bd comments add is still held; the card says it is saving, then that it was saved and
    not delivered (the request stores no session inbox), and, once the server has reread Beads, shows the reply from
    Beads as not delivered until it is, then as delivered."""
    repo.dolt()
    gate = tmp_path / "gate"
    with serving(repo, FAKE_BD_HOLD=json.dumps([["comments", "add"], str(gate)])) as (url, _):
        get = lambda: urllib.request.urlopen(f"{url}/").read().decode()
        assert post_reply(url, {"token": page_token(url), "id": "demo.1.2", "text": "Small."})[0] == 303
        assert repo.comments("demo.1.2") == []  # the write is still held
        assert "Saving your reply…</p><p>Small.</p>" in get()  # the owner sees what they sent
        gate.touch()
        until_shown(get, lambda p: "Saving your reply" not in p and ("Reply saved; not delivered" in p
                                                                       or '<span class="replied">not delivered' in p), None)
        assert [reply_body(c) for c in repo.comments("demo.1.2")] == ["Small."]
        page = until_shown(get, lambda p: '<span class="replied">not delivered' in p, None)
        assert "Reply saved;" not in page and page.count("<p>Small.</p>") == 1  # from Beads, not the note too
        subprocess.run(["bd", "update", "demo.1.2", "--set-metadata=picked_up=1"], cwd=repo.root, env=repo.env,
                       check=True, capture_output=True)
        page = until_shown(get, lambda p: "delivered to the agent&#x27;s session" in p, time.time())
        assert "not delivered" not in page and "<p>Small.</p>" in page


def test_a_card_lists_its_comments_oldest_first_escaped_with_author_time_and_delivery():
    """Each comment on an open request shows, oldest first, with its author and time, its HTML escaped and its
    pm-reply mark gone; a site reply within the picked_up count reads as delivered, a later one as not delivered, and
    any other comment (an agent's) carries no delivery state: it is no reply."""
    issue = {"id": "demo.1.2", "status": "open", "labels": ["human"], "comment_count": 3,
             "metadata": {"picked_up": 1},
             "comments": [{"author": "owner (site reply)", "text": "Pick *small*.\n\n<!-- pm-reply r-1 -->",
                           "created_at": "2026-10-03T12:00:00Z"},
                          {"author": "alice", "text": "<script>x()</script>", "created_at": "2026-10-03T12:05:00Z"},
                          {"author": "owner (site reply)", "text": "Or big.", "created_at": "2026-10-03T12:06:00Z"}]}
    out = thread(issue)
    assert re.search(r"you, on the site · 2026-10-03 12:00:00 UTC · <span class=\"picked\">delivered to the agent"
                     r"&#x27;s session</span></p><p>Pick <em>small</em>.</p>", out)
    assert "alice · 2026-10-03 12:05:00 UTC</p>" in out
    assert re.search(r"12:06:00 UTC · <span class=\"replied\">not delivered yet: the session that asked is not running",
                     out)
    assert "&lt;script&gt;" in out and "<script>" not in out and "pm-reply" not in out
    assert out.index("Pick") < out.index("alice")
    assert thread({**issue, "comments": [], "comment_count": 0}) == ""


def test_a_failed_reply_write_shows_on_its_card_with_the_text_back_in_the_box(repo):
    repo.dolt()
    with serving(repo, FAKE_BD_FAIL=json.dumps(["comments", "add"])) as (url, _):
        get = lambda: urllib.request.urlopen(f"{url}/").read().decode()
        assert post_reply(url, {"token": page_token(url), "id": "demo.1.2", "text": "Small <now>."})[0] == 303
        page = until_shown(get, lambda p: "Your reply was not stored: bd comments add demo.1.2" in p, None)
        assert ">Small &lt;now&gt;.</textarea>" in page
        assert repo.comments("demo.1.2") == []


def test_a_failed_reply_write_is_retried_until_it_lands_once(repo, tmp_path):
    """A failed write keeps the reply in the spool and tries again with backoff; once bd works it lands once, and the
    spool is emptied. The failed card's form carries the reply's id, so sending it again unchanged adds nothing."""
    repo.dolt()
    heal = tmp_path / "heal"
    with serving(repo, FAKE_BD_FAIL=json.dumps(["comments", "add"]), FAKE_BD_HEAL=str(heal)) as (url, _):
        get = lambda: urllib.request.urlopen(f"{url}/").read().decode()
        form = {"token": page_token(url), "id": "demo.1.2", "text": "Small.", "rid": "r-1"}
        assert post_reply(url, form)[0] == 303
        page = until_shown(get, lambda p: "Your reply was not stored" in p, None)
        assert '<input type="hidden" name="rid" value="r-1">' in page
        assert post_reply(url, form)[0] == 303  # sent again as it came back: the spool already holds it
        heal.touch()  # the note says sent, or the thread already shows the reply from Beads
        until_shown(get, lambda p: "Reply saved" in p or '<span class="replied">not delivered' in p, None)
    assert repo.comments("demo.1.2") == ["Small.\n\n<!-- pm-reply r-1 -->"]
    assert spool(repo).read_text() == ""


def spool(repo) -> Path:
    return repo.root / ".git" / "pm-replies.jsonl"


def kill_serve(repo, **env):
    """Start pm serve in its own process group; its URL and a function that kills the group with SIGKILL."""
    import signal
    srv = subprocess.Popen([*PM, "serve"], cwd=repo.root,
                           env=dict(repo.env, PORT="0", **env), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                           text=True, start_new_session=True)
    url = re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0)
    return url, lambda: (os.killpg(srv.pid, signal.SIGKILL), srv.wait())


def until(cond) -> None:
    deadline = time.monotonic() + 10
    while not cond():
        assert time.monotonic() < deadline, "not within 10 s"
        time.sleep(0.05)


def test_a_reply_spooled_before_a_crash_is_delivered_at_the_next_start(repo, tmp_path):
    """The POST is answered once the reply is in the spool; a kill before its Beads write loses nothing: the next
    start writes it, once, and the agent reads it without its reply id."""
    repo.dolt()
    url, kill = kill_serve(repo, FAKE_BD_HOLD=json.dumps([["comments", "add"], str(tmp_path / "never")]))
    assert post_reply(url, {"token": page_token(url), "id": "demo.1.2", "text": "Small.", "rid": "r-1"})[0] == 303
    assert [json.loads(l)["rid"] for l in spool(repo).read_text().splitlines()] == ["r-1"]
    until(lambda: ["comments", "add"] in [c[:2] for c in repo.bd_calls()])  # the write started, and is held
    kill()
    assert repo.comments("demo.1.2") == []
    with serving(repo) as (url, _):
        until(lambda: repo.comments("demo.1.2"))
        until(lambda: spool(repo).read_text() == "")
    assert repo.comments("demo.1.2") == ["Small.\n\n<!-- pm-reply r-1 -->"]
    res = repo.pm("reply", "read", "demo.1.2")
    assert res.returncode == 0, res.stderr
    assert "  [2026-10-03T12:00:00Z] Small.\nnext:" in res.stdout and "pm-reply" not in res.stdout


def test_a_reply_written_before_a_crash_is_not_written_again(repo):
    """A kill between the Beads write and the done mark leaves a pending entry whose comment exists: the next start
    finds the comment by its reply id, marks the entry done and writes nothing."""
    repo.dolt()
    spool(repo).write_text(json.dumps({"rid": "r-1", "id": "demo.1.2", "text": "Small.", "at": 0}) + "\n")
    repo.set_issue("demo.1.2", comments=[{"issue_id": "demo.1.2", "author": "owner (site reply)",
                                          "text": "Small.\n\n<!-- pm-reply r-1 -->", "created_at": "x"}])
    repo.log.write_text("")
    with serving(repo) as (url, _):
        until(lambda: spool(repo).read_text() == "")
    assert repo.comments("demo.1.2") == ["Small.\n\n<!-- pm-reply r-1 -->"]
    assert [c[:2] for c in repo.bd_writes()] == []


def test_a_double_submit_with_one_reply_id_stores_one_comment(repo, tmp_path):
    """Two POSTs of one reply id (a double click) add one spool entry and one comment; a new id is a new reply."""
    repo.dolt()
    gate = tmp_path / "gate"
    with serving(repo, FAKE_BD_HOLD=json.dumps([["comments", "add"], str(gate)])) as (url, _):
        form = {"token": page_token(url), "id": "demo.1.2", "text": "Small.", "rid": "r-1"}
        assert [post_reply(url, form)[0] for _ in range(2)] == [303, 303]
        assert len(spool(repo).read_text().splitlines()) == 1
        assert post_reply(url, dict(form, rid="bad id"))[:3:2] == (400, "refused: 'bad id' is not a reply id\n")
        gate.touch()
        until(lambda: spool(repo).read_text() == "")
    assert repo.comments("demo.1.2") == ["Small.\n\n<!-- pm-reply r-1 -->"]


def test_a_page_is_served_and_refreshed_without_waiting_for_the_store_lock(repo, served):
    """While a writer holds the store's lock, pages are served at once and an edit still shows; a state that does
    not render (a need closed, its decision not written yet) is only shown once confirmed under the lock, and until
    then pages state data from before it."""
    import fcntl
    get = lambda: load(f"{served}/sprints/demo-1.html")
    get()
    fd = os.open(repo.store, os.O_RDONLY)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)  # as a pm write holds it across its steps
        sprint = repo.records / "sprints/demo-1.md"
        sprint.write_text(sprint.read_text().replace("Ship it.", "Ship it soon."))
        until_shown(get, lambda p: "Ship it soon." in p, time.time())
        repo.set_issue("demo.1.2", status="closed")  # mid-write: no decision cites it yet
        written = time.time()
        for _ in range(3):
            page = get()
            assert "no decision cites it" not in page and asof(page) < written
            time.sleep(0.5)  # the server has read, failed, and waits for the lock to confirm
    finally:
        os.close(fd)
    until_shown(get, lambda p: "need demo.1.2 (Ask the owner) is closed but no decision cites it" in p, time.time())


def test_a_served_page_states_its_age_and_when_it_is_behind(repo):
    sys.path.insert(0, str(HARNESS))
    from pm.site import STATUS_SLOT, fill_status
    at = time.mktime((2026, 10, 5, 14, 43, 20, 0, 0, -1))
    page = f"<body>{STATUS_SLOT}<h1>x</h1>"
    fresh = fill_status(page, at, "abc", at + 3)
    assert '<p class="asof" data-asof="' in fresh and 'data-page="abc">Data as of 14:43:20 (3 s ago)</p>' in fresh
    behind = fill_status(page, at, "abc", at + SERVE_BEHIND + 2)
    assert ('<p class="asof behind"' in behind
            and "Data as of 14:43:20 (12 s ago); other sessions are writing, so it is behind" in behind)


def test_a_write_the_fingerprint_missed_shows_after_the_next_reread(repo, served):
    """A Beads write the fingerprint does not see shows once another write moves it, here the reply's."""
    def get():
        with urllib.request.urlopen(f"{served}/") as r:
            return r.read().decode()

    token = page_token(served)
    issues = json.loads(repo.state.read_text())  # an outside write the fingerprint does not see
    for i in issues:
        if i["id"] == "demo.1.2":
            i["title"] = "Renamed elsewhere"
    repo.state.write_text(json.dumps(issues))
    assert "Renamed elsewhere" not in get()
    assert post_reply(served, {"token": token, "id": "demo.1.2", "text": "Small."})[0] == 303
    deadline = time.monotonic() + 30
    while "Renamed elsewhere" not in get():
        assert time.monotonic() < deadline, "the background reread never landed"
        time.sleep(0.1)


@pytest.mark.parametrize("value", ["", "abc", ["1"]])
def test_a_bad_pickup_count_is_a_clear_error(repo, value):
    repo.set_issue("demo.1.2", metadata={"picked_up": value})
    res = repo.pm("render")
    assert res.returncode == 1 and "Traceback" not in res.stderr, res.stderr
    assert "error: demo.1.2 (Ask the owner): metadata.picked_up is " in res.stderr, res.stderr


def test_serve_logs_each_request_with_where_its_time_went(repo):
    """One stderr line per request, per read of a changed state and per reply write: what, total, store lock, git,
    render, and each bd call. Requests take no lock and run no bd."""
    repo.dolt()
    with serving(repo) as (url, err):
        token = page_token(url)
        urllib.request.urlopen(f"{url}/").read()
        assert post_reply(url, {"token": token, "id": "demo.1.2", "text": "Small."})[0] == 303
        deadline = time.monotonic() + 10  # a line is written once its work is done, so the client may be first
        while "reply demo.1.2 sent" not in err.read_text() and time.monotonic() < deadline:
            time.sleep(0.05)
    fields = r" total=\d+ms lock=\d+ms git=\d+ms render=\d+ms bd="
    lines = err.read_text().splitlines()
    assert re.fullmatch(r"refresh" + fields + r"list:\d+ms", lines[0])  # the first read, before serving
    requests = [l for l in lines if not l.startswith(("refresh", "push "))]
    assert len(requests) == 4, lines
    assert "push demo.1.2: not running: the request stores no session inbox" in lines
    assert re.fullmatch(r"GET / 200 total=\d+ms lock=0ms git=0ms render=\d+ms bd=-", requests[0])
    assert re.fullmatch(r"GET / 200 total=\d+ms lock=0ms git=0ms render=0ms bd=-", requests[1])  # from the cache
    assert re.fullmatch(r"POST /reply 303 total=\d+ms lock=0ms git=0ms render=0ms bd=-", requests[2])
    assert re.fullmatch(r"reply demo.1.2 sent" + fields + r"comments:\d+ms", requests[3])


@pytest.mark.parametrize("token, form, headers, code, said", [
    (None, {"id": "demo.1.2", "text": "Small."}, {}, 403, "no valid token"),
    ("forged", {"id": "demo.1.2", "text": "Small."}, {}, 403, "no valid token"),
    ("page", {"id": "demo.1.2", "text": "Small."}, {"Host": "evil.example:80"}, 403, "is neither this machine"),
    ("page", {"id": "demo.1.1", "text": "Small."}, {}, 400, "'demo.1.1' is not a request waiting on the owner"),
    ("page", {"id": "demo.1.2", "text": " \r\n "}, {}, 400, "the reply is empty"),
])
def test_reply_is_refused_without_the_token_from_another_host_or_off_target(repo, served, token, form, headers,
                                                                            code, said):
    if token is not None:
        form = dict(form, token=page_token(served) if token == "page" else token)
    res = post_reply(served, form, headers)
    assert res[0] == code and said in res[2]
    assert repo.bd_writes() == []


def test_reply_to_a_closed_request_is_refused(repo, served):
    token = page_token(served)
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    until_shown(lambda: urllib.request.urlopen(f"{served}/").read().decode(), lambda p: 'value="demo.1.2"' not in p,
                time.time())
    res = post_reply(served, {"token": token, "id": "demo.1.2", "text": "Small."})
    assert res[0] == 400 and "demo.1.2 is already closed (Dismissed)" in res[2]
    assert repo.bd_writes() == []


def test_a_request_records_its_session_and_inbox_never_the_token(repo):
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET="/run/x/inbox.sock",
                    CLAUDE_CODE_MESSAGING_TOKEN="secret-token")
    res = repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED)
    assert res.returncode == 0, res.stderr
    assert "pm serve pushes the owner's reply into this session" in res.stdout
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION).returncode == 0
    issues = repo.issues()
    assert issues["demo.1.3"]["metadata"] == {"session": "sess-1", "inbox": "/run/x/inbox.sock",
                                              "inbox_host": socket.gethostname()} \
        == issues["demo.1.4"]["metadata"]
    assert "secret-token" not in repo.state.read_text() + repo.log.read_text()
    repo.env.pop("CLAUDE_CODE_MESSAGING_SOCKET")
    res = repo.pm("decision", "need", "--title", "Lexer?", "--parent", "demo.1", stdin=NEED)
    assert "this session has no inbox" in res.stdout and "pm reply read demo.1.5 prints it" in res.stdout
    assert repo.issues()["demo.1.5"]["metadata"] == {"session": "sess-1"}


def test_a_request_raised_in_codex_records_its_thread(repo):
    repo.env = dict(repo.env, CODEX_THREAD_ID="thread-1")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED).returncode == 0
    assert repo.issues()["demo.1.3"]["metadata"] == {"session": "thread-1"}


@contextlib.contextmanager
def session_inbox(path: str | None = None):
    """A stand-in for a Claude Code session's inbox: a Unix socket that collects each JSON line written to it. Its
    path is short (a socket path is limited to about 100 bytes), so it lives in its own temp dir, or at `path`."""
    import threading
    d = None if path else inbox_dir()
    path = path or os.path.join(d, "s")
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(path)
    srv.listen()
    lines: list[dict] = []

    def accept():
        while True:
            try:
                conn, _ = srv.accept()
            except OSError:
                return
            with conn:
                data = b"".join(iter(lambda: conn.recv(65536), b""))
            lines.extend(json.loads(line) for line in data.decode().splitlines())
    threading.Thread(target=accept, daemon=True).start()
    try:
        yield path, lines
    finally:
        srv.close()
        os.unlink(path)
        if d:
            shutil.rmtree(d)


def inbox_dir() -> str:
    import tempfile
    return tempfile.mkdtemp(prefix="pm-inbox-")


def wait_for(cond, what: str, timeout: float = 30):
    deadline = time.time() + timeout
    while not (got := cond()):
        assert time.time() < deadline, f"not within {timeout}s: {what}"
        time.sleep(0.05)
    return got


def inbox_text(line: dict) -> str:
    """The text of a message written to a session's inbox, checked to be the user-message line Claude Code takes."""
    assert line["type"] == "user" and line["message"]["role"] == "user"
    return line["message"]["content"]


def test_every_reply_is_pushed_into_the_session_that_asked(repo, served):
    """The three replies lost on 2026-10-06, each pushed by pm serve: several requests raised in one command, one
    raised with its output piped through grep, and a second reply to a request already delivered."""
    with session_inbox() as (inbox, lines):
        env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET=inbox, NEED=NEED)
        pm = shlex.join(PM)
        raise_ = f'printf %s "$NEED" | {pm} decision need --parent demo.1'
        script = (f"{raise_} --title A >/dev/null && {raise_} --title B >/dev/null && "
                  f"{raise_} --title C | grep -o 'raised decision need [^ ;]*'")
        res = subprocess.run(["bash", "-c", script], cwd=repo.root, env=env, capture_output=True, text=True)
        assert (res.returncode, res.stdout) == (0, "raised decision need demo.1.5\n"), res.stderr
        until_shown(lambda: load(f"{served}/"), lambda p: 'value="demo.1.5"' in p, time.time())
        token = page_token(served)

        def reply(issue_id, text):
            assert post_reply(served, {"token": token, "id": issue_id, "text": text})[0] == 303

        reply("demo.1.3", "First answer.")
        wait_for(lambda: len(lines) == 1, "the first reply pushed")
        assert inbox_text(lines[0]).startswith("pm: owner reply to decision demo.1.3 (A), relayed from the site:\n")
        assert "First answer." in inbox_text(lines[0]) and "pm decision add --need demo.1.3" in inbox_text(lines[0])
        wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("picked_up") == "1", "the reply marked delivered")
        for issue_id, text in (("demo.1.4", "Answer to B."), ("demo.1.5", "Answer to C."),
                               ("demo.1.3", "On second thought, no.")):
            reply(issue_id, text)
        wait_for(lambda: len(lines) == 4, "every later reply pushed")
        texts = [inbox_text(line) for line in lines[1:]]
        assert [next(i for i in ("demo.1.4", "demo.1.5", "demo.1.3") if f" {i} " in x) for x in texts] == \
            ["demo.1.4", "demo.1.5", "demo.1.3"]
        assert "Answer to B." in texts[0] and "Answer to C." in texts[1]
        assert "On second thought, no." in texts[2] and "First answer." not in texts[2], "no reply delivered twice"
        wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("picked_up") == "2", "the second reply marked")
        page = until_shown(lambda: load(f"{served}/"), lambda p: "On second thought, no." in p
                           and "Reply saved" not in p, None)
        assert "delivered to the agent&#x27;s session" in page or "delivered to the agent's session" in page
    assert "pm reply read" not in repo.pm("show").stdout


def test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read(repo, served):
    """No socket at the stored inbox: the session ended. The reply stays undelivered, its card says so, pm show
    flags it for the next session, and pm reply read prints it once and marks it delivered."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET="/nonexistent/pm/s")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED).returncode == 0
    until_shown(lambda: load(f"{served}/"), lambda p: 'value="demo.1.3"' in p, time.time())
    assert post_reply(served, {"token": page_token(served), "id": "demo.1.3", "text": "Small, until tables."})[0] == 303
    page = until_shown(lambda: load(f"{served}/"), lambda p: "Small, until tables." in p and "Saving" not in p, None)
    assert "the session that asked is not running" in page
    assert "picked_up" not in repo.issues()["demo.1.3"]["metadata"]
    assert "Parser?  (sprint 1)  -> bd show demo.1.3  [undelivered reply: pm reply read demo.1.3]" \
        in repo.pm("show").stdout
    res = repo.pm("reply", "read")  # no ids: this session's open requests
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("pm: owner reply to decision demo.1.3 (Parser?), relayed from the site:\n  [")
    assert "Small, until tables." in res.stdout
    assert repo.issues()["demo.1.3"]["metadata"]["picked_up"] == "1"
    assert "pm reply read" not in repo.pm("show").stdout
    assert repo.pm("reply", "read", "demo.1.3").stdout == "nothing undelivered on demo.1.3\n"
    repo.log.write_text("")  # refused() checks that nothing at all was written to Beads
    refused(repo, "reply", "read", "demo.1.1", match=r"demo.1.1 is not a request to the owner")
    repo.env.pop("CLAUDE_CODE_SESSION_ID")
    refused(repo, "reply", "read", match=r"name the requests to read")


def test_a_push_that_failed_is_retried_by_the_sweep(repo):
    """The session's inbox was not there when the reply came (a push that failed, as after a crash between storing
    and pushing): pm serve's sweep, at start and every minute, pushes it once the inbox is back."""
    repo.dolt()
    path = os.path.join(inbox_dir(), "s")
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET=path)
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED).returncode == 0
    with serving(repo) as (url, log):
        until_shown(lambda: load(f"{url}/"), lambda p: 'value="demo.1.3"' in p, time.time())
        assert post_reply(url, {"token": page_token(url), "id": "demo.1.3", "text": "Small."})[0] == 303
        wait_for(lambda: "push demo.1.3: not running: the session is not running" in log.read_text(),
                 "the failed push logged")
    assert "picked_up" not in repo.issues()["demo.1.3"]["metadata"]
    with session_inbox(path) as (_, lines):
        with serving(repo) as (url, log):
            wait_for(lambda: lines, "the sweep's push")
            wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("picked_up") == "1", "the reply marked")
            assert "push demo.1.3 (sweep): delivered" in log.read_text()
    assert len(lines) == 1 and "Small." in inbox_text(lines[0])


def test_reply_read_reads_only_this_sessions_requests(repo):
    """Without ids, one narrow list of this session's open requests; never the whole database with bd list --all."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-2")
    assert repo.pm("action", "need", "--title", "Other", "--parent", "demo.1", stdin=ACTION).returncode == 0
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION).returncode == 0
    repo.log.write_text("")
    assert repo.pm("reply", "read").stdout == "nothing undelivered on demo.1.4\n"
    assert repo.bd_calls() == [["list", "--label", "human", "--metadata-field", "session=sess-1", "--limit", "0",
                                "--json"]]


@pytest.mark.parametrize("close", [
    ("decision", "add", "--level", "sprint", "--sprint", "demo.1", "--need", "demo.1.2"),
    ("decision", "close", "demo.1.2", "--reason", "sets no rule"),
])
def test_closing_a_request_with_an_undelivered_reply_is_refused(repo, close):
    """A reply not delivered yet would be buried by the close: pm show flags only open requests. So closing refuses
    until the reply is read, which marks it delivered."""
    repo.set_issue("demo.1.2", labels=["human"], metadata={"session": "sess-1", "picked_up": "1"},
                   comments=[{"issue_id": "demo.1.2", "author": "owner (site reply)", "text": t,
                              "created_at": f"2026-10-03T12:0{n}:00Z"} for n, t in enumerate(("Small.", "No, big."))])
    answer = "Use the big parser.\nThe owner changed their mind.\n"
    refused(repo, *close, stdin=answer,
            match=r"demo.1.2 holds a site reply not delivered yet; read it with pm reply read demo.1.2")
    res = repo.pm("reply", "read", "demo.1.2")
    assert res.returncode == 0 and "No, big." in res.stdout and "Small." not in res.stdout
    res = repo.pm(*close, stdin=answer)
    assert res.returncode == 0, res.stderr


def test_a_need_answered_in_chat_closes_and_nothing_is_flagged(repo):
    """The owner answers in chat and the agent records it: only site replies count as replies, so neither an agent's
    comment nor pm's closing comment reads as undelivered, the close goes through, and pm show flags nothing. A site
    reply already delivered does not hold the close either."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED).returncode == 0
    subprocess.run(["bd", "comments", "add", "demo.1.3", "Asked in chat as well."], cwd=repo.root, env=repo.env,
                   check=True, capture_output=True)
    assert "pm reply read" not in repo.pm("show").stdout
    res = repo.pm("decision", "add", "--level", "sprint", "--sprint", "demo.1", "--need", "demo.1.3",
                  stdin="Use the small parser.\nThe owner said so in chat.\n")
    assert res.returncode == 0, res.stderr
    assert repo.issues()["demo.1.3"]["status"] == "closed"
    assert "pm reply read" not in repo.pm("show").stdout and "undelivered" not in repo.pm("show").stdout
    repo.set_issue("demo.1.2", labels=["human"], metadata={"picked_up": "1"},
                   comments=[{"issue_id": "demo.1.2", "author": "owner (site reply)", "text": "Small.",
                              "created_at": "2026-10-03T12:00:00Z"},
                             {"issue_id": "demo.1.2", "author": "agent", "text": "Noted.",
                              "created_at": "2026-10-03T12:01:00Z"}])
    res = repo.pm("decision", "close", "demo.1.2", "--reason", "sets no rule", stdin="Small.\n")
    assert res.returncode == 0, res.stderr


def test_closing_an_action_with_an_undelivered_reply_is_refused(repo):
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION).returncode == 0
    repo.set_issue("demo.1.3", comments=[{"issue_id": "demo.1.3", "author": "owner (site reply)", "text": "Not yet.",
                                          "created_at": "2026-10-03T12:00:00Z"}])
    repo.log.write_text("")  # refused() checks that nothing at all was written to Beads
    refused(repo, "action", "done", "demo.1.3", "--reason", "it answers",
            match=r"demo.1.3 holds a site reply not delivered yet")


REVIEWED_PR = "https://github.com/o/r/pull/7"


def review_with_origin(repo) -> str:
    """An open review of REVIEWED_PR under demo.1, and an origin remote whose main is one commit ahead:
    the sha returned."""
    issues = json.loads(repo.state.read_text()) + [
        {"id": "demo.1.3", "title": "Review PR #7", "status": "open", "issue_type": "task", "parent": "demo.1",
         "labels": ["human", "action"], "created_at": "2026-10-01T12:00:00Z",
         "metadata": {"review": {"pr": REVIEWED_PR, "sprints": ["demo.1"], "focus": "f", "designs": []}}}]
    repo.state.write_text(json.dumps(issues))
    origin = repo.root.parent / "origin.git"
    repo.git("init", "-q", "--bare", "-b", "main", str(origin))
    repo.git("remote", "add", "origin", str(origin))
    repo.git("push", "-q", "origin", "main")
    other = repo.root.parent / "other"
    repo.git("clone", "-q", str(origin), str(other))
    repo.git("-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "merge",
             cwd=other)
    repo.git("push", "-q", "origin", "HEAD:main", cwd=other)
    return repo.git("rev-parse", "HEAD", cwd=other).strip()


def pull_main(repo) -> str:
    """The pull a merged review's wake names: the main checkout as git reports it (resolved, as on macOS /private)."""
    common = repo.git("rev-parse", "--path-format=absolute", "--git-common-dir").strip()
    return f"git -C {Path(common).parent} pull --ff-only origin main"


def test_a_reviewed_prs_merge_is_pushed_into_the_session_once(repo):
    repo.dolt()
    sha = review_with_origin(repo)
    repo.set_pr(REVIEWED_PR, "MERGED", sha)
    with session_inbox() as (inbox, lines):
        repo.set_issue("demo.1.3", metadata={**repo.issues()["demo.1.3"]["metadata"], "session": "sess-1",
                                             "inbox": inbox, "inbox_host": socket.gethostname()})
        with serving(repo):
            wait_for(lambda: lines, "the merge pushed")
            wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("merge_reported") == sha, "the merge marked")
        assert inbox_text(lines[0]) == (f"pm: PR #7 of review demo.1.3 (Review PR #7) merged to main as {sha}\n"
                                    f"next: pm action done demo.1.3 --reason \"merged as {sha}\", then update the "
                                    f"main checkout: {pull_main(repo)}")
        assert repo.issues()["demo.1.3"]["metadata"]["merged"] == sha
        assert repo.issues()["demo.1.3"]["status"] == "open", "the agent closes the review, not pm serve"
        with serving(repo) as (url, _):  # a restart: the merge is stored, so it is neither looked up nor pushed again
            until_shown(lambda: load(f"{url}/"), lambda p: "Review PR #7" in p, None)
            deadline = time.time() + 2
            while time.time() < deadline:
                assert len(lines) == 1
                time.sleep(0.1)
    assert repo.pm("reply", "read", "demo.1.3").stdout == "nothing undelivered on demo.1.3\n"


@pytest.mark.parametrize("off_main", [False, True])
def test_the_merge_watch_goes_on_while_the_pr_is_open_or_merged_off_main(repo, off_main):
    """An open PR, or one merged into a stacked base, is no merge to main: nothing is stored or pushed."""
    repo.dolt()
    review_with_origin(repo)
    if off_main:
        repo.git("-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "s")
        repo.git("push", "-q", "origin", "HEAD:stacked-base")
        repo.set_pr(REVIEWED_PR, "MERGED", repo.git("rev-parse", "HEAD").strip())
    else:
        repo.set_pr(REVIEWED_PR, "OPEN")
    gh_log = repo.root.parent / "gh.log"
    with session_inbox() as (inbox, lines):
        repo.set_issue("demo.1.3", metadata={**repo.issues()["demo.1.3"]["metadata"], "session": "sess-1",
                                             "inbox": inbox, "inbox_host": socket.gethostname()})
        with serving(repo, FAKE_GH_LOG=str(gh_log)):
            wait_for(lambda: gh_log.exists() and gh_log.read_text(), "the watch looked at the PR")
            deadline = time.time() + 2  # its git look, if any, and a push would follow within this
            while time.time() < deadline:
                assert lines == []
                time.sleep(0.1)
    assert "merged" not in repo.issues()["demo.1.3"]["metadata"]


def test_a_merge_its_session_missed_is_flagged_and_read(repo):
    """pm serve saw the merge (metadata.merged) but could not push it: pm show flags it, pm reply read prints it."""
    sha = review_with_origin(repo)
    repo.set_issue("demo.1.3", metadata={**repo.issues()["demo.1.3"]["metadata"], "merged": sha})
    assert "[undelivered reply: pm reply read demo.1.3]" in repo.pm("show").stdout
    res = repo.pm("reply", "read", "demo.1.3")
    assert res.returncode == 0 and f"merged to main as {sha}" in res.stdout, res.stderr
    assert repo.issues()["demo.1.3"]["metadata"]["merge_reported"] == sha
    assert "pm reply read" not in repo.pm("show").stdout


# ---------------------------------------------------------------- pm task claim

def transcript(repo, sid: str, age_s: float, where: str = "-repo") -> None:
    """Session sid's Claude Code transcript under the test's config dir, last written age_s seconds ago."""
    p = Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects" / where / f"{sid}.jsonl"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("{}\n")
    t = time.time() - age_s
    os.utime(p, (t, t))


def claim(repo, task: str, sid: str | None, *extra: str):
    env = dict(repo.env, **({"CLAUDE_CODE_SESSION_ID": sid} if sid else {}))
    return subprocess.run([*PM, "task", "claim", task, *extra], cwd=repo.root, env=env,
                          capture_output=True, text=True)


def test_task_claim_records_the_session_and_time(repo):
    repo.set_issue("demo.1.2", labels=[])
    res = claim(repo, "demo.1.2", "sess-a")
    assert res.returncode == 0, res.stderr
    i = repo.issues()["demo.1.2"]
    assert i["status"] == "in_progress" and i["metadata"]["claimed_by"] == "sess-a"
    assert re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", i["metadata"]["claimed_at"])
    assert repo.bd_writes()[-1][:3] == ["update", "demo.1.2", "--claim"]


def test_task_claim_needs_a_session(repo):
    res = claim(repo, "demo.1.2", None)
    assert res.returncode == 1 and "--session" in res.stderr and not repo.bd_writes()
    assert claim(repo, "demo.1.2", None, "--session", "manual").returncode == 0
    assert repo.issues()["demo.1.2"]["metadata"]["claimed_by"] == "manual"


def test_task_claim_refuses_a_task_another_live_session_holds(repo):
    repo.set_issue("demo.1.2", status="in_progress", metadata={"claimed_by": "sess-a", "claimed_at": "2026-10-01T12:00:00Z"})
    transcript(repo, "sess-a", 60, where="-other-worktree")
    res = claim(repo, "demo.1.2", "sess-b")
    assert res.returncode == 1
    assert "held by live session sess-a" in res.stderr and "last 30 minutes" in res.stderr
    assert not repo.bd_writes()
    # the same session (a subagent shares it) may claim again
    assert claim(repo, "demo.1.2", "sess-a").returncode == 0


def test_task_claim_takes_over_a_stale_holder(repo):
    repo.set_issue("demo.1.2", status="in_progress", metadata={"claimed_by": "sess-a", "claimed_at": "2026-10-01T12:00:00Z"})
    transcript(repo, "sess-a", 31 * 60)
    res = claim(repo, "demo.1.2", "sess-b")
    assert res.returncode == 0, res.stderr
    assert "took it over from idle session sess-a" in res.stdout
    assert repo.issues()["demo.1.2"]["metadata"]["claimed_by"] == "sess-b"


def test_show_names_holders_and_warns_of_other_live_sessions(repo):
    repo.set_issue("demo.1.2", status="in_progress", labels=[],
                   metadata={"claimed_by": "aaaaaaaa-1111", "claimed_at": "2026-10-01T12:00:00Z"})
    transcript(repo, "aaaaaaaa-1111", 60)
    out = repo.pm("show").stdout
    assert "  held by: aaaaaaaa\n" in out
    assert re.search(r"in_progress  \.1\.2  Ask the owner  \[held by aaaaaaaa, \d+d, live\]", out)
    assert out.startswith("warning: other live sessions hold these tasks; do not start or delegate them:\n  demo.1.2")
    mine = subprocess.run([*PM, "show"], cwd=repo.root, capture_output=True, text=True,
                          env=dict(repo.env, CLAUDE_CODE_SESSION_ID="aaaaaaaa-1111")).stdout
    assert "warning:" not in mine
    assert "[held by aaaaaaaa" in repo.pm("show", "--sprint", "demo.1").stdout
    holder = json.loads(repo.pm("show", "--json").stdout)["projects"][0]["sprints"][0]["tasks"][0]["holder"]
    assert holder == {"session": "aaaaaaaa-1111", "assignee": None, "claimed_at": "2026-10-01T12:00:00Z", "live": True}


# ---------------------------------------------------------------- pm push: the scheduled job

DAY2 = "---\ntype: day\ndate: 2026-10-02\n---\n\n## Today\n\n> What are we chasing today, and why now?\n\nMore.\n"


@pytest.fixture
def pushed(repo, tmp_path):
    """The repo with a bare origin holding its records branch, and a second clone of origin (repo.other) that can
    move it."""
    origin = tmp_path / "origin.git"
    repo.git("init", "-q", "--bare", str(origin))
    repo.git("remote", "add", "origin", str(origin))
    repo.git("push", "-q", "origin", "records", cwd=repo.store)
    repo.git("fetch", "-q", "origin")
    other = tmp_path / "other"
    repo.git("clone", "-q", "-b", "records", str(origin), str(other))
    repo.git("config", "user.email", "o@example.com", cwd=other)
    repo.git("config", "user.name", "o", cwd=other)
    repo.other = other
    return repo


def remote_records(repo) -> str:
    return repo.git("rev-parse", "refs/heads/records", cwd=repo.root.parent / "origin.git").strip()


def push_state(repo) -> dict:
    return json.loads((repo.root / ".pm/run/push.json").read_text())


def move_remote(repo, rel: str, text: str) -> None:
    (repo.other / rel).parent.mkdir(parents=True, exist_ok=True)
    (repo.other / rel).write_text(text)
    repo.git("add", "-A", cwd=repo.other)
    repo.git("commit", "-qm", "remote", cwd=repo.other)
    repo.git("push", "-q", "origin", "records", cwd=repo.other)


def test_push_pushes_beads_and_new_records_commits(pushed):
    repo = pushed
    first = repo.pm("push").stdout
    assert first.count(" ok: ") == 3 and "summary ok: summarized" in first, first
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("a day")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    assert ["dolt", "push"] in repo.bd_calls()
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    state = push_state(repo)
    assert state["beads"]["ok"] and state["records"]["ok"] and state["records"]["message"] == "pushed 2 commit(s)", "the day file and its new summary"
    assert state["records"]["last_ok"] == state["records"]["at"]
    assert len((repo.root / ".pm/run/push.log").read_text().splitlines()) == 6
    assert state["summary"]["message"].startswith("summarized"), "the new day file changed the activity"


def test_push_rebases_onto_a_moved_remote(pushed):
    repo = pushed
    move_remote(repo, "docs/remote.md", "x\n")
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("local")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    assert repo.store_log()[1:3] == ["local", "remote"] and repo.store_log()[0].startswith("pm: summarized ")
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    assert push_state(repo)["records"]["message"] == "pushed 2 commit(s) after rebasing onto 1 new on origin/records"


def test_push_rebase_conflict_leaves_store_untouched_and_records_error(pushed):
    repo = pushed
    move_remote(repo, "days/2026-10-01.md", "remote\n")
    repo.write("days/2026-10-01.md", "local\n")
    repo.commit("local")
    head, remote = repo.git("rev-parse", "HEAD", cwd=repo.store), remote_records(repo)
    res = repo.pm("push")
    assert res.returncode == 1
    assert repo.git("rev-parse", "HEAD", cwd=repo.store) == head
    assert repo.git("status", "--porcelain", cwd=repo.store) == ""
    assert remote_records(repo) == remote
    state = push_state(repo)
    assert state["beads"]["ok"] and not state["records"]["ok"] and state["records"]["last_ok"] is None
    assert state["records"]["message"].startswith(
        "rebase onto origin/records stopped, aborted and the store left as it was")


def test_push_exits_at_once_while_another_holds_the_lock(pushed):
    import fcntl
    repo = pushed
    (repo.root / ".pm/run").mkdir()
    fd = os.open(repo.root / ".pm/run/push.lock", os.O_RDWR | os.O_CREAT)
    fcntl.flock(fd, fcntl.LOCK_EX)
    try:
        res = repo.pm("push")
    finally:
        os.close(fd)
    assert res.returncode == 0 and res.stdout.strip() == "another pm push holds the lock; skipped"
    assert ["dolt", "push"] not in repo.bd_calls()
    assert not (repo.root / ".pm/run/push.json").exists()


def test_show_flags_a_failed_or_overdue_push(pushed):
    repo = pushed
    assert not repo.pm("show").stdout.startswith("warning: the scheduled push"), "no flag before the first run"
    repo.env = dict(repo.env, FAKE_BD_FAIL=json.dumps(["dolt", "push"]))
    assert repo.pm("push").returncode == 1
    log = re.escape(str(repo.root / ".pm/run/push.log"))
    lines = repo.pm("show").stdout.splitlines()
    assert lines[0] == "warning: the scheduled push needs attention (pm where; it runs bin/pm push):"
    assert re.fullmatch(r"  beads push failed at \S+: bd dolt push failed: fake bd: failing .* on purpose; log " + log,
                        lines[1]), lines[1]
    assert not lines[2].startswith("  records"), "records pushed fine"
    state = push_state(repo)
    state["beads"] = dict(state["records"])
    state["records"]["last_ok"] = "2026-01-01T00:00:00+00:00"
    (repo.root / ".pm/run/push.json").write_text(json.dumps(state))
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("unpushed")
    lines = repo.pm("show").stdout.splitlines()
    assert re.fullmatch(r"  records push overdue: last successful push 2026-01-01T00:00:00\+00:00, \d+ min ago "
                        r"\(the job runs every 10 min\); 1 records commit\(s\) not on origin/records; log " + log,
                        lines[1]), lines[1]
    assert not lines[2].startswith("  beads")


def test_serve_shows_the_push_banner_on_home_and_project_pages(pushed, served):
    repo = pushed
    get = lambda p: urllib.request.urlopen(f"{served}/{p}").read().decode()
    assert "beads push failed" not in get("")
    repo.env = dict(repo.env, FAKE_BD_FAIL=json.dumps(["dolt", "push"]))
    repo.pm("push")
    for page in ("", "projects/demo.html"):
        assert '<div class="note draft push"><p><strong>Push</strong>' in get(page)
        assert "beads push failed" in get(page)
    assert "beads push failed" not in get("sprints/demo-1.html")


@pytest.fixture
def public(repo):
    """The repo's public base URL (a tunnel to pm serve), set in its config before `served` starts."""
    write_config(repo.root, site_url="https://pm.example.com/")
    return "https://pm.example.com"


def test_record_link_uses_the_public_base_url_when_set(repo, public, served):
    res = repo.pm("record", "link", "demo.1")
    assert res.returncode == 0, res.stderr
    assert res.stdout == f"{public}/sprints/demo-1.html\n"


@pytest.mark.parametrize("host, code", [("pm.example.com", 303), ("pm.example.com.evil.example", 403),
                                        ("evil.example", 403)])
def test_reply_is_accepted_from_the_public_host_only(repo, public, served, host, code):
    res = post_reply(served, {"token": page_token(served), "id": "demo.1.2", "text": "Small."}, {"Host": host})
    assert res[0] == code, res


def test_reply_from_a_public_host_is_refused_when_none_is_set(repo, served):
    res = post_reply(served, {"token": page_token(served), "id": "demo.1.2", "text": "Small."},
                     {"Host": "pm.example.com"})
    assert res[0] == 403 and "neither this machine nor the configured site_url" in res[2]
    assert repo.bd_writes() == []


# ---------------------------------------------------------------- day pages and pm day summarize

def now_z() -> str:
    from datetime import datetime, timezone
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def claude_calls(repo) -> list[dict]:
    log = Path(repo.env["FAKE_CLAUDE_LOG"])
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def summary(repo) -> dict:
    return json.loads((repo.records / f"days/{TODAY}.summary.json").read_text())


def test_a_day_with_activity_renders_a_page_without_a_day_file(repo):
    repo.set_issue("demo.1.2", status="in_progress", started_at=now_z())
    assert repo.pm("render").returncode == 0
    assert not (repo.records / f"days/{TODAY}.md").exists()
    page = (repo.root / f"site/days/{TODAY}.html").read_text()
    assert "STARTED</span> Ask the owner" in page and "No summary has been generated for this day." in page
    assert f'href="days/{TODAY}.html"' in (repo.root / "site/index.html").read_text()
    assert "Start." in (repo.root / "site/days/2026-10-01.html").read_text(), "an older day record keeps its paragraph"


def test_day_summarize_skips_unchanged_activity_and_regenerates_on_change(repo):
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert committed(repo, [f"pm: summarized {TODAY} in records/days/{TODAY}.summary.json"])
    first = summary(repo)
    assert first["text"] == "Summary 1." and first["date"] == TODAY
    call = claude_calls(repo)[0]
    assert call["args"][:4] == ["-p", "--model", "haiku", "--tools"]
    assert "records commit: records [" in call["stdin"], "today's records commits are the activity"
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and "unchanged" in res.stdout and len(claude_calls(repo)) == 1
    repo.set_issue("demo.1.2", status="in_progress", started_at=now_z())
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert summary(repo)["text"] == "Summary 2." and summary(repo)["digest"] != first["digest"]
    assert "started: Ask the owner" in claude_calls(repo)[1]["stdin"]
    assert repo.pm("render").returncode == 0
    page = (repo.root / f"site/days/{TODAY}.html").read_text()
    assert re.search(r"<span>generated at \d\d:\d\d</span>.*Summary 2\.", page, re.S), page
    assert "Summary 2." in (repo.root / "site/index.html").read_text()
    assert f"today {TODAY}: Summary 2. (generated " in repo.pm("show").stdout


def test_day_summarize_regenerates_yesterday_once_when_its_activity_changed(repo):
    from datetime import date, timedelta
    """After midnight, the activity of yesterday's last minutes is summarized, once."""
    yesterday = (date.today() - timedelta(days=1)).isoformat()
    path = repo.records / f"days/{yesterday}.summary.json"
    path.write_text(json.dumps({"date": yesterday, "generated_at": now_z().replace("Z", "+00:00"),
                                "digest": "stale", "model": "haiku", "text": "Old."}) + "\n")
    repo.pm("commit", "-m", "an old summary", str(path))
    repo.set_issue("demo.1.2", status="in_progress", started_at=f"{yesterday}T12:00:00Z")
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert f"summarized {yesterday}" in res.stdout
    data = json.loads(path.read_text())
    assert data["text"] != "Old." and data["digest"] != "stale"
    assert any(f"for {yesterday}" in c["stdin"] and "started: Ask the owner" in c["stdin"]
               for c in claude_calls(repo))
    calls = len(claude_calls(repo))
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and f"summarized {yesterday}" not in res.stdout and len(claude_calls(repo)) == calls


def test_a_past_days_digest_ignores_later_state_changes(repo):
    """Live bug: yesterday's summary was regenerated after midnight because its digest listed the requests open now
    and each sprint's current status. A past day's digest holds only that day's events, so later changes leave it."""
    from datetime import date, timedelta
    yesterday = (date.today() - timedelta(days=1)).isoformat()
    repo.set_issue("demo.1.2", status="in_progress", started_at=f"{yesterday}T12:00:00Z")
    add_review(repo, status="open")
    repo.set_issue("demo.1.3", created_at=f"{yesterday}T12:00:00Z")
    assert repo.pm("day", "summarize").returncode == 0
    stdin = next(c["stdin"] for c in claude_calls(repo) if f"for {yesterday}" in c["stdin"])
    assert "request to the owner (action) raised: Review PR #12" in stdin and "(demo." not in stdin
    calls = len(claude_calls(repo))
    repo.set_issue("demo.1.3", status="closed", close_reason=f"merged as {SHA}", closed_at=now_z())
    repo.set_issue("demo.1", status="in_progress")
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and f"summarized {yesterday}" not in res.stdout, res.stdout
    assert all(f"for {yesterday}" not in c["stdin"] for c in claude_calls(repo)[calls:])


def test_the_digest_carries_what_shipped(repo):
    close_ready(repo)
    repo.set_issue("demo.1.2", close_reason="Shipped the parser.", closed_at=now_z())
    repo.set_issue("demo.1", status="closed", closed_at=now_z())
    assert repo.pm("day", "summarize").returncode == 0
    stdin = claude_calls(repo)[-1]["stdin"]
    assert "sprint finished: Done: shipped the thing." in stdin, stdin
    assert "closed: Ask the owner (Shipped the parser.)" in stdin, stdin
    assert "Never write ids" in stdin


def test_a_summary_with_html_shows_it_as_text(repo):
    path = repo.records / f"days/{TODAY}.summary.json"
    path.write_text(json.dumps({"date": TODAY, "generated_at": now_z().replace("Z", "+00:00"), "digest": "x",
                                "model": "haiku", "text": "Done <script>alert(1)</script>.\n\nSecond *part*."}))
    assert repo.pm("render").returncode == 0
    for page in (f"site/days/{TODAY}.html", "site/index.html"):
        text = (repo.root / page).read_text()
        assert "<script>alert" not in text and "&lt;script&gt;alert(1)&lt;/script&gt;" in text, page
    assert "<em>part</em>" in (repo.root / f"site/days/{TODAY}.html").read_text(), "Markdown still renders"


def test_a_bullet_summary_renders_a_list_on_the_day_page_and_one_line_in_lists(repo):
    repo.set_issue("demo.1.2", status="in_progress", started_at=now_z())
    repo.env = dict(repo.env, FAKE_CLAUDE_OUTPUT="- The parser *ships*.\n- Review the parser change.")
    assert repo.pm("day", "summarize").returncode == 0
    assert "Output only the bullets" in claude_calls(repo)[-1]["stdin"]
    assert repo.pm("render").returncode == 0
    day = (repo.root / f"site/days/{TODAY}.html").read_text()
    assert "<ul>\n<li>The parser <em>ships</em>.</li>\n<li>Review the parser change.</li>\n</ul>" in day, day
    index = (repo.root / "site/index.html").read_text()
    assert "The parser <em>ships</em>. · Review the parser change." in index
    assert "- The parser" not in index


def test_a_paragraph_summary_still_renders_as_a_paragraph(repo):
    path = repo.records / f"days/{TODAY}.summary.json"
    path.write_text(json.dumps({"date": TODAY, "generated_at": now_z().replace("Z", "+00:00"), "digest": "x",
                                "model": "haiku", "text": "The parser ships. Review it."}))
    assert repo.pm("render").returncode == 0
    assert "<p>The parser ships. Review it.</p>" in (repo.root / f"site/days/{TODAY}.html").read_text()
    assert "The parser ships. Review it." in (repo.root / "site/index.html").read_text()


@pytest.mark.parametrize("bad, says", [("{not json", "not valid JSON"), ('{"date": "x"}', "a summary is a JSON object")])
def test_a_malformed_summary_fails_render_and_commit_naming_the_file(repo, bad, says):
    path = repo.records / f"days/{TODAY}.summary.json"
    path.write_text(bad)
    for args in (("render",), ("commit", "-m", "a summary", str(path))):
        refused(repo, *args, match=rf"error: days/{TODAY}\.summary\.json: {says}")
    res = subprocess.run([*RENDER, "site"], cwd=repo.root, env=repo.env,
                         capture_output=True, text=True)
    assert res.returncode == 1 and f"error: days/{TODAY}.summary.json: {says}" in res.stderr, res.stderr


def test_day_summarize_failure_writes_nothing(repo):
    repo.env = dict(repo.env, FAKE_CLAUDE_FAIL="Not logged in")
    refused(repo, "day", "summarize", match=r"error: claude -p failed \(exit 1\): Not logged in; no summary written")


def test_day_summarize_without_activity_asks_nothing(repo):
    env = dict(repo.env, GIT_AUTHOR_DATE="2026-10-01T12:00:00", GIT_COMMITTER_DATE="2026-10-01T12:00:00")
    subprocess.run(["git", "commit", "-q", "--amend", "--no-edit"], cwd=repo.store, env=env, check=True)
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and "nothing to summarize" in res.stdout and claude_calls(repo) == []


def test_serve_shows_a_generated_day_page(repo, served):
    repo.set_issue("demo.1.2", status="in_progress", started_at=now_z())
    get = lambda: load(f"{served}/days/{TODAY}.html")
    until_shown(get, lambda p: "STARTED</span> Ask the owner" in p, time.time())


def test_feedback_add_ignores_other_docs_named_feedback(repo):
    feedback_env(repo)
    (repo.records / "docs").mkdir()
    other = f"---\ntype: doc\ntitle: Triage\ndate: 2026-10-01\nproject: demo\n---\n\nx\n"
    repo.write("docs/2026-10-01-triage-demo-feedback.md", other)
    repo.write("docs/2026-10-01-triage-pm-feedback.md", other)
    repo.commit()
    assert repo.pm("feedback", "add", "--project", "demo", "--text", "x").returncode == 0
    assert (repo.records / f"docs/{TODAY}-demo-feedback.md").exists()
    assert (repo.records / "docs/2026-10-01-triage-demo-feedback.md").read_text() == other


def test_feedback_add_with_text_leaves_stdin_unread(repo):
    """--text skips stdin, so a pipe that never closes does not hang the command."""
    feedback_env(repo)
    proc = subprocess.Popen([*PM, "feedback", "add", "--project", "demo", "--text", "x"],
                            cwd=repo.root, env=repo.env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True)
    try:
        assert proc.wait(timeout=20) == 0, proc.stderr.read()
    finally:
        proc.kill()
        proc.stdin.close()


def test_show_carries_feedback(repo):
    feedback_env(repo)
    hint = 'feedback: when pm gets in your way, run pm feedback add --project <p> --text "…"'
    out = repo.pm("show").stdout
    lines = out.splitlines()
    assert lines.index(hint) < lines.index("actions await you (0):") and "entries ->" not in out  # near the top
    for text in ("a", "b"):
        assert repo.pm("feedback", "add", "--project", "demo", "--text", text).returncode == 0
    out = repo.pm("show").stdout
    line = f"feedback: 2 entries -> http://localhost:8000/docs/{TODAY}-demo-feedback.html"
    lines = out.splitlines()
    assert out.count(line) == 1 and lines.index(hint) < lines.index(line) < next(i for i, l in enumerate(lines) if l.endswith("  sprints and decisions:"))
    data = json.loads(repo.pm("show", "--json").stdout)
    assert data["projects"][0]["feedback"] == [{"entries": 2, "url": line.split("-> ")[1]}]
