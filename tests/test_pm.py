"""The pm flows agents and the owner depend on, end to end against a temp repo: setup, the sprint loop,
needs and replies, tasks, the shared records store, the pm service and its push. Every refusal changes nothing, and
every happy path writes what it says; the refusal texts asserted here are the ones prime.md and the --help texts name.

Fixtures beside conftest's: `served` (a `pm service run` on a free port), `origin` (a cut-over origin, for pm init in a
second clone) and `pushed` (a bare origin plus a second clone, for pm push)."""

from __future__ import annotations

import contextlib
import datetime
import functools
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

from conftest import CHECKOUT, MACHINERY, PM, VERSION, fake_env, nouns, project, sprint, wait_for, write_config


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

    def heads() -> list[str]:
        return [repo.git("rev-parse", "HEAD", cwd=d) for d in (repo.root, repo.store)]

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


def test_pm_help_hides_the_machinery_that_help_all_lists():
    """pm --help leaves out what the runtimes' hooks, the scheduler and the pm service run (MACHINERY) and says that
    pm --help --all lists it; each hidden command still runs and keeps its own --help, and pm service --help hides
    service run the same way."""
    shown, every = nouns(), nouns("--all")
    assert set(every) - set(shown) == MACHINERY and set(shown) < set(every), (shown, every)
    top = subprocess.run([*PM, "--help"], cwd="/", capture_output=True, text=True, check=False).stdout
    assert "pm --help --all also lists the machinery commands" in top
    for cmd in (["hook"], ["prime"], ["sync"], ["day", "summarize"], ["service", "run"]):
        res = subprocess.run([*PM, *cmd, "--help"], cwd="/", capture_output=True, text=True, check=False)
        assert res.returncode == 0 and res.stdout.startswith(f"usage: pm {' '.join(cmd)} "), res.stdout + res.stderr
    service = subprocess.run([*PM, "service", "--help"], cwd="/", capture_output=True, text=True, check=False).stdout
    assert "    run " not in service and "pm service --help --all also lists" in service
    assert "    run " in subprocess.run([*PM, "service", "--help", "--all"], cwd="/", capture_output=True, text=True, check=False).stdout


def test_new_records_feedback_projects_and_moves_write_what_they_say(repo):
    """Each write that makes or extends a record: its file, its one store commit, and its work-store change."""
    # folded: test_finding_add_refuses_a_text_that_starts_with_two_dashes_and_two_texts
    # A misspelt option is not a finding: three agents stored a literal `--text=…` before pm finding add took
    # --text.
    refused(repo, "finding", "add", "--sprint", "repo-demo.1", "--txt=It parses 9 of 10.",
            match=r'the finding text starts with --, an option pm finding add does not have: "--txt=It parses 9 of '
                  r'10\."; give the text as one quoted argument')
    refused(repo, "finding", "add", "--sprint", "repo-demo.1", "It parses.", "--text=It parses too.",
            match=r"give the finding as one argument or with --text or --text-file - <<'EOF', not both")
    # folded: test_task_move_takes_a_task_from_directly_under_a_project_into_its_sprint
    # A task filed directly under a project joins one of the project's sprints; the scope it adds is a decision in
    # the sprint it joins.
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
    repo.mark()

    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    writes = [
        (("doc", "new", "probe", "--title", "Probe: one", "--project", "demo"), "Body.",
         f"pm: created records/docs/{TODAY}-probe.md"),
        (("doc", "new", "probe-sprint", "--title", "Probe: two", "--sprint", "repo-demo.1"), "Body.",
         f"pm: created records/docs/{TODAY}-probe-sprint.md"),
        (("design", "new", "parser", "--title", "The parser", "--project", "demo"), "",
         "pm: created records/design/parser.md; fill in its sections by hand in records/, then pm commit -m \"…\" "
         "records/design/parser.md"),
        (("postmortem", "new", "outage", "--title", "Outage", "--sprint", "repo-demo.1"), "",
         f"pm: created records/postmortems/{TODAY}-outage.md; fill in its sections by hand in records/, then pm "
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
    assert "bead: repo-demo.1\n" in (repo.records / f"docs/{TODAY}-probe-sprint.md").read_text()
    refused(repo, "doc", "new", "probe-task", "--title", "T", "--task", "repo-demo.1", text="Body.",
            match=r"--task repo-demo\.1 is a sprint, not a task")
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
    # folded: test_pm_show_project_prints_one_project_and_refuses_what_it_cannot
    res = repo.pm("show")
    assert res.returncode == 0, res.stderr
    assert "projects: pm show --project NAME prints" in res.stdout and "  demo  repo-demo  2 open sprints, " in res.stdout
    assert "decision .1.2  Ask the owner  (sprint 1)" in res.stdout and "Sprint 1: First" not in res.stdout
    res = repo.pm("show", "--project", "demo")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("demo  repo-demo  A demo project.\ndecisions await you (1):\n")
    assert "Sprint 1: First  .1  " in res.stdout
    refused(repo, "show", "--project", "old", match=r'no open project "old"; open ones: demo$')
    refused(repo, "show", "--project", "demo", "--json", match="--project prints text only; drop --json")
    refused(repo, "show", "--project", "demo", "--sprint", "repo-demo.1", match="--sprint prints text only, one sprint")
    refused(repo, "show", "--project", "demo", "--record", "demo", "--section", "Goal",
            match="--section needs --record, and --record goes without --sprint, --project or --json")
    refused(repo, "show", "--section", "Goal",
            match="--section needs --record, and --record goes without --sprint, --project or --json")
    # --record alone prints the outline: the header and each section with its line count and the command for it;
    # a sprint's id and its name (<project>-<n>) both name its record.
    for target in ("repo-demo.1", "demo-1"):
        res = repo.pm("show", "--record", target)
        assert res.returncode == 0, res.stderr
        assert res.stdout.startswith("records/sprints/demo-1.md\n  bead: repo-demo.1\n"), res.stdout
        assert "  ## Goal  (" in res.stdout and f"pm show --record {target} --section Goal\n" in res.stdout
        assert f"pm show --record {target} --section 'Done when'\n" in res.stdout

    # folded: test_feedback_add_refuses_to_overwrite_another_record_at_the_feedback_doc_s_path
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    (repo.records / "docs").mkdir()
    repo.write("docs/pm-feedback.md", "---\ntype: day\ndate: 2026-10-01\n---\n\nKeep this.\n")
    repo.commit("a record at the feedback doc's path")
    refused(repo, "feedback", "add", text="x",
            match=r"records/docs/pm-feedback.md exists but is no pm feedback doc \(type: doc\); move it")
    shutil.rmtree(repo.records / "docs")  # the doc's path free again for the feedback below
    repo.commit("no record at the feedback doc's path")

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


@pytest.mark.integration
def test_sprint_closes_after_its_pr_merges(repo, tmp_path):
    """The whole loop: a written report, a review under the sprint, a refusal while the review is open, the review
    closed as merged once the PR is on main, then a close that stamps the merge and names its records commit."""
    repo.set_pr(PR, "OPEN")
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

    # folded: test_sprint_close_merged_stamps_an_agent_s_merge_on_main
    # A PR an agent merged has no review: pm sprint close --merged SHA --pr URL checks the commit is on the remote's
    # main, fetched first, and stamps the line a review's close stamps; a commit not on main, a bad sha, --pr alone and a
    # sprint that holds a review are refused with nothing written.
    # (sprint 1 is closed above and repo-demo.1.2 dismissed there, so this closes a new sprint 3)
    repo.git("init", "-q", "--bare", str(tmp_path / "origin.git"))
    repo.git("remote", "add", "origin", str(tmp_path / "origin.git"))
    repo.git("push", "-q", "origin", "main")
    on_main = repo.git("rev-parse", "HEAD").strip()
    wt = repo.worktree("unmerged")
    (wt / "x.txt").write_text("x\n")
    repo.git("add", "x.txt", cwd=wt)
    repo.git("commit", "-qm", "not on main", cwd=wt)
    off_main = repo.git("rev-parse", "HEAD", cwd=wt).strip()
    assert repo.pm("sprint", "open", "demo", "--title", "Third", text=FRAME).returncode == 0
    report(repo, "Done: shipped the thing.", name="demo-3")
    assert repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.2", "--focus", "F").returncode == 0
    assert repo.pm("action", "done", "repo-demo.2.1", "--reason", f"merged as {SHA}").returncode == 0
    repo.mark()
    refused(repo, "sprint", "close", "repo-demo.3", "--merged", off_main, "--pr", PR,
            match=rf"--merged {off_main} is not on origin/main; give the merge commit once the PR is on main")
    refused(repo, "sprint", "close", "repo-demo.3", "--merged", "HEAD", match=r'--merged "HEAD" is not a commit sha')
    refused(repo, "sprint", "close", "repo-demo.3", "--pr", PR, match=r"--pr names the PR whose merge --merged gives")
    refused(repo, "sprint", "close", "repo-demo.2", "--merged", on_main,
            match=r"sprint repo-demo.2 holds PR review repo-demo.2.1 \(https://github.com/o/r/pull/12\), whose close "
                  r"stamps the merge")
    res = repo.pm("sprint", "close", "repo-demo.3", "--merged", on_main[:10], "--pr", PR)
    assert res.returncode == 0, res.stderr
    assert committed(repo, [f"[SPRINT] demo sprint 3: closed, merged as {on_main[:7]}"])
    assert repo.changes() == {"repo-demo.3": {"status": "closed", "resolution": "done", "close_reason":
                                              f"Done: shipped the thing. (records commit {store_head(repo)})"}}
    text = (repo.records / "sprints/demo-3.md").read_text()
    assert f"Done: shipped the thing.\n\nMerged as {on_main[:7]} (PR #12).\n" in text
    assert repo.pm("check").returncode == 0


def test_a_review_of_a_pr_that_does_not_target_main_is_refused_naming_the_fix(repo):
    """A stacked PR merges into another PR's branch, so its review could never close as merged on main: pm action need
    --pr refuses a GitHub PR whose base is not the main branch, or whose base gh cannot tell, and writes nothing."""
    report(repo, "Done: shipped the thing.")
    review = ("action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F")
    refused(repo, *review, match=r"--pr https://github.com/o/r/pull/12: pm reads its base branch from GitHub, and "
                                 r"cannot: gh pr view .* failed: GraphQL: Could not resolve to a PullRequest")
    repo.set_pr(PR, "OPEN", base="codex-parity")
    refused(repo, *review, match=r"--pr https://github.com/o/r/pull/12 merges into \"codex-parity\", not main, so its "
                                 r"merge does not deliver the sprint; raise the review on the PR that reaches main, or "
                                 r"retarget this one first \(gh pr edit https://github.com/o/r/pull/12 --base main\)")
    repo.set_pr(PR, "OPEN")
    res = repo.pm(*review)
    assert res.returncode == 0, res.stderr


@pytest.mark.integration
def test_action_done_restamps_a_review_closed_before_its_pr_reached_main(repo, tmp_path):
    """A review closed while its PR had merged only into a stacked base holds its sprint's close; once the work is on
    main, pm action done <review> --reason "merged as <sha>" restamps it, keeping the old reason, and the sprint closes.
    A reason without it, a sha not on main, a review closed as merged and any other closed action are refused."""
    repo.git("init", "-q", "--bare", str(tmp_path / "origin.git"))
    repo.git("remote", "add", "origin", str(tmp_path / "origin.git"))
    repo.git("push", "-q", "origin", "main")
    on_main = repo.git("rev-parse", "HEAD").strip()
    wt = repo.worktree("stacked")
    (wt / "x.txt").write_text("x\n")
    repo.git("add", "x.txt", cwd=wt)
    repo.git("commit", "-qm", "not on main", cwd=wt)
    off_main = repo.git("rev-parse", "HEAD", cwd=wt).strip()
    repo.set_issue("repo-demo.1.2", status="closed", close_reason="Dismissed", closed_at="2026-10-02T12:00:00Z")
    report(repo, "Done: shipped the thing.")
    repo.set_pr(PR, "OPEN")
    assert repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F").returncode == 0
    assert repo.pm("action", "need", "--title", "Restart", "--parent", "repo-demo.1", text=ACTION).returncode == 0
    review, action, first = "repo-demo.1.3", "repo-demo.1.4", "PR merged into stacked as d74c717, not yet on main"
    assert repo.pm("action", "done", review, "--reason", first).returncode == 0
    assert repo.pm("action", "done", action, "--reason", "restarted").returncode == 0
    repo.mark()
    refused(repo, "sprint", "close", "repo-demo.1", match=rf"PR review {review} \({PR}\) is closed as \"{first}\", not "
                                                         rf"'merged as <sha>'; .* restamp it: pm action done {review} "
                                                         r"--reason \"merged as <sha>\"")
    refused(repo, "action", "done", review, "--reason", "it landed",
            match=rf"review {review} is closed as \"{first}\", without 'merged as <sha>'; once its work is on main, "
                  rf"restamp it with pm action done {review} --reason \"merged as <sha>\"")
    refused(repo, "action", "done", review, "--reason", f"merged as {off_main}",
            match=rf"merged as {off_main} is not on origin/main; give the merge commit once the PR is on main")
    refused(repo, "action", "done", action, "--reason", f"merged as {on_main[:7]}", match=rf"action {action} is "
                                                                                          r"already closed")
    res = repo.pm("action", "done", review, "--reason", f"merged as {on_main[:7]}")
    assert res.returncode == 0, res.stderr
    restamped = f"merged as {on_main[:7]}; first closed as: {first}"
    assert repo.changes() == {review: {"close_reason": restamped}}
    repo.mark()
    refused(repo, "action", "done", review, "--reason", f"merged as {on_main[:7]}",
            match=rf"action {review} is already closed")
    res = repo.pm("sprint", "close", "repo-demo.1")
    assert res.returncode == 0, res.stderr
    assert f"Merged as {on_main[:7]} (PR #12)." in (repo.records / "sprints/demo-1.md").read_text()


# ---------------------------------------------------------------- design pages: covers and the gate


def design_page(covers: str, title: str = "The parser") -> str:
    """A design page whose header carries `covers`, every section with one line."""
    sections = ("Problem", "Goals and non-goals", "Constraints and key facts", "Design", "Alternatives considered",
                "Open questions")
    return (f"---\ntype: design\ntitle: {title}\nproject: demo\ncovers: {covers}\n---\n\n"
            + "".join(f"## {name}\n\nNone yet.\n\n" for name in sections))


def commit_on(repo, day: str, msg: str) -> None:
    """Commit every change in the records store, dated `day` (before or after the sprints opened)."""
    env = dict(os.environ, GIT_AUTHOR_DATE=f"{day}T12:00:00Z", GIT_COMMITTER_DATE=f"{day}T12:00:00Z")
    for args in (("add", "-A"), ("commit", "-qm", msg)):
        subprocess.run(["git", *args], cwd=repo.store, env=env, check=True, capture_output=True)


def set_pr_files(repo, url: str, files: list[dict | str]) -> None:
    """What the fake gh's pull request files API lists for a PR: a path, or the API's entry for one."""
    path = Path(repo.env["FAKE_GH_STATE"])
    prs = json.loads(path.read_text())
    slug, n = re.fullmatch(r"https://github.com/(\S+)/pull/(\d+)", url).groups()
    prs[f"repos/{slug}/pulls/{n}/files"] = [{"filename": f} if isinstance(f, str) else f for f in files]
    path.write_text(json.dumps(prs))


PR2 = "https://github.com/o/r/pull/13"


def push_main(repo, tmp_path, *files: str) -> str:
    """Commit `files` on main and push it to a bare origin, made on first use: the remote's main pm sprint close holds
    the design pages' covers to. Returns main's sha."""
    origin = tmp_path / "origin.git"
    if not origin.exists():
        repo.git("init", "-q", "--bare", str(origin))
        repo.git("remote", "add", "origin", str(origin))
    for rel in files:
        (repo.root / rel).parent.mkdir(parents=True, exist_ok=True)
        (repo.root / rel).write_text("x\n")
        repo.git("add", rel)
    if files:
        repo.git("commit", "-qm", "merged: " + ", ".join(files))
    repo.git("push", "-q", "origin", "main")
    return repo.git("rev-parse", "HEAD").strip()


@pytest.mark.integration
def test_a_pr_that_changes_covered_code_waits_until_its_design_page_is_updated_or_waived(repo, tmp_path):
    """A design page covering a file the PR changes holds the PR's review and its sprint's close until the sprint
    edits the page and lists it in its Design pages, or records with pm design unchanged why it stays as it is."""
    push_main(repo, tmp_path, "src/parser/lex.go")
    repo.set_issue("repo-demo.1.2", status="closed", close_reason="Dismissed", closed_at="2026-10-02T12:00:00Z")
    (repo.records / "design").mkdir()
    repo.write("design/parser.md", design_page("[src/parser/**]"))
    commit_on(repo, "2026-09-30", "the parser page, before the sprints opened")
    report(repo, "Done: shipped the thing.")
    set_pr_files(repo, PR, ["src/parser/lex.go", "README.md"])
    repo.set_pr(PR, "OPEN")
    repo.set_pr(PR2, "OPEN")
    gate = (r"https://github.com/o/r/pull/12 changes code that design pages cover, and sprint repo-demo.1 neither "
            r"updated nor waived them: design/parser.md \(covers src/parser/lex.go\)\. For each page, bring it to the "
            r"new state, commit it with pm commit and link it in the sprint record's Design pages section, or record "
            r"why it stays as it is: pm design unchanged <slug> --sprint repo-demo.1 --reason")
    refused(repo, "action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F", match=gate)
    page = repo.records / "design/parser.md"
    page.write_text(page.read_text().replace("## Design\n\nNone yet.", "## Design\n\nThe lexer reads tokens."))
    repo.commit("the parser page at the new state")
    refused(repo, "action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F", match=gate)  # not listed
    sprint1 = repo.records / "sprints/demo-1.md"
    sprint1.write_text(sprint1.read_text().replace("## Design pages\n\n> Where is the detail?\n\nNone yet.",
                                                   "## Design pages\n\n> Where is the detail?\n\n"
                                                   "- [The parser](../design/parser.md)"))
    repo.commit("the sprint lists the page")
    res = repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F")
    assert res.returncode == 0, res.stderr
    review = repo.items()["repo-demo.1.3"]
    assert review["description"].endswith("Design pages covering the changed code:\n- [The parser]"
                                          "(http://localhost:8000/design/parser.html): updated in the sprint"), review
    assert repo.pm("action", "done", "repo-demo.1.3", "--reason", f"merged as {SHA}").returncode == 0
    res = repo.pm("sprint", "close", "repo-demo.1")
    assert res.returncode == 0, res.stderr

    # Sprint 2's PR touched no covered file when its review was raised, then moved the lexer out of the parser too
    # (a rename's old path counts): the close is refused until the waiver, which is a sprint decision.
    set_pr_files(repo, PR2, ["README.md"])
    res = repo.pm("action", "need", "--pr", PR2, "--sprint", "repo-demo.2", "--focus", "F")
    assert res.returncode == 0, res.stderr
    assert "Design pages covering" not in repo.items()["repo-demo.2.1"]["description"]
    assert repo.pm("action", "done", "repo-demo.2.1", "--reason", f"merged as {SHA}").returncode == 0
    set_pr_files(repo, PR2, ["README.md", {"filename": "src/lexer/lex.go", "previous_filename": "src/parser/lex.go"}])
    repo.mark()
    refused(repo, "sprint", "close", "repo-demo.2",
            match=r"https://github.com/o/r/pull/13 changes code that design pages cover, and sprint repo-demo.2 "
                  r"neither updated nor waived them: design/parser.md \(covers src/parser/lex.go\)")
    refused(repo, "design", "unchanged", "lexer", "--sprint", "repo-demo.2", "--reason", "R.",
            match=r'no design page "lexer"; known: parser')
    refused(repo, "design", "unchanged", "parser", "--sprint", "repo-demo.1", "--reason", "R.",
            match=r"sprint repo-demo.1 is closed or not in the work store")
    res = repo.pm("design", "unchanged", "parser", "--sprint", "repo-demo.2", "--reason",
                  "The PR renames a variable in the lexer; what the page says holds.")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: recorded in records/sprints/demo-2.md that design page parser stays as it is"])
    assert (f"::: decision {{source=agent date={TODAY}}}\nThe design page parser stays as it is for this sprint's "
            "code changes.\nThe PR renames a variable in the lexer; what the page says holds.\n:::"
            in (repo.records / "sprints/demo-2.md").read_text())
    refused(repo, "design", "unchanged", "parser", "--sprint", "repo-demo.2", "--reason", "Again.",
            match=r"records/sprints/demo-2.md already records that design page parser stays as it is")
    res = repo.pm("sprint", "close", "repo-demo.2")
    assert res.returncode == 0, res.stderr


def test_the_gate_replayed_on_pr_47_names_the_pages_pm_codex_sprint_1_left_unchanged(repo):
    """pm-codex sprint 1's PR #47 changed reply delivery, session claims and the owner-request hook without editing
    their design pages. Replayed with its real file list and this repo's pages' covers, the review is refused naming
    each of those pages."""
    replay = json.loads((Path(__file__).parent / "pr47_replay.json").read_text())
    (repo.records / "design").mkdir()
    for slug, covers in replay["covers"].items():
        repo.write(f"design/{slug}.md", design_page(json.dumps(covers), title=slug))
    commit_on(repo, "2026-09-30", "the design pages, before the sprint opened")
    report(repo, "Done: shipped the thing.")
    url = "https://github.com/Yeeef/pm/pull/47"
    repo.set_pr(url, "OPEN")
    set_pr_files(repo, url, replay["files"])
    res = refused(repo, "action", "need", "--pr", url, "--sprint", "repo-demo.1", "--focus", "F",
                  match=r"https://github.com/Yeeef/pm/pull/47 changes code that design pages cover")
    named = re.findall(r"design/([a-z0-9-]+)\.md \(covers ", res.stderr)
    assert {"reply-delivery", "owner-request-hook", "session-claims"} <= set(named), named


def test_check_holds_covers_to_main_and_lists_uncovered_sources(repo):
    """A design page publishes at its sprint's close, once its code is on main, so pm check reads each published covers
    pattern against the remote's main: here its remote-tracking ref, set by hand (the Go tests fetch a stale one and
    refuse a pattern main lacks). A malformed pattern fails. With design_sources set, it lists the tracked source files
    no page covers, and says when every one is covered; a source pattern that matches nothing fails."""
    for rel in ("src/parser/lex.go", "src/cli.go", "README.md"):
        (repo.root / rel).parent.mkdir(parents=True, exist_ok=True)
        (repo.root / rel).write_text("x\n")
    (repo.records / "design").mkdir()
    repo.write("design/parser.md", design_page("[src/parser/**]"))
    repo.commit("code, and a page naming it")
    repo.git("update-ref", "refs/remotes/origin/main", "HEAD")
    res = repo.pm("check")
    assert res.returncode == 0 and res.stdout.endswith("pages render\n"), res
    repo.write("design/parser.md", design_page("[src/parser/**, /src/old/**]"))
    res = repo.pm("check")
    assert res.returncode == 1 and ('design/parser: covers pattern "/src/old/**" is not relative to the repo root '
                                    "(no leading /)") in res.stderr, res
    repo.write("design/parser.md", design_page("[src/parser/**]"))
    write_config(repo.root, design_sources=["src/**", "lib/**"])
    repo.commit("the sources, one of them dead")
    repo.git("update-ref", "refs/remotes/origin/main", "HEAD")
    res = repo.pm("check")
    assert res.returncode == 1 and res.stderr == ('error: design_sources pattern "lib/**" matches no tracked file; '
                                                  "fix or remove it\n")
    write_config(repo.root, design_sources=["src/**"])
    repo.commit("the sources")
    res = repo.pm("check")
    assert res.returncode == 0, res.stderr
    assert res.stdout.endswith("pages render; 1 tracked source files (design_sources in .pm/config.toml) no design "
                               "page covers; add each to the covers: of the page about its area:\n  src/cli.go\n")
    repo.write("design/parser.md", design_page("[src/parser/**, src/cli.go]"))
    repo.commit("every source covered")
    res = repo.pm("check")
    assert res.returncode == 0 and res.stdout.endswith("; a design page covers every tracked source file\n"), res


# ---------------------------------------------------------------- needs: answer with a decision or close

DECISION = "Use the small parser."
REASON = "It is enough for the record set and adds no dependency."
ADD = ("decision", "add", "--level", "project", "--project", "demo")
TODAY = os.environ["PM_TEST_TODAY"]  # the date pm stamps, which conftest pins for the run


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


def test_a_codex_session_never_takes_the_claude_code_inbox_it_inherits(repo):
    """A Codex run started from a Claude Code session inherits CLAUDE_CODE_MESSAGING_SOCKET: its need stores its own
    route instead, the CODEX_HOME the pm service queues the reply under (a push to the socket would land in the Claude
    Code session), and it is told the reply is queued into it."""
    repo.env = dict(repo.env, CODEX_SESSION_ID="cx-root", CLAUDE_CODE_MESSAGING_SOCKET="/run/claude-parent.sock")
    res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *SITE_URL)
    assert res.returncode == 0, res.stderr
    assert "the pm service queues the owner's reply into this Codex session (codex queue)" in res.stdout, res.stdout
    raised = repo.items()["repo-demo.1.3"]["need"]["raised_by"]
    assert raised["session"] == "cx-root" and raised["inbox"] == f"codex-queue:{repo.env['CODEX_HOME']}", raised
    # the Claude Code session itself stores its inbox, and is told the reply is pushed into it
    repo.env = dict(repo.env, CODEX_SESSION_ID="", CLAUDE_CODE_SESSION_ID="cl-1")
    res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *SITE_URL)
    assert ("the pm service pushes the owner's reply into this session; if the session has ended by then, pm show "
            "flags it for the next one, which reads it with pm reply read repo-demo.1.4;") in res.stdout, \
        res.stdout + res.stderr
    assert repo.items()["repo-demo.1.4"]["need"]["raised_by"]["inbox"] == "/run/claude-parent.sock"


def without(flags: tuple, *drop: tuple[str, str]) -> tuple:
    """flags without each named (flag, first value), e.g. without(SITE_URL, ("--option", "b")) drops --option b."""
    out, i = [], 0
    while i < len(flags):
        n = next(j for j in range(i + 1, len(flags) + 1) if j == len(flags) or flags[j].startswith("--"))
        if (flags[i], flags[i + 1]) not in drop:
            out += flags[i:n]
        i = n
    return tuple(out)


# Each malformed part of a decision need, with the refusal it gets.
MALFORMED_NEEDS = [
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
]


ACTION = "Restart the site on port 8767, which the new proxy expects.\n"


ANSWER = "Port 8767."


def test_a_decision_recorded_for_a_no_decision_need_marks_it_answered(repo):
    """pm decision close names its undo: a decision that cites the need, which marks it answered."""
    # folded: test_decision_add_refuses_a_malformed_part_and_writes_nothing
    # The decision and its reason are flags, one line each; a malformed one is refused before any write.
    for flags, error in [
        (("--decision", DECISION), "the following arguments are required: --reason"),
        (("--decision", f"{DECISION}\nMore.", "--reason", REASON), "--decision has more than one line"),
        (("--decision", DECISION, "--reason", " "), "--reason is empty"),
        (("--decision", DECISION, "--reason", "::: result"), "--reason starts with ':::'"),
    ]:
        heads = repo.store_log()
        res = repo.pm(*ADD, *flags)
        assert res.returncode != 0 and error in res.stderr, res.stderr
        assert repo.unchanged() and repo.store_log() == heads
        assert repo.git("status", "--porcelain", cwd=repo.store) == ""

    res = repo.pm("decision", "close", "repo-demo.1.2", "--reason", "It sets no rule.", text=ANSWER)
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip().endswith("; if it sets a rule after all, record it with pm decision add --need "
                                       "repo-demo.1.2, which marks the need answered")
    res = repo.pm(*ADD, "--need", "repo-demo.1.2", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("marked need repo-demo.1.2 answered and added a source=owner project decision")
    assert repo.items()["repo-demo.1.2"]["resolution"] == "answered"
    assert repo.pm("check").returncode == 0


# ---------------------------------------------------------------- pm sprint move

MOVE_REASON = "It belongs to the site.\nThe site project owns every page."


def site_project(repo):
    """Seed a second open project, site, with sprint 1 and their records, and a task in demo-1 that sess-a holds."""
    repo.write("projects/site.md", project("Site", "repo-site"))  # the import reads them
    repo.write("sprints/site-1.md", sprint("Pages", "repo-site.1"))
    repo.commit("site project")
    repo.add_issue({"id": "repo-site", "title": "Site", "status": "open", "issue_type": "epic",
                    "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    repo.add_issue({"id": "repo-site.1", "title": "Sprint 1: Pages", "status": "open", "issue_type": "epic",
                    "parent": "repo-site", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    add_task(repo)


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
        res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=MOVE_REASON)
    finally:
        sprints.chmod(0o755)
    assert res.returncode != 0
    assert "the work store holds the move (repo-demo.1 is sprint 2 of site): run the same command again" in res.stderr
    assert repo.record_files() == files and repo.git("status", "--porcelain", cwd=repo.store) == ""
    note = ("pm sprint move: from repo-demo sprint 1 to repo-site sprint 2\n" + MOVE_REASON)
    assert repo.changes() == {"repo-demo.1": {"parent": "repo-site", "number": 2, "title": "Sprint 2: First",
                                              "comments": [{"kind": "note", "author": "pm", "text": note}]}}

    def placed():
        """pm show puts the sprint under site only, named by its number, and the old id shows it there."""
        assert "Sprint 2: First  repo-demo.1  " in repo.pm("show", "--project", "site").stdout
        demo = repo.pm("show", "--project", "demo")
        assert demo.returncode == 0 and "First" not in demo.stdout and "Second" in demo.stdout
        top = repo.pm("show")
        assert top.returncode == 0 and "decision repo-demo.1.2  Ask the owner  (sprint 2)" in top.stdout
        shown = repo.pm("show", "repo-demo.1").stdout
        assert "parent: repo-site  project  open  Site" in shown and "pm sprint move: from repo-demo" in shown
        assert repo.pm("check").returncode == 0

    placed()
    assert "records/sprints/demo-1.md" in repo.pm("show", "--sprint", "repo-demo.1").stdout  # not renamed yet
    repo.mark()
    refused(repo, "sprint", "move", "repo-demo.1", "--to", "demo", text=MOVE_REASON,
            match=r"the move of sprint repo-demo.1 to site is not finished: its record is still "
                  r"records/sprints/demo-1.md; run pm sprint move repo-demo.1 --to site first")

    repo.mark()
    res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=MOVE_REASON)
    assert res.returncode == 0, res.stderr
    assert repo.unchanged(), "the rerun writes the records step alone"
    assert committed(repo, ["pm: finished moving sprint repo-demo.1 from demo (sprint 1) to site (sprint 2): record "
                            "records/sprints/site-2.md, a decision in both projects (the work store held it already)"])
    assert not (repo.records / "sprints/demo-1.md").exists()
    assert (repo.records / "sprints/site-2.md").read_text() == old
    demo, site = ((repo.records / f"projects/{n}.md").read_text() for n in ("demo", "site"))
    assert f"Moved sprint 1 (repo-demo.1) to site as sprint 2: {MOVE_REASON}\n:::" in demo
    assert f"Took in sprint 1 of demo (repo-demo.1) as sprint 2: {MOVE_REASON}\n:::" in site
    placed()
    assert "records/sprints/site-2.md" in repo.pm("show", "--sprint", "repo-demo.1").stdout
    items = repo.items()
    assert items["repo-demo.1.3"]["holder"]["session"] == "sess-a" and items["repo-demo.1.3"]["parent"] == "repo-demo.1"
    assert items["repo-demo.1.2"]["parent"] == "repo-demo.1"
    # The old record path still finds the record.
    res = repo.pm("show", "--record", "records/sprints/demo-1.md", "--section", "Findings")
    assert res.returncode == 0 and "It parses 9 of 10." in res.stdout, res.stderr
    refused(repo, "sprint", "move", "repo-demo.1", "--to", "site", text=MOVE_REASON,
            match=r"sprint repo-demo.1 is in project site already, as sprint 2 \(records/sprints/site-2.md\)")
    # Neither project mints a number it used again: demo's 1 moved away, site's 2 moved in.
    for name, n in (("demo", 3), ("site", 3)):
        res = repo.pm("sprint", "open", name, "--title", "Next", text=FRAME)
        assert res.returncode == 0 and f"opened sprint {n} of {name}" in res.stdout, res.stderr


def test_sprint_move_in_one_go_and_its_refusals(repo):
    site_project(repo)
    refused(repo, "sprint", "move", "repo-demo.1.3", "--to", "site", text=MOVE_REASON,
            match="repo-demo.1.3 is not a sprint in the work store")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "old", text=MOVE_REASON,
            match=r"project old \(repo-old\) is closed; move the sprint to an open project")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "nope", text=MOVE_REASON, match='no project record named "nope"')
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "site", text="One line.", match="the decision body is a single line")
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "demo", text=MOVE_REASON,
            match=r"sprint repo-demo.2 is in project demo already, as sprint 2")
    res = repo.pm("sprint", "move", "repo-demo.2", "--to", "site", text=MOVE_REASON)
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: moved sprint repo-demo.2 from demo (sprint 2) to site (sprint 2): record "
                            "records/sprints/site-2.md, a decision in both projects"])
    assert repo.items()["repo-demo.2"]["number"] == 2 and repo.items()["repo-demo.2"]["parent"] == "repo-site"
    assert repo.pm("sprint", "move", "repo-demo.2", "--to", "demo", text=MOVE_REASON).returncode == 0
    assert (repo.records / "sprints/demo-3.md").exists(), "back in demo, it takes a new number: 2 moved away"
    assert repo.pm("sprint", "close", "repo-demo.2").returncode == 0
    repo.mark()
    refused(repo, "sprint", "move", "repo-demo.2", "--to", "site", text=MOVE_REASON,
            match="sprint repo-demo.2 is closed; only an open sprint moves")

    # folded: test_sprint_move_rerun_on_a_clone_whose_records_lag_writes_the_same_records_step
    # Another clone finished the move; this clone's work store holds it, but its records branch has not synced the
    # other's records commit, so a rerun here writes the records step again. It writes the same step, whatever the day
    # or time zone the clone runs in (the move decisions carry the move note's UTC date), so the records sync's rebase
    # onto the other clone's commit drops it, and each project holds its move decision once.
    # (the site project seeded above; repo-demo.1 has not moved)
    env = repo.env
    repo.env = {**env, "TZ": "Pacific/Kiritimati"}  # UTC+14: the clone that finished the move
    res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=MOVE_REASON)
    assert res.returncode == 0, res.stderr
    finished = repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    repo.git("reset", "-q", "--hard", "HEAD~1", cwd=repo.store)  # this clone, whose records lag
    repo.env = {**env, "TZ": "Pacific/Pago_Pago"}  # UTC-11: a day behind the other clone's
    res = repo.pm("sprint", "move", "repo-demo.1", "--to", "site", text=MOVE_REASON)
    assert res.returncode == 0 and "finished moving" in res.stdout, res.stderr
    repo.git("rebase", "-q", finished, cwd=repo.store)  # the records sync rebases onto the remote's records
    assert repo.git("rev-parse", "HEAD", cwd=repo.store).strip() == finished
    for name in ("demo", "site"):
        assert (repo.store / f"projects/{name}.md").read_text().count("repo-demo.1") == 1


RENAME_REASON = "The goal now covers the parser's tables too.\nThe old title named the parser alone."


def test_sprint_edit_renames_the_sprint_in_both_stores_and_records_why(repo):
    """pm sprint edit --title: the work-store title keeps its "Sprint <n>: ", the record's title header changes, and the
    reason lands as a sprint decision, in one records commit; each refusal writes nothing."""
    refused(repo, "sprint", "edit", "repo-demo.1.2", "--title", "X", text=RENAME_REASON,
            match="repo-demo.1.2 is not a sprint in the work store")
    refused(repo, "sprint", "edit", "repo-demo.1", "--title", "First", text=RENAME_REASON,
            match='sprint repo-demo.1 is titled "First" already')
    refused(repo, "sprint", "edit", "repo-demo.1", "--title", "Sprint 1: Tables", text=RENAME_REASON,
            match=r"--title \"Sprint 1: Tables\" carries a 'Sprint <n>: ' prefix; give the title alone")
    refused(repo, "sprint", "edit", "repo-demo.1", "--title", "Tables", text="One line.",
            match="the decision body is a single line")
    res = repo.pm("sprint", "edit", "repo-demo.1", "--title", "Parse: tables too", text=RENAME_REASON)
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {"repo-demo.1": {"title": "Sprint 1: Parse: tables too"}}
    assert committed(repo, ["pm: renamed sprint repo-demo.1 to \"Sprint 1: Parse: tables too\" and added a sprint "
                            "decision to records/sprints/demo-1.md"])
    text = (repo.records / "sprints/demo-1.md").read_text()
    assert text.startswith('---\ntype: sprint\ntitle: "Parse: tables too"\nbead: repo-demo.1\n---\n')
    assert (f"::: decision {{source=agent date={TODAY}}}\nRenamed the sprint from \"First\" to \"Parse: tables too\": "
            f"{RENAME_REASON}\n:::") in text
    assert "Sprint 1: Parse: tables too  .1  " in repo.pm("show", "--project", "demo").stdout
    assert repo.pm("check").returncode == 0
    repo.mark()
    assert repo.pm("sprint", "close", "repo-demo.2").returncode == 0
    repo.mark()
    refused(repo, "sprint", "edit", "repo-demo.2", "--title", "Later", text=RENAME_REASON,
            match="sprint repo-demo.2 is closed; only an open sprint is renamed")


def test_the_dates_pm_stamps_hold_the_harness_s_date_after_midnight(repo):
    """The run that failed PR #104's CI took TODAY before midnight and had pm write after it. Here the harness's date
    is the day before the wall clock's, as when TODAY was taken at 23:59:59 and pm writes at 00:00:01: the feedback
    doc's date, a sprint decision (pm decision add; pm sprint edit and pm design unchanged write theirs the same way)
    and a doc's name all carry the harness's date, so the tests' TODAY still matches."""
    setup = (datetime.date.today() - datetime.timedelta(days=1)).isoformat()
    repo.env = dict(repo.env, PM_TEST_TODAY=setup, CLAUDE_CODE_SESSION_ID="sess-1")
    assert repo.pm("feedback", "add", text="A rule cost a retry.").returncode == 0
    feedback = (repo.records / "docs/pm-feedback.md").read_text()
    assert feedback.startswith(f"---\ntype: doc\ntitle: pm feedback\ndate: {setup}\n---\n\n")
    res = repo.pm(*ADD[:2], "--level", "sprint", "--sprint", "repo-demo.1", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    record = (repo.records / "sprints/demo-1.md").read_text()
    assert f"::: decision {{source=agent date={setup}}}\n{DECISION}" in record
    res = repo.pm("doc", "new", "probe", "--title", "Probe", "--project", "demo", text="Body.")
    assert res.returncode == 0 and res.stdout == f"created records/docs/{setup}-probe.md\n", res.stderr
    assert repo.pm("check").returncode == 0


def design_and_doc_to_rename(repo):
    """Seed design page parser, linked from sprint 1's Design pages, from design page lexer and from a doc beside the
    doc notes, which links it too; sprint 2 waives parser. Every link to them is committed."""
    (repo.records / "design").mkdir()
    (repo.records / "docs").mkdir()
    no_covers = lambda text: re.sub(r"covers: .*\n", "", text)  # noqa: E731
    repo.write("design/parser.md", no_covers(design_page("[x]")).replace(
        "## Design\n\nNone yet.", "## Design\n\nIt reads [itself](parser.md#design)."))
    repo.write("design/lexer.md", no_covers(design_page("[x]", title="The lexer")).replace(
        "## Design\n\nNone yet.", "## Design\n\nThe [parser](parser.md#problem) reads its tokens; "
        "see [the parser](../design/parser.md) and [the notes](../docs/2026-10-03-notes.md)."))
    repo.write("docs/2026-10-03-notes.md", "---\ntype: doc\ntitle: Notes\ndate: 2026-10-03\nproject: demo\n---\n\n"
               "Read [The parser](../design/parser.md).\n")
    repo.write("docs/2026-10-04-more.md", "---\ntype: doc\ntitle: More\ndate: 2026-10-04\nproject: demo\n---\n\n"
               "After [Notes](2026-10-03-notes.md#top).\n")
    sprint1 = repo.records / "sprints/demo-1.md"
    sprint1.write_text(sprint1.read_text().replace("## Design pages\n\n> Where is the detail?\n\nNone yet.",
                                                   "## Design pages\n\n> Where is the detail?\n\n"
                                                   "- [The parser](../design/parser.md)"))
    repo.commit("pages and docs that link each other")
    assert repo.pm("design", "unchanged", "parser", "--sprint", "repo-demo.2", "--reason", "R.").returncode == 0


def test_design_rename_moves_the_page_and_rewrites_every_link_and_waiver(repo):
    """pm design rename: the page moves and takes its new title, and every record's link to it (anchors kept, a link
    text that was the old title taken to the new one) and sprint 2's waiver of it name the new slug, in one commit;
    each refusal writes nothing, an open review naming the page included."""
    design_and_doc_to_rename(repo)
    report(repo, "Done: shipped the thing.")
    repo.set_pr(PR, "OPEN")
    assert repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.1", "--focus", "F",
                   "--design", "parser").returncode == 0
    repo.mark()
    refused(repo, "design", "rename", "parser", "--to", "grammar",
            match=r"the open PR review repo-demo\.1\.3 names design page parser \(--design\); close it")
    assert repo.pm("need", "dismiss", "repo-demo.1.3", "--reason", "[TEST] done").returncode == 0
    repo.mark()
    refused(repo, "design", "rename", "parser", match="give --to NEW_SLUG, --title")
    refused(repo, "design", "rename", "nope", "--to", "x", match=r'no design page "nope"; known: lexer, parser')
    refused(repo, "design", "rename", "parser", "--to", "Bad_Slug",
            match=r"design slug \"Bad_Slug\" must be lowercase words joined by '-'")
    refused(repo, "design", "rename", "parser", "--to", "lexer", match=r"records/design/lexer\.md already exists")
    refused(repo, "design", "rename", "parser", "--title", "  ", match="--title is empty")
    refused(repo, "design", "rename", "parser", "--to", "parser", "--title", "The parser",
            match=r'design page is named parser and titled "The parser" already')
    page = repo.records / "design/parser.md"
    page.write_text(page.read_text() + "A hand edit.\n")
    refused(repo, "design", "rename", "parser", "--to", "grammar", match=r"records/design/parser\.md has uncommitted")
    repo.git("checkout", "--", ".", cwd=repo.store)

    res = repo.pm("design", "rename", "parser", "--to", "grammar", "--title", "The grammar")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ['pm: renamed design page parser to grammar, titled "The grammar"; rewrote its links in '
                            "records/design/lexer.md, records/docs/2026-10-03-notes.md, records/sprints/demo-1.md, "
                            "records/sprints/demo-2.md"])
    assert not page.exists()
    text = (repo.records / "design/grammar.md").read_text()
    assert text.startswith('---\ntype: design\ntitle: The grammar\nproject: demo\n---\n')
    assert "It reads [itself](grammar.md#design)." in text
    lexer = (repo.records / "design/lexer.md").read_text()
    assert ("The [parser](grammar.md#problem) reads its tokens; see [the parser](../design/grammar.md) and "
            "[the notes](../docs/2026-10-03-notes.md).") in lexer
    assert "- [The grammar](../design/grammar.md)" in (repo.records / "sprints/demo-1.md").read_text()
    assert "Read [The grammar](../design/grammar.md)." in (repo.records / "docs/2026-10-03-notes.md").read_text()
    assert ("The design page grammar stays as it is for this sprint's code changes.\nR.\n"
            in (repo.records / "sprints/demo-2.md").read_text())
    assert repo.pm("check").returncode == 0
    res = repo.pm("design", "rename", "grammar", "--title", "Grammar")
    assert res.returncode == 0 and res.stdout == ('retitled design page grammar "Grammar"; rewrote its links in '
                                                  "records/docs/2026-10-03-notes.md, "
                                                  "records/sprints/demo-1.md\n"), res.stdout
    assert "- [Grammar](../design/grammar.md)" in (repo.records / "sprints/demo-1.md").read_text()


def test_doc_rename_keeps_the_date_prefix_and_rewrites_every_link(repo):
    """pm doc rename: by slug, date or path, the date prefix kept; the feedback doc and an ambiguous slug are
    refused."""
    design_and_doc_to_rename(repo)
    repo.write("docs/2026-10-05-notes.md", "---\ntype: doc\ntitle: Notes again\ndate: 2026-10-05\nproject: demo\n"
               "---\n\nAgain.\n")
    repo.write("docs/pm-feedback.md", "---\ntype: doc\ntitle: pm feedback\ndate: 2026-10-07\n---\n\nNone yet.\n")
    repo.commit("a second notes doc and the feedback doc")
    repo.mark()
    refused(repo, "doc", "rename", "notes", "--to", "log",
            match=r'"notes" names 2 docs: records/docs/2026-10-03-notes\.md, records/docs/2026-10-05-notes\.md; give')
    refused(repo, "doc", "rename", "nope", "--to", "log", match=r'no doc "nope" under records/docs')
    refused(repo, "doc", "rename", "records/docs/pm-feedback.md", "--to", "log",
            match=r"records/docs/pm-feedback\.md is the repo's feedback doc")
    refused(repo, "doc", "rename", "2026-10-05-notes", "--to", "notes",
            match=r'doc is named 2026-10-05-notes and titled "Notes again" already')
    res = repo.pm("doc", "rename", "records/docs/2026-10-03-notes.md", "--to", "log", "--title", "Log")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ['pm: renamed doc 2026-10-03-notes to 2026-10-03-log, titled "Log"; rewrote its links in '
                            "records/design/lexer.md, records/docs/2026-10-04-more.md"])
    assert not (repo.records / "docs/2026-10-03-notes.md").exists()
    assert (repo.records / "docs/2026-10-03-log.md").read_text().startswith(
        "---\ntype: doc\ntitle: Log\ndate: 2026-10-03\nproject: demo\n---\n")
    assert "After [Log](2026-10-03-log.md#top)." in (repo.records / "docs/2026-10-04-more.md").read_text()
    assert "[the notes](../docs/2026-10-03-log.md)" in (repo.records / "design/lexer.md").read_text()
    assert repo.pm("doc", "rename", "notes", "--to", "notes-again").returncode == 0, "one notes doc is left"
    assert (repo.records / "docs/2026-10-05-notes-again.md").exists()
    assert repo.pm("check").returncode == 0


def test_need_edit_rewrites_an_open_need_s_body_until_the_owner_replies(repo):
    """pm need edit: an action takes a new description, a decision need new parts in the one layout; a need that holds
    a reply, a closed need, a PR review and a task are refused, as is the wrong form of body for the kind."""
    repo.set_pr(PR, "OPEN")
    # folded: test_decision_need_refuses_a_malformed_part_and_writes_nothing
    for flags, error in MALFORMED_NEEDS:
        res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *flags)
        assert res.returncode != 0 and error in res.stderr, res.stderr
        assert repo.unchanged() and "repo-demo.1.3" not in repo.items()
    # every broken rule is listed in one refusal, each in its own wording, not only the first
    refused(repo, "decision", "need", "--title", "Site URL", "--parent", "repo-demo.9",
            *without(SITE_URL, ("--cost", "a")), "--fact", " ".join(["word"] * 26) + ".",
            match=r"this breaks 3 rules; fix them all, then run it again:\n"
                  r"- option a has no cost; add --cost a '<what it costs>'\n"
                  r"- the sentence \"word word word word word word …\" in fact 3 has 26 words; the limit is 25 "
                  r"\(ASD-STE100\); split it\n"
                  r"- --parent repo-demo.9 is not in the work store$")

    assert repo.pm("action", "need", "--title", "Restart", "--parent", "repo-demo.1", text=ACTION).returncode == 0
    res = repo.pm("decision", "need", "--title", "Site URL", "--parent", "repo-demo.1", *SITE_URL)
    assert res.returncode == 0, res.stderr
    # a session pm has no inbox for gets no push: it is told where the reply shows
    assert ("this session gets no push of the owner's reply (no inbox pm can push to): pm show flags it once it is "
            "posted, and pm reply read repo-demo.1.4 prints it") in res.stdout
    assert repo.pm("action", "need", "--pr", PR, "--sprint", "repo-demo.2", "--focus", "F").returncode == 0
    action, decision, review = "repo-demo.1.3", "repo-demo.1.4", "repo-demo.2.1"
    assert {repo.items()[i]["need"]["kind"] for i in (action, decision, review)} == {"action", "decision", "review"}
    repo.mark()
    res = subprocess.run([*PM, "need", "edit", action, "--text-file", "-"], cwd=repo.root, env=repo.env,
                         input="Restart the site on port 8768.\nThe proxy moved to `8768`.\n", capture_output=True,
                         text=True, check=False)
    assert res.returncode == 0 and res.stdout == f"edited the body of action {action}\n", res.stderr
    assert repo.changes() == {action: {"description": "Restart the site on port 8768.\nThe proxy moved to `8768`."}}
    repo.mark()
    res = repo.pm("need", "edit", decision, *NEED)
    assert res.returncode == 0, res.stderr
    assert repo.changes() == {decision: {"description": (
        "**Question:** Which parser should we use?\n\n**Facts:**\n\n- Records hold no tables yet.\n\n**Options:**\n\n"
        "- **(small) The small parser.** *Cost:* no tables.\n- **(full) The full parser.** *Cost:* a new dependency.\n\n"
        "**Default:** (small). It is cheap.")}}
    repo.mark()
    refused(repo, "need", "edit", decision, *NEED, match=f"{decision} has this body already")
    refused(repo, "need", "edit", decision, text="Free text.",
            match=f"{decision} is a decision need; its body is its parts, given as flags, not --text")
    refused(repo, "need", "edit", decision, *NEED[:2], match="give exactly one --default")
    refused(repo, "need", "edit", action, *NEED, match=f"{action} is an action; its body is the description")
    refused(repo, "need", "edit", action, text="Please review and merge PR #12.", match="raise it with the review form")
    refused(repo, "need", "edit", review, text="X.", match=f"{review} is a PR review, whose card pm builds")
    refused(repo, "need", "edit", "repo-demo.2", text="X.", match="repo-demo.2 is a sprint, not a need")
    assert repo.pm("reply", "add", action, "--text=Done it.").returncode == 0
    repo.mark()
    refused(repo, "need", "edit", action, text="Restart it twice.",
            match=f"{action} holds the owner's reply, which answers the body it has; raise a new need")
    assert repo.pm("need", "dismiss", decision, "--reason", "[TEST] done").returncode == 0
    repo.mark()
    refused(repo, "need", "edit", decision, *SITE_URL, match=rf"{decision} is closed \(dismissed\)")


# ---------------------------------------------------------------- pm task close

def add_task(repo, issue_id="repo-demo.1.3", **fields):
    """A task session sess-a claimed, as bd exports it."""
    repo.add_issue(dict(
        {"id": issue_id, "title": "Write the parser", "status": "in_progress", "issue_type": "task",
         "parent": "repo-demo.1", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z",
         "started_at": "2026-10-01T12:00:00Z",
         "metadata": {"claimed_by": "sess-a", "claimed_at": "2026-10-01T12:00:00Z"}}, **fields))


def test_task_close_dropped_records_why_and_needs_no_commit(repo):
    """A dropped task closes as not done (resolution dismissed) with its reason: no commit looked up, no warning, even
    in a dirty tree; pm show shows it as dropped."""
    add_task(repo)
    (repo.root / "wip.txt").write_text("not committed\n")
    refused(repo, "task", "close", "repo-demo.1.3", "--dropped", match=r"--dropped needs --reason")
    refused(repo, "task", "close", "repo-demo.1.3",
            match=r"--reason is required: what was done, in a sentence; pm adds the commit that holds the work, or "
                  r"close a task not done with --dropped --reason \"<why>\"")
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
    refused(repo, "task", "close", "repo-demo.1.3", "--reason", "Done", "--commit", "Yeeef/pm@fedcba9",
            match=r"--commit Yeeef/pm@fedcba9 is not a commit of Yeeef/pm on GitHub: gh api "
                  r"repos/Yeeef/pm/commits/fedcba9 failed: gh: No commit found for SHA: fedcba9")
    refused(repo, "task", "close", "repo-demo.1.3", "--reason", "Done", "--commit", "https://github.com/Yeeef/pm/pull/8",
            match=r"--commit https://github.com/Yeeef/pm/pull/8 is not a pull request on GitHub: gh pr view")
    repo.set_pr("https://github.com/Yeeef/pm/pull/10", "CLOSED")
    refused(repo, "task", "close", "repo-demo.1.3", "--reason", "Done", "--commit", "https://github.com/Yeeef/pm/pull/10",
            match=r"--commit https://github.com/Yeeef/pm/pull/10 was closed without merging, so it holds no work")
    refused(repo, "task", "close", "repo-demo.1.3", "--reason", "Done", "--commit", "no-such-ref",
            match=r"--commit no-such-ref is not a commit of this repo, an OWNER/REPO@SHA on GitHub or a PR URL")
    res = repo.pm("task", "close", "repo-demo.1.3", "--reason", "Parser written.", "--commit", "Yeeef/pm@0123456")
    assert res.returncode == 0, res.stderr
    assert repo.pm("task", "close", "repo-demo.1.4", "--reason", "Done", "--commit", merged).returncode == 0
    assert repo.pm("task", "close", "repo-demo.1.5", "--reason", "Done", "--commit", open_pr).returncode == 0
    items = repo.items()
    assert items["repo-demo.1.3"]["close_reason"] == "Parser written. (commit Yeeef/pm@0123456)"
    assert items["repo-demo.1.4"]["close_reason"] == f"Done (PR {merged}, merged as Yeeef/pm@0123456)"
    assert items["repo-demo.1.5"]["close_reason"] == f"Done (PR {open_pr})"
    # a branch of this repo named like OWNER/REPO@SHA is this repo's commit: no gh call
    repo.git("branch", "Yeeef/pm@cafe1234")
    assert repo.pm("task", "close", "repo-demo.1.6", "--reason", "Done", "--commit", "Yeeef/pm@cafe1234").returncode == 0
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    assert repo.items()["repo-demo.1.6"]["close_reason"] == f"Done (commit {head})"


def test_task_close_refuses_a_task_another_live_session_holds(repo):
    add_task(repo)  # held by sess-a
    transcript(repo, "sess-a", 60)
    refused(repo, "task", "close", "repo-demo.1.3", "--reason", "Done",
            match=r"repo-demo\.1\.3 is held by live session sess-a \(claimed \S+ ago; its transcript was written in the "
                  r"last 30 minutes\); leave it, or ask that session to close it, or to release it with pm task release "
                  r"repo-demo\.1\.3")
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-a")  # the holder itself closes it
    res = repo.pm("task", "close", "repo-demo.1.3", "--reason", "Done")
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["status"] == "closed"


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
    return subprocess.run([*PM, "init", *args], cwd=cwd, env=env, capture_output=True, text=True, check=False)


def schedule_line(clone: Path, home: Path, state: str) -> str:
    """pm where's schedule line for a clone, on this platform's scheduler as the fakes present it."""
    name = f"local.pm-push.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.timer")
    return f"schedule  {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


def sched_calls(tmp_path) -> list[list[str]]:
    log = tmp_path / "sched.log"
    return [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []


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
    assert git_in(clone, "config", "--get", "--default=", "core.hooksPath") == "\n", "pm installs no git hook"
    assert git_in(clone / ".pm/store/records", "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (clone / "records").is_symlink() and (clone / "records/sprints/demo-1.md").read_text() == "one\n"
    assert git_in(clone, "status", "--porcelain") == ""
    config = (clone / ".git/config").read_text()
    where = subprocess.run([*PM, "where"], cwd=clone,
                           env=fake_env(tmp_path, GIT_ENV), capture_output=True, text=True,
                           check=True).stdout.splitlines()
    assert where[0].startswith(f"pm        {VERSION}  ")
    assert where[1] == f"store     {clone}/.pm/store/records  branch records, 0 ahead, 0 behind origin/records (as of the last fetch)"
    # pm init gives the clone its id once, its namespace for draft refs on the remote
    clone_id = git_in(clone, "config", "--get", "pm.clone").strip()
    assert re.fullmatch("[0-9a-f]{12}", clone_id), clone_id
    assert f"gave this clone the id {clone_id}" in res.stdout
    assert where[2] == f"clone     {clone_id}  its drafts push to origin refs/pm/drafts/{clone_id}/"
    assert where[3] == f"checkout  {clone}  branch main, records link set up"
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
    # folded: test_sprints_list_in_natural_id_order_on_the_overview_the_project_page_and_pm_show
    # Beads ids order by their numbers, not as text: .9, .39, .100, where text order puts .100 before .39.
    from conftest import sprint
    for n in (100, 39, 9):  # each sprint's record first: the work store imports a sprint only with its record
        repo.write(f"sprints/demo-{n}.md", sprint(f"Number {n}", f"repo-demo.{n}"))
    repo.commit("sprints 9, 39 and 100")
    for n in (100, 39, 9):
        repo.add_issue({"id": f"repo-demo.{n}", "title": f"Sprint {n}: Number {n}", "status": "open",
                        "issue_type": "epic", "parent": "repo-demo", "created_at": "2026-10-01T12:00:00Z",
                        "updated_at": "2026-10-01T12:00:00Z"})
    titles = ("Number 9", "Number 39", "Number 100")

    def in_order(text: str) -> bool:
        return -1 < text.index(titles[0]) < text.index(titles[1]) < text.index(titles[2])

    pages = repo.pages()
    assert in_order(pages["index.html"])
    project = pages["projects/demo.html"]
    assert in_order(project) and in_order(project[project.index("<table>"):])  # the graph, then the sprint table
    res = repo.pm("show", "--project", "demo")
    assert res.returncode == 0, res.stderr
    assert in_order(res.stdout), res.stdout

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

    # folded: test_a_reviewed_prs_merge_is_pushed_into_the_session_once
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
            deadline = time.time() + 2  # nothing more arrives: an absence holds for a window, not until a condition
            while time.time() < deadline:
                assert len(lines) == 1
                time.sleep(0.1)  # noqa: TID251 — the poll interval of the window the absence must hold for
    assert repo.pm("reply", "read", "repo-demo.1.3").stdout == "nothing undelivered on repo-demo.1.3\n"


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


def wait_on_open_stdin(repo, tmp_path, args, text, with_text):
    """Run pm with `args` (and --text=`text` if with_text) and its stdin an open pipe: it must return within 5 s, with
    its output, or, without the text it needs, a refusal naming --text."""
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


def text_command(name: str, with_text: bool) -> tuple:
    """TEXT_COMMANDS' entry for the command `name` (its first two words), as (args, text, with_text)."""
    args, text = next((a, t) for a, t in TEXT_COMMANDS if " ".join(a[:2]) == name)
    return args, text, with_text


# The cases test_no_command_waits_on_an_open_stdin runs each on its own repo; the rest of TEXT_COMMANDS' cases run
# in test_text_file_reads_a_file_and_refuses_with_text, on one repo, in an order whose writes leave each its target.
OPEN_STDIN_ALONE = [text_command(n, t) for n, t in [
    ("action need", False), ("action need", True), ("doc new", False), ("feedback add", False),
    ("project open", False), ("sprint open", False), ("task move", False)]]
OPEN_STDIN_TOGETHER = [text_command(n, t) for n, t in [
    ("task move", True), ("decision close", False), ("decision close", True), ("doc new", True),
    ("feedback add", True), ("project open", True), ("sprint open", True), ("task add", False), ("task add", True)]]


@pytest.mark.parametrize(("args", "text", "with_text"), OPEN_STDIN_ALONE,
                         ids=[f"{' '.join(a[:2])}-{'text' if w else 'no-text'}" for a, _, w in OPEN_STDIN_ALONE])
def test_no_command_waits_on_an_open_stdin(repo, tmp_path, args, text, with_text):
    """Sprint 60: in Claude Code the shell's stdin is a socket that never closes, so a pm that read stdin hung
    forever. pm reads stdin only for --text-file -: each command returns, with its output or a refusal, while stdin
    stays open."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")  # feedback add names its session
    wait_on_open_stdin(repo, tmp_path, args, text, with_text)


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

    # folded: test_text_file_dash_reads_a_heredoc
    # A quoted heredoc is a pipe written and closed: its body comes through whole, quotes, backticks and $ kept.
    # (repo-demo.1.3 is the task above, so this one is repo-demo.1.4)
    body = "Parse `x`, it's $5 (or \"more\").\nSecond line )."
    code, _, err = run_task_add(repo, "--text-file", "-", stdin=subprocess.PIPE, given=body + "\n")
    assert code == 0, err
    assert repo.items()["repo-demo.1.4"]["description"] == body

    # folded: test_no_command_waits_on_an_open_stdin
    # (its cases in OPEN_STDIN_TOGETHER)
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")  # feedback add names its session
    for args, text, with_text in OPEN_STDIN_TOGETHER:
        wait_on_open_stdin(repo, tmp_path, args, text, with_text)


def test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does(repo):
    # folded: test_commit_commits_only_its_callers_records_beside_another_sessions_edit
    # Two sessions share the store: A has an edit in progress, B commits its own. B's commit holds B's file only,
    # and pm commit with no path lists both instead of sweeping A's edit in.
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
    repo.git("checkout", "-q", "--", "sprints/demo-1.md", cwd=repo.store)  # session A gave its edit up
    shutil.rmtree(repo.store / "docs")

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

    # folded: test_commit_takes_a_path_relative_to_the_store_from_anywhere
    # sprints/x.md names records/sprints/x.md from the main checkout or a worktree, as it does from inside the store;
    # a path outside the store that the store does not hold is still refused.
    wt = repo.worktree("feature-c")
    for cwd, line in ((repo.root, "Ship it, from main."), (wt, "Ship it, from a worktree.")):
        path = repo.store / "sprints/demo-1.md"
        path.write_text(re.sub(r"Ship it[^\n]*", line, path.read_text(), count=1))
        res = repo.pm("commit", "-m", line, "sprints/demo-1.md", cwd=cwd)
        assert res.returncode == 0, res.stderr
        assert res.stdout.startswith("committed records/sprints/demo-1.md as "), res.stdout
        assert committed(repo, [line])
    (repo.store / "docs").mkdir()
    (repo.store / "docs/2026-10-07-new.md").write_text("---\ntype: doc\ntitle: New\ndate: 2026-10-07\nproject: demo\n---\n\nBody.\n")
    refused(repo, "commit", "-m", "Not here", ".gitignore", match=r"\.gitignore is not in the records store")
    res = repo.pm("commit", "-m", "A new doc", "docs/2026-10-07-new.md")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["A new doc"])


def test_pm_check_and_commit_refuse_a_record_link_to_nothing(repo):
    """A .md link whose file is gone is no record anyone can read: pm check names it, and pm commit will not commit it."""
    link = repo.store / "docs/2026-10-07-gone.md"
    link.parent.mkdir()
    link.symlink_to("nowhere.md")
    said = r"docs/2026-10-07-gone.md: a link to nowhere.md, which does not exist; remove the link or restore its file"
    before = repo.git("status", "--porcelain", cwd=repo.store)
    refused(repo, "check", match="error: " + said)
    refused(repo, "commit", "-m", "Add a link", "records/docs/2026-10-07-gone.md", match="error: " + said)
    assert repo.git("status", "--porcelain", cwd=repo.store) == before


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
    def page_if_shown() -> str | None:
        page = get()
        if shown(page):
            return page
        assert written is None or asof(page) < written, "the page states data as of after a change it does not show"
        return None
    return wait_for(page_if_shown, "the change to show within the stated bound",
                    timeout=(written or time.time()) + SERVE_BEHIND - time.time())


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


def fetch(url: str) -> tuple[int, str]:
    """The status and the page at `url`."""
    import urllib.error
    try:
        with urllib.request.urlopen(url) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


@pytest.mark.integration
def test_a_hand_edit_is_a_version_beside_the_published_page(repo, served):
    """The site renders the records branch; the store's uncommitted edits are the draft "store". A hand edit to a
    design page leaves its published page as it was, plus a version bar; the version renders at its URL and its diff
    marks exactly the edited lines. A draft that does not parse fails only its version's URLs; every published page
    answers 200."""
    assert repo.pm("design", "new", "parser", "--title", "The parser", "--project", "demo").returncode == 0
    published = until_shown(lambda: load(f"{served}/design/parser.html"), lambda p: "The parser" in p, None)
    path = repo.store / "design/parser.md"
    text = path.read_text()
    problem = "## Problem\n\n> What are we solving, and why now?\n\nNone yet.\n"
    assert problem in text
    written = time.time()
    path.write_text(text.replace(problem, problem.replace("None yet.", "Records need a parser.\nIt streams."), 1))
    version_url = f"{served}/drafts/store/design/parser.html"
    version = until_shown(lambda: load(version_url), lambda p: "It streams." in p, written)
    assert fetch(version_url)[0] == 200 and "Records need a parser." in version and "<b>store</b>" in version
    code, page = fetch(f"{served}/design/parser.html")
    bar = re.search(r'<div class="note draft versions">.*?</div>\n', page, re.S)
    assert code == 200 and bar and "Records need a parser." not in page, page
    assert 'href="../drafts/store/design/parser.html"' in bar.group(0)
    assert "uncommitted edits in the shared store" in bar.group(0)
    def strip(p: str) -> str:  # the status line, whose age moves
        return re.sub(r'<p class="asof.*?</script>', "", p, flags=re.S)
    assert strip(page.replace(bar.group(0), "")) == strip(published)
    code, diff = fetch(f"{served}/drafts/store/design/parser.diff.html")
    assert code == 200 and re.findall(r'<span class="(?:add|del)">(.*)</span>', diff) == [
        "- None yet.", "+ Records need a parser.", "+ It streams."], diff

    path.write_text(path.read_text().replace("type: design\n", "type: design\n  bad: [\n", 1))
    until_shown(lambda: load(version_url), lambda p: "the version store does not render" in p, None)
    assert fetch(version_url)[0] == 500
    assert fetch(f"{served}/drafts/store/design/parser.diff.html")[0] == 200
    paths = repo.pages()
    assert "design/parser.html" in paths
    for p in ["", *paths]:
        code, page = fetch(f"{served}/{p}")
        assert code == 200, (p, page)
    assert "does not render" in fetch(f"{served}/design/parser.html")[1]


# ---------------------------------------------------------------- replies on the site

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def post_reply(url: str, form: dict[str, str] | list[tuple[str, str]], headers: dict[str, str] | None = None) -> tuple[int, str, str]:
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
    wait_for(lambda: len(repo.changes()) >= 2, "the comments, written in the background", timeout=10)
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
@pytest.mark.parametrize(("token", "form", "headers", "code", "said"), [
    ("forged", {"id": "repo-demo.1.2", "text": "Small."}, {}, 403, "no valid token"),
    ("page", {"id": "repo-demo.1.2", "text": "Small."}, {"Host": "evil.example:80"}, 403, "is neither this machine"),
])
def test_reply_is_refused_without_the_token_or_from_another_host(repo, served, token, form, headers, code, said):
    form = dict(form, token=page_token(served) if token == "page" else token)
    res = post_reply(served, form, headers)
    assert res[0] == code and said in res[2]
    assert repo.unchanged()


@pytest.mark.integration
def test_a_questions_card_posts_its_picks_and_the_service_stores_one_line_per_question(repo):
    """A session's question shows as a card whose inputs post through the form the service fills in its slot; the
    stored reply is one "<question>: <answer>" line per question, a multi-select's picks joined ", " and Other text
    following them; a post that leaves a question unanswered is refused and writes nothing. internal/item holds the
    composition's every case."""
    items = repo.items()
    question = {**items["repo-demo.1.2"], "id": "repo-demo.1.9", "title": "Session s-ask-12 asks: Ship it?",
                "description": "Ship it?", "comments": [],
                "need": {"kind": "question", "raised_by": {"session": "s-ask-1234", "inbox": None, "host": None},
                         "delivered": 0, "review": None,
                         "ask": {"call": "toolu_1", "questions": [
                             {"text": "Ship it?", "header": "Ship", "multi": False,
                              "options": [{"label": "Yes", "detail": "merge now"}, {"label": "No", "detail": ""}]},
                             {"text": "Which checks?", "header": "", "multi": True,
                              "options": [{"label": "lint", "detail": ""}, {"label": "go", "detail": ""}]}]}}}
    export = repo.root.parent / "with-question.jsonl"
    export.write_text("".join(json.dumps(i) + "\n" for i in [*items.values(), question]))
    repo.stop_service()
    shutil.rmtree(repo.root / ".pm/store/work")
    repo.start_service()
    assert repo.pm("init", "--import", str(export)).returncode == 0
    with serving(repo) as (url, _):
        page = urllib.request.urlopen(f"{url}/").read().decode()
        assert "Session s-ask-12 asks you" in page and '<form class="reply" id="reply-repo-demo.1.9"' in page
        assert '<input type="checkbox" name="q1" value="go" form="reply-repo-demo.1.9">' in page
        token = page_token(url)
        repo.mark()
        code, _, said = post_reply(url, [("token", token), ("id", "repo-demo.1.9"), ("q0", "Yes")])
        assert (code, said) == (400, 'refused: question 2, "Which checks?", has no answer: pick an option or fill in '
                                     "Other\n")
        assert repo.unchanged()
        code, where, _ = post_reply(url, [("token", token), ("id", "repo-demo.1.9"), ("q0", "No"), ("q1", "lint"),
                                          ("q1", "go"), ("q1-other", "vet")], {"Referer": f"{url}/"})
        assert (code, where) == (303, "/#need-repo-demo.1.9")
        wait_for(lambda: repo.items()["repo-demo.1.9"]["comments"], "the reply, written in the background", timeout=10)
    assert [reply_body(c["text"]) for c in repo.items()["repo-demo.1.9"]["comments"]] == [
        "Ship it?: No\nWhich checks?: lint, go, vet"]


@pytest.mark.integration
def test_service_refuses_the_public_host_without_access_and_serves_localhost(repo):
    """Every path and method on site_url's host, and any request Cloudflare's edge forwarded, is checked; localhost is
    not. Without the Access keys every public request is refused, naming them; with them, one without a token is (no
    token, so no network). A refusal names neither the store nor the build. internal/service holds the token checks."""
    import urllib.error

    def get(url: str, path: str, host: str | None = None, headers: dict | None = None) -> tuple[int, str]:
        req = urllib.request.Request(f"{url}/{path}", headers=({"Host": host} if host else {}) | (headers or {}))
        try:
            with urllib.request.urlopen(req) as r:
                return r.status, r.read().decode()
        except urllib.error.HTTPError as e:
            code, refused, body = e.code, e.headers, e.read().decode()
        assert "X-PM-Store" not in refused and "X-PM-Version" not in refused, refused
        return code, body

    write_config(repo.root, site_url="https://pm.example.com", access_aud="aud-tag")
    with serving(repo) as (url, _):
        assert get(url, "")[0] == 200
        for path in ("", "style.css", "version"):
            code, said = get(url, path, "pm.example.com")
            assert code == 403 and "lacks access_team (the Cloudflare Access team domain" in said, said
        code, said = get(url, "", "localhost", {"Cf-Connecting-IP": "203.0.113.9"})  # the tunnel rewrote Host
        assert code == 403 and "lacks access_team" in said, said
        code, _, said = post_reply(url, {"token": "t", "id": "repo-demo.1.2", "text": "Small."}, {"Host": "pm.example.com"})
        assert code == 403 and "lacks access_team" in said
        code, said = get(url, "", "evil.example")  # a rebinding domain reads no page
        assert code == 403 and "is neither this machine" in said
    write_config(repo.root, site_url="https://pm.example.com", access_team="team.cloudflareaccess.com",
                 access_aud="aud-tag")
    with serving(repo) as (url, _):
        token = page_token(url)
        code, _, said = post_reply(url, {"token": token, "id": "repo-demo.1.2", "text": "Small."},
                                   {"Host": "pm.example.com"})
        assert code == 403 and "carries no Cloudflare Access token" in said
        assert get(url, "", "pm.example.com:443")[0] == 403
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
                data = b"".join(iter(functools.partial(conn.recv, 65536), b""))
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
        res = subprocess.run(["bash", "-c", script], cwd=repo.root, env=env, capture_output=True, text=True,
                             check=False)
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

    # folded: test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read
    # No socket at the stored inbox: the session ended. The reply stays undelivered, its card says so, pm show
    # flags it for the next session, and pm reply read prints it once and marks it delivered.
    # (needs repo-demo.1.3 to .1.5 are raised above, so this one is repo-demo.1.6)
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET="/nonexistent/pm/s")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "repo-demo.1", *NEED).returncode == 0
    until_shown(lambda: load(f"{served}/"), lambda p: 'value="repo-demo.1.6"' in p, time.time())
    assert post_reply(served, {"token": page_token(served), "id": "repo-demo.1.6", "text": "Small, until tables."})[0] == 303
    page = until_shown(lambda: load(f"{served}/"), lambda p: "Small, until tables." in p and "Saving" not in p, None)
    assert "the session that asked is not running" in page
    assert repo.items()["repo-demo.1.6"]["need"]["delivered"] == 0
    assert "decision .1.6  Parser?  (sprint 1)  [undelivered reply: pm reply read repo-demo.1.6]" in repo.pm("show").stdout
    assert "Parser?  (sprint 1)  -> pm show repo-demo.1.6  [undelivered reply: pm reply read repo-demo.1.6]" \
        in repo.pm("show", "--project", "demo").stdout
    res = repo.pm("reply", "read")  # no ids: this session's open requests
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("pm: owner reply to decision repo-demo.1.6 (Parser?), relayed from the site:\n  [")
    assert "Small, until tables." in res.stdout
    assert repo.items()["repo-demo.1.6"]["need"]["delivered"] == 1
    assert "pm reply read" not in repo.pm("show").stdout
    assert repo.pm("reply", "read", "repo-demo.1.6").stdout == "nothing undelivered on repo-demo.1.6\n"
    repo.mark()  # refused() checks that nothing at all changed in the work store
    refused(repo, "reply", "read", "repo-demo.1.1", match=r"repo-demo.1.1 is not a request to the owner")
    repo.env.pop("CLAUDE_CODE_SESSION_ID")
    refused(repo, "reply", "read", match=r"name the requests to read")


REVIEWED_PR = "https://github.com/o/r/pull/7"
REVIEW = {"pr": REVIEWED_PR, "sprints": ["repo-demo.1"], "focus": "f", "designs": []}


def test_reply_read_prints_each_undelivered_reply_and_merge_once(repo):
    """Without the pm service: a site reply and a merge the service saw wait on this session's requests. A close
    refuses while a reply is undelivered; pm reply read prints each with what to do next, marks it delivered, and
    prints it no more."""
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
    # a Codex thread finds its session's requests as a Claude Code session does: a subagent's thread by its root
    # session's id (Codex 0.154), a thread without it by its own (an earlier Codex, with no rollout to map it by)
    for env in ({"CODEX_SESSION_ID": "sess-1", "CODEX_THREAD_ID": "sess-sub"}, {"CODEX_THREAD_ID": "sess-1"}):
        repo.env.update(env)
        res = repo.pm("reply", "read")
        assert res.stdout == "nothing undelivered on repo-demo.1.3, repo-demo.1.4\n", (env, res.stderr)
        for k in env:
            repo.env.pop(k)
    refused(repo, "reply", "read", match=r"name the requests to read")
    res = repo.pm(*ADD, "--need", "repo-demo.1.3", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.3"]["status"] == "closed"
    # the answer the close recorded is no reply of the owner's to deliver
    assert repo.pm("reply", "read", "repo-demo.1.3").stdout == "nothing undelivered on repo-demo.1.3\n"


def test_show_refresh_inbox_points_a_sessions_needs_at_its_own_inbox(repo):
    """A Claude Code session's open needs follow its socket; a Codex session's trade the Claude Code socket an earlier
    pm stored on them (inherited from the Claude Code session that started Codex) for its CODEX_HOME, which the pm
    service queues the reply under, so no reply is pushed into the Claude Code session."""
    stamps = {"created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"}
    for nid, session in (("repo-demo.1.3", "sess-cl"), ("repo-demo.1.4", "sess-cx")):
        repo.add_issue({"id": nid, "title": "Parser?", "status": "open", "issue_type": "task", "parent": "repo-demo.1",
                        "labels": ["human"], "metadata": {"session": session, "inbox": "/run/old.sock",
                                                          "inbox_host": socket.gethostname()}, **stamps})
    base = repo.env
    repo.env = dict(base, CLAUDE_CODE_SESSION_ID="sess-cl", CLAUDE_CODE_MESSAGING_SOCKET="/run/new.sock")
    assert repo.pm("show", "--refresh-inbox").returncode == 0
    repo.env = dict(base, CODEX_SESSION_ID="sess-cx", CLAUDE_CODE_MESSAGING_SOCKET="/run/new.sock")
    assert repo.pm("show", "--refresh-inbox").returncode == 0
    items = repo.items()
    assert items["repo-demo.1.3"]["need"]["raised_by"]["inbox"] == "/run/new.sock"
    assert items["repo-demo.1.4"]["need"]["raised_by"]["inbox"] == f"codex-queue:{base['CODEX_HOME']}"


def test_a_need_closed_by_its_decision_or_its_no_decision_close_records_the_session(repo):
    """pm decision add --need and pm decision close close the need as task close does: closed_by is the session."""
    stamps = {"created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"}
    for nid in ("repo-demo.1.3", "repo-demo.1.4"):
        repo.add_issue({"id": nid, "title": "Parser?", "status": "open", "issue_type": "task",
                        "parent": "repo-demo.1", "labels": ["human"], "metadata": {"session": "sess-1"}, **stamps})
    repo.env = dict(repo.env, CODEX_SESSION_ID="sess-cx")
    res = repo.pm(*ADD, "--need", "repo-demo.1.3", "--decision", DECISION, "--reason", REASON)
    assert res.returncode == 0, res.stderr
    res = repo.pm("decision", "close", "repo-demo.1.4", "--reason", "It sets no rule.", text="Small, until tables.")
    assert res.returncode == 0, res.stderr
    items = repo.items()
    assert items["repo-demo.1.3"]["closed_by"] == items["repo-demo.1.4"]["closed_by"] == "sess-cx"


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
                          capture_output=True, text=True, check=False)


def test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_worktree(repo):
    repo.set_issue("repo-demo.1.2", labels=[])
    repo.mark()
    for where in (repo.root, repo.store):  # from inside the store, the code worktree is the main checkout
        res = claim(repo, "repo-demo.1.2", "sess-a", cwd=where)
        assert res.returncode == 1, res.stderr
        main = repo.root.resolve()
        assert f"not claiming repo-demo.1.2 here: {main} is the main checkout; agents change code only in a worktree of " \
               f"their own" in res.stderr
        assert f"Make one with git, in any runtime, and work there: `git -C {main} fetch origin main && git -C {main} " \
               f"worktree add -b <branch> .claude/worktrees/<name> origin/main`" in res.stderr
    assert repo.unchanged()
    wt = repo.worktree("feature")
    assert claim(repo, "repo-demo.1.2", "sess-a", cwd=wt).returncode == 0

    # folded: test_task_claim_refuses_a_task_another_live_session_holds
    # (repo-demo.1.2 seeded without labels and the worktree made above)
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

    # folded: test_task_claim_records_the_session_given_with_session_even_with_one_in_the_environment
    # (sess-a lets go of repo-demo.1.2 first, so it is free as the original test's seed left it)
    res = subprocess.run([*PM, "task", "release", "repo-demo.1.2"], cwd=wt, capture_output=True, text=True, check=False,
                         env=dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-a"))
    assert res.returncode == 0, res.stderr
    res = claim(repo, "repo-demo.1.2", "sess-env", "--session", "sess-flag", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert repo.items()["repo-demo.1.2"]["holder"]["session"] == "sess-flag"
    assert "--session ID  the session to record; default: this session's id from the environment" in \
        repo.pm("task", "claim", "--help").stdout


def test_sprint_open_and_a_sessions_first_claim_in_a_sprint_print_the_design_pages(repo):
    """The design-page index (the work-context design page): the sprint's pages, then the project's, without repeats,
    on a session's first claim in a sprint only; the project's on sprint open; a slug without a page as missing."""
    (repo.records / "design").mkdir()
    repo.write("design/parser.md", design_page("[internal/parser/**]"))
    for rel, links in (("sprints/demo-1.md", "- [Parser](../design/parser.md)\n- [Gone](../design/gone.md)"),
                       ("projects/demo.md", "- [Parser](../design/parser.md)")):
        text = (repo.records / rel).read_text()
        head, sep, tail = text.partition("## Design pages\n\n> Where is the detail?\n\nNone yet.")
        assert sep, rel
        repo.write(rel, head + "## Design pages\n\n> Where is the detail?\n\n" + links + tail)
    repo.commit("design pages")
    repo.set_issue("repo-demo.1.2", labels=[])
    repo.add_issue({"id": "repo-demo.1.3", "title": "Another", "status": "open", "issue_type": "task",
                    "parent": "repo-demo.1", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"})
    wt = repo.worktree("feature")
    res = claim(repo, "repo-demo.1.2", "sess-a", cwd=wt)
    assert res.returncode == 0, res.stderr
    lines = res.stdout.splitlines()
    assert lines[0].startswith("claimed repo-demo.1.2 for session sess-a at ")
    assert lines[1] == "design pages (read the ones your task touches):"
    assert re.fullmatch(r"  parser  The parser  \S+/design/parser\.html", lines[2]), lines[2]
    assert lines[3:] == ["  gone    missing"]
    res = claim(repo, "repo-demo.1.3", "sess-a", cwd=wt)
    assert res.returncode == 0 and len(res.stdout.splitlines()) == 1, res.stdout
    res = repo.pm("sprint", "open", "demo", "--title", "Third", text=FRAME)
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines()[1:2] == ["design pages (read the ones your task touches):"]
    assert re.fullmatch(r"  parser  The parser  \S+/design/parser\.html", res.stdout.splitlines()[2])


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
    assert first.count(" ok: ") == 4 and "summary ok: summarized" in first, first
    assert "drafts ok: pushed 0 and deleted 0 draft refs; fetched the other clones'" in first, first
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
    assert len((repo.root / ".pm/run/push.log").read_text().splitlines()) == 8
    assert state["summary"]["message"].startswith("summarized"), "the new day file changed the activity"

    # folded: test_push_rebases_onto_a_moved_remote
    # (2026-10-02 is pushed above, so the local commit adds 2026-10-03, and the other clone takes that push first)
    repo.git("pull", "-q", "--rebase", "origin", "records", cwd=repo.other)
    move_remote(repo, "docs/remote.md", "x\n")
    repo.write("days/2026-10-03.md", DAY2.replace("2026-10-02", "2026-10-03"))
    repo.commit("local")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    assert repo.store_log()[1:3] == ["local", "remote"] and repo.store_log()[0].startswith("pm: summarized ")
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    assert push_state(repo)["records"]["message"] == "pushed 2 commit(s) after rebasing onto 1 new on origin/records"


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
    ["doc", "rename", "x", "--to", "y"], ["design", "rename", "x", "--to", "y"],
    ["postmortem", "new", "x", "--title", "X", "--project", "demo"],
    ["project", "open", "p", "--title", "P", "--text=x"], ["project", "close", "demo"],
    ["sprint", "open", "demo", "--title", "S",
     "--text=## Goal\n\nx\n\n## Scope\n\n**In:** a.\n\n**Out:** b.\n\n## Done when\n\n- x."],
    ["sprint", "close", "repo-demo.1"], ["sprint", "edit", "repo-demo.1", "--title", "S", "--text=a\nb"],
    ["need", "edit", "repo-demo.1.2", "--text=x"],
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
                             capture_output=True, text=True, stdin=subprocess.DEVNULL, check=False)
        assert (res.returncode, res.stdout, res.stderr) == (1, "", want), argv
    assert repo.snapshot() == before, "a command wrote with the service stopped"
    repo.start_service()
    assert repo.unchanged()


def test_day_summarize_asks_the_runtime_summary_runtime_names(repo):
    """pm day summarize asks claude unless summary_runtime in .pm/config.toml says codex, and never the other one: on
    codex it runs codex exec with hooks, rules and the user's config off, from a temp dir, and writes the answer with
    codex's model; a failing codex writes nothing and does not fall back to claude; any other value fails the config."""
    config = repo.root / ".pm/config.toml"
    base = config.read_text()
    log = {rt: Path(repo.env[f"FAKE_{rt.upper()}_LOG"]) for rt in ("claude", "codex")}

    def calls(rt):
        return [json.loads(line) for line in log[rt].read_text().splitlines()] if log[rt].exists() else []

    res = repo.pm("day", "summarize", "--dry-run")
    assert res.returncode == 0 and '"model": "claude-haiku-5-5"' in res.stdout, res.stdout + res.stderr
    assert len(calls("claude")) >= 1 and calls("codex") == []
    asked = len(calls("claude"))

    config.write_text(base + 'summary_runtime = "codex"\n')
    repo.env = dict(repo.env, FAKE_CODEX_FAIL="ERROR: usage limit")
    res = repo.pm("day", "summarize")
    assert res.returncode != 0 and "codex exec failed (exit 1): ERROR: usage limit; no summary written" in res.stdout + res.stderr
    assert len(calls("claude")) == asked, "no fallback to claude"
    assert not list((repo.store / "days").glob("*.summary.json"))
    del repo.env["FAKE_CODEX_FAIL"]
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stdout + res.stderr
    (summary,) = [json.loads(p.read_text()) for p in (repo.store / "days").glob("*.summary.json")][-1:]
    assert summary["model"] == "gpt-5.6-luna" and summary["text"].startswith("Summary "), summary
    call = calls("codex")[-1]
    args = call["args"]
    assert {"--ephemeral", "--ignore-user-config", "--ignore-rules"} <= set(args) and "hooks" in args
    assert args[args.index("hooks") - 1] == "--disable" and call["instructions"] is None
    assert repo.root not in Path(call["cwd"]).parents and "{activity}" not in call["stdin"]
    assert len(calls("claude")) == asked

    config.write_text(base + 'summary_runtime = "gpt"\n')
    res = repo.pm("day", "summarize")
    assert res.returncode != 0 and 'summary_runtime is "gpt"; set it to one of claude, codex' in res.stdout + res.stderr
    config.write_text(base)


def test_interactive_and_autopilot_set_the_mode_of_the_session_that_runs_them(repo):
    """pm interactive writes the session's mode file in the main checkout, from a worktree too, and pm autopilot removes
    it, for a Claude Code session and a Codex one alike; each then prints the request rule of the mode it set.
    Autopilot in autopilot changes nothing and says so, and with no session in the environment both refuse."""
    modes, tree = repo.root / ".pm/run/sessions", repo.worktree("wt")
    rule = {m: (CHECKOUT / f"prompts/request_rule_{m}.txt").read_text().strip() for m in ("autopilot", "interactive")}
    inter = f"\n\n# Asking the owner: this session is in interactive mode\n\n{rule['interactive']}\n"
    auto = f"\n\n# Asking the owner: this session is in autopilot\n\n{rule['autopilot']}\n"
    saved = repo.env
    none = {"CLAUDE_CODE_SESSION_ID": "", "CODEX_SESSION_ID": "", "CODEX_THREAD_ID": ""}
    for env, sid in (({"CLAUDE_CODE_SESSION_ID": "cl-1"}, "cl-1"), ({"CODEX_SESSION_ID": "cx-root"}, "cx-root")):
        repo.env = {**saved, **none, **env}
        res = repo.pm("interactive", cwd=tree)
        assert res.returncode == 0 and res.stdout == (f"session {sid} is in interactive mode: the owner-request check "
                                                      "lets asks in chat through until pm autopilot" + inter), res.stderr
        assert (modes / sid).is_file()
        res = repo.pm("autopilot")
        assert res.returncode == 0 and res.stdout.startswith(f"session {sid} is in autopilot: ") and not (modes / sid).exists()
        assert res.stdout.endswith(auto)
        res = repo.pm("autopilot")
        assert res.stdout == f"session {sid} is already in autopilot, the default; nothing changed" + auto, res.stderr
    repo.env = {**saved, **none}
    for command in ("interactive", "autopilot"):
        res = repo.pm(command)
        assert res.returncode == 1 and res.stderr == ("error: no agent session: neither CLAUDE_CODE_SESSION_ID nor "
                                                      "CODEX_THREAD_ID is set, so there is no session to set the mode "
                                                      "of\n"), res.stderr
    assert list(modes.iterdir()) == []
    repo.env = saved
