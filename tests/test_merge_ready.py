"""release/merge_ready.py: a PR is ready to merge only when its head contains its base branch as the remote has it
now and every check on that head passed. Against a local bare origin (refs/pull/N/head as GitHub serves it) and the
fake gh."""

from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import fake_env

pytestmark = pytest.mark.integration  # a bare origin

PM_DIR = Path(__file__).resolve().parents[1]
SCRIPT = PM_DIR / "release" / "merge_ready.py"
spec = importlib.util.spec_from_file_location("merge_ready", SCRIPT)
merge_ready = importlib.util.module_from_spec(spec)
spec.loader.exec_module(merge_ready)
PASSED = [{"__typename": "CheckRun", "workflowName": "pm tests", "name": "light", "status": "COMPLETED",
           "conclusion": "SUCCESS"},
          {"__typename": "CheckRun", "workflowName": "pm go", "name": "race", "status": "COMPLETED",
           "conclusion": "SUCCESS"},
          {"__typename": "CheckRun", "workflowName": "pm changelog", "name": "changelog", "status": "COMPLETED",
           "conclusion": "SKIPPED"},
          {"__typename": "StatusContext", "context": "ci/other", "state": "SUCCESS"}]


class Clone:
    def __init__(self, tmp: Path):
        self.tmp, self.dir = tmp, tmp / "work"
        self.env = dict(fake_env(tmp, os.environ), GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@t",
                        GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@t")
        subprocess.run(["git", "init", "-q", "--bare", "-b", "main", str(tmp / "origin.git")], check=True)
        subprocess.run(["git", "init", "-q", "-b", "main", str(self.dir)], check=True)
        self.git("remote", "add", "origin", str(tmp / "origin.git"))
        self.commit("base")
        self.git("push", "-q", "origin", "main")

    def git(self, *args: str) -> str:
        return subprocess.run(["git", *args], cwd=self.dir, env=self.env, check=True, capture_output=True,
                              text=True).stdout.strip()

    def commit(self, name: str) -> str:
        (self.dir / name).write_text(name)
        self.git("add", name)
        self.git("commit", "-q", "-m", name)
        return self.git("rev-parse", "HEAD")

    def open_pr(self, n: int, branch: str, checks=PASSED, state="OPEN", head: str | None = None) -> str:
        """Push branch as PR n's head, as GitHub keeps it, and let the fake gh describe the PR."""
        sha = self.git("rev-parse", branch)
        self.git("push", "-q", "-f", "origin", f"{branch}:refs/pull/{n}/head")
        Path(self.env["FAKE_GH_STATE"]).write_text(json.dumps({str(n): {
            "state": state, "headRefOid": head or sha, "baseRefName": "main", "statusCheckRollup": checks}}))
        return sha

    def ready(self, n: int) -> subprocess.CompletedProcess:
        return subprocess.run([sys.executable, str(SCRIPT), str(n)], cwd=self.dir, env=self.env, capture_output=True,
                              text=True)


@pytest.fixture
def clone(tmp_path) -> Clone:
    return Clone(tmp_path)


def test_a_pr_whose_head_contains_main_and_whose_checks_passed_is_ready(clone):
    clone.git("checkout", "-q", "-b", "fix")
    sha = clone.commit("fix")
    clone.open_pr(7, "fix")
    r = clone.ready(7)
    assert r.returncode == 0, r.stderr
    assert "contains origin/main" in r.stdout and "every check on it passed" in r.stdout
    assert f"gh pr merge 7 --squash --match-head-commit {sha}" in r.stdout


def test_a_pr_cut_before_main_moved_is_refused_until_it_is_rebased(clone):
    clone.git("checkout", "-q", "-b", "fix")
    clone.commit("fix")
    clone.git("checkout", "-q", "main")
    main = clone.commit("other")  # another PR merged meanwhile
    clone.git("push", "-q", "origin", "main")
    sha = clone.open_pr(7, "fix")
    r = clone.ready(7)
    assert r.returncode == 1
    assert f"its head {sha[:7]} does not contain origin/main {main[:7]}" in r.stderr
    clone.git("rebase", "-q", "main", "fix")
    clone.open_pr(7, "fix")
    assert clone.ready(7).returncode == 0


def test_a_pr_whose_checks_did_not_all_pass_is_refused_naming_each(clone):
    clone.git("checkout", "-q", "-b", "fix")
    clone.commit("fix")
    clone.open_pr(7, "fix", checks=PASSED + [
        {"__typename": "CheckRun", "workflowName": "pm go", "name": "vet", "status": "IN_PROGRESS", "conclusion": ""},
        {"__typename": "CheckRun", "workflowName": "pm tests", "name": "integration", "status": "COMPLETED",
         "conclusion": "FAILURE"},
        {"__typename": "StatusContext", "context": "ci/other", "state": "PENDING"}])
    r = clone.ready(7)
    assert r.returncode == 1
    assert "- check pm go / vet is IN_PROGRESS" in r.stderr
    assert "- check pm tests / integration concluded FAILURE" in r.stderr
    assert "- status 'ci/other' is PENDING" in r.stderr
    assert "does not contain" not in r.stderr
    clone.open_pr(7, "fix", checks=[])
    r = clone.ready(7)
    assert r.returncode == 1
    for workflow in ("pm changelog", "pm go", "pm tests"):
        assert f"no check of workflow {workflow!r}, which runs on every PR, is on its head yet" in r.stderr


def test_the_workflows_every_pr_runs_must_each_have_a_check_on_the_head(clone):
    assert merge_ready.pr_workflows(PM_DIR) == {"pm changelog", "pm go", "pm tests"}  # not pm release build: paths
    clone.git("checkout", "-q", "-b", "fix")
    clone.commit("fix")
    clone.open_pr(7, "fix", checks=[c for c in PASSED if c.get("workflowName") != "pm go"])
    r = clone.ready(7)
    assert r.returncode == 1 and "no check of workflow 'pm go'" in r.stderr


def test_a_check_that_ran_twice_on_the_head_is_judged_by_its_latest_run(clone):
    def run(conclusion: str, started: str, status: str = "COMPLETED") -> dict:
        return {"__typename": "CheckRun", "workflowName": "pm changelog", "name": "changelog", "status": status,
                "conclusion": conclusion, "startedAt": f"2026-10-10T15:{started}Z",
                "completedAt": f"2026-10-10T15:{started}Z" if status == "COMPLETED" else "0001-01-01T00:00:00Z"}

    clone.git("checkout", "-q", "-b", "fix")
    clone.commit("fix")
    others = [c for c in PASSED if c.get("workflowName") != "pm changelog"]
    # a run cancelled by a later one (a label added), or a failure re-run: the later run decides
    clone.open_pr(7, "fix", checks=others + [run("SUCCESS", "21:05"), run("CANCELLED", "20:55")])
    assert clone.ready(7).returncode == 0
    clone.open_pr(7, "fix", checks=others + [run("FAILURE", "20:55"), run("SUCCESS", "21:05")])
    assert clone.ready(7).returncode == 0
    clone.open_pr(7, "fix", checks=others + [run("SUCCESS", "20:55"), run("FAILURE", "21:05")])
    r = clone.ready(7)
    assert r.returncode == 1 and "check pm changelog / changelog concluded FAILURE" in r.stderr
    # a re-run still going: pending, whatever the earlier run gave
    clone.open_pr(7, "fix", checks=others + [run("SUCCESS", "20:55"), run("", "21:05", status="QUEUED")])
    r = clone.ready(7)
    assert r.returncode == 1 and "check pm changelog / changelog is QUEUED" in r.stderr


def test_a_closed_pr_or_one_whose_head_moved_is_refused(clone):
    clone.git("checkout", "-q", "-b", "fix")
    clone.commit("fix")
    clone.open_pr(7, "fix", state="MERGED")
    r = clone.ready(7)
    assert r.returncode == 1 and "it is MERGED" in r.stderr
    clone.open_pr(7, "fix", head="0" * 40)
    r = clone.ready(7)
    assert r.returncode == 1 and "its head moved on the remote" in r.stderr
