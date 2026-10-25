"""Sprint proposals end to end against a temp repo, as the sprint-proposals design page gives them: pm sprint propose
writes the labelled sprint, its record and the evidence finding, refuses a frame or evidence it lacks, and lists the
project's other proposals and open sprints; pm show (text and --json) and pm task ready leave a proposal out while
pm show --sprint shows it; its first task starts it; pm sprint close --dropped drops it with its reason and pm sprint
reopen makes it a proposal again; pm project close names the drop. test_every_item_id_command_on_a_proposal runs each
case of internal/cli/testdata/proposal_commands.json, which TestEveryItemIDArgumentIsClassedForAProposal holds to the
command tree, against a fresh proposal; the cases that need the site served (pm record link) run in
test_the_served_commands_on_a_proposal, an integration test."""

from __future__ import annotations

import json
import re
import subprocess

import pytest

from conftest import CHECKOUT, PM, write_config

FRAME = ("## Goal\n\nMake the sync test pass on every run. It fails one run in ten.\n\n## Scope\n\n**In:** the test.\n\n"
         "**Out:** the sync itself.\n\n## Done when\n\n- `go test -count 20` passes 20 of 20.\n\n")
EVIDENCE = "## Evidence\n\n```\nFAIL sync\n```\n\n`go test -count 20 ./internal/work -run Sync` failed 2 of 20.\n"
CASES = json.loads((CHECKOUT / "internal/cli/testdata/proposal_commands.json").read_text())


def propose(repo, title: str = "Fix the sync flake", body: str = FRAME + EVIDENCE):
    return repo.pm("sprint", "propose", "demo", "--title", title, "--text-file", "-", stdin=body)


def proposed(repo, title: str = "Fix the sync flake") -> str:
    """A new proposal's id."""
    res = propose(repo, title)
    assert res.returncode == 0, res.stderr
    return re.search(r"epic (\S+),", res.stdout).group(1)


def shown_sprints(repo) -> set[str]:
    data = json.loads(repo.pm("show", "--json").stdout)
    return {s["id"] for p in data["projects"] for s in p["sprints"]}


def test_propose_writes_the_labelled_sprint_its_record_and_the_evidence_finding(repo):
    proposed(repo, "Retry the setup")  # an earlier proposal, for the duplicate list
    res = propose(repo)
    assert res.returncode == 0, res.stderr
    out = res.stdout
    assert out.startswith("proposed sprint 4 of demo: epic repo-demo.4, record records/sprints/demo-4.md\n")
    assert ("the project's other proposals and open sprints; if one covers yours, drop yours: pm sprint close "
            'repo-demo.4 --dropped --reason "duplicate of <id>"') in out
    assert re.search(r"(?m)^  proposed  repo-demo\.3  Sprint 3: Retry the setup  Make the sync test pass on every "
                     r"run\.$", out)
    assert re.search(r"(?m)^  \S+ +repo-demo\.1  Sprint 1: First", out)
    item = repo.items()["repo-demo.4"]
    assert (item["type"], item["status"], item["labels"]) == ("sprint", "open", ["proposal"])
    record = (repo.records / "sprints/demo-4.md").read_text()
    assert "Make the sync test pass on every run." in record
    assert ("- Proposal evidence:\n\n  ```\n  FAIL sync\n  ```\n\n  `go test -count 20 ./internal/work -run Sync` failed 2 "
            "of 20.\n") in record
    assert repo.pm("check").returncode == 0


def test_propose_refuses_a_missing_frame_or_evidence_and_writes_nothing(repo):
    repo.mark()
    for body, says in ((FRAME, "'## Evidence' is missing or empty"),
                       (FRAME + "## Evidence\n\n", "'## Evidence' is missing or empty"),
                       (FRAME.replace("## Done when", "## Notes"), "unknown section '## Notes'"),
                       (EVIDENCE, "'## Goal' is missing or empty")):
        res = propose(repo, body=body)
        assert res.returncode != 0 and says in res.stderr, (body, res.stderr)
    assert repo.unchanged()
    assert not (repo.records / "sprints/demo-3.md").exists()


def test_pm_show_and_task_ready_leave_a_proposal_out_until_its_first_task_starts_it(repo):
    pid = proposed(repo)
    before = repo.pm("show").stdout
    assert "2 open sprints" in before and pid not in before
    assert pid not in repo.pm("show", "--project", "demo").stdout
    assert pid not in shown_sprints(repo) and "proposal" not in repo.pm("show", "--json").stdout
    assert repo.pm("task", "ready").stdout.strip() == "no ready tasks"
    detail = repo.pm("show", "--sprint", pid).stdout
    assert detail.startswith(f"Sprint 3: Fix the sync flake  {pid}  proposed  ") and "Proposal evidence:" in detail
    assert repo.pm("task", "add", "--sprint", pid, "--title", "[TEST] retry the setup").returncode == 0
    assert pid in shown_sprints(repo) and "3 open sprints" in repo.pm("show").stdout


def test_a_dropped_proposal_keeps_its_reason_and_a_reopen_makes_it_a_proposal_again(repo):
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    pid = proposed(repo)
    res = repo.pm("sprint", "close", pid)
    assert res.returncode != 0 and f"sprint {pid} is a proposal nobody started; drop it with" in res.stderr
    assert repo.pm("sprint", "close", "repo-demo.1", "--dropped", "--reason", "x").returncode != 0
    res = repo.pm("sprint", "close", pid, "--dropped", "--reason", "duplicate of repo-demo.1")
    assert res.returncode == 0, res.stderr
    item = repo.items()[pid]
    assert (item["status"], item["resolution"], item["close_reason"]) == ("closed", "dismissed",
                                                                         "duplicate of repo-demo.1")
    assert repo.pm("check").returncode == 0  # its record keeps "Not closed yet.": a drop writes no report
    res = repo.pm("sprint", "reopen", pid, "--text-file", "-", stdin="Reopen it.\nIt was no duplicate.\n")
    assert res.returncode == 0, res.stderr
    assert repo.items()[pid]["status"] == "open" and pid not in shown_sprints(repo)


def test_a_started_proposal_closes_as_a_sprint_and_refuses_dropped(repo):
    pid = proposed(repo)
    assert repo.pm("task", "add", "--sprint", pid, "--title", "[TEST] retry the setup").returncode == 0
    res = repo.pm("sprint", "close", pid, "--dropped", "--reason", "x")
    assert res.returncode != 0 and f"sprint {pid} is not a proposal nobody started" in res.stderr


def test_project_close_names_the_drop_of_an_open_proposal(repo):
    for iid in ("repo-demo.1", "repo-demo.1.2", "repo-demo.2"):
        repo.set_issue(iid, status="closed", closed_at="2026-10-02T12:00:00Z")
    pid = proposed(repo)
    project = (repo.records / "projects/demo.md").read_text()
    repo.write("projects/demo.md", project.replace("Not closed yet.", "Done: shipped."))
    repo.commit("outcome")
    res = repo.pm("project", "close", "demo")
    assert res.returncode != 0
    assert f'{pid} (Sprint 3: Fix the sync flake; a proposal: pm sprint close {pid} --dropped --reason "…")' in res.stderr


def fill(text: str, values: dict[str, str]) -> str:
    """text with each placeholder of values replaced, in values' order."""
    for k, v in values.items():
        text = text.replace(k, v)
    return text


def run_cases(repo, cases: list[dict]) -> None:
    """Run each case with a fresh proposal as {P}: allowed leaves it a proposal, starts gives it a task and puts it in
    pm show, drops closes it as dismissed, refused fails and changes nothing of it; a case's says is in its output."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1")
    write_config(repo.root, site_url="https://pm.example.test")  # what pm record link prints
    (repo.records / "design").mkdir(exist_ok=True)
    repo.write("design/parser.md", "---\ntype: design\ntitle: The parser\nproject: demo\n---\n\n" + "".join(
        f"## {name}\n\nNone yet.\n\n" for name in ("Problem", "Goals and non-goals", "Constraints and key facts",
                                                   "Design", "Alternatives considered", "Open questions")))
    repo.commit("the parser page and the site")
    res = repo.pm("project", "open", "other", "--title", "Other", "--text=Another standing goal.")
    assert res.returncode == 0, res.stderr
    failures = []
    for n, case in enumerate(cases):
        pid = proposed(repo, f"Proposal {n}")
        record = f"records/sprints/demo-{repo.items()[pid]['number']}.md"
        task = ""
        if any("{T}" in a for a in case["argv"]):
            out = repo.pm("task", "add", "--sprint", "repo-demo.2", "--title", f"[TEST] task {n}").stdout
            task = re.search(r"repo-demo\.2\.\d+", out).group(0)
        values = {"{P_RECORD}": record, "{P}": pid, "{S}": "repo-demo.1", "{T}": task}  # {P_RECORD} before {P}

        for argv in case.get("setup", []):
            res = repo.pm(*(fill(a, values) for a in argv))
            assert res.returncode == 0, (case, res.stderr)
        if case.get("edit"):
            path = repo.root / record
            path.write_text(path.read_text().replace("It fails one run in ten.", "It fails in setup."))
        before = repo.items()[pid]
        res = repo.pm(*(fill(a, values) for a in case["argv"]), stdin=case.get("stdin"))
        after = repo.items()
        it, kids = after[pid], [i for i in after.values() if i.get("parent") == pid and i["type"] == "task"]
        ok = {"allowed": res.returncode == 0 and it["status"] == "open" and not kids,
              "starts": res.returncode == 0 and bool(kids) and pid in shown_sprints(repo),
              "drops": res.returncode == 0 and (it["status"], it["resolution"]) == ("closed", "dismissed"),
              "refused": res.returncode != 0 and it == before}[case["outcome"]]
        says = fill(case.get("says", ""), values)
        if not ok or says not in res.stdout + res.stderr:
            failures.append(f"pm {' '.join(fill(a, values) for a in case['argv'])}: want {case['outcome']}"
                            f"{', saying ' + repr(says) if says else ''}; exit {res.returncode}, "
                            f"stdout {res.stdout[-300:]!r}, stderr {res.stderr[-300:]!r}")
    assert not failures, "\n".join(failures)


def test_every_item_id_command_on_a_proposal(repo):
    run_cases(repo, [c for c in CASES if not c.get("service")])


@pytest.mark.integration
def test_the_served_commands_on_a_proposal(repo):
    repo.stop_service()  # a service that serves the site holds the work store instead
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root, env=dict(repo.env, PORT="0"),
                           stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        repo.env = dict(repo.env, PORT=re.search(r"http://localhost:(\d+)", srv.stdout.readline()).group(1))
        run_cases(repo, [c for c in CASES if c.get("service")])
    finally:
        srv.terminate()
        srv.wait()
        repo.start_service()
