"""The pm flows agents and the owner depend on, end to end against a temp repo: setup, the sprint loop,
needs and replies, tasks, the shared records store, the pm service and its push."""

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
import urllib.request
from pathlib import Path

import pytest

from conftest import PM, VERSION, fake_env, project, sprint, write_config


def reply_body(text: str) -> str:
    """A site reply's comment text as the owner wrote it, without its <!-- pm-reply <id> --> mark line."""
    return re.sub(r"\s*<!-- pm-reply [A-Za-z0-9-]+ -->\s*$", "", text)

FRAME = "## Goal\n\nShip the thing.\n\n## Scope\n\n**In:** the thing.\n\n**Out:** other things.\n\n## Done when\n\n- It ships.\n"


def committed(repo, messages):
    """The latest store commits carry `messages`, newest first, and nothing is left uncommitted anywhere."""
    return (repo.store_log()[:len(messages)] == messages and repo.git("status", "--porcelain", cwd=repo.store) == ""
            and repo.git("status", "--porcelain") == "")


def refused(repo, *args, text="", match):
    """Run a command that must refuse: non-zero exit, the message, no file and no work-store change."""
    heads = lambda: [repo.git("rev-parse", "HEAD", cwd=d) for d in (repo.root, repo.store)]
    before, head = repo.snapshot(), heads()
    res = repo.pm(*args, text=text)
    assert res.returncode != 0, res.stdout
    assert re.search(match, res.stderr), res.stderr
    assert repo.snapshot() == before
    assert heads() == head
    assert repo.unchanged()
    return res


# ---------------------------------------------------------------- sprints: open, review, close


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


def store_head(repo):
    return repo.git("rev-parse", "--short", "HEAD", cwd=repo.store).strip()



def test_sprint_open_creates_epic_and_record(repo):
    res = repo.pm("sprint", "open", "demo", "--title", "Third: the end", text=FRAME)
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.3": {"id": "repo-demo.3", "type": "sprint", "title": "Sprint 3: Third: the end",
                                         "status": "open", "number": 3, "parent": "repo-demo"}}
    assert repo.items()["repo-demo.3"]["parent"] == "repo-demo"
    text = (repo.records / "sprints/demo-3.md").read_text()
    assert text.startswith('---\ntype: sprint\ntitle: "Third: the end"\nbead: repo-demo.3\n---\n')
    assert "Ship the thing." in text and "**Out:** other things." in text and "- It ships." in text
    # Every section and prompt line matches an existing sprint record (the fixture mirrors the real ones, written when
    # Progress came from Beads), but for where Progress comes from: the work store.
    skeleton = lambda t: [l.replace("from Beads", "from the work store") for l in t.splitlines()
                          if l.startswith(("#", ">"))]
    assert skeleton(text) == skeleton((repo.records / "sprints/demo-1.md").read_text())
    assert text.count("None yet.") == 3 and text.count("\n\nNot closed yet.\n") == 2


def test_sprint_and_project_close_once_their_reports_are_written_and_their_work_closed(repo):
    """A sprint with no PR review closes once its report is committed and every task is closed, naming the records
    commit; a project closes once its sprints are closed and its Outcome is committed."""
    repo.set_issue("repo-demo.1.2", status="closed", close_reason="Dismissed", closed_at="2026-10-02T12:00:00Z")
    refused(repo, "sprint", "close", "repo-demo.1",
            match=r"records/sprints/demo-1.md: the committed Delivery report Outcome is still 'Not closed yet.'")
    report(repo, "Done: shipped the thing.")
    res = repo.pm("sprint", "close", "repo-demo.1")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.1": {"status": "closed", "resolution": "done", "close_reason":
                                              f"Done: shipped the thing. (records commit {store_head(repo)})"}}
    repo.mark()  # refused() checks that nothing at all changed in the work store
    refused(repo, "project", "close", "demo",
            match=r"records/projects/demo.md: the committed ## Outcome is still 'Not closed yet.'")
    path = repo.records / "projects/demo.md"
    path.write_text(path.read_text().replace("Not closed yet.", "Shipped both sprints. The rest was retired."))
    repo.commit("outcome")
    refused(repo, "project", "close", "demo", match=r"open sprints in the project: repo-demo.2 \(Sprint 2: Second\)")
    assert repo.pm("sprint", "close", "repo-demo.2").returncode == 0
    repo.mark()

    res = repo.pm("project", "close", "demo")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo": {"status": "closed", "resolution": "done", "close_reason":
                                            f"Shipped both sprints. (commit {store_head(repo)})"}}


def test_new_records_feedback_projects_and_moves_write_what_they_say(repo):
    """Each write that makes or extends a record: its file, its one store commit, and its work-store change."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    writes = [
        (("doc", "new", "probe", "--title", "Probe: one", "--project", "demo"), "Body.",
         f"pm: created records/docs/{TODAY}-probe.md"),
        (("design", "new", "parser", "--title", "The parser", "--project", "demo"), "",
         "pm: created records/design/parser.md; fill in its sections by hand in the store, then pm commit -m \"…\" "
         "records/design/parser.md"),
        (("postmortem", "new", "outage", "--title", "Outage", "--sprint", "repo-demo.1"), "",
         f"pm: created records/postmortems/{TODAY}-outage.md; fill in its sections by hand in the store, then pm "
         f"commit -m \"…\" records/postmortems/{TODAY}-outage.md"),
        (("feedback", "add", "--project", "demo", "--sprint", "repo-demo.1"), "The refusal named no fix.",
         "pm: added feedback to records/docs/pm-feedback.md"),
        (("feedback", "add", "--project", "demo"), "Again.", "pm: added feedback to records/docs/pm-feedback.md"),
        (("finding", "add", "--sprint", "repo-demo.1", "A finding " + "long " * 20 + "enough to wrap."), "",
         "pm: added a finding to records/sprints/demo-1.md"),
    ]
    for args, text, message in writes:
        res = repo.pm(*args, text=text)
        assert res.returncode == 0, res.stderr
        assert committed(repo, [message])
    assert repo.unchanged()
    feedback = (repo.records / "docs/pm-feedback.md").read_text()
    assert feedback.count("UTC, session `sess-1`") == 2
    assert "About project `demo`, sprint `repo-demo.1`.\n\nThe refusal" in feedback
    res = repo.pm("task", "move", "repo-demo.1.2", "--to", "repo-demo.2", text="Moved on.\nIt fits sprint 2.")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.1.2": {"parent": "repo-demo.2"}}
    assert committed(repo, ["pm: moved repo-demo.1.2 from repo-demo.1 to repo-demo.2 and added a sprint decision to "
                            "records/sprints/demo-1.md"])
    repo.mark()
    res = repo.pm("project", "open", "fresh", "--title", "Fresh", text="Why we do it.")
    assert res.returncode == 0, res.stderr
    [(new, item)] = repo.changes().items()
    assert item == {"id": new, "type": "project", "title": "Fresh", "status": "open"}
    assert committed(repo, [f"pm: opened project fresh: epic {new}, record records/projects/fresh.md"])
    assert repo.pm("check").returncode == 0


def test_feedback_about_any_project_goes_to_the_repo_s_one_feedback_doc(repo):
    """pm feedback add appends to records/docs/pm-feedback.md whatever the entry is about: two projects and none, each
    entry tagged with its project; pm show and pm show --project link that one doc. A per-project doc as pm wrote
    them before fails the check, and pm feedback add refuses until it is merged."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    res = repo.pm("show")
    assert res.returncode == 0, res.stderr
    assert "feedback: none yet; when pm gets in your way, run pm feedback add [--project NAME]" in res.stdout
    assert repo.pm("project", "open", "fresh", "--title", "Fresh", text="Why we do it.").returncode == 0
    for args, text in [(("--project", "demo", "--task", "repo-demo.1.1"), "Demo's refusal named no fix."),
                       (("--project", "fresh"), "Fresh had no sprint move."), ((), "A rule cost a retry.")]:
        res = repo.pm("feedback", "add", *args, text=text)
        assert res.returncode == 0, res.stderr
        assert committed(repo, ["pm: added feedback to records/docs/pm-feedback.md"])
    assert [p.name for p in (repo.records / "docs").iterdir()] == ["pm-feedback.md"]
    doc = (repo.records / "docs/pm-feedback.md").read_text()
    assert doc.startswith(f"---\ntype: doc\ntitle: pm feedback\ndate: {TODAY}\n---\n\n")
    entries = re.split(r"(?m)^### .* UTC, session `sess-1`\n\n", doc)[1:]
    assert entries == ["About project `demo`, task `repo-demo.1.1`.\n\nDemo's refusal named no fix.\n\n",
                       "About project `fresh`.\n\nFresh had no sprint move.\n\n", "A rule cost a retry.\n"]
    site = re.search(r"site: (\S+) ", repo.pm("show").stdout)[1]
    line = f"feedback: 3 entries -> {site}/docs/pm-feedback.html; when pm gets in your way"
    for args in [(), ("--project", "demo"), ("--project", "fresh")]:
        res = repo.pm("show", *args)
        assert res.returncode == 0 and line in res.stdout, res.stdout
    assert json.loads(repo.pm("show", "--json").stdout)["feedback"] == {"entries": 3,
                                                                       "url": f"{site}/docs/pm-feedback.html"}
    assert repo.pm("check").returncode == 0

    old = f"docs/{TODAY}-demo-feedback.md"
    repo.write(old, f"---\ntype: doc\ntitle: pm feedback\ndate: {TODAY}\nproject: demo\n---\n\nOlder entries.\n")
    repo.commit("a per-project feedback doc, as pm wrote them before")
    check = repo.pm("check")
    assert check.returncode != 0 and "a repo keeps one pm feedback doc, records/docs/pm-feedback.md" in check.stderr
    repo.mark()  # refused() checks that nothing at all changed in the work store
    refused(repo, "feedback", "add", "--project", "demo", text="More.",
            match=rf"{re.escape(old[:-3])}: a repo keeps one pm feedback doc, records/docs/pm-feedback.md; move this "
                  r"doc's entries into it")


def test_feedback_add_refuses_to_overwrite_another_record_at_the_feedback_doc_s_path(repo):
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    (repo.records / "docs").mkdir()
    repo.write("docs/pm-feedback.md", "---\ntype: day\ndate: 2026-10-01\n---\n\nKeep this.\n")
    repo.commit("a record at the feedback doc's path")
    refused(repo, "feedback", "add", text="x",
            match=r"records/docs/pm-feedback.md exists but is no pm feedback doc \(type: doc\); move it")


@pytest.mark.integration
def test_sprint_closes_after_its_pr_merges(repo):
    """The whole loop: a written report, a review under the sprint, a refusal while the review is open, the review
    closed as merged once the PR is on main, then a close that stamps the merge and names its records commit."""
    repo.set_issue("repo-demo.1.2", status="closed", close_reason="Dismissed", closed_at="2026-10-02T12:00:00Z")
    report(repo, "Done: shipped the thing.")
    res = repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F")
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["parent"] == "repo-demo.1" and "under repo-demo.1;" in res.stdout
    repo.mark()
    refused(repo, "sprint", "close", "repo-demo.1", match=r"open tasks in the sprint: repo-demo.1.3 \(open\)")
    assert repo.pm("action", "done", "repo-demo.1.3", "--reason", f"merged as {SHA}").returncode == 0
    repo.mark()
    res = repo.pm("sprint", "close", "repo-demo.1")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["[SPRINT] demo sprint 1: closed, merged as 8f5c618"])
    assert repo.changes() == {"repo-demo.1": {"status": "closed", "resolution": "done", "close_reason":
                                         f"Done: shipped the thing. (records commit {store_head(repo)})"}}
    assert repo.pm("check").returncode == 0
    page = repo.page("sprints/demo-1.html")
    assert "<p>Done: shipped the thing.</p>\n<p>Merged as 8f5c618 (PR #12).</p>" in page


# ---------------------------------------------------------------- needs: answer with a decision or close

DECISION = "Use the small parser."
REASON = "It is enough for the record set and adds no dependency."
ADD = ("decision", "add", "--level", "project", "--project", "demo")
TODAY = __import__("datetime").date.today().isoformat()


NEED = ("--question", "Which parser should we use?", "--fact", "Records hold no tables yet.",
        "--option", "small", "The small parser.", "--cost", "small", "no tables.",
        "--option", "full", "The full parser.", "--cost", "full", "a new dependency.",
        "--default", "small", "It is cheap.")



SITE_URL = ("--question", "Where does pm keep the public site URL?",
            "--fact", "Today, each clone keeps the URL in its git config.",
            "--fact", "You asked why the URL is not in `.pm/config.toml`.",
            "--option", "a", "In `.pm/config.toml`. A pm command still writes it.", "--cost", "a", "the repo has one URL.",
            "--option", "b", "In each clone, as today.", "--cost", "b", "you must give the URL to each new clone.",
            "--default", "a", "All clones then give the same link.")


def test_decision_need_writes_its_flags_in_the_one_layout(repo):
    """The design page's example, given as flags, becomes the description that page shows."""
    res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *SITE_URL)
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["description"] == (
        "**Question:** Where does pm keep the public site URL?\n\n**Facts:**\n\n"
        "- Today, each clone keeps the URL in its git config.\n- You asked why the URL is not in `.pm/config.toml`.\n\n"
        "**Options:**\n\n- **(a) In `.pm/config.toml`.** A pm command still writes it. *Cost:* the repo has one URL.\n"
        "- **(b) In each clone, as today.** *Cost:* you must give the URL to each new clone.\n\n"
        "**Default:** (a). All clones then give the same link.")


def without(flags: tuple, *drop: tuple[str, str]) -> tuple:
    """flags without each named (flag, first value), e.g. without(SITE_URL, ("--option", "b")) drops --option b."""
    out, i = [], 0
    while i < len(flags):
        n = next(j for j in range(i + 1, len(flags) + 1) if j == len(flags) or flags[j].startswith("--"))
        if (flags[i], flags[i + 1]) not in drop:
            out += flags[i:n]
        i = n
    return tuple(out)


@pytest.mark.parametrize("flags, error", [
    (without(SITE_URL, ("--question", "Where does pm keep the public site URL?")), "the following arguments are required: --question"),
    (without(SITE_URL, ("--option", "b"), ("--cost", "b")), "give at least two --option flags"),
    (without(SITE_URL, ("--cost", "a")), "option a has no cost; add --cost a"),
    (SITE_URL + ("--cost", "c", "more."), "--cost c names no option; the labels are a, b"),
    (SITE_URL + ("--cost", "a", "more."), "option a has two --cost flags"),
    (SITE_URL + ("--option", "a", "Again."), "two options have the label a"),
    (SITE_URL[:-2] + ("z", "Why."), "--default z names no option; the labels are a, b"),
    (SITE_URL + ("--fact", "One.\nTwo."), "--fact has more than one line"),
    (SITE_URL + ("--default", "b", "Other."), "give exactly one --default"),
    (SITE_URL + ("--fact", "Please review and merge PR #12, " + " ".join(["word"] * 30) + "."),
     "raise it with the review form"),
    (SITE_URL + ("--fact", " ".join(["word"] * 26) + "."), "in fact 3 has 26 words; the limit is 25"),
])
def test_decision_need_refuses_a_malformed_part_and_writes_nothing(repo, flags, error):
    res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *flags)
    assert res.returncode != 0 and error in res.stderr, res.stderr
    assert repo.unchanged() and "repo-demo.1.3" not in repo.items()


ACTION = "Restart the site on port 8767, which the new proxy expects.\n"


def assert_answered(repo, need_id, resolution, text):
    """The only change is the need closed with `resolution` and one new comment holding the answer `text`."""
    change = repo.changes()
    assert list(change) == [need_id] and set(change[need_id]) <= {"status", "resolution", "close_reason", "comments"}
    assert (change[need_id]["status"], change[need_id]["resolution"]) == ("closed", resolution)
    assert len(change[need_id]["comments"]) == 1 and change[need_id]["comments"][0]["text"].endswith(text)


def test_decision_add_need_closes_need_and_records_decision(repo):
    path = repo.records / "projects/demo.md"
    res = repo.pm(*ADD, "--need", "repo-demo.1.2", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    text = f"{DECISION}\n{REASON}\nAnswers `repo-demo.1.2`."
    assert_answered(repo, "repo-demo.1.2", "answered", text)
    assert repo.items()["repo-demo.1.2"]["status"] == "closed"
    assert path.read_text().split("## Design pages")[0].rstrip().endswith(
        f"::: decision {{source=owner date={TODAY}}}\n{text}\n:::")
    assert repo.pm("check").returncode == 0


@pytest.mark.parametrize("flags, error", [
    (("--decision", DECISION), "the following arguments are required: --reason"),
    (("--decision", f"{DECISION}\nMore.", "--reason", REASON), "--decision has more than one line"),
    (("--decision", DECISION, "--reason", " "), "--reason is empty"),
    (("--decision", DECISION, "--reason", "::: result"), "--reason starts with ':::'"),
])
def test_decision_add_refuses_a_malformed_part_and_writes_nothing(repo, flags, error):
    """The decision and its reason are flags, one line each; a malformed one is refused before any write."""
    heads = repo.store_log()
    res = repo.pm(*ADD, *flags)
    assert res.returncode != 0 and error in res.stderr, res.stderr
    assert repo.unchanged() and repo.store_log() == heads
    assert repo.git("status", "--porcelain", cwd=repo.store) == ""


ANSWER = "Port 8767."


def test_decision_close_closes_small_answer_without_record(repo):
    """A small answer closes the need with the answer and the reason, labelled no-decision, and writes no record;
    the render accepts it although no decision cites it."""
    heads = repo.store_log()
    res = repo.pm("decision", "close", "repo-demo.1.2", "--reason", "It picks a port and sets no rule.", text=ANSWER)
    assert res.returncode == 0, res.stderr
    assert_answered(repo, "repo-demo.1.2", "no-decision", f"{ANSWER}\n\nNo decision record: It picks a port and sets no rule.")
    need = repo.items()["repo-demo.1.2"]
    assert (need["status"], need["type"], need["resolution"], need["labels"]) == ("closed", "need", "no-decision", [])
    assert repo.store_log() == heads and repo.git("status", "--porcelain", cwd=repo.store) == ""
    assert repo.pm("check").returncode == 0


def test_a_decision_recorded_for_a_no_decision_need_marks_it_answered(repo):
    """pm decision close names its undo: a decision that cites the need, which marks it answered."""
    res = repo.pm("decision", "close", "repo-demo.1.2", "--reason", "It sets no rule.", text=ANSWER)
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip().endswith("; if it sets a rule after all, record it with pm decision add --need "
                                       "repo-demo.1.2, which marks the need answered")
    res = repo.pm(*ADD, "--need", "repo-demo.1.2", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("marked need repo-demo.1.2 answered and added a source=owner project decision")
    assert repo.items()["repo-demo.1.2"]["resolution"] == "answered"
    assert repo.pm("check").returncode == 0


# ---------------------------------------------------------------- pm task add


def test_task_add_creates_task_in_sprint(repo):
    res = repo.pm("task", "add", "--sprint", "repo-demo.1", "--title", "Write the parser", text="Small and fast.\n")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.1.3": {"id": "repo-demo.1.3", "type": "task", "title": "Write the parser",
                                           "description": "Small and fast.", "status": "open", "parent": "repo-demo.1"}}
    assert "created task repo-demo.1.3 in sprint repo-demo.1" in res.stdout
    assert repo.items()["repo-demo.1.3"]["parent"] == "repo-demo.1"
    assert repo.git("status", "--porcelain") == ""
    repo.mark()
    res = repo.pm("task", "add", "--sprint", "repo-demo.2", "--title", "No description")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.2.1": {"id": "repo-demo.2.1", "type": "task", "title": "No description",
                                           "status": "open", "parent": "repo-demo.2"}}


# ---------------------------------------------------------------- pm sprint move

REASON = "It belongs to the site.\nThe site project owns every page."


def site_project(repo):
    """Seed a second open project, site, with sprint 1 and their records, and a task in demo-1 that sess-a holds."""
    repo.add_issue({"id": "repo-site", "title": "Site", "status": "open", "issue_type": "epic",
                    "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    repo.add_issue({"id": "repo-site.1", "title": "Sprint 1: Pages", "status": "open", "issue_type": "epic",
                    "parent": "repo-site", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    add_task(repo)
    repo.write("projects/site.md", project("Site", "repo-site"))
    repo.write("sprints/site-1.md", sprint("Pages", "repo-site.1"))
    repo.commit("site project")


def test_sprint_move_takes_the_sprint_whole_and_a_rerun_finishes_a_move_cut_short(repo):
    """The sprint's Done when: a sprint with two tasks, a need, a decision and a finding moves to another project.
    The records step fails after the work-store write (the store's sprints/ read-only: the fault point); pm show is
    consistent then, and the same command run again finishes the move from the move note."""
    site_project(repo)
    for args in (("decision", "add", "--level", "sprint", "--sprint", "repo-demo.1", "--decision", "Parse first.",
                  "--reason", "The site needs tables."), ("finding", "add", "--sprint", "repo-demo.1", "It parses 9 of 10.")):
        assert repo.pm(*args).returncode == 0
    old = (repo.records / "sprints/demo-1.md").read_text()
    assert "Parse first." in old and "It parses 9 of 10." in old
    repo.mark()
    files = repo.record_files()
    sprints = repo.store / "sprints"
    sprints.chmod(0o555)
    try:
        res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=REASON)
    finally:
        sprints.chmod(0o755)
    assert res.returncode != 0
    assert "the work store holds the move (repo-demo.1 is sprint 2 of site): run the same command again" in res.stderr
    assert repo.record_files() == files and repo.git("status", "--porcelain", cwd=repo.store) == ""
    note = ("pm sprint move: from repo-demo sprint 1 to repo-site sprint 2\n" + REASON)
    assert repo.changes() == {"repo-demo.1": {"parent": "repo-site", "number": 2, "title": "Sprint 2: First",
                                              "comments": [{"kind": "note", "author": "pm", "text": note}]}}

    def placed():
        """pm show puts the sprint under site only, named by its number, and the old id shows it there."""
        assert "Sprint 2: First  repo-demo.1  " in repo.pm("show", "--project", "site").stdout
        demo = repo.pm("show", "--project", "demo")
        assert demo.returncode == 0 and "repo-demo.1 " not in demo.stdout and "Sprint 2: Second  .2  " in demo.stdout
        top = repo.pm("show")
        assert top.returncode == 0 and "decision repo-demo.1.2  Ask the owner  (sprint 2)" in top.stdout
        shown = repo.pm("show", "repo-demo.1").stdout
        assert "parent: repo-site  project  open  Site" in shown and "pm sprint move: from repo-demo" in shown
        assert repo.pm("check").returncode == 0

    placed()
    assert "records/sprints/demo-1.md" in repo.pm("show", "--sprint", "repo-demo.1").stdout  # not renamed yet

    repo.mark()
    res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=REASON)
    assert res.returncode == 0, res.stderr
    assert repo.unchanged(), "the rerun writes the records step alone"
    assert committed(repo, ["pm: finished moving sprint repo-demo.1 from demo (sprint 1) to site (sprint 2): record "
                            "records/sprints/site-2.md, a decision in both projects (the work store held it already)"])
    assert not (repo.records / "sprints/demo-1.md").exists()
    assert (repo.records / "sprints/site-2.md").read_text() == old
    demo, site = ((repo.records / f"projects/{n}.md").read_text() for n in ("demo", "site"))
    assert f"Moved sprint 1 (repo-demo.1) to site as sprint 2: {REASON}\n:::" in demo
    assert f"Took in sprint 1 of demo (repo-demo.1) as sprint 2: {REASON}\n:::" in site
    placed()
    assert "records/sprints/site-2.md" in repo.pm("show", "--sprint", "repo-demo.1").stdout
    items = repo.items()
    assert items["repo-demo.1.3"]["holder"]["session"] == "sess-a" and items["repo-demo.1.3"]["parent"] == "repo-demo.1"
    assert items["repo-demo.1.2"]["parent"] == "repo-demo.1"
    # The old record path still finds the record.
    res = repo.pm("show", "--record", "records/sprints/demo-1.md", "--section", "Findings")
    assert res.returncode == 0 and "It parses 9 of 10." in res.stdout, res.stderr
    refused(repo, "sprint", "move", "repo-demo.1", "--to", "site", text=REASON,
            match=r"sprint repo-demo.1 is in project site already, as sprint 2 \(records/sprints/site-2.md\)")
    # Neither project mints a number it used again: demo's 1 moved away, site's 2 moved in.
    for name, n in (("demo", 3), ("site", 3)):
        res = repo.pm("sprint", "open", name, "--title", "Next", text=FRAME)
        assert res.returncode == 0 and f"opened sprint {n} of {name}" in res.stdout, res.stderr


def test_sprint_move_in_one_go_and_its_refusals(repo):
    site_project(repo)
    refused(repo, "sprint", "move", "repo-demo.1.3", "--to", "site", text=REASON,
            match="repo-demo.1.3 is not a sprint in the work store")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "old", text=REASON,
            match=r"project old \(repo-old\) is closed; move the sprint to an open project")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "nope", text=REASON, match="no project record named 'nope'")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "site", text="One line.", match="the decision body is a single line")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "demo", text=REASON,
            match=r"sprint repo-demo.2 is in project demo already, as sprint 2")
    res = repo.pm("sprint", "move", "repo-demo.2", "--to", "site", text=REASON)
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: moved sprint repo-demo.2 from demo (sprint 2) to site (sprint 2): record "
                            "records/sprints/site-2.md, a decision in both projects"])
    assert repo.items()["repo-demo.2"]["number"] == 2 and repo.items()["repo-demo.2"]["parent"] == "repo-site"
    assert repo.pm("sprint", "move", "repo-demo.2", "--to", "demo", text=REASON).returncode == 0
    assert (repo.records / "sprints/demo-3.md").exists(), "back in demo, it takes a new number: 2 moved away"
    assert repo.pm("sprint", "close", "repo-demo.2").returncode == 0
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "site", text=REASON,
            match="sprint repo-demo.2 is closed; only an open sprint moves")


# ---------------------------------------------------------------- pm task close

def add_task(repo, issue_id="repo-demo.1.3", **fields):
    """A task session sess-a claimed, as bd exports it."""
    repo.add_issue(dict(
        {"id": issue_id, "title": "Write the parser", "status": "in_progress", "issue_type": "task",
         "parent": "repo-demo.1", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z",
         "started_at": "2026-10-01T12:00:00Z",
         "metadata": {"claimed_by": "sess-a", "claimed_at": "2026-10-01T12:00:00Z"}}, **fields))


def test_task_close_names_head(repo):
    add_task(repo)
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    res = repo.pm("task", "close", "repo-demo.1.3", "--reason", "Parser written.")
    assert res.returncode == 0, res.stderr
    assert res.stderr == ""
    assert repo.changes() == {"repo-demo.1.3": {"status": "closed", "resolution": "done", "holder": None,
                                                "close_reason": f"Parser written. (commit {head})",
                                                "closed_by": "sess-a"}}
    assert repo.items()["repo-demo.1.3"]["status"] == "closed"


def test_task_close_dropped_records_why_and_needs_no_commit(repo):
    """A dropped task closes as not done (resolution dismissed) with its reason: no commit looked up, no warning, even
    in a dirty tree; pm show shows it as dropped."""
    add_task(repo)
    (repo.root / "wip.txt").write_text("not committed\n")
    refused(repo, "task", "close", "repo-demo.1.3", "--dropped", match=r"--dropped needs --reason")
    refused(repo, "task", "close", "repo-demo.1.3", "--dropped", "--reason", "x", "--commit", "HEAD",
            match=r"--dropped closes repo-demo\.1\.3 as not done, so no commit holds its work; drop --commit")
    res = repo.pm("task", "close", "repo-demo.1.3", "--dropped", "--reason", "A library parses it.")
    assert res.returncode == 0, res.stderr
    assert res.stderr == ""
    assert res.stdout.strip() == "dropped repo-demo.1.3: A library parses it."
    assert repo.changes() == {"repo-demo.1.3": {"status": "closed", "resolution": "dismissed", "holder": None,
                                                "close_reason": "A library parses it.", "closed_by": "sess-a"}}
    assert "  dropped  repo-demo.1.3  Write the parser" in repo.pm("show", "--sprint", "repo-demo.1").stdout
    # sprint 1 holds a done task, an open need and the dropped task: the dropped one counts in neither number
    assert "Sprint 1: First  .1  running  1/2 done" in repo.pm("show", "--project", "demo").stdout


def test_task_close_with_commit_warns_nothing_about_a_dirty_tree(repo):
    """--commit names the work, so the tree's uncommitted changes say nothing about it; without --commit, HEAD stands
    in for the work and a dirty tree still warns."""
    add_task(repo)
    add_task(repo, "repo-demo.1.4")
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    (repo.root / "wip.txt").write_text("not committed\n")
    res = repo.pm("task", "close", "repo-demo.1.3", "--commit", "HEAD")
    assert res.returncode == 0, res.stderr
    assert res.stderr == ""
    assert repo.items()["repo-demo.1.3"]["close_reason"] == f"Done (commit {head})"
    res = repo.pm("task", "close", "repo-demo.1.4")
    assert res.returncode == 0, res.stderr
    assert "warning: the working tree has uncommitted changes" in res.stderr


def test_task_close_resolves_another_repos_commit_or_a_pr_with_gh(repo):
    add_task(repo)
    add_task(repo, "repo-demo.1.4")
    add_task(repo, "repo-demo.1.5")
    add_task(repo, "repo-demo.1.6")
    sha = "0123456789abcdef0123456789abcdef01234567"
    repo.set_commit("Yeeef/pm", sha)
    merged, open_pr = "https://github.com/Yeeef/pm/pull/7", "https://github.com/Yeeef/pm/pull/9"
    repo.set_pr(merged, "MERGED", sha)
    repo.set_pr(open_pr, "OPEN")
    refused(repo, "task", "close", "repo-demo.1.3", "--commit", "Yeeef/pm@fedcba9",
            match=r"--commit Yeeef/pm@fedcba9 is not a commit of Yeeef/pm on GitHub: gh api "
                  r"repos/Yeeef/pm/commits/fedcba9 failed: gh: No commit found for SHA: fedcba9")
    refused(repo, "task", "close", "repo-demo.1.3", "--commit", "https://github.com/Yeeef/pm/pull/8",
            match=r"--commit https://github.com/Yeeef/pm/pull/8 is not a pull request on GitHub: gh pr view")
    repo.set_pr("https://github.com/Yeeef/pm/pull/10", "CLOSED")
    refused(repo, "task", "close", "repo-demo.1.3", "--commit", "https://github.com/Yeeef/pm/pull/10",
            match=r"--commit https://github.com/Yeeef/pm/pull/10 was closed without merging, so it holds no work")
    refused(repo, "task", "close", "repo-demo.1.3", "--commit", "no-such-ref",
            match=r"--commit no-such-ref is not a commit of this repo, an OWNER/REPO@SHA on GitHub or a PR URL")
    res = repo.pm("task", "close", "repo-demo.1.3", "--reason", "Parser written.", "--commit", "Yeeef/pm@0123456")
    assert res.returncode == 0, res.stderr
    assert repo.pm("task", "close", "repo-demo.1.4", "--commit", merged).returncode == 0
    assert repo.pm("task", "close", "repo-demo.1.5", "--commit", open_pr).returncode == 0
    items = repo.items()
    assert items["repo-demo.1.3"]["close_reason"] == "Parser written. (commit Yeeef/pm@0123456)"
    assert items["repo-demo.1.4"]["close_reason"] == f"Done (PR {merged}, merged as Yeeef/pm@0123456)"
    assert items["repo-demo.1.5"]["close_reason"] == f"Done (PR {open_pr})"
    # a branch of this repo named like OWNER/REPO@SHA is this repo's commit: no gh call
    repo.git("branch", "Yeeef/pm@cafe1234")
    assert repo.pm("task", "close", "repo-demo.1.6", "--commit", "Yeeef/pm@cafe1234").returncode == 0
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    assert repo.items()["repo-demo.1.6"]["close_reason"] == f"Done (commit {head})"


def test_task_close_refuses_a_task_another_live_session_holds(repo):
    add_task(repo)  # held by sess-a
    transcript(repo, "sess-a", 60)
    refused(repo, "task", "close", "repo-demo.1.3",
            match=r"repo-demo\.1\.3 is held by live session sess-a \(claimed \S+ ago; its transcript was written in the "
                  r"last 30 minutes\); leave it, or ask that session to close it, or to release it with pm task release "
                  r"repo-demo\.1\.3")
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-a")  # the holder itself closes it
    res = repo.pm("task", "close", "repo-demo.1.3")
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["status"] == "closed"


def test_task_move_takes_a_task_from_directly_under_a_project_into_its_sprint(repo):
    """A task filed directly under a project joins one of the project's sprints; the scope it adds is a decision in
    the sprint it joins."""
    repo.add_issue({"id": "repo-demo.3", "title": "Backlog item", "status": "open", "issue_type": "task",
                    "parent": "repo-demo", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    refused(repo, "task", "move", "repo-demo.3", "--to", "repo-demo", text="Into the project.\nNot a sprint.",
            match=r"repo-demo\.3 is already directly under project demo; move it into one of that project's open "
                  r"sprints")
    res = repo.pm("task", "move", "repo-demo.3", "--to", "repo-demo.1", text="Sprint 1 needs it.\nThe parser uses it.")
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.3": {"parent": "repo-demo.1"}}
    assert committed(repo, ["pm: moved repo-demo.3 from project repo-demo to repo-demo.1 and added a sprint decision "
                            "to records/sprints/demo-1.md"])
    record = (repo.records / "sprints/demo-1.md").read_text()
    assert "Moved repo-demo.3 into this sprint from project repo-demo: Sprint 1 needs it.\nThe parser uses it." in record


# ---------------------------------------------------------------- the store: pm init in a clone

GIT_ENV = dict(__import__("os").environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")


def git_in(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, check=True, capture_output=True, text=True).stdout


def init_in(cwd, *args):
    """pm init with the fakes, the installed pm on PATH (the hooks pm writes call `pm`) and the service under the fake
    supervisor on a port free when first asked for."""
    env = fake_env(cwd.parent, GIT_ENV)
    port = cwd.parent / "port"
    if not port.exists():
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            port.write_text(str(s.getsockname()[1]))
    env = dict(env, PORT=port.read_text())
    return subprocess.run([*PM, "init", *args], cwd=cwd, env=env, capture_output=True, text=True)


def schedule_line(clone: Path, home: Path, state: str) -> str:
    """pm where's schedule line for a clone, on this platform's scheduler as the fakes present it."""
    name = f"local.pm-push.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.timer")
    return f"schedule  {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


def sched_calls(tmp_path) -> list[list[str]]:
    log = tmp_path / "sched.log"
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def service_line(clone: Path, home: Path, state: str) -> str:
    """pm where's service line for a clone, on this platform's supervisor as the fakes present it."""
    name = f"local.pm.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.service")
    return f"service   {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


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


@pytest.mark.integration
def test_init_on_a_fresh_clone_checks_out_store_links_records_and_starts_the_service(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    res = init_in(clone)
    assert res.returncode == 0, res.stderr
    # the remote had no work store, so pm init made it and pushed it
    assert git_in(clone, "ls-remote", "origin", "refs/pm/work").strip()
    assert f"created the work store at {clone}/.pm/store/work and pushed it to origin's refs/pm/work" in res.stdout
    assert git_in(clone, "config", "core.hooksPath").strip() == str(clone / ".pm/hooks")
    assert git_in(clone / ".pm/store/records", "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (clone / "records").is_symlink() and (clone / "records/sprints/demo-1.md").read_text() == "one\n"
    assert git_in(clone, "status", "--porcelain") == ""
    config = (clone / ".git/config").read_text()
    where = subprocess.run([*PM, "where"], cwd=clone,
                           env=fake_env(tmp_path, GIT_ENV), capture_output=True, text=True).stdout.splitlines()
    assert where[0].startswith(f"pm        {VERSION}  ")
    assert where[1] == f"store     {clone}/.pm/store/records  branch records, 0 ahead, 0 behind origin/records (as of the last fetch)"
    assert where[2] == f"checkout  {clone}  branch main, records link set up"
    # the clone's half of pm init ends with the service; the repo's files, installed already, stay as they are
    port = (tmp_path / "port").read_text()
    assert service_line(clone, tmp_path / "home", f"running; the site answers on :{port}") in where, where
    assert f"push      {clone}/.pm/run/push.log  no push recorded yet" in where
    assert [c for c in sched_calls(tmp_path) if c[1:2] == ["bootstrap"] or c[2:3] == ["enable"]]
    again = init_in(clone)
    assert again.returncode == 0 and again.stdout.startswith("already set up"), again.stderr
    assert (clone / ".git/config").read_text() == config


# ---------------------------------------------------------------- the store: shared across worktrees

@pytest.mark.integration
def test_write_on_one_branch_is_visible_on_another_without_merge(repo):
    wt = repo.worktree("feature-x")
    res = repo.pm("finding", "add", "--sprint", "repo-demo.1", "Seen from every branch.", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"])
    assert repo.git("status", "--porcelain", cwd=wt) == ""
    # The main checkout is on main, which merged nothing.
    assert repo.git("rev-parse", "--abbrev-ref", "HEAD").strip() == "main"
    assert repo.git("log", "--format=%s").splitlines() == ["code"]
    assert "- Seen from every branch." in repo.pm("show", "--sprint", "repo-demo.1").stdout
    assert repo.pm("check").returncode == 0
    assert "Seen from every branch." in repo.page("sprints/demo-1.html")


def test_concurrent_writes_from_two_worktrees_land_as_separate_commits(repo):
    wts = [repo.worktree("feature-a"), repo.worktree("feature-b")]
    procs = [subprocess.Popen([*PM, "finding", "add", "--sprint", "repo-demo.1",
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


# Every command that once read stdin, with its minimal arguments and a --text it accepts.
TEXT_COMMANDS = [
    (["decision", "close", "repo-demo.1.2", "--reason", "sets no rule"], "Small."),
    (["action", "need", "--title", "Q", "--parent", "repo-demo.1"], "Restart the site: the proxy moved."),
    (["doc", "new", "probe", "--title", "Probe", "--project", "demo"], "Body."),
    (["project", "open", "fresh", "--title", "Fresh"], "Why we do it."),
    (["sprint", "open", "demo", "--title", "Third"], FRAME),
    (["task", "add", "--sprint", "repo-demo.1", "--title", "T"], "Small and fast."),
    (["task", "move", "repo-demo.1.2", "--to", "repo-demo.2"], "Moved on.\nIt fits sprint 2."),
    (["feedback", "add", "--project", "demo"], "The refusal named no fix."),
]


@pytest.mark.parametrize("with_text", [False, True], ids=["no-text", "text"])
@pytest.mark.parametrize("args, text", TEXT_COMMANDS, ids=[" ".join(a[:2]) for a, _ in TEXT_COMMANDS])
def test_no_command_waits_on_an_open_stdin(repo, tmp_path, args, text, with_text):
    """Sprint 60: in Claude Code the shell's stdin is a socket that never closes, so a pm that read stdin hung
    forever. pm reads stdin only for --text-file -: each command returns, with its output or a refusal, while stdin
    stays open."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")  # feedback add names its session
    out, err = tmp_path / "out", tmp_path / "err"
    cmd = [*PM, *args, *([f"--text={text}"] if with_text else [])]
    with out.open("w") as o, err.open("w") as e:
        proc = subprocess.Popen(cmd, cwd=repo.root, env=repo.env, stdin=subprocess.PIPE, stdout=o, stderr=e)
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pytest.fail(f"pm {' '.join(args)} still waits on its open stdin after 5 s")
        finally:
            proc.kill()
            proc.stdin.close()
    said = err.read_text()
    if with_text or args[:2] == ["task", "add"]:
        assert proc.returncode == 0 and out.read_text(), said
    else:
        assert proc.returncode == 1 and re.match(r"error: .*--text", said), said


def run_task_add(repo, *args, stdin, given=None):
    """pm task add with the given stdin (a pipe written with `given` and closed, if given); it must return in 5 s."""
    proc = subprocess.Popen([*PM, "task", "add", "--sprint", "repo-demo.1", "--title", "T", *args], cwd=repo.root,
                            env=repo.env, stdin=stdin, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        out, err = proc.communicate(given, timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        pytest.fail("pm task add --text-file still waits after 5 s")
    return proc.returncode, out, err


def test_text_file_dash_reads_a_heredoc(repo):
    """A quoted heredoc is a pipe written and closed: its body comes through whole, quotes, backticks and $ kept."""
    body = "Parse `x`, it's $5 (or \"more\").\nSecond line )."
    code, _, err = run_task_add(repo, "--text-file", "-", stdin=subprocess.PIPE, given=body + "\n")
    assert code == 0, err
    assert repo.items()["repo-demo.1.3"]["description"] == body


def test_text_file_dash_refuses_a_socket_at_once(repo):
    """Claude Code's shell stdin is a socket that never closes: --text-file - refuses it without reading."""
    ours, theirs = socket.socketpair()
    try:
        code, _, err = run_task_add(repo, "--text-file", "-", stdin=theirs)
    finally:
        ours.close()
        theirs.close()
    assert code == 1 and err.startswith("error: --text-file - reads stdin, which here is not a pipe or a file; pass "
                                        "the body with a quoted heredoc: pm … --text-file - <<'EOF' … EOF"), err
    assert repo.unchanged()


def test_text_file_reads_a_file_and_refuses_with_text(repo, tmp_path):
    path = tmp_path / "body.md"
    path.write_text("From a file.\n", encoding="utf-8")
    refused(repo, "task", "add", "--sprint", "repo-demo.1", "--title", "T", "--text-file", str(path), text="x",
            match=r"argument --text: not allowed with argument --text-file")
    refused(repo, "task", "add", "--sprint", "repo-demo.1", "--title", "T", "--text-file", str(tmp_path / "none"),
            match=r"error: --text-file .*/none: no such file")
    code, _, err = run_task_add(repo, "--text-file", str(path), stdin=subprocess.DEVNULL)
    assert code == 0, err
    assert repo.items()["repo-demo.1.3"]["description"] == "From a file."


def test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does(repo):
    assert repo.pm("where", "records").stdout == f"{repo.store}\n"
    path = repo.store / "sprints/demo-1.md"
    text = path.read_text()
    path.write_text(text.replace("## Findings", "## Finds"))
    refused(repo, "commit", "-m", "Break it", "records/sprints/demo-1.md",
            match=r"error: sprints/demo-1: sprint record needs a '## Findings'")
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n"
    path.write_text(text.replace("Ship it.", "Ship it soon."))
    res = repo.pm("commit", "-m", "Sharpen the sprint 1 goal", "records/sprints/demo-1.md")
    assert res.returncode == 0, res.stderr
    assert "committed records/sprints/demo-1.md as " in res.stdout
    assert committed(repo, ["Sharpen the sprint 1 goal"])
    refused(repo, "commit", "-m", "Nothing", "records/sprints/demo-1.md", match=r"error: nothing to commit in .*\.pm/store/records")


def test_pm_check_and_commit_take_image_files_beside_a_record(repo):
    """A figure kept next to its doc is checked with the records and committed byte for byte."""
    docs = repo.store / "docs"
    docs.mkdir()
    svg, png = b'<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"/>', bytes(range(256))
    (docs / "fig.svg").write_bytes(svg)
    (docs / "plot.png").write_bytes(png)
    (docs / "2026-10-07-figures.md").write_text("---\ntype: doc\ntitle: Figures\ndate: 2026-10-07\nproject: demo\n---\n\n"
                                               "![Stages](fig.svg)\n\n<figure><img src=\"plot.png\" alt=\"Plot\">"
                                               "<figcaption>A plot.</figcaption></figure>\n")
    res = repo.pm("check")
    assert res.returncode == 0, res.stderr
    res = repo.pm("commit", "-m", "Add the figures", "records/docs/2026-10-07-figures.md", "records/docs/fig.svg",
                  "records/docs/plot.png")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["Add the figures"])
    assert repo.git("show", "--name-only", "--format=", "HEAD", cwd=repo.store).split() == [
        "docs/2026-10-07-figures.md", "docs/fig.svg", "docs/plot.png"]
    for name, data in (("fig.svg", svg), ("plot.png", png)):
        shown = subprocess.run(["git", "show", f"HEAD:docs/{name}"], cwd=repo.store, capture_output=True, check=True)
        assert shown.stdout == data


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


SERVE_BEHIND = 10  # seconds a served page may be behind, as internal/site's ServeBehind has it


def asof(page: str) -> float:
    """The time, in epoch seconds, a page served by the pm service states its data is current as of."""
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


@pytest.mark.integration
def test_serve_shows_each_change_within_its_stated_age(repo):
    """The pm service shows a pm write and a Beads change within SERVE_BEHIND s, every page stating data from before a
    change it does not show yet, and a failing render as the error."""
    import urllib.error
    import urllib.request

    repo.stop_service()  # this test's own service holds the work store
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
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
        assert repo.pm("finding", "add", "Served fresh.", "--sprint", "repo-demo.1").returncode == 0
        until_shown(lambda: get("sprints/demo-1.html")[2], lambda p: "Served fresh." in p, time.time())
        assert get("sprints/demo-1.html")[:2] == (200, "no-store")

        assert "1 of 2 tasks done" in get("")[2]
        repo.set_issue("repo-demo.1.1", status="open", closed_at=None)  # reopened, as bd reopens: no close time
        until_shown(lambda: get("")[2], lambda p: "0 of 2 tasks done" in p, time.time())
        assert get("style.css")[0] == 200 and get("nope.html")[0] == 404

        repo.set_issue("repo-demo.1.2", status="closed", closed_at="2026-10-01T14:00:00Z")  # an answered need no decision
        # cites: the render fails
        until_shown(lambda: get("")[2], lambda p: "no decision cites it" in p, time.time())
        code, _, body = get("")
        assert code == 500 and "need repo-demo.1.2 (Ask the owner) is closed but no decision cites it" in body
    finally:
        srv.terminate()
        srv.wait()
        repo.start_service()  # the per-test service again, for what the test reads after


# ---------------------------------------------------------------- pm show: its levels


def test_pm_show_project_prints_one_project_and_refuses_what_it_cannot(repo):
    res = repo.pm("show")
    assert res.returncode == 0, res.stderr
    assert "projects: pm show --project NAME prints" in res.stdout and "  demo  repo-demo  2 open sprints, " in res.stdout
    assert "decision .1.2  Ask the owner  (sprint 1)" in res.stdout and "Sprint 1: First" not in res.stdout
    res = repo.pm("show", "--project", "demo")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("demo  repo-demo  A demo project.\ndecisions await you (1):\n")
    assert "Sprint 1: First  .1  " in res.stdout
    refused(repo, "show", "--project", "old", match=r"no open project 'old'; open ones: demo$")
    refused(repo, "show", "--project", "demo", "--json", match="--project prints text only; drop --json")
    refused(repo, "show", "--project", "demo", "--sprint", "repo-demo.1", match="--sprint prints text only, one sprint")
    refused(repo, "show", "--project", "demo", "--record", "demo", "--section", "Goal",
            match="--record and --section go together, without --sprint, --project or --json")


@pytest.mark.integration
def test_sprints_list_in_natural_id_order_on_the_overview_the_project_page_and_pm_show(repo):
    """Beads ids order by their numbers, not as text: .9, .39, .100, where text order puts .100 before .39."""
    from conftest import sprint
    for n in (100, 39, 9):  # each sprint's record first: the work store imports a sprint only with its record
        repo.write(f"sprints/demo-{n}.md", sprint(f"Number {n}", f"repo-demo.{n}"))
    repo.commit("sprints 9, 39 and 100")
    for n in (100, 39, 9):
        repo.add_issue({"id": f"repo-demo.{n}", "title": f"Sprint {n}: Number {n}", "status": "open",
                        "issue_type": "epic", "parent": "repo-demo", "created_at": "2026-10-01T12:00:00Z",
                        "updated_at": "2026-10-01T12:00:00Z"})
    titles = ("Number 9", "Number 39", "Number 100")
    in_order = lambda text: -1 < text.index(titles[0]) < text.index(titles[1]) < text.index(titles[2])
    pages = repo.pages()
    assert in_order(pages["index.html"])
    project = pages["projects/demo.html"]
    assert in_order(project) and in_order(project[project.index("<table>"):])  # the graph, then the sprint table
    res = repo.pm("show", "--project", "demo")
    assert res.returncode == 0, res.stderr
    assert in_order(res.stdout), res.stdout


# ---------------------------------------------------------------- the site: pm serve


@pytest.fixture
def served(repo):
    """The pm service for the repo's store on a free port; the repo's env carries that PORT from here on."""
    repo.stop_service()  # this test's own service holds the work store
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        port = re.search(r"http://localhost:(\d+)", srv.stdout.readline()).group(1)
        repo.env = dict(repo.env, PORT=port)
        yield f"http://localhost:{port}"
    finally:
        srv.terminate()
        srv.wait()
        repo.start_service()  # the per-test service again, for what the test reads after


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


@pytest.mark.integration
def test_reply_on_a_card_is_stored_on_its_issue_as_one_comment(repo, served):
    """A decision's answer and an action's evidence each land on their own issue, which stays open for the agent;
    each reply is one Beads write, the comment, and no label."""
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "repo-demo.1", text=ACTION).returncode == 0
    day = until_shown(lambda: urllib.request.urlopen(f"{served}/days/2026-10-01.html").read().decode(),
                      lambda p: p.count('<form class="reply" method="post" action="/reply"') == 2, time.time())
    assert '<input type="hidden" name="id" value="repo-demo.1.2">' in day
    token = page_token(served, "days/2026-10-01.html")
    repo.mark()

    for issue_id, text in (("repo-demo.1.2", "-Pick small.\r\nBecause tables can wait."), ("repo-demo.1.3", "Merged as abc123.")):
        code, where, _ = post_reply(served, {"token": token, "id": issue_id, "text": text},
                                    {"Referer": f"{served}/days/2026-10-01.html"})
        assert (code, where) == (303, f"/days/2026-10-01.html#need-{issue_id}")
    deadline = time.monotonic() + 10  # the comments are written in the background
    while len(repo.changes()) < 2 and time.monotonic() < deadline:
        time.sleep(0.05)
    items = repo.items()
    for issue_id, text in (("repo-demo.1.2", "-Pick small.\nBecause tables can wait."), ("repo-demo.1.3", "Merged as abc123.")):
        assert [(c["kind"], c["author"], reply_body(c["text"])) for c in items[issue_id]["comments"]] == [
            ("reply", "owner", text)]
        assert re.fullmatch(r"(?s).*\n\n<!-- pm-reply [0-9a-f-]{36} -->", items[issue_id]["comments"][0]["text"])
        assert (items[issue_id]["type"], items[issue_id]["need"]["kind"], items[issue_id]["labels"]) == (
            "need", "action" if issue_id == "repo-demo.1.3" else "decision", [])
        assert items[issue_id]["status"] == "open"
    assert {i: list(c) for i, c in repo.changes().items()} == {"repo-demo.1.2": ["comments"], "repo-demo.1.3": ["comments"]}
    until_shown(lambda: urllib.request.urlopen(f"{served}/").read().decode(),
                lambda p: p.count('<span class="replied">not delivered') == 2 and "Merged as abc123." in p, None)

    assert repo.pm("check").returncode == 0  # the static site keeps the slot and shows no form, but the replies
    static = repo.page("index.html")
    assert "<!--pm-reply repo-demo.1.2 decision-->" in static and "<form" not in static
    assert "<p>Merged as abc123.</p>" in static and "<!-- pm-reply" not in static


@contextlib.contextmanager
def serving(repo, **env):
    """The pm service for the repo's store on a free port with `env` added; its URL, then its stderr once it stopped."""
    log = repo.root.parent / "serve.log"
    with log.open("w") as err:
        repo.stop_service()  # this test's own service holds the work store
        srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
                               env=dict(repo.env, PORT="0", **env), stdout=subprocess.PIPE, stderr=err, text=True)
    try:
        yield re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0), log
    finally:
        srv.terminate()
        srv.wait()
        repo.start_service()  # the per-test service again, for what the test reads after


@pytest.mark.integration
@pytest.mark.parametrize("token, form, headers, code, said", [
    ("forged", {"id": "repo-demo.1.2", "text": "Small."}, {}, 403, "no valid token"),
    ("page", {"id": "repo-demo.1.2", "text": "Small."}, {"Host": "evil.example:80"}, 403, "is neither this machine"),
])
def test_reply_is_refused_without_the_token_or_from_another_host(repo, served, token, form, headers, code, said):
    form = dict(form, token=page_token(served) if token == "page" else token)
    res = post_reply(served, form, headers)
    assert res[0] == code and said in res[2]
    assert repo.unchanged()


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


@pytest.mark.integration
def test_every_reply_is_pushed_into_the_session_that_asked(repo, served):
    """The three replies lost on 2026-10-06, each pushed by the pm service: several requests raised in one command, one
    raised with its output piped through grep, and a second reply to a request already delivered."""
    with session_inbox() as (inbox, lines):
        env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET=inbox)
        raise_ = shlex.join([*PM, "decision", "need", *NEED, "--parent", "repo-demo.1"])
        script = (f"{raise_} --title A >/dev/null && {raise_} --title B >/dev/null && "
                  f"{raise_} --title C | grep -o 'raised decision need [^ ;]*'")
        res = subprocess.run(["bash", "-c", script], cwd=repo.root, env=env, capture_output=True, text=True)
        assert (res.returncode, res.stdout) == (0, "raised decision need repo-demo.1.5\n"), res.stderr
        until_shown(lambda: load(f"{served}/"), lambda p: 'value="repo-demo.1.5"' in p, time.time())
        token = page_token(served)

        def reply(issue_id, text):
            assert post_reply(served, {"token": token, "id": issue_id, "text": text})[0] == 303

        reply("repo-demo.1.3", "First answer.")
        wait_for(lambda: len(lines) == 1, "the first reply pushed")
        assert inbox_text(lines[0]).startswith("pm: owner reply to decision repo-demo.1.3 (A), relayed from the site:\n")
        assert "First answer." in inbox_text(lines[0]) and "pm decision add --need repo-demo.1.3" in inbox_text(lines[0])
        wait_for(lambda: repo.items()["repo-demo.1.3"]["need"]["delivered"] == 1, "the reply marked delivered")
        for issue_id, text in (("repo-demo.1.4", "Answer to B."), ("repo-demo.1.5", "Answer to C."),
                               ("repo-demo.1.3", "On second thought, no.")):
            reply(issue_id, text)
        wait_for(lambda: len(lines) == 4, "every later reply pushed")
        texts = [inbox_text(line) for line in lines[1:]]
        assert [next(i for i in ("repo-demo.1.4", "repo-demo.1.5", "repo-demo.1.3") if f" {i} " in x) for x in texts] == \
            ["repo-demo.1.4", "repo-demo.1.5", "repo-demo.1.3"]
        assert "Answer to B." in texts[0] and "Answer to C." in texts[1]
        assert "On second thought, no." in texts[2] and "First answer." not in texts[2], "no reply delivered twice"
        wait_for(lambda: repo.items()["repo-demo.1.3"]["need"]["delivered"] == 2, "the second reply marked")
        page = until_shown(lambda: load(f"{served}/"), lambda p: "On second thought, no." in p
                           and "Reply saved" not in p, None)
        assert "delivered to the agent&#x27;s session" in page or "delivered to the agent's session" in page
    assert "pm reply read" not in repo.pm("show").stdout


@pytest.mark.integration
def test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read(repo, served):
    """No socket at the stored inbox: the session ended. The reply stays undelivered, its card says so, pm show
    flags it for the next session, and pm reply read prints it once and marks it delivered."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET="/nonexistent/pm/s")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "repo-demo.1", *NEED).returncode == 0
    until_shown(lambda: load(f"{served}/"), lambda p: 'value="repo-demo.1.3"' in p, time.time())
    assert post_reply(served, {"token": page_token(served), "id": "repo-demo.1.3", "text": "Small, until tables."})[0] == 303
    page = until_shown(lambda: load(f"{served}/"), lambda p: "Small, until tables." in p and "Saving" not in p, None)
    assert "the session that asked is not running" in page
    assert repo.items()["repo-demo.1.3"]["need"]["delivered"] == 0
    assert "decision .1.3  Parser?  (sprint 1)  [undelivered reply: pm reply read repo-demo.1.3]" in repo.pm("show").stdout
    assert "Parser?  (sprint 1)  -> pm show repo-demo.1.3  [undelivered reply: pm reply read repo-demo.1.3]" \
        in repo.pm("show", "--project", "demo").stdout
    res = repo.pm("reply", "read")  # no ids: this session's open requests
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("pm: owner reply to decision repo-demo.1.3 (Parser?), relayed from the site:\n  [")
    assert "Small, until tables." in res.stdout
    assert repo.items()["repo-demo.1.3"]["need"]["delivered"] == 1
    assert "pm reply read" not in repo.pm("show").stdout
    assert repo.pm("reply", "read", "repo-demo.1.3").stdout == "nothing undelivered on repo-demo.1.3\n"
    repo.mark()  # refused() checks that nothing at all changed in the work store
    refused(repo, "reply", "read", "repo-demo.1.1", match=r"repo-demo.1.1 is not a request to the owner")
    repo.env.pop("CLAUDE_CODE_SESSION_ID")
    refused(repo, "reply", "read", match=r"name the requests to read")


REVIEWED_PR = "https://github.com/o/r/pull/7"
REVIEW = {"pr": REVIEWED_PR, "sprints": ["repo-demo.1"], "focus": "f", "designs": []}


def test_reply_read_prints_each_undelivered_reply_and_merge_once(repo):
    """Without the pm service: a site reply and a merge the service saw wait on this session's requests. A close
    refuses while a reply waits; pm reply read prints each with what to do next, marks it delivered, and prints it
    no more."""
    stamps = {"created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"}
    reply = {"id": "6f1c0e2a-1b2c-4d3e-8f40-5a6b7c8d9e0f", "issue_id": "repo-demo.1.3", "author": "owner (site reply)",
             "text": "Small, until tables.\n\n<!-- pm-reply 0b6c7c1e-2d3f-4a5b-9c6d-7e8f9a0b1c2d -->",
             "created_at": "2026-10-03T12:00:00Z"}
    repo.add_issue({"id": "repo-demo.1.3", "title": "Parser?", "status": "open", "issue_type": "task",
                    "parent": "repo-demo.1", "labels": ["human"], "metadata": {"session": "sess-1"},
                    "comments": [reply], **stamps})
    repo.add_issue({"id": "repo-demo.1.4", "title": "Review PR #7", "status": "open", "issue_type": "task",
                    "parent": "repo-demo.1", "labels": ["human", "action"], "external_ref": REVIEWED_PR,
                    "metadata": {"review": REVIEW, "session": "sess-1", "merged": SHA}, **stamps})
    repo.mark()
    refused(repo, *ADD, "--need", "repo-demo.1.3", "--decision", DECISION, "--reason", REASON,
            match=r"repo-demo.1.3 holds a site reply not delivered yet; read it with pm reply read repo-demo.1.3")
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    res = repo.pm("reply", "read")  # no ids: this session's open requests
    assert res.returncode == 0, res.stderr
    assert res.stdout == (
        "pm: owner reply to decision repo-demo.1.3 (Parser?), relayed from the site:\n"
        "  [2026-10-03T12:00:00Z] Small, until tables.\n"
        "next: record the answer: pm decision add --need repo-demo.1.3 --level … --decision '<the answer>' --reason "
        "'<why>' if it sets a rule, else pm decision close repo-demo.1.3 --reason \"<why it sets no rule>\" "
        "--text-file - <<'EOF' (the answer, then EOF)\n"
        f"pm: PR #7 of review repo-demo.1.4 (Review PR #7) merged to main as {SHA}\n"
        f"next: pm action done repo-demo.1.4 --reason \"merged as {SHA}\", then update the main checkout: "
        f"{pull_main(repo)}\n")
    items = repo.items()
    assert {i: list(c) for i, c in repo.changes().items()} == {"repo-demo.1.3": ["need"], "repo-demo.1.4": ["need"]}
    assert items["repo-demo.1.3"]["need"]["delivered"] == 1
    assert items["repo-demo.1.4"]["need"]["review"]["merge_reported"] == SHA
    res = repo.pm("reply", "read", "repo-demo.1.3", "repo-demo.1.4")
    assert res.stdout == "nothing undelivered on repo-demo.1.3, repo-demo.1.4\n", res.stderr
    repo.mark()
    refused(repo, "reply", "read", "repo-demo.1.1", match=r"repo-demo.1.1 is not a request to the owner")
    repo.env.pop("CLAUDE_CODE_SESSION_ID")
    refused(repo, "reply", "read", match=r"name the requests to read")
    res = repo.pm(*ADD, "--need", "repo-demo.1.3", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["status"] == "closed"
    # the answer the close recorded is no reply of the owner's to deliver
    assert repo.pm("reply", "read", "repo-demo.1.3").stdout == "nothing undelivered on repo-demo.1.3\n"


def review_with_origin(repo) -> str:
    """An open review of REVIEWED_PR under repo-demo.1, and an origin remote whose main is one commit ahead:
    the sha returned."""
    repo.add_issue({"id": "repo-demo.1.3", "title": "Review PR #7", "status": "open", "issue_type": "task",
                    "parent": "repo-demo.1", "labels": ["human", "action"], "created_at": "2026-10-01T12:00:00Z",
                    "updated_at": "2026-10-01T12:00:00Z", "metadata": {"review": REVIEW}})
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


@pytest.mark.integration
def test_a_reviewed_prs_merge_is_pushed_into_the_session_once(repo):
    sha = review_with_origin(repo)
    repo.set_pr(REVIEWED_PR, "MERGED", sha)
    with session_inbox() as (inbox, lines):
        repo.set_issue("repo-demo.1.3", metadata={"review": REVIEW, "session": "sess-1", "inbox": inbox,
                                             "inbox_host": socket.gethostname()})
        with serving(repo):
            wait_for(lambda: lines, "the merge pushed")
            wait_for(lambda: repo.items()["repo-demo.1.3"]["need"]["review"]["merge_reported"] == sha, "the merge marked")
        assert inbox_text(lines[0]) == (f"pm: PR #7 of review repo-demo.1.3 (Review PR #7) merged to main as {sha}\n"
                                    f"next: pm action done repo-demo.1.3 --reason \"merged as {sha}\", then update the "
                                    f"main checkout: {pull_main(repo)}")
        assert repo.items()["repo-demo.1.3"]["need"]["review"]["merged"] == sha
        assert repo.items()["repo-demo.1.3"]["status"] == "open", "the agent closes the review, not the pm service"
        with serving(repo) as (url, _):  # a restart: the merge is stored, so it is neither looked up nor pushed again
            until_shown(lambda: load(f"{url}/"), lambda p: "Review PR #7" in p, None)
            deadline = time.time() + 2
            while time.time() < deadline:
                assert len(lines) == 1
                time.sleep(0.1)
    assert repo.pm("reply", "read", "repo-demo.1.3").stdout == "nothing undelivered on repo-demo.1.3\n"


# ---------------------------------------------------------------- pm task claim

def transcript(repo, sid: str, age_s: float, where: str = "-repo") -> None:
    """Session sid's Claude Code transcript under the test's config dir, last written age_s seconds ago."""
    p = Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects" / where / f"{sid}.jsonl"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("{}\n")
    t = time.time() - age_s
    os.utime(p, (t, t))


def claim(repo, task: str, sid: str | None, *extra: str, cwd: Path | None = None):
    env = dict(repo.env, **({"CLAUDE_CODE_SESSION_ID": sid} if sid else {}))
    return subprocess.run([*PM, "task", "claim", task, *extra], cwd=cwd or repo.root, env=env,
                          capture_output=True, text=True)


def test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_worktree(repo):
    repo.set_issue("repo-demo.1.2", labels=[])
    repo.mark()
    for where in (repo.root, repo.store):  # from inside the store, the code worktree is the main checkout
        res = claim(repo, "repo-demo.1.2", "sess-a", cwd=where)
        assert res.returncode == 1, res.stderr
        main = repo.root.resolve()
        assert f"not claiming repo-demo.1.2 here: {main} is the main checkout; agents change code only in a worktree of " \
               f"their own" in res.stderr
        assert f"git -C {main} fetch origin main && git -C {main} worktree add -b <branch> .claude/worktrees/<name> " \
               f"origin/main" in res.stderr
    assert repo.unchanged()
    assert claim(repo, "repo-demo.1.2", "sess-a", cwd=repo.worktree("feature")).returncode == 0


def test_task_claim_refuses_a_task_another_live_session_holds(repo):
    repo.set_issue("repo-demo.1.2", labels=[])
    wt = repo.worktree("feature")
    assert claim(repo, "repo-demo.1.2", "sess-a", cwd=wt).returncode == 0
    i = repo.items()["repo-demo.1.2"]
    assert i["status"] == "open" and i["holder"]["session"] == "sess-a"
    transcript(repo, "sess-a", 60, where="-other-worktree")
    repo.mark()
    res = claim(repo, "repo-demo.1.2", "sess-b", cwd=wt)
    assert res.returncode == 1
    assert "held by live session sess-a" in res.stderr and "last 30 minutes" in res.stderr
    assert repo.unchanged()
    # the same session (a subagent shares it) may claim again
    assert claim(repo, "repo-demo.1.2", "sess-a", cwd=wt).returncode == 0


# ---------------------------------------------------------------- pm push: what the service runs

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


@pytest.mark.integration
def test_push_pushes_beads_and_new_records_commits(pushed):
    repo = pushed
    first = repo.pm("push").stdout
    assert first.count(" ok: ") == 3 and "summary ok: summarized" in first, first
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("a day")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    # the work store syncs to the repo's remote, under pm's own ref
    assert repo.git("rev-parse", "refs/pm/work", cwd=repo.root.parent / "origin.git").strip()
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    state = push_state(repo)
    assert state["work"]["ok"] and state["records"]["ok"] and state["records"]["message"] == "pushed 2 commit(s)", "the day file and its new summary"
    assert state["records"]["last_ok"] == state["records"]["at"]
    assert len((repo.root / ".pm/run/push.log").read_text().splitlines()) == 6
    assert state["summary"]["message"].startswith("summarized"), "the new day file changed the activity"


@pytest.mark.integration
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


# ---------------------------------------------------------------- day pages and pm day summarize

def now_z() -> str:
    from datetime import datetime, timezone
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def claude_calls(repo) -> list[dict]:
    log = Path(repo.env["FAKE_CLAUDE_LOG"])
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def summary(repo) -> dict:
    return json.loads((repo.records / f"days/{TODAY}.summary.json").read_text())


@pytest.mark.integration
def test_day_summarize_skips_unchanged_activity_and_regenerates_on_change(repo):
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert committed(repo, [f"pm: summarized {TODAY} in records/days/{TODAY}.summary.json"])
    first = summary(repo)
    assert first["text"] == "Summary 1." and first["date"] == TODAY
    call = claude_calls(repo)[0]
    assert call["args"][:4] == ["-p", "--model", "claude-haiku-5-5", "--tools"]
    assert "records commit: records [" in call["stdin"], "today's records commits are the activity"
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and "unchanged" in res.stdout and len(claude_calls(repo)) == 1
    repo.set_issue("repo-demo.1.2", status="in_progress", started_at=now_z(),
                   metadata={"claimed_by": "sess-x", "claimed_at": now_z()})
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert summary(repo)["text"] == "Summary 2." and summary(repo)["digest"] != first["digest"]
    assert "started: Ask the owner" in claude_calls(repo)[1]["stdin"]
    assert repo.pm("check").returncode == 0
    page = repo.page(f"days/{TODAY}.html")
    assert re.search(r"<span>generated at \d\d:\d\d</span>.*Summary 2\.", page, re.S), page
    assert "Summary 2." in repo.page("index.html")
    assert json.loads(repo.pm("show", "--json").stdout)["today"]["summary"] == "Summary 2."


# ---------------------------------------------------------------- the work store's one access path

# Every pm command that reads or writes work items, as an agent runs it: each reaches the work store only through
# the pm service (the pm-go page, Store access), so with the service stopped each fails hard, naming the fix, and writes
# nothing. pm where reports it in its work line instead, and the hooks fail open; pm record link reads no item.
STORE_COMMANDS = [
    ["show"], ["show", "--project", "demo"], ["show", "--sprint", "repo-demo.1"], ["show", "repo-demo.1"],
    ["export"], ["check"], ["commit", "-m", "x"], ["day", "summarize"],
    ["task", "add", "--sprint", "repo-demo.1", "--title", "T"], ["task", "add", "--parent", "repo-demo.1.2",
                                                                    "--title", "T"],
    ["task", "close", "repo-demo.1.2", "--reason", "x"], ["task", "claim", "repo-demo.1.2"],
    ["task", "move", "repo-demo.1.2", "--to", "repo-demo.2", "--text=a\nb"], ["task", "ready"],
    ["task", "edit", "repo-demo.1.2", "--title", "T"], ["task", "release", "repo-demo.1.2"],
    ["finding", "add", "--sprint", "repo-demo.1", "x"], ["feedback", "add", "--project", "demo", "--text=x"],
    ["doc", "new", "x", "--title", "X", "--project", "demo", "--text=x"],
    ["design", "new", "x", "--title", "X", "--project", "demo"],
    ["postmortem", "new", "x", "--title", "X", "--project", "demo"],
    ["project", "open", "p", "--title", "P", "--text=x"], ["project", "close", "demo"],
    ["sprint", "open", "demo", "--title", "S",
     "--text=## Goal\n\nx\n\n## Scope\n\n**In:** a.\n\n**Out:** b.\n\n## Done when\n\n- x."],
    ["sprint", "close", "repo-demo.1"],
    ["decision", "add", "--level", "project", "--project", "demo", "--decision", "x", "--reason", "y"],
    ["decision", "need", "--title", "Q?", "--parent", "repo-demo.1", "--question", "q", "--fact", "f",
     "--option", "a", "a", "--cost", "a", "c", "--option", "b", "b", "--cost", "b", "c", "--default", "a", "why"],
    ["decision", "close", "repo-demo.1.2", "--reason", "x", "--text=x"],
    ["action", "need", "--title", "Do", "--parent", "repo-demo.1", "--text=x"],
    ["action", "done", "repo-demo.1.2", "--reason", "x"], ["reply", "read", "repo-demo.1.2"],
    ["dep", "add", "repo-demo.1.2", "--on", "repo-demo.2"], ["dep", "rm", "repo-demo.1.2", "--on", "repo-demo.1.1"],
    ["comment", "add", "repo-demo.1.2", "--text=x"], ["need", "dismiss", "repo-demo.1.2", "--reason", "x"],
    ["reply", "add", "repo-demo.1.2", "--text=x"], ["sync"],
]


def test_every_store_command_fails_hard_with_the_service_stopped(repo):
    repo.stop_service()
    want = (f"error: the pm service does not answer on {repo.root / '.pm/run/work.sock'}; pm reaches the work store "
            "only through it: run pm service restart\n")
    env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-a")
    wt = repo.worktree("feature")  # pm task claim refuses in the main checkout before it reaches the store
    before = repo.snapshot()
    for argv in STORE_COMMANDS:
        res = subprocess.run([*PM, *argv], cwd=wt if argv[:2] == ["task", "claim"] else repo.root, env=env,
                             capture_output=True, text=True, stdin=subprocess.DEVNULL)
        assert (res.returncode, res.stdout, res.stderr) == (1, "", want), argv
    assert repo.snapshot() == before, "a command wrote with the service stopped"
    repo.start_service()
    assert repo.unchanged()
